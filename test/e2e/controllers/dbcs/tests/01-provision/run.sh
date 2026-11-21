#!/usr/bin/env bash
set -Eeuo pipefail

DBCS_CASE_SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC1090
source "$DBCS_CASE_SCRIPT_DIR/lib.sh"
dbcs_case_init
if [ -f "$DBCS_CASE_ENV_FILE" ]; then
  # shellcheck disable=SC1090
  source "$DBCS_CASE_ENV_FILE"
fi
dbcs_case_validate
mkdir -p "$DBCS_CASE_RESULTS_DIR"

# Safety net if the process exits early. With the default pre-cleanup policy the
# OCI resource is retained for debugging and is cleaned before the next run.
trap 'dbcs_case_cleanup' EXIT

echo "DBCS test01 create: applying $DBCS_TEST_NAME in $DBCS_NAMESPACE"
dbcs_case_kubectl -n "$DBCS_NAMESPACE" apply -f "$DBCS_RENDERED_MANIFEST"

elapsed=0
while [ "$elapsed" -lt "$DBCS_TIMEOUT_SECONDS" ]; do
  state="$(dbcs_case_kubectl -n "$DBCS_NAMESPACE" get dbcssystem "$DBCS_TEST_NAME" \
    -o jsonpath='{.status.state}' 2>/dev/null || true)"
  id="$(dbcs_case_kubectl -n "$DBCS_NAMESPACE" get dbcssystem "$DBCS_TEST_NAME" \
    -o jsonpath='{.status.id}' 2>/dev/null || true)"
  echo "DBCS test01 create readiness: elapsed=${elapsed}s state=${state:-<empty>} id=${id:-<empty>}"
  case "$state" in
    AVAILABLE)
      [ -n "$id" ] || { echo "DBCS test01: AVAILABLE without status.id" >&2; exit 1; }
      dbcs_case_record_db_system_id "$id"
      DBCS_DB_SYSTEM_ID="$id"
      export DBCS_DB_SYSTEM_ID
      dbcs_case_kubectl -n "$DBCS_NAMESPACE" get dbcssystem "$DBCS_TEST_NAME" -o yaml \
        > "$DBCS_CASE_RESULTS_DIR/dbcssystem-available.yaml"
      echo "DBCS test01 create passed: state=$state id=$id"
      break
      ;;
    FAILED|TERMINATED)
      dbcs_case_snapshot failed
      echo "DBCS test01: provisioning entered terminal state $state" >&2
      exit 1
      ;;
  esac
  sleep "$DBCS_POLL_INTERVAL_SECONDS"
  elapsed=$((elapsed + DBCS_POLL_INTERVAL_SECONDS))
  if [ "$elapsed" -ge "$DBCS_TIMEOUT_SECONDS" ]; then
    dbcs_case_snapshot timeout
    echo "DBCS test01: create timed out after ${DBCS_TIMEOUT_SECONDS}s" >&2
    exit 1
  fi
done

if [ "$DBCS_CASE_CLEANUP" != true ]; then
  dbcs_case_snapshot retained
  echo "DBCS test01 create passed; retaining $DBCS_NAMESPACE/$DBCS_TEST_NAME under E2E_CLEANUP_POLICY=${E2E_CLEANUP_POLICY:-pre}"
  exit 0
fi

echo "DBCS test01 unbind: removing soft-link resource $DBCS_TEST_NAME"
dbcs_case_snapshot before-delete
dbcs_case_kubectl -n "$DBCS_NAMESPACE" delete dbcssystem "$DBCS_TEST_NAME" \
  --wait=true --timeout="${DBCS_CLEANUP_TIMEOUT_SECONDS}s"

if dbcs_case_kubectl -n "$DBCS_NAMESPACE" get dbcssystem "$DBCS_TEST_NAME" >/dev/null 2>&1; then
  echo "DBCS test01: soft-link resource $DBCS_TEST_NAME was not deleted" >&2
  exit 1
fi

dbcs_case_render "$DBCS_TERMINATION_SOURCE" "$DBCS_TERMINATION_RENDERED_MANIFEST"
dbcs_case_kubectl -n "$DBCS_NAMESPACE" apply --dry-run=server -f "$DBCS_TERMINATION_RENDERED_MANIFEST" >/dev/null
echo "DBCS test01 bind-for-delete: applying $DBCS_TERMINATION_NAME for OCI DB system $id"
dbcs_case_kubectl -n "$DBCS_NAMESPACE" apply -f "$DBCS_TERMINATION_RENDERED_MANIFEST"

elapsed=0
while [ "$elapsed" -lt "$DBCS_TIMEOUT_SECONDS" ]; do
  finalizers="$(dbcs_case_kubectl -n "$DBCS_NAMESPACE" get dbcssystem "$DBCS_TERMINATION_NAME" \
    -o jsonpath='{.metadata.finalizers[*]}' 2>/dev/null || true)"
  case " $finalizers " in
    *" database.oracle.com/dbcsfinalizer "*)
      echo "DBCS test01 bind-for-delete passed: finalizer registered"
      break
      ;;
  esac
  sleep "$DBCS_POLL_INTERVAL_SECONDS"
  elapsed=$((elapsed + DBCS_POLL_INTERVAL_SECONDS))
done

if ! dbcs_case_kubectl -n "$DBCS_NAMESPACE" get dbcssystem "$DBCS_TERMINATION_NAME" \
  -o jsonpath='{.metadata.finalizers[*]}' | grep -Fq 'database.oracle.com/dbcsfinalizer'; then
  dbcs_case_snapshot missing-finalizer
  echo "DBCS test01: termination resource did not receive database.oracle.com/dbcsfinalizer" >&2
  exit 1
fi

echo "DBCS test01 delete: removing termination resource $DBCS_TERMINATION_NAME"
dbcs_case_kubectl -n "$DBCS_NAMESPACE" delete -f "$DBCS_TERMINATION_RENDERED_MANIFEST" \
  --wait=true --timeout="${DBCS_CLEANUP_TIMEOUT_SECONDS}s"

elapsed=0
while [ "$elapsed" -lt "$DBCS_CLEANUP_TIMEOUT_SECONDS" ]; do
  if ! dbcs_case_kubectl -n "$DBCS_NAMESPACE" get dbcssystem "$DBCS_TERMINATION_NAME" >/dev/null 2>&1; then
    echo "DBCS test01 delete passed: $DBCS_TERMINATION_NAME is gone"
    DBCS_CASE_CLEANUP=false
    export DBCS_CASE_CLEANUP
    dbcs_case_wait_for_db_system_termination "$id"
    dbcs_case_kubectl -n "$DBCS_NAMESPACE" delete secret oci-privatekey admin-password oci-publickey \
      --ignore-not-found >/dev/null 2>&1 || true
    echo "DBCS test01 create+delete passed"
    exit 0
  fi
  state="$(dbcs_case_kubectl -n "$DBCS_NAMESPACE" get dbcssystem "$DBCS_TERMINATION_NAME" \
    -o jsonpath='{.status.state}' 2>/dev/null || true)"
  echo "DBCS test01 delete readiness: elapsed=${elapsed}s state=${state:-<empty>}"
  sleep "$DBCS_POLL_INTERVAL_SECONDS"
  elapsed=$((elapsed + DBCS_POLL_INTERVAL_SECONDS))
done

dbcs_case_snapshot delete-timeout
echo "DBCS test01: delete timed out after ${DBCS_CLEANUP_TIMEOUT_SECONDS}s" >&2
exit 1
