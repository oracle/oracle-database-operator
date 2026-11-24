#!/usr/bin/env bash
set -Eeuo pipefail
script_dir="$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
source "$script_dir/lib.sh"
sharding_case_init
SHARDING_PHASE_DEADLINE=$((SECONDS + SHARDING_TIMEOUT_SECONDS))
wait_for_marker() {
  local pod="$1" marker="$2" source="$3"
  while (( SECONDS < SHARDING_PHASE_DEADLINE )); do
    if sharding_case_log_contains "$pod" "$marker" "$source"; then
      echo "$(sharding_case_label): $pod reported '$marker'"; return 0
    fi
    echo "$(sharding_case_label): waiting for $pod: '$marker'"
    sharding_case_sleep "$SHARDING_POLL_INTERVAL"
  done
  sharding_case_fail "Timed out waiting for $pod: '$marker'"
}
for pod in catalog-0 pshard1-0 pshard2-0; do
  wait_for_marker "$pod" 'DATABASE IS READY TO USE!' container-log
done
wait_for_marker catalog-0 'GSM Catalog Setup Completed' setup-log
for pod in pshard1-0 pshard2-0; do
  wait_for_marker "$pod" 'GSM Shard Setup Completed' setup-log
done
for pod in gsm1-0 gsm2-0; do
  wait_for_marker "$pod" 'GSM Setup Completed' setup-log
done
for pod in catalog-0 pshard1-0 pshard2-0 gsm1-0 gsm2-0; do
  until sharding_kubectl get pod "$pod" -o json | jq -e '
    .metadata.deletionTimestamp == null and .status.phase == "Running" and
    any(.status.conditions[]?; .type == "Ready" and .status == "True") and
    (.status.containerStatuses | length > 0) and all(.status.containerStatuses[]; .ready == true)' >/dev/null; do
    (( SECONDS < SHARDING_PHASE_DEADLINE )) || { sharding_case_fail "Pod $pod did not become Ready"; exit 1; }
    sharding_case_sleep "$SHARDING_POLL_INTERVAL"
  done
done
echo "$(sharding_case_label): all five pods are Ready and setup markers were observed"
