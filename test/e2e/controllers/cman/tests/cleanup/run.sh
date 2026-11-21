#!/usr/bin/env bash
set -Eeuo pipefail

namespace="${1:?namespace required}"
controller="${2:?controller required}"

if ! declare -F operator_setup_cleanup_namespace >/dev/null 2>&1; then
  repo_root="${CI_PROJECT_DIR:-$(git rev-parse --show-toplevel)}"
  # shellcheck disable=SC1091
  source "$repo_root/.gitlab/ci/operator-e2e-setup.sh"
fi

cleanup_rc=0

# Tear down ConnectionManager external NodePorts/LBs before shared cleanup.
while IFS= read -r svc; do
  [ -n "$svc" ] || continue
  echo "CMAN cleanup: requesting async delete for Service $namespace/$svc"
  kubectl -n "$namespace" delete "svc/$svc" \
    --ignore-not-found=true --wait=false || cleanup_rc=1
done < <(
  kubectl -n "$namespace" get svc -o json 2>/dev/null |
    jq -r '
      .items[]
      | select(
          (.metadata.name | startswith("cman-e2e-")) or
          any(.metadata.ownerReferences[]?;
            .kind == "ConnectionManager" and (.name | startswith("cman-e2e-")))
        )
      | .metadata.name
    '
)

if ! operator_setup_cleanup_namespace "$controller" "$namespace"; then
  cleanup_rc=1
fi

exit "$cleanup_rc"
