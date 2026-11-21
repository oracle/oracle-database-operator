#!/usr/bin/env bash

set -euo pipefail

NS="${NS:-default}"
PRIMARY_DB="${PRIMARY_DB:-sidb-sample}"
BROKER_YAML="${BROKER_YAML:-dataguardbroker.yaml}"
BROKER_NAME="${BROKER_NAME:-standbydatabase-sample-dg}"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
KUBECONFIG_FILE="${KUBECONFIG:-$HOME/.kube/config}"

mapfile -t STANDBY_DBS < <(
  kubectl get singleinstancedatabase -n "$NS" -o json \
  | jq -r --arg primary "$PRIMARY_DB" '
      .items[]
      | select(
          (.spec.createAs // "" | ascii_downcase) == "standby"
          and (.spec.primarySource.databaseRef // "") == $primary
        )
      | .metadata.name
    '
)

if [[ ${#STANDBY_DBS[@]} -eq 0 ]]; then
  echo "No standby databases found for primary $PRIMARY_DB in namespace $NS" >&2
  exit 1
fi

ARGS=(
  sidb
  --broker-name "$BROKER_NAME"
  --namespace "$NS"
  --kubeconfig "local=$KUBECONFIG_FILE"
  --member "local:$PRIMARY_DB"
)

for standby in "${STANDBY_DBS[@]}"; do
  ARGS+=(--member "local:$standby")
done

# Keep same behavior as before: first standby becomes sourceRef.
ARGS+=(--source-ref "local:${STANDBY_DBS[0]}")

"$SCRIPT_DIR/render-dg-broker-from-status.sh" \
  "${ARGS[@]}" \
  > "$BROKER_YAML"

echo "Generated $BROKER_YAML"
echo "Primary : $PRIMARY_DB"
echo "Standbys: ${STANDBY_DBS[*]}"
echo "Namespace: $NS"