#!/usr/bin/env bash
set -euo pipefail

. "$(cd "$(dirname "$0")" && pwd)/env.sh"

cat <<EOF | k_primary apply -f -
apiVersion: cert-manager.io/v1
kind: ClusterIssuer
metadata:
  name: ${ROOT_CLUSTER_ISSUER_NAME}
spec:
  selfSigned: {}
---
apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: ${ROOT_CERT_NAME}
  namespace: ${NS}
spec:
  isCA: true
  commonName: SIDB TCPS Root CA
  secretName: ${ROOT_SECRET_NAME}
  duration: 87600h
  renewBefore: 720h
  privateKey:
    algorithm: RSA
    size: 4096
  issuerRef:
    name: ${ROOT_CLUSTER_ISSUER_NAME}
    kind: ClusterIssuer
EOF

wait_primary_cert "${ROOT_CERT_NAME}"
