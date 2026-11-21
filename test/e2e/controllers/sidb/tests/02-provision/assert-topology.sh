#!/usr/bin/env bash
set -Eeuo pipefail

script_dir="$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC1091
# Test02-specific assertions include TDE, TCPS, sizing, and MAX_STRING_SIZE checks.
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

if [ "${SIDB_CASE_ID:-test01}" = "test02" ]; then
  spec_tcps_enabled="$(sidb_case_kubectl -n "$SIDB_CASE_NAMESPACE" get singleinstancedatabase "$SIDB_TEST_NAME" -o jsonpath='{.spec.security.tcps.enabled}')"
  spec_tcps_secret="$(sidb_case_kubectl -n "$SIDB_CASE_NAMESPACE" get singleinstancedatabase "$SIDB_TEST_NAME" -o jsonpath='{.spec.security.tcps.tlsSecret}')"
  spec_tde_secret="$(sidb_case_kubectl -n "$SIDB_CASE_NAMESPACE" get singleinstancedatabase "$SIDB_TEST_NAME" -o jsonpath='{.spec.security.secrets.tde.secretName}')"
  status_tcps_enabled="$(sidb_case_kubectl -n "$SIDB_CASE_NAMESPACE" get singleinstancedatabase "$SIDB_TEST_NAME" -o jsonpath='{.status.isTcpsEnabled}')"
  spec_sga="$(sidb_case_kubectl -n "$SIDB_CASE_NAMESPACE" get singleinstancedatabase "$SIDB_TEST_NAME" -o jsonpath='{.spec.initParams.sgaTarget}')"
  spec_pga="$(sidb_case_kubectl -n "$SIDB_CASE_NAMESPACE" get singleinstancedatabase "$SIDB_TEST_NAME" -o jsonpath='{.spec.initParams.pgaAggregateTarget}')"
  spec_processes="$(sidb_case_kubectl -n "$SIDB_CASE_NAMESPACE" get singleinstancedatabase "$SIDB_TEST_NAME" -o jsonpath='{.spec.initParams.processes}')"
  spec_max_string_size="$(sidb_case_kubectl -n "$SIDB_CASE_NAMESPACE" get singleinstancedatabase "$SIDB_TEST_NAME" -o jsonpath='{.spec.pdbParameters[0].value}')"
  resource_cpu="$(sidb_case_kubectl -n "$SIDB_CASE_NAMESPACE" get singleinstancedatabase "$SIDB_TEST_NAME" -o jsonpath='{.spec.resources.limits.cpu}')"
  resource_memory="$(sidb_case_kubectl -n "$SIDB_CASE_NAMESPACE" get singleinstancedatabase "$SIDB_TEST_NAME" -o jsonpath='{.spec.resources.limits.memory}')"
  [ "$spec_tcps_enabled" = "true" ] || { echo "$(sidb_case_label): TCPS is not enabled in spec" >&2; exit 1; }
  [ "$spec_tcps_secret" = "$SIDB_TCPS_TLS_SECRET" ] || { echo "$(sidb_case_label): unexpected TCPS TLS Secret '$spec_tcps_secret'" >&2; exit 1; }
  [ "$spec_tde_secret" = "$SIDB_TDE_SECRET" ] || { echo "$(sidb_case_label): unexpected TDE Secret '$spec_tde_secret'" >&2; exit 1; }
  [ "$status_tcps_enabled" = "true" ] || { echo "$(sidb_case_label): status.isTcpsEnabled is '$status_tcps_enabled'" >&2; exit 1; }
  [ "$spec_sga" = "3072" ] || { echo "$(sidb_case_label): expected SGA 3072 MB, got '$spec_sga'" >&2; exit 1; }
  [ "$spec_pga" = "1024" ] || { echo "$(sidb_case_label): expected PGA 1024 MB, got '$spec_pga'" >&2; exit 1; }
  [ "$spec_processes" = "1000" ] || { echo "$(sidb_case_label): expected processes 1000, got '$spec_processes'" >&2; exit 1; }
  [ "$spec_max_string_size" = "EXTENDED" ] || { echo "$(sidb_case_label): expected MAX_STRING_SIZE EXTENDED, got '$spec_max_string_size'" >&2; exit 1; }
  [ "$resource_cpu" = "2" ] || { echo "$(sidb_case_label): expected CPU limit 2, got '$resource_cpu'" >&2; exit 1; }
  [ "$resource_memory" = "8Gi" ] || { echo "$(sidb_case_label): expected memory limit 8Gi, got '$resource_memory'" >&2; exit 1; }
fi

mapfile -t pods < <(sidb_case_kubectl -n "$SIDB_CASE_NAMESPACE" get pods --no-headers \
  -o custom-columns=NAME:.metadata.name | awk -v needle="$SIDB_TEST_NAME" 'index($1, needle) > 0 {print $1}')
[ "${#pods[@]}" -gt 0 ] || { echo "$(sidb_case_label): no database Pod found" >&2; exit 1; }
for pod in "${pods[@]}"; do
  sidb_case_kubectl -n "$SIDB_CASE_NAMESPACE" wait --for=condition=Ready \
    "pod/$pod" --timeout=60s >/dev/null
done
sidb_case_assert_image_usage "${pods[@]}"

# Manager requirement: verify the live PDB parameter, not only the CR field.
# A CR-only check can pass while the PDB still has STANDARD.
if [ "${SIDB_CASE_ID:-test01}" = "test02" ]; then
  live_pod="${pods[0]}"
  live_container="$SIDB_TEST_NAME"
  [[ "$SIDB_TEST_PDB" =~ ^[A-Za-z][A-Za-z0-9_]*$ ]] || {
    echo "$(sidb_case_label): invalid PDB name for SQL check: $SIDB_TEST_PDB" >&2
    exit 1
  }
  mkdir -p "$SIDB_CASE_RESULTS_DIR"
  sql_stderr_file="$SIDB_CASE_RESULTS_DIR/live-max-string-size.stderr"
  # Pin the database container so kubectl does not emit "Defaulted container..."
  # on stderr, and keep SQL*Plus chatter out of the parsed value.
  if ! sql_output="$(sidb_case_kubectl -n "$SIDB_CASE_NAMESPACE" exec \
    -c "$live_container" "$live_pod" -- \
    bash -lc "sqlplus -s / as sysdba <<SQL
WHENEVER SQLERROR EXIT FAILURE
SET HEADING OFF FEEDBACK OFF VERIFY OFF ECHO OFF PAGESIZE 0 TRIMSPOOL ON
ALTER SESSION SET CONTAINER=${SIDB_TEST_PDB};
SELECT value FROM v\\\$parameter WHERE name = 'max_string_size';
EXIT;
SQL" 2>"$sql_stderr_file")"; then
    echo "$(sidb_case_label): live MAX_STRING_SIZE query failed in Pod $live_pod container $live_container" >&2
    printf 'stdout:\n%s\n' "$sql_output" >&2
    printf 'stderr:\n' >&2
    cat "$sql_stderr_file" >&2 || true
    exit 1
  fi
  live_max_string_size="$(
    printf '%s\n' "$sql_output" |
      tr -d '\r' |
      awk '
        {
          line = $0
          gsub(/^[[:space:]]+|[[:space:]]+$/, "", line)
          upper = toupper(line)
          if (upper == "EXTENDED" || upper == "STANDARD") {
            print upper
            found = 1
            exit
          }
        }
        END {
          if (!found) {
            exit 1
          }
        }
      '
  )" || live_max_string_size=""
  {
    printf 'pod=%s\n' "$live_pod"
    printf 'container=%s\n' "$live_container"
    printf 'pdb=%s\n' "$SIDB_TEST_PDB"
    printf 'max_string_size=%s\n' "${live_max_string_size:-<empty>}"
    printf 'raw_stdout=\n'
    printf '%s\n' "$sql_output"
    printf 'raw_stderr=\n'
    cat "$sql_stderr_file" 2>/dev/null || true
  } > "$SIDB_CASE_RESULTS_DIR/live-max-string-size.txt"
  [ "$live_max_string_size" = "EXTENDED" ] || {
    echo "$(sidb_case_label): live PDB MAX_STRING_SIZE is '${live_max_string_size:-<empty>}', expected EXTENDED" >&2
    printf 'stdout:\n%s\n' "$sql_output" >&2
    printf 'stderr:\n' >&2
    cat "$sql_stderr_file" >&2 || true
    exit 1
  }
  echo "$(sidb_case_label): live PDB MAX_STRING_SIZE=EXTENDED verified in $live_pod/$SIDB_TEST_PDB"
fi

pvc_count="$(sidb_case_kubectl -n "$SIDB_CASE_NAMESPACE" get pvc --no-headers 2>/dev/null |
  awk -v needle="$SIDB_TEST_NAME" 'index($1, needle) > 0 {count++} END {print count+0}')"
service_count="$(sidb_case_kubectl -n "$SIDB_CASE_NAMESPACE" get svc --no-headers 2>/dev/null |
  awk -v needle="$SIDB_TEST_NAME" 'index($1, needle) > 0 {count++} END {print count+0}')"
[ "$pvc_count" -gt 0 ] || { echo "$(sidb_case_label): no SIDB PVC found" >&2; exit 1; }
[ "$service_count" -gt 0 ] || { echo "$(sidb_case_label): no SIDB Service found" >&2; exit 1; }

sidb_case_collect_diagnostics passed
echo "$(sidb_case_label) topology assertions passed: status=$status role=$role pods=${#pods[@]} pvcs=$pvc_count services=$service_count"
