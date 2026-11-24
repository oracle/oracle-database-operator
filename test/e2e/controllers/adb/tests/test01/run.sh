#!/usr/bin/env bash
set -euo pipefail

echo "Executing test/e2e/controllers/adb/tests/test01/run.sh"

script_dir="$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
repo_root="${CI_PROJECT_DIR:-$(CDPATH='' cd -- "$script_dir/../../../../../.." && pwd)}"

adb_fail() {
  printf 'ADB test01: %s\n' "$*" >&2
  exit 1
}

# Capture resource state on failure, without dumping Secret contents.
adb_diagnostics() {
  echo "ADB test01: collecting CR and event diagnostics"
  adb_kubectl -n "$ADB_E2E_NAMESPACE" get adb "$ADB_TEST_NAME" -o yaml || true
  adb_kubectl -n "$ADB_E2E_NAMESPACE" get events --sort-by=.lastTimestamp || true
}
adb_on_exit() {
  local rc="$?"
  if [ "$rc" -ne 0 ]; then
    adb_diagnostics
  fi
  exit "$rc"
}
trap adb_on_exit EXIT

# Use the configured Kubernetes context and bounded request timeout.
if ! declare -F adb_kubectl >/dev/null 2>&1; then
  command -v kubectl >/dev/null 2>&1 || adb_fail "kubectl is required"
  adb_kubectl() {
    local -a args=(--request-timeout=30s)
    [ -z "${KUBE_CONTEXT_NAME:-}" ] || args+=(--context "$KUBE_CONTEXT_NAME")
    kubectl "${args[@]}" "$@"
  }
fi

adb_init_parameters() {
  ADB_TEST_NAME="${ADB_TEST_NAME:-test01}"
  ADB_E2E_NAMESPACE="${ADB_E2E_NAMESPACE:-adb-ns}"
  ADB_COMPARTMENT_OCID="${ADB_COMPARTMENT_OCID:-}"
  ADB_OCI_CLI_PROFILE="${ADB_OCI_CLI_PROFILE:-adb}"
  ADB_OCI_CONFIG_CONFIG_MAP_NAME="${ADB_OCI_CONFIG_CONFIG_MAP_NAME:-oci-cred}"
  ADB_OCI_CONFIG_SECRET_NAME="${ADB_OCI_CONFIG_SECRET_NAME:-oci-privatekey}"
  ADB_ADMIN_PASSWORD_SECRET_NAME="${ADB_ADMIN_PASSWORD_SECRET_NAME:-adb-admin-password}"
  ADB_ADMIN_PASSWORD_SECRET_VALUE="${ADB_ADMIN_PASSWORD_SECRET_VALUE:-}"
  ADB_TIMEOUT_SECONDS="${ADB_TIMEOUT_SECONDS:-300}"

  printf "%s\n" \
  "ADB test01 setup parameters:" \
  "  ADB name: $ADB_TEST_NAME" \
  "  ADB namespace: $ADB_E2E_NAMESPACE" \
  "  ADB compartment OCID: $ADB_COMPARTMENT_OCID" \
  "  ADB OCI CLI profile: $ADB_OCI_CLI_PROFILE" \
  "  ADB OCI config config map name: $ADB_OCI_CONFIG_CONFIG_MAP_NAME" \
  "  ADB OCI config secret name: $ADB_OCI_CONFIG_SECRET_NAME" \
  "  ADB admin password secret name: $ADB_ADMIN_PASSWORD_SECRET_NAME" \
  "  ADB admin password secret value: $ADB_ADMIN_PASSWORD_SECRET_VALUE" \
  "  ADB timeout seconds: $ADB_TIMEOUT_SECONDS" \
  "Using the default Kubernetes service account."
}
adb_init_parameters

adb_setup_require() {
  local variable_name="$1"
  [ -n "${!variable_name:-}" ] || adb_fail "$variable_name is required"
}

adb_validate_parameters() {
  # Ensure the required parameters are provided.
  adb_setup_require "ADB_COMPARTMENT_OCID"
  adb_setup_require "ADB_ADMIN_PASSWORD_SECRET_VALUE"

  # Validate the ADB test name to ensure it contains only alphanumeric characters.
  if [[ ! "$ADB_TEST_NAME" =~ ^[a-zA-Z0-9]+$ ]]; then
    adb_fail "ADB_TEST_NAME contains non-alphanumeric characters: $ADB_TEST_NAME"
  fi

  # Validate password length (maximum 30 characters).
  (( ${#ADB_ADMIN_PASSWORD_SECRET_VALUE} <= 30 )) \
    || adb_fail "ADB_ADMIN_PASSWORD_SECRET_VALUE must not exceed 30 characters"

  # Validate password contains at least one digit.
  [[ "$ADB_ADMIN_PASSWORD_SECRET_VALUE" =~ [0-9] ]] \
    || adb_fail "ADB_ADMIN_PASSWORD_SECRET_VALUE must contain at least one number"
}
adb_validate_parameters

init_kubernetes_test_resources() {
  if [[ -n "${OCI_RUNTIME_DIR:-}" ]]; then
    local OCI_RUNTIME_CONFIG="${OCI_RUNTIME_DIR}/config"
  else
    local OCI_RUNTIME_CONFIG="${CI_PROJECT_DIR:-$PWD}/.ci/oci/config"
  fi

  echo "Running the helper script set_ocicredentials.sh using OCI runtime config: $OCI_RUNTIME_CONFIG"

  bash "$repo_root/set_ocicredentials.sh" \
      run \
      -n "$ADB_E2E_NAMESPACE" \
      -path "$OCI_RUNTIME_CONFIG" \
      -profile "$ADB_OCI_CLI_PROFILE" \
      -configmap "$ADB_OCI_CONFIG_CONFIG_MAP_NAME" \
      -secret "$ADB_OCI_CONFIG_SECRET_NAME" \
      || adb_fail "Failed to create OCI credential resources"

  printf 'Creating ADB admin password secret: %s\n' "$ADB_ADMIN_PASSWORD_SECRET_NAME"
  adb_kubectl create secret generic "$ADB_ADMIN_PASSWORD_SECRET_NAME" \
    --namespace "$ADB_E2E_NAMESPACE" \
    --from-literal=password="$ADB_ADMIN_PASSWORD_SECRET_VALUE" \
    --dry-run=client -o yaml | adb_kubectl apply -f - || adb_fail "Failed to create ADB admin password secret"
}

echo "ADB test01: init test resources in namespace $ADB_E2E_NAMESPACE"
init_kubernetes_test_resources

echo "ADB test01: rendering manifest for $ADB_E2E_NAMESPACE/$ADB_TEST_NAME"
manifest="$(cat "$script_dir/test01.yaml")"
manifest="${manifest//__NAME__/"$ADB_TEST_NAME"}"
manifest="${manifest//__NAMESPACE__/"$ADB_E2E_NAMESPACE"}"
manifest="${manifest//__COMPARTMENT__/"$ADB_COMPARTMENT_OCID"}"
manifest="${manifest//__ADMIN_PASSWORD_SECRET_NAME__/"$ADB_ADMIN_PASSWORD_SECRET_NAME"}"
manifest="${manifest//__OCI_CONFIG_CONFIG_MAP_NAME__/"$ADB_OCI_CONFIG_CONFIG_MAP_NAME"}"
manifest="${manifest//__OCI_CONFIG_SECRET_NAME__/"$ADB_OCI_CONFIG_SECRET_NAME"}"
echo "ADB test01: validating manifest with server dry-run"
printf '%s\n' "$manifest" | adb_kubectl apply --dry-run=server -f - >/dev/null

echo "ADB test01: applying ADB manifest"
printf '%s\n' "$manifest" | adb_kubectl apply -f -

adb_kubectl --request-timeout=0 -n "$ADB_E2E_NAMESPACE" wait "adb/$ADB_TEST_NAME" \
  --for=jsonpath='{.status.lifecycleState}'=AVAILABLE --timeout="${ADB_TIMEOUT_SECONDS}s" \
  || adb_fail "Timed out waiting for ADB $ADB_TEST_NAME to become AVAILABLE after ${ADB_TIMEOUT_SECONDS}s"

echo "ADB test01: checking ADB OCID is present"
adb_id="$(adb_kubectl -n "$ADB_E2E_NAMESPACE" \
  get adb "$ADB_TEST_NAME" \
  -o jsonpath='{.spec.details.id}')"
[ -n "$adb_id" ] || adb_fail "expected ADB OCID to be present"
echo "Passed: ADB OCID is present"
