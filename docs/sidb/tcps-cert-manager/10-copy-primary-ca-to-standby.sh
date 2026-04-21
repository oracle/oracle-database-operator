#!/usr/bin/env bash
set -euo pipefail

. "$(cd "$(dirname "$0")" && pwd)/env.sh"

WORKDIR="$(mktemp -d)"
trap 'rm -rf "${WORKDIR}"' EXIT

normalize_tls_secret() {
  local context_fn="$1"
  local secret_name="$2"

  "${context_fn}" -n "${NS}" get secret "${secret_name}" -o jsonpath='{.data.tls\.crt}' | base64 -d > "${WORKDIR}/${secret_name}-tls.crt"
  "${context_fn}" -n "${NS}" get secret "${secret_name}" -o jsonpath='{.data.tls\.key}' | base64 -d > "${WORKDIR}/${secret_name}-tls.key"
  cat "${WORKDIR}/${secret_name}-tls.crt" "${WORKDIR}/root.crt" > "${WORKDIR}/${secret_name}-tls-fullchain.crt"

  "${context_fn}" -n "${NS}" create secret generic "${secret_name}" \
    --type=kubernetes.io/tls \
    --from-file=tls.crt="${WORKDIR}/${secret_name}-tls-fullchain.crt" \
    --from-file=tls.key="${WORKDIR}/${secret_name}-tls.key" \
    --from-file=ca.crt="${WORKDIR}/ca-bundle.crt" \
    --dry-run=client -o yaml | "${context_fn}" apply -f -
}

k_primary -n "${NS}" get secret "${INT_SECRET_NAME}" -o jsonpath='{.data.tls\.crt}' | base64 -d > "${WORKDIR}/intermediate.crt"
k_primary -n "${NS}" get secret "${ROOT_SECRET_NAME}" -o jsonpath='{.data.tls\.crt}' | base64 -d > "${WORKDIR}/root.crt"
cat "${WORKDIR}/intermediate.crt" "${WORKDIR}/root.crt" > "${WORKDIR}/ca-bundle.crt"

normalize_tls_secret k_primary "${PRIMARY_SECRET_NAME}"
normalize_tls_secret k_standby "${STANDBY_SECRET_NAME}"

k_standby -n "${NS}" create secret generic "${PRIMARY_PEER_CA_SECRET_NAME}" \
  --from-file=ca.crt="${WORKDIR}/ca-bundle.crt" \
  --dry-run=client -o yaml | k_standby apply -f -
