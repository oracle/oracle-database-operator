#!/usr/bin/env bash

dbcs_case_init() {
  : "${DBCS_CASE_ID:=test01}"
  : "${DBCS_CASE_NAME:=01-provision}"
  : "${DBCS_NAMESPACE:=${DBCS_E2E_NAMESPACE:-${E2E_NAMESPACE:-dbcs-ns}}}"
  : "${DBCS_TEST_NAME:=dbcssystem-e2e-minimal}"
  : "${DBCS_CASE_PIPELINE:=${CI_PIPELINE_ID:-local}}"
  : "${DBCS_TIMEOUT_SECONDS:=${DBCS_TIMEOUT:-9000}}"
  : "${DBCS_POLL_INTERVAL_SECONDS:=30}"
  : "${DBCS_CLEANUP_TIMEOUT_SECONDS:=${DBCS_CLEANUP_TIMEOUT:-9000}}"
  if [ -z "${DBCS_CASE_CLEANUP+x}" ]; then
    case "${E2E_CLEANUP_POLICY:-pre}" in
      post|both) DBCS_CASE_CLEANUP=true ;;
      *) DBCS_CASE_CLEANUP=false ;;
    esac
  fi
  : "${DBCS_NAMESPACE_CREATED:=false}"
  : "${DBCS_CASE_ENV_FILE:=${TMPDIR:-/tmp}/dbcs-ns.env}"
  : "${DBCS_CASE_RESULTS_DIR:=${DBCS_E2E_RESULTS_DIR:-${OPERATOR_LOG_DIR:-test/e2e/controllers/dbcs/results}}/dbcs/${DBCS_CASE_NAME}-${DBCS_NAMESPACE}}"
  : "${DBCS_RENDERED_MANIFEST:=${DBCS_CASE_RESULTS_DIR}/test01-rendered.yaml}"
  : "${DBCS_CASE_SOURCE:=${DBCS_CASE_SCRIPT_DIR}/test01.yaml}"
  : "${DBCS_TERMINATION_NAME:=${DBCS_TEST_NAME}-terminate}"
  : "${DBCS_TERMINATION_RENDERED_MANIFEST:=${DBCS_CASE_RESULTS_DIR}/terminate-rendered.yaml}"
  : "${DBCS_TERMINATION_SOURCE:=${DBCS_CASE_SCRIPT_DIR}/terminate.yaml}"
  : "${DBCS_DB_SYSTEM_ID:=}"
  : "${DBCS_DB_SYSTEM_ID_FILE:=${DBCS_CASE_RESULTS_DIR}/db-system-id}"

  export DBCS_CASE_ID DBCS_CASE_NAME DBCS_NAMESPACE DBCS_TEST_NAME
  export DBCS_CASE_PIPELINE DBCS_TIMEOUT_SECONDS DBCS_POLL_INTERVAL_SECONDS
  export DBCS_CLEANUP_TIMEOUT_SECONDS DBCS_CASE_CLEANUP DBCS_NAMESPACE_CREATED
  export DBCS_CASE_ENV_FILE DBCS_CASE_RESULTS_DIR DBCS_RENDERED_MANIFEST DBCS_CASE_SOURCE
  export DBCS_TERMINATION_NAME DBCS_TERMINATION_RENDERED_MANIFEST DBCS_TERMINATION_SOURCE
  export DBCS_DB_SYSTEM_ID DBCS_DB_SYSTEM_ID_FILE
}

dbcs_case_kubectl() {
  if [ -n "${KUBE_CONTEXT_NAME:-}" ]; then
    command kubectl --context "$KUBE_CONTEXT_NAME" "$@"
  else
    command kubectl "$@"
  fi
}

dbcs_case_require() {
  command -v "$1" >/dev/null 2>&1 || {
    echo "DBCS test01: required command is unavailable: $1" >&2
    return 2
  }
}

dbcs_case_oci_profile_value() {
  local key="$1" file="${OCI_CLI_CONFIG_FILE:-}" profile="${OCI_CLI_PROFILE:-}"
  [ -r "$file" ] && [ -n "$profile" ] || return 0
  awk -v profile="$profile" -v key="$key" '
    BEGIN { current = "" }
    /^\[/ {
      current = $0
      gsub(/\[|\]/, "", current)
      next
    }
    current == profile && index($0, "=") {
      split($0, fields, "=")
      name = fields[1]
      value = substr($0, index($0, "=") + 1)
      gsub(/^[[:space:]]+|[[:space:]]+$/, "", name)
      gsub(/^[[:space:]]+|[[:space:]]+$/, "", value)
      if (tolower(name) == tolower(key)) {
        print value
        exit
      }
    }
  ' "$file"
}

dbcs_case_validate() {
  local name value
  dbcs_case_require kubectl
  dbcs_case_require oci
  dbcs_case_require jq
  DBCS_OCI_TENANCY="${DBCS_OCI_TENANCY:-$(dbcs_case_oci_profile_value tenancy)}"
  DBCS_OCI_USER="${DBCS_OCI_USER:-$(dbcs_case_oci_profile_value user)}"
  DBCS_OCI_FINGERPRINT="${DBCS_OCI_FINGERPRINT:-$(dbcs_case_oci_profile_value fingerprint)}"
  DBCS_OCI_REGION="${DBCS_OCI_REGION:-${OCI_REGION:-}}"
  export DBCS_OCI_TENANCY DBCS_OCI_USER DBCS_OCI_FINGERPRINT DBCS_OCI_REGION
  for name in \
    DBCS_AVAILABILITY_DOMAIN DBCS_COMPARTMENT_ID DBCS_SUBNET_ID DBCS_DOMAIN \
    DBCS_DB_NAME DBCS_DISPLAY_NAME DBCS_HOST_NAME DBCS_ADMIN_PASSWORD \
    DBCS_OCI_TENANCY DBCS_OCI_USER DBCS_OCI_FINGERPRINT DBCS_OCI_REGION \
    DBCS_DB_VERSION DBCS_DB_EDITION DBCS_LICENSE_MODEL DBCS_DB_WORKLOAD \
    DBCS_SHAPE DBCS_CPU_CORE_COUNT DBCS_STORAGE_SIZE_IN_GB \
    OCI_API_PRIVATE_KEY_FILE OCI_CLI_CONFIG_FILE OCI_CLI_PROFILE DBCS_SSH_PUBLIC_KEY_FILE; do
    value="${!name:-}"
    [ -n "$value" ] || { echo "DBCS test01: $name is required" >&2; return 2; }
  done
  [ -r "$OCI_API_PRIVATE_KEY_FILE" ] || {
    echo "DBCS test01: OCI API key file is not readable" >&2
    return 2
  }
  [ -r "$OCI_CLI_CONFIG_FILE" ] || {
    echo "DBCS test01: OCI CLI config file is not readable" >&2
    return 2
  }
  [ -r "$DBCS_SSH_PUBLIC_KEY_FILE" ] || {
    echo "DBCS test01: SSH public key file is not readable" >&2
    return 2
  }
}

dbcs_case_escape_sed() {
  printf '%s' "$1" | sed 's/[&|\\]/\\&/g'
}

dbcs_case_render() {
  local source="$1" destination="$2" expression value
  mkdir -p "$(dirname "$destination")"
  expression=""
  for value in \
    DBCS_TEST_NAME DBCS_NAMESPACE DBCS_CASE_ID DBCS_CASE_PIPELINE \
    DBCS_TERMINATION_NAME DBCS_DB_SYSTEM_ID \
    DBCS_AVAILABILITY_DOMAIN DBCS_COMPARTMENT_ID DBCS_DB_EDITION DBCS_DB_NAME \
    DBCS_DISPLAY_NAME DBCS_LICENSE_MODEL DBCS_DB_VERSION DBCS_DB_WORKLOAD \
    DBCS_HOST_NAME DBCS_SHAPE DBCS_CPU_CORE_COUNT DBCS_STORAGE_SIZE_IN_GB \
    DBCS_DOMAIN DBCS_SUBNET_ID; do
    expression="${expression} -e s|__${value}__|$(dbcs_case_escape_sed "${!value}")|g"
  done
  # shellcheck disable=SC2086
  sed $expression "$source" > "$destination"
}

dbcs_case_snapshot() {
  local reason="${1:-snapshot}" directory
  directory="$DBCS_CASE_RESULTS_DIR/snapshots/$reason"
  mkdir -p "$directory"
  dbcs_case_kubectl -n "$DBCS_NAMESPACE" get dbcssystem "$DBCS_TEST_NAME" -o yaml > "$directory/dbcssystem.yaml" 2>&1 || true
  dbcs_case_kubectl -n "$DBCS_NAMESPACE" get events --sort-by=.lastTimestamp > "$directory/events.txt" 2>&1 || true
  dbcs_case_kubectl -n "$DBCS_NAMESPACE" get secret,configmap > "$directory/config-resources.txt" 2>&1 || true
}

dbcs_case_record_db_system_id() {
  local id="$1"
  mkdir -p "$DBCS_CASE_RESULTS_DIR"
  printf '%s\n' "$id" > "$DBCS_DB_SYSTEM_ID_FILE"
}

dbcs_case_db_system_state() {
  local id="$1" response
  response="$(oci --config-file "$OCI_CLI_CONFIG_FILE" --profile "$OCI_CLI_PROFILE" \
    --region "$DBCS_OCI_REGION" --auth api_key db system get --db-system-id "$id")" || return
  printf '%s\n' "$response" > "$DBCS_CASE_RESULTS_DIR/db-system.json"
  jq -r '.data["lifecycle-state"] // empty' <<<"$response"
}

dbcs_case_wait_for_db_system_termination() {
  local id="$1" elapsed=0 state
  [ -n "$id" ] || { echo "DBCS test01: OCI DB system ID is required for cleanup" >&2; return 2; }

  while [ "$elapsed" -lt "$DBCS_CLEANUP_TIMEOUT_SECONDS" ]; do
    state="$(dbcs_case_db_system_state "$id")" || {
      echo "DBCS test01: unable to get OCI DB system $id during cleanup" >&2
      return 1
    }
    echo "DBCS test01 OCI delete readiness: elapsed=${elapsed}s id=$id state=${state:-<empty>}"
    case "$state" in
      TERMINATED)
        echo "DBCS test01 OCI delete passed: id=$id state=$state"
        return 0
        ;;
      FAILED)
        echo "DBCS test01: OCI DB system $id entered terminal state $state during deletion" >&2
        return 1
        ;;
    esac
    sleep "$DBCS_POLL_INTERVAL_SECONDS"
    elapsed=$((elapsed + DBCS_POLL_INTERVAL_SECONDS))
  done

  echo "DBCS test01: OCI DB system $id did not reach TERMINATED after ${DBCS_CLEANUP_TIMEOUT_SECONDS}s" >&2
  return 1
}

dbcs_case_cleanup() {
  [ "$DBCS_CASE_CLEANUP" = true ] || {
    echo "DBCS test01 cleanup disabled; retaining $DBCS_NAMESPACE/$DBCS_TEST_NAME"
    return 0
  }

  local elapsed=0 id=""
  if [ -r "$DBCS_DB_SYSTEM_ID_FILE" ]; then
    id="$(<"$DBCS_DB_SYSTEM_ID_FILE")"
  fi
  dbcs_case_snapshot before-cleanup
  dbcs_case_kubectl -n "$DBCS_NAMESPACE" delete dbcssystem "$DBCS_TERMINATION_NAME" \
    --ignore-not-found --wait=true --timeout="${DBCS_CLEANUP_TIMEOUT_SECONDS}s" || true
  dbcs_case_kubectl -n "$DBCS_NAMESPACE" delete dbcssystem "$DBCS_TEST_NAME" \
    --ignore-not-found --wait=true --timeout="${DBCS_CLEANUP_TIMEOUT_SECONDS}s" || true
  while [ "$elapsed" -lt "$DBCS_CLEANUP_TIMEOUT_SECONDS" ]; do
    if ! dbcs_case_kubectl -n "$DBCS_NAMESPACE" get dbcssystem "$DBCS_TEST_NAME" >/dev/null 2>&1; then
      echo "DBCS test01 cleanup confirmed: $DBCS_TEST_NAME is gone"
      break
    fi
    sleep "$DBCS_POLL_INTERVAL_SECONDS"
    elapsed=$((elapsed + DBCS_POLL_INTERVAL_SECONDS))
  done
  if dbcs_case_kubectl -n "$DBCS_NAMESPACE" get dbcssystem "$DBCS_TEST_NAME" >/dev/null 2>&1 || \
    dbcs_case_kubectl -n "$DBCS_NAMESPACE" get dbcssystem "$DBCS_TERMINATION_NAME" >/dev/null 2>&1; then
    echo "DBCS test01: timed out waiting for DBCS resources deletion" >&2
    return 1
  fi
  if [ -n "$id" ]; then
    dbcs_case_wait_for_db_system_termination "$id"
  fi
  dbcs_case_kubectl -n "$DBCS_NAMESPACE" delete secret oci-privatekey admin-password oci-publickey \
    --ignore-not-found >/dev/null 2>&1 || true
  if [ "$DBCS_NAMESPACE_CREATED" = true ]; then
    dbcs_case_kubectl delete namespace "$DBCS_NAMESPACE" --ignore-not-found \
      --wait=true --timeout="${DBCS_CLEANUP_TIMEOUT_SECONDS}s" >/dev/null 2>&1 || true
  fi
}
