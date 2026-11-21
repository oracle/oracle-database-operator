#!/usr/bin/env bash
# dataguard/test01: same-cluster SIDB Data Guard (alias of sidb/test03).
set -Eeuo pipefail

case_dir="$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
repo_root="${CI_PROJECT_DIR:-$(CDPATH= cd -- "$case_dir/../../../../../.." && pwd)}"
sidb_dg_dir="$repo_root/test/e2e/controllers/sidb/tests/03-dataguard"

# Prefer cman1 / dedicated DG namespace when set; otherwise reuse SIDB namespace.
export SIDB_E2E_NAMESPACE="${DATAGUARD_E2E_NAMESPACE:-${SIDB_E2E_NAMESPACE:-sidb-ns}}"

# Fresh same-cluster DG often reports Configuration Status WARNING
# (ORA-16854 apply lag undetermined) for 15–40 minutes before SUCCESS.
# Force these above the CI/global default of 600s — `:-` does not override an
# already-exported SIDB_UNHEALTHY_GRACE_SECONDS=600 from the job env.
# Pipeline 19043328 still timed out at 1800s Unhealthy grace.
export SIDB_UNHEALTHY_GRACE_SECONDS=3600
export SIDB_DG_BROKER_TIMEOUT_SECONDS=4500
export SIDB_TIMEOUT_SECONDS=4500

# Shared CI runners leave /tmp/sidb-ns-test03.env from prior jobs; sidb/test03
# prerequisites refuse to start when that file exists. Use a unique path and
# clear the default stale file so the alias can rerun cleanly.
export SIDB_TEST03_ENV_FILE="${TMPDIR:-/tmp}/sidb-dataguard-test01-${CI_PIPELINE_ID:-local}-${CI_JOB_ID:-$$}.env"
rm -f "$SIDB_TEST03_ENV_FILE" "${TMPDIR:-/tmp}/sidb-ns-test03.env"

echo "dataguard/test01: delegating to sidb/test03 at $sidb_dg_dir (namespace=$SIDB_E2E_NAMESPACE)"
echo "dataguard/test01: broker wait timeouts grace=${SIDB_UNHEALTHY_GRACE_SECONDS}s broker=${SIDB_DG_BROKER_TIMEOUT_SECONDS}s env=$SIDB_TEST03_ENV_FILE"
exec bash "$sidb_dg_dir/run.sh"
