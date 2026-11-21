#!/usr/bin/env bash
set +x
set -Eeuo pipefail
script_dir="$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
source "$script_dir/lib.sh"
sharding_case_init
SHARDING_PHASE_DEADLINE=$((SECONDS + SHARDING_CLEANUP_TIMEOUT_SECONDS))
# Only NotFound is absence. Authentication/network/RBAC errors abort cleanup.
cr="$(sharding_kubectl get shardingdatabase "$SHARDING_NAME" --ignore-not-found -o json)"
if [[ -n "$cr" ]]; then
  jq -e '.metadata.labels["e2e.oracle.com/owner"] == "operator-e2e" and
    .metadata.labels["e2e.oracle.com/controller"] == "sharding" and
    (.metadata.labels["e2e.oracle.com/test-id"] == "01-system-native-minimal" or
     .metadata.labels["e2e.oracle.com/test-id"] == "01-system-dg-minimal") and
    .spec.isDeleteOraPvc == true' <<<"$cr" >/dev/null || {
    sharding_case_fail 'Refusing cleanup: CR is not a recognized E2E minimal case with isDeleteOraPvc=true'; exit 1;
  }
  uid="$(jq -r '.metadata.uid' <<<"$cr")"
  old_secret="$(jq -r '.spec.dbSecret.name // empty' <<<"$cr")"
  # Snapshot actual owned resources, including Native pshard3 if replacing that case.
  sharding_kubectl get pods,statefulsets,services,pvc -o json >"$SHARDING_RESULTS_DIR/pre-cleanup-resources.json"
  jq -r --arg uid "$uid" '
    .items as $all |
    ([$uid] + [$all[] | select(.kind=="StatefulSet" and any(.metadata.ownerReferences[]?; .uid==$uid)) | .metadata.uid]) as $owners |
    [$all[] | select(any(.metadata.ownerReferences[]?; .uid as $u | $owners | index($u)))] as $owned |
    ([$owned[] | (.kind+"/"+.metadata.name)] +
     [$owned[] | select(.kind=="Pod") | .spec.volumes[]? | .persistentVolumeClaim.claimName // empty | "PersistentVolumeClaim/"+.]) |
    unique[]' "$SHARDING_RESULTS_DIR/pre-cleanup-resources.json" >"$SHARDING_RESULTS_DIR/cleanup-resources.txt"
  sharding_kubectl delete shardingdatabase "$SHARDING_NAME" --cascade=foreground --wait=false
  mapfile -t resources <"$SHARDING_RESULTS_DIR/cleanup-resources.txt"
  resources+=("shardingdatabase/$SHARDING_NAME")
  for resource in "${resources[@]}"; do
    while :; do
      existing="$(sharding_kubectl get "$resource" --ignore-not-found -o name)"
      [[ -n "$existing" ]] || break
      (( SECONDS < SHARDING_PHASE_DEADLINE )) || {
        sharding_case_fail "Cleanup timed out waiting for $resource; do not redeploy over retained storage"; exit 1;
      }
      sharding_case_sleep "$SHARDING_POLL_INTERVAL"
    done
  done
  echo "$(sharding_case_label): old CR and recorded dependencies removed"
  if [[ -n "$old_secret" ]]; then
    echo "$(sharding_case_label): old DB Secret $old_secret is deleted only if it has DG case ownership labels"
  fi
fi
# Remove only unreferenced Secrets created by this DG case, including a failed run
# where manifest application did not create a CR. Never inspect/export Secret data.
selector="e2e.oracle.com/owner=operator-e2e,e2e.oracle.com/test-id=01-system-dg-minimal,e2e.oracle.com/resource=$SHARDING_NAME"
secret_names="$(sharding_kubectl get secrets -l "$selector" -o jsonpath='{range .items[*]}{.metadata.name}{"\n"}{end}')"
remaining_crs="$(sharding_kubectl get shardingdatabases -o json)"
while IFS= read -r secret; do
  [[ -n "$secret" ]] || continue
  [[ "$secret" != "$SHARDING_DB_PULL_SECRET" && "$secret" != "$SHARDING_GSM_PULL_SECRET" ]] || continue
  if jq -e --arg s "$secret" 'any(.items[]; .spec.dbSecret.name==$s)' <<<"$remaining_crs" >/dev/null; then
    continue
  fi
  sharding_kubectl delete secret "$secret" --ignore-not-found
 done <<<"$secret_names"
echo "$(sharding_case_label): cleanup completed; namespace and OCR pull Secrets retained"
