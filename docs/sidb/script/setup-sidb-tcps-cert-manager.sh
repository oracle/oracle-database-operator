#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
FLOW_DIR="${SCRIPT_DIR}/../tcps-cert-manager"

usage() {
  cat <<'EOF'
Usage:
  ./docs/sidb/script/setup-sidb-tcps-cert-manager.sh [primary|standby|both]

Environment:
  TARGET         Optional alternative to positional target. One of: primary, standby, both
  BOOTSTRAP_CA   true|false. When true, bootstrap or refresh the root/intermediate CA flow
                 before issuing leaf certificates. Default: true

Notes:
  - With no argument, the script defaults to generating both primary and standby certificates.
  - "primary" can run with BOOTSTRAP_CA=true or false.
  - "standby" with BOOTSTRAP_CA=true bootstraps the CA on primary, copies the intermediate
    to standby, creates the standby issuer, and issues only the standby leaf.
  - "standby" with BOOTSTRAP_CA=false assumes the standby intermediate issuer already exists.
  - "both" generates both primary and standby certificates.
  - Existing environment variables used by earlier revisions of this script are still honored.
EOF
}

trim_spaces() {
  local value="$1"
  value="${value#"${value%%[![:space:]]*}"}"
  value="${value%"${value##*[![:space:]]}"}"
  printf '%s' "${value}"
}

print_secret_cert_details() {
  local ctx="$1"
  local ns="$2"
  local secret="$3"
  local label="$4"
  local crt_file="${WORKDIR}/${secret}.crt"

  kubectl --context "${ctx}" -n "${ns}" get secret "${secret}" -o jsonpath='{.data.tls\.crt}' | base64 -d > "${crt_file}"

  echo "${label}:"
  openssl x509 -in "${crt_file}" -noout -subject -issuer -dates
  echo
}

run_step() {
  local script_name="$1"
  echo "Running ${script_name}"
  "${FLOW_DIR}/${script_name}"
}

should_include_primary() {
  [[ "${TARGET}" == "primary" || "${TARGET}" == "both" ]]
}

should_include_standby() {
  [[ "${TARGET}" == "standby" || "${TARGET}" == "both" ]]
}

TARGET="${TARGET:-${1:-both}}"
BOOTSTRAP_CA="${BOOTSTRAP_CA:-true}"

case "${TARGET}" in
  primary|standby|both)
    ;;
  -h|--help|help)
    usage
    exit 0
    ;;
  *)
    echo "Unsupported target: ${TARGET}" >&2
    usage >&2
    exit 1
    ;;
esac

case "${BOOTSTRAP_CA}" in
  true|false)
    ;;
  *)
    echo "BOOTSTRAP_CA must be true or false, got ${BOOTSTRAP_CA}" >&2
    exit 1
    ;;
esac

CURRENT_CONTEXT="$(kubectl config current-context 2>/dev/null || true)"

export PRIMARY_CTX="${PRIMARY_CTX:-${CURRENT_CONTEXT}}"
export STANDBY_CTX="${STANDBY_CTX:-${PRIMARY_CTX}}"
export NS="${NS:-sidb}"

export ROOT_CERT_NAME="${ROOT_CERT_NAME:-tcps-root-ca}"
export ROOT_SECRET_NAME="${ROOT_SECRET_NAME:-tcps-root-ca-secret}"
export INT_CERT_NAME="${INT_CERT_NAME:-tcps-intermediate-ca}"
export INT_SECRET_NAME="${INT_SECRET_NAME:-tcps-intermediate-ca-secret}"
export PRIMARY_PEER_CA_SECRET_NAME="${PRIMARY_PEER_CA_SECRET_NAME:-primary-peer-ca}"

export PRIMARY_CERT_NAME="${PRIMARY_CERT_NAME:-primary-db-tcps}"
export PRIMARY_SECRET_NAME="${PRIMARY_SECRET_NAME:-primary-db-tcps-secret}"
PRIMARY_DNS_NAMES_CSV="${PRIMARY_DNS_NAMES_CSV:-sidb-primary,sidb-primary.${NS},sidb-primary.${NS}.svc,sidb-primary.${NS}.svc.cluster.local}"
export PRIMARY_DNS_NAMES="${PRIMARY_DNS_NAMES:-${PRIMARY_DNS_NAMES_CSV}}"

export STANDBY_CERT_NAME="${STANDBY_CERT_NAME:-standby-db-tcps}"
export STANDBY_SECRET_NAME="${STANDBY_SECRET_NAME:-standby-db-tcps-secret}"
STANDBY_DNS_NAMES_CSV="${STANDBY_DNS_NAMES_CSV:-sidb-standby,sidb-standby.${NS},sidb-standby.${NS}.svc,sidb-standby.${NS}.svc.cluster.local}"
export STANDBY_DNS_NAMES="${STANDBY_DNS_NAMES:-${STANDBY_DNS_NAMES_CSV}}"

export ROOT_CLUSTER_ISSUER_NAME="${ROOT_CLUSTER_ISSUER_NAME:-tcps-selfsigned-bootstrap}"
export ROOT_ISSUER_NAME="${ROOT_ISSUER_NAME:-tcps-root-ca-issuer}"
export INTERMEDIATE_ISSUER_NAME="${INTERMEDIATE_ISSUER_NAME:-tcps-intermediate-issuer}"

export KUBECTL_WAIT_TIMEOUT="${KUBECTL_WAIT_TIMEOUT:-180s}"
export KUBECTL_CMD_TIMEOUT="${KUBECTL_CMD_TIMEOUT:-120}"

. "${FLOW_DIR}/env.sh"

WORKDIR="$(mktemp -d)"
trap 'rm -rf "$WORKDIR"' EXIT

echo "Target: ${TARGET}"
echo "Bootstrap CA: ${BOOTSTRAP_CA}"
echo "Primary context: ${PRIMARY_CTX}"
echo "Standby context: ${STANDBY_CTX}"
echo "Namespace: ${NS}"
echo

run_step "01-create-namespace.sh"

if [[ "${BOOTSTRAP_CA}" == "true" ]]; then
  run_step "02-bootstrap-root-ca.sh"
  run_step "03-create-root-issuer.sh"
  run_step "04-create-intermediate-ca.sh"
  run_step "05-create-primary-intermediate-issuer.sh"

  if should_include_standby; then
    run_step "06-copy-intermediate-to-standby.sh"
    run_step "07-create-standby-intermediate-issuer.sh"
  fi
else
  echo "Skipping CA bootstrap and issuer setup because BOOTSTRAP_CA=false"
  if should_include_primary; then
    echo "Expecting existing issuer ${INTERMEDIATE_ISSUER_NAME} in namespace ${NS} on primary cluster"
  fi
  if should_include_standby; then
    echo "Expecting existing issuer ${INTERMEDIATE_ISSUER_NAME} in namespace ${NS} on standby cluster"
  fi
fi

if should_include_primary; then
  run_step "08-issue-primary-certificate.sh"
fi

if should_include_standby; then
  run_step "09-issue-standby-certificate.sh"
fi

echo
echo "Verification:"
if [[ "${BOOTSTRAP_CA}" == "true" ]]; then
  print_secret_cert_details "${PRIMARY_CTX}" "${NS}" "${ROOT_SECRET_NAME}" "Root CA"
  print_secret_cert_details "${PRIMARY_CTX}" "${NS}" "${INT_SECRET_NAME}" "Intermediate CA"
fi

if should_include_primary; then
  k_primary -n "${NS}" get secret "${PRIMARY_SECRET_NAME}"
  print_secret_cert_details "${PRIMARY_CTX}" "${NS}" "${PRIMARY_SECRET_NAME}" "Primary leaf"
fi

if should_include_standby; then
  k_standby -n "${NS}" get secret "${STANDBY_SECRET_NAME}"
  print_secret_cert_details "${STANDBY_CTX}" "${NS}" "${STANDBY_SECRET_NAME}" "Standby leaf"
fi

echo "Done."
if should_include_primary; then
  echo "Primary secret: ${PRIMARY_SECRET_NAME}"
fi
if should_include_standby; then
  echo "Standby secret: ${STANDBY_SECRET_NAME}"
fi
echo
echo "Each generated leaf secret should contain:"
echo "- tls.crt"
echo "- tls.key"
echo "- ca.crt"
echo
echo "Important:"
echo "- Leaf certificate renewal is automatic through cert-manager."
echo "- The copied intermediate CA secret on standby is not auto-synced from primary."
echo "- If the primary intermediate CA rotates, sync ${INT_SECRET_NAME} again to standby and renew the standby leaf."
