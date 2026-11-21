#!/usr/bin/env bash
# Same topology workflow as the supplied gen_deg.sh; discover local standbys,
# use the supplied multi-member renderer, and write an applyable broker manifest.
set -Eeuo pipefail
NS="${NS:-shns}"
PRIMARY_DB="${PRIMARY_DB:-sidb-primary}"
BROKER_NAME="${BROKER_NAME:-${PRIMARY_DB}-dg}"
BROKER_YAML="${BROKER_YAML:-dataguardbroker.yaml}"
SCRIPT_DIR="$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
command -v kubectl >/dev/null
command -v jq >/dev/null
kubectl_current() {
  if [ -n "${KUBE_CONTEXT_NAME:-}" ]; then
    command kubectl --context "$KUBE_CONTEXT_NAME" "$@"
  else
    command kubectl "$@"
  fi
}
temporary_dir="$(mktemp -d "${TMPDIR:-/tmp}/sidb-gen-dg.XXXXXX")"
trap 'rm -rf -- "$temporary_dir"' EXIT
chmod 700 "$temporary_dir"
# Resolve a context or merged KUBECONFIG into one file for the supplied renderer.
# Private temporary file is removed on success and failure.
(umask 077; kubectl_current config view --raw --flatten --minify > "$temporary_dir/kubeconfig")
# Capture the command result first: mapfile process substitution would hide errors.
kubectl_current get singleinstancedatabase -n "$NS" -o json > "$temporary_dir/databases.json"
standby_names="$(jq -r --arg primary "$PRIMARY_DB" '
  [.items[] | select((.spec.createAs // "" | ascii_downcase) == "standby" and
    (.spec.primarySource.databaseRef // "") == $primary) | .metadata.name] | sort[]
' "$temporary_dir/databases.json")"
[ -n "$standby_names" ] || { echo "No standby found for $NS/$PRIMARY_DB" >&2; exit 1; }
mapfile -t STANDBY_DBS <<< "$standby_names"
if [ -n "${EXPECTED_STANDBY:-}" ] &&
  { [ "${#STANDBY_DBS[@]}" -ne 1 ] || [ "${STANDBY_DBS[0]}" != "$EXPECTED_STANDBY" ]; }; then
  echo "Basic test requires exactly standby $EXPECTED_STANDBY; discovered: ${STANDBY_DBS[*]}" >&2
  exit 1
fi
ARGS=(sidb --broker-name "$BROKER_NAME" --namespace "$NS"
  --kubeconfig "local=$temporary_dir/kubeconfig" --member "local:$PRIMARY_DB")
for standby in "${STANDBY_DBS[@]}"; do
  ARGS+=(--member "local:$standby")
done
ARGS+=(--source-ref "local:${STANDBY_DBS[0]}")
bash "$SCRIPT_DIR/render-dg-broker-from-status.sh" "${ARGS[@]}" > "$temporary_dir/broker.yaml"
mkdir -p "$(dirname "$BROKER_YAML")"
# Normalize whichever YAML/JSON format the supplied renderer emits into JSON.
# JSON is accepted by kubectl even with a .yaml filename. Never truncate a prior
# output on a renderer/conversion failure.
kubectl_current create --dry-run=client --validate=false -f "$temporary_dir/broker.yaml" \
  -o json > "$temporary_dir/broker.json"
jq -e '.kind == "DataguardBroker" and (.spec.topology.members | length >= 2)' \
  "$temporary_dir/broker.json" >/dev/null
cp "$temporary_dir/broker.json" "$BROKER_YAML"
echo "Generated $BROKER_YAML"
echo "Primary: $PRIMARY_DB; standbys: ${STANDBY_DBS[*]}; namespace: $NS"
