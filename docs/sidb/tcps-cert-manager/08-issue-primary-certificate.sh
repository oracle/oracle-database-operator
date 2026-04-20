#!/usr/bin/env bash
set -euo pipefail

. "$(cd "$(dirname "$0")" && pwd)/env.sh"

cat <<EOF | k_primary -n "${NS}" apply -f -
apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: ${PRIMARY_CERT_NAME}
spec:
  secretName: ${PRIMARY_SECRET_NAME}
  commonName: $(first_dns_name "${PRIMARY_DNS_NAMES}")
  dnsNames:
$(render_dns_names_yaml "${PRIMARY_DNS_NAMES}")
  privateKey:
    algorithm: RSA
    size: 2048
    rotationPolicy: Always
  duration: 2160h
  renewBefore: 360h
  usages:
  - server auth
  - digital signature
  - key encipherment
  issuerRef:
    name: ${INTERMEDIATE_ISSUER_NAME}
    kind: Issuer
EOF

wait_primary_cert "${PRIMARY_CERT_NAME}"
