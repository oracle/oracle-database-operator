#!/usr/bin/env bash
set -Eeuo pipefail
script_dir="$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
source "$script_dir/lib.sh"
sidb_dg_ops_init_config
trap sidb_dg_ops_exit_snapshot EXIT
sidb_dg_ops_assert_final
