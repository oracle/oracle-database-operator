#!/usr/bin/env bash
set -Eeuo pipefail
script_dir="$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib.sh
source "$script_dir/lib.sh"

sharding_case_init
command -v kubectl >/dev/null || { echo 'kubectl is required' >&2; exit 2; }
command -v sed >/dev/null || { echo 'sed is required' >&2; exit 2; }
command -v openssl >/dev/null || { echo 'openssl is required in the E2E job image' >&2; exit 2; }
: "${SHARDING_DB_PASSWORD:?Set SHARDING_DB_PASSWORD as a masked GitLab CI/CD variable}"
"$script_dir/verify-prerequisites.sh"
sharding_case_create_db_secret
sharding_case_render_manifest >/dev/null
echo "$(sharding_case_label): prerequisites passed"
