#!/usr/bin/env bash
# Shared True Cache lifecycle for truecache/test01 and test02.
# test01: TDE day-0, no TCPS (baseline True Cache path)
# test02: TDE day-0 + operator-managed TCPS (secure True Cache path)
set -Eeuo pipefail

case_id="${TRUECACHE_CASE_ID:-${SIDB_TRUECACHE_CASE_ID:-test01}}"
case "$case_id" in
  01|test01) case_id=test01 ;;
  02|test02) case_id=test02 ;;
  *)
    echo "TRUECACHE_CASE_ID must be test01 or test02; got: $case_id" >&2
    exit 2
    ;;
esac

case_dir="${TRUECACHE_CASE_DIR:-$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../${case_id#test}-tcp" && pwd)}"
if [ "$case_id" = "test02" ]; then
  case_dir="${TRUECACHE_CASE_DIR:-$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../02-tcps" && pwd)}"
fi
namespace="${TRUECACHE_E2E_NAMESPACE:-${SIDB_TRUECACHE_NAMESPACE:-${SIDB_E2E_NAMESPACE:-sidb-ns}}}"
image="${TRUECACHE_IMAGE:-${SIDB_TRUECACHE_IMAGE:-${SIDB_IMAGE:?Set TRUECACHE_IMAGE, SIDB_TRUECACHE_IMAGE, or SIDB_IMAGE as an environment variable}}}"
storage_class="${TRUECACHE_STORAGE_CLASS:-${SIDB_STORAGE_CLASS:-oci-bv}}"
access_mode="${TRUECACHE_ACCESS_MODE:-ReadWriteOnce}"
image_pull_policy="${TRUECACHE_IMAGE_PULL_POLICY:-IfNotPresent}"
pull_secret="${TRUECACHE_PULL_SECRET:-${SIDB_TRUECACHE_PULL_SECRET:-${SIDB_PULL_SECRET:-}}}"
pipeline="${CI_PIPELINE_ID:-local}"
timeout="${TRUECACHE_TIMEOUT_SECONDS:-${SIDB_TRUECACHE_TIMEOUT_SECONDS:-1800}}"
poll="${TRUECACHE_POLL_INTERVAL:-${SIDB_TRUECACHE_POLL_INTERVAL:-20}}"

if [ "$case_id" = "test02" ]; then
  primary="${TRUECACHE_PRIMARY_NAME:-${SIDB_TRUECACHE_PRIMARY_NAME:-sidb-e2e-tc-primary-test02}}"
  cache="${TRUECACHE_NAME:-${SIDB_TRUECACHE_NAME:-sidb-e2e-truecache-test02}}"
  admin_secret="${TRUECACHE_ADMIN_SECRET:-${SIDB_TRUECACHE_ADMIN_SECRET:-sidb-e2e-tc-admin-test02}}"
  tde_secret="${TRUECACHE_TDE_SECRET:-${SIDB_TRUECACHE_TDE_SECRET:-sidb-e2e-tc-tde-test02}}"
  tcps_tls_secret="${TRUECACHE_TCPS_TLS_SECRET:-${SIDB_TRUECACHE_TCPS_TLS_SECRET:-sidb-e2e-tc-tcps-tls-test02}}"
  enable_tcps=1
else
  primary="${TRUECACHE_PRIMARY_NAME:-${SIDB_TRUECACHE_PRIMARY_NAME:-sidb-e2e-tc-primary}}"
  cache="${TRUECACHE_NAME:-${SIDB_TRUECACHE_NAME:-sidb-e2e-truecache}}"
  admin_secret="${TRUECACHE_ADMIN_SECRET:-${SIDB_TRUECACHE_ADMIN_SECRET:-sidb-e2e-tc-admin}}"
  tde_secret="${TRUECACHE_TDE_SECRET:-${SIDB_TRUECACHE_TDE_SECRET:-sidb-e2e-tc-tde}}"
  tcps_tls_secret=""
  enable_tcps=0
fi

results_dir="${OPERATOR_LOG_DIR:-test/e2e/controllers/truecache/results}/truecache/${case_id}-${namespace}"

if [ -n "${KUBE_CONTEXT_NAME:-}" ]; then
  kubectl_cmd=(kubectl --context "$KUBE_CONTEXT_NAME")
else
  kubectl_cmd=(kubectl)
fi
k() { "${kubectl_cmd[@]}" "$@"; }

mkdir -p "$results_dir/manifests" "$results_dir/secret-manifests"

if ! k get namespace "$namespace" >/dev/null 2>&1; then
  k create namespace "$namespace"
fi
k label namespace "$namespace" e2e.oracle.com/owner=operator-e2e \
  e2e.oracle.com/controller=truecache e2e.oracle.com/test-id="${case_id}" \
  e2e.oracle.com/pipeline="$pipeline" --overwrite >/dev/null

admin_password="${TRUECACHE_ADMIN_PASSWORD:-${SIDB_TRUECACHE_ADMIN_PASSWORD:-}}"
tde_password="${TRUECACHE_TDE_PASSWORD:-${SIDB_TRUECACHE_TDE_PASSWORD:-}}"
if [ -z "$admin_password" ]; then admin_password="E2e_$(openssl rand -hex 12)"; fi
if [ -z "$tde_password" ]; then tde_password="E2e_Tde_$(openssl rand -hex 12)"; fi

k -n "$namespace" create secret generic "$admin_secret" \
  --from-literal=oracle_pwd="$admin_password" --dry-run=client -o yaml |
  tee "$results_dir/secret-manifests/admin-secret.yaml" | k -n "$namespace" apply -f - >/dev/null
k -n "$namespace" create secret generic "$tde_secret" \
  --from-literal=tde_wallet_pwd="$tde_password" --dry-run=client -o yaml |
  tee "$results_dir/secret-manifests/tde-secret.yaml" | k -n "$namespace" apply -f - >/dev/null

create_tcps_tls_secret() {
  local temporary_dir certificate_file key_file san

  [ "$enable_tcps" -eq 1 ] || return 0
  if k -n "$namespace" get secret "$tcps_tls_secret" >/dev/null 2>&1; then
    k -n "$namespace" get secret "$tcps_tls_secret" -o yaml \
      > "$results_dir/secret-manifests/tcps-tls-secret-existing.yaml"
    return 0
  fi

  command -v openssl >/dev/null 2>&1 || {
    echo "SIDB truecache/${case_id}: openssl is required to generate the TCPS TLS Secret" >&2
    return 1
  }

  temporary_dir="$(mktemp -d "${TMPDIR:-/tmp}/sidb-tc-tcps.XXXXXX")"
  certificate_file="$temporary_dir/tls.crt"
  key_file="$temporary_dir/tls.key"
  san="DNS:${primary},DNS:${primary}.${namespace},DNS:${primary}.${namespace}.svc,DNS:${primary}.${namespace}.svc.cluster.local,DNS:${cache},DNS:${cache}.${namespace},DNS:${cache}.${namespace}.svc,DNS:${cache}.${namespace}.svc.cluster.local"

  if ! openssl req -x509 -nodes -newkey rsa:2048 -days 365 \
    -subj "/CN=${primary}" \
    -addext "subjectAltName=${san}" \
    -keyout "$key_file" -out "$certificate_file" >/dev/null 2>&1; then
    rm -rf -- "$temporary_dir"
    echo "SIDB truecache/${case_id}: failed to generate the TCPS test certificate" >&2
    return 1
  fi

  if ! k -n "$namespace" create secret tls "$tcps_tls_secret" \
    --cert="$certificate_file" --key="$key_file" \
    --dry-run=client -o yaml |
    tee "$results_dir/secret-manifests/tcps-tls-secret.yaml" |
    k -n "$namespace" apply -f - >/dev/null; then
    rm -rf -- "$temporary_dir"
    echo "SIDB truecache/${case_id}: failed to create TCPS TLS Secret $tcps_tls_secret" >&2
    return 1
  fi
  rm -rf -- "$temporary_dir"
}

create_tcps_tls_secret


render_template() {
  local source_file="$1" destination_file="$2" line
  mkdir -p "$(dirname "$destination_file")"
  : > "$destination_file"
  while IFS= read -r line || [ -n "$line" ]; do
    if [ -z "$pull_secret" ] && [ "$line" = '    pullSecrets: "__TRUECACHE_PULL_SECRET__"' ]; then
      continue
    fi
    line="${line//__TRUECACHE_CASE_ID__/$case_id}"
    line="${line//__TRUECACHE_PRIMARY_NAME__/$primary}"
    line="${line//__TRUECACHE_NAME__/$cache}"
    line="${line//__TRUECACHE_PIPELINE__/$pipeline}"
    line="${line//__TRUECACHE_ADMIN_SECRET__/$admin_secret}"
    line="${line//__TRUECACHE_TDE_SECRET__/$tde_secret}"
    line="${line//__TRUECACHE_TCPS_TLS_SECRET__/$tcps_tls_secret}"
    line="${line//__TRUECACHE_IMAGE__/$image}"
    line="${line//__TRUECACHE_IMAGE_PULL_POLICY__/$image_pull_policy}"
    line="${line//__TRUECACHE_PULL_SECRET__/$pull_secret}"
    line="${line//__TRUECACHE_STORAGE_CLASS__/$storage_class}"
    line="${line//__TRUECACHE_ACCESS_MODE__/$access_mode}"
    printf '%s\n' "$line" >> "$destination_file"
  done < "$source_file"
}

primary_template="$case_dir/primary.yaml"
truecache_template="$case_dir/truecache.yaml"
[ -f "$primary_template" ] && [ -f "$truecache_template" ] || {
  echo "True Cache ${case_id}: template files are missing under $case_dir" >&2
  exit 2
}
echo "True Cache ${case_id}: rendering templates $primary_template and $truecache_template"
render_template "$primary_template" "$results_dir/manifests/primary.yaml"
render_template "$truecache_template" "$results_dir/manifests/truecache.yaml"

wait_sidb() {
  local name="$1" expected_role="$2" elapsed=0 status role
  while [ "$elapsed" -lt "$timeout" ]; do
    status="$(k -n "$namespace" get sidb "$name" -o jsonpath='{.status.status}' 2>/dev/null || true)"
    role="$(k -n "$namespace" get sidb "$name" -o jsonpath='{.status.role}' 2>/dev/null || true)"
    echo "SIDB truecache/${case_id} readiness: name=$name elapsed=${elapsed}s status=${status:-<empty>} role=${role:-<empty>}"
    case "$status" in
      Failed|FAILED|Error|ERROR) return 1 ;;
      Unhealthy|UNHEALTHY)
        echo "SIDB truecache/${case_id}: $name is still initializing; continuing to poll"
        ;;
      Ready|READY|Healthy|HEALTHY|Available|AVAILABLE)
        [ "$role" = "$expected_role" ] && return 0
        ;;
    esac
    sleep "$poll"
    elapsed=$((elapsed + poll))
  done
  echo "SIDB truecache/${case_id} timed out: name=$name status=${status:-<empty>} role=${role:-<empty>}" >&2
  k -n "$namespace" get sidb "$name" -o json |
    jq '{metadata: {name: .metadata.name, generation: .metadata.generation}, status: .status}' >&2 || true
  k -n "$namespace" get pods -o wide >&2 || true
  k -n "$namespace" get events --sort-by=.lastTimestamp | tail -80 >&2 || true
  for pod in $(k -n "$namespace" get pods -l "app=$name" -o name 2>/dev/null || true); do
    k -n "$namespace" logs "$pod" --all-containers --tail=200 >&2 || true
    k -n "$namespace" logs "$pod" --all-containers --previous --tail=200 >&2 || true
    k -n "$namespace" describe "$pod" >&2 || true
  done
  return 1
}

assert_security() {
  local name="$1"
  local secret_name tcps_enabled tcps_secret

  secret_name="$(k -n "$namespace" get sidb "$name" -o jsonpath='{.spec.security.secrets.tde.secretName}')"
  [ "$secret_name" = "$tde_secret" ] || {
    echo "SIDB truecache/${case_id}: unexpected TDE secret on $name: ${secret_name:-<empty>}" >&2
    return 1
  }

  if [ "$enable_tcps" -eq 1 ]; then
    tcps_enabled="$(k -n "$namespace" get sidb "$name" -o jsonpath='{.spec.security.tcps.enabled}')"
    tcps_secret="$(k -n "$namespace" get sidb "$name" -o jsonpath='{.spec.security.tcps.tlsSecret}')"
    [ "$tcps_enabled" = "true" ] || {
      echo "SIDB truecache/${case_id}: TCPS is not enabled on $name" >&2
      return 1
    }
    [ "$tcps_secret" = "$tcps_tls_secret" ] || {
      echo "SIDB truecache/${case_id}: unexpected TCPS TLS Secret on $name: ${tcps_secret:-<empty>}" >&2
      return 1
    }
  else
    tcps_enabled="$(k -n "$namespace" get sidb "$name" -o jsonpath='{.spec.security.tcps.enabled}' 2>/dev/null || true)"
    if [ "$tcps_enabled" = "true" ]; then
      echo "SIDB truecache/${case_id}: expected TCPS disabled on $name for test01" >&2
      return 1
    fi
  fi
}

echo "SIDB truecache/${case_id}: applying primary $primary (tcps=${enable_tcps})"
k -n "$namespace" apply -f "$results_dir/manifests/primary.yaml"
wait_sidb "$primary" PRIMARY
assert_security "$primary"

echo "SIDB truecache/${case_id}: waiting for generated blob ConfigMap ${primary}-truecache-blob"
elapsed=0
while [ "$elapsed" -lt "$timeout" ]; do
  blob_status="$(k -n "$namespace" get sidb "$primary" -o json 2>/dev/null |
    jq -r '.status.conditions[]? | select(.type=="TrueCacheBlobReady") | .status' | tail -1 || true)"
  if [ "$blob_status" = "True" ] && k -n "$namespace" get configmap "${primary}-truecache-blob" >/dev/null 2>&1; then
    echo "SIDB truecache/${case_id}: primary blob and ConfigMap are ready"
    break
  fi
  echo "SIDB truecache/${case_id} blob readiness: elapsed=${elapsed}s condition=${blob_status:-<empty>}"
  sleep "$poll"
  elapsed=$((elapsed + poll))
done
[ "$elapsed" -lt "$timeout" ] || exit 1

echo "SIDB truecache/${case_id}: applying True Cache $cache"
k -n "$namespace" apply -f "$results_dir/manifests/truecache.yaml"
wait_sidb "$cache" TRUE_CACHE
assert_security "$cache"

for name in "$primary" "$cache"; do
  k -n "$namespace" wait --for=condition=Ready pod -l "app=$name" --timeout=120s >/dev/null
done
primary_cm="$(k -n "$namespace" get configmap "${primary}-truecache-blob" -o jsonpath='{.binaryData.tc_config_blob\.tar\.gz}' 2>/dev/null || true)"
[ -n "$primary_cm" ] || { echo "SIDB truecache/${case_id}: blob ConfigMap has no binary blob key" >&2; exit 1; }
pvc_count="$(k -n "$namespace" get pvc --no-headers | awk -v n="$cache" 'index($1,n)>0{c++} END{print c+0}')"
service_count="$(k -n "$namespace" get svc --no-headers | awk -v n="$cache" 'index($1,n)>0{c++} END{print c+0}')"
[ "$pvc_count" -gt 0 ] && [ "$service_count" -gt 0 ] || exit 1
k -n "$namespace" get sidb,pods,pvc,svc,configmap "${primary}-truecache-blob" -o yaml > "$results_dir/resources.yaml" 2>&1 || true
echo "SIDB truecache/${case_id} assertions passed: primary=$primary cache=$cache tcps=${enable_tcps} blobConfigMap=1 cachePVCs=$pvc_count cacheServices=$service_count"
