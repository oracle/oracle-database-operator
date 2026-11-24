#!/usr/bin/env bash
set -Eeuo pipefail

ORESTART_CASE_SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
source "$ORESTART_CASE_SCRIPT_DIR/lib.sh"
orestart_case_init
orestart_validate
mkdir -p "$ORESTART_RESULTS_DIR"

# The namespace is intentionally fixed, but every run starts from an empty
# namespace. This also repairs runs whose after_script was interrupted.
if orestart_kubectl get namespace "$ORESTART_NAMESPACE" >/dev/null 2>&1; then
  echo "Oracle Restart E2E: resetting existing test namespace $ORESTART_NAMESPACE"
  orestart_cleanup
fi
if ! orestart_kubectl get namespace "$ORESTART_NAMESPACE" >/dev/null 2>&1; then
  orestart_kubectl create namespace "$ORESTART_NAMESPACE" >/dev/null
  ORESTART_NAMESPACE_CREATED=true
else
  ORESTART_NAMESPACE_CREATED=false
fi

if [ "$ORESTART_COPY_SECRETS" = true ]; then
  orestart_copy_secret "$ORESTART_DB_SECRET_NAME"
  orestart_copy_secret "$ORESTART_SSH_SECRET_NAME"
  orestart_copy_secret "$ORESTART_TDE_SECRET_NAME"
else
  orestart_kubectl -n "$ORESTART_NAMESPACE" create secret generic "$ORESTART_DB_SECRET_NAME" \
    --from-file=key.pem="$ORESTART_DB_KEY_FILE" --from-file=pwdfile.enc="$ORESTART_DB_PASSWORD_FILE" \
    --dry-run=client -o yaml | orestart_kubectl -n "$ORESTART_NAMESPACE" apply -f - >/dev/null
  orestart_kubectl -n "$ORESTART_NAMESPACE" create secret generic "$ORESTART_SSH_SECRET_NAME" \
    --from-file=ssh-privkey="$ORESTART_SSH_PRIVATE_KEY_FILE" --from-file=ssh-pubkey="$ORESTART_SSH_PUBLIC_KEY_FILE" \
    --dry-run=client -o yaml | orestart_kubectl -n "$ORESTART_NAMESPACE" apply -f - >/dev/null
  orestart_kubectl -n "$ORESTART_NAMESPACE" create secret generic "$ORESTART_TDE_SECRET_NAME" \
    --from-file=key.pem="$ORESTART_TDE_KEY_FILE" --from-file=pwdfile.enc="$ORESTART_TDE_PASSWORD_FILE" \
    --dry-run=client -o yaml | orestart_kubectl -n "$ORESTART_NAMESPACE" apply -f - >/dev/null
fi

orestart_render "$ORESTART_SOURCE_MANIFEST" "$ORESTART_MANIFEST"
orestart_kubectl -n "$ORESTART_NAMESPACE" apply --dry-run=server -f "$ORESTART_MANIFEST" >/dev/null
printf 'export ORESTART_NAMESPACE=%q\nexport ORESTART_NAMESPACE_CREATED=%q\nexport ORESTART_MANIFEST=%q\n' \
  "$ORESTART_NAMESPACE" "$ORESTART_NAMESPACE_CREATED" "$ORESTART_MANIFEST" > "${TMPDIR:-/tmp}/orestart-e2e-${CI_PIPELINE_ID:-local}.env"
chmod 600 "${TMPDIR:-/tmp}/orestart-e2e-${CI_PIPELINE_ID:-local}.env"
echo "Oracle Restart prerequisites prepared: namespace=$ORESTART_NAMESPACE manifest=$ORESTART_MANIFEST"
