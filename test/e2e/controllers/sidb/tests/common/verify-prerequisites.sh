#!/usr/bin/env bash
set -Eeuo pipefail

script_dir="$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC1091
source "$script_dir/lib.sh"

sidb_case_load_env
sidb_case_init_config
sidb_case_validate_config
sidb_case_require_command kubectl
[ -s "$SIDB_RENDERED_MANIFEST" ] || {
  echo "$(sidb_case_label): rendered manifest is missing: $SIDB_RENDERED_MANIFEST" >&2
  exit 2
}

SIDB_CASE_STAGE=verify-prerequisites
trap sidb_case_setup_exit_snapshot EXIT

sidb_case_kubectl get namespace "$SIDB_CASE_NAMESPACE" >/dev/null
sidb_case_kubectl get storageclass "$SIDB_STORAGE_CLASS" >/dev/null
sidb_case_kubectl get crd singleinstancedatabases.database.oracle.com >/dev/null
sidb_case_kubectl wait --for=condition=Established \
  crd/singleinstancedatabases.database.oracle.com \
  --timeout=60s >/dev/null
sidb_case_kubectl -n "$SIDB_CASE_NAMESPACE" get secret "$SIDB_ADMIN_SECRET" >/dev/null
[ -z "${SIDB_TDE_SECRET:-}" ] || sidb_case_kubectl -n "$SIDB_CASE_NAMESPACE" get secret "$SIDB_TDE_SECRET" >/dev/null
[ -z "${SIDB_TCPS_TLS_SECRET:-}" ] || sidb_case_kubectl -n "$SIDB_CASE_NAMESPACE" get secret "$SIDB_TCPS_TLS_SECRET" >/dev/null

if sidb_case_kubectl -n "$SIDB_CASE_NAMESPACE" get singleinstancedatabase "$SIDB_TEST_NAME" >/dev/null 2>&1; then
  echo "$(sidb_case_label): target SIDB already exists: $SIDB_CASE_NAMESPACE/$SIDB_TEST_NAME" >&2
  exit 1
fi

if ! sidb_case_kubectl auth can-i create singleinstancedatabases.database.oracle.com \
  -n "$SIDB_CASE_NAMESPACE" | grep -qx 'yes'; then
  echo "$(sidb_case_label): current identity cannot create SingleInstanceDatabase resources" >&2
  exit 1
fi

if [ -n "${OPERATOR_NAMESPACE:-}" ] && [ -n "${OPERATOR_DEPLOYMENT:-}" ]; then
  available_replicas="$(sidb_case_kubectl -n "$OPERATOR_NAMESPACE" get deployment "$OPERATOR_DEPLOYMENT" \
    -o jsonpath='{.status.availableReplicas}' 2>/dev/null || true)"
  case "${available_replicas:-0}" in
    ''|0)
      echo "$(sidb_case_label): operator deployment is not available: $OPERATOR_NAMESPACE/$OPERATOR_DEPLOYMENT" >&2
      exit 1
      ;;
  esac
fi

sidb_case_kubectl -n "$SIDB_CASE_NAMESPACE" apply --dry-run=server -f "$SIDB_RENDERED_MANIFEST" >/dev/null
echo "$(sidb_case_label) prerequisites verified"
