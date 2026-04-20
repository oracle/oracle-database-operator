#!/usr/bin/env bash
set -euo pipefail

. "$(cd "$(dirname "$0")" && pwd)/env.sh"

echo "Primary secret:"
k_primary -n "${NS}" get secret "${PRIMARY_SECRET_NAME}"

echo
echo "Standby secret:"
k_standby -n "${NS}" get secret "${STANDBY_SECRET_NAME}"

echo
echo "Use these in SIDB manifests:"
echo "  primary tlsSecret: ${PRIMARY_SECRET_NAME}"
echo "  standby tlsSecret: ${STANDBY_SECRET_NAME}"
