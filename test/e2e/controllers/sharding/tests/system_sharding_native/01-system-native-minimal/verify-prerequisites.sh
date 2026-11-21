#!/usr/bin/env bash
set -Eeuo pipefail
script_dir="$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib.sh
source "$script_dir/lib.sh"

kubectl get namespace "$SHARDING_E2E_NAMESPACE" >/dev/null 2>&1 || {
  echo "Namespace $SHARDING_E2E_NAMESPACE does not exist" >&2; exit 1;
}
kubectl get crd shardingdatabases.database.oracle.com >/dev/null 2>&1 || {
  echo 'ShardingDatabase CRD is not installed' >&2; exit 1;
}
kubectl get storageclass "$SHARDING_STORAGE_CLASS" >/dev/null 2>&1 || {
  echo "StorageClass $SHARDING_STORAGE_CLASS does not exist" >&2; exit 1;
}
sharding_kubectl get secret "$SHARDING_DB_PULL_SECRET" >/dev/null 2>&1 || {
  echo "Database image pull Secret $SHARDING_DB_PULL_SECRET is missing" >&2; exit 1;
}
sharding_kubectl get secret "$SHARDING_GSM_PULL_SECRET" >/dev/null 2>&1 || {
  echo "GSM image pull Secret $SHARDING_GSM_PULL_SECRET is missing" >&2; exit 1;
}
echo 'Verify that the operator is installed and configured to watch this namespace.'
echo "$(sharding_case_label): cluster prerequisites passed"
