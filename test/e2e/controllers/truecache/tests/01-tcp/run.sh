#!/usr/bin/env bash
# truecache/test01: True Cache baseline TCP path (TDE day-0, no TCPS).
set -Eeuo pipefail

case_dir="$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
export TRUECACHE_CASE_ID=test01 TRUECACHE_CASE_DIR="$case_dir"
exec bash "$case_dir/../common/run.sh"
