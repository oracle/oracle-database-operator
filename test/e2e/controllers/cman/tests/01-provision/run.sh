#!/usr/bin/env bash
# cman/test01: single-cluster SIDB + ConnectionManager (file-mode next_hop).
# Uses kind ConnectionManager (TrafficManager type=cman is deprecated on master).
set -Eeuo pipefail

namespace="${CMAN_E2E_NAMESPACE:-cman-ns}"
pipeline="${CI_PIPELINE_ID:-local}"
timeout="${CMAN_E2E_TIMEOUT_SECONDS:-1800}"
poll="${CMAN_E2E_POLL_INTERVAL:-20}"
storage_class="${SIDB_STORAGE_CLASS:-oci-bv}"
sidb_image="${SIDB_IMAGE:?Set SIDB_IMAGE as an environment variable}"
cman_image="${CMAN_E2E_IMAGE:?Set CMAN_E2E_IMAGE as an environment variable}"
sidb_name="${CMAN_E2E_SIDB_NAME:-sidb-e2e-cman-primary}"
cman_name="${CMAN_E2E_NAME:-cman-e2e-sidb}"
admin_secret="${CMAN_E2E_ADMIN_SECRET:-sidb-e2e-cman-admin}"
node_port="${CMAN_E2E_NODE_PORT:-30189}"
results_dir="${OPERATOR_LOG_DIR:-test/e2e/controllers/cman/results}/test01-${namespace}"

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
  e2e.oracle.com/controller=cman e2e.oracle.com/test-id=test01 \
  e2e.oracle.com/pipeline="$pipeline" --overwrite >/dev/null

admin_password="${CMAN_E2E_ADMIN_PASSWORD:-}"
if [ -z "$admin_password" ]; then
  admin_password="E2e_$(openssl rand -hex 12)"
fi
k -n "$namespace" create secret generic "$admin_secret" \
  --from-literal=oracle_pwd="$admin_password" --dry-run=client -o yaml |
  tee "$results_dir/secret-manifests/admin-secret.yaml" | k -n "$namespace" apply -f - >/dev/null

# SIDB primary (TCP only). CMAN file-mode next_hop routes to this Service.
cat > "$results_dir/manifests/sidb-primary.yaml" <<EOF
apiVersion: database.oracle.com/v4
kind: SingleInstanceDatabase
metadata:
  name: $sidb_name
  labels:
    e2e.oracle.com/owner: operator-e2e
    e2e.oracle.com/controller: cman
    e2e.oracle.com/test-id: test01
    e2e.oracle.com/pipeline: "$pipeline"
spec:
  sid: ORCLCMA
  pdbName: ORCLPDB1
  createAs: primary
  edition: enterprise
  archiveLog: true
  security:
    secrets:
      admin:
        secretName: $admin_secret
        secretKey: oracle_pwd
        keepSecret: true
  image:
    pullFrom: "$sidb_image"
    prebuiltDB: false
    imagePullPolicy: IfNotPresent
  persistence:
    oradata:
      size: 50Gi
      storageClass: "$storage_class"
      accessMode: ReadWriteOnce
    setWritePermissions: true
  resources:
    requests:
      memory: "8Gi"
      cpu: "2"
    limits:
      memory: "16Gi"
      cpu: "4"
  initParams:
    sgaTarget: 3072
    pgaAggregateTarget: 1024
    cpuCount: 2
    processes: 300
  replicas: 1
EOF

# ConnectionManager file-mode with global next_hop to the SIDB Service.
# Clients reach CMAN; CMAN forwards to sidb Service:1521 (no remote_listener required).
cat > "$results_dir/manifests/cman-configmap.yaml" <<EOF
apiVersion: v1
kind: ConfigMap
metadata:
  name: ${cman_name}-filemode-config
  labels:
    e2e.oracle.com/owner: operator-e2e
    e2e.oracle.com/controller: cman
    e2e.oracle.com/test-id: test01
data:
  cman.ora: |
    CMAN_${cman_name}.${namespace}.svc.cluster.local =
    (configuration=
      (address=(protocol=tcp)(host=${cman_name}.${namespace}.svc.cluster.local)(port=1521))
      (next_hop=
        (description=
          (address=(protocol=tcp)(host=${sidb_name}.${namespace}.svc.cluster.local)(port=1521))
        )
      )
      (parameter_list=
        (connection_statistics=yes)
        (log_directory=/u01/app/oracle/product/23ai/client_1/network/log)
        (log_level=user)
        (trace_directory=/u01/app/oracle/product/23ai/client_1/network/trace)
        (trace_level=user)
        (valid_node_checking_registration=on)
        (registration_invited_nodes=*)
      )
      (rule_list=
        (rule=
          (src=*)(dst=*)(srv=*)(act=accept)
          (action_list=(aut=off)(moct=0)(mct=0)(mit=0)(conn_stats=on))
        )
      )
    )
EOF

cat > "$results_dir/manifests/cman.yaml" <<EOF
apiVersion: network.oracle.com/v4
kind: ConnectionManager
metadata:
  name: $cman_name
  labels:
    e2e.oracle.com/owner: operator-e2e
    e2e.oracle.com/controller: cman
    e2e.oracle.com/test-id: test01
    e2e.oracle.com/pipeline: "$pipeline"
spec:
  runtime:
    image: "$cman_image"
    imagePullPolicy: Always
    replicas: 1
  service:
    internal:
      enabled: true
      ports:
        - name: cman
          port: 1521
          targetPort: 1521
    external:
      enabled: true
      serviceType: NodePort
      externalTrafficPolicy: Cluster
      ports:
        - name: cman
          port: 1521
          targetPort: 1521
          nodePort: $node_port
  configSource:
    configMapRef:
      name: ${cman_name}-filemode-config
      key: cman.ora
EOF

wait_sidb() {
  local name="$1" elapsed=0 status role
  while [ "$elapsed" -lt "$timeout" ]; do
    status="$(k -n "$namespace" get sidb "$name" -o jsonpath='{.status.status}' 2>/dev/null || true)"
    role="$(k -n "$namespace" get sidb "$name" -o jsonpath='{.status.role}' 2>/dev/null || true)"
    echo "cman/test01 SIDB readiness: name=$name elapsed=${elapsed}s status=${status:-<empty>} role=${role:-<empty>}"
    case "$status" in
      Failed|FAILED|Error|ERROR) return 1 ;;
      Ready|READY|Healthy|HEALTHY|Available|AVAILABLE)
        [ "$role" = "PRIMARY" ] && return 0
        ;;
    esac
    sleep "$poll"
    elapsed=$((elapsed + poll))
  done
  echo "cman/test01 SIDB timed out: name=$name status=${status:-<empty>}" >&2
  k -n "$namespace" get sidb "$name" -o yaml >&2 || true
  return 1
}

wait_cman() {
  local name="$1" elapsed=0 status ready
  while [ "$elapsed" -lt "$timeout" ]; do
    status="$(k -n "$namespace" get connectionmanager "$name" -o jsonpath='{.status.status}' 2>/dev/null || true)"
    ready="$(k -n "$namespace" get deployment "$name" -o jsonpath='{.status.readyReplicas}' 2>/dev/null || true)"
    echo "cman/test01 ConnectionManager readiness: name=$name elapsed=${elapsed}s status=${status:-<empty>} readyReplicas=${ready:-0}"
    case "$status" in
      Failed|FAILED|Error|ERROR) return 1 ;;
      Ready|READY|Healthy|HEALTHY)
        [ "${ready:-0}" -ge 1 ] && return 0
        ;;
    esac
    sleep "$poll"
    elapsed=$((elapsed + poll))
  done
  echo "cman/test01 ConnectionManager timed out: name=$name status=${status:-<empty>}" >&2
  k -n "$namespace" get connectionmanager "$name" -o yaml >&2 || true
  k -n "$namespace" get pods,svc,deploy -l "app.kubernetes.io/name=$name" -o wide >&2 || true
  return 1
}

ensure_connectionmanager_api() {
  local elapsed=0 est names short_names

  # Older TrafficManager CRDs claimed shortNames cman/connectionmanager and
  # blocked ConnectionManager from becoming Established. Prefer trm only.
  if k get crd trafficmanagers.network.oracle.com >/dev/null 2>&1; then
    short_names="$(k get crd trafficmanagers.network.oracle.com -o jsonpath='{.spec.names.shortNames}' 2>/dev/null || true)"
    case "$short_names" in
      *cman*|*connectionmanager*)
        echo "cman/test01: stripping conflicting shortNames from trafficmanagers CRD (keep trm)"
        k get crd trafficmanagers.network.oracle.com -o json |
          jq '.spec.names.shortNames=["trm"]' |
          k apply -f - >/dev/null
        ;;
    esac
  fi

  if ! k get crd connectionmanagers.network.oracle.com >/dev/null 2>&1; then
    echo "cman/test01: ConnectionManager CRD is missing; apply operator CRDs first" >&2
    return 1
  fi

  # Avoid nested double-quotes inside "$(...)" around the jq program; that
  # caused: unexpected EOF while looking for matching ')' after SIDB Healthy.
  while [ "$elapsed" -lt 120 ]; do
    est=$(k get crd connectionmanagers.network.oracle.com -o json |
      jq -r '[.status.conditions[]? | select(.type=="Established") | .status] | first // "False"')
    names=$(k get crd connectionmanagers.network.oracle.com -o json |
      jq -r '[.status.conditions[]? | select(.type=="NamesAccepted") | .status] | first // "False"')
    echo "cman/test01 ConnectionManager CRD: elapsed=${elapsed}s Established=$est NamesAccepted=$names"
    if [ "$est" = "True" ] && [ "$names" = "True" ]; then
      return 0
    fi
    sleep 5
    elapsed=$((elapsed + 5))
  done
  echo "cman/test01: ConnectionManager CRD did not become Established/NamesAccepted" >&2
  k get crd connectionmanagers.network.oracle.com -o yaml >&2 || true
  return 1
}

echo "cman/test01: applying SIDB primary $sidb_name"
k -n "$namespace" apply -f "$results_dir/manifests/sidb-primary.yaml"
wait_sidb "$sidb_name"

echo "cman/test01: ensuring ConnectionManager API is discoverable"
ensure_connectionmanager_api

echo "cman/test01: applying ConnectionManager $cman_name (file-mode next_hop)"
k -n "$namespace" apply -f "$results_dir/manifests/cman-configmap.yaml"
k -n "$namespace" apply -f "$results_dir/manifests/cman.yaml"
wait_cman "$cman_name"

# Basic topology asserts
k -n "$namespace" get svc "$cman_name" >/dev/null
k -n "$namespace" get svc "${cman_name}-ext" >/dev/null
ext_type="$(k -n "$namespace" get svc "${cman_name}-ext" -o jsonpath='{.spec.type}')"
[ "$ext_type" = "NodePort" ] || {
  echo "cman/test01: expected external Service type NodePort, got ${ext_type:-<empty>}" >&2
  exit 1
}
k -n "$namespace" wait --for=condition=Ready pod -l "app=$sidb_name" --timeout=120s >/dev/null
k -n "$namespace" get sidb,connectionmanager,pods,svc,pvc -o yaml > "$results_dir/resources.yaml" 2>&1 || true

echo "cman/test01 assertions passed: sidb=$sidb_name cman=$cman_name namespace=$namespace nodePort=$node_port"