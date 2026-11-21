#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'USAGE'
Usage: run.sh --namespace <namespace> [--tests provision,clone,dataguard|all] [--profile <compatibility-alias>] [--cleanup]

Required environment:
  SIDB_IMAGE, SIDB_STORAGE_CLASS, SIDB_ADMIN_PASSWORD

Optional environment:
  SIDB_PULL_SECRET, SIDB_ADMIN_SECRET, SIDB_ADMIN_SECRET_KEY
  SIDB_PRIMARY_EXTERNAL_CONNECT_STRING (required for external Data Guard unless status.externalConnectString exists)
  SIDB_TIMEOUT_SECONDS (default 1500), SIDB_UNHEALTHY_GRACE_SECONDS (default 600), SIDB_POLL_INTERVAL (default 20)
USAGE
}
namespace=""; tests="${SIDB_TESTS:-${SIDB_PROFILE:-all}}"; cleanup=false
while [ "$#" -gt 0 ]; do
  case "$1" in
    --namespace) namespace="$2"; shift 2;;
    --tests) tests="$2"; shift 2;;
    --profile) tests="$2"; shift 2;;
    --cleanup) cleanup=true; shift;;
    -h|--help) usage; exit 0;;
    *) echo "Unknown argument: $1" >&2; usage >&2; exit 2;;
  esac
done
[ -n "$namespace" ] || { usage >&2; exit 2; }
case "$tests" in
  all|provision|clone|dataguard) ;;
  *,*) ;;
  *) echo "invalid tests: $tests" >&2; exit 2;;
esac
: "${SIDB_IMAGE:?Set SIDB_IMAGE as an environment variable}"
: "${SIDB_STORAGE_CLASS:=oci-bv}"
: "${SIDB_IMAGE:?SIDB_IMAGE is required}"
: "${SIDB_ADMIN_PASSWORD:?SIDB_ADMIN_PASSWORD is required}"
: "${SIDB_PULL_SECRET:=}"
: "${SIDB_ADMIN_SECRET:=sidb-e2e-admin}"
: "${SIDB_ADMIN_SECRET_KEY:=oracle_pwd}"
: "${SIDB_PRIMARY_NAME:=sidb-e2e-primary}"
: "${SIDB_PRIMARY_SID:=ORCL1}"
: "${SIDB_PRIMARY_PDB:=ORCLPDB1}"
: "${SIDB_CLONE_NAME:=sidb-e2e-clone}"
: "${SIDB_CLONE_SID:=ORCL2}"
: "${SIDB_STANDBY_NAME:=sidb-e2e-standby}"
: "${SIDB_STANDBY_SID:=ORCLS}"
: "${SIDB_BROKER_NAME:=sidb-e2e-dg}"
: "${SIDB_TIMEOUT_SECONDS:=1500}"
: "${SIDB_UNHEALTHY_GRACE_SECONDS:=600}"
: "${SIDB_POLL_INTERVAL:=20}"
command -v kubectl >/dev/null || { echo "kubectl is required" >&2; exit 2; }
# prepare-env.sh exports KUBECONFIG. Apply an optional named context without
# mutating the kubeconfig file supplied by the runner.
if [ -n "${KUBE_CONTEXT_NAME:-}" ]; then
  kubectl() { command kubectl --context "$KUBE_CONTEXT_NAME" "$@"; }
fi
results_root="${SIDB_E2E_RESULTS_DIR:-test/e2e/controllers/sidb/results}"
run_timestamp="$(date -u +%Y%m%dT%H%M%SZ)"
results_dir="${results_root}/${run_timestamp}-${tests//,/-}-${namespace}"
mkdir -p "$results_dir"

fail(){ echo "ASSERT FAIL: $*" >&2; exit 1; }
pass(){ echo "ASSERT PASS: $*"; }
cleanup_resources(){
  $cleanup || return 0
  kubectl -n "$namespace" delete dataguardbroker "$SIDB_BROKER_NAME" --ignore-not-found
  kubectl -n "$namespace" delete singleinstancedatabase "$SIDB_STANDBY_NAME" "$SIDB_CLONE_NAME" "$SIDB_PRIMARY_NAME" --ignore-not-found
}
trap cleanup_resources EXIT

wait_sidb(){
  local name="$1" elapsed=0 status role unhealthy_since=-1 unhealthy_elapsed
  while [ "$elapsed" -lt "$SIDB_TIMEOUT_SECONDS" ]; do
    status="$(kubectl -n "$namespace" get singleinstancedatabase "$name" -o jsonpath='{.status.status}' 2>/dev/null || true)"
    role="$(kubectl -n "$namespace" get singleinstancedatabase "$name" -o jsonpath='{.status.role}' 2>/dev/null || true)"
    case "$status" in
      Healthy|HEALTHY|Ready|READY|Available|AVAILABLE) pass "$name status=$status role=$role"; return 0;;
      Failed|FAILED|Error|ERROR) kubectl -n "$namespace" get singleinstancedatabase "$name" -o yaml > "$results_dir/${name}-failure.yaml" || true; fail "$name status=$status";;
      Unhealthy|UNHEALTHY)
        if [ "$unhealthy_since" -lt 0 ]; then
          unhealthy_since="$elapsed"
          echo "$name entered Unhealthy grace period: ${SIDB_UNHEALTHY_GRACE_SECONDS}s"
        fi
        unhealthy_elapsed=$((elapsed - unhealthy_since))
        if [ "$unhealthy_elapsed" -ge "$SIDB_UNHEALTHY_GRACE_SECONDS" ]; then
          kubectl -n "$namespace" get singleinstancedatabase "$name" -o yaml > "$results_dir/${name}-unhealthy.yaml" || true
          kubectl -n "$namespace" get pods -o wide > "$results_dir/${name}-pods.txt" || true
          kubectl -n "$namespace" get pvc -o wide > "$results_dir/${name}-pvc.txt" || true
          kubectl -n "$namespace" get events --sort-by=.lastTimestamp > "$results_dir/${name}-events.txt" || true
          fail "$name remained Unhealthy for ${unhealthy_elapsed}s"
        fi
        ;;
      *) unhealthy_since=-1;;
    esac
    sleep "$SIDB_POLL_INTERVAL"; elapsed=$((elapsed + SIDB_POLL_INTERVAL))
  done
  kubectl -n "$namespace" get singleinstancedatabase "$name" -o yaml > "$results_dir/${name}-timeout.yaml" || true
  fail "timed out waiting for $name"
}
assert_sidb(){
  local name="$1" expected_role="$2" role pod_count pvc_count
  role="$(kubectl -n "$namespace" get singleinstancedatabase "$name" -o jsonpath='{.status.role}')"
  [ "$role" = "$expected_role" ] || fail "$name role=$role expected=$expected_role"
  pod_count="$(kubectl -n "$namespace" get pods --no-headers 2>/dev/null | awk -v n="$name" '$1 ~ n {c++} END{print c+0}')"
  pvc_count="$(kubectl -n "$namespace" get pvc --no-headers 2>/dev/null | awk -v n="$name" '$1 ~ n {c++} END{print c+0}')"
  service_count="$(kubectl -n "$namespace" get svc --no-headers 2>/dev/null | awk -v n="$name" '$1 ~ n {c++} END{print c+0}')"
  [ "$pod_count" -gt 0 ] || fail "$name has no Pod"
  [ "$pvc_count" -gt 0 ] || fail "$name has no PVC"
  [ "$service_count" -gt 0 ] || fail "$name has no Service"
  kubectl -n "$namespace" get singleinstancedatabase "$name" -o yaml > "$results_dir/${name}.yaml"
  kubectl -n "$namespace" get pods -o wide > "$results_dir/pods.txt" || true
  kubectl -n "$namespace" get pvc -o wide > "$results_dir/pvc.txt" || true
  pass "$name role=$expected_role with Pod and PVC"
}
render_sidb(){
  local name="$1" sid="$2" create_as="$3" source="$4" prereqs="$5" endpoint="$6"
  cat <<YAML
apiVersion: database.oracle.com/v4
kind: SingleInstanceDatabase
metadata:
  name: ${name}
  namespace: ${namespace}
spec:
  sid: ${sid}
  createAs: ${create_as}
${source}
  security:
    secrets:
      admin:
        secretName: ${SIDB_ADMIN_SECRET}
        secretKey: ${SIDB_ADMIN_SECRET_KEY}
        keepSecret: true
  edition: enterprise
  charset: AL32UTF8
  pdbName: ${SIDB_PRIMARY_PDB}
  archiveLog: true
  image:
    pullFrom: ${SIDB_IMAGE}
$(if [ -n "$SIDB_PULL_SECRET" ]; then echo "    pullSecrets: ${SIDB_PULL_SECRET}"; fi)
  persistence:
    oradata:
      size: 100Gi
      storageClass: ${SIDB_STORAGE_CLASS}
      accessMode: ReadWriteOnce
${endpoint}
${prereqs}
  replicas: 1
YAML
}
provision_primary(){
  local endpoint=""
  if [ "${1:-false}" = true ]; then
    printf -v endpoint "  services:\n    endpoints:\n      - name: nodeport\n        type: NodePort\n        tcp:\n          enabled: true"
  fi
  if ! kubectl -n "$namespace" get secret "$SIDB_ADMIN_SECRET" >/dev/null 2>&1; then
    kubectl -n "$namespace" create secret generic "$SIDB_ADMIN_SECRET" --from-literal="${SIDB_ADMIN_SECRET_KEY}=${SIDB_ADMIN_PASSWORD}"
  fi
  render_sidb "$SIDB_PRIMARY_NAME" "$SIDB_PRIMARY_SID" primary "" "" "$endpoint" > "$results_dir/primary.yaml"
  kubectl apply -f "$results_dir/primary.yaml"
  wait_sidb "$SIDB_PRIMARY_NAME"; assert_sidb "$SIDB_PRIMARY_NAME" PRIMARY
}
validate_webhook(){
  if kubectl -n "$namespace" apply --dry-run=server -f - > /dev/null 2> "$results_dir/invalid-validation.err" <<YAML
apiVersion: database.oracle.com/v4
kind: SingleInstanceDatabase
metadata:
  name: sidb-e2e-invalid
spec:
  sid: BAD_SID_TOO_LONG
  createAs: primary
YAML
  then cat "$results_dir/invalid-validation.err" >&2; fail "invalid SIDB was accepted"; fi
  pass "invalid SIDB was rejected by webhook"
}
run_clone(){
  printf -v source "  primarySource:\n    databaseRef: %s" "$SIDB_PRIMARY_NAME"
  render_sidb "$SIDB_CLONE_NAME" "$SIDB_CLONE_SID" clone "$source" "" "" > "$results_dir/clone.yaml"
  kubectl apply -f "$results_dir/clone.yaml"
  wait_sidb "$SIDB_CLONE_NAME"; assert_sidb "$SIDB_CLONE_NAME" PRIMARY
}
run_dataguard(){
  local external source endpoint prereqs broker_yaml elapsed dg_status primary standby
  if [ -n "${SIDB_PRIMARY_EXTERNAL_CONNECT_STRING:-}" ]; then
    external="$SIDB_PRIMARY_EXTERNAL_CONNECT_STRING"
  else
    external="$(kubectl -n "$namespace" get singleinstancedatabase "$SIDB_PRIMARY_NAME" -o jsonpath='{.status.externalConnectString}' 2>/dev/null || true)"
    if [ -z "$external" ]; then
      external="$(kubectl -n "$namespace" get singleinstancedatabase "$SIDB_PRIMARY_NAME" -o jsonpath='{.status.connectString}' 2>/dev/null || true)"
    fi
  fi
  [ -n "$external" ] || fail "primary connect string is empty; set SIDB_PRIMARY_EXTERNAL_CONNECT_STRING or expose a primary endpoint"
  printf -v source "  primarySource:\n    connectString: %s" "$external"
  printf -v endpoint "  services:\n    endpoints:\n      - name: nodeport\n        type: NodePort\n        tcp:\n          enabled: true"
  printf -v prereqs "  dataguard:\n    prereqs:\n      enabled: true"
  render_sidb "$SIDB_STANDBY_NAME" "$SIDB_STANDBY_SID" standby "$source" "$prereqs" "$endpoint" > "$results_dir/standby.yaml"
  kubectl apply -f "$results_dir/standby.yaml"
  wait_sidb "$SIDB_STANDBY_NAME"; assert_sidb "$SIDB_STANDBY_NAME" PHYSICAL_STANDBY
  broker_yaml="$results_dir/${SIDB_BROKER_NAME}.yaml"
  kubectl -n "$namespace" get singleinstancedatabase "$SIDB_STANDBY_NAME" -o jsonpath='{.status.dataguard.renderedBrokerSpec}' > "$broker_yaml"
  [ -s "$broker_yaml" ] || fail "renderedBrokerSpec is empty"
  kubectl apply -f "$broker_yaml"
  elapsed=0
  while [ "$elapsed" -lt "$SIDB_TIMEOUT_SECONDS" ]; do
    dg_status="$(kubectl -n "$namespace" get dataguardbroker "$SIDB_BROKER_NAME" -o jsonpath='{.status.status}' 2>/dev/null || true)"
    primary="$(kubectl -n "$namespace" get dataguardbroker "$SIDB_BROKER_NAME" -o jsonpath='{.status.primaryDatabase}' 2>/dev/null || true)"
    standby="$(kubectl -n "$namespace" get dataguardbroker "$SIDB_BROKER_NAME" -o jsonpath='{.status.standbyDatabases}' 2>/dev/null || true)"
    case "$dg_status" in Healthy|HEALTHY|Ready|READY|Success|SUCCESS) [ -n "$primary" ] || fail "Broker primaryDatabase is empty"; [ -n "$standby" ] || fail "Broker standbyDatabases is empty"; pass "Data Guard ready: primary=$primary standby=$standby"; break;; Failed|FAILED|Error|ERROR) fail "Broker status=$dg_status";; esac
    sleep "$SIDB_POLL_INTERVAL"; elapsed=$((elapsed + SIDB_POLL_INTERVAL))
  done
  [ "$elapsed" -lt "$SIDB_TIMEOUT_SECONDS" ] || fail "timed out waiting for Data Guard Broker"
  kubectl -n "$namespace" get dataguardbroker "$SIDB_BROKER_NAME" -o yaml > "$results_dir/${SIDB_BROKER_NAME}.yaml"
}
run_provision=false
run_clone_test=false
run_dataguard_test=false
case "$tests" in
  all) run_provision=true; run_clone_test=true; run_dataguard_test=true;;
  *)
    IFS=',' read -r -a selected_tests <<< "$tests"
    for test_name in "${selected_tests[@]}"; do
      case "$test_name" in
        provision) run_provision=true;;
        clone) run_clone_test=true;;
        dataguard) run_dataguard_test=true;;
        *) fail "unknown test '$test_name'; choose provision, clone, dataguard, or all";;
      esac
    done
    ;;
esac

# One primary is shared by all selected tests in this invocation.
if $run_provision || $run_clone_test || $run_dataguard_test; then
  provision_primary "$run_dataguard_test"
fi
$run_provision && validate_webhook
$run_clone_test && run_clone
$run_dataguard_test && run_dataguard
echo "SIDB e2e tests '$tests' succeeded in namespace '$namespace'"
