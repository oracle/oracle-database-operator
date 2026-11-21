#!/usr/bin/env bash
set -Eeuo pipefail

script_dir="$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC1091
source "$script_dir/lib.sh"

sidb_case_load_env
sidb_case_init_config
sidb_case_validate_config

SIDB_CASE_STAGE=cleanup
sidb_case_collect_diagnostics before-cleanup

if [ "${SIDB_CASE_CLEANUP}" != "true" ]; then
  echo "$(sidb_case_label) cleanup disabled; retaining $SIDB_CASE_NAMESPACE/$SIDB_TEST_NAME"
  exit 0
fi
cleanup_failed=false

if ! sidb_case_kubectl -n "$SIDB_CASE_NAMESPACE" delete singleinstancedatabase "$SIDB_TEST_NAME" \
  --ignore-not-found --wait=true --timeout="${SIDB_CLEANUP_TIMEOUT_SECONDS}s"; then
  echo "$(sidb_case_label): failed to delete SIDB $SIDB_CASE_NAMESPACE/$SIDB_TEST_NAME" >&2
  echo "$(sidb_case_label): retaining credentials, namespace and saved env until SIDB deletion succeeds" >&2
  exit 1
fi

if ! sidb_case_kubectl -n "$SIDB_CASE_NAMESPACE" delete secret "$SIDB_ADMIN_SECRET" \
  --ignore-not-found; then
  echo "$(sidb_case_label): failed to delete admin Secret $SIDB_CASE_NAMESPACE/$SIDB_ADMIN_SECRET" >&2
  cleanup_failed=true
fi

if [ -n "${SIDB_TDE_SECRET:-}" ] && ! sidb_case_kubectl -n "$SIDB_CASE_NAMESPACE" delete secret "$SIDB_TDE_SECRET" \
  --ignore-not-found; then
  echo "$(sidb_case_label): failed to delete TDE Secret $SIDB_CASE_NAMESPACE/$SIDB_TDE_SECRET" >&2
  cleanup_failed=true
fi

if [ -n "${SIDB_TCPS_TLS_SECRET:-}" ] && ! sidb_case_kubectl -n "$SIDB_CASE_NAMESPACE" delete secret "$SIDB_TCPS_TLS_SECRET" \
  --ignore-not-found; then
  echo "$(sidb_case_label): failed to delete TCPS TLS Secret $SIDB_CASE_NAMESPACE/$SIDB_TCPS_TLS_SECRET" >&2
  cleanup_failed=true
fi

if [ "$cleanup_failed" = "false" ] && [ "${SIDB_CASE_NAMESPACE_CREATED}" = "true" ]; then
  if ! sidb_case_kubectl delete namespace "$SIDB_CASE_NAMESPACE" \
    --ignore-not-found --wait=true --timeout="${SIDB_CLEANUP_TIMEOUT_SECONDS}s"; then
    echo "$(sidb_case_label): failed to delete owned namespace $SIDB_CASE_NAMESPACE" >&2
    cleanup_failed=true
  fi
fi

if [ "$cleanup_failed" = "true" ]; then
  echo "$(sidb_case_label): cleanup incomplete; saved env retained for retry" >&2
  exit 1
fi
rm -f "$SIDB_CASE_ENV_FILE"
echo "$(sidb_case_label) cleanup completed"
