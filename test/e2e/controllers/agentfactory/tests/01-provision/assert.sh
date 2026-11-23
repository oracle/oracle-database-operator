#!/usr/bin/env bash
set -Eeuo pipefail

script_dir="$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
# Test01-specific assertions for the Wallet/TCPS database path after SIDB
# enable-tcps patches the primary and prepare-database.sh installs the wallet.
# shellcheck disable=SC1091
source "$script_dir/../common/lib.sh"

agentfactory_case_init
test_id="$AGENTFACTORY_CASE_ID"
SIDB_CASE_NAMESPACE="${SIDB_CASE_NAMESPACE:-${SIDB_E2E_NAMESPACE:-sidb-ns}}"
SIDB_TEST_NAME="${SIDB_TEST_NAME:-sidb-e2e-primary}"
SIDB_TEST_PDB="${SIDB_TEST_PDB:-ORCLPDB1}"
export SIDB_CASE_NAMESPACE SIDB_TEST_NAME SIDB_TEST_PDB

mkdir -p "$AGENTFACTORY_CASE_RESULTS_DIR"
trap 'agentfactory_case_collect_diagnostics assertion-failure || true' ERR

require_nonempty() {
  local label="$1" value="$2"
  [ -n "$value" ] || {
    echo "Agent Factory $test_id assertion failed: $label is empty" >&2
    return 1
  }
}

require_number_at_least_one() {
  local label="$1" value="$2"
  [[ "$value" =~ ^[0-9]+$ ]] && [ "$value" -ge 1 ] || {
    echo "Agent Factory $test_id assertion failed: $label is '$value'" >&2
    return 1
  }
}

setup_manifest_dir="$AGENTFACTORY_CASE_RESULTS_DIR/setup-manifests"
for setup_file in \
  00-namespace.yaml \
  00-namespace-observed.yaml \
  01-pvc.yaml \
  02-runtime-secret.redacted.yaml \
  03-bootstrap-secret.redacted.yaml \
  04-agentfactory.yaml; do
  [ -s "$setup_manifest_dir/$setup_file" ] || {
    echo "Agent Factory $test_id assertion failed: missing setup manifest $setup_manifest_dir/$setup_file" >&2
    exit 1
  }
done
[ -s "$AGENTFACTORY_CASE_RESULTS_DIR/setup-order.txt" ] || {
  echo "Agent Factory $test_id assertion failed: setup-order.txt is missing" >&2
  exit 1
}

phase="$(agentfactory_case_kubectl -n "$AGENTFACTORY_CASE_NAMESPACE" \
  get agentfactory "$AGENTFACTORY_NAME" -o jsonpath='{.status.phase}' 2>/dev/null || true)"
ready_replicas="$(agentfactory_case_kubectl -n "$AGENTFACTORY_CASE_NAMESPACE" \
  get agentfactory "$AGENTFACTORY_NAME" -o jsonpath='{.status.readyReplicas}' 2>/dev/null || true)"
database_phase="$(agentfactory_case_kubectl -n "$AGENTFACTORY_CASE_NAMESPACE" \
  get agentfactory "$AGENTFACTORY_NAME" -o jsonpath='{.status.database.phase}' 2>/dev/null || true)"
local_service="$(agentfactory_case_kubectl -n "$AGENTFACTORY_CASE_NAMESPACE" \
  get agentfactory "$AGENTFACTORY_NAME" -o jsonpath='{.status.localService}' 2>/dev/null || true)"

case "$phase" in
  Ready|READY) ;;
  *)
    echo "Agent Factory $test_id assertion failed: phase is '$phase'" >&2
    exit 1
    ;;
esac
[ "$database_phase" = "Ready" ] || {
  echo "Agent Factory $test_id assertion failed: database phase is '$database_phase'" >&2
  exit 1
}

bootstrap_job_succeeded="$(agentfactory_case_kubectl -n "$AGENTFACTORY_CASE_NAMESPACE" \
  get job "${AGENTFACTORY_NAME}-db-setup" -o jsonpath='{.status.succeeded}' 2>/dev/null || true)"
require_number_at_least_one "database setup Job succeeded" "${bootstrap_job_succeeded:-0}"
require_number_at_least_one readyReplicas "$ready_replicas"
require_nonempty status.localService "$local_service"

deployment_ready="$(agentfactory_case_kubectl -n "$AGENTFACTORY_CASE_NAMESPACE" \
  get deployment "$AGENTFACTORY_NAME" -o jsonpath='{.status.availableReplicas}' 2>/dev/null || true)"
require_number_at_least_one deployment.availableReplicas "${deployment_ready:-0}"

# Job Pods share the AgentFactory instance label, but a completed one-shot
# setup Pod never becomes Ready. Limit this check to the long-running
# Deployment Pods.
mapfile -t agentfactory_pods < <(
  agentfactory_case_kubectl -n "$AGENTFACTORY_CASE_NAMESPACE" get pods \
    -l "app.kubernetes.io/instance=$AGENTFACTORY_NAME,!job-name" \
    -o jsonpath='{range .items[*]}{.metadata.name}{"\n"}{end}' 2>/dev/null || true
)
[ "${#agentfactory_pods[@]}" -gt 0 ] || {
  echo "Agent Factory $test_id assertion failed: no Agent Factory Pods found" >&2
  exit 1
}
for pod in "${agentfactory_pods[@]}"; do
  agentfactory_case_kubectl -n "$AGENTFACTORY_CASE_NAMESPACE" wait \
    --for=condition=Ready "pod/$pod" --timeout=60s >/dev/null
done

service_type="$(agentfactory_case_kubectl -n "$AGENTFACTORY_CASE_NAMESPACE" \
  get svc "$local_service" -o jsonpath='{.spec.type}' 2>/dev/null || true)"
[ "$service_type" = "ClusterIP" ] || {
  echo "Agent Factory $test_id assertion failed: service $local_service type is '$service_type'" >&2
  exit 1
}
service_endpoints="$(agentfactory_case_kubectl -n "$AGENTFACTORY_CASE_NAMESPACE" \
  get endpoints "$local_service" -o jsonpath='{range .subsets[*].addresses[*]}{.ip}{"\n"}{end}' \
  2>/dev/null || true)"
require_nonempty "service endpoints" "$service_endpoints"

public_service="$(agentfactory_case_kubectl -n "$AGENTFACTORY_CASE_NAMESPACE" \
  get agentfactory "$AGENTFACTORY_NAME" -o jsonpath='{.status.publicLoadBalancerService}' \
  2>/dev/null || true)"
public_service="${public_service:-${AGENTFACTORY_NAME}-public-lb}"
public_service_type=""
public_external_policy=""
public_lb_ip=""
public_lb_hostname=""
public_port=""
public_target_port=""
public_lb_elapsed=0
while [ "$public_lb_elapsed" -lt "$AGENTFACTORY_PUBLIC_LB_TIMEOUT_SECONDS" ]; do
  public_service_type="$(agentfactory_case_kubectl -n "$AGENTFACTORY_CASE_NAMESPACE" \
    get svc "$public_service" -o jsonpath='{.spec.type}' 2>/dev/null || true)"
  public_external_policy="$(agentfactory_case_kubectl -n "$AGENTFACTORY_CASE_NAMESPACE" \
    get svc "$public_service" -o jsonpath='{.spec.externalTrafficPolicy}' 2>/dev/null || true)"
  public_lb_ip="$(agentfactory_case_kubectl -n "$AGENTFACTORY_CASE_NAMESPACE" \
    get svc "$public_service" -o jsonpath='{.status.loadBalancer.ingress[0].ip}' 2>/dev/null || true)"
  public_lb_hostname="$(agentfactory_case_kubectl -n "$AGENTFACTORY_CASE_NAMESPACE" \
    get svc "$public_service" -o jsonpath='{.status.loadBalancer.ingress[0].hostname}' 2>/dev/null || true)"
  public_port="$(agentfactory_case_kubectl -n "$AGENTFACTORY_CASE_NAMESPACE" \
    get svc "$public_service" -o jsonpath='{.spec.ports[0].port}' 2>/dev/null || true)"
  public_target_port="$(agentfactory_case_kubectl -n "$AGENTFACTORY_CASE_NAMESPACE" \
    get svc "$public_service" -o jsonpath='{.spec.ports[0].targetPort}' 2>/dev/null || true)"
  if [ "$public_service_type" = "LoadBalancer" ] &&
     { [ -n "$public_lb_ip" ] || [ -n "$public_lb_hostname" ]; }; then
    break
  fi
  sleep "$AGENTFACTORY_POLL_INTERVAL"
  public_lb_elapsed=$((public_lb_elapsed + AGENTFACTORY_POLL_INTERVAL))
done
[ "$public_service_type" = "LoadBalancer" ] || {
  echo "Agent Factory $test_id assertion failed: public Service $public_service type is '$public_service_type'" >&2
  exit 1
}
[ "$public_external_policy" = "$AGENTFACTORY_PUBLIC_LB_EXTERNAL_TRAFFIC_POLICY" ] || {
  echo "Agent Factory $test_id assertion failed: public Service externalTrafficPolicy is '$public_external_policy'" >&2
  exit 1
}
[ "$public_port" = "443" ] || {
  echo "Agent Factory $test_id assertion failed: public Service port is '$public_port', expected 443" >&2
  exit 1
}
[ "$public_target_port" = "8080" ] || {
  echo "Agent Factory $test_id assertion failed: public Service targetPort is '$public_target_port', expected 8080" >&2
  exit 1
}
public_endpoint="${public_lb_ip:-$public_lb_hostname}"
require_nonempty "public LoadBalancer endpoint" "$public_endpoint"

pvc_phase="$(agentfactory_case_kubectl -n "$AGENTFACTORY_CASE_NAMESPACE" \
  get pvc "$AGENTFACTORY_PVC_NAME" -o jsonpath='{.status.phase}' 2>/dev/null || true)"
[ "$pvc_phase" = "Bound" ] || {
  echo "Agent Factory $test_id assertion failed: PVC phase is '$pvc_phase'" >&2
  exit 1
}

for secret_key in "$AGENTFACTORY_UI_ADMIN_PASSWORD_KEY" "$AGENTFACTORY_DB_PASSWORD_KEY"; do
  secret_value="$(agentfactory_case_kubectl -n "$AGENTFACTORY_CASE_NAMESPACE" \
    get secret "$AGENTFACTORY_RUNTIME_SECRET" -o "jsonpath={.data.$secret_key}" \
    2>/dev/null || true)"
  require_nonempty "runtime Secret key $secret_key" "$secret_value"
done

agentfactory_image="$(agentfactory_case_kubectl -n "$AGENTFACTORY_CASE_NAMESPACE" \
  get agentfactory "$AGENTFACTORY_NAME" -o jsonpath='{.spec.runtime.image.name}' 2>/dev/null || true)"
database_user="$(agentfactory_case_kubectl -n "$AGENTFACTORY_CASE_NAMESPACE" \
  get agentfactory "$AGENTFACTORY_NAME" -o jsonpath='{.spec.database.username}' 2>/dev/null || true)"
require_nonempty "spec.runtime.image.name" "$agentfactory_image"
require_nonempty "spec.database.username" "$database_user"
connection_type="$(agentfactory_case_kubectl -n "$AGENTFACTORY_CASE_NAMESPACE" \
  get agentfactory "$AGENTFACTORY_NAME" -o jsonpath='{.spec.database.connectionType}' 2>/dev/null || true)"
database_host=""
database_service=""
wallet_alias=""
wallet_has_oci_certificates=""
wallet_zip_key=""
[ "${connection_type^^}" = "WALLET" ] || {
  echo "Agent Factory $test_id assertion failed: test01 connection type is '$connection_type', expected Wallet" >&2
  exit 1
}
wallet_alias="$(agentfactory_case_kubectl -n "$AGENTFACTORY_CASE_NAMESPACE" \
  get agentfactory "$AGENTFACTORY_NAME" -o jsonpath='{.spec.database.wallet.selectedTnsAlias}' 2>/dev/null || true)"
wallet_has_oci_certificates="$(agentfactory_case_kubectl -n "$AGENTFACTORY_CASE_NAMESPACE" \
  get agentfactory "$AGENTFACTORY_NAME" -o jsonpath='{.spec.database.wallet.hasOciCertificates}' 2>/dev/null || true)"
wallet_zip_key="$(agentfactory_case_kubectl -n "$AGENTFACTORY_CASE_NAMESPACE" \
  get agentfactory "$AGENTFACTORY_NAME" -o jsonpath='{.spec.security.secret.walletZipKey}' 2>/dev/null || true)"
wallet_zip_key="${wallet_zip_key:-${AGENTFACTORY_WALLET_ZIP_KEY:-db_wallet.zip}}"
require_nonempty "spec.database.wallet.selectedTnsAlias" "$wallet_alias"
[ "$wallet_has_oci_certificates" = "false" ] || {
  echo "Agent Factory $test_id assertion failed: wallet.hasOciCertificates is '$wallet_has_oci_certificates', expected false for the SIDB operator wallet" >&2
  exit 1
}
wallet_zip_key_escaped="${wallet_zip_key//./\\.}"
wallet_secret_value="$(agentfactory_case_kubectl -n "$AGENTFACTORY_CASE_NAMESPACE" \
  get secret "$AGENTFACTORY_RUNTIME_SECRET" -o "jsonpath={.data.${wallet_zip_key_escaped}}" \
  2>/dev/null || true)"
require_nonempty "runtime Secret key $wallet_zip_key" "$wallet_secret_value"
bootstrap_cleanup_policy="$(agentfactory_case_kubectl -n "$AGENTFACTORY_CASE_NAMESPACE" \
  get agentfactory "$AGENTFACTORY_NAME" \
  -o jsonpath='{.spec.database.bootstrap.cleanupPolicy}' 2>/dev/null || true)"
if [ "${bootstrap_cleanup_policy:-Retain}" = "DeleteOnSuccess" ] &&
   agentfactory_case_kubectl -n "$AGENTFACTORY_CASE_NAMESPACE" get secret "$AGENTFACTORY_BOOTSTRAP_SECRET" >/dev/null 2>&1; then
  echo "Agent Factory $test_id assertion failed: temporary bootstrap Secret still exists after successful Wallet setup" >&2
  exit 1
fi

sidb_status=not-checked
sidb_role=not-checked
sidb_pdb=not-checked
sidb_connect_string=not-checked
sidb_tcps_enabled=not-checked
sidb_tcps_connect_string=not-checked
sidb_pods=()
if [ "${AGENTFACTORY_REQUIRES_SIDB:-true}" = "true" ]; then
  sidb_status="$(agentfactory_case_kubectl -n "$SIDB_CASE_NAMESPACE" \
    get singleinstancedatabase "$SIDB_TEST_NAME" -o jsonpath='{.status.status}' 2>/dev/null || true)"
  sidb_role="$(agentfactory_case_kubectl -n "$SIDB_CASE_NAMESPACE" \
    get singleinstancedatabase "$SIDB_TEST_NAME" -o jsonpath='{.status.role}' 2>/dev/null || true)"
  sidb_pdb="$(agentfactory_case_kubectl -n "$SIDB_CASE_NAMESPACE" \
    get singleinstancedatabase "$SIDB_TEST_NAME" -o jsonpath='{.spec.pdbName}' 2>/dev/null || true)"
  sidb_connect_string="$(agentfactory_case_kubectl -n "$SIDB_CASE_NAMESPACE" \
    get singleinstancedatabase "$SIDB_TEST_NAME" -o jsonpath='{.status.connectString}' 2>/dev/null || true)"
  sidb_tcps_enabled="$(agentfactory_case_kubectl -n "$SIDB_CASE_NAMESPACE" \
    get singleinstancedatabase "$SIDB_TEST_NAME" -o jsonpath='{.status.isTcpsEnabled}' 2>/dev/null || true)"
  sidb_tcps_connect_string="$(agentfactory_case_kubectl -n "$SIDB_CASE_NAMESPACE" \
    get singleinstancedatabase "$SIDB_TEST_NAME" -o jsonpath='{.status.tcpsConnectString}' 2>/dev/null || true)"
  case "$sidb_status" in
    Ready|READY|Healthy|HEALTHY|Available|AVAILABLE) ;;
    *)
      echo "Agent Factory $test_id assertion failed: SIDB status is '$sidb_status'" >&2
      exit 1
      ;;
  esac
  [ "$sidb_role" = "PRIMARY" ] || {
    echo "Agent Factory $test_id assertion failed: SIDB role is '$sidb_role'" >&2
    exit 1
  }
  [ "$sidb_pdb" = "$SIDB_TEST_PDB" ] || {
    echo "Agent Factory $test_id assertion failed: SIDB PDB is '$sidb_pdb', expected '$SIDB_TEST_PDB'" >&2
    exit 1
  }
  require_nonempty "SIDB status.connectString" "$sidb_connect_string"
  [ "$sidb_tcps_enabled" = "true" ] || {
    echo "Agent Factory $test_id assertion failed: SIDB isTcpsEnabled is '$sidb_tcps_enabled'" >&2
    exit 1
  }
  require_nonempty "SIDB status.tcpsConnectString" "$sidb_tcps_connect_string"
  [[ "$sidb_tcps_connect_string" == *":2484"* || "$sidb_tcps_connect_string" == *"2484/"* ]] || {
    echo "Agent Factory $test_id assertion failed: expected TCPS port 2484 in '$sidb_tcps_connect_string'" >&2
    exit 1
  }

  mapfile -t sidb_pods < <(
    agentfactory_case_kubectl -n "$SIDB_CASE_NAMESPACE" get pods --no-headers \
      -o custom-columns=NAME:.metadata.name 2>/dev/null |
      awk -v needle="$SIDB_TEST_NAME" 'index($1, needle) > 0 {print $1}'
  )
  [ "${#sidb_pods[@]}" -gt 0 ] || {
    echo "Agent Factory $test_id assertion failed: no SIDB Pods found" >&2
    exit 1
  }
  for pod in "${sidb_pods[@]}"; do
    agentfactory_case_kubectl -n "$SIDB_CASE_NAMESPACE" wait \
      --for=condition=Ready "pod/$pod" --timeout=60s >/dev/null
  done
else
  echo "Agent Factory $test_id standalone mode: skipping SIDB-in-cluster assertions"
fi

operator_namespace="${OPERATOR_NAMESPACE:-oracle-database-operator-system}"
operator_deployment="${OPERATOR_DEPLOYMENT:-oracle-database-operator-controller-manager}"
operator_available="$(agentfactory_case_kubectl -n "$operator_namespace" \
  get deployment "$operator_deployment" -o jsonpath='{.status.availableReplicas}' 2>/dev/null || true)"
require_number_at_least_one "operator availableReplicas" "${operator_available:-0}"

agentfactory_case_collect_diagnostics assertion-passed
cat > "$AGENTFACTORY_CASE_RESULTS_DIR/assertion-summary.txt" <<EOF
test=agentfactory/$test_id
result=PASS
namespace=$AGENTFACTORY_CASE_NAMESPACE
name=$AGENTFACTORY_NAME
agentfactory_phase=$phase
agentfactory_ready_replicas=$ready_replicas
agentfactory_database_phase=$database_phase
agentfactory_database_connection_type=$connection_type
agentfactory_local_service=$local_service
agentfactory_service_type=$service_type
agentfactory_pod_count=${#agentfactory_pods[@]}
public_load_balancer_service=$public_service
public_load_balancer_endpoint=$public_endpoint
public_load_balancer_port=$public_port
public_load_balancer_target_port=$public_target_port
agentfactory_pvc=$AGENTFACTORY_PVC_NAME
agentfactory_pvc_phase=$pvc_phase
agentfactory_image=$agentfactory_image
database_user=$database_user
database_host=$database_host
database_service=$database_service
database_setup_job_succeeded=$bootstrap_job_succeeded
wallet_alias=$wallet_alias
wallet_has_oci_certificates=$wallet_has_oci_certificates
wallet_zip_key=$wallet_zip_key
runtime_secret=$AGENTFACTORY_RUNTIME_SECRET
bootstrap_secret=$AGENTFACTORY_BOOTSTRAP_SECRET
sidb_namespace=$SIDB_CASE_NAMESPACE
sidb_name=$SIDB_TEST_NAME
sidb_status=$sidb_status
sidb_role=$sidb_role
sidb_pdb=$sidb_pdb
sidb_tcps_enabled=$sidb_tcps_enabled
sidb_tcps_connect_string=$sidb_tcps_connect_string
sidb_pod_count=${#sidb_pods[@]}
operator_namespace=$operator_namespace
operator_deployment=$operator_deployment
operator_available_replicas=$operator_available
diagnostics=$AGENTFACTORY_CASE_RESULTS_DIR/diagnostics
rendered_manifest=$AGENTFACTORY_RENDERED_MANIFEST
setup_manifests=$setup_manifest_dir
setup_order=$AGENTFACTORY_CASE_RESULTS_DIR/setup-order.txt
EOF

trap - ERR
if [ "${AGENTFACTORY_REQUIRES_SIDB:-true}" = "true" ]; then
  echo "Agent Factory $test_id assertions passed: Wallet/TCPS 2484 against SIDB PRIMARY/$SIDB_TEST_PDB, and operator are ready"
else
  echo "Agent Factory $test_id assertions passed: standalone Agent Factory and operator are ready"
fi
