#!/usr/bin/env bash
set -Eeuo pipefail

script_dir="$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC1091
source "$script_dir/lib.sh"

privateai_case_init
case "$PRIVATEAI_IMAGE_PULL_POLICY" in
  Always|IfNotPresent|Never) ;;
  *)
    echo "PrivateAI test01: PRIVATEAI_IMAGE_PULL_POLICY must be Always, IfNotPresent, or Never" >&2
    exit 2
    ;;
esac

[ -n "$PRIVATEAI_IMAGE" ] || {
  echo "PrivateAI test01: PRIVATEAI_IMAGE (or PRIVATE_AI_IMAGE/PAI_IMAGE) is required" >&2
  exit 2
}
privateai_case_kubectl get namespace "$PRIVATEAI_CASE_NAMESPACE" >/dev/null
mkdir -p "$PRIVATEAI_CASE_RESULTS_DIR"
trap 'privateai_case_remove_secret_material' EXIT
trap 'privateai_case_collect_diagnostics failure || true' ERR

privateai_case_create_quickstart_secrets
cp "$script_dir/test01.yaml" "$PRIVATEAI_CASE_RESULTS_DIR/test01-source.yaml"
privateai_case_render_manifest "$script_dir/test01.yaml" "$PRIVATEAI_RENDERED_MANIFEST"
privateai_case_kubectl -n "$PRIVATEAI_CASE_NAMESPACE" apply \
  --dry-run=server -f "$PRIVATEAI_RENDERED_MANIFEST" >/dev/null
privateai_case_kubectl -n "$PRIVATEAI_CASE_NAMESPACE" apply \
  -f "$PRIVATEAI_RENDERED_MANIFEST"

elapsed=0
startup_failure_since=-1
while [ "$elapsed" -lt "$PRIVATEAI_TIMEOUT_SECONDS" ]; do
  status="$(privateai_case_kubectl -n "$PRIVATEAI_CASE_NAMESPACE" get privateai "$PRIVATEAI_NAME" \
    -o jsonpath='{.status.status}' 2>/dev/null || true)"
  ready="$(privateai_case_kubectl -n "$PRIVATEAI_CASE_NAMESPACE" get deployment "$PRIVATEAI_NAME" \
    -o jsonpath='{.status.availableReplicas}' 2>/dev/null || true)"
  echo "PrivateAI test01 readiness: elapsed=${elapsed}s status=${status:-<empty>} available=${ready:-0}"

  case "$status" in
    Failed|FAILED|Error|ERROR|Unhealthy|UNHEALTHY)
      echo "PrivateAI test01: resource entered failure state: $status" >&2
      exit 1
      ;;
    Healthy|HEALTHY|Ready|READY|Available|AVAILABLE)
      if [ "${ready:-0}" -ge 1 ] 2>/dev/null; then
        service="$(privateai_case_kubectl -n "$PRIVATEAI_CASE_NAMESPACE" get privateai "$PRIVATEAI_NAME" \
          -o jsonpath='{.status.localService}' 2>/dev/null || true)"
        [ -n "$service" ] || {
          echo "PrivateAI test01: status.localService is empty" >&2
          exit 1
        }
        bash "$script_dir/assert.sh"
        privateai_case_assert_image_usage
        privateai_case_collect_diagnostics passed
        echo "PrivateAI test01 passed: status=$status service=$service"
        exit 0
      fi
      ;;
  esac

  startup_failure_reason="$(e2e_workload_failure_reason \
    privateai_case_kubectl "$PRIVATEAI_CASE_NAMESPACE" \
    "app.kubernetes.io/instance=PrivateAi-${PRIVATEAI_NAME}" || true)"
  if e2e_workload_failure_grace_update startup_failure_since \
    "$startup_failure_reason" "$elapsed" "PrivateAI test01"; then
    :
  else
    startup_failure_status=$?
    if [ "$startup_failure_status" -eq 2 ]; then
      privateai_case_collect_diagnostics "startup-failure-${startup_failure_reason}" || true
      echo "PrivateAI test01: workload remained in startup failure for $(e2e_workload_failure_grace_seconds)s: $startup_failure_reason" >&2
      exit 1
    fi
  fi

  sleep "$PRIVATEAI_POLL_INTERVAL"
  elapsed=$((elapsed + PRIVATEAI_POLL_INTERVAL))
done

privateai_case_collect_diagnostics timeout
echo "PrivateAI test01: timed out after ${PRIVATEAI_TIMEOUT_SECONDS}s" >&2
exit 1
