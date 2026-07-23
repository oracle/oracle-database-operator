# PrivateAI Auth Secret

Use this helper to create, update, and manage the auth secret used by:

- `spec.security.authEnabled: true`
- `spec.security.secret.name`

The auth secret in this flow is auth-only. It does not generate TLS files.

## Files

- [create_auth_secret.sh](./create_auth_secret.sh): creates or updates the auth secret and manages JSON API keys

## Secret Keys

The script writes these secret entries:

- `api-key`
- `privateai-ssl-pwd`

`api-key` remains the canonical key expected by the current PrivateAI operator status checks. If you provide multiple API keys with repeated `--api-key` flags, the script writes them into the same `api-key` file, one per line.

If your PrivateAI image expects a different file format for multiple keys, prepare that file yourself and pass it with `--api-keys-file`.

For structured key management, the script can also maintain `api-keys.json` in the same Secret. The JSON entry stores key metadata such as `key_id`, `alias`, and `active`. Whenever JSON keys are changed, the script regenerates the compatibility `api-key` file from the keys where `active` is `true`.

The JSON entry uses this shape:

```json
[
  { "key_id": "client1", "alias": "ServiceA", "key": "abc123", "active": true },
  { "key_id": "client2", "alias": "ServiceB", "key": "xyz789", "active": false }
]
```

**NOTE:** By default, when you run the script, the files `api-key` and `privateai-ssl-pwd` are written to a directory named `privateai-auth-secret` under the current working directory.

**NOTE:** JSON key-management commands require `jq`.

## Quick Start

Generate one API key and create the secret:

```bash
cd oracle-database-operator/docs/privateai/auth-secret

./create_auth_secret.sh \
  --namespace pai \
  --secret-name paisecret \
  --generate-api-key \
  --ssl-pwd-value '<PASSWORD>'
  --list-secret
```

Create the secret with multiple API keys:

```bash
cd oracle-database-operator/docs/privateai/auth-secret

./create_auth_secret.sh \
  --namespace pai \
  --secret-name paisecret \
  --api-key <API_KEY1> \
  --api-key <API_KEY2> \
  --ssl-pwd-value '<PASSWORD>'
```

Use a preformatted `api-key` file:

```bash
cd oracle-database-operator/docs/privateai/auth-secret

./create_auth_secret.sh \
  --namespace pai \
  --secret-name paisecret \
  --api-keys-file ./api-key \
  --ssl-pwd-file ./privateai-ssl-pwd
```

Create the secret with one structured JSON API key:

```bash
cd oracle-database-operator/docs/privateai/auth-secret

./create_auth_secret.sh create \
  --namespace pai \
  --secret-name paisecret \
  --generate-api-key \
  --json-api-keys \
  --key-id client1 \
  --alias ServiceA \
  --active true \
  --ssl-pwd-value '<PASSWORD>' \
  --list-secret
```

## Wire It to PrivateAI

```yaml
spec:
  security:
    authEnabled: true
    secret:
      name: paisecret
      mountLocation: /privateai/ssl
    tls:
      secretName: privateai-tls
      mountLocation: /privateai/ssl
```

If `mountLocation` is omitted for `spec.security.secret` or `spec.security.tls`, the PrivateAI controller defaults it to `/privateai/ssl`.

## Update an Existing Secret

The script uses `kubectl apply`, so rerun it with the same secret name to update the existing secret in place. The PrivateAI controller watches the secret resource version and rolls the deployment when the auth secret changes.

Example:

```bash
cd oracle-database-operator/docs/privateai/auth-secret

./create_auth_secret.sh \
  --namespace pai \
  --secret-name paisecret \
  --api-key replacement-token-one \
  --api-key replacement-token-two \
  --ssl-pwd-file ./privateai-ssl-pwd
```

## Manage JSON API Keys

The same helper can add, activate, deactivate, delete, and list structured JSON API keys. These commands update `api-keys.json` and regenerate the legacy `api-key` entry from active keys only.

The examples below start from this JSON state:

```json
[
  { "key_id": "client1", "alias": "ServiceA", "key": "abc123", "active": true },
  { "key_id": "client2", "alias": "ServiceB", "key": "xyz789", "active": false }
]
```

Add an active key and generate the secret value:

```bash
cd oracle-database-operator/docs/privateai/auth-secret

./create_auth_secret.sh add-key \
  --namespace pai \
  --secret-name paisecret \
  --key-id client3 \
  --alias ServiceC \
  --generate-api-key \
  --active true
```

Add an inactive key with a provided value:

```bash
./create_auth_secret.sh add-key \
  --namespace pai \
  --secret-name paisecret \
  --key-id client2 \
  --alias ServiceB \
  --key xyz789 \
  --active false
```

Deactivate a key by alias:

```bash
./create_auth_secret.sh set-active \
  --namespace pai \
  --secret-name paisecret \
  --alias ServiceB \
  --active true
```

Deactivate a key by `key_id`:

```bash
./create_auth_secret.sh set-active \
  --namespace pai \
  --secret-name paisecret \
  --key-id client1 \
  --active false
```

Delete a key:

```bash
./create_auth_secret.sh delete-key \
  --namespace pai \
  --secret-name paisecret \
  --alias ServiceB
```

List JSON keys without printing key values:

```bash
./create_auth_secret.sh list-key \
  --namespace pai \
  --secret-name paisecret
```

Example `list-key` output:

```text
KEY_ID    ALIAS     ACTIVE
client1   ServiceA  true
client2   ServiceB  false
```

The script requires unique `key_id` and unique `alias` values. Use `active: false` when you want to disable a key but keep it in `api-keys.json`; use `delete-key` when you want to remove the entry.

## Manual Patch Example

Patch both secret entries directly:

```bash
kubectl patch secret paisecret -n pai --type merge -p '{
  "stringData": {
    "api-key": "token-one\ntoken-two",
    "privateai-ssl-pwd": "<< SSL Password >>"
  }
}'
```

If you want to update from local files instead of inline strings:

```bash
kubectl create secret generic paisecret \
  --namespace pai \
  --from-file=api-key=./api-key \
  --from-file=privateai-ssl-pwd=./privateai-ssl-pwd \
  --dry-run=client -o yaml | kubectl apply -f -
```

## List the Secret

Show the secret object:

```bash
kubectl get secret paisecret -n pai
```

Show the secret keys:

```bash
kubectl get secret paisecret -n pai -o go-template='{{range $k, $v := .data}}{{printf "%s\n" $k}}{{end}}'
```

Decode the `api-key` entry:

```bash
kubectl get secret paisecret -n pai -o jsonpath='{.data.api-key}' | base64 -d
```

Decode the structured key metadata:

```bash
kubectl get secret paisecret -n pai -o jsonpath='{.data.api-keys\.json}' | base64 -d | jq .
```
