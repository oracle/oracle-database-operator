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

## Files

- `env.sh`: shared defaults and kubectl helpers
- `01-create-namespace.sh` through `11-verify-secrets.sh`: one step per action
- `run-all.sh`: runs the full flow in order

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

Override them when your namespace, DNS names, or contexts differ:

```bash
export PRIMARY_CTX=context-primary
export STANDBY_CTX=context-standby
export NS=default
export PRIMARY_DNS_NAMES=orcl-production.internal.example.com,orcl-production,orcl-production.default.svc.cluster.local
export STANDBY_DNS_NAMES=truecache-production.internal.example.com,truecache-production,truecache-production.default.svc.cluster.local
```

The leaf issuance steps use the first hostname in `PRIMARY_DNS_NAMES` or `STANDBY_DNS_NAMES` as the certificate common name and add every hostname in the list as a SAN.

`10-copy-primary-ca-to-standby.sh` rewrites both TLS secrets so `tls.crt` carries the full `leaf + root` chain and `ca.crt` contains the `intermediate + root` CA bundle before copying the primary trust bundle to the True Cache side.

## Run

Execute the full flow:

```bash
./docs/sidb/tcps-cert-manager/run-all.sh
```

Or execute individual steps:

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

## SIDB Manifest Wiring

Use the generated secrets like this:

```yaml
spec:
  security:
    tcps:
      enabled: true
      tlsSecret: sidb-primary-tcps-tls
```

and

```yaml
spec:
  security:
    tcps:
      enabled: true
      tlsSecret: sidb-standby-tcps-tls
```
