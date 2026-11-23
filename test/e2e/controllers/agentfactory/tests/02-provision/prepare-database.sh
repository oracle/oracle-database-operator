#!/usr/bin/env bash
#
# Test02 database preparation hook.
#
# This file is sourced by common/run.sh after the temporary bootstrap
# Secret has been created and before the AgentFactory manifest is rendered.
# It first synchronizes the test02 bootstrap Secret from the admin Secret
# referenced by the dependent SIDB CR. This keeps the database administrator
# password in one test-owned source and avoids requiring a duplicate CI
# variable. When the dependent SIDB has operator-managed TCPS enabled, it then
# copies the SIDB client wallet out of the database Pod and stores the wallet
# zip in the AgentFactory runtime Secret. The controller-created paf-db-setup
# Job uses those credentials to create and validate the PAF users. Without
# SIDB TCPS, the function keeps the password bootstrap flow after the same
# administrator Secret synchronization.

prepare_database_die() {
  echo "Agent Factory test02 database preparation: $*" >&2
  # This file is sourced by the provisioning runner. A preparation failure
  # must stop the runner immediately, including when the caller used the
  # helper in an &&/|| list under `set -e`.
  exit 1
}

prepare_database_sync_sidb_admin_secret() {
  local sidb_admin_secret_name sidb_admin_secret_key
  local sync_status_file

  sidb_admin_secret_name="$(agentfactory_case_kubectl -n "$SIDB_CASE_NAMESPACE" \
    get singleinstancedatabase "$SIDB_TEST_NAME" \
    -o jsonpath='{.spec.security.secrets.admin.secretName}' 2>/dev/null || true)"
  [ -n "$sidb_admin_secret_name" ] ||
    prepare_database_die "SIDB $SIDB_TEST_NAME does not specify spec.security.secrets.admin.secretName"

  sidb_admin_secret_key="$(agentfactory_case_kubectl -n "$SIDB_CASE_NAMESPACE" \
    get singleinstancedatabase "$SIDB_TEST_NAME" \
    -o jsonpath='{.spec.security.secrets.admin.secretKey}' 2>/dev/null || true)"
  sidb_admin_secret_key="${sidb_admin_secret_key:-${SIDB_ADMIN_SECRET_KEY:-oracle_pwd}}"
  [ -n "$sidb_admin_secret_key" ] ||
    prepare_database_die "SIDB $SIDB_TEST_NAME does not specify an admin Secret key"
  command -v jq >/dev/null 2>&1 ||
    prepare_database_die "jq is required to synchronize the SIDB admin Secret without exposing its password"

  # Keep the source password as Secret data throughout the pipeline. It is
  # never assigned to a shell variable, printed, or written to diagnostics.
  if ! agentfactory_case_kubectl -n "$SIDB_CASE_NAMESPACE" \
    get secret "$sidb_admin_secret_name" -o json |
    jq -e \
      --arg source_key "$sidb_admin_secret_key" \
      --arg target_name "$AGENTFACTORY_BOOTSTRAP_SECRET" \
      --arg target_namespace "$AGENTFACTORY_CASE_NAMESPACE" \
      --arg username_key "$AGENTFACTORY_BOOTSTRAP_USERNAME_KEY" \
      --arg password_key "$AGENTFACTORY_BOOTSTRAP_PASSWORD_KEY" \
      --arg username "$AGENTFACTORY_BOOTSTRAP_USERNAME" '
        if (.data[$source_key] // "") == "" then
          error("SIDB admin Secret key is empty or missing")
        else
          {
            apiVersion: "v1",
            kind: "Secret",
            metadata: {name: $target_name, namespace: $target_namespace},
            type: "Opaque",
            data: {
              ($username_key): ($username | @base64),
              ($password_key): .data[$source_key]
            }
          }
        end
      ' |
    agentfactory_case_kubectl -n "$AGENTFACTORY_CASE_NAMESPACE" apply -f - >/dev/null; then
    prepare_database_die "unable to synchronize SIDB admin Secret $sidb_admin_secret_name[$sidb_admin_secret_key] into bootstrap Secret $AGENTFACTORY_BOOTSTRAP_SECRET"
  fi

  sync_status_file="$AGENTFACTORY_CASE_RESULTS_DIR/database-admin-secret-sync.txt"
  printf '%s\n' \
    "source_namespace=$SIDB_CASE_NAMESPACE" \
    "source_secret=$sidb_admin_secret_name" \
    "source_key=$sidb_admin_secret_key" \
    "target_namespace=$AGENTFACTORY_CASE_NAMESPACE" \
    "target_secret=$AGENTFACTORY_BOOTSTRAP_SECRET" \
    "target_username_key=$AGENTFACTORY_BOOTSTRAP_USERNAME_KEY" \
    "target_password_key=$AGENTFACTORY_BOOTSTRAP_PASSWORD_KEY" \
    'password_value=not_recorded' \
    > "$sync_status_file"
  chmod 600 "$sync_status_file"
  if [ -n "${setup_order_file:-}" ]; then
    printf '03a bootstrap-secret status=synchronized source_namespace=%s source_secret=%s source_key=%s target_secret=%s\n' \
      "$SIDB_CASE_NAMESPACE" "$sidb_admin_secret_name" "$sidb_admin_secret_key" \
      "$AGENTFACTORY_BOOTSTRAP_SECRET" >> "$setup_order_file"
  fi
  echo "Agent Factory test02: SIDB admin Secret synchronized for controller database setup"
}

prepare_database_make_wallet_portable() {
  local tnsnames_path="$1"
  local sqlnet_path="${2:-}"
  local connect_host="${3:-}"
  local normalized_path="${tnsnames_path}.normalized"
  local sqlnet_normalized

  # SIDB can generate an absolute MY_WALLET_DIRECTORY pointing back to its
  # own filesystem. That path is invalid in the AgentFactory bootstrap Job
  # and runtime Pod. For this single-wallet archive, sqlnet.ora supplies the
  # wallet location, so remove the source-Pod-specific override.
  awk '
    {
      line = $0
      gsub(/\(MY_WALLET_DIRECTORY[[:space:]]*=[^)]*\)[[:space:]]*/, "", line)
      print line
    }
  ' "$tnsnames_path" > "$normalized_path" ||
    prepare_database_die "failed to make SIDB tnsnames.ora portable"
  chmod --reference="$tnsnames_path" "$normalized_path" 2>/dev/null || true
  mv -- "$normalized_path" "$tnsnames_path"

  # Rewrite HOST to the cross-namespace Service DNS name used by Agent Factory.
  # SIDB client wallets often store "<service>.<namespace>", which is ambiguous
  # from another namespace without relying on cluster DNS search paths.
  if [ -n "$connect_host" ]; then
    [[ "$connect_host" =~ ^[A-Za-z0-9._-]+$ ]] ||
      prepare_database_die "invalid wallet connect host: $connect_host"
    awk -v connect_host="$connect_host" '
      {
        line = $0
        gsub(/\([Hh][Oo][Ss][Tt][[:space:]]*=[^)]*\)/, "(HOST=" connect_host ")", line)
        print line
      }
    ' "$tnsnames_path" > "$normalized_path" ||
      prepare_database_die "failed to rewrite SIDB tnsnames.ora HOST for Agent Factory"
    chmod --reference="$tnsnames_path" "$normalized_path" 2>/dev/null || true
    mv -- "$normalized_path" "$tnsnames_path"
  fi

  # SIDB sqlnet.ora WALLET_LOCATION DIRECTORY usually points at the absolute
  # path inside the SIDB Pod. Point it at the directory that contains the
  # packaged wallet files (".") so consumers that extract the zip flat can
  # open cwallet.sso. paf-db-setup also rewrites this to its extract path.
  if [ -n "$sqlnet_path" ] && [ -r "$sqlnet_path" ]; then
    sqlnet_normalized="${sqlnet_path}.normalized"
    awk '
      {
        line = $0
        gsub(/\([Dd][Ii][Rr][Ee][Cc][Tt][Oo][Rr][Yy][[:space:]]*=[^)]*\)/,
          "(DIRECTORY=.)", line)
        print line
      }
    ' "$sqlnet_path" > "$sqlnet_normalized" ||
      prepare_database_die "failed to make SIDB sqlnet.ora WALLET_LOCATION portable"
    chmod --reference="$sqlnet_path" "$sqlnet_normalized" 2>/dev/null || true
    mv -- "$sqlnet_normalized" "$sqlnet_path"
  fi
}

prepare_database_for_test02() {
  local sidb_tcps_spec sidb_tcps_legacy sidb_tcps_status sidb_wallet_dir sidb_sid
  local sidb_pod="" candidate_pod preferred_alias candidate_upper
  local wallet_alias requested_alias default_alias pdb_alias
  local wallet_source_dir wallet_zip wallet_b64 wallet_patch_file
  local tnsnames_file sqlnet_file cwallet_file ewallet_file
  local zip_bin status_file old_umask
  local -a sidb_pods wallet_files wallet_aliases

  : "${SIDB_CASE_NAMESPACE:=${SIDB_E2E_NAMESPACE:-sidb-ns}}"
  # Prefer an explicitly exported SIDB name (test01 uses sidb-e2e-primary).
  # Fall back to the secure test02 primary when callers leave the name unset.
  : "${SIDB_TEST_NAME:=${SIDB_TEST02_NAME:-sidb-e2e-primary-test02}}"
  : "${SIDB_TEST_PDB:=ORCLPDB1}"
  : "${AGENTFACTORY_WALLET_ZIP_KEY:=db_wallet.zip}"
  export SIDB_CASE_NAMESPACE SIDB_TEST_NAME SIDB_TEST_PDB AGENTFACTORY_WALLET_ZIP_KEY

  status_file="$AGENTFACTORY_CASE_RESULTS_DIR/database-preparation.txt"
  mkdir -p "$AGENTFACTORY_CASE_RESULTS_DIR"

  prepare_database_sync_sidb_admin_secret

  sidb_tcps_spec="$(agentfactory_case_kubectl -n "$SIDB_CASE_NAMESPACE" \
    get singleinstancedatabase "$SIDB_TEST_NAME" \
    -o jsonpath='{.spec.security.tcps.enabled}' 2>/dev/null || true)"
  # Keep compatibility with older SIDB manifests that used the top-level
  # enableTCPS field while preferring the current grouped security.tcps field.
  sidb_tcps_legacy="$(agentfactory_case_kubectl -n "$SIDB_CASE_NAMESPACE" \
    get singleinstancedatabase "$SIDB_TEST_NAME" \
    -o jsonpath='{.spec.enableTCPS}' 2>/dev/null || true)"
  if [ "$sidb_tcps_spec" != "true" ] && [ "$sidb_tcps_legacy" != "true" ]; then
    printf '%s\n' \
      'mode=password' \
      'sidb_tcps_configured=false' \
      'message=SIDB TCPS wallet is not configured; Details/password bootstrap remains enabled' \
      > "$status_file"
    if [ -n "${setup_order_file:-}" ]; then
      printf '03b database-preparation mode=password status=fallback path=%s\n' "$status_file" >> "$setup_order_file"
    fi
    echo "Agent Factory test02: SIDB TCPS wallet is not configured; using the existing password bootstrap path"
    return 0
  fi

  sidb_tcps_status="$(agentfactory_case_kubectl -n "$SIDB_CASE_NAMESPACE" \
    get singleinstancedatabase "$SIDB_TEST_NAME" \
    -o jsonpath='{.status.isTcpsEnabled}' 2>/dev/null || true)"
  [ "$sidb_tcps_status" = "true" ] ||
    prepare_database_die "SIDB declares TCPS but status.isTcpsEnabled is '$sidb_tcps_status'; wait for SIDB TCPS reconciliation"

  mapfile -t sidb_pods < <(
    agentfactory_case_kubectl -n "$SIDB_CASE_NAMESPACE" get pods --no-headers \
      -o custom-columns=NAME:.metadata.name 2>/dev/null |
      awk -v needle="$SIDB_TEST_NAME" 'index($1, needle) > 0 {print $1}'
  )
  for candidate_pod in "${sidb_pods[@]}"; do
    if agentfactory_case_kubectl -n "$SIDB_CASE_NAMESPACE" wait \
      --for=condition=Ready "pod/$candidate_pod" --timeout=60s >/dev/null 2>&1; then
      sidb_pod="$candidate_pod"
      break
    fi
  done
  [ -n "$sidb_pod" ] || prepare_database_die "no Ready SIDB Pod was found for $SIDB_TEST_NAME"

  sidb_wallet_dir="$(agentfactory_case_kubectl -n "$SIDB_CASE_NAMESPACE" \
    get singleinstancedatabase "$SIDB_TEST_NAME" \
    -o jsonpath='{.status.clientWalletLoc}' 2>/dev/null || true)"
  sidb_sid="$(agentfactory_case_kubectl -n "$SIDB_CASE_NAMESPACE" \
    get singleinstancedatabase "$SIDB_TEST_NAME" \
    -o jsonpath='{.spec.sid}' 2>/dev/null || true)"
  sidb_wallet_dir="${sidb_wallet_dir:-/opt/oracle/oradata/clientWallet/${sidb_sid}}"
  [[ "$sidb_wallet_dir" =~ ^/[A-Za-z0-9_./\$-]+$ ]] ||
    prepare_database_die "SIDB client wallet path is invalid: $sidb_wallet_dir"

  prepare_database_tmp_dir="$(mktemp -d "${TMPDIR:-/tmp}/agentfactory-test02-wallet.XXXXXX")"
  chmod 700 "$prepare_database_tmp_dir"
  prepare_database_cleanup() {
    if [ -n "${prepare_database_tmp_dir:-}" ] && [ -d "$prepare_database_tmp_dir" ]; then
      rm -rf -- "$prepare_database_tmp_dir"
    fi
  }
  trap prepare_database_cleanup EXIT
  old_umask="$(umask)"
  umask 077

  sidb_exec() {
    if [ -n "${SIDB_DB_CONTAINER:-}" ]; then
      agentfactory_case_kubectl -n "$SIDB_CASE_NAMESPACE" exec \
        -c "$SIDB_DB_CONTAINER" "$sidb_pod" -- "$@"
    else
      agentfactory_case_kubectl -n "$SIDB_CASE_NAMESPACE" exec "$sidb_pod" -- "$@"
    fi
  }

  prepare_database_copy_wallet_file() {
    local file_name="$1" required="$2" destination="$prepare_database_tmp_dir/$1"
    if sidb_exec bash -c \
      'if [ -r "$1" ]; then cat "$1"; else exit 44; fi' \
      sidb-wallet-cat "$sidb_wallet_dir/$file_name" > "$destination"; then
      [ -s "$destination" ] || {
        [ "$required" = "true" ] && prepare_database_die "SIDB wallet file $file_name is empty"
        rm -f -- "$destination"
        return 0
      }
      printf '%s\n' "$destination"
      return 0
    fi
    rm -f -- "$destination"
    if [ "$required" = "true" ]; then
      prepare_database_die "unable to retrieve required SIDB wallet file $file_name from $sidb_wallet_dir"
    fi
  }

  cwallet_file="$(prepare_database_copy_wallet_file cwallet.sso true)"
  sqlnet_file="$(prepare_database_copy_wallet_file sqlnet.ora true)"
  tnsnames_file="$(prepare_database_copy_wallet_file tnsnames.ora true)"
  ewallet_file="$(prepare_database_copy_wallet_file ewallet.p12 false || true)"

  # Prefer the explicit Agent Factory DB host, which test02 sets to the
  # sidb-ns Service FQDN before this hook runs.
  if [ -z "${AGENTFACTORY_DB_HOST:-}" ]; then
    AGENTFACTORY_DB_HOST="${SIDB_TEST_NAME}.${SIDB_CASE_NAMESPACE}.svc.cluster.local"
  fi
  prepare_database_make_wallet_portable "$tnsnames_file" "$sqlnet_file" "$AGENTFACTORY_DB_HOST"

  # Keep the non-secret TNS configuration in the per-test diagnostics even
  # when alias selection fails. Do not archive the wallet files here.
  mkdir -p "$AGENTFACTORY_CASE_RESULTS_DIR/setup-manifests"
  cp -- "$tnsnames_file" "$AGENTFACTORY_CASE_RESULTS_DIR/setup-manifests/sidb-tnsnames.ora"
  chmod 600 "$AGENTFACTORY_CASE_RESULTS_DIR/setup-manifests/sidb-tnsnames.ora"

  mapfile -t wallet_aliases < <(
    awk '
      function record_alias(line, alias) {
        if (line !~ /^[A-Za-z][A-Za-z0-9_.-]*[[:space:]]*=/) {
          return
        }
        alias = line
        sub(/[[:space:]]*=.*$/, "", alias)
        if (!seen[toupper(alias)]++) {
          print alias
        }
      }
      {
        line = $0
        sub(/^[[:space:]]+/, "", line)
        if (depth == 0) {
          record_alias(line)
        }
        opens = gsub(/\(/, "(", line)
        closes = gsub(/\)/, ")", line)
        depth += opens - closes
        if (depth < 0) {
          depth = 0
        }
      }
    ' "$tnsnames_file"
  )
  [ "${#wallet_aliases[@]}" -gt 0 ] ||
    prepare_database_die "no TNS alias was found in the SIDB tnsnames.ora"

  requested_alias="${AGENTFACTORY_SIDB_WALLET_ALIAS:-}"
  default_alias="${SIDB_TEST_PDB^^}_TCPS"
  pdb_alias="${SIDB_TEST_PDB^^}"
  wallet_alias=""
  if [ -n "$requested_alias" ]; then
    for candidate_pod in "${wallet_aliases[@]}"; do
      if [ "${candidate_pod^^}" = "${requested_alias^^}" ]; then
        wallet_alias="$candidate_pod"
        break
      fi
    done
  else
    for preferred_alias in "$default_alias" "$pdb_alias"; do
      for candidate_pod in "${wallet_aliases[@]}"; do
        if [ "${candidate_pod^^}" = "$preferred_alias" ]; then
          wallet_alias="$candidate_pod"
          break 2
        fi
      done
    done
    if [ -z "$wallet_alias" ]; then
      for candidate_pod in "${wallet_aliases[@]}"; do
        candidate_upper="${candidate_pod^^}"
        if [[ "$candidate_upper" == "${pdb_alias}_"* ]] ||
           [[ "$candidate_upper" == *"$pdb_alias"* ]]; then
          wallet_alias="$candidate_pod"
          break
        fi
      done
    fi
    if [ -z "$wallet_alias" ] && [ "${#wallet_aliases[@]}" -eq 1 ]; then
      wallet_alias="${wallet_aliases[0]}"
    fi
  fi
  [ -n "$wallet_alias" ] ||
    prepare_database_die "could not select a wallet alias for PDB $SIDB_TEST_PDB; set AGENTFACTORY_SIDB_WALLET_ALIAS; available aliases: ${wallet_aliases[*]}"
  [[ "$wallet_alias" =~ ^[A-Za-z][A-Za-z0-9_.-]*$ ]] ||
    prepare_database_die "selected SIDB wallet alias is invalid"

  wallet_source_dir="$prepare_database_tmp_dir/wallet"
  mkdir -m 700 "$wallet_source_dir"
  cp -- "$cwallet_file" "$wallet_source_dir/cwallet.sso"
  cp -- "$sqlnet_file" "$wallet_source_dir/sqlnet.ora"
  cp -- "$tnsnames_file" "$wallet_source_dir/tnsnames.ora"
  wallet_files=(
    "$wallet_source_dir/cwallet.sso"
    "$wallet_source_dir/sqlnet.ora"
    "$wallet_source_dir/tnsnames.ora"
  )
  if [ -n "$ewallet_file" ]; then
    cp -- "$ewallet_file" "$wallet_source_dir/ewallet.p12"
    wallet_files+=("$wallet_source_dir/ewallet.p12")
  fi
  zip_bin="$(command -v zip 2>/dev/null || true)"
  [ -n "$zip_bin" ] || prepare_database_die "zip is required on the E2E runner to package the SIDB client wallet"
  wallet_zip="$prepare_database_tmp_dir/db_wallet.zip"
  "$zip_bin" -q -j "$wallet_zip" "${wallet_files[@]}" ||
    prepare_database_die "failed to package the SIDB client wallet"
  chmod 600 "$wallet_zip"
  cp -- "$wallet_zip" "$AGENTFACTORY_CASE_RESULTS_DIR/setup-manifests/db-wallet.zip"
  chmod 600 "$AGENTFACTORY_CASE_RESULTS_DIR/setup-manifests/db-wallet.zip"

  wallet_b64="$(base64 < "$wallet_zip" | tr -d '\n')"
  [ -n "$wallet_b64" ] || prepare_database_die "the packaged SIDB wallet is empty"
  [[ "$AGENTFACTORY_WALLET_ZIP_KEY" =~ ^[A-Za-z0-9._-]+$ ]] ||
    prepare_database_die "AGENTFACTORY_WALLET_ZIP_KEY is not a valid Kubernetes Secret key"
  wallet_patch_file="$prepare_database_tmp_dir/wallet-secret.patch.json"
  printf '{"data":{"%s":"%s"}}\n' "$AGENTFACTORY_WALLET_ZIP_KEY" "$wallet_b64" > "$wallet_patch_file"
  chmod 600 "$wallet_patch_file"
  agentfactory_case_kubectl -n "$AGENTFACTORY_CASE_NAMESPACE" patch secret \
    "$AGENTFACTORY_RUNTIME_SECRET" --type=merge --patch-file "$wallet_patch_file" >/dev/null
  agentfactory_case_kubectl -n "$AGENTFACTORY_CASE_NAMESPACE" get secret \
    "$AGENTFACTORY_RUNTIME_SECRET" -o yaml \
    > "$AGENTFACTORY_CASE_RESULTS_DIR/setup-manifests/02-runtime-secret-after-wallet.yaml"
  chmod 600 "$AGENTFACTORY_CASE_RESULTS_DIR/setup-manifests/02-runtime-secret-after-wallet.yaml"
  if [ -f "$AGENTFACTORY_CASE_RESULTS_DIR/setup-manifests/02-runtime-secret.redacted.yaml" ]; then
    printf '  %s: REDACTED\n' "$AGENTFACTORY_WALLET_ZIP_KEY" \
      >> "$AGENTFACTORY_CASE_RESULTS_DIR/setup-manifests/02-runtime-secret.redacted.yaml"
  fi

  printf '%s\n' \
    'mode=wallet' \
    'sidb_tcps_configured=true' \
    "sidb_name=$SIDB_TEST_NAME" \
    "sidb_pod=$sidb_pod" \
    "wallet_source_path=$sidb_wallet_dir" \
    "wallet_alias=$wallet_alias" \
    "wallet_connect_host=$AGENTFACTORY_DB_HOST" \
    'wallet_sqlnet_directory=.' \
    "wallet_secret_key=$AGENTFACTORY_WALLET_ZIP_KEY" \
    'schema_setup=controller_job_pending' \
    'bootstrap_secret=retained_for_controller_job' \
    > "$status_file"
  if [ -n "${setup_order_file:-}" ]; then
    printf '03b database-preparation mode=wallet status=wallet-prepared path=%s\n' "$status_file" >> "$setup_order_file"
  fi

  export AGENTFACTORY_DB_CONNECTION_TYPE=Wallet
  export AGENTFACTORY_DB_SELECTED_TNS_ALIAS="$wallet_alias"
  export AGENTFACTORY_DB_HAS_OCI_CERTIFICATES=false
  # test01 and test02 both reuse this hook. Prefer an explicit wallet template
  # from the caller, otherwise keep the test02 wallet manifest next to this script.
  export AGENTFACTORY_TEST_TEMPLATE="${AGENTFACTORY_WALLET_TEMPLATE:-${prepare_script_dir}/test02-wallet.yaml}"
  umask "$old_umask"
  echo "Agent Factory ${AGENTFACTORY_CASE_ID:-test}: SIDB TCPS wallet retrieved, runtime Secret patched, and controller database setup Job enabled"
}

prepare_script_dir="$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
prepare_database_for_test02
