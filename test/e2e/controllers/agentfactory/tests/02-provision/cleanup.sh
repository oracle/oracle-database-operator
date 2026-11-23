#!/usr/bin/env bash
set -Eeuo pipefail

script_dir="$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"

export AGENTFACTORY_CASE_ID=test02
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

exec "$script_dir/../common/cleanup.sh"
