#!/usr/bin/env bash
set -Eeuo pipefail

script_dir="$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
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

require_ipv4() {
  local label="$1" value="$2" octet
  local -a octets
  [[ "$value" =~ ^[0-9]+(\.[0-9]+){3}$ ]] || {
    echo "Agent Factory $test_id assertion failed: $label '$value' is not an IPv4 address" >&2
    return 1
  }
  IFS=. read -r -a octets <<< "$value"
  for octet in "${octets[@]}"; do
    if (( 10#$octet > 255 )); then
      echo "Agent Factory $test_id assertion failed: $label '$value' is not an IPv4 address" >&2
      return 1
    fi
  done
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

expected_ip="${AGENTFACTORY_PUBLIC_LB_IP:-}"
require_nonempty "AGENTFACTORY_PUBLIC_LB_IP" "$expected_ip"
require_ipv4 "AGENTFACTORY_PUBLIC_LB_IP" "$expected_ip"

phase="$(agentfactory_case_kubectl -n "$AGENTFACTORY_CASE_NAMESPACE" \
  get agentfactory "$AGENTFACTORY_NAME" -o jsonpath='{.status.phase}' 2>/dev/null || true)"
ready_replicas="$(agentfactory_case_kubectl -n "$AGENTFACTORY_CASE_NAMESPACE" \
  get agentfactory "$AGENTFACTORY_NAME" -o jsonpath='{.status.readyReplicas}' 2>/dev/null || true)"
database_phase="$(agentfactory_case_kubectl -n "$AGENTFACTORY_CASE_NAMESPACE" \
  get agentfactory "$AGENTFACTORY_NAME" -o jsonpath='{.status.database.phase}' 2>/dev/null || true)"
local_service="$(agentfactory_case_kubectl -n "$AGENTFACTORY_CASE_NAMESPACE" \
  get agentfactory "$AGENTFACTORY_NAME" -o jsonpath='{.status.localService}' 2>/dev/null || true)"

[ "$phase" = "Ready" ] || {
  echo "Agent Factory $test_id assertion failed: phase is '$phase'" >&2
  exit 1
}
connection_type="$(agentfactory_case_kubectl -n "$AGENTFACTORY_CASE_NAMESPACE" \
  get agentfactory "$AGENTFACTORY_NAME" -o jsonpath='{.spec.database.connectionType}' 2>/dev/null || true)"
database_protocol="$(agentfactory_case_kubectl -n "$AGENTFACTORY_CASE_NAMESPACE" \
  get agentfactory "$AGENTFACTORY_NAME" -o jsonpath='{.spec.database.details.protocol}' 2>/dev/null || true)"
database_port="$(agentfactory_case_kubectl -n "$AGENTFACTORY_CASE_NAMESPACE" \
  get agentfactory "$AGENTFACTORY_NAME" -o jsonpath='{.spec.database.details.port}' 2>/dev/null || true)"
[ "${connection_type^^}" = "DETAILS" ] || {
  echo "Agent Factory $test_id assertion failed: CR connection type is '$connection_type'" >&2
  exit 1
}
[ "${database_protocol^^}" = "TCP" ] || {
  echo "Agent Factory $test_id assertion failed: CR database protocol is '$database_protocol', expected TCP" >&2
  exit 1
}
[ "$database_port" = "1521" ] || {
  echo "Agent Factory $test_id assertion failed: CR database port is '$database_port', expected 1521" >&2
  exit 1
}
[ "$database_phase" = "Ready" ] || {
  echo "Agent Factory $test_id assertion failed: database phase is '$database_phase'" >&2
  exit 1
}

bootstrap_job_succeeded="$(agentfactory_case_kubectl -n "$AGENTFACTORY_CASE_NAMESPACE" \
  get job "${AGENTFACTORY_NAME}-db-setup" -o jsonpath='{.status.succeeded}' 2>/dev/null || true)"
require_number_at_least_one "database setup Job succeeded" "${bootstrap_job_succeeded:-0}"
setup_job_args="$(agentfactory_case_kubectl -n "$AGENTFACTORY_CASE_NAMESPACE" \
  get job "${AGENTFACTORY_NAME}-db-setup" \
  -o jsonpath='{.spec.template.spec.containers[0].args[*]}' 2>/dev/null || true)"
case " $setup_job_args " in
  *" --wallet-file "*)
    echo "Agent Factory $test_id assertion failed: database setup Job unexpectedly uses a wallet" >&2
    exit 1
    ;;
esac
require_number_at_least_one readyReplicas "$ready_replicas"
require_nonempty status.localService "$local_service"

deployment_ready="$(agentfactory_case_kubectl -n "$AGENTFACTORY_CASE_NAMESPACE" \
  get deployment "$AGENTFACTORY_NAME" -o jsonpath='{.status.availableReplicas}' 2>/dev/null || true)"
require_number_at_least_one deployment.availableReplicas "${deployment_ready:-0}"

mapfile -t agentfactory_pods < <(
  agentfactory_case_kubectl -n "$AGENTFACTORY_CASE_NAMESPACE" get pods \
    -l "app.kubernetes.io/instance=$AGENTFACTORY_NAME,!job-name" \
    -o jsonpath='{range .items[*]}{.metadata.name}{"\n"}{end}' 2>/dev/null || true
)
[ "${#agentfactory_pods[@]}" -gt 0 ] || {
  echo "Agent Factory $test_id assertion failed: no AgentFactory Pods found" >&2
  exit 1
}
for pod in "${agentfactory_pods[@]}"; do
  agentfactory_case_kubectl -n "$AGENTFACTORY_CASE_NAMESPACE" wait \
    --for=condition=Ready "pod/$pod" --timeout=60s >/dev/null
done

service_type="$(agentfactory_case_kubectl -n "$AGENTFACTORY_CASE_NAMESPACE" \
  get svc "$local_service" -o jsonpath='{.spec.type}' 2>/dev/null || true)"
[ "$service_type" = "ClusterIP" ] || {
  echo "Agent Factory $test_id assertion failed: local Service type is '$service_type'" >&2
  exit 1
}
service_endpoints="$(agentfactory_case_kubectl -n "$AGENTFACTORY_CASE_NAMESPACE" \
  get endpoints "$local_service" -o jsonpath='{range .subsets[*].addresses[*]}{.ip}{"\n"}{end}' \
  2>/dev/null || true)"
require_nonempty "local Service endpoints" "$service_endpoints"

cert_ip="$(agentfactory_case_kubectl -n "$AGENTFACTORY_CASE_NAMESPACE" \
  get agentfactory "$AGENTFACTORY_NAME" -o jsonpath='{.spec.configuration.certIpAddress}' 2>/dev/null || true)"
[ "$cert_ip" = "$expected_ip" ] || {
  echo "Agent Factory $test_id assertion failed: certIpAddress is '$cert_ip', expected '$expected_ip'" >&2
  exit 1
}
if [ -n "${AGENTFACTORY_CERT_FQDN:-}" ]; then
  cert_fqdn="$(agentfactory_case_kubectl -n "$AGENTFACTORY_CASE_NAMESPACE" \
    get agentfactory "$AGENTFACTORY_NAME" -o jsonpath='{.spec.configuration.certFqdn}' 2>/dev/null || true)"
  [ "$cert_fqdn" = "$AGENTFACTORY_CERT_FQDN" ] || {
    echo "Agent Factory $test_id assertion failed: certFqdn is '$cert_fqdn', expected '$AGENTFACTORY_CERT_FQDN'" >&2
    exit 1
  }
fi

public_service="$(agentfactory_case_kubectl -n "$AGENTFACTORY_CASE_NAMESPACE" \
  get agentfactory "$AGENTFACTORY_NAME" -o jsonpath='{.status.publicLoadBalancerService}' \
  2>/dev/null || true)"
public_service="${public_service:-${AGENTFACTORY_NAME}-public-lb}"
public_service_type=""
public_external_policy=""
public_spec_ip=""
public_status_ip=""
public_lb_hostname=""
public_port=""
public_target_port=""
public_subnet=""
public_lb_elapsed=0
while [ "$public_lb_elapsed" -lt "$AGENTFACTORY_PUBLIC_LB_TIMEOUT_SECONDS" ]; do
  public_service_type="$(agentfactory_case_kubectl -n "$AGENTFACTORY_CASE_NAMESPACE" \
    get svc "$public_service" -o jsonpath='{.spec.type}' 2>/dev/null || true)"
  public_external_policy="$(agentfactory_case_kubectl -n "$AGENTFACTORY_CASE_NAMESPACE" \
    get svc "$public_service" -o jsonpath='{.spec.externalTrafficPolicy}' 2>/dev/null || true)"
  public_spec_ip="$(agentfactory_case_kubectl -n "$AGENTFACTORY_CASE_NAMESPACE" \
    get svc "$public_service" -o jsonpath='{.spec.loadBalancerIP}' 2>/dev/null || true)"
  public_status_ip="$(agentfactory_case_kubectl -n "$AGENTFACTORY_CASE_NAMESPACE" \
    get svc "$public_service" -o jsonpath='{.status.loadBalancer.ingress[0].ip}' 2>/dev/null || true)"
  public_lb_hostname="$(agentfactory_case_kubectl -n "$AGENTFACTORY_CASE_NAMESPACE" \
    get svc "$public_service" -o jsonpath='{.status.loadBalancer.ingress[0].hostname}' 2>/dev/null || true)"
  public_port="$(agentfactory_case_kubectl -n "$AGENTFACTORY_CASE_NAMESPACE" \
    get svc "$public_service" -o jsonpath='{.spec.ports[0].port}' 2>/dev/null || true)"
  public_target_port="$(agentfactory_case_kubectl -n "$AGENTFACTORY_CASE_NAMESPACE" \
    get svc "$public_service" -o jsonpath='{.spec.ports[0].targetPort}' 2>/dev/null || true)"
  public_subnet="$(agentfactory_case_kubectl -n "$AGENTFACTORY_CASE_NAMESPACE" \
    get svc "$public_service" \
    -o jsonpath='{.metadata.annotations.service\.beta\.kubernetes\.io/oci-load-balancer-subnet1}' \
    2>/dev/null || true)"
  if [ "$public_service_type" = "LoadBalancer" ] &&
     [ "$public_spec_ip" = "$expected_ip" ] &&
     [ "$public_status_ip" = "$expected_ip" ]; then
    break
  fi
  sleep "$AGENTFACTORY_POLL_INTERVAL"
  public_lb_elapsed=$((public_lb_elapsed + AGENTFACTORY_POLL_INTERVAL))
done

[ "$public_service_type" = "LoadBalancer" ] || {
  echo "Agent Factory $test_id assertion failed: public Service type is '$public_service_type'" >&2
  exit 1
}
[ "$public_spec_ip" = "$expected_ip" ] || {
  echo "Agent Factory $test_id assertion failed: Service loadBalancerIP is '$public_spec_ip', expected '$expected_ip'" >&2
  exit 1
}
[ "$public_status_ip" = "$expected_ip" ] || {
  echo "Agent Factory $test_id assertion failed: assigned LoadBalancer IP is '$public_status_ip', expected '$expected_ip'" >&2
  exit 1
}
[ "$public_external_policy" = "$AGENTFACTORY_PUBLIC_LB_EXTERNAL_TRAFFIC_POLICY" ] || {
  echo "Agent Factory $test_id assertion failed: public externalTrafficPolicy is '$public_external_policy'" >&2
  exit 1
}
[ "$public_port" = "443" ] || {
  echo "Agent Factory $test_id assertion failed: public Service port is '$public_port'" >&2
  exit 1
}
[ "$public_target_port" = "8080" ] || {
  echo "Agent Factory $test_id assertion failed: public Service targetPort is '$public_target_port'" >&2
  exit 1
}
if [ -n "${AGENTFACTORY_PUBLIC_LB_SUBNET:-}" ]; then
  [ "$public_subnet" = "$AGENTFACTORY_PUBLIC_LB_SUBNET" ] || {
    echo "Agent Factory $test_id assertion failed: subnet annotation is '$public_subnet'" >&2
    exit 1
  }
fi

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

database_user="$(agentfactory_case_kubectl -n "$AGENTFACTORY_CASE_NAMESPACE" \
  get agentfactory "$AGENTFACTORY_NAME" -o jsonpath='{.spec.database.username}' 2>/dev/null || true)"
database_host="$(agentfactory_case_kubectl -n "$AGENTFACTORY_CASE_NAMESPACE" \
  get agentfactory "$AGENTFACTORY_NAME" -o jsonpath='{.spec.database.details.host}' 2>/dev/null || true)"
database_service="$(agentfactory_case_kubectl -n "$AGENTFACTORY_CASE_NAMESPACE" \
  get agentfactory "$AGENTFACTORY_NAME" -o jsonpath='{.spec.database.details.serviceName}' 2>/dev/null || true)"
require_nonempty "spec.database.username" "$database_user"
require_nonempty "spec.database.details.host" "$database_host"
require_nonempty "spec.database.details.serviceName" "$database_service"

sidb_status=not-checked
sidb_role=not-checked
sidb_pdb=not-checked
sidb_connect_string=not-checked
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
fi

operator_namespace="${OPERATOR_NAMESPACE:-oracle-database-operator-system}"
operator_deployment="${OPERATOR_DEPLOYMENT:-oracle-database-operator-controller-manager}"
operator_available="$(agentfactory_case_kubectl -n "$operator_namespace" \
  get deployment "$operator_deployment" -o jsonpath='{.status.availableReplicas}' 2>/dev/null || true)"
require_number_at_least_one "operator availableReplicas" "${operator_available:-0}"

runtime_env="$(agentfactory_case_kubectl -n "$AGENTFACTORY_CASE_NAMESPACE" \
  get configmap "${AGENTFACTORY_NAME}-runtime" -o jsonpath='{.data.runtime\.env}' 2>/dev/null || true)"
printf '%s\n' "$runtime_env" | grep -Fx 'AGENT_FACTORY_DB_CONNECTION_TYPE=Details' >/dev/null || {
  echo "Agent Factory $test_id assertion failed: runtime.env does not select Details" >&2
  exit 1
}
printf '%s\n' "$runtime_env" | grep -Fx 'AGENT_FACTORY_DB_PROTOCOL=TCP' >/dev/null || {
  echo "Agent Factory $test_id assertion failed: runtime.env does not select TCP" >&2
  exit 1
}
printf '%s\n' "$runtime_env" | grep -Fx 'AGENT_FACTORY_DB_USES_WALLET=no' >/dev/null || {
  echo "Agent Factory $test_id assertion failed: runtime.env does not disable wallet mode" >&2
  exit 1
}
if printf '%s\n' "$runtime_env" | grep -Eq '^AGENT_FACTORY_DB_(SELECTED_TNS_ALIAS|WALLET_HAS_OCI_CERTIFICATES)='; then
  echo "Agent Factory $test_id assertion failed: runtime.env contains Wallet-only settings" >&2
  exit 1
fi

agentfactory_case_collect_diagnostics assertion-passed
cat > "$AGENTFACTORY_CASE_RESULTS_DIR/assertion-summary.txt" <<EOF
test=agentfactory/$test_id
result=PASS
namespace=$AGENTFACTORY_CASE_NAMESPACE
name=$AGENTFACTORY_NAME
agentfactory_phase=$phase
agentfactory_ready_replicas=$ready_replicas
agentfactory_database_phase=$database_phase
agentfactory_database_connection_type=Details
agentfactory_database_protocol=$database_protocol
agentfactory_database_port=$database_port
agentfactory_cert_ip=$cert_ip
agentfactory_local_service=$local_service
agentfactory_service_type=$service_type
public_load_balancer_service=$public_service
public_load_balancer_requested_ip=$public_spec_ip
public_load_balancer_assigned_ip=$public_status_ip
public_load_balancer_hostname=$public_lb_hostname
public_load_balancer_subnet=$public_subnet
agentfactory_pvc=$AGENTFACTORY_PVC_NAME
agentfactory_pvc_phase=$pvc_phase
database_user=$database_user
database_host=$database_host
database_service=$database_service
database_setup_job_succeeded=$bootstrap_job_succeeded
runtime_secret=$AGENTFACTORY_RUNTIME_SECRET
bootstrap_secret=$AGENTFACTORY_BOOTSTRAP_SECRET
sidb_namespace=$SIDB_CASE_NAMESPACE
sidb_name=$SIDB_TEST_NAME
sidb_status=$sidb_status
sidb_role=$sidb_role
sidb_pdb=$sidb_pdb
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
echo "Agent Factory $test_id assertions passed: Details connection, certificate IP, pre-allocated LoadBalancer, and workload are ready"
