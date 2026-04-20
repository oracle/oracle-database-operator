#!/usr/bin/env bash
set -euo pipefail

. "$(cd "$(dirname "$0")" && pwd)/env.sh"

cat <<EOF | k_standby -n "${NS}" apply -f -
apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: ${STANDBY_CERT_NAME}
spec:
  secretName: ${STANDBY_SECRET_NAME}
  commonName: $(first_dns_name "${STANDBY_DNS_NAMES}")
  dnsNames:
$(render_dns_names_yaml "${STANDBY_DNS_NAMES}")
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

wait_standby_cert "${STANDBY_CERT_NAME}"
