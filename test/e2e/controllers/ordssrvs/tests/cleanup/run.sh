#!/usr/bin/env bash
set -Eeuo pipefail

namespace="${1:?namespace required}"
controller="${2:?controller required}"

if ! declare -F operator_setup_cleanup_namespace >/dev/null 2>&1; then
  script_dir="$(CDPATH='' cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
  repo_root="${CI_PROJECT_DIR:-$(CDPATH='' cd -- "$script_dir/../../../../../.." && pwd)}"
  # shellcheck disable=SC1091
  source "$repo_root/.gitlab/ci/operator-e2e-setup.sh"
fi

cleanup_rc=0

separator='+++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++'
printf '%s\nOrdsSrvs resources before cleanup (all namespaces):\n' "$separator"
if ! kubectl get ordssrvs.database.oracle.com -A; then
  echo "Unable to list OrdsSrvs resources before cleanup" >&2
  cleanup_rc=1
fi
printf '%s\n' "$separator"

printf 'Starting OrdsSrvs cleanup: controller=%s namespace=%s\n' "$controller" "$namespace"
echo "Shared cleanup deletes non-protected resources in this namespace; namespace and protected RBAC/service accounts are retained."
if ! operator_setup_cleanup_namespace "$controller" "$namespace"; then
  cleanup_rc=1
fi

printf '%s\nOrdsSrvs resources after cleanup (all namespaces):\n' "$separator"
if ! kubectl get ordssrvs.database.oracle.com -A; then
  echo "Unable to list OrdsSrvs resources after cleanup" >&2
  cleanup_rc=1
fi
printf '%s\n' "$separator"
if [ "$cleanup_rc" -eq 0 ]; then
  printf 'OrdsSrvs cleanup passed: namespace=%s\n' "$namespace"
else
  printf 'OrdsSrvs cleanup failed: namespace=%s\n' "$namespace" >&2
fi
exit "$cleanup_rc"
