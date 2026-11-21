#!/usr/bin/env bash
set -Eeuo pipefail

script_dir="$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC1091
source "$script_dir/lib.sh"
template_file=""

agentfactory_case_init
test_id="$AGENTFACTORY_CASE_ID"
assert_script="${AGENTFACTORY_ASSERT_SCRIPT:-}"

case "$AGENTFACTORY_IMAGE_PULL_POLICY" in
  Always|IfNotPresent|Never) ;;
  *)
    echo "Agent Factory $test_id: AGENTFACTORY_IMAGE_PULL_POLICY must be Always, IfNotPresent, or Never" >&2
    exit 2
    ;;
esac

[ -n "$assert_script" ] || {
  echo "Agent Factory $test_id: AGENTFACTORY_ASSERT_SCRIPT is required" >&2
  exit 2
}
[ -r "$assert_script" ] || {
  echo "Agent Factory $test_id: assertion script is not readable: $assert_script" >&2
  exit 2
}

[ -n "$AGENTFACTORY_IMAGE" ] || {
  echo "Agent Factory $test_id: AGENTFACTORY_IMAGE (or AGENT_FACTORY_IMAGE) is required" >&2
  exit 2
}
# Cases with AGENTFACTORY_PREPARE_DATABASE_SCRIPT synchronize the administrator
# password from the dependent SIDB admin Secret before applying AgentFactory.
# Keep the password requirement for every other E2E case. Customer controller
# behavior is not affected by this E2E-only exception.
if [ -z "${AGENTFACTORY_BOOTSTRAP_PASSWORD:-}" ] &&
   [ -z "${AGENTFACTORY_PREPARE_DATABASE_SCRIPT:-}" ]; then
  echo "Agent Factory $test_id: AGENTFACTORY_BOOTSTRAP_PASSWORD is required (or SIDB_ADMIN_PASSWORD for the same-cluster dependency)" >&2
  exit 2
fi
SIDB_CASE_NAMESPACE="${SIDB_CASE_NAMESPACE:-${SIDB_E2E_NAMESPACE:-sidb-ns}}"
AGENTFACTORY_DB_HOST="${AGENTFACTORY_DB_HOST:-sidb-e2e-primary.${SIDB_CASE_NAMESPACE}.svc.cluster.local}"
export SIDB_CASE_NAMESPACE AGENTFACTORY_DB_HOST

mkdir -p "$AGENTFACTORY_CASE_RESULTS_DIR"
trap 'agentfactory_case_collect_diagnostics failure || true' ERR

setup_manifest_dir="$AGENTFACTORY_CASE_RESULTS_DIR/setup-manifests"
setup_order_file="$AGENTFACTORY_CASE_RESULTS_DIR/setup-order.txt"
mkdir -p "$setup_manifest_dir"
chmod 700 "$setup_manifest_dir"
: > "$setup_order_file"

printf '%s\n' \
  "apiVersion: v1" \
  "kind: Namespace" \
  "metadata:" \
  "  name: $AGENTFACTORY_CASE_NAMESPACE" \
  > "$setup_manifest_dir/00-namespace.yaml"
if agentfactory_case_kubectl get namespace "$AGENTFACTORY_CASE_NAMESPACE" >/dev/null 2>&1; then
  printf '00 namespace status=existing name=%s\n' "$AGENTFACTORY_CASE_NAMESPACE" >> "$setup_order_file"
else
  agentfactory_case_kubectl apply -f "$setup_manifest_dir/00-namespace.yaml"
  printf '00 namespace status=created name=%s\n' "$AGENTFACTORY_CASE_NAMESPACE" >> "$setup_order_file"
fi
agentfactory_case_kubectl get namespace "$AGENTFACTORY_CASE_NAMESPACE" -o yaml \
  > "$setup_manifest_dir/00-namespace-observed.yaml" 2>&1

if [ "$AGENTFACTORY_STORAGE_MODE" = "dynamicFSS" ]; then
  [ -n "${AGENTFACTORY_PREPARE_STORAGE_SCRIPT:-}" ] || {
    echo "Agent Factory $test_id: AGENTFACTORY_PREPARE_STORAGE_SCRIPT is required for dynamicFSS" >&2
    exit 2
  }
  [ -r "$AGENTFACTORY_PREPARE_STORAGE_SCRIPT" ] || {
    echo "Agent Factory $test_id: storage preparation script is not readable: $AGENTFACTORY_PREPARE_STORAGE_SCRIPT" >&2
    exit 2
  }
  export AGENTFACTORY_SETUP_MANIFEST_DIR="$setup_manifest_dir"
  export AGENTFACTORY_SETUP_ORDER_FILE="$setup_order_file"
  # shellcheck disable=SC1090
  source "$AGENTFACTORY_PREPARE_STORAGE_SCRIPT"
  [ -n "$AGENTFACTORY_STORAGE_CLASS" ] || {
    echo "Agent Factory $test_id: dynamicFSS did not provide a StorageClass" >&2
    exit 2
  }
else
  # The Agent Factory controller requires a volume mounted at /mount. Existing
  # cases use a case-owned PVC so they do not depend on an RWX default class.
  agentfactory_case_kubectl -n "$AGENTFACTORY_CASE_NAMESPACE" delete pvc "$AGENTFACTORY_PVC_NAME" \
    --ignore-not-found=true --wait=true >/dev/null 2>&1 || true
  storage_class_line=""
  if [ -n "$AGENTFACTORY_STORAGE_CLASS" ]; then
    storage_class_line="  storageClassName: $AGENTFACTORY_STORAGE_CLASS\n"
  fi
  printf '%b' \
    "apiVersion: v1\nkind: PersistentVolumeClaim\nmetadata:\n  name: $AGENTFACTORY_PVC_NAME\n  namespace: $AGENTFACTORY_CASE_NAMESPACE\nspec:\n  accessModes:\n  - ReadWriteOnce\n${storage_class_line}  resources:\n    requests:\n      storage: $AGENTFACTORY_PVC_SIZE\n" |
    tee "$setup_manifest_dir/01-pvc.yaml" |
    agentfactory_case_kubectl apply -f -
  printf '01 pvc status=applied name=%s\n' "$AGENTFACTORY_PVC_NAME" >> "$setup_order_file"
fi
agentfactory_case_kubectl -n "$AGENTFACTORY_CASE_NAMESPACE" create secret generic \
  "$AGENTFACTORY_RUNTIME_SECRET" \
  --from-literal="${AGENTFACTORY_UI_ADMIN_PASSWORD_KEY}=${AGENTFACTORY_UI_ADMIN_PASSWORD_VALUE:-AgentFactory-E2e-Aa1}" \
  --from-literal="${AGENTFACTORY_DB_PASSWORD_KEY}=${AGENTFACTORY_DB_PASSWORD:-AgentFactoryDb-E2e-Aa1}" \
  --dry-run=client -o yaml |
  tee "$setup_manifest_dir/02-runtime-secret.yaml" |
  agentfactory_case_kubectl -n "$AGENTFACTORY_CASE_NAMESPACE" apply -f -
agentfactory_case_kubectl -n "$AGENTFACTORY_CASE_NAMESPACE" create secret generic \
  "$AGENTFACTORY_RUNTIME_SECRET" \
  --from-literal="${AGENTFACTORY_UI_ADMIN_PASSWORD_KEY}=${AGENTFACTORY_UI_ADMIN_PASSWORD_VALUE:-AgentFactory-E2e-Aa1}" \
  --from-literal="${AGENTFACTORY_DB_PASSWORD_KEY}=${AGENTFACTORY_DB_PASSWORD:-AgentFactoryDb-E2e-Aa1}" \
  --dry-run=client -o yaml |
  sed -E "s/^  (${AGENTFACTORY_UI_ADMIN_PASSWORD_KEY}|${AGENTFACTORY_DB_PASSWORD_KEY}): .*/  \\1: REDACTED/" \
  | tee "$setup_manifest_dir/02-runtime-secret.redacted.yaml"
printf '02 runtime-secret status=applied name=%s keys=%s,%s redacted_manifest=true full_manifest=true\n' \
  "$AGENTFACTORY_RUNTIME_SECRET" "$AGENTFACTORY_UI_ADMIN_PASSWORD_KEY" "$AGENTFACTORY_DB_PASSWORD_KEY" \
  >> "$setup_order_file"

agentfactory_case_kubectl -n "$AGENTFACTORY_CASE_NAMESPACE" create secret generic \
  "$AGENTFACTORY_BOOTSTRAP_SECRET" \
  --from-literal="${AGENTFACTORY_BOOTSTRAP_USERNAME_KEY}=${AGENTFACTORY_BOOTSTRAP_USERNAME}" \
  --from-literal="${AGENTFACTORY_BOOTSTRAP_PASSWORD_KEY}=${AGENTFACTORY_BOOTSTRAP_PASSWORD}" \
  --dry-run=client -o yaml |
  tee "$setup_manifest_dir/03-bootstrap-secret.yaml" |
  agentfactory_case_kubectl -n "$AGENTFACTORY_CASE_NAMESPACE" apply -f -
agentfactory_case_kubectl -n "$AGENTFACTORY_CASE_NAMESPACE" create secret generic \
  "$AGENTFACTORY_BOOTSTRAP_SECRET" \
  --from-literal="${AGENTFACTORY_BOOTSTRAP_USERNAME_KEY}=${AGENTFACTORY_BOOTSTRAP_USERNAME}" \
  --from-literal="${AGENTFACTORY_BOOTSTRAP_PASSWORD_KEY}=${AGENTFACTORY_BOOTSTRAP_PASSWORD}" \
  --dry-run=client -o yaml |
  sed -E "s/^  (${AGENTFACTORY_BOOTSTRAP_USERNAME_KEY}|${AGENTFACTORY_BOOTSTRAP_PASSWORD_KEY}): .*/  \\1: REDACTED/" \
  | tee "$setup_manifest_dir/03-bootstrap-secret.redacted.yaml"
printf '03 bootstrap-secret status=applied name=%s keys=%s,%s redacted_manifest=true full_manifest=true\n' \
  "$AGENTFACTORY_BOOTSTRAP_SECRET" "$AGENTFACTORY_BOOTSTRAP_USERNAME_KEY" \
  "$AGENTFACTORY_BOOTSTRAP_PASSWORD_KEY" >> "$setup_order_file"

if [ -n "${AGENTFACTORY_PREPARE_DATABASE_SCRIPT:-}" ]; then
  [ -r "$AGENTFACTORY_PREPARE_DATABASE_SCRIPT" ] || {
    echo "Agent Factory $test_id: database preparation script is not readable: $AGENTFACTORY_PREPARE_DATABASE_SCRIPT" >&2
    exit 2
  }
  # The test02 preparation script is sourced so it can select the Wallet
  # manifest and export the resulting TNS alias before rendering the CR.
  # shellcheck disable=SC1090
  source "$AGENTFACTORY_PREPARE_DATABASE_SCRIPT"
fi

template_file="${AGENTFACTORY_TEST_TEMPLATE:-$script_dir/${test_id}.yaml}"
cp -- "$template_file" "$setup_manifest_dir/04-agentfactory-source.yaml"
agentfactory_case_render_manifest "$template_file" "$AGENTFACTORY_RENDERED_MANIFEST"
cp "$AGENTFACTORY_RENDERED_MANIFEST" "$setup_manifest_dir/04-agentfactory.yaml"
printf '04 agentfactory status=rendered path=%s\n' "$setup_manifest_dir/04-agentfactory.yaml" >> "$setup_order_file"
agentfactory_case_kubectl -n "$AGENTFACTORY_CASE_NAMESPACE" apply \
  --dry-run=server -f "$AGENTFACTORY_RENDERED_MANIFEST" >/dev/null
agentfactory_case_kubectl -n "$AGENTFACTORY_CASE_NAMESPACE" apply \
  -f "$AGENTFACTORY_RENDERED_MANIFEST"

elapsed=0
startup_failure_since=-1
while [ "$elapsed" -lt "$AGENTFACTORY_TIMEOUT_SECONDS" ]; do
  phase="$(agentfactory_case_kubectl -n "$AGENTFACTORY_CASE_NAMESPACE" get agentfactory "$AGENTFACTORY_NAME" \
    -o jsonpath='{.status.phase}' 2>/dev/null || true)"
  ready="$(agentfactory_case_kubectl -n "$AGENTFACTORY_CASE_NAMESPACE" get agentfactory "$AGENTFACTORY_NAME" \
    -o jsonpath='{.status.readyReplicas}' 2>/dev/null || true)"
  database="$(agentfactory_case_kubectl -n "$AGENTFACTORY_CASE_NAMESPACE" get agentfactory "$AGENTFACTORY_NAME" \
    -o jsonpath='{.status.database.phase}' 2>/dev/null || true)"
  echo "Agent Factory $test_id readiness: elapsed=${elapsed}s phase=${phase:-<empty>} ready=${ready:-0} database=${database:-<empty>}"

  case "$phase" in
    Error|ERROR|Failed|FAILED|Blocked|BLOCKED)
      database_message="$(agentfactory_case_kubectl -n "$AGENTFACTORY_CASE_NAMESPACE" get agentfactory "$AGENTFACTORY_NAME" \
        -o jsonpath='{.status.database.message}' 2>/dev/null || true)"
      echo "Agent Factory $test_id: resource entered failure state: $phase" >&2
      echo "Agent Factory $test_id: database.phase=${database:-<empty>} message=${database_message:-<empty>}" >&2
      agentfactory_case_kubectl -n "$AGENTFACTORY_CASE_NAMESPACE" \
        get job,pod -l "app.kubernetes.io/instance=$AGENTFACTORY_NAME" -o wide >&2 || true
      agentfactory_case_kubectl -n "$AGENTFACTORY_CASE_NAMESPACE" \
        logs "job/${AGENTFACTORY_NAME}-db-setup" --all-containers --prefix --tail=200 >&2 || true
      exit 1
      ;;
    Ready|READY)
      # The controller can publish phase=Ready before readyReplicas is
      # updated. Do not start assertions until the workload has at least one
      # ready replica. Invoke the assertion script through Bash because the
      # checkout may not preserve its executable bit.
      if [[ "$ready" =~ ^[0-9]+$ ]] && [ "$ready" -ge 1 ]; then
        bash "$assert_script"
        agentfactory_case_assert_image_usage
        echo "Agent Factory $test_id passed: phase=$phase readyReplicas=$ready"
        exit 0
      fi
      echo "Agent Factory $test_id: phase=$phase but readyReplicas=${ready:-0}; waiting for a ready replica"
      ;;
  esac

  startup_failure_reason="$(e2e_workload_failure_reason \
    agentfactory_case_kubectl "$AGENTFACTORY_CASE_NAMESPACE" \
    "app.kubernetes.io/instance=${AGENTFACTORY_NAME}" || true)"
  if e2e_workload_failure_grace_update startup_failure_since \
    "$startup_failure_reason" "$elapsed" "Agent Factory $test_id"; then
    :
  else
    startup_failure_status=$?
    if [ "$startup_failure_status" -eq 2 ]; then
      agentfactory_case_collect_diagnostics "startup-failure-${startup_failure_reason}" || true
      echo "Agent Factory $test_id: workload remained in startup failure for $(e2e_workload_failure_grace_seconds)s: $startup_failure_reason" >&2
      exit 1
    fi
  fi

  sleep "$AGENTFACTORY_POLL_INTERVAL"
  elapsed=$((elapsed + AGENTFACTORY_POLL_INTERVAL))
done

agentfactory_case_collect_diagnostics timeout
echo "Agent Factory $test_id: timed out after ${AGENTFACTORY_TIMEOUT_SECONDS}s" >&2
exit 1
