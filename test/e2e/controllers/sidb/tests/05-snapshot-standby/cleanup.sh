#!/usr/bin/env bash
# Operation cases share test03 resources. Finalization collects logs and retains them.
set -Eeuo pipefail
script_dir="$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
source "$script_dir/lib.sh"
sidb_dg_ops_init_config
sidb_dg_ops_collect_diagnostics final
echo "$(sidb_case_label): shared test03 resources retained; cleanup is an explicit separate operation"
