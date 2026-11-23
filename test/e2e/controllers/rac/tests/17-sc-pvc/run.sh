#!/usr/bin/env bash
# rac/test17: Storage Class-backed ASM and PVC-backed RAC software.
set -Eeuo pipefail

case_dir="$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
repo_root="${CI_PROJECT_DIR:-$(CDPATH= cd -- "$case_dir/../../../../../.." && pwd)}"
namespace="${RAC_E2E_NAMESPACE:-rac-ns}"
manifest="${RAC_TEST17_MANIFEST_FILE:-$case_dir/test17.yaml}"
multus_rbac="${RAC_TEST17_MULTUS_RBAC_FILE:-$repo_root/rbac/multus-rbac.yaml}"
name="${RAC_TEST17_NAME:-racdbprov-sample}"
image="${RAC_TEST17_IMAGE:?Set RAC_TEST17_IMAGE as an environment variable}"
grid_home="${RAC_TEST17_GRID_HOME:-/u01/app/26ai/grid}"
db_home="${RAC_TEST17_DB_HOME:-/u01/app/oracle/product/26ai/dbhome_1}"
asm_storage_class="${RAC_TEST17_ASM_STORAGE_CLASS:-rook-ceph-block}"
# Oracle CVU rejects the CephFS FUSE statfs sentinel while checking /u01.
# Keep the shared archive staging PVC on CephFS, but use block storage here.
software_storage_class="${RAC_TEST17_SOFTWARE_STORAGE_CLASS:-rook-ceph-block}"
stage_storage_class="${RAC_TEST17_STAGE_STORAGE_CLASS:-rook-cephfs}"
stage_pvc="${RAC_TEST17_STAGE_PVC:-rac-software-stage}"
stage_mount="${RAC_TEST17_STAGE_MOUNT:-/stage/software/26ai/260922}"
service_account="${RAC_TEST17_SERVICE_ACCOUNT:-rac-prov-sa}"
rac_node_name="${RAC_TEST17_RAC_NODE_NAME:-racnode}"
worker_node="${RAC_TEST17_WORKER_NODE:-}"
worker_label_key="${RAC_TEST17_WORKER_LABEL_KEY:-raccluster}"
worker_label_value="${RAC_TEST17_WORKER_LABEL_VALUE:-raccluster01}"
nad_one="${RAC_TEST17_NAD_ONE:-macvlan-conf1}"
nad_two="${RAC_TEST17_NAD_TWO:-macvlan-conf2}"
stage_source="${RAC_TEST17_STAGE_SOURCE:-}"
source_host="${RAC_TEST17_SOURCE_HOST:-}"
source_user="${RAC_TEST17_SOURCE_USER:-opc}"
source_key="${RAC_TEST17_SOURCE_KEY:-${BASTION_SSH_PRIVATE_KEY_FILE:-}}"
source_bastion="${RAC_TEST17_SOURCE_BASTION:-${BASTION_SSH_HOST:-}}"
source_bastion_user="${RAC_TEST17_SOURCE_BASTION_USER:-${BASTION_SSH_USER:-opc}}"
source_proxy_command="${RAC_TEST17_SOURCE_PROXY_COMMAND:-${BASTION_SSH_PROXY_COMMAND:-}}"
source_connect_timeout="${RAC_TEST17_SOURCE_CONNECT_TIMEOUT:-15}"
source_port="${RAC_TEST17_SOURCE_PORT:-22}"
source_bastion_port="${RAC_TEST17_SOURCE_BASTION_PORT:-22}"
source_local_port="${RAC_TEST17_SOURCE_LOCAL_PORT:-6451}"
source_session_id=""
source_tunnel_pid=""
timeout="${RAC_TEST17_TIMEOUT:-90m}"
work_dir="${TMPDIR:-/tmp}/rac-test17-${CI_JOB_ID:-$$}"
rendered="$work_dir/racdb.yaml"

cleanup() { rm -rf "$work_dir"; }
trap cleanup EXIT
fail() { echo "rac/test17: ERROR: $*" >&2; exit 2; }

rac_status_value_is_usable() {
  case "${1:-}" in
    ""|NOTAVAILABLE|PENDING|Pending|UNKNOWN|Unknown)
      return 1
      ;;
    *)
      return 0
      ;;
  esac
}

render_rac_manifest() {
  local source="$1" destination="$2"
  mkdir -p "$(dirname "$destination")"
  sed \
    -e "s|__RAC_NAMESPACE__|$namespace|g" \
    -e "s|__RAC_NAME__|$name|g" \
    -e "s|__RAC_IMAGE__|$image|g" \
    -e "s|__RAC_GRID_HOME__|$grid_home|g" \
    -e "s|__RAC_DB_HOME__|$db_home|g" \
    -e "s|__RAC_ASM_STORAGE_CLASS__|$asm_storage_class|g" \
    -e "s|__RAC_SOFTWARE_STORAGE_CLASS__|$software_storage_class|g" \
    -e "s|__RAC_STAGE_PVC__|$stage_pvc|g" \
    -e "s|__RAC_STAGE_MOUNT__|$stage_mount|g" \
    -e "s|__RAC_SERVICE_ACCOUNT__|$service_account|g" \
    -e "s|__RAC_NODE_NAME__|$rac_node_name|g" \
    -e "s|__RAC_WORKER_LABEL_KEY__|$worker_label_key|g" \
    -e "s|__RAC_WORKER_LABEL_VALUE__|$worker_label_value|g" \
    -e "s|__RAC_NAD_ONE__|$nad_one|g" \
    -e "s|__RAC_NAD_TWO__|$nad_two|g" \
    "$source" > "$destination"
}

timeout_seconds() {
  local value="$1"
  case "$value" in
    *h) echo $(( ${value%h} * 3600 )) ;;
    *m) echo $(( ${value%m} * 60 )) ;;
    *s) echo $(( ${value%s} )) ;;
    *) echo "$value" ;;
  esac
}

command -v kubectl >/dev/null 2>&1 || fail "kubectl is required"
test -r "$manifest" || fail "manifest not readable: $manifest"
test -r "$multus_rbac" || fail "Multus RBAC manifest not readable: $multus_rbac"

remote_ssh() {
  local -a args=(ssh -o BatchMode=yes -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o "ConnectTimeout=$source_connect_timeout")
  [ "$source_port" != 22 ] && args+=( -p "$source_port" )
  [ -n "$source_key" ] && args+=( -i "$source_key" )
  if [ -n "$source_bastion" ]; then
    [ -n "$source_key" ] || fail "RAC_TEST17_SOURCE_KEY is required with RAC_TEST17_SOURCE_BASTION"
    local bastion_proxy="ssh -i $source_key -o BatchMode=yes -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o ConnectTimeout=$source_connect_timeout"
    if [ -n "$source_proxy_command" ]; then
      local bastion_transport="$source_proxy_command"
      bastion_transport="${bastion_transport//%h/$source_bastion}"
      bastion_transport="${bastion_transport//%p/$source_bastion_port}"
      bastion_proxy+=" -o $(printf '%q' "ProxyCommand=$bastion_transport")"
    fi
    bastion_proxy+=" -W %h:%p $source_bastion_user@$source_bastion"
    args+=( -o "ProxyCommand=$bastion_proxy" )
  fi
  "${args[@]}" "$source_user@$source_host" "$@"
}

start_oci_source_tunnel() {
  command -v oci >/dev/null 2>&1 || fail "oci is required for the OCI source tunnel"
  command -v jq >/dev/null 2>&1 || fail "jq is required for the OCI source tunnel"
  [ -n "${BASTION_OCID:-}" ] || fail "BASTION_OCID is required for the OCI source tunnel"
  [ -n "${BASTION_REGION:-}" ] || fail "BASTION_REGION is required for the OCI source tunnel"
  test -r "${BASTION_SSH_PUBLIC_KEY_FILE:-}" || fail "BASTION_SSH_PUBLIC_KEY_FILE is required for the OCI source tunnel"
  test -r "${BASTION_SSH_PRIVATE_KEY_FILE:-}" || fail "BASTION_SSH_PRIVATE_KEY_FILE is required for the OCI source tunnel"
  local session_json ssh_command state deadline
  session_json="$(OCI_CLI_REGION="$BASTION_REGION" oci --config-file "$OCI_CLI_CONFIG_FILE" --profile "${OCI_CLI_PROFILE:-DEFAULT}" bastion session create-port-forwarding \
    --bastion-id "$BASTION_OCID" --display-name "${RAC_TEST17_SOURCE_SESSION_NAME:-rac-test17-source}" \
    --target-private-ip "${RAC_TEST17_SOURCE_HOST}" --target-port 22 \
    --ssh-public-key-file "$BASTION_SSH_PUBLIC_KEY_FILE" --session-ttl "${RAC_TEST17_SOURCE_SESSION_TTL:-10800}" --output json)" ||
    fail "failed to create OCI Bastion session for ${RAC_TEST17_SOURCE_HOST}:22"
  source_session_id="$(printf '%s' "$session_json" | jq -er '.data.id')" || fail "OCI source session has no id"
  deadline=$((SECONDS + ${RAC_TEST17_SOURCE_SESSION_WAIT_TIMEOUT:-600}))
  while [ "$SECONDS" -lt "$deadline" ]; do
    state="$(OCI_CLI_REGION="$BASTION_REGION" oci --config-file "$OCI_CLI_CONFIG_FILE" --profile "${OCI_CLI_PROFILE:-DEFAULT}" bastion session get --session-id "$source_session_id" --query 'data."lifecycle-state"' --raw-output 2>/dev/null || true)"
    [ "$state" = ACTIVE ] && break
    { [ "$state" = FAILED ] || [ "$state" = DELETED ]; } && fail "OCI source session reached state $state"
    sleep 5
  done
  [ "$state" = ACTIVE ] || fail "timed out waiting for OCI source session"
  ssh_command="$(OCI_CLI_REGION="$BASTION_REGION" oci --config-file "$OCI_CLI_CONFIG_FILE" --profile "${OCI_CLI_PROFILE:-DEFAULT}" bastion session get --session-id "$source_session_id" --query 'data."ssh-metadata".command' --raw-output)"
  ssh_command="${ssh_command//<privateKey>/$BASTION_SSH_PRIVATE_KEY_FILE}"
  ssh_command="${ssh_command//<localPort>/$source_local_port}"
  bash -c "exec $ssh_command" >"$work_dir/source-tunnel.log" 2>&1 &
  source_tunnel_pid=$!
  for _ in $(seq 1 60); do
    (exec 3<>"/dev/tcp/127.0.0.1/$source_local_port") >/dev/null 2>&1 && return 0
    kill -0 "$source_tunnel_pid" 2>/dev/null || break
    sleep 2
  done
  cat "$work_dir/source-tunnel.log" >&2 || true
  fail "OCI source tunnel did not open 127.0.0.1:$source_local_port"
}

prepare_stage_pvc() {
  [ -n "$stage_source" ] || return 0
  echo "rac/test17: preparing stage PVC $namespace/$stage_pvc and copying archives from runner path $stage_source"
  kubectl apply -f - <<EOF
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: $stage_pvc
  namespace: $namespace
spec:
  accessModes: [ReadWriteMany]
  storageClassName: $stage_storage_class
  resources:
    requests:
      storage: 100Gi
EOF
  kubectl wait --for=jsonpath='{.status.phase}'=Bound pvc/"$stage_pvc" -n "$namespace" --timeout=10m
  kubectl delete pod rac-stage-loader -n "$namespace" --ignore-not-found=true --wait=true
  kubectl apply -f - <<EOF
apiVersion: v1
kind: Pod
metadata:
  name: rac-stage-loader
  namespace: $namespace
spec:
  restartPolicy: Never
  containers:
  - name: loader
    image: $image
    command: ["/bin/bash", "-c", "mkdir -p '$stage_mount' && sleep infinity"]
    volumeMounts:
    - name: stage
      mountPath: /stage
  volumes:
  - name: stage
    persistentVolumeClaim:
      claimName: $stage_pvc
EOF
  deadline=$((SECONDS + 1800))
  while (( SECONDS < deadline )); do
    phase="$(kubectl get pod rac-stage-loader -n "$namespace" -o jsonpath='{.status.phase}' 2>/dev/null || true)"
    echo "rac-stage-loader: phase=${phase:-unknown} elapsed=$((1800 - deadline + SECONDS))s"
    case "$phase" in
      Running) break ;;
      Failed)
        kubectl describe pod rac-stage-loader -n "$namespace" || true
        kubectl logs rac-stage-loader -n "$namespace" --all-containers=true || true
        fail "software staging loader failed to start"
        ;;
    esac
    kubectl logs rac-stage-loader -n "$namespace" --all-containers=true --tail=5 2>/dev/null || true
    sleep 30
  done
  if [[ "$(kubectl get pod rac-stage-loader -n "$namespace" -o jsonpath='{.status.phase}' 2>/dev/null || true)" != Running ]]; then
    kubectl get pod rac-stage-loader -n "$namespace" -o wide || true
    kubectl describe pod rac-stage-loader -n "$namespace" || true
    kubectl logs rac-stage-loader -n "$namespace" --all-containers=true || true
    fail "software staging loader did not become ready within 30m"
  fi
  if [ "${RAC_TEST17_SOURCE_BASTION_MODE:-}" = oci-bastion ]; then
    start_oci_source_tunnel
    source_host=127.0.0.1
    source_port="$source_local_port"
    source_key="$BASTION_SSH_PRIVATE_KEY_FILE"
    source_bastion=""
  fi
  for archive in grid_home.zip db_home.zip; do
    if [ -n "$source_host" ]; then
      source_file="$(remote_ssh "find '$stage_source' -type f -name '$archive' -print -quit")"
      test -n "$source_file" || fail "required archive not found on $source_user@$source_host:$stage_source: $archive"
      echo "rac-stage-loader: streaming $archive from $source_user@$source_host:$source_file"
      remote_ssh "cat '$source_file'" | kubectl exec -i -n "$namespace" rac-stage-loader -c loader -- bash -c "cat > '$stage_mount/$archive'"
    else
      source_file="$(find "$stage_source" -type f -name "$archive" -print -quit)"
      test -n "$source_file" || fail "required archive not found under $stage_source: $archive"
      echo "rac-stage-loader: copying $archive from runner path $source_file"
      kubectl cp "$source_file" "$namespace/rac-stage-loader:$stage_mount/$archive" -c loader
    fi
  done
  kubectl exec -n "$namespace" rac-stage-loader -c loader -- bash -c \
    "test -s '$stage_mount/grid_home.zip' && test -s '$stage_mount/db_home.zip'"
  echo "rac-stage-loader: copied required ZIP files"
  kubectl delete pod rac-stage-loader -n "$namespace" --ignore-not-found=true --wait=true
}

prepare_test_secrets() {
  local ssh_private="$work_dir/ssh-privkey"
  local ssh_public="$work_dir/ssh-pubkey"
  local db_key="$work_dir/db-key.pem"
  local db_public="$work_dir/db-key.pub"
  local db_password="$work_dir/db-password"
  local db_encrypted="$work_dir/db-password.enc"

  echo "rac/test17: ensuring RAC prerequisite secrets in namespace $namespace"
  if ! kubectl get secret ssh-key-secret -n "$namespace" >/dev/null 2>&1; then
    ssh-keygen -q -t rsa -b 2048 -N '' -f "$ssh_private"
    # Some runner images do not leave the derived public-key file at the
    # requested path.  Derive it explicitly before creating the Secret.
    if [ ! -s "$ssh_public" ]; then
      ssh-keygen -y -f "$ssh_private" >"$ssh_public"
    fi
    test -s "$ssh_public" || fail "failed to generate $ssh_public"
    kubectl create secret generic ssh-key-secret -n "$namespace" \
      --from-file=ssh-privkey="$ssh_private" --from-file=ssh-pubkey="$ssh_public"
  fi

  if ! kubectl get secret db-user-pass -n "$namespace" >/dev/null 2>&1 ||
     ! kubectl get secret tde-user-pass -n "$namespace" >/dev/null 2>&1; then
    openssl genrsa -out "$db_key" 2048 >/dev/null 2>&1
    openssl rsa -in "$db_key" -pubout -out "$db_public" >/dev/null 2>&1
    printf '%s' 'Oracle_23ai' >"$db_password"
    openssl pkeyutl -in "$db_password" -out "$db_encrypted" -pubin \
      -inkey "$db_public" -encrypt -pkeyopt rsa_padding_mode:oaep \
      -pkeyopt rsa_oaep_md:sha256 -pkeyopt rsa_mgf1_md:sha256
    if ! kubectl get secret db-user-pass -n "$namespace" >/dev/null 2>&1; then
      kubectl create secret generic db-user-pass -n "$namespace" \
        --from-file=key.pem="$db_key" --from-file=pwdfile.enc="$db_encrypted"
    fi
    if ! kubectl get secret tde-user-pass -n "$namespace" >/dev/null 2>&1; then
      kubectl create secret generic tde-user-pass -n "$namespace" \
        --from-file=key.pem="$db_key" --from-file=pwdfile.enc="$db_encrypted"
    fi
  fi
}

ensure_worker_label() {
  if [ -z "$worker_node" ]; then
    worker_node="$(kubectl get nodes -l node-role.kubernetes.io/worker -o name | sed -n '1p;2q')"
  fi
  [ -n "$worker_node" ] || fail "no worker node available for RAC scheduling"
  echo "rac/test17: labeling $worker_node with $worker_label_key=$worker_label_value"
  kubectl label "$worker_node" "$worker_label_key=$worker_label_value" --overwrite
}

ensure_multus_networks() {
  for nad in "$nad_one" "$nad_two"; do
    kubectl get network-attachment-definition "$nad" -n "$namespace" >/dev/null ||
      fail "required Multus NAD is missing: $namespace/$nad; provision the two RAC private networks before running rac/test17"
  done
}

echo "rac/test17: validating prerequisites in namespace $namespace"
kubectl get namespace "$namespace" >/dev/null || fail "namespace does not exist: $namespace"
kubectl get storageclass "$asm_storage_class" >/dev/null || fail "ASM StorageClass is missing: $asm_storage_class"
kubectl get storageclass "$software_storage_class" >/dev/null || fail "software StorageClass is missing: $software_storage_class"
kubectl get storageclass "$stage_storage_class" >/dev/null || fail "stage StorageClass is missing: $stage_storage_class"
echo "rac/test17: ensuring OpenShift RAC service account and SCC binding"
kubectl create serviceaccount "$service_account" -n "$namespace" --dry-run=client -o yaml | kubectl apply -f -
kubectl create clusterrolebinding "rac-prov-sa-privileged-$namespace" \
  --clusterrole=system:openshift:scc:privileged \
  --serviceaccount="$namespace:$service_account" \
  --dry-run=client -o yaml | kubectl apply -f -
mkdir -p "$work_dir"
echo "rac/test17: applying Multus RBAC from $multus_rbac"
kubectl apply -f "$multus_rbac"
ensure_multus_networks
prepare_test_secrets
ensure_worker_label
prepare_stage_pvc
kubectl get pvc "$stage_pvc" -n "$namespace" >/dev/null || fail "software staging PVC is missing: $namespace/$stage_pvc"

render_rac_manifest "$manifest" "$rendered"
kubectl apply --dry-run=server -f "$rendered" >/dev/null ||
  fail "rendered RAC manifest failed server-side validation: $rendered"

echo "rac/test17: applying $rendered"
kubectl apply -f "$rendered"

node_name="${RAC_TEST17_NODE_NAME:-racnode1-0}"
echo "rac/test17: waiting for RAC pod $node_name to be created"
kubectl wait --for=create "pod/$node_name" -n "$namespace" --timeout="$timeout"
# The RAC image's GI CVU probes these standard paths before it can become
# Ready.  Some slim image layers omit the empty directories, and the probe can
# block on the overlay filesystem while resolving the missing paths.  Create
# them as soon as the pod is running, before waiting for readiness.
echo "rac/test17: preparing standard sysctl directories in pod $node_name"
kubectl wait --for=jsonpath='{.status.phase}'=Running "pod/$node_name" -n "$namespace" --timeout="$timeout"
kubectl exec -n "$namespace" "$node_name" -- bash -c \
  'mkdir -p /etc/sysctl.d /run/sysctl.d /usr/local/lib/sysctl.d'
echo "rac/test17: waiting for RAC pod $node_name to become Ready"
kubectl wait --for=condition=ready pod "$node_name" -n "$namespace" --timeout="$timeout"

echo "rac/test17: verifying Oracle RAC readiness in pod $node_name"
marker_deadline=$((SECONDS + $(timeout_seconds "$timeout")))
while (( SECONDS < marker_deadline )); do
  if kubectl exec -n "$namespace" "$node_name" -- bash -c \
    'test -f /tmp/orod/oracle_db_setup.log && grep -F "ORACLE RAC DATABASE IS READY TO USE" /tmp/orod/oracle_db_setup.log' \
    >/dev/null 2>&1; then
    echo "rac/test17: Oracle RAC readiness marker found"
    break
  fi
  echo "rac/test17: Oracle RAC readiness marker not present yet; waiting"
  sleep 30
done
kubectl exec -n "$namespace" "$node_name" -- bash -c \
  'test -f /tmp/orod/oracle_db_setup.log && grep -F "ORACLE RAC DATABASE IS READY TO USE" /tmp/orod/oracle_db_setup.log' \
  >/dev/null 2>&1 || fail "RAC readiness marker was not found in /tmp/orod/oracle_db_setup.log"

status_timeout="${RAC_TEST17_STATUS_TIMEOUT:-10m}"
status_deadline=$((SECONDS + $(timeout_seconds "$status_timeout")))
rac_state=""
db_state=""
role=""
version=""
while (( SECONDS < status_deadline )); do
  rac_state="$(kubectl get racdatabase "$name" -n "$namespace" -o jsonpath='{.status.state}' 2>/dev/null || true)"
  db_state="$(kubectl get racdatabase "$name" -n "$namespace" -o jsonpath='{.status.dbState}' 2>/dev/null || true)"
  role="$(kubectl get racdatabase "$name" -n "$namespace" -o jsonpath='{.status.role}' 2>/dev/null || true)"
  version="$(kubectl get racdatabase "$name" -n "$namespace" -o jsonpath='{.status.releaseUpdate}' 2>/dev/null || true)"

  if [[ "$rac_state" == AVAILABLE ]] &&
     rac_status_value_is_usable "$db_state" &&
     rac_status_value_is_usable "$role" &&
     rac_status_value_is_usable "$version"; then
    echo "rac/test17: RAC database status is healthy: state=$rac_state dbState=$db_state role=$role version=$version"
    break
  fi

  echo "rac/test17: waiting for RacDatabase status: state=${rac_state:-NOTAVAILABLE} dbState=${db_state:-NOTAVAILABLE} role=${role:-NOTAVAILABLE} version=${version:-NOTAVAILABLE}"
  sleep 30
done

if [[ "$rac_state" != AVAILABLE ]] ||
   ! rac_status_value_is_usable "$db_state" ||
   ! rac_status_value_is_usable "$role" ||
   ! rac_status_value_is_usable "$version"; then
  kubectl get racdatabase "$name" -n "$namespace" -o wide || true
  kubectl get racdatabase "$name" -n "$namespace" -o yaml || true
  fail "RAC database did not reach a usable status: state=${rac_state:-NOTAVAILABLE} dbState=${db_state:-NOTAVAILABLE} role=${role:-NOTAVAILABLE} version=${version:-NOTAVAILABLE}"
fi

kubectl get racdatabase "$name" -n "$namespace" -o wide
echo "rac/test17: PASS"
