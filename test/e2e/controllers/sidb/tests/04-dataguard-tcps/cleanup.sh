#!/usr/bin/env bash
set -Eeuo pipefail
script_dir="$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
source "$script_dir/lib.sh"
[ -f "$SIDB_CASE_ENV_FILE" ] || {
  echo "SIDB test04: no saved env; cleanup requires the state created by prerequisites.sh" >&2; exit 2;
}
sidb_dg_init_config
sidb_dg_collect_diagnostics before-cleanup
# Explicit cleanup can override the saved retention setting.
if [ "${SIDB_TEST04_CLEANUP:-$SIDB_CASE_CLEANUP}" != true ]; then
  echo "$(sidb_case_label): resources retained; set SIDB_TEST04_CLEANUP=true for explicit cleanup"
  exit 0
fi
# Stop on deletion failure: never remove primary/admin Secret while dependents remain.
sidb_case_kubectl -n "$SIDB_CASE_NAMESPACE" delete dataguardbroker "$SIDB_DG_BROKER_NAME" \
  --ignore-not-found --wait=true --timeout="${SIDB_CLEANUP_TIMEOUT_SECONDS}s"
sidb_case_kubectl -n "$SIDB_CASE_NAMESPACE" delete singleinstancedatabase "$SIDB_DG_STANDBY_NAME" \
  --ignore-not-found --wait=true --timeout="${SIDB_CLEANUP_TIMEOUT_SECONDS}s"
# Common cleanup removes primary, admin Secret and any namespace this test created.
# Write the override into the saved env, which the common script loads first.
SIDB_CASE_CLEANUP=true
sidb_dg_write_env
bash "$SIDB_CASE_DIR/../common/cleanup.sh"
