#!/usr/bin/env bash
set -euo pipefail

DIR="$(cd "$(dirname "$0")" && pwd)"

"${DIR}/01-create-namespace.sh"
"${DIR}/02-bootstrap-root-ca.sh"
"${DIR}/03-create-root-issuer.sh"
"${DIR}/04-create-intermediate-ca.sh"
"${DIR}/05-create-primary-intermediate-issuer.sh"
"${DIR}/06-copy-intermediate-to-standby.sh"
"${DIR}/07-create-standby-intermediate-issuer.sh"
"${DIR}/08-issue-primary-certificate.sh"
"${DIR}/09-issue-standby-certificate.sh"
"${DIR}/10-copy-primary-ca-to-standby.sh"
"${DIR}/11-verify-secrets.sh"
