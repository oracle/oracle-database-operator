#!/usr/bin/env bash
# truecache/test02: True Cache secure TCPS path (TDE day-0 + operator-managed TCPS).
set -Eeuo pipefail

case_dir="$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
export TRUECACHE_CASE_ID=test02 TRUECACHE_CASE_DIR="$case_dir"
exec bash "$case_dir/../common/run.sh"
