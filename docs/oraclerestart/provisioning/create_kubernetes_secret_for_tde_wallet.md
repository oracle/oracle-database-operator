# Create Kubernetes Secret for TDE Wallet

Oracle Restart supports two secret layouts for `spec.tdeWalletSecret`. Create this secret before provisioning the database if your Oracle Restart YAML includes a `tdeWalletSecret` block.

Use this page specifically for `spec.tdeWalletSecret`. For the database user secret (`spec.dbSecret`), see [Create Kubernetes Secret for DB User](./create_kubernetes_secret_for_db_user.md).

**NOTE:** Use `tdeWalletSecret` only when your Oracle Restart deployment is intended to configure Transparent Data Encryption (TDE). If your YAML does not include `spec.tdeWalletSecret`, you do not need to create this secret.

## Option 1: OpenSSL encrypted password file

Use this when the Oracle Restart YAML sets `tdeWalletSecret.keyFileName`, `tdeWalletSecret.pwdFileName`, and `encryptionType: pkeyutl`, for example `key.pem` and `pwdfile.enc`. The default `pkeyopt` is `rsa_padding_mode:oaep;rsa_oaep_md:sha256;rsa_mgf1_md:sha256`.

For example, if your Oracle Restart YAML includes:

```yaml
spec:
  tdeWalletSecret:
    name: tde-user-pass-pkutl
    keyFileName: key.pem
    pwdFileName: pwdfile.enc
    encryptionType: pkeyutl
    pkeyopt: rsa_padding_mode:oaep;rsa_oaep_md:sha256;rsa_mgf1_md:sha256
```

**NOTE:** The openssl version on the system where you run these commands to generate the secret and the openssl version of the Oracle Restart Slim Image must be compatible with each other.

```sh
mkdir -p /tmp/.secrets

# Generate a private/public key pair and an encrypted password file
echo Oracle_23ai > /tmp/.secrets/tdepwdfile.txt
openssl genrsa -out /tmp/.secrets/key.pem
openssl rsa -in /tmp/.secrets/key.pem -out /tmp/.secrets/key.pub -pubout
openssl pkeyutl -in /tmp/.secrets/tdepwdfile.txt -out /tmp/.secrets/pwdfile.enc \
  -pubin -inkey /tmp/.secrets/key.pub -encrypt \
  -pkeyopt rsa_padding_mode:oaep \
  -pkeyopt rsa_oaep_md:sha256 \
  -pkeyopt rsa_mgf1_md:sha256
rm -f /tmp/.secrets/tdepwdfile.txt

# Delete any existing secret with the same name
# Replace tde-user-pass-pkutl with the secret name from your YAML
kubectl delete secret tde-user-pass-pkutl -n orestart --ignore-not-found

# Create the Kubernetes secret in namespace "orestart"
kubectl create secret generic tde-user-pass-pkutl \
  --from-file=/tmp/.secrets/pwdfile.enc \
  --from-file=/tmp/.secrets/key.pem \
  -n orestart

# Verify the secret keys
kubectl get secret tde-user-pass-pkutl -n orestart -o json | jq -r '.data | keys[]'
```

Expected secret keys:

```text
key.pem
pwdfile.enc
```

## Option 2: Base64 `pwdfile`

Use this when the Oracle Restart YAML sets `tdeWalletSecret.encryptionType: base64` and omits `tdeWalletSecret.pwdFileName`. Keep `keyFileName: key.pem` in the YAML alongside `encryptionType: base64`.

For example, if your Oracle Restart YAML includes:

```yaml
spec:
  tdeWalletSecret:
    name: tde-user-pass-pkutl
    keyFileName: key.pem
    encryptionType: base64
```

```sh
mkdir -p /tmp/.secrets

# Generate the required key file and a Base64-encoded pwdfile entry
openssl genrsa -out /tmp/.secrets/key.pem
printf 'Oracle_23ai' | base64 -w0 > /tmp/.secrets/pwdfile

# Delete any existing secret with the same name
# Replace tde-user-pass-pkutl with the secret name from your YAML
kubectl delete secret tde-user-pass-pkutl -n orestart --ignore-not-found

# Create the Kubernetes secret in namespace "orestart"
kubectl create secret generic tde-user-pass-pkutl \
  --from-file=/tmp/.secrets/pwdfile \
  --from-file=/tmp/.secrets/key.pem \
  -n orestart

# Verify the secret keys
kubectl get secret tde-user-pass-pkutl -n orestart -o json | jq -r '.data | keys[]'
```

Expected secret keys:

```text
key.pem
pwdfile
```

## Using the same key pair for dbSecret and tdeWalletSecret

If both `spec.dbSecret` and `spec.tdeWalletSecret` use `key.pem`, you can generate one RSA key pair and reuse the same `key.pem` in both secrets. Create the two secrets independently with different names but using the same key file:

```sh
mkdir -p /tmp/.secrets

# Generate a single key pair for both secrets
openssl genrsa -out /tmp/.secrets/key.pem
openssl rsa -in /tmp/.secrets/key.pem -out /tmp/.secrets/key.pub -pubout

# Encrypt the DB password
echo Oracle_23ai > /tmp/.secrets/dbpwdfile.txt
openssl pkeyutl -in /tmp/.secrets/dbpwdfile.txt -out /tmp/.secrets/pwdfile.enc \
  -pubin -inkey /tmp/.secrets/key.pub -encrypt \
  -pkeyopt rsa_padding_mode:oaep \
  -pkeyopt rsa_oaep_md:sha256 \
  -pkeyopt rsa_mgf1_md:sha256
rm -f /tmp/.secrets/dbpwdfile.txt

# Encrypt the TDE wallet password (can be different from the DB password)
echo Oracle_TDE_23ai > /tmp/.secrets/tdepwdfile.txt
openssl pkeyutl -in /tmp/.secrets/tdepwdfile.txt -out /tmp/.secrets/tdepwdfile.enc \
  -pubin -inkey /tmp/.secrets/key.pub -encrypt \
  -pkeyopt rsa_padding_mode:oaep \
  -pkeyopt rsa_oaep_md:sha256 \
  -pkeyopt rsa_mgf1_md:sha256
rm -f /tmp/.secrets/tdepwdfile.txt

# Create the DB secret
kubectl delete secret db-user-pass-pkutl -n orestart --ignore-not-found
kubectl create secret generic db-user-pass-pkutl \
  --from-file=pwdfile.enc=/tmp/.secrets/pwdfile.enc \
  --from-file=/tmp/.secrets/key.pem \
  -n orestart

# Create the TDE wallet secret (different name, different encrypted password, same key.pem)
kubectl delete secret tde-user-pass-pkutl -n orestart --ignore-not-found
kubectl create secret generic tde-user-pass-pkutl \
  --from-file=pwdfile.enc=/tmp/.secrets/tdepwdfile.enc \
  --from-file=/tmp/.secrets/key.pem \
  -n orestart

# Verify both secrets
kubectl get secret db-user-pass-pkutl -n orestart -o json | jq -r '.data | keys[]'
kubectl get secret tde-user-pass-pkutl -n orestart -o json | jq -r '.data | keys[]'
```

Both secrets will have the same key structure:

```text
key.pem
pwdfile.enc
```
