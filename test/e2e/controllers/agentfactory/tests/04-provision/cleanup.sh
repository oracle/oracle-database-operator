#!/usr/bin/env bash
set -Eeuo pipefail

script_dir="$(CDPATH= cd -- "$(dirname "$0")" && pwd)"
export AGENTFACTORY_CASE_ID=test04
export AGENTFACTORY_NAME=agentfactory-e2e-test04
export AGENTFACTORY_RUNTIME_SECRET=agentfactory-runtime-secret-test04
export AGENTFACTORY_BOOTSTRAP_SECRET=paf-db-bootstrap-admin-test04
export AGENTFACTORY_STORAGE_VOLUME_NAME=app-mount
: "${AGENTFACTORY_CASE_CLEANUP:=false}"
export AGENTFACTORY_PVC_NAME="$AGENTFACTORY_NAME-$AGENTFACTORY_STORAGE_VOLUME_NAME"
: "${AGENTFACTORY_FSS_STORAGE_CLASS:=}"
: "${CI_PIPELINE_ID:=local}"
if [ -n "$AGENTFACTORY_FSS_STORAGE_CLASS" ]; then
  export AGENTFACTORY_STORAGE_CLASS="$AGENTFACTORY_FSS_STORAGE_CLASS"
else
  export AGENTFACTORY_STORAGE_CLASS="agentfactory-fss-test04-$CI_PIPELINE_ID"
fi

"$script_dir/../common/cleanup.sh"
cleanup_rc=$?
if [ "$AGENTFACTORY_CASE_CLEANUP" = true ] && [ -z "$AGENTFACTORY_FSS_STORAGE_CLASS" ]; then
  kubectl delete storageclass "$AGENTFACTORY_STORAGE_CLASS" --ignore-not-found=true --wait=true || cleanup_rc=1
fi
exit "$cleanup_rc"
