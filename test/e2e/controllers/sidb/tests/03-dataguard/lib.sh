#!/usr/bin/env bash
# Basic same-cluster TCP Data Guard configuration, isolated from test01/test02.
sidb_test03_dir="$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
export SIDB_CASE_ID=test03
export SIDB_CASE_NAME=03-dataguard
export SIDB_CASE_DIR="$sidb_test03_dir"
export SIDB_CASE_ENV_FILE="${SIDB_TEST03_ENV_FILE:-${SIDB_CASE_ENV_FILE:-${TMPDIR:-/tmp}/sidb-ns-test03.env}}"
export SIDB_DG_PRIMARY_NAME="${SIDB_TEST03_PRIMARY_NAME:-sidb-e2e-primary-test03}"
export SIDB_DG_STANDBY_NAME="${SIDB_TEST03_STANDBY_NAME:-sidb-e2e-standby-test03}"
export SIDB_DG_BROKER_NAME="${SIDB_TEST03_BROKER_NAME:-sidb-e2e-dg-test03}"
export SIDB_TEST_NAME="$SIDB_DG_PRIMARY_NAME"
export SIDB_TEST_SID="${SIDB_TEST03_PRIMARY_SID:-PRIMDB}"
export SIDB_TEST_PDB="${SIDB_TEST03_PRIMARY_PDB:-PRIMPDB}"
export SIDB_DG_STANDBY_SID="${SIDB_TEST03_STANDBY_SID:-STBYDB}"
export SIDB_DG_STANDBY_PDB="${SIDB_TEST03_STANDBY_PDB:-STBYPDB}"
export SIDB_STORAGE_SIZE="${SIDB_TEST03_PRIMARY_STORAGE_SIZE:-100Gi}"
export SIDB_DG_STANDBY_STORAGE_SIZE="${SIDB_TEST03_STANDBY_STORAGE_SIZE:-200Gi}"
export SIDB_ADMIN_SECRET="${SIDB_TEST03_ADMIN_SECRET:-sidb-e2e-admin-test03}"
export SIDB_IMAGE_PULL_POLICY="${SIDB_IMAGE_PULL_POLICY:-Always}"
export SIDB_TDE_SECRET="" SIDB_TCPS_TLS_SECRET=""
export SIDB_CASE_MANIFEST="${SIDB_TEST03_PRIMARY_MANIFEST:-$sidb_test03_dir/test03.yaml}"
export SIDB_DG_STANDBY_MANIFEST="${SIDB_TEST03_STANDBY_MANIFEST:-$sidb_test03_dir/standby.yaml}"
export SIDB_REQUIRE_HEALTHY=true
export SIDB_EXPECTED_ROLE=PRIMARY
# Numbered tests retain resources. cleanup.sh is an explicit separate operation.
export SIDB_CASE_CLEANUP="${SIDB_CASE_CLEANUP:-false}"
source "$sidb_test03_dir/../common/lib.sh"
source "$sidb_test03_dir/../common/dataguard-lib.sh"
unset sidb_test03_dir
