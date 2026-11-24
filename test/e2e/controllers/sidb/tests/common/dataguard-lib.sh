#!/usr/bin/env bash
# Shared multi-database helpers; source common/lib.sh first.

sidb_dg_init_config() {
  sidb_case_load_env
  sidb_case_init_config
  : "${SIDB_DG_BROKER_TIMEOUT_SECONDS:=$SIDB_TIMEOUT_SECONDS}"
  # ORA-16525 can be transient while the primary finishes enabling Broker
  # after both SIDBs report Healthy. Keep retrying long enough for that
  # primary-side initialization, while the overall broker timeout remains the
  # terminal bound for a genuinely failed configuration.
  : "${SIDB_DG_ERROR_GRACE_SECONDS:=${SIDB_UNHEALTHY_GRACE_SECONDS:-900}}"
  : "${SIDB_DG_RENDER_TIMEOUT_SECONDS:=$SIDB_TIMEOUT_SECONDS}"
  : "${SIDB_DG_BROKER_HEALTH_FIELD:=status}"
  : "${SIDB_DG_RENDERED_STANDBY:=$SIDB_CASE_RESULTS_DIR/standby-rendered.yaml}"
  : "${SIDB_DG_RENDERED_BROKER:=$SIDB_CASE_RESULTS_DIR/dataguardbroker-rendered.yaml}"
  export SIDB_DG_RENDERED_STANDBY SIDB_DG_RENDERED_BROKER
  sidb_case_validate_config
  sidb_case_require_command jq
  local value
  for value in SIDB_TIMEOUT_SECONDS SIDB_POLL_INTERVAL SIDB_DG_BROKER_TIMEOUT_SECONDS SIDB_DG_RENDER_TIMEOUT_SECONDS SIDB_DG_ERROR_GRACE_SECONDS; do
    [[ "${!value}" =~ ^[1-9][0-9]*$ ]] || {
      echo "$(sidb_case_label): $value must be a positive integer" >&2; return 2;
    }
  done
  [[ "$SIDB_DG_BROKER_HEALTH_FIELD" =~ ^[a-zA-Z][a-zA-Z0-9]*$ ]] || return 2
  [[ "$SIDB_EDITION" = enterprise && "$SIDB_REPLICAS" = 1 ]] || {
    echo "$(sidb_case_label): basic Data Guard requires enterprise and replicas=1" >&2; return 2;
  }
  [ "$SIDB_DG_PRIMARY_NAME" != "$SIDB_DG_STANDBY_NAME" ] &&
    [ "$SIDB_TEST_SID" != "$SIDB_DG_STANDBY_SID" ] || {
      echo "$(sidb_case_label): primary and standby names/SIDs must differ" >&2; return 2;
    }
  # Reuse the common scalar/name/SID validation for the standby values too.
  ( SIDB_TEST_NAME="$SIDB_DG_STANDBY_NAME"; SIDB_TEST_SID="$SIDB_DG_STANDBY_SID"
    SIDB_TEST_PDB="$SIDB_DG_STANDBY_PDB"; SIDB_STORAGE_SIZE="$SIDB_DG_STANDBY_STORAGE_SIZE"
    sidb_case_validate_config )
  [[ "$SIDB_DG_BROKER_NAME" =~ ^[a-z0-9][a-z0-9.-]*$ ]] || return 2
}

sidb_dg_write_env() {
  sidb_case_write_env
  local name
  for name in SIDB_DG_PRIMARY_NAME SIDB_DG_STANDBY_NAME SIDB_DG_BROKER_NAME \
    SIDB_DG_STANDBY_SID SIDB_DG_STANDBY_PDB SIDB_DG_STANDBY_STORAGE_SIZE \
    SIDB_DG_STANDBY_MANIFEST SIDB_DG_RENDERED_STANDBY SIDB_DG_RENDERED_BROKER \
    SIDB_DG_BROKER_TIMEOUT_SECONDS SIDB_DG_RENDER_TIMEOUT_SECONDS SIDB_DG_ERROR_GRACE_SECONDS SIDB_DG_BROKER_HEALTH_FIELD \
    SIDB_REQUIRE_HEALTHY SIDB_EXPECTED_ROLE; do
    printf 'export %s=%q\n' "$name" "${!name}" >> "$SIDB_CASE_ENV_FILE"
  done
}

sidb_dg_render_standby() (
  # A subshell keeps the common primary configuration intact.
  SIDB_TEST_NAME="$SIDB_DG_STANDBY_NAME"
  SIDB_TEST_SID="$SIDB_DG_STANDBY_SID"
  SIDB_TEST_PDB="$SIDB_DG_STANDBY_PDB"
  SIDB_STORAGE_SIZE="$SIDB_DG_STANDBY_STORAGE_SIZE"
  if sidb_case_omit_standby_tde_secret_key; then
    export SIDB_RENDER_OMIT_TDE_SECRET_KEY=true
    echo "sidb_dg_render_standby: 19c image detected (or SIDB_OMIT_STANDBY_TDE_SECRET_KEY set); omitting tde.secretKey"
  else
    export SIDB_RENDER_OMIT_TDE_SECRET_KEY=false
  fi
  sidb_case_render_manifest "$SIDB_DG_STANDBY_MANIFEST" "$SIDB_DG_RENDERED_STANDBY"
)

sidb_dg_collect_diagnostics() {
  local reason="${1:-final}" directory="$SIDB_CASE_RESULTS_DIR/diagnostics/dataguard"
  mkdir -p "$directory"
  sidb_case_kubectl -n "$SIDB_CASE_NAMESPACE" get singleinstancedatabase \
    "$SIDB_DG_PRIMARY_NAME" "$SIDB_DG_STANDBY_NAME" -o yaml > "$directory/databases.yaml" 2>&1 || true
  sidb_case_kubectl -n "$SIDB_CASE_NAMESPACE" get dataguardbroker \
    "$SIDB_DG_BROKER_NAME" -o yaml > "$directory/broker.yaml" 2>&1 || true
  sidb_case_collect_diagnostics "$reason"
}

sidb_dg_exit_snapshot() {
  local code=$?
  [ "$code" -eq 0 ] || sidb_dg_collect_diagnostics "${SIDB_CASE_STAGE:-failure}" || true
  return "$code"
}

sidb_dg_wait_rendered_spec() {
  local elapsed=0 json
  while [ "$elapsed" -lt "$SIDB_DG_RENDER_TIMEOUT_SECONDS" ]; do
    json="$(sidb_case_kubectl -n "$SIDB_CASE_NAMESPACE" get singleinstancedatabase \
      "$SIDB_DG_STANDBY_NAME" -o json 2>/dev/null || true)"
    if jq -e '.status.dataguard.readyForBroker == true and
        (.status.dataguard.renderedBrokerSpec.spec | type == "object")' \
        >/dev/null 2>&1 <<< "$json"; then
      # Primary often never publishes readyForBroker on same-cluster TCP; gen_dg
      # synthesizes the primary member by SID when that happens.
      echo "$(sidb_case_label): standby readyForBroker=true with rendered spec"
      return 0
    fi
    echo "$(sidb_case_label): waiting for standby rendered broker spec (${elapsed}s)"
    sleep "$SIDB_POLL_INTERVAL"
    elapsed=$((elapsed + SIDB_POLL_INTERVAL))
  done
  echo "$(sidb_case_label): timed out waiting for standby readyForBroker/renderedBrokerSpec" >&2
  return 1
}

sidb_dg_assert_broker_spec() {
  # Validate the same schema used by the supplied status renderer.
  jq -e --arg primary "$SIDB_DG_PRIMARY_NAME" --arg standby "$SIDB_DG_STANDBY_NAME" \
    --arg psid "$SIDB_TEST_SID" --arg ssid "$SIDB_DG_STANDBY_SID" \
    --arg ns "$SIDB_CASE_NAMESPACE" --arg broker "$SIDB_DG_BROKER_NAME" \
    --arg secret "$SIDB_ADMIN_SECRET" --arg key "$SIDB_ADMIN_SECRET_KEY" '
      . as $doc | .spec.topology as $t |
      .apiVersion == "database.oracle.com/v4" and .kind == "DataguardBroker" and
      .metadata.name == $broker and .metadata.namespace == $ns and
      $t.sourceKind == "SingleInstanceDatabase" and
      $t.sourceRef.name == $standby and $t.sourceRef.namespace == $ns and
      ($t.members | length) == 2 and
      ([$t.members[] | select(.name == $primary and .dbUniqueName == $psid and .role == "PRIMARY")]|length) == 1 and
      ([$t.members[] | select(.name == $standby and .dbUniqueName == $ssid and .role == "PHYSICAL_STANDBY")]|length) == 1 and
      all($t.members[];
        .localRef.name == .name and .localRef.namespace == $ns and
        ((.adminSecretRef // $t.defaults.adminSecretRef) as $s |
          $s.secretName == $secret and $s.secretKey == $key) and
        (.endpoints | length > 0) and
        all(.endpoints[];
          (.host | type == "string" and length > 0 and (contains("REPLACE")|not)) and
          (.port | type == "number" and . > 0) and
          ((.protocol | ascii_upcase) == "TCP" or (.protocol | ascii_upcase) == "TCPS")))
    ' "$1" >/dev/null || {
    echo "$(sidb_case_label): broker topology/secret/TCP(S) endpoint assertion failed: $1" >&2
    return 1
  }
}

# Wait until each SIDB publishes a real dg-client-wallet Secret name and the
# Secret exists. readyForBroker can be true with placeholder wallet names.
sidb_dg_ensure_tcps_client_wallets() {
  local elapsed=0 name secret
  local timeout="${SIDB_DG_WALLET_TIMEOUT_SECONDS:-$SIDB_DG_RENDER_TIMEOUT_SECONDS}"
  while [ "$elapsed" -lt "$timeout" ]; do
    local missing=0
    for name in "$SIDB_DG_PRIMARY_NAME" "$SIDB_DG_STANDBY_NAME"; do
      secret="$(sidb_case_kubectl -n "$SIDB_CASE_NAMESPACE" get singleinstancedatabase "$name" \
        -o jsonpath='{.status.clientWalletSecret}' 2>/dev/null || true)"
      if [ -z "$secret" ] || [[ "$secret" == replace-with-* ]]; then
        echo "$(sidb_case_label): waiting for $name status.clientWalletSecret (${elapsed}s)"
        missing=1
        continue
      fi
      if ! sidb_case_kubectl -n "$SIDB_CASE_NAMESPACE" get secret "$secret" >/dev/null 2>&1; then
        echo "$(sidb_case_label): waiting for Secret $secret from $name (${elapsed}s)"
        missing=1
      fi
    done
    [ "$missing" -eq 0 ] && {
      echo "$(sidb_case_label): TCPS dg-client-wallet Secrets ready for primary and standby"
      return 0
    }
    sleep "$SIDB_POLL_INTERVAL"
    elapsed=$((elapsed + SIDB_POLL_INTERVAL))
  done
  echo "$(sidb_case_label): timed out waiting for TCPS client wallet Secrets" >&2
  return 1
}

# Replace placeholder/missing tcps.clientWalletSecret values using SIDB status.
# Broker reconcile treats a non-empty placeholder as set, then fails when the
# Secret does not exist (Ready=False/ReconcileFailed).
sidb_dg_patch_broker_tcps_wallets() {
  local broker_file="${1:-$SIDB_DG_RENDERED_BROKER}"
  local primary_secret standby_secret patched
  primary_secret="$(sidb_case_kubectl -n "$SIDB_CASE_NAMESPACE" get singleinstancedatabase \
    "$SIDB_DG_PRIMARY_NAME" -o jsonpath='{.status.clientWalletSecret}')"
  standby_secret="$(sidb_case_kubectl -n "$SIDB_CASE_NAMESPACE" get singleinstancedatabase \
    "$SIDB_DG_STANDBY_NAME" -o jsonpath='{.status.clientWalletSecret}')"
  [ -n "$primary_secret" ] && [ -n "$standby_secret" ] || {
    echo "$(sidb_case_label): missing status.clientWalletSecret primary=${primary_secret:-<empty>} standby=${standby_secret:-<empty>}" >&2
    return 1
  }
  [[ "$primary_secret" == replace-with-* || "$standby_secret" == replace-with-* ]] && {
    echo "$(sidb_case_label): refusing placeholder clientWalletSecret primary=$primary_secret standby=$standby_secret" >&2
    return 1
  }

  patched="$(mktemp "${TMPDIR:-/tmp}/sidb-dg-broker-tcps.XXXXXX")"
  jq --arg primary "$SIDB_DG_PRIMARY_NAME" --arg standby "$SIDB_DG_STANDBY_NAME" \
    --arg psecret "$primary_secret" --arg ssecret "$standby_secret" '
      .spec.topology.defaults = (.spec.topology.defaults // {})
      | .spec.topology.defaults.tcps = (.spec.topology.defaults.tcps // {})
      | .spec.topology.defaults.tcps.clientWalletSecret = $ssecret
      | .spec.topology.members |= map(
          . as $m
          | if ($m.name == $primary) or (($m.localRef.name // "") == $primary) then
              .tcps = ((.tcps // {}) + {enabled: true, clientWalletSecret: $psecret})
            elif ($m.name == $standby) or (($m.localRef.name // "") == $standby) then
              .tcps = ((.tcps // {}) + {enabled: true, clientWalletSecret: $ssecret})
            else
              .
            end
        )
      | if (
          ((.spec.topology.defaults.tcps.clientWalletSecret // "") | startswith("replace-with-"))
          or any(.spec.topology.members[]?;
            ((.tcps.clientWalletSecret // "") | startswith("replace-with-")))
        ) then
          error("broker still contains replace-with-* clientWalletSecret placeholders")
        else
          .
        end
    ' "$broker_file" > "$patched"
  mv "$patched" "$broker_file"
  echo "$(sidb_case_label): patched broker TCPS wallets primary=$primary_secret standby=$standby_secret"
}

sidb_dg_generate_broker() {
  NS="$SIDB_CASE_NAMESPACE" PRIMARY_DB="$SIDB_DG_PRIMARY_NAME" \
    BROKER_NAME="$SIDB_DG_BROKER_NAME" BROKER_YAML="$SIDB_DG_RENDERED_BROKER" \
    EXPECTED_STANDBY="$SIDB_DG_STANDBY_NAME" \
    bash "$SIDB_CASE_DIR/../../scripts/gen_dg.sh"
  # gen_dg emits JSON into .yaml; kubectl accepts JSON. Avoid yq flavor dependencies.
  if [ "${SIDB_DG_REQUIRE_TCPS_WALLETS:-false}" = "true" ]; then
    sidb_dg_patch_broker_tcps_wallets "$SIDB_DG_RENDERED_BROKER"
  fi
  sidb_dg_assert_broker_spec "$SIDB_DG_RENDERED_BROKER"
}

# Return 0 when dgmgrl SHOW CONFIGURATION reports SUCCESS on the primary.
# Used when the DataguardBroker CR lags behind a healthy broker configuration
# (common while Configuration Status is WARNING/ORA-16854 then flips to SUCCESS).
sidb_dg_broker_dgmgrl_success() {
  local primary_pod out
  primary_pod="$(sidb_case_kubectl -n "$SIDB_CASE_NAMESPACE" get pods \
    -l "app=$SIDB_DG_PRIMARY_NAME" -o jsonpath='{.items[0].metadata.name}' 2>/dev/null || true)"
  [ -n "$primary_pod" ] || return 1
  out="$(sidb_case_kubectl -n "$SIDB_CASE_NAMESPACE" exec "$primary_pod" -- \
    bash -lc 'printf "show configuration;\n" | dgmgrl /' 2>/dev/null || true)"
  printf '%s\n' "$out" | grep -Eiq 'Configuration Status:[[:space:]]*SUCCESS'
}

sidb_dg_broker_wait_diagnostics() {
  local json="$1" ready_msg runner_ready runner_summary
  ready_msg="$(jq -r '[.status.conditions[]? | select(.type=="Ready") | .message] | first // empty' \
    <<< "$json" 2>/dev/null || true)"
  runner_ready="$(jq -r '[.status.conditions[]? | select(.type=="RunnerReady") | "\(.status)/\(.reason)/\(.message)"] | first // empty' \
    <<< "$json" 2>/dev/null || true)"
  runner_summary="$(sidb_case_kubectl -n "$SIDB_CASE_NAMESPACE" get pods \
    -l "database.oracle.com/dataguard-broker=$SIDB_DG_BROKER_NAME,database.oracle.com/component=execution-runner" \
    -o jsonpath='{range .items[*]}{.metadata.name}{" phase="}{.status.phase}{" waiting="}{range .status.containerStatuses[*]}{.name}={.state.waiting.reason}{" "}{end}{"\n"}{end}' \
    2>/dev/null || true)"
  [ -n "$ready_msg" ] && echo "$(sidb_case_label) broker Ready.message: $ready_msg"
  [ -n "$runner_ready" ] && echo "$(sidb_case_label) broker RunnerReady: $runner_ready"
  if [ -n "$runner_summary" ]; then
    echo "$(sidb_case_label) execution runners:"
    printf '%s\n' "$runner_summary" | sed "s/^/$(sidb_case_label)   /"
  else
    echo "$(sidb_case_label) execution runners: <none>"
  fi
}

sidb_dg_wait_broker() {
  local elapsed=0 unhealthy_since=-1 error_since=-1 json status ready_cond members_configured
  local dgmgrl_success_streak=0
  local runner_restarted=0
  local ready_msg=""
  while [ "$elapsed" -lt "$SIDB_DG_BROKER_TIMEOUT_SECONDS" ]; do
    json="$(sidb_case_kubectl -n "$SIDB_CASE_NAMESPACE" get dataguardbroker \
      "$SIDB_DG_BROKER_NAME" -o json 2>/dev/null || true)"
    status="$(jq -r --arg field "$SIDB_DG_BROKER_HEALTH_FIELD" '.status[$field] // empty' <<< "$json" 2>/dev/null || true)"
    ready_cond="$(jq -r '[.status.conditions[]? | select(.type=="Ready") | "\(.status)/\(.reason)"] | first // empty' <<< "$json" 2>/dev/null || true)"
    ready_msg="$(jq -r '[.status.conditions[]? | select(.type=="Ready") | .message] | first // empty' \
      <<< "$json" 2>/dev/null || true)"
    members_configured="$(jq -r '[.status.resolvedMembers[]? | select((.phase // "")=="Configured")] | length' <<< "$json" 2>/dev/null || true)"
    echo "$(sidb_case_label) broker readiness: elapsed=${elapsed}s status=${status:-<empty>} ready=${ready_cond:-<empty>} configuredMembers=${members_configured:-0} message=${ready_msg:-<empty>}"
    if [ $((elapsed % 120)) -eq 0 ]; then
      sidb_dg_broker_wait_diagnostics "$json"
    fi
    case "$status" in
      Healthy|HEALTHY) return 0;;
      Failed|FAILED|Error|ERROR)
        # A transient broker command error can leave the CR in Error without
        # a controller requeue. Keep the controller retrying during the
        # configured grace period instead of polling stale Error status.
        if [ $((elapsed % 120)) -eq 0 ] && [ "$elapsed" -gt 0 ]; then
          sidb_case_kubectl -n "$SIDB_CASE_NAMESPACE" annotate dataguardbroker \
            "$SIDB_DG_BROKER_NAME" "e2e.oracle.com/broker-error-wait=$(date +%s)" --overwrite >/dev/null 2>&1 || true
        fi
        if sidb_dg_broker_dgmgrl_success; then
          dgmgrl_success_streak=$((dgmgrl_success_streak + 1))
          echo "$(sidb_case_label): dgmgrl Configuration Status=SUCCESS (streak=${dgmgrl_success_streak}) while CR status=${status}"
          if [ "$dgmgrl_success_streak" -ge 3 ]; then
            echo "$(sidb_case_label): accepting broker ready via dgmgrl SUCCESS (CR status still ${status})"
            return 0
          fi
        else
          dgmgrl_success_streak=0
        fi
        [ "$error_since" -ge 0 ] || error_since="$elapsed"
        if [ $((elapsed - error_since)) -lt "$SIDB_DG_ERROR_GRACE_SECONDS" ]; then
          sleep "$SIDB_POLL_INTERVAL"
          elapsed=$((elapsed + SIDB_POLL_INTERVAL))
          continue
        fi
        echo "$(sidb_case_label): broker failure: $status ready=${ready_cond:-<empty>} message=${ready_msg:-<empty>}" >&2
        sidb_dg_broker_wait_diagnostics "$json" >&2 || true
        return 1;;
      Unhealthy|UNHEALTHY)
        error_since=-1
        [ "$unhealthy_since" -ge 0 ] || unhealthy_since="$elapsed"
        # A runner can remain Pending after a transient API/tunnel outage and
        # never recover from its original pod state. Recreate it once after a
        # bounded wait; the controller still must create a Ready runner and
        # the broker must reach Healthy before this test can pass.
        if [ "$runner_restarted" -eq 0 ] && [ "$members_configured" -eq 0 ] &&
          [ "$elapsed" -ge 600 ]; then
          echo "$(sidb_case_label): recreating stuck Data Guard execution runner after ${elapsed}s RunnerPending" >&2
          sidb_dg_broker_wait_diagnostics "$json" >&2 || true
          sidb_case_kubectl -n "$SIDB_CASE_NAMESPACE" delete pod \
            -l "database.oracle.com/dataguard-broker=$SIDB_DG_BROKER_NAME,database.oracle.com/component=execution-runner" \
            --ignore-not-found >/dev/null 2>&1 || true
          runner_restarted=1
        fi
        # Nudge reconcile while waiting for WARNING→SUCCESS status sync.
        if [ $((elapsed % 120)) -eq 0 ] && [ "$elapsed" -gt 0 ]; then
          sidb_case_kubectl -n "$SIDB_CASE_NAMESPACE" annotate dataguardbroker \
            "$SIDB_DG_BROKER_NAME" "e2e.oracle.com/broker-wait=$(date +%s)" --overwrite >/dev/null 2>&1 || true
        fi
        if sidb_dg_broker_dgmgrl_success; then
          dgmgrl_success_streak=$((dgmgrl_success_streak + 1))
          echo "$(sidb_case_label): dgmgrl Configuration Status=SUCCESS (streak=${dgmgrl_success_streak}) while CR status=${status}"
          # CR status can lag after SUCCESS; accept after a few consecutive confirms.
          if [ "$dgmgrl_success_streak" -ge 3 ]; then
            echo "$(sidb_case_label): accepting broker ready via dgmgrl SUCCESS (CR status still ${status})"
            return 0
          fi
        else
          dgmgrl_success_streak=0
        fi
        if [ $((elapsed - unhealthy_since)) -ge "$SIDB_UNHEALTHY_GRACE_SECONDS" ]; then
          echo "$(sidb_case_label): broker Unhealthy grace expired message=${ready_msg:-<empty>}" >&2
          sidb_dg_broker_wait_diagnostics "$json" >&2 || true
          return 1
        fi;;
      *)
        unhealthy_since=-1
        error_since=-1
        dgmgrl_success_streak=0
        ;;
    esac
    sleep "$SIDB_POLL_INTERVAL"
    elapsed=$((elapsed + SIDB_POLL_INTERVAL))
  done
  echo "$(sidb_case_label): timed out waiting for Healthy broker message=${ready_msg:-<empty>}" >&2
  sidb_dg_broker_wait_diagnostics "${json:-}" >&2 || true
  return 1
}

sidb_dg_assert_database() {
  local name="$1" role="$2" sid="$3" pdb="$4" create_as="$5" json pods pvcs services
  json="$(sidb_case_kubectl -n "$SIDB_CASE_NAMESPACE" get singleinstancedatabase "$name" -o json)"
  jq -e --arg role "$role" --arg sid "$sid" --arg pdb "$pdb" --arg image "$SIDB_IMAGE" \
    --arg create_as "$create_as" --arg primary "$SIDB_DG_PRIMARY_NAME" '
      (.status.status | ascii_upcase) == "HEALTHY" and
      (.status.role | gsub(" "; "_")) == $role and
      .spec.sid == $sid and .spec.pdbName == $pdb and
      .spec.createAs == $create_as and .spec.image.pullFrom == $image and
      .spec.dataguard.prereqs.enabled == true and
      (.status.connectString | type == "string" and length > 0) and
      (if $create_as == "standby" then .spec.primarySource.databaseRef == $primary
       else .spec.archiveLog == true and .spec.forceLog == true and .spec.flashBack == true end)
    ' >/dev/null <<< "$json" || {
    echo "$(sidb_case_label): $name database assertions failed" >&2; return 1;
  }
  pods="$(sidb_case_kubectl -n "$SIDB_CASE_NAMESPACE" get pods -l "app=$name" -o json)"
  jq -e '.items | length > 0 and all(.[];
      .metadata.deletionTimestamp == null and .status.phase == "Running" and
      any(.status.conditions[]?; .type == "Ready" and .status == "True"))' >/dev/null <<< "$pods" || {
    echo "$(sidb_case_label): $name has no ready database Pod" >&2; return 1;
  }
  # Check PVCs actually mounted by these database pods, avoiding name substring matches.
  pvcs="$(jq -r '[.items[].spec.volumes[]? | .persistentVolumeClaim.claimName // empty] | unique[]' <<< "$pods")"
  [ -n "$pvcs" ] || { echo "$(sidb_case_label): $name has no mounted PVC" >&2; return 1; }
  local pvc
  while IFS= read -r pvc; do
    [ "$(sidb_case_kubectl -n "$SIDB_CASE_NAMESPACE" get pvc "$pvc" -o jsonpath='{.status.phase}')" = Bound ] || {
      echo "$(sidb_case_label): $name PVC $pvc is not Bound" >&2; return 1;
    }
  done <<< "$pvcs"
  services="$(sidb_case_kubectl -n "$SIDB_CASE_NAMESPACE" get svc -o json)"
  jq -e --arg name "$name" 'any(.items[]; .spec.selector.app == $name and (.spec.ports | length > 0))' \
    >/dev/null <<< "$services" || { echo "$(sidb_case_label): $name has no database Service" >&2; return 1; }
  echo "$(sidb_case_label): $name Healthy role=$role; Pods ready, PVCs Bound, Service present"
}