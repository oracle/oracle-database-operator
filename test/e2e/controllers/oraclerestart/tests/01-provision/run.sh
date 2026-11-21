#!/usr/bin/env bash
set -Eeuo pipefail

ORESTART_CASE_SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
source "$ORESTART_CASE_SCRIPT_DIR/lib.sh"
orestart_case_init
if [ -f "${TMPDIR:-/tmp}/orestart-e2e-${CI_PIPELINE_ID:-local}.env" ]; then
  source "${TMPDIR:-/tmp}/orestart-e2e-${CI_PIPELINE_ID:-local}.env"
fi
orestart_validate
trap 'orestart_cleanup' EXIT

echo "Oracle Restart create: applying $ORESTART_NAMESPACE/$ORESTART_NAME"
orestart_kubectl -n "$ORESTART_NAMESPACE" apply -f "$ORESTART_MANIFEST"
orestart_clean_asm_devices
orestart_wait_for_available
orestart_kubectl -n "$ORESTART_NAMESPACE" get oracleRestart "$ORESTART_NAME" -o yaml > "$ORESTART_RESULTS_DIR/oraclerestart-available.yaml"
orestart_kubectl -n "$ORESTART_NAMESPACE" get all,pvc -o wide > "$ORESTART_RESULTS_DIR/workloads-available.txt"
echo "Oracle Restart create and verification passed"
orestart_cleanup
ORESTART_CASE_CLEANUP=false
export ORESTART_CASE_CLEANUP
