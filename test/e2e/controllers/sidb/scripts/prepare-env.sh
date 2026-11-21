#!/usr/bin/env bash
set -euo pipefail

output_file="${SIDB_ENV_FILE:-/tmp/sidb-e2e.env}"
mkdir -p "$(dirname "$output_file")"

: "${E2E_NAMESPACE:=sidb-ns}"
: "${SIDB_IMAGE:?Set SIDB_IMAGE as an environment variable}"
: "${SIDB_STORAGE_CLASS:=oci-bv}"
: "${SIDB_TESTS:=${SIDB_PROFILE:-all}}"
: "${SIDB_PROFILE:=$SIDB_TESTS}"
: "${SIDB_ADMIN_SECRET:=sidb-e2e-admin}"
: "${SIDB_ADMIN_SECRET_KEY:=oracle_pwd}"
: "${SIDB_PRIMARY_NAME:=sidb-e2e-primary}"
: "${SIDB_PRIMARY_SID:=ORCL1}"
: "${SIDB_PRIMARY_PDB:=ORCLPDB1}"
: "${SIDB_CLONE_NAME:=sidb-e2e-clone}"
: "${SIDB_CLONE_SID:=ORCL2}"
: "${SIDB_STANDBY_NAME:=sidb-e2e-standby}"
: "${SIDB_STANDBY_SID:=ORCLS}"
: "${SIDB_BROKER_NAME:=sidb-e2e-dg}"
: "${SIDB_TIMEOUT_SECONDS:=1500}"
: "${SIDB_UNHEALTHY_GRACE_SECONDS:=600}"
: "${SIDB_POLL_INTERVAL:=20}"
: "${SIDB_E2E_RESULTS_DIR:=test/e2e/controllers/sidb/results/${SIDB_PROFILE}-${E2E_NAMESPACE}}"

if [ -z "${SIDB_ADMIN_PASSWORD:-}" ]; then
  if command -v openssl >/dev/null 2>&1; then
    SIDB_ADMIN_PASSWORD="E2e_$(openssl rand -hex 12)"
  else
    SIDB_ADMIN_PASSWORD="E2e_${CI_PIPELINE_ID:-local}_ChangeMe"
  fi
  echo "SIDB_ADMIN_PASSWORD was not supplied; generated a temporary test password"
fi

# KUBE_CONTEXT_FILE is a path or GitLab file-variable path. If omitted,
# kubectl uses its normal default ($HOME/.kube/config).
if [ -n "${KUBE_CONTEXT_FILE:-}" ]; then
  [ -f "$KUBE_CONTEXT_FILE" ] || { echo "KUBE_CONTEXT_FILE does not exist: $KUBE_CONTEXT_FILE" >&2; exit 2; }
  KUBECONFIG="$KUBE_CONTEXT_FILE"
else
  KUBECONFIG="${KUBECONFIG:-${HOME}/.kube/config}"
fi

cat > "$output_file" <<EOF
export E2E_NAMESPACE=$(printf '%q' "$E2E_NAMESPACE")
export SIDB_TESTS=$(printf '%q' "$SIDB_TESTS")
export SIDB_PROFILE=$(printf '%q' "$SIDB_PROFILE")
export SIDB_IMAGE=$(printf '%q' "${SIDB_IMAGE:-}")
export SIDB_PULL_SECRET=$(printf '%q' "${SIDB_PULL_SECRET:-}")
export SIDB_STORAGE_CLASS=$(printf '%q' "${SIDB_STORAGE_CLASS:-}")
export SIDB_ADMIN_PASSWORD=$(printf '%q' "$SIDB_ADMIN_PASSWORD")
export SIDB_ADMIN_SECRET=$(printf '%q' "$SIDB_ADMIN_SECRET")
export SIDB_ADMIN_SECRET_KEY=$(printf '%q' "$SIDB_ADMIN_SECRET_KEY")
export SIDB_PRIMARY_NAME=$(printf '%q' "$SIDB_PRIMARY_NAME")
export SIDB_PRIMARY_SID=$(printf '%q' "$SIDB_PRIMARY_SID")
export SIDB_PRIMARY_PDB=$(printf '%q' "$SIDB_PRIMARY_PDB")
export SIDB_CLONE_NAME=$(printf '%q' "$SIDB_CLONE_NAME")
export SIDB_CLONE_SID=$(printf '%q' "$SIDB_CLONE_SID")
export SIDB_STANDBY_NAME=$(printf '%q' "$SIDB_STANDBY_NAME")
export SIDB_STANDBY_SID=$(printf '%q' "$SIDB_STANDBY_SID")
export SIDB_BROKER_NAME=$(printf '%q' "$SIDB_BROKER_NAME")
export SIDB_PRIMARY_EXTERNAL_CONNECT_STRING=$(printf '%q' "${SIDB_PRIMARY_EXTERNAL_CONNECT_STRING:-}")
export SIDB_TIMEOUT_SECONDS=$(printf '%q' "$SIDB_TIMEOUT_SECONDS")
export SIDB_UNHEALTHY_GRACE_SECONDS=$(printf '%q' "$SIDB_UNHEALTHY_GRACE_SECONDS")
export SIDB_POLL_INTERVAL=$(printf '%q' "$SIDB_POLL_INTERVAL")
export SIDB_E2E_RESULTS_DIR=$(printf '%q' "$SIDB_E2E_RESULTS_DIR")
export KUBECONFIG=$(printf '%q' "$KUBECONFIG")
export KUBE_CONTEXT_NAME=$(printf '%q' "${KUBE_CONTEXT_NAME:-}")
EOF
chmod 600 "$output_file"
echo "Wrote SIDB e2e environment to $output_file"
