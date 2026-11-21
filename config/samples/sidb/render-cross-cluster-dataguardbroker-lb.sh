#!/usr/bin/env bash
# Convenience wrapper for the two-cluster cross-cluster LB runbook.
# It discovers current NLB endpoints through render-dg-broker-from-status.sh.
set -euo pipefail

: "${PRIMARY_KUBECONFIG:?Set PRIMARY_KUBECONFIG to the primary kubeconfig}"
: "${STANDBY_KUBECONFIG:?Set STANDBY_KUBECONFIG to the standby kubeconfig}"

NS="${NS:-default}"
PRIMARY_DB="${PRIMARY_DB:-sidb-sample}"
STANDBY_DB="${STANDBY_DB:-sidb-standby}"
BROKER_NAME="${BROKER_NAME:-${STANDBY_DB}-dg}"
OUT="${OUT:-dataguardbroker-generated.yaml}"
SCRIPT="${SCRIPT:-$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/render-dg-broker-from-status.sh}"

[[ -f "$PRIMARY_KUBECONFIG" ]] || { echo "ERROR: primary kubeconfig not found: $PRIMARY_KUBECONFIG" >&2; exit 1; }
[[ -f "$STANDBY_KUBECONFIG" ]] || { echo "ERROR: standby kubeconfig not found: $STANDBY_KUBECONFIG" >&2; exit 1; }

bash "$SCRIPT" sidb \
  --broker-name "$BROKER_NAME" \
  --namespace "$NS" \
  --kubeconfig primary="$PRIMARY_KUBECONFIG" \
  --kubeconfig standby="$STANDBY_KUBECONFIG" \
  --member "primary:${PRIMARY_DB}:lb=${PRIMARY_DB}-lb" \
  --member "standby:${STANDBY_DB}:lb=${STANDBY_DB}-lb" \
  --source-ref "standby:${STANDBY_DB}" \
  > "$OUT"

echo "Wrote $OUT"
