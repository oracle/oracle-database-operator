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

exec "$script_dir/../common/cleanup.sh"
