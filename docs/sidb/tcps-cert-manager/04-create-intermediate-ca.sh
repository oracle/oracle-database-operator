#!/usr/bin/env bash
set -euo pipefail

. "$(cd "$(dirname "$0")" && pwd)/env.sh"

cat <<EOF | k_primary -n "${NS}" apply -f -
apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: ${INT_CERT_NAME}
spec:
  isCA: true
  commonName: SIDB TCPS Intermediate CA
  secretName: ${INT_SECRET_NAME}
  duration: 43800h
  renewBefore: 720h
  privateKey:
    algorithm: RSA
    size: 4096
  usages:
  - cert sign
  - crl sign
  - digital signature
  - key encipherment
  issuerRef:
    name: ${ROOT_ISSUER_NAME}
    kind: Issuer
EOF

wait_primary_cert "${INT_CERT_NAME}"
