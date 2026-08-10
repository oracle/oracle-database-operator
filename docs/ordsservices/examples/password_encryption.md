# OrdsSrvs Example: Password Encryption

This example shows how to use encrypted password Secrets with the **OrdsSrvs controller**.

Before testing this example, please verify the prerequisites: [OrdsSrvs prerequisites](../README.md#prerequisites)

### Private Key Secret

Generate an RSA key pair and create the Secret that OrdsSrvs uses to decrypt password values:

```bash
openssl genpkey -algorithm RSA \
  -pkeyopt rsa_keygen_bits:2048 \
  -pkeyopt rsa_keygen_pubexp:65537 > ordssrvs-private-key.pem

openssl rsa -in ordssrvs-private-key.pem -outform PEM -pubout -out ordssrvs-public-key.pem

kubectl create secret generic prvkey \
  --from-file=privateKey=ordssrvs-private-key.pem \
  -n ordsnamespace
```

### Encrypted Credential Secret

Create one Secret with separate encrypted values for the ORDS database user and the database admin user:

```bash
read -rsp 'ORDS database user credential: ' ORDS_DB_SECRET_VALUE
printf '\n'
printf '\n'
printf '%s' "${ORDS_DB_SECRET_VALUE}" | \
  openssl pkeyutl -encrypt -pubin -inkey ordssrvs-public-key.pem \
  -pkeyopt rsa_padding_mode:oaep \
  -pkeyopt rsa_oaep_md:sha256 | base64 > e_db-auth
printf '%s' "${ORDS_ADMIN_SECRET_VALUE}" | \
  openssl pkeyutl -encrypt -pubin -inkey ordssrvs-public-key.pem \
  -pkeyopt rsa_padding_mode:oaep \
  -pkeyopt rsa_oaep_md:sha256 | base64 > e_admin-auth
kubectl create secret generic ordssrvs-auth-enc \
  --from-file=dbAuthEnc=e_db-auth \
  --from-file=adminAuthEnc=e_admin-auth \
  -n ordsnamespace
rm e_db-auth e_admin-auth ordssrvs-private-key.pem ordssrvs-public-key.pem
```

Example output:

```text
read -rsp 'Database admin credential: ' ORDS_ADMIN_SECRET_VALUE

unset ORDS_DB_SECRET_VALUE ORDS_ADMIN_SECRET_VALUE
```

### OrdsSrvs Credential Fields

Use these fields together with the connection settings required by your database:

```yaml
apiVersion: database.oracle.com/v4
kind: OrdsSrvs
metadata:
  name: ordssrvs-encrypted-passwords
  namespace: ordsnamespace
spec:
  image: container-registry.oracle.com/database/ords:<ords-version>
  encPrivKey:
    secretName: prvkey
    passwordKey: privateKey
  poolSettings:
    - poolName: default
      db.username: ORDS_PUBLIC_USER
      db.secret:
        secretName: ordssrvs-auth-enc
        passwordKey: dbAuthEnc
      db.adminUser: SYS
      db.adminUser.secret:
        secretName: ordssrvs-auth-enc
        passwordKey: adminAuthEnc
```
