#!/usr/bin/env bash
set -Eeuo pipefail
script_dir="$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
source "$script_dir/lib.sh"
sidb_dg_init_config
SIDB_CASE_STAGE=wait-primary
trap sidb_dg_exit_snapshot EXIT
bash "$SIDB_CASE_DIR/../common/wait-ready.sh" "$SIDB_DG_PRIMARY_NAME" PRIMARY

# Export primary TDE wallet into the shared TDE Secret before creating standby.
SIDB_CASE_STAGE=export-tde-wallet
primary_pod="$(sidb_case_kubectl -n "$SIDB_CASE_NAMESPACE" get pods \
  -l "app=$SIDB_DG_PRIMARY_NAME" -o jsonpath='{.items[0].metadata.name}')"
[ -n "$primary_pod" ] || {
  echo "$(sidb_case_label): primary Pod not found for TDE wallet export" >&2
  exit 1
}
wallet_tmp="$(mktemp -d "${TMPDIR:-/tmp}/sidb-dg-wallet.XXXXXX")"
mkdir -p "$wallet_tmp/tde"
wallet_src="/opt/oracle/oradata/${SIDB_TEST_SID}/tdewallet/tde/ewallet.p12"
echo "$(sidb_case_label): exporting TDE wallet from $primary_pod:$wallet_src"
if ! sidb_case_kubectl -n "$SIDB_CASE_NAMESPACE" cp \
  "$primary_pod:$wallet_src" "$wallet_tmp/tde/ewallet.p12"; then
  echo "$(sidb_case_label): failed to copy ewallet.p12 from primary" >&2
  rm -rf -- "$wallet_tmp"
  exit 1
fi
(cd "$wallet_tmp" && zip -qr "$wallet_tmp/standby-wallet.zip" tde)
unzip -t "$wallet_tmp/standby-wallet.zip" >/dev/null
sidb_case_kubectl -n "$SIDB_CASE_NAMESPACE" create secret generic "$SIDB_TDE_SECRET" \
  --from-file="${SIDB_TDE_WALLET_ZIP_KEY}=$wallet_tmp/standby-wallet.zip" \
  --from-literal="${SIDB_TDE_SECRET_KEY}=${SIDB_TDE_PASSWORD}" \
  --dry-run=client -o yaml |
  tee "$SIDB_CASE_RESULTS_DIR/secret-manifests/tde-secret-with-wallet.yaml" |
  sidb_case_kubectl -n "$SIDB_CASE_NAMESPACE" apply -f - >/dev/null
rm -rf -- "$wallet_tmp"
echo "$(sidb_case_label): refreshed $SIDB_TDE_SECRET with standby-wallet.zip"

# Expand TLS SANs for standby Service DNS if secret was created for primary only.
# Prefer creating primary+standby SANs in prerequisites (SIDB_TCPS_TLS_EXTRA_DNS_NAMES)
# so this path is a no-op. Regenerating here replaces the leaf after primary TCPS
# and can leave primary on the old wallet while standby uses the new one (ORA-29024).
SIDB_CASE_STAGE=ensure-standby-tls-sans
if ! openssl x509 -in <(sidb_case_kubectl -n "$SIDB_CASE_NAMESPACE" get secret "$SIDB_TCPS_TLS_SECRET" \
  -o jsonpath='{.data.tls\.crt}' | base64 -d) -noout -text 2>/dev/null |
  grep -q "${SIDB_DG_STANDBY_NAME}.${SIDB_CASE_NAMESPACE}.svc.cluster.local"; then
  echo "$(sidb_case_label): regenerating TCPS TLS Secret with primary+standby SANs"
  old_primary_hash="$(sidb_case_kubectl -n "$SIDB_CASE_NAMESPACE" get singleinstancedatabase \
    "$SIDB_DG_PRIMARY_NAME" -o jsonpath='{.status.tcpsTlsSecretHash}' 2>/dev/null || true)"
  temporary_dir="$(mktemp -d "${TMPDIR:-/tmp}/sidb-dg-tcps.XXXXXX")"
  san="$(sidb_case_tcps_san_for_name "$SIDB_DG_PRIMARY_NAME"),$(sidb_case_tcps_san_for_name "$SIDB_DG_STANDBY_NAME")"
  openssl req -x509 -nodes -newkey rsa:2048 -days 365 \
    -subj "/CN=${SIDB_DG_PRIMARY_NAME}" \
    -addext "subjectAltName=${san}" \
    -keyout "$temporary_dir/tls.key" -out "$temporary_dir/tls.crt" >/dev/null 2>&1
  sidb_case_kubectl -n "$SIDB_CASE_NAMESPACE" create secret tls "$SIDB_TCPS_TLS_SECRET" \
    --cert="$temporary_dir/tls.crt" --key="$temporary_dir/tls.key" \
    --dry-run=client -o yaml |
    sidb_case_kubectl -n "$SIDB_CASE_NAMESPACE" apply -f - >/dev/null
  rm -rf -- "$temporary_dir"

  # Wait until primary reconciles the rotated secret into a new wallet before
  # creating standby (which would otherwise mint a mismatched TCPS leaf).
  SIDB_CASE_STAGE=wait-primary-tcps-secret-reconfig
  elapsed=0
  timeout="${SIDB_DG_TLS_RECONFIG_TIMEOUT_SECONDS:-900}"
  while [ "$elapsed" -lt "$timeout" ]; do
    new_hash="$(sidb_case_kubectl -n "$SIDB_CASE_NAMESPACE" get singleinstancedatabase \
      "$SIDB_DG_PRIMARY_NAME" -o jsonpath='{.status.tcpsTlsSecretHash}' 2>/dev/null || true)"
    primary_status="$(sidb_case_kubectl -n "$SIDB_CASE_NAMESPACE" get singleinstancedatabase \
      "$SIDB_DG_PRIMARY_NAME" -o jsonpath='{.status.status}' 2>/dev/null || true)"
    if [ -n "$new_hash" ] && [ "$new_hash" != "$old_primary_hash" ] && [ "$primary_status" = "Healthy" ]; then
      echo "$(sidb_case_label): primary TCPS wallet refreshed after TLS Secret rotate (hash changed)"
      break
    fi
    echo "$(sidb_case_label): waiting for primary TCPS reconfig after TLS Secret rotate (${elapsed}s) status=${primary_status:-?} hash_changed=$([ "$new_hash" != "$old_primary_hash" ] && echo yes || echo no)"
    sleep "$SIDB_POLL_INTERVAL"
    elapsed=$((elapsed + SIDB_POLL_INTERVAL))
  done
  if [ "$elapsed" -ge "$timeout" ]; then
    echo "$(sidb_case_label): timed out waiting for primary TCPS reconfig after TLS Secret rotate" >&2
    exit 1
  fi
else
  echo "$(sidb_case_label): TCPS TLS Secret already includes standby SANs; skipping regenerate"
fi

SIDB_CASE_STAGE=apply-standby
sidb_dg_render_standby
sidb_case_kubectl -n "$SIDB_CASE_NAMESPACE" apply --dry-run=server -f "$SIDB_DG_RENDERED_STANDBY" >/dev/null
sidb_case_kubectl -n "$SIDB_CASE_NAMESPACE" apply -f "$SIDB_DG_RENDERED_STANDBY"
SIDB_CASE_STAGE=wait-standby
bash "$SIDB_CASE_DIR/../common/wait-ready.sh" "$SIDB_DG_STANDBY_NAME" PHYSICAL_STANDBY
SIDB_CASE_STAGE=wait-rendered-broker-spec
sidb_dg_wait_rendered_spec
SIDB_CASE_STAGE=wait-tcps-client-wallets
sidb_dg_ensure_tcps_client_wallets
SIDB_CASE_STAGE=generate-broker
sidb_dg_generate_broker
SIDB_CASE_STAGE=apply-broker
sidb_case_kubectl -n "$SIDB_CASE_NAMESPACE" apply --dry-run=server -f "$SIDB_DG_RENDERED_BROKER" >/dev/null
sidb_case_kubectl -n "$SIDB_CASE_NAMESPACE" apply -f "$SIDB_DG_RENDERED_BROKER"
SIDB_CASE_STAGE=wait-broker
sidb_dg_wait_broker
echo "$(sidb_case_label): primary, standby and DataguardBroker reached Healthy (TCPS)"
