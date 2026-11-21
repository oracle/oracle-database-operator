#!/usr/bin/env bash
set -Eeuo pipefail

ORESTART_CASE_SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
source "$ORESTART_CASE_SCRIPT_DIR/lib.sh"
orestart_case_init
orestart_cleanup
