#!/usr/bin/env bash
set -Eeuo pipefail

script_dir="$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC1091
source "$script_dir/lib.sh"

sidb_case_load_env
sidb_case_init_config

if [ -z "${SIDB_ADMIN_PASSWORD:-}" ]; then
  if command -v openssl >/dev/null 2>&1; then
    SIDB_ADMIN_PASSWORD="E2e_$(openssl rand -hex 12)"
  else
    SIDB_ADMIN_PASSWORD="E2e_${CI_PIPELINE_ID:-local}_ChangeMe"
  fi
  echo "$(sidb_case_label): generated a temporary admin password"
fi

if [ -n "${SIDB_TDE_SECRET:-}" ] && [ -z "${SIDB_TDE_PASSWORD:-}" ]; then
  if command -v openssl >/dev/null 2>&1; then
    SIDB_TDE_PASSWORD="E2e_Tde_$(openssl rand -hex 12)"
  else
    SIDB_TDE_PASSWORD="E2e_Tde_${CI_PIPELINE_ID:-local}_Aa1"
  fi
fi

sidb_case_validate_config
mkdir -p "$SIDB_CASE_RESULTS_DIR"

if sidb_case_kubectl get namespace "$SIDB_CASE_NAMESPACE" >/dev/null 2>&1; then
  SIDB_CASE_NAMESPACE_CREATED=false
  echo "$(sidb_case_label): using existing namespace $SIDB_CASE_NAMESPACE"
else
  sidb_case_kubectl create namespace "$SIDB_CASE_NAMESPACE"
  SIDB_CASE_NAMESPACE_CREATED=true
  echo "$(sidb_case_label): created namespace $SIDB_CASE_NAMESPACE"
fi

sidb_case_kubectl label namespace "$SIDB_CASE_NAMESPACE" \
  e2e.oracle.com/owner=operator-e2e \
  e2e.oracle.com/test-id="$SIDB_CASE_ID" \
  e2e.oracle.com/pipeline="$SIDB_CASE_PIPELINE" \
  --overwrite >/dev/null

secret_manifest_dir="$SIDB_CASE_RESULTS_DIR/secret-manifests"
mkdir -p "$secret_manifest_dir"

sidb_case_kubectl -n "$SIDB_CASE_NAMESPACE" create secret generic "$SIDB_ADMIN_SECRET" \
  --from-literal="${SIDB_ADMIN_SECRET_KEY}=${SIDB_ADMIN_PASSWORD}" \
  --dry-run=client -o json |
  sidb_case_label_new_secret_manifest "$SIDB_ADMIN_SECRET" |
  tee "$secret_manifest_dir/admin-secret.yaml" |
  sidb_case_kubectl -n "$SIDB_CASE_NAMESPACE" apply -f - >/dev/null

if [ -n "${SIDB_TDE_SECRET:-}" ]; then
  sidb_case_kubectl -n "$SIDB_CASE_NAMESPACE" create secret generic "$SIDB_TDE_SECRET" \
    --from-literal="${SIDB_TDE_SECRET_KEY}=${SIDB_TDE_PASSWORD}" \
    --dry-run=client -o json |
    sidb_case_label_new_secret_manifest "$SIDB_TDE_SECRET" |
    tee "$secret_manifest_dir/tde-secret.yaml" |
    sidb_case_kubectl -n "$SIDB_CASE_NAMESPACE" apply -f - >/dev/null
fi

sidb_case_create_tcps_tls_secret
cp -- "$SIDB_CASE_MANIFEST" "$SIDB_CASE_RESULTS_DIR/source-manifest.yaml"
sidb_case_render_manifest "$SIDB_CASE_MANIFEST" "$SIDB_RENDERED_MANIFEST"
sidb_case_write_env

cat <<INFO
$(sidb_case_label) prerequisites prepared
  namespace: $SIDB_CASE_NAMESPACE
  SIDB name: $SIDB_TEST_NAME
  SID: $SIDB_TEST_SID
  image: $SIDB_IMAGE
  storage class: $SIDB_STORAGE_CLASS
  rendered manifest: $SIDB_RENDERED_MANIFEST
INFO
