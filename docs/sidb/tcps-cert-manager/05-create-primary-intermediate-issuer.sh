#!/usr/bin/env bash
set -euo pipefail

. "$(cd "$(dirname "$0")" && pwd)/env.sh"

cat <<EOF | k_primary -n "${NS}" apply -f -
apiVersion: cert-manager.io/v1
kind: Issuer
metadata:
  name: ${INTERMEDIATE_ISSUER_NAME}
spec:
  ca:
    secretName: ${INT_SECRET_NAME}
EOF
