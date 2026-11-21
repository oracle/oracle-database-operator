#!/usr/bin/env bash

orestart_case_init() {
  # This cluster has one worker. Reuse the dedicated E2E namespace so a
  # canceled job cannot strand a per-pipeline database and consume capacity.
  : "${ORESTART_NAMESPACE:=orestart-e2e}"
  : "${ORESTART_NAME:=oraclerestart-e2e}"
  : "${ORESTART_INSTANCE_NAME:=dbmc1}"
  if [ -z "${ORESTART_WORKER_NODE:-}" ]; then
    ORESTART_WORKER_NODE="$(orestart_kubectl get nodes --no-headers \
      | awk '$2 == "Ready" {print $1; exit}')"
  fi
  : "${ORESTART_WORKER_NODE:=}"
  : "${ORESTART_HOST_SW_LOCATION:=/scratch/orestart/}"
  : "${ORESTART_HOST_SW_STAGE_LOCATION:=/scratch/software/19c/1928GoldImage}"
  : "${ORESTART_IMAGE:?Set ORESTART_IMAGE as an environment variable}"
  : "${ORESTART_DB_NAME:=PORCLCDB}"
  : "${ORESTART_SERVICE_NAME:=soepdb}"
  : "${ORESTART_TIMEOUT_SECONDS:=10800}"
  : "${ORESTART_DELETE_TIMEOUT_SECONDS:=1800}"
  : "${ORESTART_POLL_INTERVAL_SECONDS:=30}"
  : "${ORESTART_RESULTS_DIR:=${ORESTART_E2E_RESULTS_DIR:-${OPERATOR_LOG_DIR:-test/e2e/controllers/oraclerestart/results}}/01-provision-${ORESTART_NAMESPACE}}"
  : "${ORESTART_MANIFEST:=$ORESTART_RESULTS_DIR/oraclerestart-rendered.yaml}"
  : "${ORESTART_SOURCE_MANIFEST:=${ORESTART_CASE_SCRIPT_DIR:-$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)}/test01.yaml}"
  : "${ORESTART_NAMESPACE_CREATED:=false}"
  # The dedicated E2E runner owns this stable namespace.  Reuse it between
  # runs and verify Oracle Restart CR deletion instead of waiting for the
  # namespace finalizer to remove the namespace itself.
  : "${ORESTART_DELETE_NAMESPACE:=false}"
  : "${ORESTART_CASE_CLEANUP:=true}"
  : "${ORESTART_DB_SECRET_NAME:=db-user-pass-pkutl}"
  : "${ORESTART_SSH_SECRET_NAME:=ssh-key-secret}"
  : "${ORESTART_TDE_SECRET_NAME:=tde-user-pass-pkutl}"
  : "${ORESTART_SECRET_SOURCE_NAMESPACE:=orestart}"
  : "${ORESTART_COPY_SECRETS:=false}"
  export ORESTART_NAMESPACE ORESTART_NAME ORESTART_INSTANCE_NAME ORESTART_WORKER_NODE
  export ORESTART_HOST_SW_LOCATION ORESTART_HOST_SW_STAGE_LOCATION ORESTART_IMAGE
  export ORESTART_DB_NAME ORESTART_SERVICE_NAME ORESTART_TIMEOUT_SECONDS
  export ORESTART_DELETE_TIMEOUT_SECONDS ORESTART_POLL_INTERVAL_SECONDS ORESTART_RESULTS_DIR
  export ORESTART_MANIFEST ORESTART_SOURCE_MANIFEST ORESTART_NAMESPACE_CREATED ORESTART_DELETE_NAMESPACE ORESTART_CASE_CLEANUP
  export ORESTART_DB_SECRET_NAME ORESTART_SSH_SECRET_NAME ORESTART_TDE_SECRET_NAME
  export ORESTART_SECRET_SOURCE_NAMESPACE ORESTART_COPY_SECRETS
  echo "Oracle Restart E2E: selected worker node ${ORESTART_WORKER_NODE:-<none>}"
}

orestart_kubectl() {
  if [ -n "${KUBE_CONTEXT_NAME:-}" ]; then
    command kubectl --context "$KUBE_CONTEXT_NAME" "$@"
  else
    command kubectl "$@"
  fi
}

orestart_require() {
  command -v "$1" >/dev/null 2>&1 || {
    echo "Oracle Restart E2E: required command is unavailable: $1" >&2
    return 2
  }
}

orestart_validate() {
  orestart_require kubectl
  orestart_require sed
  orestart_require jq
  for variable_name in ORESTART_WORKER_NODE ORESTART_HOST_SW_LOCATION ORESTART_HOST_SW_STAGE_LOCATION ORESTART_IMAGE; do
    [ -n "${!variable_name:-}" ] || {
      echo "Oracle Restart E2E: $variable_name is required" >&2
      return 2
    }
  done
  if [ -r "${ORESTART_DB_KEY_FILE:-}" ] && [ -r "${ORESTART_DB_PASSWORD_FILE:-}" ] &&
     [ -r "${ORESTART_SSH_PRIVATE_KEY_FILE:-}" ] && [ -r "${ORESTART_SSH_PUBLIC_KEY_FILE:-}" ] &&
     [ -r "${ORESTART_TDE_KEY_FILE:-}" ] && [ -r "${ORESTART_TDE_PASSWORD_FILE:-}" ]; then
    ORESTART_COPY_SECRETS=false
    export ORESTART_COPY_SECRETS
    return 0
  fi
  for secret_name in "$ORESTART_DB_SECRET_NAME" "$ORESTART_SSH_SECRET_NAME" "$ORESTART_TDE_SECRET_NAME"; do
    orestart_kubectl -n "$ORESTART_SECRET_SOURCE_NAMESPACE" get secret "$secret_name" >/dev/null 2>&1 || {
      echo "Oracle Restart E2E: missing files and source secret $ORESTART_SECRET_SOURCE_NAMESPACE/$secret_name" >&2
      return 2
    }
  done
  ORESTART_COPY_SECRETS=true
  export ORESTART_COPY_SECRETS
  echo "Oracle Restart E2E: reusing prerequisite secrets from namespace $ORESTART_SECRET_SOURCE_NAMESPACE"
}

orestart_copy_secret() {
  local secret_name="$1"
  echo "Oracle Restart E2E: copying prerequisite secret $ORESTART_SECRET_SOURCE_NAMESPACE/$secret_name to $ORESTART_NAMESPACE"
  orestart_kubectl -n "$ORESTART_SECRET_SOURCE_NAMESPACE" get secret "$secret_name" -o json |
    jq --arg namespace "$ORESTART_NAMESPACE" '
      del(.metadata.creationTimestamp, .metadata.resourceVersion, .metadata.uid,
          .metadata.managedFields, .metadata.ownerReferences)
      | .metadata.namespace = $namespace
    ' |
    orestart_kubectl apply --request-timeout=120s -f - >/dev/null
  echo "Oracle Restart E2E: copied prerequisite secret $secret_name"
}

orestart_render() {
  local source="$1" destination="$2"
  mkdir -p "$(dirname "$destination")"
  sed \
    -e "s|__ORESTART_NAMESPACE__|$ORESTART_NAMESPACE|g" \
    -e "s|__ORESTART_NAME__|$ORESTART_NAME|g" \
    -e "s|__ORESTART_INSTANCE_NAME__|$ORESTART_INSTANCE_NAME|g" \
    -e "s|__ORESTART_WORKER_NODE__|$ORESTART_WORKER_NODE|g" \
    -e "s|__ORESTART_HOST_SW_LOCATION__|$ORESTART_HOST_SW_LOCATION|g" \
    -e "s|__ORESTART_HOST_SW_STAGE_LOCATION__|$ORESTART_HOST_SW_STAGE_LOCATION|g" \
    -e "s|__ORESTART_IMAGE__|$ORESTART_IMAGE|g" \
    -e "s|__ORESTART_DB_NAME__|$ORESTART_DB_NAME|g" \
    -e "s|__ORESTART_SERVICE_NAME__|$ORESTART_SERVICE_NAME|g" \
    -e "s|__ORESTART_DB_SECRET_NAME__|$ORESTART_DB_SECRET_NAME|g" \
    -e "s|__ORESTART_SSH_SECRET_NAME__|$ORESTART_SSH_SECRET_NAME|g" \
    -e "s|__ORESTART_TDE_SECRET_NAME__|$ORESTART_TDE_SECRET_NAME|g" \
    "$source" > "$destination"
}

orestart_snapshot() {
  local reason="${1:-snapshot}" directory="$ORESTART_RESULTS_DIR/snapshots/${1:-snapshot}"
  mkdir -p "$directory"
  orestart_kubectl -n "$ORESTART_NAMESPACE" get oracleRestart "$ORESTART_NAME" -o yaml > "$directory/oraclerestart.yaml" 2>&1 || true
  orestart_kubectl -n "$ORESTART_NAMESPACE" get pods,statefulsets,pvc,services -o wide > "$directory/workloads.txt" 2>&1 || true
  orestart_kubectl -n "$ORESTART_NAMESPACE" get events --sort-by=.lastTimestamp > "$directory/events.txt" 2>&1 || true
  orestart_kubectl -n "$ORESTART_NAMESPACE" get secret > "$directory/secrets.txt" 2>&1 || true
}

orestart_wait_for_available() {
  local elapsed=0 state db_state role pod_state
  while [ "$elapsed" -lt "$ORESTART_TIMEOUT_SECONDS" ]; do
    state="$(orestart_kubectl -n "$ORESTART_NAMESPACE" get oracleRestart "$ORESTART_NAME" -o jsonpath='{.status.state}' 2>/dev/null || true)"
    db_state="$(orestart_kubectl -n "$ORESTART_NAMESPACE" get oracleRestart "$ORESTART_NAME" -o jsonpath='{.status.dbState}' 2>/dev/null || true)"
    role="$(orestart_kubectl -n "$ORESTART_NAMESPACE" get oracleRestart "$ORESTART_NAME" -o jsonpath='{.status.role}' 2>/dev/null || true)"
    pod_state="$(orestart_kubectl -n "$ORESTART_NAMESPACE" get oracleRestart "$ORESTART_NAME" -o jsonpath='{.status.oracleRestartNodes[0].podState}' 2>/dev/null || true)"
    echo "Oracle Restart readiness: elapsed=${elapsed}s state=${state:-<empty>} dbState=${db_state:-<empty>} role=${role:-<empty>} podState=${pod_state:-<empty>}"
    if [ "$state" = AVAILABLE ] && [ "$db_state" = OPEN ] && [ "$role" = PRIMARY ]; then
      orestart_kubectl -n "$ORESTART_NAMESPACE" wait --for=condition=ready pod/${ORESTART_INSTANCE_NAME}-0 --timeout=60s
      return 0
    fi
    case "$state" in FAILED|DELETE_ERROR|PODFAILURE|STATEFULSETFAILURE)
      orestart_snapshot failed
      echo "Oracle Restart entered terminal failure state: $state" >&2
      return 1
      ;;
    esac
    sleep "$ORESTART_POLL_INTERVAL_SECONDS"
    elapsed=$((elapsed + ORESTART_POLL_INTERVAL_SECONDS))
  done
  orestart_snapshot timeout
  echo "Oracle Restart did not become AVAILABLE within ${ORESTART_TIMEOUT_SECONDS}s" >&2
  return 1
}

orestart_clean_asm_devices() {
  local statefulset="${ORESTART_INSTANCE_NAME}" cleaner="orestart-asm-cleaner"
  local elapsed=0 pvc_names pvc1 pvc2 phase

  while [ "$elapsed" -lt 900 ]; do
    pvc_names="$(orestart_kubectl -n "$ORESTART_NAMESPACE" get pvc -o json |
      jq -r '.items[].metadata.name | select(startswith("asm-pvc-"))' | sort)"
    pvc1="$(printf '%s\n' "$pvc_names" | sed -n '1p')"
    pvc2="$(printf '%s\n' "$pvc_names" | sed -n '2p')"
    if [ -n "$pvc1" ] && [ -n "$pvc2" ]; then
      phase="$(orestart_kubectl -n "$ORESTART_NAMESPACE" get pvc "$pvc1" -o jsonpath='{.status.phase}')"
      if [ "$phase" = Bound ]; then
        phase="$(orestart_kubectl -n "$ORESTART_NAMESPACE" get pvc "$pvc2" -o jsonpath='{.status.phase}')"
        [ "$phase" = Bound ] && break
      fi
    fi
    sleep 15
    elapsed=$((elapsed + 15))
  done
  [ -n "$pvc1" ] && [ -n "$pvc2" ] && [ "$phase" = Bound ] || {
    echo "Oracle Restart E2E: ASM PVCs were not ready for header cleanup" >&2
    return 1
  }

  orestart_kubectl -n "$ORESTART_NAMESPACE" scale statefulset "$statefulset" --replicas=0 >/dev/null
  elapsed=0
  while orestart_kubectl -n "$ORESTART_NAMESPACE" get pod "${statefulset}-0" >/dev/null 2>&1; do
    [ "$elapsed" -lt 300 ] || {
      orestart_kubectl -n "$ORESTART_NAMESPACE" scale statefulset "$statefulset" --replicas=1 >/dev/null || true
      echo "Oracle Restart E2E: ${statefulset}-0 did not stop before ASM cleanup" >&2
      return 1
    }
    sleep 10
    elapsed=$((elapsed + 10))
  done

  orestart_kubectl -n "$ORESTART_NAMESPACE" delete pod "$cleaner" --ignore-not-found --wait=true >/dev/null 2>&1 || true
  cat <<EOF | orestart_kubectl apply -f - >/dev/null
apiVersion: v1
kind: Pod
metadata:
  name: ${cleaner}
  namespace: ${ORESTART_NAMESPACE}
  labels:
    e2e.oracle.com/owner: operator-e2e
    e2e.oracle.com/controller: oraclerestart
    e2e.oracle.com/purpose: asm-header-cleanup
spec:
  nodeName: ${ORESTART_WORKER_NODE}
  restartPolicy: Never
  containers:
    - name: cleaner
      image: ${ORESTART_IMAGE}
      imagePullPolicy: IfNotPresent
      command: ["/bin/bash", "-c"]
      args:
        - >-
          set -eux;
          rm -rf /host-software/*;
          for disk in /dev/asm-disk1 /dev/asm-disk2; do
            wipefs -a "\$disk";
            dd if=/dev/zero of="\$disk" bs=8k count=10000 conv=fsync;
          done
      securityContext:
        privileged: true
        runAsUser: 0
      volumeDevices:
        - name: asm-disk1
          devicePath: /dev/asm-disk1
        - name: asm-disk2
          devicePath: /dev/asm-disk2
      volumeMounts:
        - name: software
          mountPath: /host-software
  volumes:
    - name: asm-disk1
      persistentVolumeClaim:
        claimName: ${pvc1}
    - name: asm-disk2
      persistentVolumeClaim:
        claimName: ${pvc2}
    - name: software
      hostPath:
        path: ${ORESTART_HOST_SW_LOCATION}
        type: DirectoryOrCreate
EOF
  if ! orestart_kubectl -n "$ORESTART_NAMESPACE" wait --for=jsonpath='{.status.phase}'=Succeeded pod/${cleaner} --timeout=300s; then
    orestart_kubectl -n "$ORESTART_NAMESPACE" logs pod/${cleaner} || true
    orestart_kubectl -n "$ORESTART_NAMESPACE" scale statefulset "$statefulset" --replicas=1 >/dev/null || true
    return 1
  fi
  orestart_kubectl -n "$ORESTART_NAMESPACE" delete pod "$cleaner" --wait=true >/dev/null
  orestart_kubectl -n "$ORESTART_NAMESPACE" scale statefulset "$statefulset" --replicas=1 >/dev/null
  echo "Oracle Restart E2E: software path and ASM headers cleaned"
}

orestart_cleanup() {
  [ "$ORESTART_CASE_CLEANUP" = true ] || return 0
  orestart_snapshot before-cleanup
  orestart_kubectl -n "$ORESTART_NAMESPACE" delete oracleRestart "$ORESTART_NAME" \
    --ignore-not-found --wait=true --timeout="${ORESTART_DELETE_TIMEOUT_SECONDS}s" || true
  local elapsed=0
  while [ "$elapsed" -lt "$ORESTART_DELETE_TIMEOUT_SECONDS" ]; do
    if ! orestart_kubectl -n "$ORESTART_NAMESPACE" get oracleRestart "$ORESTART_NAME" >/dev/null 2>&1; then
      echo "Oracle Restart CR deleted: $ORESTART_NAMESPACE/$ORESTART_NAME"
      break
    fi
    sleep "$ORESTART_POLL_INTERVAL_SECONDS"
    elapsed=$((elapsed + ORESTART_POLL_INTERVAL_SECONDS))
  done
  if orestart_kubectl -n "$ORESTART_NAMESPACE" get oracleRestart "$ORESTART_NAME" >/dev/null 2>&1; then
    orestart_snapshot delete-timeout
    echo "Oracle Restart CR was not deleted before timeout" >&2
    return 1
  fi
  echo "Oracle Restart E2E: removing stale prerequisite secrets from $ORESTART_NAMESPACE"
  # Secrets are not part of the Oracle Restart teardown gate. Do not let a
  # stuck secret finalizer block the next run from applying fresh copies.
  orestart_kubectl -n "$ORESTART_NAMESPACE" delete secret \
    "$ORESTART_DB_SECRET_NAME" "$ORESTART_SSH_SECRET_NAME" "$ORESTART_TDE_SECRET_NAME" \
    --ignore-not-found --wait=false --timeout=60s >/dev/null 2>&1 || true
  echo "Oracle Restart E2E: stale prerequisite secret cleanup requested"
  if [ "$ORESTART_DELETE_NAMESPACE" = true ]; then
    orestart_kubectl delete namespace "$ORESTART_NAMESPACE" --ignore-not-found --wait=true \
      --timeout="${ORESTART_DELETE_TIMEOUT_SECONDS}s" >/dev/null 2>&1 || true
  fi
}
