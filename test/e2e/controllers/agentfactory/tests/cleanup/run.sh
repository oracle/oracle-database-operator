#!/usr/bin/env bash
set -Eeuo pipefail

namespace="${1:?namespace required}"
controller="${2:?controller required}"

if ! declare -F operator_setup_cleanup_namespace >/dev/null 2>&1; then
  repo_root="${CI_PROJECT_DIR:-$(git rev-parse --show-toplevel)}"
  # shellcheck disable=SC1091
  source "$repo_root/.gitlab/ci/operator-e2e-setup.sh"
fi

managed_pvcs="$(
  kubectl -n "$namespace" get pvc \
    -o json 2>/dev/null |
    jq -r '
      .items[]
      | select(any(.metadata.ownerReferences[]?;
          .kind == "AgentFactory" and
          (.name | startswith("agentfactory-e2e-"))))
      | .metadata.name
    '
)"
test04_storageclasses="$(
  kubectl get storageclass \
    -l 'e2e.oracle.com/controller=agentfactory,e2e.oracle.com/test-id=test04' \
    -o name 2>/dev/null || true
)"

cleanup_rc=0

# Public LoadBalancer Services hold reserved IPs and can outlive the CR delete
# wait. Tear them down asynchronously before the shared namespace cleanup so a
# later Agent Factory case can reuse the same IP and so CR finalizers are not
# blocked behind OCI LB teardown.
while IFS= read -r svc; do
  [ -n "$svc" ] || continue
  echo "Agent Factory cleanup: requesting async delete for LoadBalancer Service $namespace/$svc"
  kubectl -n "$namespace" delete "svc/$svc" \
    --ignore-not-found=true --wait=false || cleanup_rc=1
done < <(
  kubectl -n "$namespace" get svc -o json 2>/dev/null |
    jq -r '
      .items[]
      | select(.spec.type == "LoadBalancer")
      | select(
          (.metadata.name | startswith("agentfactory-e2e-")) or
          any(.metadata.ownerReferences[]?;
            .kind == "AgentFactory" and (.name | startswith("agentfactory-e2e-")))
        )
      | .metadata.name
    '
)

if ! operator_setup_cleanup_namespace "$controller" "$namespace"; then
  cleanup_rc=1
fi

# If an AgentFactory CR is still terminating with a finalizer, strip it so the
# next case is not blocked by the default single-worker reconcile queue.
while IFS= read -r af; do
  [ -n "$af" ] || continue
  deletion_timestamp="$(kubectl -n "$namespace" get "agentfactory/$af" \
    -o jsonpath='{.metadata.deletionTimestamp}' 2>/dev/null || true)"
  [ -n "$deletion_timestamp" ] || continue
  echo "Agent Factory cleanup: removing finalizers from stuck deleting AgentFactory $namespace/$af"
  if kubectl -n "$namespace" patch "agentfactory/$af" --type=merge \
    -p '{"metadata":{"finalizers":[]}}' >/dev/null 2>&1; then
    kubectl -n "$namespace" delete "agentfactory/$af" \
      --ignore-not-found=true --wait=false || true
  else
    cleanup_rc=1
  fi
done < <(
  kubectl -n "$namespace" get agentfactory -o json 2>/dev/null |
    jq -r '
      .items[]
      | select(.metadata.name | startswith("agentfactory-e2e-"))
      | .metadata.name
    '
)

while IFS= read -r pvc; do
  [ -n "$pvc" ] || continue
  kubectl -n "$namespace" delete "pvc/$pvc" \
    --ignore-not-found=true --wait=true \
    --timeout="${E2E_RESOURCE_CLEANUP_TIMEOUT:-600s}" || cleanup_rc=1
done <<< "$managed_pvcs"

while IFS= read -r storageclass; do
  [ -n "$storageclass" ] || continue
  kubectl delete "$storageclass" \
    --ignore-not-found=true --wait=true || cleanup_rc=1
done <<< "$test04_storageclasses"

exit "$cleanup_rc"
