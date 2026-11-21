#!/usr/bin/env bash
set -Eeuo pipefail
script_dir="$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib.sh
source "$script_dir/lib.sh"

if ! sharding_kubectl get shardingdatabase "$SHARDING_NAME" >/dev/null 2>&1; then
  echo "$(sharding_case_label): resource already absent"
else
  sharding_kubectl delete shardingdatabase "$SHARDING_NAME" --wait=true \
    --timeout="${SHARDING_CLEANUP_TIMEOUT_SECONDS:-900}s"
fi
sharding_kubectl delete secret "$SHARDING_DB_SECRET_NAME" --ignore-not-found >/dev/null
echo "$(sharding_case_label): resource and per-run DB Secret removed; OCR pull Secret retained"
