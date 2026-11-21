#!/usr/bin/env bash
# Both requests and their completion checks are captured in this stage's log.
set -Eeuo pipefail
script_dir="$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
source "$script_dir/lib.sh"
sidb_dg_ops_init_config
trap sidb_dg_ops_exit_snapshot EXIT
sidb_dg_ops_execute_case
