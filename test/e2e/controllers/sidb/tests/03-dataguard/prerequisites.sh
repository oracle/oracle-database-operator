#!/usr/bin/env bash
set -Eeuo pipefail
script_dir="$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
source "$script_dir/lib.sh"
# Do not let a stale local env file override a newly chosen namespace or password.
# In CI, shared runners often leave /tmp/sidb-ns-test03.env behind; remove it when
# no DG CRs remain so retries can start without a manual cleanup.sh.
if [ -e "$SIDB_CASE_ENV_FILE" ]; then
  live_crs=0
  for resource_name in "singleinstancedatabase/$SIDB_DG_PRIMARY_NAME" \
    "singleinstancedatabase/$SIDB_DG_STANDBY_NAME" "dataguardbroker/$SIDB_DG_BROKER_NAME"; do
    if sidb_case_kubectl -n "${SIDB_E2E_NAMESPACE:-sidb-ns}" get "$resource_name" >/dev/null 2>&1; then
      live_crs=1
      break
    fi
  done
  if [ "$live_crs" -eq 1 ]; then
    echo "SIDB test03: env file already exists with live CRs: $SIDB_CASE_ENV_FILE. Use cleanup.sh or a new SIDB_TEST03_ENV_FILE." >&2
    exit 2
  fi
  echo "SIDB test03: removing stale env file without live CRs: $SIDB_CASE_ENV_FILE"
  rm -f "$SIDB_CASE_ENV_FILE"
fi
sidb_dg_init_config
# Reject leftover CRs from a prior run. An orphan admin Secret is fine:
# common/prerequisites.sh applies it with create --dry-run | apply.
for resource_name in "singleinstancedatabase/$SIDB_DG_PRIMARY_NAME" \
  "singleinstancedatabase/$SIDB_DG_STANDBY_NAME" "dataguardbroker/$SIDB_DG_BROKER_NAME"; do
  if sidb_case_kubectl -n "$SIDB_CASE_NAMESPACE" get "$resource_name" >/dev/null 2>&1; then
    echo "$(sidb_case_label): target already exists: $resource_name; clean up this test first" >&2
    exit 1
  fi
done
bash "$SIDB_CASE_DIR/../common/prerequisites.sh"
# Load generated admin password/namespace ownership before persisting extra fields.
sidb_dg_init_config
SIDB_CASE_STAGE=prerequisites
trap sidb_dg_exit_snapshot EXIT
cp "$SIDB_DG_STANDBY_MANIFEST" "$SIDB_CASE_RESULTS_DIR/source-standby.yaml"
sidb_dg_render_standby
sidb_dg_write_env
echo "$(sidb_case_label): standby template rendered: $SIDB_DG_RENDERED_STANDBY"
