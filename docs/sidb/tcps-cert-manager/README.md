# TCPS Cert-Manager Flow

This folder is a sanitized, reusable version of the `tcps-cert-manager` setup used for SIDB primary and True Cache TCPS certificates.

It creates:

- a self-signed root CA on the primary cluster
- an intermediate CA on the primary cluster
- the intermediate CA secret on the True Cache cluster
- a primary leaf secret, default `sidb-primary-tcps-tls`
- a True Cache leaf secret, default `sidb-standby-tcps-tls`
- an optional `primary-peer-ca` trust secret on the True Cache side

Each leaf secret contains:

- `tls.crt`
- `tls.key`
- `ca.crt`

The leaf certificates are issued with both **Server Authentication** and **Client Authentication** extended key usages. Data Guard TCPS redo transport requires both because a database acts as both a TCPS server and a TCPS client when communicating with its peer.

---

## Files

- `env.sh`: shared defaults and kubectl helpers
- `01-create-namespace.sh` through `11-verify-secrets.sh`: one step per action
- `run-all.sh`: runs the full flow in order

---

## Defaults

If you do not export variables first, the scripts use these defaults:

```bash
PRIMARY_CTX="$(kubectl config current-context)"
STANDBY_CTX="$PRIMARY_CTX"

NS=default

PRIMARY_CERT_NAME=sidb-primary-tcps
PRIMARY_SECRET_NAME=sidb-primary-tcps-tls
PRIMARY_DNS=orcl-production.internal.example.com
PRIMARY_DNS_NAMES=orcl-production.internal.example.com,orcl-production,orcl-production.default.svc.cluster.local

STANDBY_CERT_NAME=sidb-standby-tcps
STANDBY_SECRET_NAME=sidb-standby-tcps-tls
STANDBY_DNS=truecache-production.internal.example.com
STANDBY_DNS_NAMES=truecache-production.internal.example.com,truecache-production,truecache-production.default.svc.cluster.local
```

Override them when your namespace, DNS names, or Kubernetes contexts differ:

```bash
export PRIMARY_CTX=$(kubectl config current-context)
export STANDBY_CTX=$PRIMARY_CTX
export NS=my-namespace
export PRIMARY_CERT_NAME=sidb-primary-tcps
export PRIMARY_SECRET_NAME=sidb-primary-tcps-tls
export PRIMARY_DNS=sidb-primary
export PRIMARY_DNS_NAMES=sidb-primary,sidb-primary.sidb,sidb-primary.sidb.svc,sidb-primary.sidb.svc.cluster.local
export STANDBY_CERT_NAME=sidb-standby-tcps
export STANDBY_SECRET_NAME=sidb-standby-tcps-tls
export STANDBY_DNS=sidb-standby
export STANDBY_DNS_NAMES=sidb-standby,sidb-standby.sidb,sidb-standby.sidb.svc,sidb-standby.sidb.svc.cluster.local
```

The leaf issuance steps use the first hostname in `PRIMARY_DNS_NAMES` or `STANDBY_DNS_NAMES` as the certificate Common Name (CN) and add every hostname in the list as a Subject Alternative Name (SAN).

`10-copy-primary-ca-to-standby.sh` rewrites both TLS secrets so that:

- `tls.crt` contains the complete **leaf + intermediate + root** certificate chain
- `ca.crt` contains the **intermediate + root** CA bundle

It then copies the primary trust bundle to the True Cache cluster.

---

## Run

Before running the scripts, export the namespace you want to use.

For example:

```bash
export NS=my-namespace
```

For separate primary and standby clusters, also export the Kubernetes contexts:

```bash
export PRIMARY_CTX=context-primary
export STANDBY_CTX=context-standby
export NS=my-namespace
```

Execute the complete workflow:

```bash
./docs/sidb/tcps-cert-manager/run-all.sh
```

If you want a simpler compatibility wrapper that can issue only the primary leaf, only the standby leaf, or both, use:

```bash
./docs/sidb/script/setup-sidb-tcps-cert-manager.sh [primary|standby|both]
```

Examples:

```bash
./docs/sidb/script/setup-sidb-tcps-cert-manager.sh
./docs/sidb/script/setup-sidb-tcps-cert-manager.sh primary
./docs/sidb/script/setup-sidb-tcps-cert-manager.sh standby
BOOTSTRAP_CA=false ./docs/sidb/script/setup-sidb-tcps-cert-manager.sh standby
./docs/sidb/script/setup-sidb-tcps-cert-manager.sh both
```

With no argument, the wrapper generates both primary and standby certificates.

Setting

```bash
BOOTSTRAP_CA=false
```

skips root and intermediate CA creation and expects the intermediate issuer to already exist on the target cluster(s).

You can also execute the workflow one step at a time:

```bash
./docs/sidb/tcps-cert-manager/01-create-namespace.sh

./docs/sidb/tcps-cert-manager/02-bootstrap-root-ca.sh
./docs/sidb/tcps-cert-manager/03-create-root-issuer.sh
./docs/sidb/tcps-cert-manager/04-create-intermediate-ca.sh
./docs/sidb/tcps-cert-manager/05-create-primary-intermediate-issuer.sh
./docs/sidb/tcps-cert-manager/06-copy-intermediate-to-standby.sh
./docs/sidb/tcps-cert-manager/07-create-standby-intermediate-issuer.sh
./docs/sidb/tcps-cert-manager/08-issue-primary-certificate.sh
./docs/sidb/tcps-cert-manager/09-issue-standby-certificate.sh
./docs/sidb/tcps-cert-manager/10-copy-primary-ca-to-standby.sh
./docs/sidb/tcps-cert-manager/11-verify-secrets.sh
```

---

## Validation

### Verify Certificate resources

Ensure cert-manager successfully issued both certificates.

```bash
kubectl -n "${NS}" get certificates

kubectl -n "${NS}" describe certificate "${PRIMARY_CERT_NAME}"

kubectl -n "${NS}" describe certificate "${STANDBY_CERT_NAME}"
```

Expected:

- both Certificate resources report `READY=True`
- no issuance or renewal errors are present

---

### Verify TLS secrets

Ensure both TLS secrets exist.

```bash
kubectl -n "${NS}" get secret \
  "${PRIMARY_SECRET_NAME}" \
  "${STANDBY_SECRET_NAME}"
```

Expected:

- both secrets exist
- type is `kubernetes.io/tls`

Each secret should contain:

- `tls.crt`
- `tls.key`
- `ca.crt`

Run the built-in verification script:

```bash
./docs/sidb/tcps-cert-manager/11-verify-secrets.sh
```

---

### Validate the leaf certificates

Inspect the primary certificate:

```bash
kubectl -n "${NS}" get secret "${PRIMARY_SECRET_NAME}" \
  -o jsonpath='{.data.tls\.crt}' \
| base64 -d \
| openssl x509 -noout -text
```

Inspect the standby certificate:

```bash
kubectl -n "${NS}" get secret "${STANDBY_SECRET_NAME}" \
  -o jsonpath='{.data.tls\.crt}' \
| base64 -d \
| openssl x509 -noout -text
```

Verify the following:

- Subject Common Name (CN)
- Subject Alternative Names (SANs)
- Issuer
- Validity period
- Key Usage
- Extended Key Usage

Each certificate should contain:

- TLS Web Server Authentication
- TLS Web Client Authentication

To inspect only the key usages:

```bash
kubectl -n "${NS}" get secret "${PRIMARY_SECRET_NAME}" \
  -o jsonpath='{.data.tls\.crt}' \
| base64 -d \
| openssl x509 -noout -text \
| egrep -A3 'Key Usage|Extended Key Usage'
```

If checking from inside a database pod:

```bash
openssl x509 \
  -in /run/secrets/tls_secret/tls.crt \
  -noout \
  -text \
| egrep -A3 'Key Usage|Extended Key Usage'
```

---

### Verify the certificate chain

```bash
kubectl -n "${NS}" get secret "${PRIMARY_SECRET_NAME}" \
  -o jsonpath='{.data.tls\.crt}' \
| base64 -d \
> /tmp/leaf-chain.pem

kubectl -n "${NS}" get secret "${PRIMARY_SECRET_NAME}" \
  -o jsonpath='{.data.ca\.crt}' \
| base64 -d \
> /tmp/ca-chain.pem

openssl verify \
  -CAfile /tmp/ca-chain.pem \
  /tmp/leaf-chain.pem
```

Expected output:

```text
/tmp/leaf-chain.pem: OK
```

---

## Cleanup

Delete all generated resources if you need to recreate the TCPS certificates from scratch.

For a single-cluster deployment:

```bash
kubectl -n "${NS}" delete certificate \
  sidb-primary-tcps \
  sidb-standby-tcps \
  --ignore-not-found

kubectl -n "${NS}" delete secret \
  sidb-primary-tcps-tls \
  sidb-standby-tcps-tls \
  primary-peer-ca \
  --ignore-not-found

kubectl -n "${NS}" delete certificate \
  sidb-tcps-intermediate-ca \
  sidb-tcps-root-ca \
  --ignore-not-found

kubectl -n "${NS}" delete secret \
  sidb-tcps-intermediate-ca-secret \
  sidb-tcps-root-ca-secret \
  --ignore-not-found

kubectl -n "${NS}" delete issuer \
  sidb-tcps-intermediate-issuer \
  sidb-tcps-root-ca-issuer \
  --ignore-not-found

kubectl delete clusterissuer \
  sidb-tcps-selfsigned-bootstrap \
  --ignore-not-found
```

For separate primary and standby clusters:

```bash
kubectl --context "${PRIMARY_CTX}" -n "${NS}" delete certificate \
  "${PRIMARY_CERT_NAME}" \
  "${INT_CERT_NAME}" \
  "${ROOT_CERT_NAME}" \
  --ignore-not-found

kubectl --context "${PRIMARY_CTX}" -n "${NS}" delete secret \
  "${PRIMARY_SECRET_NAME}" \
  "${INT_SECRET_NAME}" \
  "${ROOT_SECRET_NAME}" \
  --ignore-not-found

kubectl --context "${PRIMARY_CTX}" -n "${NS}" delete issuer \
  "${INTERMEDIATE_ISSUER_NAME}" \
  "${ROOT_ISSUER_NAME}" \
  --ignore-not-found

kubectl --context "${PRIMARY_CTX}" delete clusterissuer \
  "${ROOT_CLUSTER_ISSUER_NAME}" \
  --ignore-not-found

kubectl --context "${STANDBY_CTX}" -n "${NS}" delete certificate \
  "${STANDBY_CERT_NAME}" \
  --ignore-not-found

kubectl --context "${STANDBY_CTX}" -n "${NS}" delete secret \
  "${STANDBY_SECRET_NAME}" \
  "${INT_SECRET_NAME}" \
  "${PRIMARY_PEER_CA_SECRET_NAME}" \
  --ignore-not-found

kubectl --context "${STANDBY_CTX}" -n "${NS}" delete issuer \
  "${INTERMEDIATE_ISSUER_NAME}" \
  --ignore-not-found
```

---

## SIDB Manifest Wiring

Use the generated primary TLS secret:

```yaml
spec:
  security:
    tcps:
      enabled: true
      tlsSecret: sidb-primary-tcps-tls
```

Use the generated True Cache TLS secret:

```yaml
spec:
  security:
    tcps:
      enabled: true
      tlsSecret: sidb-standby-tcps-tls
```
