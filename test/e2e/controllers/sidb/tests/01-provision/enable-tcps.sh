#!/usr/bin/env bash
set -Eeuo pipefail

# Enable TCPS on an already-ready SIDB by patching spec.security.tcps.
# Day-0 test01 remains TCP-only; this stage exercises the post-provision
# TCPS enable path required by Agent Factory wallet connections on port 2484.

script_dir="$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC1091
source "$script_dir/../common/lib.sh"

sidb_case_load_env
sidb_case_init_config

: "${SIDB_TCPS_TLS_SECRET:=sidb-e2e-tcps-tls-test01}"
export SIDB_TCPS_TLS_SECRET

sidb_case_validate_config

SIDB_CASE_STAGE=enable-tcps
trap sidb_case_setup_exit_snapshot EXIT

mkdir -p "$SIDB_CASE_RESULTS_DIR"

if ! sidb_case_kubectl -n "$SIDB_CASE_NAMESPACE" get singleinstancedatabase "$SIDB_TEST_NAME" >/dev/null 2>&1; then
  echo "$(sidb_case_label): $SIDB_CASE_NAMESPACE/$SIDB_TEST_NAME has not been applied" >&2
  exit 1
fi

status_before="$(sidb_case_kubectl -n "$SIDB_CASE_NAMESPACE" get singleinstancedatabase "$SIDB_TEST_NAME" \
  -o jsonpath='{.status.status}' 2>/dev/null || true)"
case "$status_before" in
  Ready|READY|Healthy|HEALTHY|Available|AVAILABLE) ;;
  *)
    echo "$(sidb_case_label): refuse to enable TCPS while status is '$status_before'" >&2
    exit 1
    ;;
esac

sidb_case_create_tcps_tls_secret || exit 1

spec_tcps_enabled="$(sidb_case_kubectl -n "$SIDB_CASE_NAMESPACE" get singleinstancedatabase "$SIDB_TEST_NAME" \
  -o jsonpath='{.spec.security.tcps.enabled}' 2>/dev/null || true)"
spec_tcps_secret="$(sidb_case_kubectl -n "$SIDB_CASE_NAMESPACE" get singleinstancedatabase "$SIDB_TEST_NAME" \
  -o jsonpath='{.spec.security.tcps.tlsSecret}' 2>/dev/null || true)"
status_tcps_enabled="$(sidb_case_kubectl -n "$SIDB_CASE_NAMESPACE" get singleinstancedatabase "$SIDB_TEST_NAME" \
  -o jsonpath='{.status.isTcpsEnabled}' 2>/dev/null || true)"

if [ "$spec_tcps_enabled" = "true" ] &&
   [ "$spec_tcps_secret" = "$SIDB_TCPS_TLS_SECRET" ] &&
   [ "$status_tcps_enabled" = "true" ]; then
  echo "$(sidb_case_label): TCPS already enabled with tlsSecret=$SIDB_TCPS_TLS_SECRET"
else
  patch_file="$SIDB_CASE_RESULTS_DIR/enable-tcps.patch.json"
  printf '{"spec":{"security":{"tcps":{"enabled":true,"tlsSecret":"%s"}}}}\n' \
    "$SIDB_TCPS_TLS_SECRET" > "$patch_file"
  chmod 600 "$patch_file"
  echo "$(sidb_case_label): patching security.tcps.enabled=true tlsSecret=$SIDB_TCPS_TLS_SECRET"
  sidb_case_kubectl -n "$SIDB_CASE_NAMESPACE" patch singleinstancedatabase "$SIDB_TEST_NAME" \
    --type=merge --patch-file "$patch_file" >/dev/null
fi

# Persist the TLS Secret name so later Agent Factory stages and cleanup see it.
sidb_case_write_env

elapsed=0
while [ "$elapsed" -lt "$SIDB_TIMEOUT_SECONDS" ]; do
  status="$(sidb_case_kubectl -n "$SIDB_CASE_NAMESPACE" get singleinstancedatabase "$SIDB_TEST_NAME" \
    -o jsonpath='{.status.status}' 2>/dev/null || true)"
  role="$(sidb_case_kubectl -n "$SIDB_CASE_NAMESPACE" get singleinstancedatabase "$SIDB_TEST_NAME" \
    -o jsonpath='{.status.role}' 2>/dev/null || true)"
  status_tcps_enabled="$(sidb_case_kubectl -n "$SIDB_CASE_NAMESPACE" get singleinstancedatabase "$SIDB_TEST_NAME" \
    -o jsonpath='{.status.isTcpsEnabled}' 2>/dev/null || true)"
  tcps_connect="$(sidb_case_kubectl -n "$SIDB_CASE_NAMESPACE" get singleinstancedatabase "$SIDB_TEST_NAME" \
    -o jsonpath='{.status.tcpsConnectString}' 2>/dev/null || true)"
  wallet_loc="$(sidb_case_kubectl -n "$SIDB_CASE_NAMESPACE" get singleinstancedatabase "$SIDB_TEST_NAME" \
    -o jsonpath='{.status.clientWalletLoc}' 2>/dev/null || true)"

  echo "$(sidb_case_label) tcps: elapsed=${elapsed}s status=${status:-<empty>} role=${role:-<empty>} isTcpsEnabled=${status_tcps_enabled:-<empty>}"

  case "$status" in
    Failed|FAILED|Error|ERROR)
      echo "$(sidb_case_label): SIDB entered failure state while enabling TCPS: $status" >&2
      exit 1
      ;;
  esac

  if [ "$status_tcps_enabled" = "true" ] &&
     [ -n "$tcps_connect" ] &&
     [ -n "$wallet_loc" ] &&
     [[ "$tcps_connect" == *":2484"* || "$tcps_connect" == *"2484/"* ]] &&
     case "$status" in Ready|READY|Healthy|HEALTHY|Available|AVAILABLE) true ;; *) false ;; esac &&
     [ "${role// /_}" = "PRIMARY" ]; then
    printf '%s\n' \
      "enabled=true" \
      "tls_secret=$SIDB_TCPS_TLS_SECRET" \
      "status=$status" \
      "tcps_connect_string=$tcps_connect" \
      "client_wallet_loc=$wallet_loc" \
      > "$SIDB_CASE_RESULTS_DIR/enable-tcps-status.txt"
    echo "$(sidb_case_label): TCPS enabled; wallet at $wallet_loc; connect=$tcps_connect"
    exit 0
  fi

  sleep "$SIDB_POLL_INTERVAL"
  elapsed=$((elapsed + SIDB_POLL_INTERVAL))
done

sidb_case_collect_diagnostics "enable-tcps-timeout" || true
echo "$(sidb_case_label): timed out enabling TCPS after ${SIDB_TIMEOUT_SECONDS}s" >&2
exit 1
