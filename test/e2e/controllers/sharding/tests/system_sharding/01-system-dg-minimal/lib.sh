#!/usr/bin/env bash
# Source from a dedicated DG test process; never enable shell tracing here.
set +x
set -Eeuo pipefail
SHARDING_CASE_DIR="$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
SHARDING_REPO_ROOT="$(CDPATH= cd -- "$SHARDING_CASE_DIR/../../../../../../.." && pwd)"
: "${SHARDING_E2E_NAMESPACE:=${E2E_NAMESPACE:-gdd-ns}}"
: "${SHARDING_NAME:=sdb-system}"
: "${USE_19c_IMAGES=false}"
: "${SHARDING_STORAGE_CLASS:=oci-bv}"
: "${SHARDING_DB_PULL_SECRET:=ocr-reg-cred}"
: "${SHARDING_GSM_PULL_SECRET:=ocr-reg-cred}"
# Select the password Secret independently for this DG case.
: "${SHARDING_DG_DB_SECRET_NAME:=sharding-dg-e2e-db-user-pass-${CI_PIPELINE_ID:-local}-${CI_JOB_ID:-manual}}"
SHARDING_DB_SECRET_NAME="$SHARDING_DG_DB_SECRET_NAME"
export SHARDING_DG_DB_SECRET_NAME SHARDING_DB_SECRET_NAME
: "${SHARDING_TIMEOUT_SECONDS:=7200}"
: "${SHARDING_POLL_INTERVAL:=20}"
: "${SHARDING_GDSCTL_TIMEOUT_SECONDS:=1800}"
: "${SHARDING_GDSCTL_POLL_INTERVAL_SECONDS:=15}"
: "${SHARDING_COMMAND_TIMEOUT_SECONDS:=90}"
: "${SHARDING_CLEANUP_TIMEOUT_SECONDS:=900}"
: "${SHARDING_RESULTS_DIR:=${CI_PROJECT_DIR:-$SHARDING_REPO_ROOT}/test/e2e/controllers/sharding/results/dg-minimal}"

sharding_case_label() { printf 'sharding/dg-minimal %s/%s' "$SHARDING_E2E_NAMESPACE" "$SHARDING_NAME"; }
sharding_case_fail() { echo "$(sharding_case_label): $*" >&2; return 1; }

# Resolve both images together. Keep the common image inputs unchanged.
# SHARDING_DG_*_IMAGE are internal results, not CI/CD overrides.
sharding_case_select_images() {
  local db_variable gsm_variable image_variable
  case "$USE_19c_IMAGES" in
    true)
      db_variable=SHARDING_DB_19c_IMAGE
      gsm_variable=SHARDING_GSM_19c_IMAGE
      ;;
    false)
      db_variable=SHARDING_DB_IMAGE
      gsm_variable=SHARDING_GSM_IMAGE
      ;;
    *)
      sharding_case_fail 'USE_19c_IMAGES must be exactly true or false'
      return 2
      ;;
  esac
  for image_variable in "$db_variable" "$gsm_variable"; do
    if [[ -z "${!image_variable:-}" || "${!image_variable:-}" =~ [[:space:]] ]]; then
      sharding_case_fail "$image_variable must be a nonempty image reference without whitespace when USE_19c_IMAGES=$USE_19c_IMAGES"
      return 2
    fi
  done
  SHARDING_DG_DB_IMAGE="${!db_variable}"
  SHARDING_DG_GSM_IMAGE="${!gsm_variable}"
  export USE_19c_IMAGES SHARDING_DG_DB_IMAGE SHARDING_DG_GSM_IMAGE
}

sharding_case_init() {
  local tool var
  sharding_case_select_images || return 2
  for tool in kubectl openssl jq python3 timeout awk; do
    command -v "$tool" >/dev/null || { sharding_case_fail "$tool is required"; return 2; }
  done
  for var in SHARDING_TIMEOUT_SECONDS SHARDING_POLL_INTERVAL SHARDING_GDSCTL_TIMEOUT_SECONDS \
    SHARDING_GDSCTL_POLL_INTERVAL_SECONDS SHARDING_COMMAND_TIMEOUT_SECONDS SHARDING_CLEANUP_TIMEOUT_SECONDS; do
    [[ "${!var}" =~ ^[1-9][0-9]*$ ]] || { sharding_case_fail "$var must be a positive integer"; return 2; }
  done
  mkdir -p "$SHARDING_RESULTS_DIR"
  chmod 700 "$SHARDING_RESULTS_DIR"
}

# GNU timeout also bounds exec/log streams. Pollers cap each call by their deadline.
sharding_kubectl() {
  local budget="$SHARDING_COMMAND_TIMEOUT_SECONDS" remaining
  if [[ -n "${SHARDING_PHASE_DEADLINE:-}" ]]; then
    remaining=$((SHARDING_PHASE_DEADLINE - SECONDS))
    (( remaining > 0 )) || return 124
    (( budget <= remaining )) || budget="$remaining"
  fi
  timeout --kill-after=5s "${budget}s" kubectl --request-timeout="${budget}s" \
    -n "$SHARDING_E2E_NAMESPACE" "$@"
}
sharding_case_sleep() {
  local interval="$1" remaining=$((SHARDING_PHASE_DEADLINE - SECONDS))
  (( remaining > 0 )) || return 0
  (( interval <= remaining )) || interval="$remaining"
  sleep "$interval"
}

sharding_case_collect_diagnostics() (
  unset SHARDING_PHASE_DEADLINE
  local out="$SHARDING_RESULTS_DIR/diagnostics-${1:-failure}" pod
  mkdir -p "$out"
  sharding_kubectl get shardingdatabase "$SHARDING_NAME" -o yaml >"$out/shardingdatabase.yaml" 2>&1 || true
  sharding_kubectl get pods,svc,pvc,statefulsets -o wide >"$out/resources.txt" 2>&1 || true
  sharding_kubectl get events --sort-by=.lastTimestamp >"$out/events.txt" 2>&1 || true
  for pod in catalog-0 pshard1-0 pshard2-0 gsm1-0 gsm2-0; do
    sharding_kubectl logs "$pod" --all-containers=true --tail=2000 >"$out/$pod.log" 2>&1 || true
    sharding_kubectl exec "$pod" -- tail -n 2000 /var/tmp/gdd/oracle_sharding_setup.log \
      >"$out/$pod-setup.log" 2>&1 || true
  done
  echo "Diagnostics collected in $out"
)

sharding_case_render_manifest() {
  sharding_case_select_images || return 2
  echo "$(sharding_case_label): USE_19c_IMAGES=$USE_19c_IMAGES; dbImage=$SHARDING_DG_DB_IMAGE; gsmImage=$SHARDING_DG_GSM_IMAGE" >&2
  export SHARDING_NAME SHARDING_E2E_NAMESPACE
  export SHARDING_DB_PULL_SECRET SHARDING_GSM_PULL_SECRET SHARDING_STORAGE_CLASS SHARDING_DB_SECRET_NAME
  python3 - "$SHARDING_CASE_DIR/system-dg-minimal.yaml.tmpl" "$SHARDING_RESULTS_DIR/system-dg-minimal.yaml" <<'PY'
import json, os, pathlib, re, sys
text = pathlib.Path(sys.argv[1]).read_text()
mapping = {k: os.environ[k] for k in (
    'SHARDING_NAME',
    'SHARDING_DB_PULL_SECRET', 'SHARDING_GSM_PULL_SECRET',
    'SHARDING_STORAGE_CLASS', 'SHARDING_DB_SECRET_NAME')}
mapping['SHARDING_DB_IMAGE'] = os.environ['SHARDING_DG_DB_IMAGE']
mapping['SHARDING_GSM_IMAGE'] = os.environ['SHARDING_DG_GSM_IMAGE']
mapping['SHARDING_NAMESPACE'] = os.environ['SHARDING_E2E_NAMESPACE']
mapping['CI_PIPELINE_ID'] = os.environ.get('CI_PIPELINE_ID', 'local')
for key, value in mapping.items():
    text = text.replace('"__' + key + '__"', json.dumps(value))
if re.search(r'__[A-Z0-9_]+__', text):
    raise SystemExit('Unrendered manifest placeholder')
target = pathlib.Path(sys.argv[2]); target.write_text(text); target.chmod(0o600)
print(target)
PY
}

sharding_case_create_db_secret() (
  set +x
  umask 077
  : "${SHARDING_DB_PASSWORD:?Set SHARDING_DB_PASSWORD as a masked CI/CD variable}"
  local workdir
  workdir="$(mktemp -d "${TMPDIR:-/tmp}/sharding-dg-secret.XXXXXX")"
  trap 'rm -rf "$workdir"' EXIT
  printf '%s' "$SHARDING_DB_PASSWORD" >"$workdir/password"
  if (( $(wc -c <"$workdir/password") > 318 )); then
    sharding_case_fail 'Password exceeds the RSA-3072 OAEP/SHA-256 limit of 318 bytes'; return 2
  fi
  openssl genpkey -algorithm RSA -pkeyopt rsa_keygen_bits:3072 -out "$workdir/key.pem" 2>/dev/null
  openssl pkey -in "$workdir/key.pem" -pubout -out "$workdir/key.pub" 2>/dev/null
  openssl pkeyutl -encrypt -pubin -inkey "$workdir/key.pub" \
    -in "$workdir/password" -out "$workdir/pwdfile.enc" \
    -pkeyopt rsa_padding_mode:oaep -pkeyopt rsa_oaep_md:sha256 -pkeyopt rsa_mgf1_md:sha256
  openssl pkeyutl -decrypt -inkey "$workdir/key.pem" \
    -in "$workdir/pwdfile.enc" -out "$workdir/decrypted" \
    -pkeyopt rsa_padding_mode:oaep -pkeyopt rsa_oaep_md:sha256 -pkeyopt rsa_mgf1_md:sha256
  cmp -s "$workdir/password" "$workdir/decrypted" || return 1
  rm -f "$workdir/password" "$workdir/decrypted" "$workdir/key.pub"
  # Create, never rotate a Secret that an existing deployment may be using.
  sharding_kubectl create secret generic "$SHARDING_DB_SECRET_NAME" \
    --from-file="pwdfile.enc=$workdir/pwdfile.enc" --from-file="key.pem=$workdir/key.pem" \
    --dry-run=client -o json |
    jq --arg cr "$SHARDING_NAME" '.metadata.labels = {
      "e2e.oracle.com/owner":"operator-e2e", "e2e.oracle.com/controller":"sharding",
      "e2e.oracle.com/test-id":"01-system-dg-minimal", "e2e.oracle.com/resource":$cr}' |
    sharding_kubectl create -f - >/dev/null
)

sharding_case_log_contains() {
  local pod="$1" marker="$2" source="$3"
  if [[ "$source" = setup-log ]]; then
    sharding_kubectl exec "$pod" -- grep -Fq -- "$marker" /var/tmp/gdd/oracle_sharding_setup.log 2>/dev/null
  else
    # Read the entire stream: grep -q can cause SIGPIPE under pipefail.
    sharding_kubectl logs "$pod" --all-containers=true 2>/dev/null | grep -F -- "$marker" >/dev/null
  fi
}

# Normalize whole lines: Data Guard is a two-word value.
sharding_gds_normalize() {
  awk '{gsub(/\r/, ""); gsub(/\033\[[0-?]*[ -\/]*[@-~]/, "");
        sub(/^[[:space:]]+/, ""); sub(/[[:space:]]+$/, "");
        gsub(/[[:space:]]+/, " "); print}' "$1"
}
sharding_gds_shards_ready() {
  awk '
    $1 == "Name" && $2 == "Shard" {table=1; next}
    table && /^[- ]+$/ {next}
    table && NF {
      if (NF != 6 || ($1 != "pshard1_pshard1pdb" && $1 != "pshard2_pshard2pdb")) bad=1
      if ($2 != "pshg1" || $3 != "Ok" || $4 != "Deployed" || $5 != "primary" || $6 != "ONLINE") bad=1
      rows[$1]++; count++
    }
    END {exit !(table && !bad && count==2 && rows["pshard1_pshard1pdb"]==1 && rows["pshard2_pshard2pdb"]==1)}
  ' "$1"
}
sharding_gds_sdb_ready() {
  awk '
    /^Replication Type$/ {section="replication"; next}
    /^Shard type$/ {section="type"; next}
    /^Shard spaces$/ {section="spaces"; next}
    /^Services$/ {section="services"; next}
    /^GDS Pool administrators$/ {section="admins"; next}
    /^[- ]*$/ {next}
    section=="replication" {if ($0=="Data Guard") replication++; else bad=1}
    section=="type" {if ($0=="System-managed") type++; else bad=1}
    section=="spaces" {if ($0=="shardspaceora") spaces++; else bad=1}
    END {exit !(replication==1 && type==1 && spaces==1 && !bad)}
  ' "$1"
}
