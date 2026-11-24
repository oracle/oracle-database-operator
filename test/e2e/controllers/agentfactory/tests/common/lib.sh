#!/usr/bin/env bash

e2e_workload_health_file="${CI_PROJECT_DIR:-$(git rev-parse --show-toplevel 2>/dev/null || pwd)}/.gitlab/ci/e2e-workload-health.sh"
# shellcheck disable=SC1090
source "$e2e_workload_health_file"
unset e2e_workload_health_file

agentfactory_case_init() {
  : "${AGENTFACTORY_CASE_ID:=test01}"
  : "${AGENTFACTORY_CASE_NAMESPACE:=${AGENTFACTORY_E2E_NAMESPACE:-paf-ns}}"
  : "${AGENTFACTORY_NAME:=agentfactory-e2e-${AGENTFACTORY_CASE_ID}}"
  : "${AGENTFACTORY_CASE_PIPELINE:=${CI_PIPELINE_ID:-local}}"
  : "${AGENTFACTORY_TIMEOUT_SECONDS:=2400}"
  : "${AGENTFACTORY_POLL_INTERVAL:=15}"
  : "${AGENTFACTORY_PUBLIC_LB_TIMEOUT_SECONDS:=600}"
  : "${AGENTFACTORY_IMAGE:=${AGENT_FACTORY_IMAGE:-${AGENTFACTORY_CONTAINER_IMAGE:-container-registry.oracle.com/database/applied-ai:latest}}}"
  : "${AGENTFACTORY_IMAGE_PULL_POLICY:=Always}"
  : "${AGENTFACTORY_PULL_SECRET:=${AGENT_FACTORY_PULL_SECRET:-}}"
  : "${AGENTFACTORY_DB_USERNAME:=paf_user}"
  : "${AGENTFACTORY_DB_HOST:=sidb-e2e-primary.${SIDB_E2E_NAMESPACE:-sidb-ns}.svc.cluster.local}"
  : "${AGENTFACTORY_DB_SERVICE_NAME:=${SIDB_TEST_PDB:-ORCLPDB1}}"
  : "${AGENTFACTORY_DB_PORT:=1521}"
  : "${AGENTFACTORY_DB_CONNECTION_TYPE:=Details}"
  : "${AGENTFACTORY_DB_SELECTED_TNS_ALIAS:=}"
  : "${AGENTFACTORY_DB_HAS_OCI_CERTIFICATES:=false}"
  : "${AGENTFACTORY_DB_AIR_GAPPED:=false}"
  : "${AGENTFACTORY_DB_PROTOCOL:=TCP}"
  : "${AGENTFACTORY_DB_HEALTH_PATH:=/agentFactory/v1/getAppState}"
  : "${AGENTFACTORY_CERT_IP_ADDRESS:=}"
  : "${AGENTFACTORY_UI_ADMIN_USERNAME:=admin@example.com}"
  : "${AGENTFACTORY_RUNTIME_SECRET:=agentfactory-runtime-secret}"
  : "${AGENTFACTORY_UI_ADMIN_PASSWORD_KEY:=paf_ui_admin_password}"
  : "${AGENTFACTORY_DB_PASSWORD_KEY:=paf_db_password}"
  : "${AGENTFACTORY_WALLET_ZIP_KEY:=db_wallet.zip}"
  : "${AGENTFACTORY_BOOTSTRAP_SECRET:=paf-db-bootstrap-admin}"
  : "${AGENTFACTORY_BOOTSTRAP_USERNAME:=SYS}"
  : "${AGENTFACTORY_BOOTSTRAP_USERNAME_KEY:=sys}"
  : "${AGENTFACTORY_BOOTSTRAP_PASSWORD_KEY:=password}"
  : "${AGENTFACTORY_BOOTSTRAP_PASSWORD:=${SIDB_ADMIN_PASSWORD:-}}"
  : "${AGENTFACTORY_BOOTSTRAP_ENABLED:=true}"
  : "${AGENTFACTORY_BOOTSTRAP_CLEANUP_POLICY:=Retain}"
  : "${AGENTFACTORY_BOOTSTRAP_PRIVILEGE:=SYSDBA}"
  : "${AGENTFACTORY_BOOTSTRAP_PRESERVE_FAILED_JOB:=false}"
  : "${AGENTFACTORY_PVC_NAME:=agentfactory-e2e-pvc}"
  : "${AGENTFACTORY_PVC_SIZE:=100Gi}"
  : "${AGENTFACTORY_STORAGE_VOLUME_NAME:=app-mount}"
  : "${AGENTFACTORY_STORAGE_MOUNT_PATH:=/mount}"
  : "${AGENTFACTORY_STORAGE_MODE:=existingPVC}"
  : "${AGENTFACTORY_MANAGED_STORAGE_CLASS:=}"
  : "${AGENTFACTORY_REPLICAS:=1}"
  : "${AGENTFACTORY_WORKER_NODES:=}"
  : "${AGENTFACTORY_WORKER_NODE_1:=}"
  : "${AGENTFACTORY_WORKER_NODE_2:=}"
  : "${AGENTFACTORY_WORKER_NODE_3:=}"
  : "${AGENTFACTORY_WORKER_NODE_SELECTOR_KEY:=}"
  : "${AGENTFACTORY_WORKER_NODE_SELECTOR_VALUE:=}"
  : "${AGENTFACTORY_PUBLIC_LB_ENABLED:=true}"
  : "${AGENTFACTORY_PUBLIC_LB_IP:=}"
  : "${AGENTFACTORY_PUBLIC_LB_SUBNET:=}"
  : "${AGENTFACTORY_PUBLIC_LB_EXTERNAL_TRAFFIC_POLICY:=Cluster}"
  : "${AGENTFACTORY_CERT_FQDN:=}"
  : "${AGENTFACTORY_STORAGE_CLASS:=${SIDB_STORAGE_CLASS:-}}"
  : "${AGENTFACTORY_CASE_CLEANUP:=false}"
  : "${AGENTFACTORY_CASE_RESULTS_DIR:=${OPERATOR_LOG_DIR:-${CI_PROJECT_DIR:-$PWD}/artifacts}/agentfactory/${AGENTFACTORY_CASE_NAMESPACE}-${AGENTFACTORY_CASE_ID}}"
  : "${AGENTFACTORY_RENDERED_MANIFEST:=${AGENTFACTORY_CASE_RESULTS_DIR}/${AGENTFACTORY_CASE_ID}-rendered.yaml}"
  export AGENTFACTORY_CASE_ID AGENTFACTORY_CASE_NAMESPACE AGENTFACTORY_NAME
  export AGENTFACTORY_CASE_PIPELINE AGENTFACTORY_TIMEOUT_SECONDS AGENTFACTORY_POLL_INTERVAL
  export AGENTFACTORY_PUBLIC_LB_TIMEOUT_SECONDS AGENTFACTORY_PUBLIC_LB_ENABLED
  export AGENTFACTORY_PUBLIC_LB_IP AGENTFACTORY_PUBLIC_LB_SUBNET
  export AGENTFACTORY_PUBLIC_LB_EXTERNAL_TRAFFIC_POLICY
  export AGENTFACTORY_CERT_FQDN
  export AGENTFACTORY_IMAGE AGENTFACTORY_IMAGE_PULL_POLICY AGENTFACTORY_PULL_SECRET AGENTFACTORY_DB_USERNAME AGENTFACTORY_DB_HOST
  export AGENTFACTORY_DB_SERVICE_NAME AGENTFACTORY_DB_PORT AGENTFACTORY_DB_CONNECTION_TYPE
  export AGENTFACTORY_DB_SELECTED_TNS_ALIAS AGENTFACTORY_DB_HAS_OCI_CERTIFICATES
  export AGENTFACTORY_DB_AIR_GAPPED AGENTFACTORY_DB_PROTOCOL AGENTFACTORY_DB_HEALTH_PATH
  export AGENTFACTORY_CERT_IP_ADDRESS
  export AGENTFACTORY_UI_ADMIN_USERNAME AGENTFACTORY_RUNTIME_SECRET
  export AGENTFACTORY_UI_ADMIN_PASSWORD_KEY AGENTFACTORY_DB_PASSWORD_KEY
  export AGENTFACTORY_WALLET_ZIP_KEY
  export AGENTFACTORY_BOOTSTRAP_SECRET AGENTFACTORY_BOOTSTRAP_USERNAME
  export AGENTFACTORY_BOOTSTRAP_USERNAME_KEY AGENTFACTORY_BOOTSTRAP_PASSWORD_KEY
  export AGENTFACTORY_BOOTSTRAP_PASSWORD
  export AGENTFACTORY_BOOTSTRAP_ENABLED AGENTFACTORY_BOOTSTRAP_CLEANUP_POLICY
  export AGENTFACTORY_BOOTSTRAP_PRIVILEGE AGENTFACTORY_BOOTSTRAP_PRESERVE_FAILED_JOB AGENTFACTORY_CASE_CLEANUP
  export AGENTFACTORY_PVC_NAME AGENTFACTORY_PVC_SIZE AGENTFACTORY_STORAGE_CLASS
  export AGENTFACTORY_STORAGE_VOLUME_NAME AGENTFACTORY_STORAGE_MOUNT_PATH
  export AGENTFACTORY_STORAGE_MODE AGENTFACTORY_MANAGED_STORAGE_CLASS
  export AGENTFACTORY_REPLICAS AGENTFACTORY_WORKER_NODES
  export AGENTFACTORY_WORKER_NODE_SELECTOR_KEY AGENTFACTORY_WORKER_NODE_SELECTOR_VALUE
  export AGENTFACTORY_CASE_RESULTS_DIR AGENTFACTORY_RENDERED_MANIFEST
}

agentfactory_case_kubectl() {
  if [ -n "${KUBE_CONTEXT_NAME:-}" ]; then
    command kubectl --context "$KUBE_CONTEXT_NAME" "$@"
  else
    command kubectl "$@"
  fi
}

agentfactory_case_render_manifest() {
  local source_file="$1" destination_file="$2" line
  local public_lb_subnet="${AGENTFACTORY_PUBLIC_LB_SUBNET//[[:space:]]/}"

  if [[ "$public_lb_subnet" == *,* ]]; then
    echo "AGENTFACTORY_PUBLIC_LB_SUBNET accepts one subnet OCID" >&2
    return 2
  fi

  mkdir -p "$(dirname "$destination_file")"
  : > "$destination_file"
  while IFS= read -r line || [ -n "$line" ]; do
    if [ -z "$AGENTFACTORY_PULL_SECRET" ] &&
       [ "$line" = '      pullSecret: "__AGENTFACTORY_PULL_SECRET__"' ]; then
      continue
    fi
    if [ -z "$AGENTFACTORY_PUBLIC_LB_IP" ] &&
       [ "$line" = '        ip: "__AGENTFACTORY_PUBLIC_LB_IP__"' ]; then
      continue
    fi
    if [ -z "$public_lb_subnet" ] &&
       [ "$line" = '        annotations:' ]; then
      continue
    fi
    if [ -z "$public_lb_subnet" ] &&
       [ "$line" = '          service.beta.kubernetes.io/oci-load-balancer-subnet1: "__AGENTFACTORY_PUBLIC_LB_SUBNET__"' ]; then
      continue
    fi
    if [ -z "$AGENTFACTORY_CERT_FQDN" ] &&
       [ "$line" = '    certFqdn: "__AGENTFACTORY_CERT_FQDN__"' ]; then
      continue
    fi
    if [ -z "$AGENTFACTORY_CERT_IP_ADDRESS" ] &&
       [ "$line" = '    certIpAddress: "__AGENTFACTORY_CERT_IP_ADDRESS__"' ]; then
      continue
    fi
    if [ -z "$AGENTFACTORY_PUBLIC_LB_IP" ] &&
       [ "$line" = '    certIpAddress: "__AGENTFACTORY_PUBLIC_LB_IP__"' ]; then
      continue
    fi
    line="${line//__AGENTFACTORY_NAME__/$AGENTFACTORY_NAME}"
    line="${line//__AGENTFACTORY_IMAGE__/$AGENTFACTORY_IMAGE}"
    line="${line//__AGENTFACTORY_IMAGE_PULL_POLICY__/$AGENTFACTORY_IMAGE_PULL_POLICY}"
    line="${line//__AGENTFACTORY_PULL_SECRET__/$AGENTFACTORY_PULL_SECRET}"
    line="${line//__AGENTFACTORY_CASE_PIPELINE__/$AGENTFACTORY_CASE_PIPELINE}"
    line="${line//__AGENTFACTORY_UI_ADMIN_USERNAME__/$AGENTFACTORY_UI_ADMIN_USERNAME}"
    line="${line//__AGENTFACTORY_DB_USERNAME__/$AGENTFACTORY_DB_USERNAME}"
    line="${line//__AGENTFACTORY_DB_HOST__/$AGENTFACTORY_DB_HOST}"
    line="${line//__AGENTFACTORY_DB_PORT__/$AGENTFACTORY_DB_PORT}"
    line="${line//__AGENTFACTORY_DB_SERVICE_NAME__/$AGENTFACTORY_DB_SERVICE_NAME}"
    line="${line//__AGENTFACTORY_DB_CONNECTION_TYPE__/$AGENTFACTORY_DB_CONNECTION_TYPE}"
    line="${line//__AGENTFACTORY_DB_SELECTED_TNS_ALIAS__/$AGENTFACTORY_DB_SELECTED_TNS_ALIAS}"
    line="${line//__AGENTFACTORY_DB_HAS_OCI_CERTIFICATES__/$AGENTFACTORY_DB_HAS_OCI_CERTIFICATES}"
    line="${line//__AGENTFACTORY_DB_AIR_GAPPED__/$AGENTFACTORY_DB_AIR_GAPPED}"
    line="${line//__AGENTFACTORY_DB_PROTOCOL__/$AGENTFACTORY_DB_PROTOCOL}"
    line="${line//__AGENTFACTORY_DB_HEALTH_PATH__/$AGENTFACTORY_DB_HEALTH_PATH}"
    line="${line//__AGENTFACTORY_CERT_IP_ADDRESS__/$AGENTFACTORY_CERT_IP_ADDRESS}"
    line="${line//__AGENTFACTORY_REPLICAS__/$AGENTFACTORY_REPLICAS}"
    line="${line//__AGENTFACTORY_WORKER_NODE_1__/$AGENTFACTORY_WORKER_NODE_1}"
    line="${line//__AGENTFACTORY_WORKER_NODE_2__/$AGENTFACTORY_WORKER_NODE_2}"
    line="${line//__AGENTFACTORY_WORKER_NODE_3__/$AGENTFACTORY_WORKER_NODE_3}"
    line="${line//__AGENTFACTORY_WORKER_NODE_SELECTOR_KEY__/$AGENTFACTORY_WORKER_NODE_SELECTOR_KEY}"
    line="${line//__AGENTFACTORY_WORKER_NODE_SELECTOR_VALUE__/$AGENTFACTORY_WORKER_NODE_SELECTOR_VALUE}"
    line="${line//__AGENTFACTORY_PVC_SIZE__/$AGENTFACTORY_PVC_SIZE}"
    line="${line//__AGENTFACTORY_STORAGE_CLASS__/$AGENTFACTORY_STORAGE_CLASS}"
    line="${line//__AGENTFACTORY_BOOTSTRAP_SECRET__/$AGENTFACTORY_BOOTSTRAP_SECRET}"
    line="${line//__AGENTFACTORY_BOOTSTRAP_USERNAME_KEY__/$AGENTFACTORY_BOOTSTRAP_USERNAME_KEY}"
    line="${line//__AGENTFACTORY_BOOTSTRAP_PASSWORD_KEY__/$AGENTFACTORY_BOOTSTRAP_PASSWORD_KEY}"
    line="${line//__AGENTFACTORY_BOOTSTRAP_ENABLED__/$AGENTFACTORY_BOOTSTRAP_ENABLED}"
    line="${line//__AGENTFACTORY_BOOTSTRAP_CLEANUP_POLICY__/$AGENTFACTORY_BOOTSTRAP_CLEANUP_POLICY}"
    line="${line//__AGENTFACTORY_BOOTSTRAP_PRIVILEGE__/$AGENTFACTORY_BOOTSTRAP_PRIVILEGE}"
    line="${line//__AGENTFACTORY_BOOTSTRAP_PRESERVE_FAILED_JOB__/$AGENTFACTORY_BOOTSTRAP_PRESERVE_FAILED_JOB}"
    line="${line//__AGENTFACTORY_RUNTIME_SECRET__/$AGENTFACTORY_RUNTIME_SECRET}"
    line="${line//__AGENTFACTORY_UI_ADMIN_PASSWORD_KEY__/$AGENTFACTORY_UI_ADMIN_PASSWORD_KEY}"
    line="${line//__AGENTFACTORY_DB_PASSWORD_KEY__/$AGENTFACTORY_DB_PASSWORD_KEY}"
    line="${line//__AGENTFACTORY_WALLET_ZIP_KEY__/$AGENTFACTORY_WALLET_ZIP_KEY}"
    line="${line//__AGENTFACTORY_PVC_NAME__/$AGENTFACTORY_PVC_NAME}"
    line="${line//__AGENTFACTORY_STORAGE_VOLUME_NAME__/$AGENTFACTORY_STORAGE_VOLUME_NAME}"
    line="${line//__AGENTFACTORY_STORAGE_MOUNT_PATH__/$AGENTFACTORY_STORAGE_MOUNT_PATH}"
    line="${line//__AGENTFACTORY_PUBLIC_LB_ENABLED__/$AGENTFACTORY_PUBLIC_LB_ENABLED}"
    line="${line//__AGENTFACTORY_PUBLIC_LB_IP__/$AGENTFACTORY_PUBLIC_LB_IP}"
    line="${line//__AGENTFACTORY_PUBLIC_LB_SUBNET__/$public_lb_subnet}"
    line="${line//__AGENTFACTORY_PUBLIC_LB_EXTERNAL_TRAFFIC_POLICY__/$AGENTFACTORY_PUBLIC_LB_EXTERNAL_TRAFFIC_POLICY}"
    line="${line//__AGENTFACTORY_CERT_FQDN__/$AGENTFACTORY_CERT_FQDN}"
    printf '%s\n' "$line" >> "$destination_file"
  done < "$source_file"
}

agentfactory_case_collect_pod_logs() {
  local namespace="$1" output_dir="$2" pod container safe_name pod_names containers
  mkdir -p "$output_dir"
  pod_names="$(agentfactory_case_kubectl -n "$namespace" get pods \
    -o jsonpath='{range .items[*]}{.metadata.name}{"\n"}{end}' 2>/dev/null || true)"

  while IFS= read -r pod; do
    [ -n "$pod" ] || continue
    local pod_dir="$output_dir/$pod"
    mkdir -p "$pod_dir"
    agentfactory_case_kubectl -n "$namespace" get pod "$pod" -o yaml \
      > "$pod_dir/pod.yaml" 2>&1 || true
    containers="$(agentfactory_case_kubectl -n "$namespace" get pod "$pod" \
      -o jsonpath='{range .spec.initContainers[*]}{.name}{"\n"}{end}{range .spec.containers[*]}{.name}{"\n"}{end}' \
      2>/dev/null || true)"

    while IFS= read -r container; do
      [ -n "$container" ] || continue
      safe_name="${container//[^A-Za-z0-9_.-]/_}"
      agentfactory_case_kubectl -n "$namespace" logs "$pod" -c "$container" \
        --timestamps > "$pod_dir/${safe_name}.log" 2>&1 || true
      agentfactory_case_kubectl -n "$namespace" logs "$pod" -c "$container" \
        --previous --timestamps > "$pod_dir/${safe_name}-previous.log" 2>&1 || true
    done <<< "$containers"
  done <<< "$pod_names"
}

agentfactory_case_collect_operator_diagnostics() {
  local output_dir="${AGENTFACTORY_CASE_RESULTS_DIR}/diagnostics/operator"
  local operator_namespace="${OPERATOR_NAMESPACE:-oracle-database-operator-system}"
  local operator_deployment="${OPERATOR_DEPLOYMENT:-oracle-database-operator-controller-manager}"
  mkdir -p "$output_dir"

  agentfactory_case_kubectl -n "$operator_namespace" \
    get deployment "$operator_deployment" -o yaml \
    > "$output_dir/deployment.yaml" 2>&1 || true
  agentfactory_case_kubectl -n "$operator_namespace" \
    get pods -o yaml \
    > "$output_dir/pods.yaml" 2>&1 || true
  agentfactory_case_kubectl -n "$operator_namespace" \
    get events --sort-by=.lastTimestamp \
    > "$output_dir/events.txt" 2>&1 || true
  agentfactory_case_kubectl -n "$operator_namespace" \
    logs "deployment/$operator_deployment" --all-containers --prefix --timestamps \
    > "$output_dir/controller-manager.log" 2>&1 || true
  grep -iE 'agentfactory|agent.factory|privateai.oracle.com' \
    "$output_dir/controller-manager.log" > "$output_dir/agentfactory-controller.log" || true
  grep -iE 'singleinstancedatabase|single.instance.database|sidb|database.oracle.com' \
    "$output_dir/controller-manager.log" > "$output_dir/sidb-controller.log" || true
  printf 'namespace=%s\ndeployment=%s\ncontrollers=agentfactory,sidb\n' \
    "$operator_namespace" "$operator_deployment" > "$output_dir/run-info.txt"
}

agentfactory_case_collect_diagnostics() {
  local reason="${1:-final}"
  local diagnostics_dir="$AGENTFACTORY_CASE_RESULTS_DIR/diagnostics"
  mkdir -p "$diagnostics_dir"
  cp "$AGENTFACTORY_RENDERED_MANIFEST" \
    "$diagnostics_dir/${AGENTFACTORY_CASE_ID}-rendered.yaml" 2>/dev/null || true
  agentfactory_case_kubectl -n "$AGENTFACTORY_CASE_NAMESPACE" \
    get agentfactory "$AGENTFACTORY_NAME" -o yaml > "$diagnostics_dir/agentfactory.yaml" 2>&1 || true
  agentfactory_case_kubectl -n "$AGENTFACTORY_CASE_NAMESPACE" \
    get deployment,job,svc,pods,pvc,events -o yaml > "$diagnostics_dir/workloads.yaml" 2>&1 || true
  agentfactory_case_kubectl -n "$AGENTFACTORY_CASE_NAMESPACE" \
    logs "deployment/$AGENTFACTORY_NAME" --all-containers --prefix \
    > "$diagnostics_dir/deployment.log" 2>&1 || true
  # The bootstrap database setup is a one-shot Job. Its Pod is expected to
  # reach Succeeded/Completed rather than Ready, so collect its container
  # logs separately from the Deployment logs for failure diagnosis.
  agentfactory_case_collect_pod_logs "$AGENTFACTORY_CASE_NAMESPACE" \
    "$diagnostics_dir/agentfactory-pod-logs"

  local sidb_namespace="${SIDB_CASE_NAMESPACE:-${SIDB_E2E_NAMESPACE:-sidb-ns}}"
  local sidb_name="${SIDB_TEST_NAME:-sidb-e2e-primary}"
  mkdir -p "$diagnostics_dir/sidb"
  agentfactory_case_kubectl -n "$sidb_namespace" \
    get singleinstancedatabase "$sidb_name" -o yaml \
    > "$diagnostics_dir/sidb/sidb.yaml" 2>&1 || true
  agentfactory_case_kubectl -n "$sidb_namespace" \
    get deployment,svc,pods,pvc,events -o yaml \
    > "$diagnostics_dir/sidb/workloads.yaml" 2>&1 || true
  agentfactory_case_collect_pod_logs "$sidb_namespace" "$diagnostics_dir/sidb/pod-logs"

  agentfactory_case_collect_operator_diagnostics
  printf 'reason=%s\nnamespace=%s\nname=%s\nimage=%s\ndatabase_host=%s\n' \
    "$reason" "$AGENTFACTORY_CASE_NAMESPACE" "$AGENTFACTORY_NAME" \
    "$AGENTFACTORY_IMAGE" "$AGENTFACTORY_DB_HOST" > "$diagnostics_dir/run-info.txt"
}

agentfactory_case_assert_image_usage() {
  local spec_policy deployment_policy job_name job_policy pod pod_policy image_id expected_digest
  spec_policy="$(agentfactory_case_kubectl -n "$AGENTFACTORY_CASE_NAMESPACE" get agentfactory "$AGENTFACTORY_NAME" -o jsonpath='{.spec.runtime.image.pullPolicy}')"
  [ "$spec_policy" = "$AGENTFACTORY_IMAGE_PULL_POLICY" ] || {
    echo "Agent Factory $AGENTFACTORY_CASE_ID: CR image pullPolicy is '$spec_policy', expected '$AGENTFACTORY_IMAGE_PULL_POLICY'" >&2
    return 1
  }
  deployment_policy="$(agentfactory_case_kubectl -n "$AGENTFACTORY_CASE_NAMESPACE" get deployment "$AGENTFACTORY_NAME" -o jsonpath='{.spec.template.spec.containers[0].imagePullPolicy}')"
  [ "$deployment_policy" = "$AGENTFACTORY_IMAGE_PULL_POLICY" ] || {
    echo "Agent Factory $AGENTFACTORY_CASE_ID: Deployment imagePullPolicy is '$deployment_policy', expected '$AGENTFACTORY_IMAGE_PULL_POLICY'" >&2
    return 1
  }
  expected_digest=""
  [[ "$AGENTFACTORY_IMAGE" == *@sha256:* ]] && expected_digest="${AGENTFACTORY_IMAGE##*@}"
  mapfile -t pods < <(agentfactory_case_kubectl -n "$AGENTFACTORY_CASE_NAMESPACE" get pods -l "app.kubernetes.io/instance=$AGENTFACTORY_NAME,!job-name" -o jsonpath='{range .items[*]}{.metadata.name}{"\n"}{end}')
  [ "${#pods[@]}" -gt 0 ] || {
    echo "Agent Factory $AGENTFACTORY_CASE_ID: no runtime Pods found for image verification" >&2
    return 1
  }
  for pod in "${pods[@]}"; do
    pod_policy="$(agentfactory_case_kubectl -n "$AGENTFACTORY_CASE_NAMESPACE" get pod "$pod" -o jsonpath='{.spec.containers[0].imagePullPolicy}')"
    [ "$pod_policy" = "$AGENTFACTORY_IMAGE_PULL_POLICY" ] || {
      echo "Agent Factory $AGENTFACTORY_CASE_ID: Pod $pod imagePullPolicy is '$pod_policy', expected '$AGENTFACTORY_IMAGE_PULL_POLICY'" >&2
      return 1
    }
    image_id="$(agentfactory_case_kubectl -n "$AGENTFACTORY_CASE_NAMESPACE" get pod "$pod" -o jsonpath='{.status.containerStatuses[0].imageID}')"
    [ -n "$image_id" ] || {
      echo "Agent Factory $AGENTFACTORY_CASE_ID: Pod $pod has no resolved imageID" >&2
      return 1
    }
    if [ -n "$expected_digest" ] && [[ "$image_id" != *"$expected_digest" ]]; then
      echo "Agent Factory $AGENTFACTORY_CASE_ID: Pod $pod imageID '$image_id' does not match '$expected_digest'" >&2
      return 1
    fi
  done
  job_name="${AGENTFACTORY_NAME}-db-setup"
  if agentfactory_case_kubectl -n "$AGENTFACTORY_CASE_NAMESPACE" get job "$job_name" >/dev/null 2>&1; then
    job_policy="$(agentfactory_case_kubectl -n "$AGENTFACTORY_CASE_NAMESPACE" get job "$job_name" -o jsonpath='{.spec.template.spec.containers[0].imagePullPolicy}')"
    [ "$job_policy" = "$AGENTFACTORY_IMAGE_PULL_POLICY" ] || {
      echo "Agent Factory $AGENTFACTORY_CASE_ID: database setup Job imagePullPolicy is '$job_policy', expected '$AGENTFACTORY_IMAGE_PULL_POLICY'" >&2
      return 1
    }
  fi
}
