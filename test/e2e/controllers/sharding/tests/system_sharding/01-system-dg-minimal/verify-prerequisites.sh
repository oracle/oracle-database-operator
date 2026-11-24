#!/usr/bin/env bash
set +x
set -Eeuo pipefail
script_dir="$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
source "$script_dir/lib.sh"
sharding_case_init
sharding_kubectl get namespace "$SHARDING_E2E_NAMESPACE" >/dev/null
sharding_kubectl get crd shardingdatabases.database.oracle.com >/dev/null
sharding_kubectl get storageclass "$SHARDING_STORAGE_CLASS" >/dev/null
for secret in "$SHARDING_DB_PULL_SECRET" "$SHARDING_GSM_PULL_SECRET"; do
  type="$(sharding_kubectl get secret "$secret" -o jsonpath='{.type}')"
  [[ "$type" = kubernetes.io/dockerconfigjson || "$type" = kubernetes.io/dockercfg ]] || {
    sharding_case_fail "Secret $secret has unexpected image-pull type: $type"; exit 1;
  }
done
[[ "$SHARDING_DB_SECRET_NAME" != "$SHARDING_DB_PULL_SECRET" &&
   "$SHARDING_DB_SECRET_NAME" != "$SHARDING_GSM_PULL_SECRET" ]] || {
  sharding_case_fail 'DB password Secret and image pull Secret names must differ'; exit 2;
}
echo "$(sharding_case_label): namespace, CRD, storage class and image pull Secrets found"
