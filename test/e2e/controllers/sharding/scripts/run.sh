#!/usr/bin/env bash

set -euo pipefail

usage() {
  cat <<'EOF'
Usage: run.sh --namespace <ns> [--profile smoke|full|dg-minimal|native-minimal] [--name <cr-name>] [--results-dir <dir>] [--cleanup]

Profiles:
  smoke          Apply the existing smoke manifest sequence and assert the system.
  full           Apply the existing full manifest sequence and assert the system.
  dg-minimal     Deploy System-Managed Sharding with Data Guard replication.
  native-minimal Deploy System-Managed Sharding with Native (Raft) replication.

--cleanup deletes the native-minimal ShardingDatabase resource and its per-run DB Secret after assertions pass.
For dg-minimal, --cleanup selects pre- and post-test cleanup; otherwise the case uses SHARDING_DG_CLEANUP_POLICY or E2E_CLEANUP_POLICY (default: pre).
EOF
}

ns=""
profile="smoke"
name="sdb-system"
results_dir="test/e2e/controllers/sharding/results"
cleanup_requested=false

while [ "$#" -gt 0 ]; do
  case "$1" in
    --namespace)
      [ "$#" -ge 2 ] || { echo "Missing value for --namespace" >&2; usage >&2; exit 2; }
      ns="$2"
      shift 2
      ;;
    --profile)
      [ "$#" -ge 2 ] || { echo "Missing value for --profile" >&2; usage >&2; exit 2; }
      profile="$2"
      shift 2
      ;;
    --name)
      [ "$#" -ge 2 ] || { echo "Missing value for --name" >&2; usage >&2; exit 2; }
      name="$2"
      shift 2
      ;;
    --results-dir)
      [ "$#" -ge 2 ] || { echo "Missing value for --results-dir" >&2; usage >&2; exit 2; }
      results_dir="$2"
      shift 2
      ;;
    --cleanup)
      cleanup_requested=true
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "Unknown argument: $1" >&2
      usage >&2
      exit 2
      ;;
  esac
done

[ -n "$ns" ] || { usage >&2; exit 2; }

if [ "$profile" = "dg-minimal" ]; then
  set +x
  repo_root="${CI_PROJECT_DIR:-$(git rev-parse --show-toplevel 2>/dev/null || pwd)}"
  case_dir="$repo_root/test/e2e/controllers/sharding/tests/system_sharding/01-system-dg-minimal"

  [ -d "$case_dir" ] || { echo "Missing test case directory: $case_dir" >&2; exit 2; }

  export SHARDING_E2E_NAMESPACE="$ns"
  export E2E_NAMESPACE="$ns"
  export SHARDING_NAME="$name"
  export SHARDING_RESULTS_DIR="$results_dir"

  # shellcheck source=/dev/null
  source "$case_dir/lib.sh"
  sharding_case_init
  export SHARDING_DB_SECRET_NAME SHARDING_DG_DB_IMAGE SHARDING_DG_GSM_IMAGE

  dg_policy="${SHARDING_DG_CLEANUP_POLICY:-${E2E_CLEANUP_POLICY:-pre}}"
  if [ "$cleanup_requested" = true ]; then
    dg_policy=both
  fi
  case "$dg_policy" in
    pre|post|both|none) ;;
    *) echo "Invalid cleanup policy: $dg_policy" >&2; exit 2 ;;
  esac

  : "${SHARDING_DB_PASSWORD:?Set SHARDING_DB_PASSWORD as a masked CI/CD variable}"
  bash "$case_dir/verify-prerequisites.sh"

  dg_run_started=false
  sharding_dg_finish() {
    local rc="$?" cleanup_rc=0
    trap - EXIT

    if [ "$rc" -ne 0 ]; then
      sharding_case_collect_diagnostics failure || true
    fi

    if [[ "$dg_run_started" = true && ( "$dg_policy" = post || "$dg_policy" = both ) ]]; then
      bash "$case_dir/cleanup.sh" || cleanup_rc=$?
      if [ "$rc" -eq 0 ] && [ "$cleanup_rc" -ne 0 ]; then
        rc="$cleanup_rc"
      fi
    fi

    if [ "$rc" -eq 0 ]; then
      echo "Sharding ${profile} e2e succeeded for ${name} in namespace ${ns}"
    fi
    exit "$rc"
  }
  trap sharding_dg_finish EXIT

  if [[ "$dg_policy" = pre || "$dg_policy" = both ]]; then
    bash "$case_dir/cleanup.sh"
  fi

  bash "$case_dir/prerequisites.sh"
  dg_run_started=true
  sharding_kubectl apply -f "$SHARDING_RESULTS_DIR/system-dg-minimal.yaml"
  bash "$case_dir/wait-ready.sh"
  bash "$case_dir/assert-topology.sh"
  exit 0
fi

if [ "$profile" = "native-minimal" ]; then
  repo_root="${CI_PROJECT_DIR:-$(git rev-parse --show-toplevel 2>/dev/null || pwd)}"
  case_dir="$repo_root/test/e2e/controllers/sharding/tests/system_sharding_native/01-system-native-minimal"

  [ -d "$case_dir" ] || { echo "Missing test case directory: $case_dir" >&2; exit 2; }

  export SHARDING_E2E_NAMESPACE="$ns"
  export E2E_NAMESPACE="$ns"
  export SHARDING_NAME="$name"
  export SHARDING_RESULTS_DIR="$results_dir"

  : "${SHARDING_DB_IMAGE:=container-registry.oracle.com/database/enterprise_ru:latest-26}"
  : "${SHARDING_GSM_IMAGE:=container-registry.oracle.com/database/gsm_ru:latest-26}"
  : "${SHARDING_STORAGE_CLASS:=oci-bv}"
  : "${SHARDING_DB_PULL_SECRET:=ocr-reg-cred}"
  : "${SHARDING_GSM_PULL_SECRET:=ocr-reg-cred}"
  : "${SHARDING_DB_SECRET_NAME:=sharding-native-e2e-db-user-pass-${CI_PIPELINE_ID:-local}}"
  : "${SHARDING_TIMEOUT_SECONDS:=7200}"
  : "${SHARDING_POLL_INTERVAL:=20}"
  export SHARDING_DB_IMAGE SHARDING_GSM_IMAGE SHARDING_STORAGE_CLASS
  export SHARDING_DB_PULL_SECRET SHARDING_GSM_PULL_SECRET SHARDING_DB_SECRET_NAME
  export SHARDING_TIMEOUT_SECONDS SHARDING_POLL_INTERVAL

  command -v kubectl >/dev/null 2>&1 || { echo "kubectl is required" >&2; exit 2; }
  command -v openssl >/dev/null 2>&1 || { echo "openssl is required in the E2E job image" >&2; exit 2; }


  mkdir -p "$SHARDING_RESULTS_DIR"
  "$case_dir/prerequisites.sh"
  kubectl apply -f "$SHARDING_RESULTS_DIR/system-native-minimal.yaml"
  "$case_dir/wait-ready.sh"
  "$case_dir/assert-topology.sh"

  if [ "$cleanup_requested" = true ]; then
    "$case_dir/cleanup.sh"
  fi

  echo "Sharding ${profile} e2e succeeded for ${name} in namespace ${ns}"
  exit 0
fi

if [ "$cleanup_requested" = true ]; then
  echo "--cleanup is supported only with --profile native-minimal or dg-minimal" >&2
  exit 2
fi

order_file="test/e2e/controllers/sharding/${profile}/manifest-order.txt"
[ -f "$order_file" ] || { echo "Missing order file: ${order_file}" >&2; exit 2; }

: "${DB_IMAGE:=container-registry.oracle.com/database/database:26.0.0-ee}"
: "${GSM_IMAGE:=container-registry.oracle.com/database/database:26.0.0-gsm}"
: "${SHARDING_STORAGE_CLASS:=oci}"
: "${SHARDING_DB_SECRET:=db-user-pass-pkutl}"
: "${SHARDING_DB_PWD_KEY:=pwdfile.enc}"
: "${SHARDING_DB_PRIV_KEY:=key.pem}"
: "${SHARDING_SCRIPTS_URL:=}"

export E2E_NAMESPACE="$ns"
export SHARDING_NAME="$name"
export DB_IMAGE GSM_IMAGE SHARDING_STORAGE_CLASS SHARDING_SCRIPTS_URL
export SHARDING_DB_SECRET SHARDING_DB_PWD_KEY SHARDING_DB_PRIV_KEY

mkdir -p "$results_dir"

while IFS= read -r manifest; do
  [ -n "$manifest" ] || continue
  case "$manifest" in
    \#*) continue ;;
  esac
  if [[ "$manifest" == *.tmpl ]]; then
    rendered="${results_dir}/$(basename "$manifest" .tmpl).yaml"
    envsubst < "$manifest" > "$rendered"
    kubectl apply -f "$rendered"
  else
    kubectl apply -f "$manifest"
  fi
done < "$order_file"

chmod +x test/e2e/controllers/sharding/scripts/assert-system.sh
test/e2e/controllers/sharding/scripts/assert-system.sh "$ns" "$name" "$results_dir"

echo "Sharding ${profile} e2e succeeded for ${name} in namespace ${ns}"
