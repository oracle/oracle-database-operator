#!/usr/bin/env bash
set -Eeuo pipefail

script_dir="$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"

# Use the shared Agent Factory provisioning/assertion flow while giving test02
# its own manifest and resource names when both cases run in one pipeline.
export AGENTFACTORY_CASE_ID=test02
# The Agent Factory test02 database hook follows SIDB test02. Preserve an
# explicitly supplied SIDB name or host, but do not inherit the test01
# defaults when the two tests run in the same shell.
sidb_namespace="${SIDB_CASE_NAMESPACE:-${SIDB_E2E_NAMESPACE:-sidb-ns}}"
if [ -z "${SIDB_TEST_NAME:-}" ] || [ "$SIDB_TEST_NAME" = "sidb-e2e-primary" ]; then
  export SIDB_TEST_NAME="${SIDB_TEST02_NAME:-sidb-e2e-primary-test02}"
fi
if [ -z "${SIDB_TEST_PDB:-}" ]; then
  export SIDB_TEST_PDB="${SIDB_TEST02_PDB:-ORCLPDB1}"
fi
if [ -z "${AGENTFACTORY_DB_HOST:-}" ] ||
   [ "$AGENTFACTORY_DB_HOST" = "sidb-e2e-primary.${sidb_namespace}.svc.cluster.local" ]; then
  export AGENTFACTORY_DB_HOST="sidb-e2e-primary-test02.${sidb_namespace}.svc.cluster.local"
fi
if [ -z "${AGENTFACTORY_NAME:-}" ] ||
   [ "$AGENTFACTORY_NAME" = "agentfactory-e2e-test01" ]; then
  export AGENTFACTORY_NAME=agentfactory-e2e-test02
fi
if [ -z "${AGENTFACTORY_RUNTIME_SECRET:-}" ] ||
   [ "$AGENTFACTORY_RUNTIME_SECRET" = "agentfactory-runtime-secret" ]; then
  export AGENTFACTORY_RUNTIME_SECRET=agentfactory-runtime-secret-test02
fi
if [ -z "${AGENTFACTORY_BOOTSTRAP_SECRET:-}" ] ||
   [ "$AGENTFACTORY_BOOTSTRAP_SECRET" = "paf-db-bootstrap-admin" ]; then
  export AGENTFACTORY_BOOTSTRAP_SECRET=paf-db-bootstrap-admin-test02
fi
if [ -z "${AGENTFACTORY_PVC_NAME:-}" ] ||
   [ "$AGENTFACTORY_PVC_NAME" = "agentfactory-e2e-pvc" ]; then
  export AGENTFACTORY_PVC_NAME=agentfactory-e2e-test02-pvc
fi
export AGENTFACTORY_TEST_TEMPLATE="$script_dir/test02.yaml"
export AGENTFACTORY_PREPARE_DATABASE_SCRIPT="$script_dir/prepare-database.sh"
export AGENTFACTORY_ASSERT_SCRIPT="$script_dir/assert.sh"

# Test02 uses the controller-managed database setup Job. Delete the temporary
# administrator Secret after the Job succeeds unless the caller explicitly
# requests Retain for debugging.
if [ -z "${AGENTFACTORY_BOOTSTRAP_CLEANUP_POLICY:-}" ]; then
  export AGENTFACTORY_BOOTSTRAP_CLEANUP_POLICY=DeleteOnSuccess
fi
# Preserve a failed bootstrap Pod in this E2E case so SQL*Plus diagnostics are
# available. Customer resources keep the controller default unless they opt in.
if [ -z "${AGENTFACTORY_BOOTSTRAP_PRESERVE_FAILED_JOB:-}" ]; then
  export AGENTFACTORY_BOOTSTRAP_PRESERVE_FAILED_JOB=true
fi

exec "$script_dir/../common/run.sh"
