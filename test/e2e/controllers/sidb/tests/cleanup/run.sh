#!/usr/bin/env bash
set -Eeuo pipefail

namespace="${1:?namespace required}"
controller="${2:?controller required}"

if ! declare -F operator_setup_cleanup_namespace >/dev/null 2>&1; then
  repo_root="${CI_PROJECT_DIR:-$(git rev-parse --show-toplevel)}"
  # shellcheck disable=SC1091
  source "$repo_root/.gitlab/ci/operator-e2e-setup.sh"
fi

operator_setup_cleanup_namespace "$controller" "$namespace"
