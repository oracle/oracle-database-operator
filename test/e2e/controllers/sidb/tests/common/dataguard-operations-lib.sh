#!/usr/bin/env bash
# Day-2 Data Guard helpers. Source common/lib.sh and common/dataguard-lib.sh first.
# Test03 remains responsible for provisioning; these cases operate on its resources.

sidb_dg_ops_configure() {
  local case_id="$1" case_name="$2" case_dir="$3" prefix variable
  prefix="SIDB_${case_id^^}"
  variable="${prefix}_ENV_FILE"
  export SIDB_CASE_ID="$case_id" SIDB_CASE_NAME="$case_name" SIDB_CASE_DIR="$case_dir"
  export SIDB_CASE_ENV_FILE="${!variable:-${SIDB_CASE_ENV_FILE:-${TMPDIR:-/tmp}/sidb-ns-${case_id}.env}}"
  variable="${prefix}_PRIMARY_NAME"
  export SIDB_DG_PRIMARY_NAME="${!variable:-${SIDB_TEST03_PRIMARY_NAME:-sidb-e2e-primary-test03}}"
  variable="${prefix}_STANDBY_NAME"
  export SIDB_DG_STANDBY_NAME="${!variable:-${SIDB_TEST03_STANDBY_NAME:-sidb-e2e-standby-test03}}"
  variable="${prefix}_BROKER_NAME"
  export SIDB_DG_BROKER_NAME="${!variable:-${SIDB_TEST03_BROKER_NAME:-sidb-e2e-dg-test03}}"
  export SIDB_TEST_NAME="$SIDB_DG_PRIMARY_NAME" SIDB_CASE_CLEANUP=false
}

sidb_dg_ops_init_config() {
  if [ "${1:-}" != --fresh ]; then sidb_case_load_env; fi
  sidb_case_init_config
  sidb_case_require_command kubectl || return
  sidb_case_require_command jq || return
  : "${SIDB_DG_OPERATION_TIMEOUT_SECONDS:=1800}"
  : "${SIDB_DG_FINAL_TIMEOUT_SECONDS:=600}"
  : "${SIDB_DG_PREFLIGHT_TIMEOUT_SECONDS:=600}"
  : "${SIDB_DG_OBSERVER_NAME:=${SIDB_DG_BROKER_NAME}-observer}"
  # Same image as the tested broker runner unless explicitly overridden.
  : "${SIDB_DG_OBSERVER_IMAGE:=}"
  local variable value
  for variable in SIDB_DG_OPERATION_TIMEOUT_SECONDS SIDB_DG_FINAL_TIMEOUT_SECONDS SIDB_DG_PREFLIGHT_TIMEOUT_SECONDS SIDB_POLL_INTERVAL; do
    value="${!variable}"
    [[ "$value" =~ ^[1-9][0-9]*$ ]] || {
      echo "$(sidb_case_label): $variable must be a positive integer" >&2; return 2;
    }
  done
  for variable in SIDB_CASE_NAMESPACE SIDB_DG_PRIMARY_NAME SIDB_DG_STANDBY_NAME SIDB_DG_BROKER_NAME SIDB_DG_OBSERVER_NAME; do
    value="${!variable}"
    [[ "$value" =~ ^[a-z0-9]([a-z0-9.-]*[a-z0-9])?$ ]] && [ "${#value}" -le 253 ] || {
      echo "$(sidb_case_label): invalid Kubernetes name in $variable" >&2; return 2;
    }
  done
  [ "$SIDB_DG_PRIMARY_NAME" != "$SIDB_DG_STANDBY_NAME" ] || return 2
  export SIDB_DG_OPERATION_TIMEOUT_SECONDS SIDB_DG_FINAL_TIMEOUT_SECONDS SIDB_DG_PREFLIGHT_TIMEOUT_SECONDS SIDB_DG_OBSERVER_NAME SIDB_DG_OBSERVER_IMAGE
  mkdir -p "$SIDB_CASE_RESULTS_DIR/operations"
}

sidb_dg_ops_get_broker() {
  sidb_case_kubectl -n "$SIDB_CASE_NAMESPACE" get dataguardbroker "$SIDB_DG_BROKER_NAME" -o json
}

sidb_dg_ops_snapshot() {
  local reason="$1" directory="$SIDB_CASE_RESULTS_DIR/snapshots/$1"
  mkdir -p "$directory"
  sidb_dg_ops_get_broker > "$directory/broker.json" 2>&1 || true
  sidb_case_kubectl -n "$SIDB_CASE_NAMESPACE" get singleinstancedatabase \
    "$SIDB_DG_PRIMARY_NAME" "$SIDB_DG_STANDBY_NAME" -o yaml > "$directory/databases.yaml" 2>&1 || true
  sidb_case_snapshot "$reason"
}

sidb_dg_ops_collect_diagnostics() {
  sidb_dg_ops_snapshot "${1:-final}"
  sidb_dg_collect_diagnostics "${1:-final}"
}

sidb_dg_ops_write_env() {
  local variable
  mkdir -p "$(dirname "$SIDB_CASE_ENV_FILE")"
  (
    umask 077
    : > "$SIDB_CASE_ENV_FILE"
    for variable in SIDB_CASE_ID SIDB_CASE_NAME SIDB_CASE_DIR SIDB_CASE_NAMESPACE \
      SIDB_CASE_RESULTS_DIR SIDB_CASE_ENV_FILE SIDB_TEST_NAME SIDB_CASE_CLEANUP \
      SIDB_DG_PRIMARY_NAME SIDB_DG_STANDBY_NAME SIDB_DG_BROKER_NAME \
      SIDB_DG_OPERATION_TIMEOUT_SECONDS SIDB_DG_FINAL_TIMEOUT_SECONDS SIDB_DG_PREFLIGHT_TIMEOUT_SECONDS SIDB_POLL_INTERVAL SIDB_DG_OBSERVER_NAME \
      SIDB_DG_OBSERVER_IMAGE SIDB_DG_BASE_PRIMARY SIDB_DG_BASE_STANDBY \
      SIDB_DG_BASE_PRIMARY_TARGET SIDB_DG_BASE_STANDBY_TARGET \
      SIDB_DG_BASE_BROKER_UID SIDB_DG_BASE_PRIMARY_UID SIDB_DG_BASE_STANDBY_UID \
      SIDB_DG_RUN_ID; do
      printf 'export %s=%q\n' "$variable" "${!variable}" >> "$SIDB_CASE_ENV_FILE"
    done
  )
}

sidb_dg_ops_operation_record() {
  local broker="$1" operation="$2"
  jq -c --arg op "$operation" '
    [
      (try .status.operations[$op] catch null),
      (try .status.operationStatus[$op] catch null),
      (try .status[$op] catch null),
      (.status.lastOperation | select(type == "object") |
        select((.operation // .type // "") == $op))
    ] | map(select(type == "object") | select(length > 0)) | .[0] // {}
    ' <<< "$broker"
}

sidb_dg_ops_check_idle() {
  local broker="$1" operation record phase legacy_phases
  for operation in switchover roleConversion failover protectionMode; do
    record="$(sidb_dg_ops_operation_record "$broker" "$operation")" || return
    phase="$(jq -r '(.phase // .status // "") | ascii_upcase' <<< "$record")" || return
    case "$phase" in
      RUNNING|PENDING|INPROGRESS|IN_PROGRESS|PROCESSING)
        echo "$(sidb_case_label): another broker operation is active: $operation ($phase)" >&2; return 2;;
    esac
    # Legacy .status.<operation> fields may be strings rather than receipts.
    # They cannot prove completion, but an explicit active phase still blocks a new request.
    legacy_phases="$(jq -r --arg op "$operation" '
      [(try .status.operations[$op] catch null),
       (try .status.operationStatus[$op] catch null),
       (try .status[$op] catch null)][] | select(type == "string") | ascii_upcase
      ' <<< "$broker")" || return
    while IFS= read -r phase; do
      case "$phase" in
        RUNNING|PENDING|INPROGRESS|IN_PROGRESS|PROCESSING)
          echo "$(sidb_case_label): another broker operation is active: $operation ($phase)" >&2; return 2;;
      esac
    done <<< "$legacy_phases"
  done
}

sidb_dg_ops_observer_pods() {
  local pods
  pods="$(sidb_case_kubectl -n "$SIDB_CASE_NAMESPACE" get pods -o json)" || return
  # Exact configured Pod name; also support controller-generated suffixes if owned by this broker.
  jq --arg name "$SIDB_DG_OBSERVER_NAME" --arg uid "${SIDB_DG_BASE_BROKER_UID:-}" '
    {items: [.items[] | select(.metadata.name == $name or
      ((.metadata.name | startswith($name + "-")) and
        any(.metadata.ownerReferences[]?; .uid == $uid)))]}' <<< "$pods"
}

sidb_dg_ops_prepare() {
  local broker primary standby primary_role standby_role
  sidb_case_kubectl get namespace "$SIDB_CASE_NAMESPACE" >/dev/null || return
  broker="$(sidb_dg_ops_get_broker)" || {
    echo "$(sidb_case_label): broker is missing; run sidb/test03 first" >&2; return 2;
  }
  primary="$(sidb_case_kubectl -n "$SIDB_CASE_NAMESPACE" get singleinstancedatabase "$SIDB_DG_PRIMARY_NAME" -o json)" || return
  standby="$(sidb_case_kubectl -n "$SIDB_CASE_NAMESPACE" get singleinstancedatabase "$SIDB_DG_STANDBY_NAME" -o json)" || return
  # Health may fluctuate between cases. Validate ownership/configuration now;
  # verify_prerequisites waits for health and SQL before any operation is issued.
  jq -e --arg p "$SIDB_DG_PRIMARY_NAME" --arg s "$SIDB_DG_STANDBY_NAME" --arg ns "$SIDB_CASE_NAMESPACE" '
    .metadata.deletionTimestamp == null and
    (.status.fastStartFailover | tostring | ascii_downcase) == "false" and
    (.spec.topology.policy.fastStartFailover // false) == false and
    (.spec.topology.observer.enabled // false) == false and
    (.spec.topology.members | length) == 2 and
    ([.spec.topology.members[].localRef.name] | sort) == ([$p, $s] | sort) and
    all(.spec.topology.members[];
      .localRef.kind == "SingleInstanceDatabase" and
      (.localRef.namespace // $ns) == $ns and
      (.dbUniqueName | type == "string" and test("^[A-Za-z0-9_$#]+$")))
    ' >/dev/null <<< "$broker" || {
    echo "$(sidb_case_label): need a local two-member broker with FSFO/observer disabled" >&2; return 2;
  }
  sidb_dg_ops_check_idle "$broker" || return
  primary_role="$(jq -r '.status.role | gsub(" "; "_") | ascii_upcase' <<< "$primary")" || return
  standby_role="$(jq -r '.status.role | gsub(" "; "_") | ascii_upcase' <<< "$standby")" || return
  case "$primary_role/$standby_role" in
    PRIMARY/PHYSICAL_STANDBY)
      SIDB_DG_BASE_PRIMARY="$SIDB_DG_PRIMARY_NAME"; SIDB_DG_BASE_STANDBY="$SIDB_DG_STANDBY_NAME";;
    PHYSICAL_STANDBY/PRIMARY)
      SIDB_DG_BASE_PRIMARY="$SIDB_DG_STANDBY_NAME"; SIDB_DG_BASE_STANDBY="$SIDB_DG_PRIMARY_NAME";;
    *) echo "$(sidb_case_label): need exactly one PRIMARY and one PHYSICAL_STANDBY; got $primary_role/$standby_role" >&2; return 2;;
  esac
  SIDB_DG_BASE_PRIMARY_TARGET="$(jq -r --arg n "$SIDB_DG_BASE_PRIMARY" '.spec.topology.members[] | select(.localRef.name == $n) | .dbUniqueName' <<< "$broker")" || return
  SIDB_DG_BASE_STANDBY_TARGET="$(jq -r --arg n "$SIDB_DG_BASE_STANDBY" '.spec.topology.members[] | select(.localRef.name == $n) | .dbUniqueName' <<< "$broker")" || return
  SIDB_DG_BASE_BROKER_UID="$(jq -r '.metadata.uid' <<< "$broker")" || return
  SIDB_DG_BASE_PRIMARY_UID="$(jq -r '.metadata.uid' <<< "$primary")" || return
  SIDB_DG_BASE_STANDBY_UID="$(jq -r '.metadata.uid' <<< "$standby")" || return
  [ "$SIDB_DG_BASE_PRIMARY_TARGET" != "$SIDB_DG_BASE_STANDBY_TARGET" ] || return 2
  SIDB_DG_OBSERVER_IMAGE="${SIDB_DG_OBSERVER_IMAGE:-$(jq -r '.spec.execution.image // empty' <<< "$broker")}" || return
  SIDB_DG_RUN_ID="e2e-${SIDB_CASE_ID}-${CI_PIPELINE_ID:-local}-${CI_JOB_ID:-local}-$(date -u +%Y%m%dT%H%M%S)-$$-${RANDOM}"
  export SIDB_DG_BASE_PRIMARY SIDB_DG_BASE_STANDBY SIDB_DG_BASE_PRIMARY_TARGET SIDB_DG_BASE_STANDBY_TARGET
  export SIDB_DG_BASE_BROKER_UID SIDB_DG_BASE_PRIMARY_UID SIDB_DG_BASE_STANDBY_UID SIDB_DG_RUN_ID SIDB_DG_OBSERVER_IMAGE
  if [ "$SIDB_CASE_ID" = test06 ]; then
    jq -e '.spec.topology.observer == null' >/dev/null <<< "$broker" || {
      echo "$(sidb_case_label): test06 requires no existing observer configuration" >&2; return 2;
    }
    [ -n "$SIDB_DG_OBSERVER_IMAGE" ] || { echo "Set SIDB_DG_OBSERVER_IMAGE" >&2; return 2; }
    sidb_dg_ops_observer_pods | jq -e '.items | length == 0' >/dev/null || {
      echo "$(sidb_case_label): observer Pod name is already in use" >&2; return 2;
    }
  fi
  sidb_dg_ops_write_env || return
  sidb_dg_ops_snapshot before
  echo "$(sidb_case_label): using broker $SIDB_DG_BROKER_NAME; primary=$SIDB_DG_BASE_PRIMARY ($SIDB_DG_BASE_PRIMARY_TARGET); standby=$SIDB_DG_BASE_STANDBY ($SIDB_DG_BASE_STANDBY_TARGET)"
}

sidb_dg_ops_fetch_state() {
  local directory="$1" broker primary standby
  mkdir -p "$directory"
  broker="$(sidb_dg_ops_get_broker)" || return
  primary="$(sidb_case_kubectl -n "$SIDB_CASE_NAMESPACE" get singleinstancedatabase "$SIDB_DG_PRIMARY_NAME" -o json)" || return
  standby="$(sidb_case_kubectl -n "$SIDB_CASE_NAMESPACE" get singleinstancedatabase "$SIDB_DG_STANDBY_NAME" -o json)" || return
  printf '%s\n' "$broker" > "$directory/broker.json"
  printf '%s\n' "$primary" > "$directory/$SIDB_DG_PRIMARY_NAME.json"
  printf '%s\n' "$standby" > "$directory/$SIDB_DG_STANDBY_NAME.json"
  jq -e --arg uid "$SIDB_DG_BASE_BROKER_UID" '.metadata.uid == $uid and .metadata.deletionTimestamp == null' >/dev/null <<< "$broker" &&
    jq -e --arg uid "$SIDB_DG_BASE_PRIMARY_UID" '.metadata.uid == $uid and .metadata.deletionTimestamp == null' >/dev/null <<< "$primary" &&
    jq -e --arg uid "$SIDB_DG_BASE_STANDBY_UID" '.metadata.uid == $uid and .metadata.deletionTimestamp == null' >/dev/null <<< "$standby" || {
      echo "$(sidb_case_label): a database or broker was replaced/deleted during the test" >&2; return 2;
    }
}

sidb_dg_ops_sql_state() {
  local name="$1" role="$2" directory="$3" fsfo="${4:-}" pods pod container sid result state mrp
  local cdb pdb_count pdb_states pdb_name pdb_mode expected_pdb_mode observed_pdb_count=0
  pods="$(sidb_case_kubectl -n "$SIDB_CASE_NAMESPACE" get pods -l "app=$name" -o json)" || return
  pod="$(jq -r '.items | map(select(.metadata.deletionTimestamp == null and .status.phase == "Running" and
    any(.status.conditions[]?; .type == "Ready" and .status == "True"))) | sort_by(.metadata.name) | .[0].metadata.name // empty' <<< "$pods")" || return
  [ -n "$pod" ] || return 1
  container="$(jq -r --arg pod "$pod" --arg name "$name" '.items[] | select(.metadata.name == $pod) |
    ([.spec.containers[] | select(.name == $name)][0].name //
      (if (.spec.containers | length) == 1 then .spec.containers[0].name else empty end))' <<< "$pods")" || return
  [ -n "$container" ] || return 1
  sid="$(jq -r '.spec.sid' "$directory/$name.json")" || return
  mkdir -p "$directory"
  # OS authentication inside the database container; no password is read or logged.
  sidb_case_kubectl -n "$SIDB_CASE_NAMESPACE" exec -i "$pod" -c "$container" -- \
    env "ORACLE_SID=$sid" bash -c 'exec sqlplus -s "/ as sysdba"' \
    > "$directory/$name-sql.log" 2>&1 <<'SQL' || return 1
whenever oserror exit failure
whenever sqlerror exit failure
set heading off feedback off pagesize 0 linesize 300 trimspool on verify off echo off
select 'E2E_DB_STATE=' || replace(database_role,' ','_') || '|' || replace(open_mode,' ','_') from v$database;
select 'E2E_MRP_COUNT=' || count(*) from v$managed_standby where process like 'MRP%';
select 'E2E_FSFO=' || fs_failover_status || '|' || fs_failover_observer_present from v$database;
select 'E2E_CDB=' || cdb from v$database;
select 'E2E_PDB_COUNT=' || count(*) from v$pdbs where con_id > 2;
select 'E2E_PDB_STATE=' || name || '|' || replace(open_mode,' ','_') from v$pdbs where con_id > 2;
exit
SQL
  result="$(tr -d '\r' < "$directory/$name-sql.log")"
  state="$(sed -n 's/^[[:space:]]*E2E_DB_STATE=//p' <<< "$result")"
  mrp="$(sed -n 's/^[[:space:]]*E2E_MRP_COUNT=//p' <<< "$result" | tr -d '[:space:]')"
  cdb="$(sed -n 's/^[[:space:]]*E2E_CDB=//p' <<< "$result" | tr -d '[:space:]')"
  pdb_count="$(sed -n 's/^[[:space:]]*E2E_PDB_COUNT=//p' <<< "$result" | tr -d '[:space:]')"
  [[ "$cdb" = YES || "$cdb" = NO ]] && [[ "$pdb_count" =~ ^[0-9]+$ ]] || return 1
  case "$role/$state" in
    PRIMARY/PRIMARY\|READ_WRITE) ;;
    SNAPSHOT_STANDBY/SNAPSHOT_STANDBY\|READ_WRITE)
      [ "$mrp" = 0 ] || return 1;;
    PHYSICAL_STANDBY/PHYSICAL_STANDBY\|READ_ONLY_WITH_APPLY)
      [[ "$mrp" =~ ^[0-9]+$ ]] && [ "$mrp" -gt 0 ] || return 1;;
    PHYSICAL_STANDBY/PHYSICAL_STANDBY\|MOUNTED)
      [ "$cdb" = NO ] || return 1
      [[ "$mrp" =~ ^[0-9]+$ ]] && [ "$mrp" -gt 0 ] || return 1;;
    *) return 1;;
  esac
  expected_pdb_mode=READ_WRITE
  [ "$role" != PHYSICAL_STANDBY ] || expected_pdb_mode=READ_ONLY
  pdb_states="$(sed -n 's/^[[:space:]]*E2E_PDB_STATE=//p' <<< "$result")"
  if [ "$pdb_count" -gt 0 ]; then
    while IFS='|' read -r pdb_name pdb_mode; do
      [ -n "$pdb_name" ] && [ "$pdb_mode" = "$expected_pdb_mode" ] || {
        echo "$(sidb_case_label): $name user PDB ${pdb_name:-<missing>} is ${pdb_mode:-<missing>}; expected $expected_pdb_mode" >&2
        return 1
      }
      observed_pdb_count=$((observed_pdb_count + 1))
    done <<< "$pdb_states"
  else
    [ -z "$pdb_states" ] || return 1
    if [ "$cdb" = YES ] && jq -e '(.spec.pdbName // "") != ""' "$directory/$name.json" >/dev/null; then
      echo "$(sidb_case_label): $name has no user PDB although the test configures one" >&2
      return 1
    fi
  fi
  [ "$observed_pdb_count" -eq "$pdb_count" ] || return 1
  if [ -n "$fsfo" ]; then
    local fsfo_state observer_present
    state="$(sed -n 's/^[[:space:]]*E2E_FSFO=//p' <<< "$result")"
    fsfo_state="${state%%|*}"; observer_present="${state#*|}"
    case "$fsfo" in
      true)
        case "$fsfo_state" in SYNCHRONIZED|TARGET\ UNDER\ LAG\ LIMIT) ;; *) return 1;; esac
        [ "$observer_present" = YES ] || return 1;;
      false)
        # Oracle documents other FS_FAILOVER_* columns as irrelevant when DISABLED.
        [ "$fsfo_state" = DISABLED ] || return 1;;
    esac
  fi
}

sidb_dg_ops_state_matches() {
  local directory="$1" expected_primary="$2" expected_standby="$3" standby_role="$4" generation="${5:-0}"
  local fsfo="${6:-}"
  local name role
  jq -e --arg p "$expected_primary" --arg s "$expected_standby" --arg sr "$standby_role" --argjson gen "$generation" '
    .status.status == "Healthy" and .status.primaryDatabaseRef == $p and
    any(.status.conditions[]?; .type == "Ready" and .status == "True" and (.observedGeneration // 0) >= $gen) and
    ([.status.resolvedMembers[] | select(.name == $p and (.role | gsub(" "; "_") | ascii_upcase) == "PRIMARY")] | length) == 1 and
    ([.status.resolvedMembers[] | select(.name == $s and (.role | gsub(" "; "_") | ascii_upcase) == $sr)] | length) == 1
    ' "$directory/broker.json" >/dev/null || return 1
  for name in "$expected_primary" "$expected_standby"; do
    role=PRIMARY; [ "$name" != "$expected_standby" ] || role="$standby_role"
    jq -e --arg role "$role" '.status.status == "Healthy" and
      (.status.role | gsub(" "; "_") | ascii_upcase) == $role' "$directory/$name.json" >/dev/null || return 1
    sidb_dg_ops_sql_state "$name" "$role" "$directory" "$fsfo" || return 1
  done
}

# Return 2 for an unsafe baseline; return 1 for readiness that may recover.
sidb_dg_ops_check_preflight() {
  local directory="$1" broker_generation
  sidb_dg_ops_fetch_state "$directory" || return
  sidb_dg_ops_check_idle "$(cat "$directory/broker.json")" || return
  jq -e '(.status.fastStartFailover | tostring | ascii_downcase) == "false" and
    (.spec.topology.policy.fastStartFailover // false) == false and
    (.spec.topology.observer.enabled // false) == false' "$directory/broker.json" >/dev/null || {
    echo "$(sidb_case_label): preflight requires FSFO and observer disabled" >&2; return 2;
  }
  broker_generation="$(jq -r '.metadata.generation' "$directory/broker.json")" || return 2
  sidb_dg_ops_state_matches "$directory" "$SIDB_DG_BASE_PRIMARY" "$SIDB_DG_BASE_STANDBY" \
    PHYSICAL_STANDBY "$broker_generation" false || {
    echo "$(sidb_case_label): waiting for broker/database health, current Ready generation, roles, Pods and SQL (including PDBs and FSFO disabled)" >&2
    return 1
  }
}

sidb_dg_ops_verify_prerequisites() {
  [ -n "${SIDB_DG_RUN_ID:-}" ] || { echo "Run prerequisites.sh first" >&2; return 2; }
  local directory="$SIDB_CASE_RESULTS_DIR/operations/preflight"
  local start=$SECONDS stable=0 preflight_rc remaining delay
  mkdir -p "$directory"
  while [ $((SECONDS - start)) -lt "$SIDB_DG_PREFLIGHT_TIMEOUT_SECONDS" ]; do
    # Discard SQL from earlier polls so a failed status check cannot expose stale SQL.
    rm -f -- "$directory/"*-sql.log
    if sidb_dg_ops_check_preflight "$directory" > "$directory/check.log" 2>&1; then
      stable=$((stable + 1))
      echo "$(sidb_case_label): preflight elapsed=$((SECONDS - start))s stable-checks=$stable/2" | tee -a "$directory/checks.log"
      if [ "$stable" -ge 2 ]; then
        echo "$(sidb_case_label): prerequisites verified against broker, database status and SQL"
        return 0
      fi
    else
      preflight_rc=$?
      stable=0
      cat "$directory/check.log" >&2
      if [ "$preflight_rc" -eq 2 ]; then
        echo "$(sidb_case_label): preflight rejected unsafe baseline; not retrying" >&2
        return 2
      fi
      echo "$(sidb_case_label): preflight elapsed=$((SECONDS - start))s not ready; retrying" | tee -a "$directory/checks.log"
    fi
    remaining=$((SIDB_DG_PREFLIGHT_TIMEOUT_SECONDS - (SECONDS - start)))
    [ "$remaining" -gt 0 ] || break
    delay="$SIDB_POLL_INTERVAL"
    [ "$delay" -le "$remaining" ] || delay="$remaining"
    sleep "$delay"
  done
  echo "$(sidb_case_label): preflight timed out after ${SIDB_DG_PREFLIGHT_TIMEOUT_SECONDS}s; inspect $directory" >&2
  return 1
}

sidb_dg_ops_wait_operation() {
  local directory="$1" operation="$2" request_id="$3"
  local expected_primary="$4" expected_standby="$5" role="$6" generation="$7"
  local start=$SECONDS broker record reported phase message acknowledged
  local require_receipt=false

  while [ $((SECONDS - start)) -lt "$SIDB_DG_OPERATION_TIMEOUT_SECONDS" ]; do
    if sidb_dg_ops_fetch_state "$directory"; then
      broker="$(cat "$directory/broker.json")" || return
      record="$(sidb_dg_ops_operation_record "$broker" "$operation")" || return
      printf '%s\n' "$record" > "$directory/operation-status.json"

      reported="$(jq -r '.observedRequestId // .requestId // .lastRequestId // .lastProcessedRequestId // empty' <<< "$record")" || return
      phase="$(jq -r '(.phase // .status // "") | ascii_upcase' <<< "$record")" || return
      acknowledged=false

      if [ "$reported" = "$request_id" ]; then
        case "$phase" in
          FAILED|ERROR|REJECTED|CANCELLED|CANCELED)
            message="$(jq -r '.message // .reason // empty' <<< "$record")" || return
            if [[ ( "$operation" == switchover || "$operation" == roleConversion ) &&
                  ( "$phase" == FAILED || "$phase" == ERROR ) &&
                  "$message" == *"unable to upgrade connection: error dialing backend: EOF"* ]]; then
              require_receipt=true
              echo "$(sidb_case_label): $operation request $request_id: $message; waiting for reconciliation" >&2
            else
              echo "$(sidb_case_label): $operation request $request_id failed: $message" >&2
              return 1
            fi
            ;;
          SUCCEEDED|SUCCESS|COMPLETED)
            acknowledged=true
            ;;
        esac
      elif [[ "$record" == '{}' && "$require_receipt" == false ]]; then
        # Preserve compatibility with builds that never publish a receipt.
        acknowledged=true
      fi

      echo "$(sidb_case_label): $operation elapsed=$((SECONDS - start))s request=${reported:-not-published} phase=${phase:-not-published}"
      if $acknowledged && jq -e --arg op "$operation" --arg id "$request_id" \
          '.spec.operations[$op].requestId == $id' "$directory/broker.json" >/dev/null &&
        sidb_dg_ops_state_matches "$directory" "$expected_primary" "$expected_standby" "$role" "$generation"; then
        printf '%s\n' "$request_id" > "$directory/completed-request-id.txt"
        echo "$(sidb_case_label): $operation completed and database roles verified"
        return 0
      fi
    else
      local code=$?
      [ "$code" -ne 2 ] || return 2
    fi
    sleep "$SIDB_POLL_INTERVAL"
  done

  echo "$(sidb_case_label): timed out waiting for $operation request $request_id; see $directory" >&2
  return 1
}

sidb_dg_ops_request() {
  local step="$1" operation="$2" target="$3" expected_primary="$4" expected_standby="$5" role="$6"
  local directory="$SIDB_CASE_RESULTS_DIR/operations/$step" request_id="${SIDB_DG_RUN_ID}-$step" generation
  mkdir -p "$directory"
  sidb_dg_ops_fetch_state "$directory/before" || return
  sidb_dg_ops_check_idle "$(cat "$directory/before/broker.json")" || return
  jq -n --arg op "$operation" --arg target "$target" --arg id "$request_id" --arg role "$role" '
    {spec:{operations:{($op):({target:$target,requestId:$id} +
      (if $op == "roleConversion" then {role:$role} else {} end))}}}' > "$directory/patch.json" || return
  sidb_case_kubectl -n "$SIDB_CASE_NAMESPACE" patch dataguardbroker "$SIDB_DG_BROKER_NAME" \
    --type=merge --patch-file "$directory/patch.json" -o json > "$directory/patch-response.json" || return
  jq -e --arg op "$operation" --arg id "$request_id" '.spec.operations[$op].requestId == $id' \
    "$directory/patch-response.json" >/dev/null || {
    echo "$(sidb_case_label): API did not persist spec.operations.$operation; check installed CRD/operator support" >&2; return 2;
  }
  generation="$(jq -r '.metadata.generation' "$directory/patch-response.json")" || return
  sidb_dg_ops_wait_operation "$directory" "$operation" "$request_id" "$expected_primary" "$expected_standby" "$role" "$generation" || return
  sidb_dg_ops_snapshot "$step"
}

sidb_dg_ops_wait_fsfo() {
  local directory="$1" expected="$2" generation="$3" start=$SECONDS pods count observer_ready
  while [ $((SECONDS - start)) -lt "$SIDB_DG_OPERATION_TIMEOUT_SECONDS" ]; do
    if sidb_dg_ops_fetch_state "$directory"; then
      pods="$(sidb_dg_ops_observer_pods)" || return
      printf '%s\n' "$pods" > "$directory/observer-pods.json"
      count="$(jq '.items | length' <<< "$pods")" || return
      echo "$(sidb_case_label): FSFO expected=$expected elapsed=$((SECONDS - start))s observer-pods=$count"
      observer_ready=false
      if [ "$expected" = true ]; then
        if jq -e '.items | length > 0 and all(.[]; .metadata.deletionTimestamp == null and
          .status.phase == "Running" and any(.status.conditions[]?; .type == "Ready" and .status == "True"))' \
          >/dev/null <<< "$pods"; then observer_ready=true; fi
      elif [ "$count" -eq 0 ]; then observer_ready=true; fi
      if $observer_ready && jq -e --arg expected "$expected" '
          (.status.fastStartFailover | tostring | ascii_downcase) == $expected and
          (.spec.topology.policy.fastStartFailover | tostring) == $expected and
          (if $expected == "true" then .spec.topology.observer.enabled == true else .spec.topology.observer == null end)' \
          "$directory/broker.json" >/dev/null &&
        sidb_dg_ops_state_matches "$directory" "$SIDB_DG_BASE_PRIMARY" "$SIDB_DG_BASE_STANDBY" PHYSICAL_STANDBY "$generation" &&
        sidb_dg_ops_sql_state "$SIDB_DG_BASE_PRIMARY" PRIMARY "$directory" "$expected" &&
        sidb_dg_ops_sql_state "$SIDB_DG_BASE_STANDBY" PHYSICAL_STANDBY "$directory" "$expected"; then
        printf '%s|%s\n' "$SIDB_DG_RUN_ID" "$expected" > "$directory/completed-fsfo-state.txt"
        echo "$(sidb_case_label): FSFO=$expected confirmed in broker and both databases; observer state verified"
        return 0
      fi
    else
      local code=$?
      [ "$code" -ne 2 ] || return 2
    fi
    sleep "$SIDB_POLL_INTERVAL"
  done
  echo "$(sidb_case_label): timed out verifying FSFO=$expected; see $directory" >&2
  return 1
}

sidb_dg_ops_fsfo_request() {
  local step="$1" enabled="$2" directory="$SIDB_CASE_RESULTS_DIR/operations/$1" generation
  mkdir -p "$directory"
  sidb_dg_ops_fetch_state "$directory/before" || return
  sidb_dg_ops_check_idle "$(cat "$directory/before/broker.json")" || return
  jq -n --argjson enabled "$enabled" --arg name "$SIDB_DG_OBSERVER_NAME" --arg image "$SIDB_DG_OBSERVER_IMAGE" '
    {spec:{topology:{policy:{fastStartFailover:$enabled},observer:
      (if $enabled then {enabled:true,name:$name,image:$image} else null end)}}}' > "$directory/patch.json" || return
  sidb_case_kubectl -n "$SIDB_CASE_NAMESPACE" patch dataguardbroker "$SIDB_DG_BROKER_NAME" \
    --type=merge --patch-file "$directory/patch.json" -o json > "$directory/patch-response.json" || return
  jq -e --argjson enabled "$enabled" '.spec.topology.policy.fastStartFailover == $enabled and
    (if $enabled then .spec.topology.observer.enabled == true else .spec.topology.observer == null end)' \
    "$directory/patch-response.json" >/dev/null || {
    echo "$(sidb_case_label): API did not persist FSFO/observer patch; check installed CRD/operator support" >&2; return 2;
  }
  generation="$(jq -r '.metadata.generation' "$directory/patch-response.json")" || return
  sidb_dg_ops_wait_fsfo "$directory" "$enabled" "$generation" || return
  if [ "$enabled" = true ]; then
    # Keep the observer's logs before the disable step removes its Pod.
    sidb_case_collect_pod_logs "$SIDB_CASE_NAMESPACE" "$directory/pod-logs"
  fi
  sidb_dg_ops_snapshot "$step"
}

sidb_dg_ops_execute_case() {
  sidb_dg_ops_verify_prerequisites || return
  case "$SIDB_CASE_ID" in
    test04)
      sidb_dg_ops_request 01-switchover switchover "$SIDB_DG_BASE_STANDBY_TARGET" \
        "$SIDB_DG_BASE_STANDBY" "$SIDB_DG_BASE_PRIMARY" PHYSICAL_STANDBY || return
      sidb_dg_ops_request 02-switchback switchover "$SIDB_DG_BASE_PRIMARY_TARGET" \
        "$SIDB_DG_BASE_PRIMARY" "$SIDB_DG_BASE_STANDBY" PHYSICAL_STANDBY || return;;
    test05)
      sidb_dg_ops_request 01-snapshot roleConversion "$SIDB_DG_BASE_STANDBY_TARGET" \
        "$SIDB_DG_BASE_PRIMARY" "$SIDB_DG_BASE_STANDBY" SNAPSHOT_STANDBY || return
      sidb_dg_ops_request 02-physical roleConversion "$SIDB_DG_BASE_STANDBY_TARGET" \
        "$SIDB_DG_BASE_PRIMARY" "$SIDB_DG_BASE_STANDBY" PHYSICAL_STANDBY || return;;
    test06)
      sidb_dg_ops_fsfo_request 01-enable-fsfo true || return
      sidb_dg_ops_fsfo_request 02-disable-fsfo false || return;;
    *) return 2;;
  esac
}

sidb_dg_ops_assert_workload() {
  local name="$1" pods claims claim
  pods="$(sidb_case_kubectl -n "$SIDB_CASE_NAMESPACE" get pods -l "app=$name" -o json)" || return
  jq -e '.items | length > 0 and all(.[]; .metadata.deletionTimestamp == null and .status.phase == "Running" and
    any(.status.conditions[]?; .type == "Ready" and .status == "True"))' >/dev/null <<< "$pods" || return 1
  claims="$(jq -r '[.items[].spec.volumes[]? | .persistentVolumeClaim.claimName // empty] | unique[]' <<< "$pods")" || return
  [ -n "$claims" ] || return 1
  while IFS= read -r claim; do
    [ "$(sidb_case_kubectl -n "$SIDB_CASE_NAMESPACE" get pvc "$claim" -o jsonpath='{.status.phase}')" = Bound ] || return 1
  done <<< "$claims"
  sidb_case_kubectl -n "$SIDB_CASE_NAMESPACE" get svc -o json |
    jq -e --arg name "$name" 'any(.items[]; .spec.selector.app == $name and (.spec.ports | length > 0))' >/dev/null
}

sidb_dg_ops_wait_final_ready() {
  local directory="$1" start=$SECONDS stable=0 pending generation code
  while [ $((SECONDS - start)) -lt "$SIDB_DG_FINAL_TIMEOUT_SECONDS" ]; do
    pending="resource access"
    if sidb_dg_ops_fetch_state "$directory"; then
      generation="$(jq -r '.metadata.generation' "$directory/broker.json")" || return
      pending="broker/database roles, Ready generation, SQL or database Pod readiness"
      if sidb_dg_ops_state_matches "$directory" "$SIDB_DG_BASE_PRIMARY" "$SIDB_DG_BASE_STANDBY" \
          PHYSICAL_STANDBY "$generation" false; then
        pending="reported FSFO state"
        if jq -e '(.status.fastStartFailover | tostring | ascii_downcase) == "false"' \
            "$directory/broker.json" >/dev/null; then
          pending="primary Pod/PVC/Service readiness"
          if sidb_dg_ops_assert_workload "$SIDB_DG_PRIMARY_NAME"; then
            pending="standby Pod/PVC/Service readiness"
            if sidb_dg_ops_assert_workload "$SIDB_DG_STANDBY_NAME"; then
              pending=""
              if [ "$SIDB_CASE_ID" = test06 ] && ! sidb_dg_ops_observer_pods |
                  jq -e '.items | length == 0' >/dev/null; then
                pending="observer removal"
              fi
            fi
          fi
        fi
      fi
    else
      code=$?
      [ "$code" -ne 2 ] || return 2
    fi
    if [ -z "$pending" ]; then
      stable=$((stable + 1))
      echo "$(sidb_case_label): final readiness elapsed=$((SECONDS - start))s stable-checks=$stable/2"
      if [ "$stable" -ge 2 ]; then return 0; fi
    else
      stable=0
      echo "$(sidb_case_label): final readiness elapsed=$((SECONDS - start))s waiting for $pending"
    fi
    sleep "$SIDB_POLL_INTERVAL"
  done
  echo "$(sidb_case_label): final readiness did not stabilize within ${SIDB_DG_FINAL_TIMEOUT_SECONDS}s; see $directory and diagnostics" >&2
  return 1
}

sidb_dg_ops_assert_final() {
  local step directory="$SIDB_CASE_RESULTS_DIR/operations/final"
  case "$SIDB_CASE_ID" in
    test04) for step in 01-switchover 02-switchback; do
      [ "$(cat "$SIDB_CASE_RESULTS_DIR/operations/$step/completed-request-id.txt")" = "$SIDB_DG_RUN_ID-$step" ] || return 1
    done;;
    test05) for step in 01-snapshot 02-physical; do
      [ "$(cat "$SIDB_CASE_RESULTS_DIR/operations/$step/completed-request-id.txt")" = "$SIDB_DG_RUN_ID-$step" ] || return 1
    done;;
    test06)
      [ "$(cat "$SIDB_CASE_RESULTS_DIR/operations/01-enable-fsfo/completed-fsfo-state.txt")" = "$SIDB_DG_RUN_ID|true" ] &&
        [ "$(cat "$SIDB_CASE_RESULTS_DIR/operations/02-disable-fsfo/completed-fsfo-state.txt")" = "$SIDB_DG_RUN_ID|false" ] || return 1
      sidb_dg_ops_observer_pods | jq -e '.items | length == 0' >/dev/null || return 1;;
    *) return 2;;
  esac
  sidb_dg_ops_wait_final_ready "$directory" || return
  # A final-check retry must capture the recovered state, not reuse failure logs.
  rm -f -- "$SIDB_CASE_RESULTS_DIR/diagnostics/.complete"
  sidb_dg_ops_collect_diagnostics passed
  echo "$(sidb_case_label): both operation checkpoints passed; original roles restored, databases/broker Healthy, Pods ready, PVCs Bound"
}

sidb_dg_ops_exit_snapshot() {
  local code=$?
  if [ "$code" -ne 0 ]; then
    sidb_dg_ops_collect_diagnostics failure || true
    echo "$(sidb_case_label): failure; resources retained in their current state. Inspect operations/ before requesting recovery." >&2
  fi
  return "$code"
}
