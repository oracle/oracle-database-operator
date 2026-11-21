#!/usr/bin/env bash
# Direct runner for an already installed operator and existing test03 resources.
set -Eeuo pipefail
script_dir="$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
[ "$#" -eq 0 ] || { echo "Usage: run.sh (shared resources are retained)" >&2; exit 2; }
source "$script_dir/lib.sh"
# Own environment/results locations for direct invocations.
export SIDB_CASE_ENV_FILE="${SIDB_TEST06_ENV_FILE:-${TMPDIR:-/tmp}/sidb-e2e-${CI_PIPELINE_ID:-local}-test06.env}"
export SIDB_CASE_RESULTS_DIR="${SIDB_TEST06_RESULTS_DIR:-${OPERATOR_LOG_DIR:-${SIDB_E2E_RESULTS_DIR:-test/e2e/controllers/sidb/results}}/cases/sidb-test06}"
sidb_case_init_config
trap sidb_dg_ops_exit_snapshot EXIT
bash "$SIDB_CASE_DIR/prerequisites.sh"
sidb_dg_ops_init_config
bash "$SIDB_CASE_DIR/verify-prerequisites.sh"
bash "$SIDB_CASE_DIR/wait-ready.sh"
bash "$SIDB_CASE_DIR/assert-topology.sh"
bash "$SIDB_CASE_DIR/cleanup.sh"
echo "$(sidb_case_label): fsfo and observer enable/disable E2E passed"
