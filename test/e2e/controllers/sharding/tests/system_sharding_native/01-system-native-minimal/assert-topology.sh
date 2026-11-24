#!/usr/bin/env bash
set -Eeuo pipefail
script_dir="$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib.sh
source "$script_dir/lib.sh"
sharding_case_init
trap 'rc=$?; if [ "$rc" -ne 0 ]; then sharding_case_collect_diagnostics assertion-failure || true; fi; exit "$rc"' EXIT
: "${SHARDING_GDSCTL_TIMEOUT_SECONDS:=1800}"
: "${SHARDING_GDSCTL_POLL_INTERVAL_SECONDS:=15}"

fail() {
  echo "$(sharding_case_label): ASSERT FAIL: $*" >&2
  exit 1
}

pass() {
  echo "$(sharding_case_label): ASSERT PASS: $*"
}

sharding_kubectl get shardingdatabase "$SHARDING_NAME" >/dev/null
type="$(sharding_kubectl get shardingdatabase "$SHARDING_NAME" -o jsonpath='{.spec.shardingType}')"
replication="$(sharding_kubectl get shardingdatabase "$SHARDING_NAME" -o jsonpath='{.spec.replicationType}')"
[ "$type" = SYSTEM ] || fail "Expected SYSTEM sharding, got '$type'"
[ "$replication" = NATIVE ] || fail "Expected NATIVE replication, got '$replication'"

expected_pods=(catalog-0 gsm1-0 gsm2-0 pshard1-0 pshard2-0 pshard3-0)
for pod in "${expected_pods[@]}"; do
  sharding_case_wait_pod_ready "$pod"
  ready_flags="$(sharding_kubectl get pod "$pod" -o jsonpath='{.status.containerStatuses[*].ready}')"
  [[ -n "$ready_flags" && "$ready_flags" != *false* ]] ||
    fail "Pod $pod has a container that is not ready (container readiness: '$ready_flags')"
done

expected_db_pods=(catalog-0 pshard1-0 pshard2-0 pshard3-0)
for pod in "${expected_db_pods[@]}"; do
  sharding_case_log_contains "$pod" 'DATABASE IS READY TO USE!' container-log || {
    fail "Database readiness marker missing from $pod"
  }
done
sharding_case_log_contains catalog-0 'GSM Catalog Setup Completed' setup-log || {
  fail 'Catalog setup marker missing'
}
for pod in pshard1-0 pshard2-0 pshard3-0; do
  sharding_case_log_contains "$pod" 'GSM Shard Setup Completed' setup-log || {
    fail "Shard setup marker missing from $pod"
  }
done
for pod in gsm1-0 gsm2-0; do
  sharding_case_log_contains "$pod" 'GSM Setup Completed' setup-log || {
    fail "GSM setup marker missing from $pod"
  }
done

expected_statefulsets=(catalog gsm1 gsm2 pshard1 pshard2 pshard3)
for statefulset in "${expected_statefulsets[@]}"; do
  desired="$(sharding_kubectl get statefulset "$statefulset" -o jsonpath='{.spec.replicas}')"
  ready="$(sharding_kubectl get statefulset "$statefulset" -o jsonpath='{.status.readyReplicas}')"
  [ "$desired" = 1 ] || fail "StatefulSet $statefulset expected 1 replica, got '$desired'"
  [ "$ready" = "$desired" ] || fail "StatefulSet $statefulset has $ready/$desired ready replicas"
done

assert_headless_service_ports() {
  local service="$1" expected_ports="$2" cluster_ip actual_ports port
  cluster_ip="$(sharding_kubectl get service "$service" -o jsonpath='{.spec.clusterIP}')"
  [ "$cluster_ip" = None ] || fail "Service $service should be headless (clusterIP=None), got '$cluster_ip'"
  actual_ports=" $(sharding_kubectl get service "$service" -o jsonpath='{range .spec.ports[*]}{.port}{" "}{end}') "
  for port in $expected_ports; do
    [[ "$actual_ports" == *" $port "* ]] || fail "Service $service is missing expected port $port (ports:$actual_ports)"
  done
}

assert_headless_service_ports catalog '1521 6234 6123 8080'
assert_headless_service_ports gsm1 '1522 6234 6123 8080'
assert_headless_service_ports gsm2 '1522 6234 6123 8080'
for service in pshard1 pshard2 pshard3; do
  assert_headless_service_ports "$service" '1521 6234 6123 8080'
done

# Capture the Kubernetes state shown by the test and all GDSCTL output as artifacts.
sharding_kubectl get all -o wide >"$SHARDING_RESULTS_DIR/kubernetes-all-final.txt"
sharding_kubectl get pods -o wide >"$SHARDING_RESULTS_DIR/pods-final.txt"
sharding_kubectl get services -o wide >"$SHARDING_RESULTS_DIR/services-final.txt"
sharding_kubectl get statefulsets -o wide >"$SHARDING_RESULTS_DIR/statefulsets-final.txt"
sharding_kubectl get pvc -o wide >"$SHARDING_RESULTS_DIR/pvcs-final.txt"
sharding_kubectl get shardingdatabase "$SHARDING_NAME" -o yaml >"$SHARDING_RESULTS_DIR/shardingdatabase-final.yaml"

gdsctl_output() {
  local command="$1" output_file="$2"
  sharding_kubectl exec gsm1-0 -- bash -lc "$command" >"$output_file" 2>&1
}

shards_output="$SHARDING_RESULTS_DIR/gdsctl-config-shard.txt"
sdb_output="$SHARDING_RESULTS_DIR/gdsctl-config-sdb.txt"
ru_output="$SHARDING_RESULTS_DIR/gdsctl-status-ru.txt"
gdsctl_shards_ready() {
  awk '
    $1 ~ /^pshard[0-9]+_/ {
      found++
      if ($1 != "pshard1_pshard1pdb" && $1 != "pshard2_pshard2pdb" && $1 != "pshard3_pshard3pdb") bad = 1
      if ($3 != "Ok" || $4 != "Deployed" || $5 != "primary" || $6 != "ONLINE") bad = 1
      rows[$1]++
    }
    END {
      if (found != 3 || rows["pshard1_pshard1pdb"] != 1 ||
          rows["pshard2_pshard2pdb"] != 1 || rows["pshard3_pshard3pdb"] != 1 || bad) exit 1
    }
  ' "$shards_output"
}

gdsctl_rus_ready() {
  awk '
    $1 ~ /^pshard[0-9]+_/ {
      shard = $1
      ru = $2
      role = $3
      if (shard != "pshard1_pshard1pdb" && shard != "pshard2_pshard2pdb" && shard != "pshard3_pshard3pdb") bad = 1
      if (ru !~ /^[1-6]$/ || seen[shard, ru]++ || $6 != "Ok") bad = 1
      if (role == "Leader") leaders++
      else if (role == "Follower") followers++
      else bad = 1
      rows[shard]++
    }
    END {
      if (rows["pshard1_pshard1pdb"] != 6 || rows["pshard2_pshard2pdb"] != 6 ||
          rows["pshard3_pshard3pdb"] != 6 || leaders != 6 || followers != 12 || bad) exit 1
    }
  ' "$ru_output"
}

wait_for_gdsctl_deployment() {
  local deadline remaining sleep_for attempt=0
  deadline=$((SECONDS + SHARDING_GDSCTL_TIMEOUT_SECONDS))

  while :; do
    attempt=$((attempt + 1))
    if gdsctl_output 'gdsctl config shard' "$shards_output" && gdsctl_shards_ready; then
      if gdsctl_output 'gdsctl status ru' "$ru_output" && gdsctl_rus_ready; then
        pass 'GDSCTL reports all three shards online and all 18 RUs healthy'
        return 0
      fi
    fi

    remaining=$((deadline - SECONDS))
    if [ "$remaining" -le 0 ]; then
      echo "$(sharding_case_label): timed out after ${SHARDING_GDSCTL_TIMEOUT_SECONDS}s waiting for GDSCTL shard/RU deployment state" >&2
      echo 'Last gdsctl config shard output:' >&2
      cat "$shards_output" >&2 || true
      echo 'Last gdsctl status ru output:' >&2
      cat "$ru_output" >&2 || true
      return 1
    fi

    sleep_for="$SHARDING_GDSCTL_POLL_INTERVAL_SECONDS"
    [ "$sleep_for" -lt "$remaining" ] || sleep_for="$remaining"
    echo "$(sharding_case_label): waiting for GDSCTL shards to be Deployed/ONLINE and RUs to be Ok (attempt $attempt; timeout ${SHARDING_GDSCTL_TIMEOUT_SECONDS}s)"
    sleep "$sleep_for"
  done
}

[[ "$SHARDING_GDSCTL_TIMEOUT_SECONDS" =~ ^[1-9][0-9]*$ ]] ||
  fail 'SHARDING_GDSCTL_TIMEOUT_SECONDS must be a positive integer'
[[ "$SHARDING_GDSCTL_POLL_INTERVAL_SECONDS" =~ ^[1-9][0-9]*$ ]] ||
  fail 'SHARDING_GDSCTL_POLL_INTERVAL_SECONDS must be a positive integer'

wait_for_gdsctl_deployment || fail 'GDSCTL deployment did not reach the expected shard/RU state'
gdsctl_output 'gdsctl config sdb' "$sdb_output" || {
  echo 'gdsctl config sdb command output:' >&2
  cat "$sdb_output" >&2 || true
  fail 'gdsctl config sdb failed'
}

gdsctl_sdb_has_value() {
  local expected_value="$1"
  awk -v expected="$expected_value" '
    {
      gsub(/\r/, "")
      gsub(/\033\[[0-9;]*[[:alpha:]]/, "")
      if ($1 == expected) found = 1
    }
    END { exit !found }
  ' "$sdb_output"
}

for expected_value in Native System-managed shardspaceora; do
  if ! gdsctl_sdb_has_value "$expected_value"; then
    echo "gdsctl config sdb output did not contain the expected value '$expected_value':" >&2
    cat "$sdb_output" >&2 || true
    fail "GDSCTL SDB configuration is missing '$expected_value'"
  fi
done

pass 'six pods and StatefulSets are ready; expected headless Services and ports exist'
pass 'GDSCTL SDB configuration is Native, System-managed, and uses shardspaceora'
echo "$(sharding_case_label): topology assertions passed"
trap - EXIT
