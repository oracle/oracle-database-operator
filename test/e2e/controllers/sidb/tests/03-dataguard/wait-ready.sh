#!/usr/bin/env bash
set -Eeuo pipefail
script_dir="$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
source "$script_dir/lib.sh"
sidb_dg_init_config
SIDB_CASE_STAGE=wait-primary
trap sidb_dg_exit_snapshot EXIT
# The dispatcher or run.sh applies ONLY test03-rendered.yaml (the primary).
bash "$SIDB_CASE_DIR/../common/wait-ready.sh" "$SIDB_DG_PRIMARY_NAME" PRIMARY
SIDB_CASE_STAGE=apply-standby
sidb_case_kubectl -n "$SIDB_CASE_NAMESPACE" apply --dry-run=server -f "$SIDB_DG_RENDERED_STANDBY" >/dev/null
sidb_case_kubectl -n "$SIDB_CASE_NAMESPACE" apply -f "$SIDB_DG_RENDERED_STANDBY"
SIDB_CASE_STAGE=wait-standby
bash "$SIDB_CASE_DIR/../common/wait-ready.sh" "$SIDB_DG_STANDBY_NAME" PHYSICAL_STANDBY
SIDB_CASE_STAGE=wait-rendered-broker-spec
sidb_dg_wait_rendered_spec
SIDB_CASE_STAGE=generate-broker
sidb_dg_generate_broker
SIDB_CASE_STAGE=apply-broker
sidb_case_kubectl -n "$SIDB_CASE_NAMESPACE" apply --dry-run=server -f "$SIDB_DG_RENDERED_BROKER" >/dev/null
sidb_case_kubectl -n "$SIDB_CASE_NAMESPACE" apply -f "$SIDB_DG_RENDERED_BROKER"
SIDB_CASE_STAGE=wait-broker
sidb_dg_wait_broker
echo "$(sidb_case_label): primary, standby and DataguardBroker reached Healthy"
