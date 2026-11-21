#!/usr/bin/env bash
set -Eeuo pipefail
script_dir="$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
source "$script_dir/lib.sh"
sidb_dg_init_config
SIDB_CASE_STAGE=verify-prerequisites
trap sidb_dg_exit_snapshot EXIT
bash "$SIDB_CASE_DIR/../common/verify-prerequisites.sh"
sidb_case_kubectl get crd dataguardbrokers.database.oracle.com >/dev/null
sidb_case_kubectl wait --for=condition=Established \
  crd/dataguardbrokers.database.oracle.com --timeout=60s >/dev/null
sidb_case_kubectl -n "$SIDB_CASE_NAMESPACE" get secret "$SIDB_ADMIN_SECRET" -o json |
  jq -e --arg key "$SIDB_ADMIN_SECRET_KEY" '.data[$key] | type == "string" and length > 0' >/dev/null
if [ -n "${SIDB_PULL_SECRET:-}" ]; then
  sidb_case_kubectl -n "$SIDB_CASE_NAMESPACE" get secret "$SIDB_PULL_SECRET" >/dev/null
fi
for resource_name in "singleinstancedatabase/$SIDB_DG_STANDBY_NAME" "dataguardbroker/$SIDB_DG_BROKER_NAME"; do
  if sidb_case_kubectl -n "$SIDB_CASE_NAMESPACE" get "$resource_name" >/dev/null 2>&1; then
    echo "$(sidb_case_label): target already exists: $resource_name" >&2; exit 1
  fi
done
[ -s "$SIDB_DG_RENDERED_STANDBY" ]
# Standby server dry-run is deferred until the primary is Healthy, since admission
# may require primarySource.databaseRef to resolve to an existing primary.
[ "$(sidb_case_kubectl auth can-i create dataguardbrokers.database.oracle.com -n "$SIDB_CASE_NAMESPACE")" = yes ]
echo "$(sidb_case_label): Data Guard prerequisites verified"
