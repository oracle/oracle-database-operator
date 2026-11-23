#!/usr/bin/env bash
# Direct runner for the same stages used by the numbered dispatcher.
set -Eeuo pipefail
script_dir="$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
case "${1:-}" in
  '') ;;
  --cleanup) export SIDB_TEST03_CLEANUP=true;;
  *) echo "Usage: run.sh [--cleanup] (otherwise resources are retained)" >&2; exit 2;;
esac
source "$script_dir/lib.sh"
prepared=false
finalize_test03() {
  local code=$?
  if $prepared; then
    if [ "$code" -ne 0 ]; then sidb_dg_collect_diagnostics failure || true; fi
    if [ "${SIDB_TEST03_CLEANUP:-false}" = true ]; then
      if ! bash "$SIDB_CASE_DIR/cleanup.sh"; then
        [ "$code" -ne 0 ] || code=1
      fi
    fi
  fi
  exit "$code"
}
trap finalize_test03 EXIT
bash "$SIDB_CASE_DIR/prerequisites.sh"
prepared=true
sidb_dg_init_config
bash "$SIDB_CASE_DIR/verify-prerequisites.sh"
sidb_case_kubectl -n "$SIDB_CASE_NAMESPACE" apply -f "$SIDB_RENDERED_MANIFEST"
bash "$SIDB_CASE_DIR/wait-ready.sh"
bash "$SIDB_CASE_DIR/assert-topology.sh"
echo "$(sidb_case_label): basic Data Guard E2E passed"
