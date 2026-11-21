#!/usr/bin/env bash
set -Eeuo pipefail

script_dir="$(CDPATH= cd -- "$(dirname "$0")" && pwd)"

export AGENTFACTORY_CASE_ID=test04
export AGENTFACTORY_NAME=agentfactory-e2e-test04
export AGENTFACTORY_RUNTIME_SECRET=agentfactory-runtime-secret-test04
export AGENTFACTORY_BOOTSTRAP_SECRET=paf-db-bootstrap-admin-test04
export AGENTFACTORY_STORAGE_MODE=dynamicFSS
export AGENTFACTORY_STORAGE_VOLUME_NAME=app-mount
export AGENTFACTORY_STORAGE_MOUNT_PATH=/mount
export AGENTFACTORY_REPLICAS=3
export AGENTFACTORY_DB_CONNECTION_TYPE=Details
export AGENTFACTORY_DB_PROTOCOL=TCP
export AGENTFACTORY_DB_PORT=1521
export AGENTFACTORY_DB_HAS_OCI_CERTIFICATES=false
export AGENTFACTORY_BOOTSTRAP_ENABLED=true
export AGENTFACTORY_BOOTSTRAP_PRIVILEGE=SYSDBA
: "${AGENTFACTORY_PUBLIC_LB_ENABLED:=false}"
: "${AGENTFACTORY_PUBLIC_LB_IP:=}"
if [ -n "$AGENTFACTORY_PUBLIC_LB_IP" ]; then
  # A supplied pre-allocated IP is the opt-in signal for public exposure.
  export AGENTFACTORY_PUBLIC_LB_ENABLED=true
  # The certificate identity must match the externally reachable endpoint.
  export AGENTFACTORY_CERT_IP_ADDRESS="$AGENTFACTORY_PUBLIC_LB_IP"
fi
export AGENTFACTORY_PUBLIC_LB_ENABLED AGENTFACTORY_PUBLIC_LB_IP
unset AGENTFACTORY_DB_SELECTED_TNS_ALIAS
unset AGENTFACTORY_PREPARE_DATABASE_SCRIPT
unset AGENTFACTORY_MANAGED_STORAGE_CLASS

: "${AGENTFACTORY_FSS_AVAILABILITY_DOMAIN:=}"
: "${AGENTFACTORY_FSS_MOUNT_TARGET_OCID:=}"
: "${AGENTFACTORY_FSS_MOUNT_TARGET_SUBNET_OCID:=}"
: "${AGENTFACTORY_FSS_STORAGE_CLASS:=}"
: "${AGENTFACTORY_WORKER_NODES:=}"
: "${AGENTFACTORY_BOOTSTRAP_CLEANUP_POLICY:=DeleteOnSuccess}"
: "${AGENTFACTORY_BOOTSTRAP_PRESERVE_FAILED_JOB:=true}"
: "${AGENTFACTORY_CERT_FQDN:=}"
: "${AGENTFACTORY_CERT_IP_ADDRESS:=}"

export AGENTFACTORY_PVC_NAME="$AGENTFACTORY_NAME-$AGENTFACTORY_STORAGE_VOLUME_NAME"
if [ -n "$AGENTFACTORY_FSS_STORAGE_CLASS" ]; then
  export AGENTFACTORY_MANAGED_STORAGE_CLASS="$AGENTFACTORY_FSS_STORAGE_CLASS"
  export AGENTFACTORY_STORAGE_CLASS="$AGENTFACTORY_FSS_STORAGE_CLASS"
else
  pipeline_id="${CI_PIPELINE_ID:-local}"
  export AGENTFACTORY_STORAGE_CLASS="agentfactory-fss-test04-$pipeline_id"
fi

sidb_namespace="${SIDB_CASE_NAMESPACE:-${SIDB_E2E_NAMESPACE:-sidb-ns}}"
export SIDB_TEST_NAME=sidb-e2e-primary
export SIDB_TEST_PDB="${SIDB_TEST_PDB:-ORCLPDB1}"
if [ -z "${AGENTFACTORY_DB_HOST:-}" ] ||
   [[ "$AGENTFACTORY_DB_HOST" == sidb-e2e-primary-test*."$sidb_namespace".svc.cluster.local ]]; then
  export AGENTFACTORY_DB_HOST="sidb-e2e-primary.$sidb_namespace.svc.cluster.local"
fi

export AGENTFACTORY_PREPARE_STORAGE_SCRIPT="$script_dir/prepare-storage.sh"
export AGENTFACTORY_TEST_TEMPLATE="$script_dir/test04.yaml"
export AGENTFACTORY_ASSERT_SCRIPT="$script_dir/assert.sh"

: "${AGENTFACTORY_WORKER_NODE_SELECTOR_KEY:=kubernetes.io/os}"
: "${AGENTFACTORY_WORKER_NODE_SELECTOR_VALUE:=linux}"
export AGENTFACTORY_WORKER_NODE_SELECTOR_KEY AGENTFACTORY_WORKER_NODE_SELECTOR_VALUE
unset AGENTFACTORY_WORKER_NODES AGENTFACTORY_WORKER_NODE_1 AGENTFACTORY_WORKER_NODE_2 AGENTFACTORY_WORKER_NODE_3

node_selector="${AGENTFACTORY_WORKER_NODE_SELECTOR_KEY}=${AGENTFACTORY_WORKER_NODE_SELECTOR_VALUE}"
node_rows="$(kubectl get nodes -l "$node_selector" -o json 2>/dev/null |
  jq -r '.items[]
    | select(any(.status.conditions[]?; .type == "Ready" and .status == "True"))
    | .metadata.name' | sort)"
node1= node2= node3=
while IFS= read -r node; do
  [ -n "$node" ] || continue
  if [ -z "$node1" ]; then node1="$node"
  elif [ -z "$node2" ]; then node2="$node"
  elif [ -z "$node3" ]; then node3="$node"; break
  fi
done <<< "$node_rows"

[ -n "$node1" ] && [ -n "$node2" ] && [ -n "$node3" ] || {
  echo "Agent Factory test04: selector $node_selector matched fewer than three Ready nodes" >&2
  exit 2
}
[ "$node1" != "$node2" ] && [ "$node1" != "$node3" ] && [ "$node2" != "$node3" ] || {
  echo "Agent Factory test04: selector $node_selector did not identify three distinct nodes" >&2
  exit 2
}
for node in "$node1" "$node2" "$node3"; do
  ready="$(kubectl get node "$node" -o jsonpath='{range .status.conditions[?(@.type=="Ready")]}{.status}{end}' 2>/dev/null || true)"
  [ "$ready" = "True" ] || { echo "Agent Factory test04: node is not Ready: $node" >&2; exit 2; }
done

exec "$script_dir/../common/run.sh"
