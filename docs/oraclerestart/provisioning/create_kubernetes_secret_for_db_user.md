# Create Kubernetes secret for db user

Oracle Restart supports two database-secret layouts for both `spec.dbSecret` and `spec.tdeWalletSecret`.

Use this page for both secrets:
- for `dbSecret`, create the secret name used in your YAML, typically `db-user-pass-pkutl`
- for `tdeWalletSecret`, create the TDE secret name used in your YAML, typically `tde-user-pass-pkutl`

## Option 1: OpenSSL encrypted password file

Use this when the Oracle Restart YAML sets `dbSecret.pwdFileName` or `tdeWalletSecret.pwdFileName`, for example `pwdfile.enc`, and uses `encryptionType: pkeyutl`. The default `pkeyopt` is `rsa_padding_mode:oaep;rsa_oaep_md:sha256;rsa_mgf1_md:sha256`.

Create the Kubernetes secret using a password in a text file, and then encrypt it using an `openssl` key. The text file will be removed after the secret is created. Note that openssl _must_ be installed on worker nodes.

**NOTE:** The openssl version on the system where you run the commands in this procedure to generate the secret and the openssl version of the Oracle Restart Slim Image must be compatible with each other.

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

# Deleting the existing secret
# Replace db-user-pass-pkutl with the secret name from your YAML, for example tde-user-pass-pkutl
kubectl delete secret db-user-pass-pkutl -n orestart --ignore-not-found

# Create the Kubernetes secret in namespace "orestart"
kubectl create secret generic db-user-pass-pkutl --from-file=/tmp/.secrets/pwdfile.enc --from-file=/tmp/.secrets/key.pem -n orestart

# Check the secret details
kubectl get secret db-user-pass-pkutl -n orestart -o json | jq -r '.data | keys[]'
```

Expected secret keys:

```text
key.pem
pwdfile.enc
```

## Option 2: Base64 `pwdfile`

Use this when the Oracle Restart YAML sets `encryptionType: base64` and omits `pwdFileName`. You can keep `keyFileName: key.pem` as in the RAC encoded-secret flow, or use `key: key.pem`.

```sh
mkdir -p /tmp/.secrets

# Generate the required key file and a Base64-encoded pwdfile entry
openssl genrsa -out /tmp/.secrets/key.pem
printf 'Oracle_23ai' | base64 -w0 > /tmp/.secrets/pwdfile

# Recreate the Kubernetes secret in namespace "orestart"
# Replace db-user-pass-pkutl with the secret name from your YAML, for example tde-user-pass-pkutl
kubectl delete secret db-user-pass-pkutl -n orestart --ignore-not-found
kubectl create secret generic db-user-pass-pkutl \
  --from-file=/tmp/.secrets/pwdfile \
  --from-file=/tmp/.secrets/key.pem \
  -n orestart

# Verify the secret keys
kubectl get secret db-user-pass-pkutl -n orestart -o json | jq -r '.data | keys[]'
```

Expected secret keys:

```text
key.pem
pwdfile
```
