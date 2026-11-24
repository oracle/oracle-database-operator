#!/usr/bin/env bash
set -Eeuo pipefail

script_dir="$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC1091
source "$script_dir/lib.sh"
agentfactory_case_init
test_id="$AGENTFACTORY_CASE_ID"

agentfactory_case_collect_diagnostics before-cleanup
if [ "$AGENTFACTORY_CASE_CLEANUP" != "true" ]; then
  echo "Agent Factory $test_id cleanup disabled; retaining $AGENTFACTORY_CASE_NAMESPACE/$AGENTFACTORY_NAME"
  exit 0
fi

agentfactory_case_kubectl -n "$AGENTFACTORY_CASE_NAMESPACE" delete agentfactory "$AGENTFACTORY_NAME" \
  --ignore-not-found=true --wait=true \
  --timeout="${AGENTFACTORY_CLEANUP_TIMEOUT_SECONDS:-900}s"
agentfactory_case_kubectl -n "$AGENTFACTORY_CASE_NAMESPACE" delete secret \
  "$AGENTFACTORY_RUNTIME_SECRET" "$AGENTFACTORY_BOOTSTRAP_SECRET" \
  --ignore-not-found=true
agentfactory_case_kubectl -n "$AGENTFACTORY_CASE_NAMESPACE" delete pvc "$AGENTFACTORY_PVC_NAME" \
  --ignore-not-found=true --wait=true
echo "Agent Factory $test_id cleanup completed"
