#!/usr/bin/env bash
# Switchover and switchback; reuse the test03 Data Guard resources.
sidb_case_wrapper_dir="$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
source "$sidb_case_wrapper_dir/../common/lib.sh"
source "$sidb_case_wrapper_dir/../common/dataguard-lib.sh"
source "$sidb_case_wrapper_dir/../common/dataguard-operations-lib.sh"
sidb_dg_ops_configure test04 04-switchover "$sidb_case_wrapper_dir"
unset sidb_case_wrapper_dir
