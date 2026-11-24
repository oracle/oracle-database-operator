#!/usr/bin/env bash
# dataguard/test02: SIDB Data Guard with TDE+TCPS (alias of sidb/test04).
set -Eeuo pipefail

case_dir="$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
repo_root="${CI_PROJECT_DIR:-$(CDPATH= cd -- "$case_dir/../../../../../.." && pwd)}"
sidb_dg_dir="$repo_root/test/e2e/controllers/sidb/tests/04-dataguard-tcps"

export SIDB_E2E_NAMESPACE="${DATAGUARD_E2E_NAMESPACE:-${SIDB_E2E_NAMESPACE:-sidb-ns}}"

# Force broker waits above CI defaults (same lesson as dataguard/test01).
export SIDB_UNHEALTHY_GRACE_SECONDS=3600
export SIDB_DG_BROKER_TIMEOUT_SECONDS=4500
export SIDB_TIMEOUT_SECONDS=4500

export SIDB_TEST04_ENV_FILE="${TMPDIR:-/tmp}/sidb-dataguard-test02-${CI_PIPELINE_ID:-local}-${CI_JOB_ID:-$$}.env"
rm -f "$SIDB_TEST04_ENV_FILE" "${TMPDIR:-/tmp}/sidb-ns-test04.env"

echo "dataguard/test02: delegating to sidb/test04 at $sidb_dg_dir (namespace=$SIDB_E2E_NAMESPACE)"
echo "dataguard/test02: broker wait timeouts grace=${SIDB_UNHEALTHY_GRACE_SECONDS}s broker=${SIDB_DG_BROKER_TIMEOUT_SECONDS}s"
exec bash "$sidb_dg_dir/run.sh"
