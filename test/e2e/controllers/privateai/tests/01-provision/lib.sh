#!/usr/bin/env bash

e2e_workload_health_file="${CI_PROJECT_DIR:-$(git rev-parse --show-toplevel 2>/dev/null || pwd)}/.gitlab/ci/e2e-workload-health.sh"
# shellcheck disable=SC1090
source "$e2e_workload_health_file"
unset e2e_workload_health_file

privateai_case_init() {
  : "${PRIVATEAI_CASE_ID:=test01}"
  : "${PRIVATEAI_CASE_NAMESPACE:=${PRIVATEAI_E2E_NAMESPACE:-pai-ns}}"
  : "${PRIVATEAI_NAME:=privateai-e2e-service}"
  : "${PRIVATEAI_CASE_PIPELINE:=${CI_PIPELINE_ID:-local}}"
  : "${PRIVATEAI_TIMEOUT_SECONDS:=1800}"
  : "${PRIVATEAI_POLL_INTERVAL:=15}"
  : "${PRIVATEAI_CASE_RESULTS_DIR:=${OPERATOR_LOG_DIR:-${CI_PROJECT_DIR:-$PWD}/artifacts}/privateai/${PRIVATEAI_CASE_NAMESPACE}-${PRIVATEAI_CASE_ID}}"
  : "${PRIVATEAI_RENDERED_MANIFEST:=${PRIVATEAI_CASE_RESULTS_DIR}/test01-rendered.yaml}"
  : "${PRIVATEAI_IMAGE:=${PRIVATE_AI_IMAGE:-${PAI_IMAGE:-}}}"; : "${PRIVATEAI_IMAGE:?Set PRIVATEAI_IMAGE (or PRIVATE_AI_IMAGE/PAI_IMAGE) as an environment variable}"
  : "${PRIVATEAI_IMAGE_PULL_POLICY:=Always}"
  : "${PRIVATEAI_PULL_SECRET:=${PRIVATE_AI_PULL_SECRET:-}}"
  : "${PRIVATEAI_AUTH_SECRET:=paisecret}"
  : "${PRIVATEAI_TLS_SECRET:=privateai-tls}"
  : "${PRIVATEAI_CREATE_SECRETS:=true}"
  : "${PRIVATEAI_API_KEY:=}"
  : "${PRIVATEAI_SSL_PASSWORD:=}"
  : "${PRIVATEAI_TLS_COMMON_NAME:=${PRIVATEAI_NAME}.${PRIVATEAI_CASE_NAMESPACE}.svc}"
  : "${PRIVATEAI_CASE_CLEANUP:=false}"
  PRIVATEAI_SECRET_TMP_DIR=""
  export PRIVATEAI_CASE_ID PRIVATEAI_CASE_NAMESPACE PRIVATEAI_NAME PRIVATEAI_CASE_PIPELINE
  export PRIVATEAI_TIMEOUT_SECONDS PRIVATEAI_POLL_INTERVAL PRIVATEAI_CASE_RESULTS_DIR
  export PRIVATEAI_RENDERED_MANIFEST PRIVATEAI_IMAGE PRIVATEAI_IMAGE_PULL_POLICY PRIVATEAI_PULL_SECRET PRIVATEAI_CASE_CLEANUP
  export PRIVATEAI_AUTH_SECRET PRIVATEAI_TLS_SECRET PRIVATEAI_CREATE_SECRETS
  export PRIVATEAI_API_KEY PRIVATEAI_SSL_PASSWORD PRIVATEAI_TLS_COMMON_NAME
}

privateai_case_kubectl() {
  if [ -n "${KUBE_CONTEXT_NAME:-}" ]; then
    command kubectl --context "$KUBE_CONTEXT_NAME" "$@"
  else
    command kubectl "$@"
  fi
}

privateai_case_render_manifest() {
  local source_file="$1" destination_file="$2" line
  mkdir -p "$(dirname "$destination_file")"
  : > "$destination_file"
  while IFS= read -r line || [ -n "$line" ]; do
    if [ -z "$PRIVATEAI_PULL_SECRET" ] &&
       [ "$line" = '      pullSecret: "__PRIVATEAI_PULL_SECRET__"' ]; then
      continue
    fi
    line="${line//__PRIVATEAI_NAME__/$PRIVATEAI_NAME}"
    line="${line//__PRIVATEAI_IMAGE__/$PRIVATEAI_IMAGE}"
    line="${line//__PRIVATEAI_IMAGE_PULL_POLICY__/$PRIVATEAI_IMAGE_PULL_POLICY}"
    line="${line//__PRIVATEAI_PULL_SECRET__/$PRIVATEAI_PULL_SECRET}"
    line="${line//__PRIVATEAI_AUTH_SECRET__/$PRIVATEAI_AUTH_SECRET}"
    line="${line//__PRIVATEAI_TLS_SECRET__/$PRIVATEAI_TLS_SECRET}"
    line="${line//__PRIVATEAI_CASE_PIPELINE__/$PRIVATEAI_CASE_PIPELINE}"
    printf '%s\n' "$line" >> "$destination_file"
  done < "$source_file"
}

privateai_case_create_quickstart_secrets() {
  local work_dir common_name

  if [ "$PRIVATEAI_CREATE_SECRETS" != "true" ]; then
    privateai_case_kubectl -n "$PRIVATEAI_CASE_NAMESPACE" get secret "$PRIVATEAI_AUTH_SECRET" >/dev/null
    privateai_case_kubectl -n "$PRIVATEAI_CASE_NAMESPACE" get secret "$PRIVATEAI_TLS_SECRET" >/dev/null
    return 0
  fi

  command -v openssl >/dev/null 2>&1 || {
    echo "PrivateAI test01: openssl is required to create Quick Start test Secrets" >&2
    return 2
  }

  work_dir="$(mktemp -d "${TMPDIR:-/tmp}/privateai-test01.XXXXXX")"
  chmod 700 "$work_dir"
  PRIVATEAI_SECRET_TMP_DIR="$work_dir"
  common_name="$PRIVATEAI_TLS_COMMON_NAME"

  if [ -n "$PRIVATEAI_API_KEY" ]; then
    printf '%s' "$PRIVATEAI_API_KEY" > "$work_dir/api-key"
  else
    openssl rand -hex 32 > "$work_dir/api-key"
  fi
  if [ -n "$PRIVATEAI_SSL_PASSWORD" ]; then
    printf '%s' "$PRIVATEAI_SSL_PASSWORD" > "$work_dir/privateai-ssl-pwd"
  else
    openssl rand -hex 24 > "$work_dir/privateai-ssl-pwd"
  fi
  chmod 600 "$work_dir/api-key" "$work_dir/privateai-ssl-pwd"

  cat > "$work_dir/openssl.cnf" <<EOF
[req]
distinguished_name = req_distinguished_name
x509_extensions = v3_req
prompt = no

[req_distinguished_name]
CN = $common_name

[v3_req]
subjectAltName = @alt_names

[alt_names]
DNS.1 = $common_name
DNS.2 = ${PRIVATEAI_NAME}.${PRIVATEAI_CASE_NAMESPACE}.svc.cluster.local
DNS.3 = localhost
IP.1 = 127.0.0.1
EOF

  openssl req -x509 -newkey rsa:2048 -nodes \
    -keyout "$work_dir/tls.key" \
    -out "$work_dir/tls.crt" \
    -days 7 -config "$work_dir/openssl.cnf" >/dev/null 2>&1
  chmod 600 "$work_dir/tls.key"
  openssl pkcs12 -export \
    -inkey "$work_dir/tls.key" \
    -in "$work_dir/tls.crt" \
    -name privateai \
    -out "$work_dir/keystore.p12" \
    -passout "file:$work_dir/privateai-ssl-pwd" >/dev/null 2>&1
  chmod 600 "$work_dir/keystore.p12"

  privateai_case_kubectl -n "$PRIVATEAI_CASE_NAMESPACE" create secret generic \
    "$PRIVATEAI_AUTH_SECRET" \
    --from-file=api-key="$work_dir/api-key" \
    --from-file=privateai-ssl-pwd="$work_dir/privateai-ssl-pwd" \
    --dry-run=client -o yaml | privateai_case_kubectl apply -f - >/dev/null
  privateai_case_kubectl -n "$PRIVATEAI_CASE_NAMESPACE" create secret generic \
    "$PRIVATEAI_TLS_SECRET" \
    --from-file=tls.crt="$work_dir/tls.crt" \
    --from-file=tls.key="$work_dir/tls.key" \
    --from-file=keystore.p12="$work_dir/keystore.p12" \
    --dry-run=client -o yaml | privateai_case_kubectl apply -f - >/dev/null
}

privateai_case_remove_secret_material() {
  if [ -n "${PRIVATEAI_SECRET_TMP_DIR:-}" ] && [ -d "$PRIVATEAI_SECRET_TMP_DIR" ]; then
    rm -rf -- "$PRIVATEAI_SECRET_TMP_DIR"
  fi
}

privateai_case_collect_diagnostics() {
  local reason="${1:-final}"
  local diagnostics_dir="$PRIVATEAI_CASE_RESULTS_DIR/diagnostics"
  mkdir -p "$diagnostics_dir"
  privateai_case_kubectl -n "$PRIVATEAI_CASE_NAMESPACE" \
    get privateai "$PRIVATEAI_NAME" -o yaml > "$diagnostics_dir/privateai.yaml" 2>&1 || true
  privateai_case_kubectl -n "$PRIVATEAI_CASE_NAMESPACE" \
    get deployment "$PRIVATEAI_NAME" -o yaml > "$diagnostics_dir/deployment.yaml" 2>&1 || true
  privateai_case_kubectl -n "$PRIVATEAI_CASE_NAMESPACE" \
    get pods,svc,events -o yaml > "$diagnostics_dir/workloads.yaml" 2>&1 || true
  privateai_case_kubectl -n "$PRIVATEAI_CASE_NAMESPACE" \
    logs "deployment/$PRIVATEAI_NAME" --all-containers --prefix \
    > "$diagnostics_dir/deployment.log" 2>&1 || true
  printf 'reason=%s\nnamespace=%s\nname=%s\nimage=%s\nauth_secret=%s\ntls_secret=%s\n' \
    "$reason" "$PRIVATEAI_CASE_NAMESPACE" "$PRIVATEAI_NAME" "$PRIVATEAI_IMAGE" \
    "$PRIVATEAI_AUTH_SECRET" "$PRIVATEAI_TLS_SECRET" \
    > "$diagnostics_dir/run-info.txt"
}

privateai_case_assert_image_usage() {
  local spec_policy deployment_policy pod pod_policy image_id expected_digest
  spec_policy="$(privateai_case_kubectl -n "$PRIVATEAI_CASE_NAMESPACE" get privateai "$PRIVATEAI_NAME" -o jsonpath='{.spec.runtime.image.pullPolicy}')"
  [ "$spec_policy" = "$PRIVATEAI_IMAGE_PULL_POLICY" ] || {
    echo "PrivateAI test01: CR image pullPolicy is '$spec_policy', expected '$PRIVATEAI_IMAGE_PULL_POLICY'" >&2
    return 1
  }
  deployment_policy="$(privateai_case_kubectl -n "$PRIVATEAI_CASE_NAMESPACE" get deployment "$PRIVATEAI_NAME" -o jsonpath='{.spec.template.spec.containers[0].imagePullPolicy}')"
  [ "$deployment_policy" = "$PRIVATEAI_IMAGE_PULL_POLICY" ] || {
    echo "PrivateAI test01: Deployment imagePullPolicy is '$deployment_policy', expected '$PRIVATEAI_IMAGE_PULL_POLICY'" >&2
    return 1
  }
  expected_digest=""
  [[ "$PRIVATEAI_IMAGE" == *@sha256:* ]] && expected_digest="${PRIVATEAI_IMAGE##*@}"
  mapfile -t pods < <(privateai_case_kubectl -n "$PRIVATEAI_CASE_NAMESPACE" get pods -l "app.kubernetes.io/instance=PrivateAi-$PRIVATEAI_NAME" -o jsonpath='{range .items[*]}{.metadata.name}{"\n"}{end}')
  [ "${#pods[@]}" -gt 0 ] || {
    echo "PrivateAI test01: no runtime Pods found for image verification" >&2
    return 1
  }
  for pod in "${pods[@]}"; do
    pod_policy="$(privateai_case_kubectl -n "$PRIVATEAI_CASE_NAMESPACE" get pod "$pod" -o jsonpath='{.spec.containers[0].imagePullPolicy}')"
    [ "$pod_policy" = "$PRIVATEAI_IMAGE_PULL_POLICY" ] || {
      echo "PrivateAI test01: Pod $pod imagePullPolicy is '$pod_policy', expected '$PRIVATEAI_IMAGE_PULL_POLICY'" >&2
      return 1
    }
    image_id="$(privateai_case_kubectl -n "$PRIVATEAI_CASE_NAMESPACE" get pod "$pod" -o jsonpath='{.status.containerStatuses[0].imageID}')"
    [ -n "$image_id" ] || {
      echo "PrivateAI test01: Pod $pod has no resolved imageID" >&2
      return 1
    }
    if [ -n "$expected_digest" ] && [[ "$image_id" != *"$expected_digest" ]]; then
      echo "PrivateAI test01: Pod $pod imageID '$image_id' does not match '$expected_digest'" >&2
      return 1
    fi
  done
}
