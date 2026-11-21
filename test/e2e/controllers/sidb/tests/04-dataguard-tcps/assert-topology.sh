#!/usr/bin/env bash
set -Eeuo pipefail
script_dir="$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
source "$script_dir/lib.sh"
sidb_dg_init_config
SIDB_CASE_STAGE=assert-topology
trap sidb_dg_exit_snapshot EXIT
sidb_dg_assert_database "$SIDB_DG_PRIMARY_NAME" PRIMARY "$SIDB_TEST_SID" "$SIDB_TEST_PDB" primary
sidb_dg_assert_database "$SIDB_DG_STANDBY_NAME" PHYSICAL_STANDBY "$SIDB_DG_STANDBY_SID" "$SIDB_DG_STANDBY_PDB" standby

# Confirm TCPS enabled on both databases.
for name in "$SIDB_DG_PRIMARY_NAME" "$SIDB_DG_STANDBY_NAME"; do
  tcps="$(sidb_case_kubectl -n "$SIDB_CASE_NAMESPACE" get singleinstancedatabase "$name" \
    -o jsonpath='{.status.isTcpsEnabled}')"
  [ "$tcps" = "true" ] || {
    echo "$(sidb_case_label): expected isTcpsEnabled=true on $name, got ${tcps:-<empty>}" >&2
    exit 1
  }
done

broker_json="$SIDB_CASE_RESULTS_DIR/broker-final.json"
sidb_case_kubectl -n "$SIDB_CASE_NAMESPACE" get dataguardbroker "$SIDB_DG_BROKER_NAME" -o json > "$broker_json"
sidb_dg_assert_broker_spec "$broker_json"
jq -e '
  [.spec.topology.members[].endpoints[]?
   | select((.protocol | ascii_upcase) == "TCPS" and .port == 2484)]
  | length == 2
' "$broker_json" >/dev/null || {
  echo "$(sidb_case_label): expected one TCPS/2484 endpoint per broker member" >&2
  exit 1
}
broker_status="$(jq -r --arg field "$SIDB_DG_BROKER_HEALTH_FIELD" \
  '(.status[$field] // empty)' "$broker_json")"
case "$(printf '%s' "$broker_status" | tr '[:lower:]' '[:upper:]')" in
  HEALTHY) ;;
  *)
    if sidb_dg_broker_dgmgrl_success; then
      echo "$(sidb_case_label): broker CR status=${broker_status:-<empty>} but dgmgrl SUCCESS; accepting"
    else
      echo "$(sidb_case_label): broker status ${broker_status:-<empty>} and dgmgrl not SUCCESS" >&2
      exit 1
    fi
    ;;
esac
sidb_dg_collect_diagnostics passed
echo "$(sidb_case_label): topology assertions passed (PRIMARY + PHYSICAL_STANDBY + TCPS + broker)"
