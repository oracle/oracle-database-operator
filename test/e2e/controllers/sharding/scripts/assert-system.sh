#!/usr/bin/env bash
set -euo pipefail

ns="${1:?namespace required}"
name="${2:?shardingdatabase name required}"
results_dir="${3:-test/e2e/controllers/sharding/results}"

mkdir -p "${results_dir}"

fail() {
  echo "ASSERT FAIL: $*"
  exit 1
}

pass() {
  echo "ASSERT PASS: $*"
}

get_state() {
  kubectl -n "${ns}" get shardingdatabase "${name}" -o jsonpath='{.status.state}' 2>/dev/null || true
}

wait_for_success_state() {
  local timeout_secs=5400
  local sleep_secs=20
  local elapsed=0

  while [ "${elapsed}" -lt "${timeout_secs}" ]; do
    local state
    state="$(get_state)"
    case "${state}" in
      AVAILABLE|Available|READY|Ready|ONLINE|Online)
        pass "ShardingDatabase reached success state: ${state}"
        return 0
        ;;
      FAILED|Failed|ERROR|Error)
        fail "ShardingDatabase entered failure state: ${state}"
        ;;
    esac
    sleep "${sleep_secs}"
    elapsed=$((elapsed + sleep_secs))
  done
  fail "Timed out waiting for sharding success state"
}

wait_for_success_state

pod_count="$(kubectl -n "${ns}" get pods --no-headers 2>/dev/null | wc -l | tr -d ' ')"
[ "${pod_count}" -gt 0 ] || fail "Expected pods to be created, found ${pod_count}"
pass "Found ${pod_count} pod(s)"

running_count="$(kubectl -n "${ns}" get pods --no-headers 2>/dev/null | awk '$3=="Running"{c++} END{print c+0}')"
[ "${running_count}" -gt 0 ] || fail "Expected at least one running pod"
pass "Found ${running_count} running pod(s)"

pvc_count="$(kubectl -n "${ns}" get pvc --no-headers 2>/dev/null | wc -l | tr -d ' ')"
[ "${pvc_count}" -gt 0 ] || fail "Expected PVCs to be created, found ${pvc_count}"
pass "Found ${pvc_count} PVC(s)"

kubectl -n "${ns}" get shardingdatabase "${name}" -o yaml > "${results_dir}/shardingdatabase-${name}.yaml" || true
kubectl -n "${ns}" get pods -o wide > "${results_dir}/pods.txt" || true
kubectl -n "${ns}" get pvc -o wide > "${results_dir}/pvc.txt" || true
