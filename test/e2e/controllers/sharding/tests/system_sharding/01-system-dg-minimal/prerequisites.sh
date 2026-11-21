#!/usr/bin/env bash
set +x
set -Eeuo pipefail
script_dir="$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
source "$script_dir/lib.sh"
sharding_case_init
: "${SHARDING_DB_PASSWORD:?Set SHARDING_DB_PASSWORD as a masked CI/CD variable}"
bash "$script_dir/verify-prerequisites.sh"
# Fixed component names require a free sharding namespace. API errors fail here.
crs="$(sharding_kubectl get shardingdatabases -o json)"
jq -e '.items | length == 0' <<<"$crs" >/dev/null || {
  sharding_case_fail 'Existing ShardingDatabase resources remain; cleanup must complete before provisioning'; exit 1;
}
resources="$(sharding_kubectl get pods,statefulsets,services,pvc -o json)"
jq -e '[.items[] | select(.metadata.name | test("^(catalog|gsm[12]|pshard[0-9]+)(-|$)"))] | length == 0' \
  <<<"$resources" >/dev/null || {
  sharding_case_fail 'Old sharding workloads/services/PVCs remain in this namespace; resolve them before provisioning'; exit 1;
}
sharding_case_render_manifest >/dev/null
sharding_case_create_db_secret
echo "$(sharding_case_label): prerequisites passed; encrypted DB Secret created"
