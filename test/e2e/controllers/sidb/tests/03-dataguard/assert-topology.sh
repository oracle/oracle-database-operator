#!/usr/bin/env bash
set -Eeuo pipefail
script_dir="$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
source "$script_dir/lib.sh"
sidb_dg_init_config
SIDB_CASE_STAGE=assert-topology
trap sidb_dg_exit_snapshot EXIT
sidb_dg_assert_database "$SIDB_DG_PRIMARY_NAME" PRIMARY "$SIDB_TEST_SID" "$SIDB_TEST_PDB" primary
sidb_dg_assert_database "$SIDB_DG_STANDBY_NAME" PHYSICAL_STANDBY "$SIDB_DG_STANDBY_SID" "$SIDB_DG_STANDBY_PDB" standby
broker_json="$SIDB_CASE_RESULTS_DIR/broker-final.json"
sidb_case_kubectl -n "$SIDB_CASE_NAMESPACE" get dataguardbroker "$SIDB_DG_BROKER_NAME" -o json > "$broker_json"
sidb_dg_assert_broker_spec "$broker_json"
jq -e --arg field "$SIDB_DG_BROKER_HEALTH_FIELD" \
  '(.status[$field] | ascii_upcase) == "HEALTHY"' "$broker_json" >/dev/null
sidb_dg_collect_diagnostics passed
echo "$(sidb_case_label): topology assertions passed (PRIMARY + PHYSICAL_STANDBY + Healthy broker)"
