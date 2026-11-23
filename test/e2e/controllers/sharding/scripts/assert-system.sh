#!/usr/bin/env bash
set -euo pipefail

e2e_workload_health_file="${CI_PROJECT_DIR:-$(git rev-parse --show-toplevel 2>/dev/null || pwd)}/.gitlab/ci/e2e-workload-health.sh"
# shellcheck disable=SC1090
source "$e2e_workload_health_file"
unset e2e_workload_health_file

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

collect_startup_failure_diagnostics() {
  local reason="$1"
  kubectl -n "${ns}" get shardingdatabase "${name}" -o yaml \
    > "${results_dir}/shardingdatabase-startup-failure.yaml" 2>&1 || true
  kubectl -n "${ns}" get jobs,pods,events -o yaml \
    > "${results_dir}/startup-failure-workloads.yaml" 2>&1 || true
  kubectl -n "${ns}" describe pods \
    > "${results_dir}/startup-failure-pods-describe.txt" 2>&1 || true
  printf 'reason=%s\nnamespace=%s\nname=%s\n' \
    "$reason" "$ns" "$name" > "${results_dir}/startup-failure-info.txt"
}

get_state() {
  kubectl -n "${ns}" get shardingdatabase "${name}" -o jsonpath='{.status.state}' 2>/dev/null || true
}

wait_for_success_state() {
  local timeout_secs=5400
  local sleep_secs=20
  local elapsed=0
  local startup_failure_since=-1

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

    local startup_failure_reason startup_failure_status
    startup_failure_reason="$(e2e_workload_failure_reason kubectl "${ns}" \
      "app.kubernetes.io/instance=${name}" || true)"
    if e2e_workload_failure_grace_update startup_failure_since \
      "${startup_failure_reason}" "${elapsed}" "Sharding ${name}"; then
      :
    else
      startup_failure_status=$?
      if [ "${startup_failure_status}" -eq 2 ]; then
        collect_startup_failure_diagnostics "${startup_failure_reason}"
        fail "Workload remained in startup failure for $(e2e_workload_failure_grace_seconds)s: ${startup_failure_reason}"
      fi
    fi
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
