#!/usr/bin/env bash
set -Eeuo pipefail

script_dir="$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"

export AGENTFACTORY_CASE_ID=test01
: "${AGENTFACTORY_TEST_TEMPLATE:=$script_dir/test01.yaml}"
export AGENTFACTORY_ASSERT_SCRIPT="$script_dir/assert.sh"

# After sidb/test01 patches TCPS onto the ready primary, consume the client
# wallet and connect on port 2484. Reuse the shared wallet preparation hook.
sidb_namespace="${SIDB_CASE_NAMESPACE:-${SIDB_E2E_NAMESPACE:-sidb-ns}}"
export SIDB_TEST_NAME="${SIDB_TEST_NAME:-sidb-e2e-primary}"
export SIDB_TEST_PDB="${SIDB_TEST_PDB:-ORCLPDB1}"
if [ -z "${AGENTFACTORY_DB_HOST:-}" ] ||
   [ "$AGENTFACTORY_DB_HOST" = "sidb-e2e-primary-test02.${sidb_namespace}.svc.cluster.local" ]; then
  export AGENTFACTORY_DB_HOST="sidb-e2e-primary.${sidb_namespace}.svc.cluster.local"
fi
export AGENTFACTORY_PREPARE_DATABASE_SCRIPT="${AGENTFACTORY_PREPARE_DATABASE_SCRIPT:-$script_dir/../02-provision/prepare-database.sh}"
export AGENTFACTORY_WALLET_TEMPLATE="${AGENTFACTORY_WALLET_TEMPLATE:-$script_dir/test01-wallet.yaml}"
export AGENTFACTORY_TEST_TEMPLATE AGENTFACTORY_ASSERT_SCRIPT

# Prefer DeleteOnSuccess for the temporary administrator Secret once the
# controller wallet bootstrap Job completes.
if [ -z "${AGENTFACTORY_BOOTSTRAP_CLEANUP_POLICY:-}" ]; then
  export AGENTFACTORY_BOOTSTRAP_CLEANUP_POLICY=DeleteOnSuccess
fi
if [ -z "${AGENTFACTORY_BOOTSTRAP_PRESERVE_FAILED_JOB:-}" ]; then
  export AGENTFACTORY_BOOTSTRAP_PRESERVE_FAILED_JOB=true
fi

exec "$script_dir/../common/run.sh"
