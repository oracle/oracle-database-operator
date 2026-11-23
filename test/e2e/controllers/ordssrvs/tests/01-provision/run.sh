#!/usr/bin/env bash
set -euo pipefail

echo "Executing test/e2e/controllers/ordssrvs/tests/01-provision/run.sh"

# Provision a minimal OrdsSrvs against the retained SIDB, then verify health.
ordssrvs_fail() {
  printf 'OrdsSrvs test01: %s\n' "$*" >&2
  exit 1
}

# Use the configured Kubernetes context and bounded request timeout.
if ! declare -F ordssrvs_kubectl >/dev/null 2>&1; then
  command -v kubectl >/dev/null 2>&1 || ordssrvs_fail "kubectl is required"
  ordssrvs_kubectl() {
    local -a args=(--request-timeout=30s)
    [ -z "${KUBE_CONTEXT_NAME:-}" ] || args+=(--context "$KUBE_CONTEXT_NAME")
    kubectl "${args[@]}" "$@"
  }
fi

sidb_namespace="${SIDB_CASE_NAMESPACE:-${SIDB_E2E_NAMESPACE:-sidb-ns}}"
sidb_name="${SIDB_TEST_NAME:-sidb-e2e-primary}"

sidb_query='{.metadata.name}{"\n"}{.metadata.namespace}{"\n"}'
sidb_query+='{.spec.sid}{"\n"}{.spec.pdbName}{"\n"}'
sidb_query+='{.status.pdbConnectString}{"\n"}'
sidb_query+='{.spec.security.secrets.admin.secretName}{"\n"}'
sidb_query+='{.spec.security.secrets.admin.secretKey}{"\n"}'
echo "OrdsSrvs test01: listing SIDBs in namespace $sidb_namespace"
ordssrvs_kubectl -n "$sidb_namespace" get singleinstancedatabase
echo "OrdsSrvs test01: waiting for SIDB $sidb_namespace/$sidb_name to become Healthy (timeout: 300s)"
ordssrvs_kubectl --request-timeout=0 -n "$sidb_namespace" \
  wait "singleinstancedatabase/$sidb_name" \
  --for=jsonpath='{.status.status}'=Healthy --timeout=300s ||
  ordssrvs_fail "SIDB $sidb_namespace/$sidb_name did not become Healthy within 300s"
echo "OrdsSrvs test01: SIDB readiness wait passed; reading PDB connection details and admin Secret reference"
sidb_fields="$(ordssrvs_kubectl -n "$sidb_namespace" \
  get singleinstancedatabase "$sidb_name" -o "jsonpath=$sidb_query")" ||
  ordssrvs_fail "cannot read SIDB $sidb_namespace/$sidb_name"

{
  IFS= read -r actual_name || true
  IFS= read -r actual_namespace || true
  IFS= read -r sidb_sid || true
  IFS= read -r sidb_pdb || true
  IFS= read -r sidb_connect_string || true
  IFS= read -r sidb_admin_secret || true
  IFS= read -r sidb_admin_key || true
} <<< "$sidb_fields"
unset sidb_fields sidb_query

for field in actual_name actual_namespace sidb_sid sidb_pdb sidb_connect_string sidb_admin_secret sidb_admin_key; do
  [ -n "${!field}" ] || ordssrvs_fail "SIDB field is empty or missing: $field"
done
[ "$actual_name" = "$sidb_name" ] && [ "$actual_namespace" = "$sidb_namespace" ] ||
  ordssrvs_fail "SIDB does not match requested $sidb_namespace/$sidb_name"

case "${sidb_connect_string,,}" in
  unavailable|unknown|pending|*[[:space:]]*)
    ordssrvs_fail "SIDB PDB connection string is unresolved: $sidb_connect_string"
    ;;
esac

ORDSSRVS_E2E_NAMESPACE="${ORDSSRVS_E2E_NAMESPACE:-ordssrvs-ns}"
ORDSSRVS_TEST_NAME="${ORDSSRVS_TEST_NAME:-ordssrvs-e2e-base}"
ORDS_IMAGE="${ORDS_IMAGE:-${ORDSSRVS_IMAGE:-${ORDSIMG:-container-registry.oracle.com/database/ords:latest}}}"
ORDSSRVS_AUTH_SECRET="${ORDSSRVS_AUTH_SECRET:-ordssrvs-auth}"
ORDSSRVS_DB_URL="jdbc:oracle:thin:@//$sidb_connect_string"

printf '%s\n' \
  'OrdsSrvs test01 setup parameters:' \
  "  SIDB: $sidb_namespace/$sidb_name" \
  "  SID: $sidb_sid" \
  "  PDB: $sidb_pdb" \
  '  SIDB readiness: Healthy wait passed' \
  "  OrdsSrvs namespace: $ORDSSRVS_E2E_NAMESPACE" \
  "  OrdsSrvs name: $ORDSSRVS_TEST_NAME" \
  "  ORDS image: $ORDS_IMAGE" \
  '  poolName: default' \
  '  autoUpgradeORDS: true' \
  '  db.connectionType: customurl' \
  "  db.customURL: $ORDSSRVS_DB_URL" \
  '  db.username: ORDS_PUBLIC_USER' \
  "  db.secret: ${ORDSSRVS_AUTH_SECRET}[dbAuth]" \
  '  db.adminUser: SYS' \
  "  db.adminUser.secret: ${ORDSSRVS_AUTH_SECRET}[adminAuth]" \
  "  admin credential source: $sidb_namespace/${sidb_admin_secret}[$sidb_admin_key]" \
  'Using the default Kubernetes service account.'


script_dir="$(CDPATH='' cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
: "${ORDSSRVS_TIMEOUT_SECONDS:=900}"
: "${ORDSSRVS_CURL_ATTEMPTS:=5}"
for number in "$ORDSSRVS_TIMEOUT_SECONDS" "$ORDSSRVS_CURL_ATTEMPTS"; do
  [[ "$number" =~ ^[1-9][0-9]*$ ]] || ordssrvs_fail "timeouts and attempt counts must be positive integers"
done
for value in "$ORDSSRVS_E2E_NAMESPACE" "$ORDSSRVS_TEST_NAME" "$ORDS_IMAGE" "$ORDSSRVS_AUTH_SECRET" "$ORDSSRVS_DB_URL"; do
  case "$value" in
    *[\"\'\\]*|*$'\n'*|*$'\r'*) ordssrvs_fail "unsupported character in manifest parameter" ;;
  esac
done

# Capture resource state on failure, without dumping Secret contents.
ordssrvs_diagnostics() {
  echo "OrdsSrvs test01: collecting CR, Pod, Service, and event diagnostics"
  ordssrvs_kubectl -n "$ORDSSRVS_E2E_NAMESPACE" get ordssrvs "$ORDSSRVS_TEST_NAME" -o yaml || true
  ordssrvs_kubectl -n "$ORDSSRVS_E2E_NAMESPACE" get pods,svc -l "app.kubernetes.io/instance=$ORDSSRVS_TEST_NAME" -o wide || true
  ordssrvs_kubectl -n "$ORDSSRVS_E2E_NAMESPACE" get events --sort-by=.lastTimestamp || true
}
ordssrvs_on_exit() {
  local rc="$?"
  if [ "$rc" -ne 0 ]; then
    ordssrvs_diagnostics
  fi
  exit "$rc"
}
trap ordssrvs_on_exit EXIT

echo "OrdsSrvs test01: ensuring namespace $ORDSSRVS_E2E_NAMESPACE exists"
if ! ordssrvs_kubectl get namespace "$ORDSSRVS_E2E_NAMESPACE" >/dev/null 2>&1; then
  ordssrvs_kubectl create namespace "$ORDSSRVS_E2E_NAMESPACE" >/dev/null
fi

# Preserve the runtime password on reruns; copy the SIDB admin value encoded.
echo "OrdsSrvs test01: preparing runtime and admin credentials in Secret $ORDSSRVS_E2E_NAMESPACE/$ORDSSRVS_AUTH_SECRET; password values are not logged"
db_auth="$(ordssrvs_kubectl -n "$ORDSSRVS_E2E_NAMESPACE" \
  get secret "$ORDSSRVS_AUTH_SECRET" --ignore-not-found=true \
  -o jsonpath='{.data.dbAuth}')" ||
  ordssrvs_fail "cannot read OrdsSrvs auth Secret"
if [ -z "$db_auth" ]; then
  command -v openssl >/dev/null 2>&1 || ordssrvs_fail "openssl is required to generate the ORDS runtime password"
  runtime_password="E2e_$(openssl rand -hex 24)" || ordssrvs_fail "cannot generate ORDS runtime password"
  db_auth="$(printf '%s' "$runtime_password" | openssl base64 -A)"
  unset runtime_password
fi

[[ "$sidb_admin_key" =~ ^[a-zA-Z0-9._-]+$ ]] || ordssrvs_fail "invalid SIDB admin Secret key"
admin_key_path="${sidb_admin_key//./\\.}"
admin_auth="$(ordssrvs_kubectl -n "$sidb_namespace" \
  get secret "$sidb_admin_secret" -o "jsonpath={.data.${admin_key_path}}")" ||
  ordssrvs_fail "cannot read SIDB admin Secret"
[ -n "$admin_auth" ] || ordssrvs_fail "SIDB admin Secret key is empty or missing"

ordssrvs_kubectl apply -f - >/dev/null <<EOF
apiVersion: v1
kind: Secret
metadata:
  name: "$ORDSSRVS_AUTH_SECRET"
  namespace: "$ORDSSRVS_E2E_NAMESPACE"
  labels:
    e2e.oracle.com/owner: operator-e2e
    e2e.oracle.com/controller: ordssrvs
type: Opaque
data:
  dbAuth: "$db_auth"
  adminAuth: "$admin_auth"
EOF
unset db_auth admin_auth admin_key_path
printf '%s\n' 'OrdsSrvs credentials prepared; password values are not logged.'

echo "OrdsSrvs test01: rendering manifest for $ORDSSRVS_E2E_NAMESPACE/$ORDSSRVS_TEST_NAME"
manifest="$(cat "$script_dir/test01.yaml")"
manifest="${manifest//__NAME__/"$ORDSSRVS_TEST_NAME"}"
manifest="${manifest//__NAMESPACE__/"$ORDSSRVS_E2E_NAMESPACE"}"
manifest="${manifest//__IMAGE__/"$ORDS_IMAGE"}"
manifest="${manifest//__URL__/"$ORDSSRVS_DB_URL"}"
manifest="${manifest//__SECRET__/"$ORDSSRVS_AUTH_SECRET"}"
echo "OrdsSrvs test01: validating manifest with server dry-run"
printf '%s\n' "$manifest" | ordssrvs_kubectl apply --dry-run=server -f - >/dev/null
echo "OrdsSrvs test01: applying OrdsSrvs manifest"
printf '%s\n' "$manifest" | ordssrvs_kubectl apply -f -

echo "OrdsSrvs test01: waiting for CR status Healthy (timeout: ${ORDSSRVS_TIMEOUT_SECONDS}s)"
ordssrvs_kubectl --request-timeout=0 -n "$ORDSSRVS_E2E_NAMESPACE" wait "ordssrvs/$ORDSSRVS_TEST_NAME" \
  --for=jsonpath='{.status.status}'=Healthy --timeout="${ORDSSRVS_TIMEOUT_SECONDS}s"
echo "OrdsSrvs test01: waiting for CR condition Available (timeout: ${ORDSSRVS_TIMEOUT_SECONDS}s)"
ordssrvs_kubectl --request-timeout=0 -n "$ORDSSRVS_E2E_NAMESPACE" wait "ordssrvs/$ORDSSRVS_TEST_NAME" \
  --for=condition=Available --timeout="${ORDSSRVS_TIMEOUT_SECONDS}s"
echo "OrdsSrvs test01: waiting for Pod readiness (timeout: ${ORDSSRVS_TIMEOUT_SECONDS}s)"
ordssrvs_kubectl --request-timeout=0 -n "$ORDSSRVS_E2E_NAMESPACE" wait pod \
  -l "app.kubernetes.io/instance=$ORDSSRVS_TEST_NAME" --for=condition=Ready \
  --timeout="${ORDSSRVS_TIMEOUT_SECONDS}s"
echo "OrdsSrvs test01: waiting for pool health Healthy (timeout: ${ORDSSRVS_TIMEOUT_SECONDS}s)"
ordssrvs_kubectl --request-timeout=0 -n "$ORDSSRVS_E2E_NAMESPACE" wait "ordssrvs/$ORDSSRVS_TEST_NAME" \
  --for=jsonpath='{.status.poolsHealth}'=Healthy --timeout="${ORDSSRVS_TIMEOUT_SECONDS}s"

printf 'OrdsSrvs status after health waits: %s/%s\n' "$ORDSSRVS_E2E_NAMESPACE" "$ORDSSRVS_TEST_NAME"
ordssrvs_kubectl -n "$ORDSSRVS_E2E_NAMESPACE" get ordssrvs.database.oracle.com "$ORDSSRVS_TEST_NAME"

echo "OrdsSrvs test01: checking pool counts (expected: total=1 OK=1 failed=0)"
pool_counts="$(ordssrvs_kubectl -n "$ORDSSRVS_E2E_NAMESPACE" \
  get ordssrvs "$ORDSSRVS_TEST_NAME" \
  -o jsonpath='{.status.poolsTotal}{"|"}{.status.poolsOK}{"|"}{.status.poolsFailed}')"
IFS='|' read -r pools_total pools_ok pools_failed <<< "$pool_counts"
[ "$pools_total" = 1 ] && [ "$pools_ok" = 1 ] && [ "$pools_failed" = 0 ] ||
  ordssrvs_fail "unexpected pool counts: total=$pools_total OK=$pools_ok failed=$pools_failed"

echo "OrdsSrvs test01: selecting a Ready Pod for logs and HTTPS checks"
pod_rows="$(ordssrvs_kubectl -n "$ORDSSRVS_E2E_NAMESPACE" get pods \
  -l "app.kubernetes.io/instance=$ORDSSRVS_TEST_NAME" \
  -o jsonpath='{range .items[*]}{.metadata.name}{"|"}{.metadata.deletionTimestamp}{"|"}{.status.conditions[?(@.type=="Ready")].status}{"\n"}{end}')"
pod=''
while IFS='|' read -r pod_name deletion_timestamp ready; do
  if [ -n "$pod_name" ] && [ -z "$deletion_timestamp" ] && [ "$ready" = True ]; then
    pod="$pod_name"
    break
  fi
done <<< "$pod_rows"
[ -n "$pod" ] || ordssrvs_fail "no Ready OrdsSrvs Pod"
separator='+++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++'
printf '\n%s\nordssrvs-init logs (%s; full output)\n' "$separator" "$pod"
ordssrvs_kubectl -n "$ORDSSRVS_E2E_NAMESPACE" logs "$pod" -c ordssrvs-init --tail=-1 ||
  printf 'Unable to read ordssrvs-init logs for %s\n' "$pod" >&2
printf '%s\n' "$separator"
printf '\n%s\nordssrvs-main logs (%s; last 10 lines)\n' "$separator" "$pod"
ordssrvs_kubectl -n "$ORDSSRVS_E2E_NAMESPACE" logs "$pod" -c ordssrvs-main --tail=10 ||
  printf 'Unable to read ordssrvs-main logs for %s\n' "$pod" >&2
printf '%s\n' "$separator"

base_url="https://${ORDSSRVS_TEST_NAME}.${ORDSSRVS_E2E_NAMESPACE}.svc:8443"
ordssrvs_check_url() {
  local check="$1" url="$2" attempt response http_status download_size redirect_url
  printf 'Checking ORDS %s URL: %s\n' "$check" "$url"
  for ((attempt=1; attempt<=ORDSSRVS_CURL_ATTEMPTS; attempt++)); do
    if response="$(ordssrvs_kubectl -n "$ORDSSRVS_E2E_NAMESPACE" exec "$pod" -c ordssrvs-main -- \
      curl --insecure --silent --show-error --fail --connect-timeout 5 --max-time 10 \
      -H 'Host: localhost' --output /dev/null \
      --write-out $'%{http_code}\n%{size_download}\n%{redirect_url}\n' "$url")"; then
      {
        IFS= read -r http_status
        IFS= read -r download_size
        IFS= read -r redirect_url || true
      } <<< "$response"
      printf 'ORDS %s response: HTTP %s, downloaded %s bytes\n' "$check" "$http_status" "$download_size"
      [ -z "$redirect_url" ] || printf 'ORDS %s redirect: %s\n' "$check" "$redirect_url"
      if [ "$check" = favicon ]; then
        if [ "$http_status" = 200 ] && [[ "$download_size" =~ ^[0-9]+$ ]] && [ "$download_size" -gt 0 ]; then
          return 0
        fi
      elif [[ "$http_status" =~ ^2[0-9][0-9]$ ]] ||
           { [[ "$http_status" =~ ^3[0-9][0-9]$ ]] && [ -n "$redirect_url" ]; }; then
        return 0
      fi
    fi
    [ "$attempt" -eq "$ORDSSRVS_CURL_ATTEMPTS" ] || sleep 2
  done
  ordssrvs_fail "ORDS $check check failed after $ORDSSRVS_CURL_ATTEMPTS attempts"
}

ordssrvs_check_url favicon "$base_url/favicon.ico"
ordssrvs_check_url 'default pool' "$base_url/ords/"
printf '%s\n' 'OrdsSrvs test01 passed: Healthy, Ready Pod, healthy default pool, HTTPS favicon and pool response.'
