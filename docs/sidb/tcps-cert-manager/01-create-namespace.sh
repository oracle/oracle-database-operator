#!/usr/bin/env bash
set -euo pipefail

. "$(cd "$(dirname "$0")" && pwd)/env.sh"

k_primary get ns "${NS}" >/dev/null 2>&1 || k_primary create ns "${NS}"
k_standby get ns "${NS}" >/dev/null 2>&1 || k_standby create ns "${NS}"

echo "Namespace ${NS} is present on:"
echo "  primary: ${PRIMARY_CTX}"
echo "  standby: ${STANDBY_CTX}"
