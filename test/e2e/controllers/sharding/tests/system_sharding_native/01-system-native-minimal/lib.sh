#!/usr/bin/env bash
set -Eeuo pipefail
SHARDING_CASE_DIR="$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
SHARDING_REPO_ROOT="$(CDPATH= cd -- "$SHARDING_CASE_DIR/../../../../../../.." && pwd)"
: "${SHARDING_E2E_NAMESPACE:=${E2E_NAMESPACE:-gdd-ns}}"
: "${SHARDING_NAME:=sdb-system}"
: "${SHARDING_DB_IMAGE:=container-registry.oracle.com/database/enterprise_ru:latest-26}"
: "${SHARDING_GSM_IMAGE:=container-registry.oracle.com/database/gsm_ru:latest-26}"
: "${SHARDING_STORAGE_CLASS:=oci-bv}"
: "${SHARDING_DB_PULL_SECRET:=ocr-reg-cred}"
: "${SHARDING_GSM_PULL_SECRET:=ocr-reg-cred}"
: "${SHARDING_DB_SECRET_NAME:=sharding-native-e2e-db-user-pass-${CI_PIPELINE_ID:-local}}"
: "${SHARDING_TIMEOUT_SECONDS:=7200}"
: "${SHARDING_POLL_INTERVAL:=20}"
: "${SHARDING_RESULTS_DIR:=${CI_PROJECT_DIR:-$SHARDING_REPO_ROOT}/test/e2e/controllers/sharding/results/native-minimal}"

sharding_kubectl() { kubectl -n "$SHARDING_E2E_NAMESPACE" "$@"; }
sharding_case_label() { printf 'sharding/native-minimal %s/%s' "$SHARDING_E2E_NAMESPACE" "$SHARDING_NAME"; }
sharding_case_init() { mkdir -p "$SHARDING_RESULTS_DIR"; chmod 700 "$SHARDING_RESULTS_DIR" 2>/dev/null || true; }

sharding_case_collect_diagnostics() {
  local reason="${1:-manual}" out pod
  out="$SHARDING_RESULTS_DIR/diagnostics-$reason"
  mkdir -p "$out/pods"
  sharding_kubectl get shardingdatabase "$SHARDING_NAME" -o yaml >"$out/shardingdatabase.yaml" 2>&1 || true
  sharding_kubectl get pods,svc,pvc,statefulsets -o wide >"$out/resources.txt" 2>&1 || true
  sharding_kubectl get events --sort-by=.lastTimestamp >"$out/events.txt" 2>&1 || true
  for pod in catalog-0 pshard1-0 pshard2-0 pshard3-0 gsm1-0 gsm2-0; do
    sharding_kubectl logs "$pod" --all-containers=true >"$out/pods/$pod.log" 2>&1 || true
    sharding_kubectl exec "$pod" -- cat /var/tmp/gdd/oracle_sharding_setup.log >"$out/pods/$pod-setup.log" 2>&1 || true
  done
  printf 'reason=%s\nnamespace=%s\nresource=%s\n' "$reason" "$SHARDING_E2E_NAMESPACE" "$SHARDING_NAME" >"$out/run-info.txt"
  echo "Diagnostics collected in $out"
}

sharding_case_render_manifest() {
  local source="$SHARDING_CASE_DIR/system-native-minimal.yaml.tmpl" target="$SHARDING_RESULTS_DIR/system-native-minimal.yaml"
  local pipeline_id="${CI_PIPELINE_ID:-local}" key value escaped
  [ -r "$source" ] || { echo "Missing template: $source" >&2; return 2; }
  cp "$source" "$target"; chmod 600 "$target"
  while IFS='=' read -r key value; do
    escaped="$(printf '%s' "$value" | sed 's/[\\&|]/\\&/g')"
    sed -i "s|__${key}__|${escaped}|g" "$target"
  done <<EOF
SHARDING_NAME=$SHARDING_NAME
SHARDING_NAMESPACE=$SHARDING_E2E_NAMESPACE
SHARDING_DB_IMAGE=$SHARDING_DB_IMAGE
SHARDING_DB_PULL_SECRET=$SHARDING_DB_PULL_SECRET
SHARDING_GSM_IMAGE=$SHARDING_GSM_IMAGE
SHARDING_GSM_PULL_SECRET=$SHARDING_GSM_PULL_SECRET
SHARDING_STORAGE_CLASS=$SHARDING_STORAGE_CLASS
SHARDING_DB_SECRET_NAME=$SHARDING_DB_SECRET_NAME
CI_PIPELINE_ID=$pipeline_id
EOF
  if grep -q '__[A-Z0-9_]*__' "$target"; then echo "Unrendered placeholder in $target" >&2; return 2; fi
  printf '%s\n' "$target"
}

sharding_case_create_db_secret() (
  set -Eeuo pipefail
  umask 077
  : "${SHARDING_DB_PASSWORD:?Set SHARDING_DB_PASSWORD as a masked GitLab CI/CD variable}"
  command -v openssl >/dev/null || { echo 'openssl is required in the E2E job image' >&2; return 2; }
  local workdir password_bytes
  workdir="$(mktemp -d "${TMPDIR:-/tmp}/sharding-secret.XXXXXX")"
  trap 'rm -rf "$workdir"' EXIT
  openssl genpkey -algorithm RSA -pkeyopt rsa_keygen_bits:3072 \
    -out "$workdir/key.pem" 2>/dev/null
  openssl pkey -in "$workdir/key.pem" -pubout -out "$workdir/key.pub" 2>/dev/null
  printf '%s' "$SHARDING_DB_PASSWORD" >"$workdir/pwdfile.txt"
  password_bytes="$(wc -c <"$workdir/pwdfile.txt" | tr -d '[:space:]')"
  if [ "$password_bytes" -gt 318 ]; then
    echo 'SHARDING_DB_PASSWORD is too long for RSA-3072 OAEP with SHA-256 (maximum 318 bytes)' >&2
    return 2
  fi
  openssl pkeyutl -encrypt -pubin -inkey "$workdir/key.pub" \
    -in "$workdir/pwdfile.txt" -out "$workdir/pwdfile.enc" \
    -pkeyopt rsa_padding_mode:oaep -pkeyopt rsa_oaep_md:sha256 \
    -pkeyopt rsa_mgf1_md:sha256
  openssl pkeyutl -decrypt -inkey "$workdir/key.pem" \
    -in "$workdir/pwdfile.enc" -out "$workdir/pwdfile.dec" \
    -pkeyopt rsa_padding_mode:oaep -pkeyopt rsa_oaep_md:sha256 \
    -pkeyopt rsa_mgf1_md:sha256
  cmp -s "$workdir/pwdfile.txt" "$workdir/pwdfile.dec" || {
    echo 'Local verification of the encrypted DB password failed' >&2; return 1;
  }
  rm -f "$workdir/pwdfile.txt" "$workdir/pwdfile.dec" "$workdir/key.pub"
  kubectl -n "$SHARDING_E2E_NAMESPACE" create secret generic "$SHARDING_DB_SECRET_NAME" \
    --from-file=pwdfile.enc="$workdir/pwdfile.enc" --from-file=key.pem="$workdir/key.pem" \
    --dry-run=client -o yaml | kubectl apply -f - >/dev/null
)

sharding_case_wait_pod_ready() {
  kubectl -n "$SHARDING_E2E_NAMESPACE" wait --for=condition=Ready "pod/$1" --timeout="${SHARDING_TIMEOUT_SECONDS}s" >/dev/null
}

sharding_case_log_contains() {
  local pod="$1" marker="$2" source="${3:-container-log}"
  if [ "$source" = setup-log ]; then
    sharding_kubectl exec "$pod" -- grep -Fq -- "$marker" /var/tmp/gdd/oracle_sharding_setup.log 2>/dev/null
  else
    # Do not use grep -q here: with pipefail, grep can exit early after a
    # match and cause kubectl logs to receive SIGPIPE, making the pipeline
    # return non-zero even though the marker was found.
    sharding_kubectl logs "$pod" --all-containers=true 2>/dev/null | grep -F -- "$marker" >/dev/null
  fi
}
