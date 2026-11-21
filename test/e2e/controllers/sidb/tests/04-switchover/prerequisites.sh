#!/usr/bin/env bash
set -Eeuo pipefail
script_dir="$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
source "$script_dir/lib.sh"
# Build new run state; never read a stale saved environment when preparing a case.
sidb_dg_ops_init_config --fresh
rm -f -- "$SIDB_CASE_ENV_FILE" "$SIDB_CASE_RESULTS_DIR/diagnostics/.complete"
trap sidb_dg_ops_exit_snapshot EXIT
sidb_dg_ops_prepare
