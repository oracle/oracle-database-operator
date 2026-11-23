#!/usr/bin/env bash
set -Eeuo pipefail

script_dir="$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC1091
source "$script_dir/lib.sh"

sidb_case_load_env
sidb_case_init_config
sidb_case_validate_config

# Optional target/role arguments let multi-database cases reuse this lifecycle.
SIDB_TEST_NAME="${1:-$SIDB_TEST_NAME}"
SIDB_EXPECTED_ROLE="${2:-${SIDB_EXPECTED_ROLE:-PRIMARY}}"

SIDB_CASE_STAGE=wait-ready
trap sidb_case_setup_exit_snapshot EXIT

if ! sidb_case_kubectl -n "$SIDB_CASE_NAMESPACE" get singleinstancedatabase "$SIDB_TEST_NAME" >/dev/null 2>&1; then
  echo "$(sidb_case_label): $SIDB_CASE_NAMESPACE/$SIDB_TEST_NAME has not been applied" >&2
  exit 1
fi

elapsed=0
unhealthy_since=-1
startup_failure_since=-1
while [ "$elapsed" -lt "$SIDB_TIMEOUT_SECONDS" ]; do
  status="$(sidb_case_kubectl -n "$SIDB_CASE_NAMESPACE" get singleinstancedatabase "$SIDB_TEST_NAME" \
    -o jsonpath='{.status.status}' 2>/dev/null || true)"
  role="$(sidb_case_kubectl -n "$SIDB_CASE_NAMESPACE" get singleinstancedatabase "$SIDB_TEST_NAME" \
    -o jsonpath='{.status.role}' 2>/dev/null || true)"
  echo "$(sidb_case_label) readiness: elapsed=${elapsed}s status=${status:-<empty>} role=${role:-<empty>}"

  case "$status" in
    Failed|FAILED|Error|ERROR)
      echo "$(sidb_case_label): SIDB entered failure state: $status" >&2
      exit 1
      ;;
    Unhealthy|UNHEALTHY)
      if [ "$unhealthy_since" -lt 0 ]; then
        unhealthy_since="$elapsed"
        echo "$(sidb_case_label): SIDB entered Unhealthy grace period: ${SIDB_UNHEALTHY_GRACE_SECONDS}s"
      fi
      unhealthy_elapsed=$((elapsed - unhealthy_since))
      if [ "$unhealthy_elapsed" -ge "$SIDB_UNHEALTHY_GRACE_SECONDS" ]; then
        echo "$(sidb_case_label): SIDB remained Unhealthy for ${unhealthy_elapsed}s" >&2
        sidb_case_collect_diagnostics "unhealthy-grace-expired" || true
        exit 1
      fi
      ;;
    Ready|READY|Healthy|HEALTHY|Available|AVAILABLE)
      unhealthy_since=-1
      if [ "${role// /_}" = "${SIDB_EXPECTED_ROLE:-PRIMARY}" ] &&
        { [ "${SIDB_REQUIRE_HEALTHY:-false}" != true ] || [[ "$status" = Healthy || "$status" = HEALTHY ]]; }; then
        echo "$(sidb_case_label): SIDB is ready as ${SIDB_EXPECTED_ROLE:-PRIMARY}"
        exit 0
      fi
      ;;
    *)
      unhealthy_since=-1
      ;;
  esac

  startup_failure_reason="$(e2e_workload_failure_reason \
    sidb_case_kubectl "$SIDB_CASE_NAMESPACE" "app=$SIDB_TEST_NAME" || true)"
  if e2e_workload_failure_grace_update startup_failure_since \
    "$startup_failure_reason" "$elapsed" "$(sidb_case_label)"; then
    :
  else
    startup_failure_status=$?
    if [ "$startup_failure_status" -eq 2 ]; then
      sidb_case_collect_diagnostics "startup-failure-${startup_failure_reason}" || true
      echo "$(sidb_case_label): workload remained in startup failure for $(e2e_workload_failure_grace_seconds)s: $startup_failure_reason" >&2
      exit 1
    fi
  fi

  sleep "$SIDB_POLL_INTERVAL"
  elapsed=$((elapsed + SIDB_POLL_INTERVAL))
done

sidb_case_collect_diagnostics "readiness-timeout" || true
echo "$(sidb_case_label): timed out after ${SIDB_TIMEOUT_SECONDS}s" >&2
exit 1
