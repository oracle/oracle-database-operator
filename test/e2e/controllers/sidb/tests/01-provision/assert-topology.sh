#!/usr/bin/env bash
set -Eeuo pipefail

script_dir="$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC1091
# Test01-specific assertions for the baseline primary database.
source "$script_dir/../common/lib.sh"

sidb_case_load_env
sidb_case_init_config
sidb_case_validate_config

SIDB_CASE_STAGE=assert-topology
trap sidb_case_setup_exit_snapshot EXIT

sidb_case_kubectl -n "$SIDB_CASE_NAMESPACE" get singleinstancedatabase "$SIDB_TEST_NAME" >/dev/null

status="$(sidb_case_kubectl -n "$SIDB_CASE_NAMESPACE" get singleinstancedatabase "$SIDB_TEST_NAME" -o jsonpath='{.status.status}')"
role="$(sidb_case_kubectl -n "$SIDB_CASE_NAMESPACE" get singleinstancedatabase "$SIDB_TEST_NAME" -o jsonpath='{.status.role}')"
spec_create_as="$(sidb_case_kubectl -n "$SIDB_CASE_NAMESPACE" get singleinstancedatabase "$SIDB_TEST_NAME" -o jsonpath='{.spec.createAs}')"
spec_sid="$(sidb_case_kubectl -n "$SIDB_CASE_NAMESPACE" get singleinstancedatabase "$SIDB_TEST_NAME" -o jsonpath='{.spec.sid}')"
spec_pdb="$(sidb_case_kubectl -n "$SIDB_CASE_NAMESPACE" get singleinstancedatabase "$SIDB_TEST_NAME" -o jsonpath='{.spec.pdbName}')"
spec_image="$(sidb_case_kubectl -n "$SIDB_CASE_NAMESPACE" get singleinstancedatabase "$SIDB_TEST_NAME" -o jsonpath='{.spec.image.pullFrom}')"
connect_string="$(sidb_case_kubectl -n "$SIDB_CASE_NAMESPACE" get singleinstancedatabase "$SIDB_TEST_NAME" -o jsonpath='{.status.connectString}')"

case "$status" in
  Ready|READY|Healthy|HEALTHY|Available|AVAILABLE) ;;
  *) echo "$(sidb_case_label): expected a ready status, got '$status'" >&2; exit 1 ;;
esac
[ "$role" = "PRIMARY" ] || { echo "$(sidb_case_label): expected PRIMARY role, got '$role'" >&2; exit 1; }
[ "$spec_create_as" = "primary" ] || { echo "$(sidb_case_label): expected spec.createAs=primary, got '$spec_create_as'" >&2; exit 1; }
[ "$spec_sid" = "$SIDB_TEST_SID" ] || { echo "$(sidb_case_label): expected SID '$SIDB_TEST_SID', got '$spec_sid'" >&2; exit 1; }
[ "$spec_pdb" = "$SIDB_TEST_PDB" ] || { echo "$(sidb_case_label): expected PDB '$SIDB_TEST_PDB', got '$spec_pdb'" >&2; exit 1; }
[ "$spec_image" = "$SIDB_IMAGE" ] || { echo "$(sidb_case_label): expected image '$SIDB_IMAGE', got '$spec_image'" >&2; exit 1; }
[ -n "$connect_string" ] || { echo "$(sidb_case_label): status.connectString is empty" >&2; exit 1; }

# After enable-tcps.sh, test01 must expose TCPS on 2484 with a client wallet.
spec_tcps_enabled="$(sidb_case_kubectl -n "$SIDB_CASE_NAMESPACE" get singleinstancedatabase "$SIDB_TEST_NAME" -o jsonpath='{.spec.security.tcps.enabled}')"
spec_tcps_secret="$(sidb_case_kubectl -n "$SIDB_CASE_NAMESPACE" get singleinstancedatabase "$SIDB_TEST_NAME" -o jsonpath='{.spec.security.tcps.tlsSecret}')"
status_tcps_enabled="$(sidb_case_kubectl -n "$SIDB_CASE_NAMESPACE" get singleinstancedatabase "$SIDB_TEST_NAME" -o jsonpath='{.status.isTcpsEnabled}')"
tcps_connect_string="$(sidb_case_kubectl -n "$SIDB_CASE_NAMESPACE" get singleinstancedatabase "$SIDB_TEST_NAME" -o jsonpath='{.status.tcpsConnectString}')"
wallet_loc="$(sidb_case_kubectl -n "$SIDB_CASE_NAMESPACE" get singleinstancedatabase "$SIDB_TEST_NAME" -o jsonpath='{.status.clientWalletLoc}')"
expected_tcps_secret="${SIDB_TCPS_TLS_SECRET:-sidb-e2e-tcps-tls-test01}"
[ "$spec_tcps_enabled" = "true" ] || { echo "$(sidb_case_label): expected TCPS enabled via patch, got spec.security.tcps.enabled='$spec_tcps_enabled'" >&2; exit 1; }
[ "$spec_tcps_secret" = "$expected_tcps_secret" ] || { echo "$(sidb_case_label): unexpected TCPS TLS Secret '$spec_tcps_secret'" >&2; exit 1; }
[ "$status_tcps_enabled" = "true" ] || { echo "$(sidb_case_label): status.isTcpsEnabled is '$status_tcps_enabled'" >&2; exit 1; }
[ -n "$tcps_connect_string" ] || { echo "$(sidb_case_label): status.tcpsConnectString is empty" >&2; exit 1; }
[[ "$tcps_connect_string" == *":2484"* || "$tcps_connect_string" == *"2484/"* ]] || {
  echo "$(sidb_case_label): expected TCPS port 2484 in '$tcps_connect_string'" >&2
  exit 1
}
[ -n "$wallet_loc" ] || { echo "$(sidb_case_label): status.clientWalletLoc is empty" >&2; exit 1; }

mapfile -t pods < <(sidb_case_kubectl -n "$SIDB_CASE_NAMESPACE" get pods --no-headers \
  -o custom-columns=NAME:.metadata.name | awk -v needle="$SIDB_TEST_NAME" 'index($1, needle) > 0 {print $1}')
[ "${#pods[@]}" -gt 0 ] || { echo "$(sidb_case_label): no database Pod found" >&2; exit 1; }
for pod in "${pods[@]}"; do
  sidb_case_kubectl -n "$SIDB_CASE_NAMESPACE" wait --for=condition=Ready \
    "pod/$pod" --timeout=60s >/dev/null
done
sidb_case_assert_image_usage "${pods[@]}"

pvc_count="$(sidb_case_kubectl -n "$SIDB_CASE_NAMESPACE" get pvc --no-headers 2>/dev/null |
  awk -v needle="$SIDB_TEST_NAME" 'index($1, needle) > 0 {count++} END {print count+0}')"
service_count="$(sidb_case_kubectl -n "$SIDB_CASE_NAMESPACE" get svc --no-headers 2>/dev/null |
  awk -v needle="$SIDB_TEST_NAME" 'index($1, needle) > 0 {count++} END {print count+0}')"
[ "$pvc_count" -gt 0 ] || { echo "$(sidb_case_label): no SIDB PVC found" >&2; exit 1; }
[ "$service_count" -gt 0 ] || { echo "$(sidb_case_label): no SIDB Service found" >&2; exit 1; }

service_ports="$(sidb_case_kubectl -n "$SIDB_CASE_NAMESPACE" get svc "$SIDB_TEST_NAME" \
  -o jsonpath='{range .spec.ports[*]}{.port}{"\n"}{end}' 2>/dev/null || true)"
printf '%s\n' "$service_ports" | grep -qx '2484' || {
  echo "$(sidb_case_label): Service $SIDB_TEST_NAME does not publish port 2484" >&2
  exit 1
}

sidb_case_collect_diagnostics passed
echo "$(sidb_case_label) topology assertions passed: status=$status role=$role tcps=true port=2484 pods=${#pods[@]} pvcs=$pvc_count services=$service_count"
