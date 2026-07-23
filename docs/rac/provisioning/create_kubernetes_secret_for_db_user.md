# Create Kubernetes secret for db user

RAC supports two database-secret layouts. Choose the one that matches the YAML you are applying.

Use this page for both `spec.dbSecret` and `spec.tdeWalletSecret`:
- for `dbSecret`, create the secret name used in your YAML, such as `db-user-pass`
- for `tdeWalletSecret`, use the same file layout but create the TDE secret name from your YAML, such as `tde-user-pass`

## Option 1: OpenSSL encrypted password file

Use this when the RAC YAML sets `keyFileName`, `pwdFileName`, and `encryptionType: pkeyutl`, for example `key.pem` and `pwdfile.enc`. The default `pkeyopt` is `rsa_padding_mode:oaep;rsa_oaep_md:sha256;rsa_mgf1_md:sha256`.

```sh
mkdir -p /tmp/.secrets

# Generate a private/public key pair and an encrypted password file
echo Oracle_23ai > /tmp/.secrets/pwdfile.txt
openssl genrsa -out /tmp/.secrets/key.pem
openssl rsa -in /tmp/.secrets/key.pem -out /tmp/.secrets/key.pub -pubout
openssl pkeyutl -in /tmp/.secrets/pwdfile.txt -out /tmp/.secrets/pwdfile.enc \
  -pubin -inkey /tmp/.secrets/key.pub -encrypt \
  -pkeyopt rsa_padding_mode:oaep \
  -pkeyopt rsa_oaep_md:sha256 \
  -pkeyopt rsa_mgf1_md:sha256
rm -f /tmp/.secrets/pwdfile.txt

# Recreate the Kubernetes secret in namespace rac
# Replace db-user-pass with the secret name from your YAML, for example tde-user-pass
kubectl delete secret db-user-pass -n rac --ignore-not-found
kubectl create secret generic db-user-pass \
  --from-file=/tmp/.secrets/pwdfile.enc \
  --from-file=/tmp/.secrets/key.pem \
  -n rac

# Verify the secret keys
kubectl get secret db-user-pass -n rac -o json | jq -r '.data | keys[]'
```

Expected secret keys:

```text
key.pem
pwdfile.enc
```

## Option 2: Base64 `pwdfile`

Use this when the RAC YAML sets `pwdFileName: pwdfile` and omits `keyFileName`, such as `racdb_prov_encoded.yaml`.

```sh
mkdir -p /tmp/.secrets

# Generate a Base64-encoded pwdfile entry
printf 'Oracle_23ai' | base64 -w0 > /tmp/.secrets/pwdfile

# Recreate the Kubernetes secret in namespace rac
# Replace db-user-pass with the secret name from your YAML
kubectl delete secret db-user-pass -n rac --ignore-not-found
kubectl create secret generic db-user-pass \
  --from-file=/tmp/.secrets/pwdfile \
  -n rac

# Verify the secret keys
kubectl get secret db-user-pass -n rac -o json | jq -r '.data | keys[]'
```

Expected secret keys:

```text
pwdfile
```
