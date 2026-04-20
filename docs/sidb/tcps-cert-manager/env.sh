#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

CURRENT_CONTEXT="$(kubectl config current-context 2>/dev/null || true)"

PRIMARY_CTX="${PRIMARY_CTX:-${CURRENT_CONTEXT}}"
STANDBY_CTX="${STANDBY_CTX:-${PRIMARY_CTX}}"
NS="${NS:-default}"

PRIMARY_CERT_NAME="${PRIMARY_CERT_NAME:-sidb-primary-tcps}"
PRIMARY_SECRET_NAME="${PRIMARY_SECRET_NAME:-sidb-primary-tcps-tls}"
PRIMARY_DNS="${PRIMARY_DNS:-orcl-production.internal.example.com}"
PRIMARY_DNS_NAMES="${PRIMARY_DNS_NAMES:-${PRIMARY_DNS},orcl-production,orcl-production.default.svc.cluster.local}"

STANDBY_CERT_NAME="${STANDBY_CERT_NAME:-sidb-standby-tcps}"
STANDBY_SECRET_NAME="${STANDBY_SECRET_NAME:-sidb-standby-tcps-tls}"
STANDBY_DNS="${STANDBY_DNS:-truecache-production.internal.example.com}"
STANDBY_DNS_NAMES="${STANDBY_DNS_NAMES:-${STANDBY_DNS},truecache-production,truecache-production.default.svc.cluster.local}"

ROOT_CERT_NAME="${ROOT_CERT_NAME:-sidb-tcps-root-ca}"
ROOT_SECRET_NAME="${ROOT_SECRET_NAME:-sidb-tcps-root-ca-secret}"
INT_CERT_NAME="${INT_CERT_NAME:-sidb-tcps-intermediate-ca}"
INT_SECRET_NAME="${INT_SECRET_NAME:-sidb-tcps-intermediate-ca-secret}"
PRIMARY_PEER_CA_SECRET_NAME="${PRIMARY_PEER_CA_SECRET_NAME:-primary-peer-ca}"

ROOT_CLUSTER_ISSUER_NAME="${ROOT_CLUSTER_ISSUER_NAME:-sidb-tcps-selfsigned-bootstrap}"
ROOT_ISSUER_NAME="${ROOT_ISSUER_NAME:-sidb-tcps-root-ca-issuer}"
INTERMEDIATE_ISSUER_NAME="${INTERMEDIATE_ISSUER_NAME:-sidb-tcps-intermediate-issuer}"

KUBECTL_CMD_TIMEOUT="${KUBECTL_CMD_TIMEOUT:-120}"
KUBECTL_WAIT_TIMEOUT="${KUBECTL_WAIT_TIMEOUT:-180s}"

require_contexts() {
  if [[ -z "${PRIMARY_CTX}" ]]; then
    echo "PRIMARY_CTX is not set and kubectl has no current context." >&2
    exit 1
  fi
  if [[ -z "${STANDBY_CTX}" ]]; then
    echo "STANDBY_CTX is not set and could not be inferred." >&2
    exit 1
  fi
}

k_primary() {
  timeout "${KUBECTL_CMD_TIMEOUT}" kubectl --context "${PRIMARY_CTX}" "$@"
}

k_standby() {
  timeout "${KUBECTL_CMD_TIMEOUT}" kubectl --context "${STANDBY_CTX}" "$@"
}

wait_primary_cert() {
  k_primary -n "${NS}" wait --for=condition=Ready "certificate/${1}" --timeout="${KUBECTL_WAIT_TIMEOUT}"
}

wait_standby_cert() {
  k_standby -n "${NS}" wait --for=condition=Ready "certificate/${1}" --timeout="${KUBECTL_WAIT_TIMEOUT}"
}

trim_spaces() {
  local value="$1"
  value="${value#"${value%%[![:space:]]*}"}"
  value="${value%"${value##*[![:space:]]}"}"
  printf '%s' "${value}"
}

first_dns_name() {
  local raw="$1"
  local entry
  IFS=',' read -r -a entries <<< "${raw}"
  for entry in "${entries[@]}"; do
    entry="$(trim_spaces "${entry}")"
    if [[ -n "${entry}" ]]; then
      printf '%s' "${entry}"
      return 0
    fi
  done
  return 1
}

render_dns_names_yaml() {
  local raw="$1"
  local entry
  IFS=',' read -r -a entries <<< "${raw}"
  for entry in "${entries[@]}"; do
    entry="$(trim_spaces "${entry}")"
    if [[ -n "${entry}" ]]; then
      printf '  - %s\n' "${entry}"
    fi
  done
}

require_contexts
