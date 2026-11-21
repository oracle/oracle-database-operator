#!/usr/bin/env bash
set -Eeuo pipefail

script_dir="$(CDPATH= cd -- "$(dirname "$0")" && pwd)"
source "$script_dir/../common/lib.sh"
agentfactory_case_init
: "${OPERATOR_NAMESPACE:=oracle-database-operator-system}"
test_id="$AGENTFACTORY_CASE_ID"
: "${SIDB_CASE_NAMESPACE:=${SIDB_E2E_NAMESPACE:-sidb-ns}}"
: "${SIDB_TEST_NAME:=sidb-e2e-primary}"
mkdir -p "$AGENTFACTORY_CASE_RESULTS_DIR"
trap 'agentfactory_case_collect_diagnostics assertion-failure || true' ERR

require_number() {
  local label="$1" value="$2" expected="$3"
  [[ "$value" =~ ^[0-9]+$ ]] && [ "$value" -eq "$expected" ] || {
    echo "Agent Factory $test_id assertion failed: $label=$value expected=$expected" >&2
    exit 1
  }
}
require_nonempty() {
  local label="$1" value="$2"
  [ -n "$value" ] || { echo "Agent Factory $test_id assertion failed: $label is empty" >&2; exit 1; }
}

for setup_file in 00-namespace.yaml 00-namespace-observed.yaml 01-storageclass.yaml \
  02-runtime-secret.redacted.yaml 03-bootstrap-secret.redacted.yaml 04-agentfactory.yaml; do
  [ -s "$AGENTFACTORY_CASE_RESULTS_DIR/setup-manifests/$setup_file" ] || {
    echo "Agent Factory $test_id assertion failed: missing setup manifest $setup_file" >&2
    exit 1
  }
done

phase="$(agentfactory_case_kubectl -n "$AGENTFACTORY_CASE_NAMESPACE" get agentfactory "$AGENTFACTORY_NAME" -o jsonpath='{.status.phase}' 2>/dev/null || true)"
database_phase="$(agentfactory_case_kubectl -n "$AGENTFACTORY_CASE_NAMESPACE" get agentfactory "$AGENTFACTORY_NAME" -o jsonpath='{.status.database.phase}' 2>/dev/null || true)"
ready_replicas="$(agentfactory_case_kubectl -n "$AGENTFACTORY_CASE_NAMESPACE" get agentfactory "$AGENTFACTORY_NAME" -o jsonpath='{.status.readyReplicas}' 2>/dev/null || true)"
local_service="$(agentfactory_case_kubectl -n "$AGENTFACTORY_CASE_NAMESPACE" get agentfactory "$AGENTFACTORY_NAME" -o jsonpath='{.status.localService}' 2>/dev/null || true)"
[ "$phase" = Ready ] || { echo "Agent Factory $test_id assertion failed: phase=$phase" >&2; exit 1; }
[ "$database_phase" = Ready ] || { echo "Agent Factory $test_id assertion failed: database=$database_phase" >&2; exit 1; }
require_number readyReplicas "$ready_replicas" 3
require_nonempty localService "$local_service"

deployment_replicas="$(agentfactory_case_kubectl -n "$AGENTFACTORY_CASE_NAMESPACE" get deployment "$AGENTFACTORY_NAME" -o jsonpath='{.spec.replicas}' 2>/dev/null || true)"
deployment_available="$(agentfactory_case_kubectl -n "$AGENTFACTORY_CASE_NAMESPACE" get deployment "$AGENTFACTORY_NAME" -o jsonpath='{.status.availableReplicas}' 2>/dev/null || true)"
require_number deployment.replicas "$deployment_replicas" 3
require_number deployment.availableReplicas "$deployment_available" 3

pod_names="$(agentfactory_case_kubectl -n "$AGENTFACTORY_CASE_NAMESPACE" get pods -l "app.kubernetes.io/instance=$AGENTFACTORY_NAME,!job-name" -o jsonpath='{range .items[*]}{.metadata.name}{"\n"}{end}' 2>/dev/null || true)"
pod_count="$(printf '%s\n' "$pod_names" | sed '/^$/d' | wc -l | tr -d ' ')"
require_number pod_count "$pod_count" 3
while IFS= read -r pod; do
  [ -n "$pod" ] || continue
  agentfactory_case_kubectl -n "$AGENTFACTORY_CASE_NAMESPACE" wait --for=condition=Ready "pod/$pod" --timeout=120s >/dev/null
done <<< "$pod_names"

pod_nodes="$(while IFS= read -r pod; do
  [ -n "$pod" ] || continue
  agentfactory_case_kubectl -n "$AGENTFACTORY_CASE_NAMESPACE" get pod "$pod" -o jsonpath='{.spec.nodeName}{"\n"}'
done <<< "$pod_names")"
node_count="$(printf '%s\n' "$pod_nodes" | sed '/^$/d' | sort -u | wc -l | tr -d ' ')"
require_number distinct_nodes "$node_count" 3
deployment_json="$(agentfactory_case_kubectl -n "$AGENTFACTORY_CASE_NAMESPACE" get deployment "$AGENTFACTORY_NAME" -o json)"
printf '%s' "$deployment_json" | jq -e \
  --arg selector_key "$AGENTFACTORY_WORKER_NODE_SELECTOR_KEY" \
  --arg selector_value "$AGENTFACTORY_WORKER_NODE_SELECTOR_VALUE" \
  'any(.spec.template.spec.affinity.nodeAffinity.requiredDuringSchedulingIgnoredDuringExecution.nodeSelectorTerms[]?.matchExpressions[]?; .key == $selector_key and .operator == "In" and ((.values // []) | index($selector_value) != null))' >/dev/null || {
  echo "Agent Factory $test_id assertion failed: deployment does not require $AGENTFACTORY_WORKER_NODE_SELECTOR_KEY=$AGENTFACTORY_WORKER_NODE_SELECTOR_VALUE" >&2
  exit 1
}
pod_zones="$(while IFS= read -r pod; do
  [ -n "$pod" ] || continue
  node="$(agentfactory_case_kubectl -n "$AGENTFACTORY_CASE_NAMESPACE" get pod "$pod" -o jsonpath='{.spec.nodeName}')"
  zone="$(agentfactory_case_kubectl get node "$node" -o jsonpath='{.metadata.labels.topology\.kubernetes\.io/zone}' 2>/dev/null || true)"
  [ -n "$zone" ] || zone="$(agentfactory_case_kubectl get node "$node" -o jsonpath='{.metadata.labels.failure-domain\.beta\.kubernetes\.io/zone}' 2>/dev/null || true)"
  printf '%s\n' "$zone"
done <<< "$pod_names")"
service_type="$(agentfactory_case_kubectl -n "$AGENTFACTORY_CASE_NAMESPACE" get svc "$local_service" -o jsonpath='{.spec.type}' 2>/dev/null || true)"
[ "$service_type" = ClusterIP ] || { echo "Agent Factory $test_id assertion failed: service type=$service_type" >&2; exit 1; }
service_endpoints="$(agentfactory_case_kubectl -n "$AGENTFACTORY_CASE_NAMESPACE" get endpoints "$local_service" -o jsonpath='{range .subsets[*].addresses[*]}{.ip}{"\n"}{end}' 2>/dev/null || true)"
require_nonempty service_endpoints "$service_endpoints"

pvc_name="$AGENTFACTORY_NAME-$AGENTFACTORY_STORAGE_VOLUME_NAME"
pvc_phase="$(agentfactory_case_kubectl -n "$AGENTFACTORY_CASE_NAMESPACE" get pvc "$pvc_name" -o jsonpath='{.status.phase}' 2>/dev/null || true)"
pvc_class="$(agentfactory_case_kubectl -n "$AGENTFACTORY_CASE_NAMESPACE" get pvc "$pvc_name" -o jsonpath='{.spec.storageClassName}' 2>/dev/null || true)"
pvc_mode="$(agentfactory_case_kubectl -n "$AGENTFACTORY_CASE_NAMESPACE" get pvc "$pvc_name" -o jsonpath='{.spec.accessModes[0]}' 2>/dev/null || true)"
pvc_owner="$(agentfactory_case_kubectl -n "$AGENTFACTORY_CASE_NAMESPACE" get pvc "$pvc_name" -o jsonpath='{.metadata.ownerReferences[?(@.kind=="AgentFactory")].name}' 2>/dev/null || true)"
[ "$pvc_phase" = Bound ] || { echo "Agent Factory $test_id assertion failed: PVC phase=$pvc_phase" >&2; exit 1; }
[ "$pvc_class" = "$AGENTFACTORY_STORAGE_CLASS" ] || { echo "Agent Factory $test_id assertion failed: PVC class=$pvc_class" >&2; exit 1; }
[ "$pvc_mode" = ReadWriteMany ] || { echo "Agent Factory $test_id assertion failed: PVC mode=$pvc_mode" >&2; exit 1; }
[ "$pvc_owner" = "$AGENTFACTORY_NAME" ] || { echo "Agent Factory $test_id assertion failed: PVC owner=$pvc_owner" >&2; exit 1; }
provisioner="$(agentfactory_case_kubectl get storageclass "$AGENTFACTORY_STORAGE_CLASS" -o jsonpath='{.provisioner}' 2>/dev/null || true)"
[ "$provisioner" = fss.csi.oraclecloud.com ] || { echo "Agent Factory $test_id assertion failed: provisioner=$provisioner" >&2; exit 1; }

job_succeeded="$(agentfactory_case_kubectl -n "$AGENTFACTORY_CASE_NAMESPACE" get job "$AGENTFACTORY_NAME-db-setup" -o jsonpath='{.status.succeeded}' 2>/dev/null || true)"
require_number database_setup_job "${job_succeeded:-0}" 1
job_args="$(agentfactory_case_kubectl -n "$AGENTFACTORY_CASE_NAMESPACE" get job "$AGENTFACTORY_NAME-db-setup" -o jsonpath='{.spec.template.spec.containers[0].args[*]}' 2>/dev/null || true)"
case " $job_args " in *" --wallet-file "*) echo "Agent Factory $test_id assertion failed: setup Job uses wallet" >&2; exit 1 ;; esac

runtime_env="$(agentfactory_case_kubectl -n "$AGENTFACTORY_CASE_NAMESPACE" get configmap "$AGENTFACTORY_NAME-runtime" -o jsonpath='{.data.runtime\.env}' 2>/dev/null || true)"
printf '%s\n' "$runtime_env" | grep -Fx AGENT_FACTORY_DB_CONNECTION_TYPE=Details >/dev/null || { echo "Agent Factory $test_id assertion failed: runtime not Details" >&2; exit 1; }
printf '%s\n' "$runtime_env" | grep -Fx AGENT_FACTORY_DB_PROTOCOL=TCP >/dev/null || { echo "Agent Factory $test_id assertion failed: runtime not TCP" >&2; exit 1; }
printf '%s\n' "$runtime_env" | grep -Fx AGENT_FACTORY_DB_USES_WALLET=no >/dev/null || { echo "Agent Factory $test_id assertion failed: runtime uses wallet" >&2; exit 1; }
if printf '%s\n' "$runtime_env" | grep -E 'AGENT_FACTORY_DB_SELECTED_TNS_ALIAS|AGENT_FACTORY_DB_WALLET_HAS_OCI_CERTIFICATES' >/dev/null; then
  echo "Agent Factory $test_id assertion failed: runtime contains Wallet settings" >&2
  exit 1
fi

cert_fqdn="$(agentfactory_case_kubectl -n "$AGENTFACTORY_CASE_NAMESPACE" get agentfactory "$AGENTFACTORY_NAME" -o jsonpath='{.spec.configuration.certFqdn}' 2>/dev/null || true)"
cert_ip="$(agentfactory_case_kubectl -n "$AGENTFACTORY_CASE_NAMESPACE" get agentfactory "$AGENTFACTORY_NAME" -o jsonpath='{.spec.configuration.certIpAddress}' 2>/dev/null || true)"
[ "$cert_fqdn" = "$AGENTFACTORY_CERT_FQDN" ] || { echo "Agent Factory $test_id assertion failed: certFqdn=$cert_fqdn" >&2; exit 1; }
[ "$cert_ip" = "$AGENTFACTORY_CERT_IP_ADDRESS" ] || { echo "Agent Factory $test_id assertion failed: certIpAddress=$cert_ip" >&2; exit 1; }

if [ -n "${AGENTFACTORY_PUBLIC_LB_IP:-}" ]; then
  public_service="$(agentfactory_case_kubectl -n "$AGENTFACTORY_CASE_NAMESPACE" \
    get agentfactory "$AGENTFACTORY_NAME" \
    -o jsonpath='{.status.publicLoadBalancerService}' 2>/dev/null || true)"
  public_service="${public_service:-${AGENTFACTORY_NAME}-public-lb}"
  public_lb_elapsed=0
  public_service_type=""
  public_spec_ip=""
  public_status_ip=""
  while [ "$public_lb_elapsed" -lt "$AGENTFACTORY_PUBLIC_LB_TIMEOUT_SECONDS" ]; do
    public_service_type="$(agentfactory_case_kubectl -n "$AGENTFACTORY_CASE_NAMESPACE" \
      get svc "$public_service" -o jsonpath='{.spec.type}' 2>/dev/null || true)"
    public_spec_ip="$(agentfactory_case_kubectl -n "$AGENTFACTORY_CASE_NAMESPACE" \
      get svc "$public_service" -o jsonpath='{.spec.loadBalancerIP}' 2>/dev/null || true)"
    public_status_ip="$(agentfactory_case_kubectl -n "$AGENTFACTORY_CASE_NAMESPACE" \
      get svc "$public_service" \
      -o jsonpath='{.status.loadBalancer.ingress[0].ip}' 2>/dev/null || true)"
    if [ "$public_service_type" = LoadBalancer ] &&
       [ "$public_spec_ip" = "$AGENTFACTORY_PUBLIC_LB_IP" ] &&
       [ "$public_status_ip" = "$AGENTFACTORY_PUBLIC_LB_IP" ]; then
      break
    fi
    sleep "$AGENTFACTORY_POLL_INTERVAL"
    public_lb_elapsed=$((public_lb_elapsed + AGENTFACTORY_POLL_INTERVAL))
  done
  [ "$public_service_type" = LoadBalancer ] || {
    echo "Agent Factory $test_id assertion failed: public Service type=$public_service_type" >&2
    exit 1
  }
  [ "$public_spec_ip" = "$AGENTFACTORY_PUBLIC_LB_IP" ] || {
    echo "Agent Factory $test_id assertion failed: public Service loadBalancerIP=$public_spec_ip" >&2
    exit 1
  }
  [ "$public_status_ip" = "$AGENTFACTORY_PUBLIC_LB_IP" ] || {
    echo "Agent Factory $test_id assertion failed: assigned public IP=$public_status_ip" >&2
    exit 1
  }
fi

sidb_status="$(agentfactory_case_kubectl -n "$SIDB_CASE_NAMESPACE" get singleinstancedatabase "$SIDB_TEST_NAME" -o jsonpath='{.status.status}' 2>/dev/null || true)"
case "$sidb_status" in Ready|READY|Healthy|HEALTHY|Available|AVAILABLE) ;; *) echo "Agent Factory $test_id assertion failed: SIDB status=$sidb_status" >&2; exit 1 ;; esac

operator_deployment="$OPERATOR_DEPLOYMENT"
[ -n "$operator_deployment" ] || operator_deployment=oracle-database-operator-controller-manager
operator_available="$(agentfactory_case_kubectl -n "$OPERATOR_NAMESPACE" get deployment "$operator_deployment" -o jsonpath='{.status.availableReplicas}' 2>/dev/null || true)"
[[ "$operator_available" =~ ^[0-9]+$ ]] && [ "$operator_available" -ge 1 ] || { echo "Agent Factory $test_id assertion failed: operator unavailable" >&2; exit 1; }

printf '%s\n' "case=$test_id" "phase=$phase" "database_phase=$database_phase" \
  "ready_replicas=$ready_replicas" "nodes=$pod_nodes" "zones=$pod_zones" \
  "managed_pvc=$pvc_name" "storage_class=$AGENTFACTORY_STORAGE_CLASS" \
  "worker_node_selector=${AGENTFACTORY_WORKER_NODE_SELECTOR_KEY}=${AGENTFACTORY_WORKER_NODE_SELECTOR_VALUE}" \
  "cert_fqdn=$AGENTFACTORY_CERT_FQDN" "cert_ip=$AGENTFACTORY_CERT_IP_ADDRESS" \
  > "$AGENTFACTORY_CASE_RESULTS_DIR/assertion-summary.txt"
echo "Agent Factory $test_id assertions passed: 3 replicas across 3 label-selected worker nodes using managed RWX FSS PVC"
