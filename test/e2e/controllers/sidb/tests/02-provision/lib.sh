#!/usr/bin/env bash

# SIDB test02 uses the shared numbered SIDB lifecycle with TDE and
# TCPS-specific prerequisites and manifest values.
test02_dir="$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
export SIDB_CASE_ID=test02
export SIDB_CASE_NAME=02-provision
export SIDB_TEST_NAME="${SIDB_TEST02_NAME:-sidb-e2e-primary-test02}"
export SIDB_CASE_MANIFEST="${SIDB_TEST02_MANIFEST:-$test02_dir/test02.yaml}"
export SIDB_CASE_ENV_FILE="${SIDB_CASE_ENV_FILE:-${TMPDIR:-/tmp}/sidb-ns-test02.env}"
export SIDB_TDE_SECRET="${SIDB_TDE_SECRET:-sidb-e2e-tde-test02}"
export SIDB_TDE_SECRET_KEY="${SIDB_TDE_SECRET_KEY:-tde_wallet_pwd}"
export SIDB_TDE_PASSWORD="${SIDB_TDE_PASSWORD:-E2e_Tde_${CI_PIPELINE_ID:-local}_Aa1}"
export SIDB_TCPS_TLS_SECRET="${SIDB_TCPS_TLS_SECRET:-sidb-e2e-tcps-tls-test02}"

# shellcheck disable=SC1091
source "$test02_dir/../common/lib.sh"
unset test02_dir
