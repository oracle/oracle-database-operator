#!/usr/bin/env bash
set -Eeuo pipefail

script_dir="$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC1091
source "$script_dir/lib.sh"
privateai_case_init

privateai_case_collect_diagnostics before-cleanup
if [ "$PRIVATEAI_CASE_CLEANUP" != "true" ]; then
  echo "PrivateAI test01 cleanup disabled; retaining $PRIVATEAI_CASE_NAMESPACE/$PRIVATEAI_NAME"
  exit 0
fi

privateai_case_kubectl -n "$PRIVATEAI_CASE_NAMESPACE" delete privateai "$PRIVATEAI_NAME" \
  --ignore-not-found=true --wait=true \
  --timeout="${PRIVATEAI_CLEANUP_TIMEOUT_SECONDS:-600}s"
echo "PrivateAI test01 cleanup completed"
