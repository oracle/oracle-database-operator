# Separate cluster access: manual intermediate CA handoff

Use this runbook when primary and peer clusters are accessed separately and one terminal **cannot** use both kubeconfigs at once.

It implements the same PKI as `run-all.sh`:

1. Create the root + intermediate CA on the **primary** cluster
2. Copy the intermediate CA Secret to every peer cluster that hosts a Data Guard member
3. Issue each member’s leaf certificate **locally** from that shared intermediate Issuer

If one admin client can reach both clusters, prefer [`README.md`](./README.md) / `run-all.sh` instead. Both paths produce the same shared-CA trust model for Data Guard TCPS (and True Cache peers that reuse these secret names).

Cloud-agnostic: no dependence on bastion hosts or a specific provider. Any secure Secret transfer between admin hosts works.

---

## Trust model

| Artifact | Shared across clusters? |
| --- | --- |
| Root CA | Yes — created on primary |
| Intermediate CA Secret | Yes — created on primary, copied to peers |
| Intermediate Issuer | Yes — same CA Secret name on each cluster |
| Leaf TLS Secret | No — each database member has its own leaf |

Primary and standby leaves have different CN/SANs and keys. They must share the **same certification path**. Independent CA trees break TCPS mutual authentication for Data Guard.

Leaf certificates must include **Server Authentication** and **Client Authentication** EKUs.

---

## Warnings

1. Do **not** bootstrap a new CA on a peer cluster:

   ```bash
   ./docs/sidb/script/setup-sidb-tcps-cert-manager.sh standby
   # BOOTSTRAP_CA defaults to true and can create a new local CA
   ```

2. On peer clusters, issue leaves only after the shared intermediate exists:

   ```bash
   BOOTSTRAP_CA=false ./docs/sidb/script/setup-sidb-tcps-cert-manager.sh standby
   ```

3. Keep namespace and CA/issuer/secret names aligned with your SIDB manifests on every cluster.

4. `setup-sidb-tcps-cert-manager.sh` does not run the fullchain rewrite in `10-copy-primary-ca-to-standby.sh`. Run that helper later if peers need leaf+intermediate+root in `tls.crt`.

5. Operator CRD/RBAC on every cluster must match the manifests (for example `spec.primarySource.tcpsPort` on SIDB v4).

---

## Prerequisites

- cert-manager on every participating cluster
- Oracle Database Operator watching the target namespace
- Peer-reachable DNS names or IP addresses for certificate SANs
- A secure way to copy a Kubernetes Secret manifest between clusters

---

## Environment

Export names that match your SIDB manifests. Example:

```bash
export NS=default

# SANs must include how peers connect to this database endpoint
export PRIMARY_DNS_NAMES='db1.example.com,10.0.2.240,sidb-primary,sidb-primary.default.svc.cluster.local'
export STANDBY_DNS_NAMES='db2.example.com,10.2.1.131,sidb-standby,sidb-standby.default.svc.cluster.local'

export PRIMARY_CERT_NAME=sidb-primary-tcps
export PRIMARY_SECRET_NAME=sidb-primary-tcps-tls
export STANDBY_CERT_NAME=sidb-standby-tcps
export STANDBY_SECRET_NAME=sidb-standby-tcps-tls

export ROOT_CERT_NAME=sidb-tcps-root-ca
export ROOT_SECRET_NAME=sidb-tcps-root-ca-secret
export INT_CERT_NAME=sidb-tcps-intermediate-ca
export INT_SECRET_NAME=sidb-tcps-intermediate-ca-secret
export INTERMEDIATE_ISSUER_NAME=sidb-tcps-intermediate-issuer
```

Put the local peer-reachable name/IP first in each `*_DNS_NAMES` list; that value becomes the certificate CN. Every entry is added as a SAN.

For multiple standby members that share one standby TLS secret name on a cluster, include every peer-reachable host/IP for those members in `STANDBY_DNS_NAMES`.

---

## Step 1 — Primary cluster: create CA and primary leaf

```bash
# kubeconfig points at the primary cluster only
cd /path/to/oracle-database-operator
./docs/sidb/script/setup-sidb-tcps-cert-manager.sh primary
```

Creates:

- root CA
- intermediate CA (`sidb-tcps-intermediate-ca-secret`)
- intermediate Issuer
- primary leaf (`sidb-primary-tcps-tls`)

If the primary cluster also hosts a standby member that uses `sidb-standby-tcps-tls`, issue that leaf on the primary cluster too:

```bash
BOOTSTRAP_CA=false ./docs/sidb/script/setup-sidb-tcps-cert-manager.sh standby
```

---

## Step 2 — Export the intermediate CA

```bash
mkdir -p /tmp/tcps-ca-transfer
cd /tmp/tcps-ca-transfer

kubectl -n "$NS" get secret "$INT_SECRET_NAME" -o json \
| jq 'del(
      .metadata.uid,
      .metadata.resourceVersion,
      .metadata.creationTimestamp,
      .metadata.managedFields,
      .metadata.ownerReferences,
      .metadata.annotations
    )
    | .metadata |= {name, namespace, labels}' \
> intermediate-ca-secret.json

jq '{name: .metadata.name, ns: .metadata.namespace, keys: (.data|keys)}' intermediate-ca-secret.json
```

Expected keys: `ca.crt`, `tls.crt`, `tls.key`.

Copy `intermediate-ca-secret.json` to each peer cluster’s admin host by your approved secure channel.

Optional issuer manifest for peers (file avoids fragile shell heredocs):

```bash
printf '%s\n' \
'apiVersion: cert-manager.io/v1' \
'kind: Issuer' \
'metadata:' \
"  name: ${INTERMEDIATE_ISSUER_NAME}" \
"  namespace: ${NS}" \
'spec:' \
'  ca:' \
"    secretName: ${INT_SECRET_NAME}" \
> standby-intermediate-issuer.yaml
```

---

## Step 3 — Peer cluster: import CA and issue local leaf

On each peer cluster that hosts a Data Guard standby (or other TCPS peer):

```bash
# kubeconfig points at the peer cluster only
kubectl -n "$NS" apply -f intermediate-ca-secret.json
kubectl -n "$NS" apply -f standby-intermediate-issuer.yaml

cd /path/to/oracle-database-operator
BOOTSTRAP_CA=false ./docs/sidb/script/setup-sidb-tcps-cert-manager.sh standby
```

Repeat for every peer cluster. Each cluster needs its own leaf Secret object even when the Secret **name** is the same.

---

## Step 4 — Validate shared CA before Data Guard bring-up

```bash
PRIMARY_KUBECONFIG=...
PEER_KUBECONFIG=...
NS=default

kubectl --kubeconfig="$PRIMARY_KUBECONFIG" -n "$NS" get secret sidb-primary-tcps-tls \
  -o jsonpath='{.data.tls\.crt}' | base64 -d | openssl x509 -noout -subject -issuer -ext subjectAltName,extendedKeyUsage

kubectl --kubeconfig="$PEER_KUBECONFIG" -n "$NS" get secret sidb-standby-tcps-tls \
  -o jsonpath='{.data.tls\.crt}' | base64 -d | openssl x509 -noout -subject -issuer -ext subjectAltName,extendedKeyUsage

echo "Intermediate fingerprints must match:"
kubectl --kubeconfig="$PRIMARY_KUBECONFIG" -n "$NS" get secret sidb-tcps-intermediate-ca-secret \
  -o jsonpath='{.data.tls\.crt}' | base64 -d | openssl x509 -noout -fingerprint -sha256
kubectl --kubeconfig="$PEER_KUBECONFIG" -n "$NS" get secret sidb-tcps-intermediate-ca-secret \
  -o jsonpath='{.data.tls\.crt}' | base64 -d | openssl x509 -noout -fingerprint -sha256
```

Pass criteria:

- Same intermediate issuer on all leaves
- Identical intermediate CA fingerprints on all clusters
- SANs cover peer connect hosts/IPs used by Data Guard / SIDB endpoints
- EKU includes clientAuth and serverAuth

Only after this validation should you create SIDB primary/standby resources and the DataguardBroker configuration.

---

## Step 5 — Use the secrets in Data Guard

Point SIDB members at the leaf secrets:

```yaml
security:
  tcps:
    enabled: true
    tlsSecret: sidb-primary-tcps-tls   # or sidb-standby-tcps-tls on standbys
```

Standby members that connect to a primary over TCPS also need a reachable primary TCPS port in `primarySource` (for example `tcpsPort`) when using IP connect strings.

After databases are Healthy, configure DataguardBroker members with TCPS endpoints and the per-member DG client wallet secrets produced by the operator. Broker and wallet steps are covered in the multi-cluster Data Guard TCPS guides under `docs/sidb/`.

---

## Intermediate CA rotation

1. Rotate/recreate the intermediate CA on the primary cluster
2. Re-export and re-apply the intermediate Secret on every peer cluster
3. Re-issue peer leaves with `BOOTSTRAP_CA=false`
4. Roll database pods if they cached previous TLS material

Copied intermediate Secrets are not synced automatically.

---

## Helper scripts in this folder

| Script / wrapper | Role in this method |
| --- | --- |
| `../script/setup-sidb-tcps-cert-manager.sh primary` | Create CA + primary leaf on primary cluster |
| `../script/setup-sidb-tcps-cert-manager.sh standby` with `BOOTSTRAP_CA=false` | Issue peer leaf after shared intermediate exists |
| `06-copy-intermediate-to-standby.sh` | Optional automation of Step 2→3 when both contexts are available |
| `10-copy-primary-ca-to-standby.sh` | Optional fullchain / trust-bundle normalize |
| `run-all.sh` | Optional automation of the same PKI steps when both contexts are available |

---

## See also

- [`README.md`](./README.md)
- [`../dataguard/DATAGUARD_MULTI_CLUSTER_TCPS_SEPARATE_CLUSTER.md`](../dataguard/DATAGUARD_MULTI_CLUSTER_TCPS_SEPARATE_CLUSTER.md)
- [`../dataguard/DATAGUARD_MULTI_CLUSTER_TCPS_NODEPORT.md`](../dataguard/DATAGUARD_MULTI_CLUSTER_TCPS_NODEPORT.md)
- [`../prerequisites/PREREQUISITES.md`](../prerequisites/PREREQUISITES.md)
