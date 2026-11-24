#!/usr/bin/env bash
set -Eeuo pipefail
script_dir="$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib.sh
source "$script_dir/lib.sh"
sharding_case_init
start=$SECONDS

wait_for_marker() {
  local pod="$1" marker="$2" source="$3" phase
  while (( SECONDS - start < SHARDING_TIMEOUT_SECONDS )); do
    if sharding_case_log_contains "$pod" "$marker" "$source"; then
      echo "$(sharding_case_label): $pod reported '$marker'"
      return 0
    fi
    phase="$(kubectl -n "$SHARDING_E2E_NAMESPACE" get pod "$pod" -o jsonpath='{.status.phase}' 2>/dev/null || true)"
    case "$phase" in
      Failed|Unknown)
        echo "Pod $pod entered phase $phase" >&2
        sharding_case_collect_diagnostics "pod-${pod}-${phase}" || true
        return 1
        ;;
    esac
    echo "$(sharding_case_label): waiting on $pod: '$marker' ($((SECONDS-start))/${SHARDING_TIMEOUT_SECONDS}s)"
    sleep "$SHARDING_POLL_INTERVAL"
  done
  echo "Timed out waiting for $pod: '$marker'" >&2
  sharding_case_collect_diagnostics timeout || true
  return 1
}

for pod in catalog-0 pshard1-0 pshard2-0 pshard3-0; do
  wait_for_marker "$pod" 'DATABASE IS READY TO USE!' container-log
done
wait_for_marker catalog-0 'GSM Catalog Setup Completed' setup-log
for pod in pshard1-0 pshard2-0 pshard3-0; do
  wait_for_marker "$pod" 'GSM Shard Setup Completed' setup-log
done
for pod in gsm1-0 gsm2-0; do
  wait_for_marker "$pod" 'GSM Setup Completed' setup-log
done
for pod in catalog-0 pshard1-0 pshard2-0 pshard3-0 gsm1-0 gsm2-0; do
  sharding_case_wait_pod_ready "$pod"
done
echo "$(sharding_case_label): all readiness and setup markers observed"
