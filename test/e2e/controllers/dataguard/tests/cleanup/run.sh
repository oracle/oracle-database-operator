#!/usr/bin/env bash
set -Eeuo pipefail

namespace="${1:?namespace required}"
controller="${2:?controller required}"

# Data Guard resources live in the SIDB namespace; reuse SIDB cleanup semantics.
repo_root="${CI_PROJECT_DIR:-$(git rev-parse --show-toplevel)}"
exec bash "$repo_root/test/e2e/controllers/sidb/tests/cleanup/run.sh" \
  "${DATAGUARD_E2E_NAMESPACE:-${SIDB_E2E_NAMESPACE:-$namespace}}" \
  sidb
