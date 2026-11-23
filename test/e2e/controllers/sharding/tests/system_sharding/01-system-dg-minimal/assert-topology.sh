#!/usr/bin/env bash
# DG minimal topology assertions. Run after wait-ready.sh has checked setup logs.
set -Eeuo pipefail
script_dir="$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=/dev/null
source "$script_dir/lib.sh"
sharding_case_init
SHARDING_PHASE_DEADLINE=$((SECONDS + SHARDING_GDSCTL_TIMEOUT_SECONDS))
shards="$SHARDING_RESULTS_DIR/gdsctl-config-shard.txt"
sdb="$SHARDING_RESULTS_DIR/gdsctl-config-sdb.txt"
topology_checks="$SHARDING_RESULTS_DIR/topology-checks.tsv"
query_errors="$SHARDING_RESULTS_DIR/topology-query.stderr"
parser_errors="$SHARDING_RESULTS_DIR/topology-parser.stderr"

capture_gdsctl() {
  local command="$1" target="$2"
  sharding_kubectl exec gsm1-0 -- bash -lc "$command" >"$target" 2>"$target.stderr" || return
  sharding_gds_normalize "$target" >"$target.normalized" || return
  if grep -E '(^|[[:space:]])(GSM|ORA|TNS|SP2|ERROR)-[0-9]+' "$target.normalized" >/dev/null; then
    return 1
  fi
  return 0
}

topology_ready() {
  : >"$topology_checks"
  : >"$query_errors"
  : >"$parser_errors"
  if ! sharding_kubectl get shardingdatabase "$SHARDING_NAME" -o json \
    >"$SHARDING_RESULTS_DIR/cr.json" 2>"$query_errors"; then
    printf 'ShardingDatabase/%s\tFAIL\tCannot read resource; see topology-query.stderr\n' \
      "$SHARDING_NAME" >"$topology_checks"
    return 1
  fi
  if ! sharding_kubectl get pods,statefulsets,services,pvc -o json \
    >"$SHARDING_RESULTS_DIR/resources.json" 2>>"$query_errors"; then
    printf 'Kubernetes resources\tFAIL\tCannot read resources; see topology-query.stderr\n' \
      >"$topology_checks"
    return 1
  fi

  # Validate the deployed components. Do not depend on the stored shardInfo
  # representation, direct CR owner references, or image-specific helper ports.
  # Report each failed condition instead of reducing everything to one Boolean.
  if ! jq -r --arg name "$SHARDING_NAME" --arg ns "$SHARDING_E2E_NAMESPACE" \
    --slurpfile cr "$SHARDING_RESULTS_DIR/cr.json" '
    (.items // []) as $items | $cr[0] as $db |
    ["catalog", "gsm1", "gsm2", "pshard1", "pshard2"] as $names |
    def object($kind; $name):
      first($items[] | select(.kind == $kind and .metadata.name == $name)) // null;
    def live($resource):
      $resource != null and $resource.metadata.deletionTimestamp == null;
    def check($label; $ok; $detail):
      {check: $label, pass: ($ok == true), detail: $detail};
    [
      check("ShardingDatabase identity";
        live($db) and $db.metadata.name == $name and $db.metadata.namespace == $ns;
        "expected \($ns)/\($name), deletionTimestamp=\($db.metadata.deletionTimestamp // "none")"),
      check("ShardingDatabase configuration";
        $db.spec.shardingType == "SYSTEM" and $db.spec.replicationType == "DG";
        "shardingType=\($db.spec.shardingType // "missing"), replicationType=\($db.spec.replicationType // "missing")"),
      ($names[] as $component |
        object("StatefulSet"; $component) as $sts |
        object("Pod"; ($component + "-0")) as $pod |
        object("Service"; $component) as $svc |
        (if ($component | startswith("gsm")) then 1522 else 1521 end) as $port |
        [$pod.spec.volumes[]? | .persistentVolumeClaim.claimName // empty] as $claims |
        check("StatefulSet/\($component) readiness";
          live($sts) and ($sts.spec.replicas // 1) == 1 and
          ($sts.status.readyReplicas // 0) == 1 and
          ($sts.status.observedGeneration // 0) >= ($sts.metadata.generation // 1);
          "desired=\($sts.spec.replicas // 0), ready=\($sts.status.readyReplicas // 0), observedGeneration=\($sts.status.observedGeneration // 0), generation=\($sts.metadata.generation // 0)"),
        check("Pod/\($component)-0 readiness";
          live($pod) and $pod.status.phase == "Running" and
          any($pod.status.conditions[]?; .type == "Ready" and .status == "True") and
          (($pod.status.containerStatuses // []) | length > 0) and
          all($pod.status.containerStatuses[]?; .ready == true);
          "phase=\($pod.status.phase // "missing"), Ready=\([$pod.status.conditions[]? | select(.type == "Ready") | .status] | join(",")), containersReady=\([$pod.status.containerStatuses[]? | .ready] | tojson)"),
        check("Service/\($component) listener";
          live($svc) and any($svc.spec.ports[]?;
            .port == $port and (.protocol // "TCP") == "TCP");
          "expected TCP/\($port), ports=\([$svc.spec.ports[]? | .port] | tojson)"),
        check("Pod/\($component)-0 storage";
          ($claims | length) > 0;
          "PVCs=\($claims | join(","))"),
        ($claims[] as $claim |
          object("PersistentVolumeClaim"; $claim) as $pvc |
          check("PVC/\($claim) binding";
            live($pvc) and $pvc.status.phase == "Bound";
            "phase=\($pvc.status.phase // "missing"), deletionTimestamp=\($pvc.metadata.deletionTimestamp // "none")"))
      )
    ] | .[] |
    [.check, (if .pass then "PASS" else "FAIL" end), .detail] | @tsv
  ' "$SHARDING_RESULTS_DIR/resources.json" >"$topology_checks" 2>"$parser_errors"; then
    printf 'Topology JSON validation\tFAIL\tCannot evaluate snapshots; see topology-parser.stderr\n' \
      >"$topology_checks"
    return 1
  fi

  [ -s "$topology_checks" ] || return 1
  awk -F '\t' '$2 == "FAIL" {failed=1} END {exit (failed ? 1 : 0)}' "$topology_checks"
}

attempt=0
while (( SECONDS < SHARDING_PHASE_DEADLINE )); do
  attempt=$((attempt + 1))
  shards_ok=false
  sdb_ok=false
  kubernetes_ok=false
  if capture_gdsctl 'gdsctl config shard' "$shards" && sharding_gds_shards_ready "$shards.normalized"; then
    shards_ok=true
  fi
  if capture_gdsctl 'gdsctl config sdb' "$sdb" && sharding_gds_sdb_ready "$sdb.normalized"; then
    sdb_ok=true
  fi
  if topology_ready; then
    kubernetes_ok=true
  fi

  if [[ "$shards_ok" = true && "$sdb_ok" = true && "$kubernetes_ok" = true ]]; then
    # These display snapshots supplement the JSON checks; their capture is best effort.
    sharding_kubectl get all -o wide >"$SHARDING_RESULTS_DIR/kubernetes-all-final.txt" \
      2>"$SHARDING_RESULTS_DIR/kubernetes-all-final.stderr" || true
    sharding_kubectl get pvc -o wide >"$SHARDING_RESULTS_DIR/pvcs-final.txt" \
      2>"$SHARDING_RESULTS_DIR/pvcs-final.stderr" || true
    cat "$shards" "$sdb" "$topology_checks"
    echo "$(sharding_case_label): ASSERT PASS: two shards are Ok/Deployed/ONLINE in pshg1/primary"
    echo "$(sharding_case_label): ASSERT PASS: Data Guard, System-managed, shardspaceora"
    echo "$(sharding_case_label): ASSERT PASS: catalog, gsm1, gsm2, pshard1 and pshard2 pods/StatefulSets Ready; listener Services and Bound PVCs verified"
    exit 0
  fi

  echo "$(sharding_case_label): waiting for DG topology (attempt $attempt; shards=$shards_ok; sdb=$sdb_ok; kubernetes=$kubernetes_ok; timeout=${SHARDING_GDSCTL_TIMEOUT_SECONDS}s)"
  awk -F '\t' '$2 == "FAIL" {printf "  pending: %s: %s\n", $1, $3}' "$topology_checks"
  for output in "$query_errors" "$parser_errors"; do
    if [ -s "$output" ]; then
      cat "$output" >&2
    fi
  done
  sharding_case_sleep "$SHARDING_GDSCTL_POLL_INTERVAL_SECONDS"
done

for output in "$topology_checks" "$query_errors" "$parser_errors" \
  "$shards" "$shards.stderr" "$sdb" "$sdb.stderr"; do
  if [ -s "$output" ]; then
    echo "Last output: $output" >&2
    cat "$output" >&2
  fi
done
sharding_case_fail 'ASSERT FAIL: timed out waiting for the complete DG minimal topology; see topology-checks.tsv and GDSCTL outputs'
