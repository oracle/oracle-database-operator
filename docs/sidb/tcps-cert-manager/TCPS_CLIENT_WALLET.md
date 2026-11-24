# SIDB TCPS Client Wallet

Use this procedure to create a portable TCPS client wallet from a healthy
TCPS-enabled SingleInstanceDatabase (SIDB). The wallet is for Oracle Net client
connections; it is not the TDE wallet that encrypts database data.

**Important:** Treat the wallet files and the resulting archive as sensitive
connection material. Create and handle them on a secured workstation, store
them in Kubernetes Secrets as required, and never commit them to source
control.

## Check the client wallet

Confirm that TCPS is enabled and that SIDB has published a wallet Secret:

```sh
NS=default
SIDB=sidb-sample

kubectl get singleinstancedatabase "$SIDB" -n "$NS" \
  -o jsonpath='tcps={.status.isTcpsEnabled} walletDir={.status.clientWalletLoc} walletSecret={.status.clientWalletSecret}{"\n"}'
```

`status.isTcpsEnabled` must be `true` before consuming the wallet.
`status.clientWalletLoc` is a path inside the SIDB Pod and is not portable.
`status.clientWalletSecret` identifies the Secret containing the client wallet
files. When `spec.security.tcps.clientWalletSecret` is unset, the operator
normally generates `<sidb-name>-dg-client-wallet`; otherwise it uses the named
Secret.

Confirm the Secret without displaying its values:

```sh
WALLET_SECRET="$(kubectl get singleinstancedatabase "$SIDB" -n "$NS" \
  -o jsonpath='{.status.clientWalletSecret}')"

test -n "$WALLET_SECRET" || {
  echo "The SIDB has not published a client wallet Secret" >&2
  exit 1
}

kubectl describe secret "$WALLET_SECRET" -n "$NS"
```

The Secret normally contains `cwallet.sso`, `sqlnet.ora`, and `tnsnames.ora`.
It can also contain `ewallet.p12`.

## Create a portable wallet archive

Extract the client wallet files from the Secret and remove the SIDB-Pod-only
`MY_WALLET_DIRECTORY` override from `tnsnames.ora` before packaging the
archive:

```sh
TMP="$(mktemp -d)"
chmod 700 "$TMP"

for file in cwallet.sso sqlnet.ora tnsnames.ora; do
  key="${file//./\\.}"
  value="$(kubectl get secret "$WALLET_SECRET" -n "$NS" \
    -o "jsonpath={.data.${key}}")"
  test -n "$value" || {
    echo "Required wallet file is missing: $file" >&2
    exit 1
  }
  printf '%s' "$value" | base64 -d > "$TMP/$file"
done

optional_key="$(kubectl get secret "$WALLET_SECRET" -n "$NS" \
  -o jsonpath='{.data.ewallet\.p12}' 2>/dev/null || true)"
if [ -n "$optional_key" ]; then
  printf '%s' "$optional_key" | base64 -d > "$TMP/ewallet.p12"
fi

awk '
  {
    line = $0
    gsub(/\(MY_WALLET_DIRECTORY[[:space:]]*=[^)]*\)[[:space:]]*/, "", line)
    print line
  }
' "$TMP/tnsnames.ora" > "$TMP/tnsnames.ora.portable"
mv "$TMP/tnsnames.ora.portable" "$TMP/tnsnames.ora"

WALLET_ZIP="$TMP/db_wallet.zip"
zip -q -j "$WALLET_ZIP" \
  "$TMP/cwallet.sso" "$TMP/sqlnet.ora" "$TMP/tnsnames.ora"
if [ -f "$TMP/ewallet.p12" ]; then
  zip -q -j "$WALLET_ZIP" "$TMP/ewallet.p12"
fi

unzip -Z1 "$WALLET_ZIP"
```

Select an alias that is present in the archive; do not assume that the PDB
alias ends in `_TCPS`:

```sh
unzip -p "$WALLET_ZIP" tnsnames.ora \
  | grep -E '^[[:space:]]*[A-Za-z][A-Za-z0-9_.-]*[[:space:]]*='
```

Use `WALLET_ZIP` to create the target application's Secret, then remove the
temporary wallet files when no longer required:

```sh
rm -rf "$TMP"
```

