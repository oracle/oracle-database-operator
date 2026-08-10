#!/usr/bin/env bash

set -euo pipefail

NS="${NS:-default}"
PRIMARY_DB="${PRIMARY_DB:-sidb-sample}"
STANDBY_DB="${STANDBY_DB:-standbydatabase-sample}"
BROKER_YAML="${BROKER_YAML:-dataguardbroker.yaml}"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

"$SCRIPT_DIR/render-dg-broker-from-status.sh" sidb "$STANDBY_DB" "$NS" > "$BROKER_YAML"

echo "Generated $BROKER_YAML from $STANDBY_DB.status.dataguard.renderedBrokerSpec in namespace $NS"
