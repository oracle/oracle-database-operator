#!/usr/bin/env bash
set -euo pipefail

. "$(cd "$(dirname "$0")" && pwd)/env.sh"

if [[ "${PRIMARY_CTX}" == "${STANDBY_CTX}" ]]; then
  echo "Primary and standby contexts are the same; skipping intermediate secret copy."
  exit 0
fi

WORKDIR="$(mktemp -d)"
trap 'rm -rf "${WORKDIR}"' EXIT

k_primary -n "${NS}" get secret "${INT_SECRET_NAME}" -o jsonpath='{.data.tls\.crt}' | base64 -d > "${WORKDIR}/tls.crt"
k_primary -n "${NS}" get secret "${INT_SECRET_NAME}" -o jsonpath='{.data.tls\.key}' | base64 -d > "${WORKDIR}/tls.key"

CA_PRESENT="false"
if k_primary -n "${NS}" get secret "${INT_SECRET_NAME}" -o jsonpath='{.data.ca\.crt}' >/dev/null 2>&1; then
  RAW_CA="$(k_primary -n "${NS}" get secret "${INT_SECRET_NAME}" -o jsonpath='{.data.ca\.crt}')"
  if [[ -n "${RAW_CA}" ]]; then
    printf '%s' "${RAW_CA}" | base64 -d > "${WORKDIR}/ca.crt"
    CA_PRESENT="true"
  fi
fi

if [[ "${CA_PRESENT}" == "true" ]]; then
  k_standby -n "${NS}" create secret generic "${INT_SECRET_NAME}" \
    --type=kubernetes.io/tls \
    --from-file=tls.crt="${WORKDIR}/tls.crt" \
    --from-file=tls.key="${WORKDIR}/tls.key" \
    --from-file=ca.crt="${WORKDIR}/ca.crt" \
    --dry-run=client -o yaml | k_standby apply -f -
else
  k_standby -n "${NS}" create secret generic "${INT_SECRET_NAME}" \
    --type=kubernetes.io/tls \
    --from-file=tls.crt="${WORKDIR}/tls.crt" \
    --from-file=tls.key="${WORKDIR}/tls.key" \
    --dry-run=client -o yaml | k_standby apply -f -
fi
