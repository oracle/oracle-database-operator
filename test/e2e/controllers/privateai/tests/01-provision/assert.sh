#!/usr/bin/env bash
set -Eeuo pipefail

script_dir="$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC1091
source "$script_dir/lib.sh"

privateai_case_init

fail() {
  echo "PrivateAI test01 assertion failed: $*" >&2
  exit 1
}

require_equal() {
  local label="$1" expected="$2" actual="$3"
  [ "$actual" = "$expected" ] || fail "$label is '$actual', expected '$expected'"
}

require_nonempty() {
  local label="$1" value="$2"
  [ -n "$value" ] || fail "$label is empty"
}

require_at_least_one() {
  local label="$1" value="$2"
  [[ "$value" =~ ^[0-9]+$ ]] && [ "$value" -ge 1 ] ||
    fail "$label is '$value'"
}

privateai_status="$(privateai_case_kubectl -n "$PRIVATEAI_CASE_NAMESPACE" \
  get privateai "$PRIVATEAI_NAME" -o jsonpath='{.status.status}')"
require_equal "PrivateAI status" "Healthy" "$privateai_status"

auth_enabled="$(privateai_case_kubectl -n "$PRIVATEAI_CASE_NAMESPACE" \
  get privateai "$PRIVATEAI_NAME" -o jsonpath='{.spec.security.authEnabled}')"
require_equal "spec.security.authEnabled" "true" "$auth_enabled"

auth_secret="$(privateai_case_kubectl -n "$PRIVATEAI_CASE_NAMESPACE" \
  get privateai "$PRIVATEAI_NAME" -o jsonpath='{.spec.security.secret.name}')"
tls_secret="$(privateai_case_kubectl -n "$PRIVATEAI_CASE_NAMESPACE" \
  get privateai "$PRIVATEAI_NAME" -o jsonpath='{.spec.security.tls.secretName}')"
require_equal "auth Secret name" "$PRIVATEAI_AUTH_SECRET" "$auth_secret"
require_equal "TLS Secret name" "$PRIVATEAI_TLS_SECRET" "$tls_secret"

auth_mount="$(privateai_case_kubectl -n "$PRIVATEAI_CASE_NAMESPACE" \
  get privateai "$PRIVATEAI_NAME" -o jsonpath='{.spec.security.secret.mountLocation}')"
tls_mount="$(privateai_case_kubectl -n "$PRIVATEAI_CASE_NAMESPACE" \
  get privateai "$PRIVATEAI_NAME" -o jsonpath='{.spec.security.tls.mountLocation}')"
require_equal "auth mount location" "/privateai/ssl" "$auth_mount"
require_equal "TLS mount location" "/privateai/ssl" "$tls_mount"

https_enabled="$(privateai_case_kubectl -n "$PRIVATEAI_CASE_NAMESPACE" \
  get privateai "$PRIVATEAI_NAME" -o jsonpath='{.spec.networking.listeners.https.enabled}')"
https_port="$(privateai_case_kubectl -n "$PRIVATEAI_CASE_NAMESPACE" \
  get privateai "$PRIVATEAI_NAME" -o jsonpath='{.spec.networking.listeners.https.port}')"
require_equal "HTTPS listener enabled" "true" "$https_enabled"
require_equal "HTTPS listener port" "8443" "$https_port"

replicas="$(privateai_case_kubectl -n "$PRIVATEAI_CASE_NAMESPACE" \
  get privateai "$PRIVATEAI_NAME" -o jsonpath='{.spec.runtime.replicas}')"
require_equal "runtime replicas" "1" "$replicas"

available_replicas="$(privateai_case_kubectl -n "$PRIVATEAI_CASE_NAMESPACE" \
  get deployment "$PRIVATEAI_NAME" -o jsonpath='{.status.availableReplicas}' 2>/dev/null || true)"
ready_replicas="$(privateai_case_kubectl -n "$PRIVATEAI_CASE_NAMESPACE" \
  get deployment "$PRIVATEAI_NAME" -o jsonpath='{.status.readyReplicas}' 2>/dev/null || true)"
require_at_least_one "Deployment availableReplicas" "${available_replicas:-0}"
require_at_least_one "Deployment readyReplicas" "${ready_replicas:-0}"

mapfile -t pods < <(
  privateai_case_kubectl -n "$PRIVATEAI_CASE_NAMESPACE" get pods \
    -l "app.kubernetes.io/instance=PrivateAi-$PRIVATEAI_NAME" \
    -o jsonpath='{range .items[*]}{.metadata.name}{"\n"}{end}'
)
[ "${#pods[@]}" -gt 0 ] || fail "no PrivateAI Pods found"
for pod in "${pods[@]}"; do
  privateai_case_kubectl -n "$PRIVATEAI_CASE_NAMESPACE" wait \
    --for=condition=Ready "pod/$pod" --timeout=60s >/dev/null
done

local_service="$(privateai_case_kubectl -n "$PRIVATEAI_CASE_NAMESPACE" \
  get privateai "$PRIVATEAI_NAME" -o jsonpath='{.status.localService}')"
require_nonempty "status.localService" "$local_service"
require_equal "local Service name" "$PRIVATEAI_NAME" "$local_service"

service_type="$(privateai_case_kubectl -n "$PRIVATEAI_CASE_NAMESPACE" \
  get service "$local_service" -o jsonpath='{.spec.type}')"
service_port="$(privateai_case_kubectl -n "$PRIVATEAI_CASE_NAMESPACE" \
  get service "$local_service" -o jsonpath='{.spec.ports[0].port}')"
service_target_port="$(privateai_case_kubectl -n "$PRIVATEAI_CASE_NAMESPACE" \
  get service "$local_service" -o jsonpath='{.spec.ports[0].targetPort}')"
require_equal "local Service type" "ClusterIP" "$service_type"
require_equal "local Service port" "8443" "$service_port"
require_equal "local Service targetPort" "8443" "$service_target_port"

service_endpoints="$(privateai_case_kubectl -n "$PRIVATEAI_CASE_NAMESPACE" \
  get endpoints "$local_service" \
  -o jsonpath='{range .subsets[*].addresses[*]}{.ip}{"\n"}{end}' 2>/dev/null || true)"
require_nonempty "local Service endpoints" "$service_endpoints"

if privateai_case_kubectl -n "$PRIVATEAI_CASE_NAMESPACE" \
  get service "${PRIVATEAI_NAME}-public-lb" >/dev/null 2>&1; then
  fail "public LoadBalancer Service exists although Quick Start is ClusterIP-only"
fi

api_key_data="$(privateai_case_kubectl -n "$PRIVATEAI_CASE_NAMESPACE" \
  get secret "$PRIVATEAI_AUTH_SECRET" -o jsonpath="{.data['api-key']}" 2>/dev/null || true)"
ssl_password_data="$(privateai_case_kubectl -n "$PRIVATEAI_CASE_NAMESPACE" \
  get secret "$PRIVATEAI_AUTH_SECRET" -o jsonpath="{.data['privateai-ssl-pwd']}" 2>/dev/null || true)"
tls_cert_data="$(privateai_case_kubectl -n "$PRIVATEAI_CASE_NAMESPACE" \
  get secret "$PRIVATEAI_TLS_SECRET" -o jsonpath='{.data.tls\.crt}' 2>/dev/null || true)"
tls_key_data="$(privateai_case_kubectl -n "$PRIVATEAI_CASE_NAMESPACE" \
  get secret "$PRIVATEAI_TLS_SECRET" -o jsonpath='{.data.tls\.key}' 2>/dev/null || true)"
keystore_data="$(privateai_case_kubectl -n "$PRIVATEAI_CASE_NAMESPACE" \
  get secret "$PRIVATEAI_TLS_SECRET" -o jsonpath='{.data.keystore\.p12}' 2>/dev/null || true)"
require_nonempty "auth Secret api-key" "$api_key_data"
require_nonempty "auth Secret privateai-ssl-pwd" "$ssl_password_data"
require_nonempty "TLS Secret tls.crt" "$tls_cert_data"
require_nonempty "TLS Secret tls.key" "$tls_key_data"
require_nonempty "TLS Secret keystore.p12" "$keystore_data"

status_auth_secret="$(privateai_case_kubectl -n "$PRIVATEAI_CASE_NAMESPACE" \
  get privateai "$PRIVATEAI_NAME" -o jsonpath='{.status.paiSecret.name}')"
status_tls_secret="$(privateai_case_kubectl -n "$PRIVATEAI_CASE_NAMESPACE" \
  get privateai "$PRIVATEAI_NAME" -o jsonpath='{.status.tlsSecret.name}')"
has_api_key="$(privateai_case_kubectl -n "$PRIVATEAI_CASE_NAMESPACE" \
  get privateai "$PRIVATEAI_NAME" -o jsonpath='{.status.paiSecret.hasAPIKey}')"
require_equal "status.paiSecret.name" "$PRIVATEAI_AUTH_SECRET" "$status_auth_secret"
require_equal "status.tlsSecret.name" "$PRIVATEAI_TLS_SECRET" "$status_tls_secret"
require_equal "status.paiSecret.hasAPIKey" "true" "$has_api_key"

printf '%s\n' \
  "test=privateai/test01" \
  "status=$privateai_status" \
  "name=$PRIVATEAI_NAME" \
  "namespace=$PRIVATEAI_CASE_NAMESPACE" \
  "auth_secret=$PRIVATEAI_AUTH_SECRET" \
  "tls_secret=$PRIVATEAI_TLS_SECRET" \
  "service=$local_service" \
  "service_port=8443" \
  "https_listener_port=8443" \
  "public_load_balancer=false" \
  > "$PRIVATEAI_CASE_RESULTS_DIR/assertion-summary.txt"

echo "PrivateAI test01 assertions passed: Quick Start auth/TLS, HTTPS 8443, ClusterIP, and one ready replica"
