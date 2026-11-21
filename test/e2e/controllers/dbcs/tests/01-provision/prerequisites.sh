#!/usr/bin/env bash
set -Eeuo pipefail

DBCS_CASE_SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC1090
source "$DBCS_CASE_SCRIPT_DIR/lib.sh"
dbcs_case_init
dbcs_case_validate
mkdir -p "$DBCS_CASE_RESULTS_DIR"

if dbcs_case_kubectl get namespace "$DBCS_NAMESPACE" >/dev/null 2>&1; then
  DBCS_NAMESPACE_CREATED=false
else
  dbcs_case_kubectl create namespace "$DBCS_NAMESPACE" >/dev/null
  DBCS_NAMESPACE_CREATED=true
fi

dbcs_case_kubectl -n "$DBCS_NAMESPACE" create configmap oci-cred \
  --from-literal="tenancy=$DBCS_OCI_TENANCY" \
  --from-literal="user=$DBCS_OCI_USER" \
  --from-literal="fingerprint=$DBCS_OCI_FINGERPRINT" \
  --from-literal="region=$DBCS_OCI_REGION" \
  --dry-run=client -o yaml | dbcs_case_kubectl -n "$DBCS_NAMESPACE" apply -f - >/dev/null

dbcs_case_kubectl -n "$DBCS_NAMESPACE" create secret generic oci-privatekey \
  --from-file=privatekey="$OCI_API_PRIVATE_KEY_FILE" \
  --dry-run=client -o yaml | dbcs_case_kubectl -n "$DBCS_NAMESPACE" apply -f - >/dev/null
# Controller reads key "admin-password" (docs --from-file=./admin-password).
dbcs_case_kubectl -n "$DBCS_NAMESPACE" create secret generic admin-password \
  --from-literal=admin-password="$DBCS_ADMIN_PASSWORD" \
  --dry-run=client -o yaml | dbcs_case_kubectl -n "$DBCS_NAMESPACE" apply -f - >/dev/null
dbcs_case_kubectl -n "$DBCS_NAMESPACE" create secret generic oci-publickey \
  --from-file=publickey="$DBCS_SSH_PUBLIC_KEY_FILE" \
  --dry-run=client -o yaml | dbcs_case_kubectl -n "$DBCS_NAMESPACE" apply -f - >/dev/null

dbcs_case_render "$DBCS_CASE_SOURCE" "$DBCS_RENDERED_MANIFEST"
dbcs_case_kubectl -n "$DBCS_NAMESPACE" apply --dry-run=server -f "$DBCS_RENDERED_MANIFEST" >/dev/null
printf 'export DBCS_NAMESPACE=%q\nexport DBCS_NAMESPACE_CREATED=%q\nexport DBCS_RENDERED_MANIFEST=%q\n' \
  "$DBCS_NAMESPACE" "${DBCS_NAMESPACE_CREATED:-false}" "$DBCS_RENDERED_MANIFEST" \
  > "$DBCS_CASE_ENV_FILE"
chmod 600 "$DBCS_CASE_ENV_FILE"
echo "DBCS test01 prerequisites prepared: namespace=$DBCS_NAMESPACE manifest=$DBCS_RENDERED_MANIFEST"
