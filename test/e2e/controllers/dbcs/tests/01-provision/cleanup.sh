#!/usr/bin/env bash
set -Eeuo pipefail

DBCS_CASE_SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC1090
source "$DBCS_CASE_SCRIPT_DIR/lib.sh"
dbcs_case_init
dbcs_case_cleanup
