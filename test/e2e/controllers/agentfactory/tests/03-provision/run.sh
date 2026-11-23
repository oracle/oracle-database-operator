#!/usr/bin/env bash
set -Eeuo pipefail

script_dir="$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"

export AGENTFACTORY_CASE_ID=test03
if [ -z "${AGENTFACTORY_NAME:-}" ] || [[ "$AGENTFACTORY_NAME" == agentfactory-e2e-test* ]]; then
  export AGENTFACTORY_NAME=agentfactory-e2e-test03
fi
if [ -z "${AGENTFACTORY_RUNTIME_SECRET:-}" ] || [[ "$AGENTFACTORY_RUNTIME_SECRET" == agentfactory-runtime-secret-test* ]]; then
  export AGENTFACTORY_RUNTIME_SECRET=agentfactory-runtime-secret-test03
fi
if [ -z "${AGENTFACTORY_BOOTSTRAP_SECRET:-}" ] || [[ "$AGENTFACTORY_BOOTSTRAP_SECRET" == paf-db-bootstrap-admin-test* ]]; then
  export AGENTFACTORY_BOOTSTRAP_SECRET=paf-db-bootstrap-admin-test03
fi
if [ -z "${AGENTFACTORY_PVC_NAME:-}" ] || [[ "$AGENTFACTORY_PVC_NAME" == agentfactory-e2e-test*-pvc ]]; then
  export AGENTFACTORY_PVC_NAME=agentfactory-e2e-test03-pvc
fi
# Test03 is deliberately isolated from the Wallet/TCPS test02 environment.
# Force the complete Details connection tuple instead of allowing a CI
# variable or a caller's inherited shell state to select TCPS.
export AGENTFACTORY_DB_CONNECTION_TYPE=Details
export AGENTFACTORY_DB_PROTOCOL=TCP
export AGENTFACTORY_DB_PORT=1521
export AGENTFACTORY_DB_HAS_OCI_CERTIFICATES=false
unset AGENTFACTORY_DB_SELECTED_TNS_ALIAS
unset AGENTFACTORY_PREPARE_DATABASE_SCRIPT
export AGENTFACTORY_PUBLIC_LB_ENABLED=true

sidb_namespace="${SIDB_CASE_NAMESPACE:-${SIDB_E2E_NAMESPACE:-sidb-ns}}"
if [ -z "${SIDB_TEST_NAME:-}" ] || [[ "$SIDB_TEST_NAME" == sidb-e2e-primary-test* ]]; then
  export SIDB_TEST_NAME=sidb-e2e-primary
fi
: "${SIDB_TEST_PDB:=ORCLPDB1}"
if [ -z "${AGENTFACTORY_DB_HOST:-}" ] ||
   [[ "$AGENTFACTORY_DB_HOST" == sidb-e2e-primary-test*."$sidb_namespace".svc.cluster.local ]]; then
  export AGENTFACTORY_DB_HOST="sidb-e2e-primary.${sidb_namespace}.svc.cluster.local"
fi
export SIDB_TEST_PDB

if [ -z "${AGENTFACTORY_PUBLIC_LB_IP:-}" ]; then
  echo "Agent Factory test03: AGENTFACTORY_PUBLIC_LB_IP must be a pre-allocated LoadBalancer IP" >&2
  exit 2
fi

export AGENTFACTORY_TEST_TEMPLATE="$script_dir/test03.yaml"
export AGENTFACTORY_ASSERT_SCRIPT="$script_dir/assert.sh"

if [ -z "${AGENTFACTORY_BOOTSTRAP_CLEANUP_POLICY:-}" ]; then
  export AGENTFACTORY_BOOTSTRAP_CLEANUP_POLICY=DeleteOnSuccess
fi
if [ -z "${AGENTFACTORY_BOOTSTRAP_PRESERVE_FAILED_JOB:-}" ]; then
  export AGENTFACTORY_BOOTSTRAP_PRESERVE_FAILED_JOB=true
fi

exec "$script_dir/../common/run.sh"
