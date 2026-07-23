#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'EOF'
Usage: run.sh --namespace <ns> [--profile smoke|full] [--name <cr-name>]
EOF
}

ns=""
profile="smoke"
name="sdb-system"

while [ $# -gt 0 ]; do
  case "$1" in
    --namespace)
      ns="$2"
      shift 2
      ;;
    --profile)
      profile="$2"
      shift 2
      ;;
    --name)
      name="$2"
      shift 2
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "Unknown argument: $1" >&2
      usage
      exit 2
      ;;
  esac
done

[ -n "${ns}" ] || { usage; exit 2; }

order_file="test/e2e/controllers/sharding/${profile}/manifest-order.txt"
[ -f "${order_file}" ] || { echo "Missing order file: ${order_file}" >&2; exit 2; }

: "${DB_IMAGE:=phx.ocir.io/intsanjaysingh/db-repo/oracle/database:26.0.0-ee}"
: "${GSM_IMAGE:=phx.ocir.io/intsanjaysingh/db-repo/oracle/database:26.0.0-gsm}"
: "${SHARDING_STORAGE_CLASS:=oci}"
: "${SHARDING_SCRIPTS_URL:=https://objectstorage.us-phoenix-1.oraclecloud.com/p/skNq1WdNGhXonbDQN8c55d95WjXYeG9AGyVZ1SoWQPKNHKq0b6ALwCSFalQtFnWD/n/intsanjaysingh/b/sharding-scripts/o/db-main-sharding.tgz}"
: "${SHARDING_DB_SECRET:=db-user-pass-pkutl}"
: "${SHARDING_DB_PWD_KEY:=pwdfile.enc}"
: "${SHARDING_DB_PRIV_KEY:=key.pem}"

export E2E_NAMESPACE="${ns}"
export SHARDING_NAME="${name}"
export DB_IMAGE GSM_IMAGE SHARDING_STORAGE_CLASS SHARDING_SCRIPTS_URL SHARDING_DB_SECRET SHARDING_DB_PWD_KEY SHARDING_DB_PRIV_KEY

results_dir="test/e2e/controllers/sharding/results/${profile}-${name}-${ns}"
mkdir -p "${results_dir}"

while IFS= read -r manifest; do
  [ -n "${manifest}" ] || continue
  case "${manifest}" in
    \#*) continue ;;
  esac
  if [[ "${manifest}" == *.tmpl ]]; then
    rendered="${results_dir}/$(basename "${manifest}" .tmpl).yaml"
    envsubst < "${manifest}" > "${rendered}"
    kubectl apply -f "${rendered}"
  else
    kubectl apply -f "${manifest}"
  fi
done < "${order_file}"

chmod +x test/e2e/controllers/sharding/scripts/assert-system.sh
test/e2e/controllers/sharding/scripts/assert-system.sh "${ns}" "${name}" "${results_dir}"

echo "Sharding ${profile} e2e succeeded for ${name} in namespace ${ns}"
