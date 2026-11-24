#!/usr/bin/env bash
# Same-cluster Data Guard with TDE + TCPS (ClusterIP), isolated from test03 TCP.
sidb_test04_dir="$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
export SIDB_CASE_ID=test04
export SIDB_CASE_NAME=04-dataguard-tcps
export SIDB_CASE_DIR="$sidb_test04_dir"
export SIDB_CASE_ENV_FILE="${SIDB_TEST04_ENV_FILE:-${SIDB_CASE_ENV_FILE:-${TMPDIR:-/tmp}/sidb-ns-test04.env}}"
export SIDB_DG_PRIMARY_NAME="${SIDB_TEST04_PRIMARY_NAME:-sidb-e2e-primary-test04}"
export SIDB_DG_STANDBY_NAME="${SIDB_TEST04_STANDBY_NAME:-sidb-e2e-standby-test04}"
export SIDB_DG_BROKER_NAME="${SIDB_TEST04_BROKER_NAME:-sidb-e2e-dg-test04}"
export SIDB_TEST_NAME="$SIDB_DG_PRIMARY_NAME"
export SIDB_TEST_SID="${SIDB_TEST04_PRIMARY_SID:-PRIMTCP}"
export SIDB_TEST_PDB="${SIDB_TEST04_PRIMARY_PDB:-PRIMPDB}"
export SIDB_DG_STANDBY_SID="${SIDB_TEST04_STANDBY_SID:-STBYTCP}"
export SIDB_DG_STANDBY_PDB="${SIDB_TEST04_STANDBY_PDB:-STBYPDB}"
export SIDB_STORAGE_SIZE="${SIDB_TEST04_PRIMARY_STORAGE_SIZE:-100Gi}"
export SIDB_DG_STANDBY_STORAGE_SIZE="${SIDB_TEST04_STANDBY_STORAGE_SIZE:-200Gi}"
export SIDB_ADMIN_SECRET="${SIDB_TEST04_ADMIN_SECRET:-sidb-e2e-admin-test04}"
export SIDB_TDE_SECRET="${SIDB_TEST04_TDE_SECRET:-sidb-e2e-tde-test04}"
export SIDB_TDE_SECRET_KEY="${SIDB_TDE_SECRET_KEY:-tde_wallet_pwd}"
# Same default pattern as sidb/test02 — required whenever SIDB_TDE_SECRET is set.
export SIDB_TDE_PASSWORD="${SIDB_TDE_PASSWORD:-E2e_Tde_${CI_PIPELINE_ID:-local}_Aa1}"
# Primary and standby share one TLS Secret (see test04.yaml / standby.yaml).
# Include standby Service DNS in the initial cert so wait-ready does not
# regenerate a new leaf after primary TCPS (that race causes intermittent ORA-29024).
export SIDB_TCPS_TLS_SECRET="${SIDB_TEST04_TCPS_TLS_SECRET:-sidb-e2e-tcps-tls-test04}"
export SIDB_TCPS_TLS_EXTRA_DNS_NAMES="${SIDB_TCPS_TLS_EXTRA_DNS_NAMES:-$SIDB_DG_STANDBY_NAME}"
export SIDB_TDE_WALLET_ZIP_KEY="${SIDB_TDE_WALLET_ZIP_KEY:-standby-wallet.zip}"
export SIDB_IMAGE_PULL_POLICY="${SIDB_IMAGE_PULL_POLICY:-Always}"
export SIDB_CASE_MANIFEST="${SIDB_TEST04_PRIMARY_MANIFEST:-$sidb_test04_dir/test04.yaml}"
export SIDB_DG_STANDBY_MANIFEST="${SIDB_TEST04_STANDBY_MANIFEST:-$sidb_test04_dir/standby.yaml}"
export SIDB_REQUIRE_HEALTHY=true
export SIDB_EXPECTED_ROLE=PRIMARY
export SIDB_CASE_CLEANUP="${SIDB_CASE_CLEANUP:-false}"
# Broker waits: reuse longer grace for WARNING→SUCCESS (same as dataguard/test01).
export SIDB_UNHEALTHY_GRACE_SECONDS="${SIDB_UNHEALTHY_GRACE_SECONDS:-3600}"
export SIDB_DG_BROKER_TIMEOUT_SECONDS="${SIDB_DG_BROKER_TIMEOUT_SECONDS:-4500}"
export SIDB_TIMEOUT_SECONDS="${SIDB_TIMEOUT_SECONDS:-4500}"
# Patch generated Broker with real *-dg-client-wallet Secrets before apply.
export SIDB_DG_REQUIRE_TCPS_WALLETS=true
source "$sidb_test04_dir/../common/lib.sh"
source "$sidb_test04_dir/../common/dataguard-lib.sh"
unset sidb_test04_dir
