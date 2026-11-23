# Cross-Cluster Data Guard over TCPS with NLB (Primary + Standby + DataguardBroker)

Use this document for a **two-cluster** (often two-region) topology where:

- Primary and standby each expose an **OCI Network Load Balancer (NLB)**
- Listeners publish **TCP `1521`** (bootstrap) and **TCPS `2484`** (Broker / redo after Healthy)
- Hostnames come from **ExternalDNS** / private DNS. Each region should publish
  its NLB hostname from its own private DNS zone, with cross-region resolution
  enabled in both directions.
- `DataguardBroker` runs in the **standby** cluster and uses TCPS client-wallet Secrets for both members

## Contents

| Step | Task | Link |
| --- | --- | --- |
| 1 | Prepare requirements | [Before You Begin](#before-you-begin) |
| 2 | Set cluster access and values | [Set the example variables](#set-the-example-variables) |
| 3 | Check deployment gates | [Required Before Deployment](#required-before-deployment) |
| 4 | Review the manifests | [Review Manifest Requirements](#review-manifest-requirements) |
| 5 | Deploy the operator | [Deploy the operator](#1-deploy-the-operator-in-both-clusters) |
| 6 | Create namespaces, Secrets, and certificates | [Create namespaces and Secrets](#2-create-namespaces-and-secrets) |
| 7 | Create the primary | [Create the primary SIDB](#3-create-the-primary-sidb) |
| 8 | Copy the TDE wallet | [Export the TDE wallet](#4-export-the-tde-wallet-to-the-standby-cluster) |
| 9 | Create the standby | [Create the standby SIDB](#5-create-the-standby-sidb) |
| 10 | Verify both databases | [Validate before Broker](#6-validate-primary-and-standby-before-broker) |
| 11 | Generate Broker manifest | [Generate DataguardBroker](#7-generate-dataguardbroker) |
| 12 | Apply and verify Broker | [Apply, verify, and run DGMGRL](#8-apply-verify-and-run-dgmgrl) |
| 13 | Optional role change | [Switchover](#9-switchover) |
| 14 | Optional FSFO validation | [FSFO and reinstate validation](#10-fsfo-and-reinstate-validation) |
| — | Troubleshoot | [Troubleshooting](#troubleshooting) |
| — | Example reference | [Example: Ashburn–Mumbai Flannel profile](#example-ashburnmumbai-flannel-profile) |

## Before You Begin

Complete every item below in **both** clusters unless a row says otherwise.

| Requirement | What to prepare | How to confirm |
| --- | --- | --- |
| Matching OraOperator | Same operator image, CRDs, and RBAC on both clusters | **[Deploy and verify the operator](#1-deploy-the-operator-in-both-clusters)** |
| Watched namespace | Operator watches `$NS` (example: `dg`) | **[Create namespaces and Secrets](#2-create-namespaces-and-secrets)**; verify operator logs |
| Storage | StorageClass that can provision SIDB PVCs (example: `oci-bv`) | **[Review the SIDB manifest storage](#review-manifest-requirements)** |
| Networking | Peered VCNs / routes so workers reach the peer NLB subnet on TCP `1521` and TCPS `2484` | **[Test NLB TCP/TCPS reachability](#nlb-traffic-policy-notes)** |
| DNS | Private DNS zone + ExternalDNS in each OKE cluster; cross-region resolution both ways | **[Verify DNS names from both clusters](#prefer-externaldns-hostnames)** |
| Admin password | Same SYS/admin password in both clusters | **[Create the admin Secret](#2-create-namespaces-and-secrets)**; confirm key `oracle_pwd` |
| TDE password | Password for the TDE wallet on the primary | **[Create the primary TDE Secret](#2-create-namespaces-and-secrets)**; confirm key `tde_wallet_pwd` |
| Image pull | OCR or private registry Secret when required | **[Create image-pull Secrets](#2-create-namespaces-and-secrets)** in both `$NS` namespaces |
| Tools | `kubectl`, `jq`, `zip`, `unzip`, Bash 4+; `yq` recommended for YAML Broker output | **[Confirm the required tools](#set-the-example-variables)** |

### Certificate access model

Pick **one** model and stay with it:

| Admin access | Use |
| --- | --- |
| One host can run `kubectl --context` against both clusters | Dual-context certificate helpers under `docs/sidb/tcps-cert-manager/` |
| Separate bastions / no shared dual-context kubectl | Separate-cluster intermediate CA export/import (steps in [Create TCPS certificates](#create-tcps-certificates)) |

Do not create an independent CA on the standby cluster. Reuse the primary intermediate CA.

## Set the example variables

These are the values you provide before starting. The shell variables do not
substitute values into YAML automatically; use them while editing the samples.
Do not set NLB addresses separately—the DNS names below are the intended stable
endpoints.

```sh
export PRIMARY_CTX=<primary-kube-context>
export STANDBY_CTX=<standby-kube-context>

export NS=dg
export PRIMARY_DB=<primary-sidb-name>
export STANDBY_DB=<standby-sidb-name>
export PRIMARY_SID=<primary-db-unique-name>
export STANDBY_SID=<standby-db-unique-name>
export DGB=${STANDBY_DB}-dg

export PRIMARY_DNS=<primary-private-dns-name>
export STANDBY_DNS=<standby-private-dns-name>
export PRIMARY_SUBNET_OCID=<primary-oke-svclb-subnet-ocid>
export STANDBY_SUBNET_OCID=<standby-oke-svclb-subnet-ocid>

export ADMIN_SECRET=db-admin-secret
export TDE_SECRET=tde-wallet-secret
export IMAGE_PULL_SECRET=ocr-reg-cred
export PRIMARY_TLS_SECRET=${PRIMARY_DB}-tcps-tls
export STANDBY_TLS_SECRET=${STANDBY_DB}-tcps-tls
export PRIMARY_WALLET_SECRET=${PRIMARY_DB}-dg-client-wallet
export STANDBY_WALLET_SECRET=${STANDBY_DB}-dg-client-wallet

export DB_IMAGE=container-registry.oracle.com/database/enterprise_ru:latest
export STORAGE_CLASS=oci-bv
export STORAGE_SIZE=100Gi
```

Confirm that the contexts target different clusters before applying anything:

```sh
kubectl --context "$PRIMARY_CTX" config current-context
kubectl --context "$STANDBY_CTX" config current-context
kubectl --context "$PRIMARY_CTX" get nodes
kubectl --context "$STANDBY_CTX" get nodes
```

## Required Before Deployment

> **Important:** Complete every action in the **Complete before applying** column before applying the related manifest.

| Requirement | Example value | Complete before applying |
| --- | --- | --- |
| Operator in both clusters | Same image/CRD/RBAC | [1. Deploy the operator in both clusters](#1-deploy-the-operator-in-both-clusters) |
| Namespace + admin/TDE/pull Secrets | `$NS`, `$ADMIN_SECRET`, `$TDE_SECRET` | [2. Create namespaces and Secrets](#2-create-namespaces-and-secrets) |
| Shared-CA TCPS leaf Secrets | `$PRIMARY_TLS_SECRET`, `$STANDBY_TLS_SECRET` | [Create TCPS certificates](#create-tcps-certificates) |
| Bidirectional DNS / NLB reachability | `$PRIMARY_DNS`, `$STANDBY_DNS` on `1521` and `2484` | Confirm before SIDB create |
| Primary Healthy + TDE wallet zip in standby | `standby-wallet.zip` in `$TDE_SECRET` | [4. Export the TDE wallet to the standby cluster](#4-export-the-tde-wallet-to-the-standby-cluster) |
| Primary DG client wallet in Broker namespace | `$PRIMARY_WALLET_SECRET` on standby cluster | [Copy the primary Data Guard client wallet](#copy-the-primary-data-guard-client-wallet) |

## Review Manifest Requirements

> Start from the linked sample, copy it, then replace placeholders with the values from [Set the example variables](#set-the-example-variables). The create steps below show the same fields again in context.

Primary SIDB — start from [`singleinstancedatabase_primary_cross_cluster_lb.yaml`](../../../config/samples/sidb/singleinstancedatabase_primary_cross_cluster_lb.yaml), then add TCPS / ExternalDNS as in [3. Create the primary SIDB](#3-create-the-primary-sidb).

| Setting | Example value | Required relationship |
| --- | --- | --- |
| `createAs` | `primary` | Primary role |
| `security.tcps` | `enabled: true`, `tlsSecret: $PRIMARY_TLS_SECRET` | Shared-CA leaf Secret from step 3 |
| External Service | NLB + ExternalDNS `$PRIMARY_DNS`, TCP `1521` + TCPS `2484` | Publishes bootstrap TCP and post-Ready TCPS |
| `dataguard.prereqs.enabled` | `true` | Data Guard prerequisites |
| `dataguard.standbySources[].host` | `$STANDBY_DNS` | Peer standby NLB hostname (update after standby exists if needed) |

Standby SIDB — start from [`singleinstancedatabase_standby_cross_cluster_lb.yaml`](../../../config/samples/sidb/singleinstancedatabase_standby_cross_cluster_lb.yaml), then add TCPS fields as in [5. Create the standby SIDB](#5-create-the-standby-sidb).

| Setting | Example value | Required relationship |
| --- | --- | --- |
| `createAs` | `standby` | Physical standby role |
| `primarySource.connectString` | `$PRIMARY_DNS:1521/$PRIMARY_SID` | **TCP** bootstrap from the standby Pod to the primary NLB |
| `primarySource.tcpsPort` | `2484` | Post-Ready TCPS peer port |
| `security.secrets.tde.walletZipFileKey` | `standby-wallet.zip` | Current primary wallet zip in `$TDE_SECRET` |
| `security.tcps` | `enabled: true`, `tlsSecret: $STANDBY_TLS_SECRET` | Standby leaf Secret from step 3 |
| External Service | NLB + ExternalDNS `$STANDBY_DNS`, TCP `1521` + TCPS `2484` | Cross-cluster reachability |

DataguardBroker — generate with the renderer ([7. Generate DataguardBroker](#7-generate-dataguardbroker)).

| Setting | Example value | Required relationship |
| --- | --- | --- |
| Member hosts | `$PRIMARY_DNS`, `$STANDBY_DNS` | Current ExternalDNS / NLB hostnames |
| TCPS endpoints | `:2484` on both members | Broker / redo after Healthy |
| `tcps.clientWalletSecret` | `$PRIMARY_WALLET_SECRET`, `$STANDBY_WALLET_SECRET` | Both Secrets must exist in the Broker namespace |
| `localRef` | Local standby SIDB | Apply only after both SIDBs are Healthy |

### Two-step model and TCP bootstrap

| Step | What it does | What it does **not** do |
| --- | --- | --- |
| 1. Create standby SIDB with `dataguard.prereqs.enabled: true` | DBCA Duplicate, TDE, PHYSICAL STANDBY, `DATABASE IS READY TO USE!` | Configure complete Broker topology |
| 2. Apply `DataguardBroker` with TCPS wallets | Broker connect IDs, MaxPerformance, TCPS redo/Broker traffic | Re-run Duplicate |

Current behavior (Mode A):

- Standby first contact uses primary **TCP** `$PRIMARY_DNS:1521/$PRIMARY_SID`
- Set `primarySource.tcpsPort: 2484` for post-Ready TCPS aliases
- After Broker is Healthy, primary↔standby Data Guard traffic uses **TCPS `:2484`**
- TCPS-only bootstrap (no TCP `1521`) is not supported yet

## Prefer ExternalDNS hostnames

Annotate each SIDB external Service so ExternalDNS publishes a stable name:

```yaml
annotations:
  external-dns.alpha.kubernetes.io/hostname: "<primary-private-dns-name>"
  oci.oraclecloud.com/load-balancer-type: "nlb"
  oci-network-load-balancer.oraclecloud.com/internal: "true"
  oci-network-load-balancer.oraclecloud.com/subnet: "<subnet-ocid>"
```

Use those hostnames in `primarySource.connectString`, `dataguard.standbySources[].host`, and Broker members. Keep NLB IPs as a troubleshooting fallback only.

### NLB traffic policy notes

| Pattern | Typical `externalTrafficPolicy` | `is-preserve-source` |
| --- | --- | --- |
| VCN-native / peered (common default) | `Local` | `"false"` |
| Overlay CNI (for example Flannel) when remote cannot hit Local VIP | `Cluster` | **omit** (OCI rejects Cluster + preserve-source) |

Validate from the primary Pod after the standby NLB exists and before Duplicate:

```sh
kubectl --context "$PRIMARY_CTX" -n "$NS" exec -it deploy/"$PRIMARY_DB" -- \
  bash -c "timeout 5 bash -c 'echo >/dev/tcp/${STANDBY_DNS}/1521' && echo OK_1521"
kubectl --context "$PRIMARY_CTX" -n "$NS" exec -it deploy/"$PRIMARY_DB" -- \
  bash -c "timeout 5 bash -c 'echo >/dev/tcp/${STANDBY_DNS}/2484' && echo OK_2484"
```

Adjust Pod/container names if your resource uses a different label or container name.

## 1. Deploy the operator in both clusters

Use the same `oracle-database-operator.yaml` file. Apply it first to the
primary cluster and then to the standby cluster; do not apply both commands to
the same cluster.

```sh
kubectl --context "$PRIMARY_CTX" apply -f oracle-database-operator.yaml
kubectl --context "$PRIMARY_CTX" -n oracle-database-operator-system get deploy,pods

kubectl --context "$STANDBY_CTX" apply -f oracle-database-operator.yaml
kubectl --context "$STANDBY_CTX" -n oracle-database-operator-system get deploy,pods
```

Continue only when the operator is Ready in both clusters.

Confirm SIDB v4 includes `primarySource.tcpsPort` when you use that field:

```sh
kubectl --context "$PRIMARY_CTX" get crd singleinstancedatabases.database.oracle.com -o json \
  | jq '.spec.versions[] | select(.name=="v4") | .schema.openAPIV3Schema.properties.spec.properties.primarySource.properties | has("tcpsPort")'
```

If the result is `false`, apply the current CRD from this repository on that cluster.

## 2. Create namespaces and Secrets

Create the namespace in both clusters, then create the same admin Secret in
both. Create the TDE password Secret only in the primary at this stage.

```sh
kubectl --context "$PRIMARY_CTX" create namespace "$NS" --dry-run=client -o yaml | kubectl --context "$PRIMARY_CTX" apply -f -
kubectl --context "$STANDBY_CTX" create namespace "$NS" --dry-run=client -o yaml | kubectl --context "$STANDBY_CTX" apply -f -

# Same admin password in both clusters
kubectl --context "$PRIMARY_CTX" -n "$NS" create secret generic "$ADMIN_SECRET" \
  --from-literal=oracle_pwd='<admin-password>' \
  --dry-run=client -o yaml | kubectl --context "$PRIMARY_CTX" apply -f -
kubectl --context "$STANDBY_CTX" -n "$NS" create secret generic "$ADMIN_SECRET" \
  --from-literal=oracle_pwd='<admin-password>' \
  --dry-run=client -o yaml | kubectl --context "$STANDBY_CTX" apply -f -

# Primary TDE password Secret (wallet zip is added after primary is Healthy)
kubectl --context "$PRIMARY_CTX" -n "$NS" create secret generic "$TDE_SECRET" \
  --from-literal=tde_wallet_pwd='<tde-wallet-password>' \
  --dry-run=client -o yaml | kubectl --context "$PRIMARY_CTX" apply -f -

# Image-pull Secret when required (repeat on standby)
# kubectl --context "$PRIMARY_CTX" -n "$NS" create secret docker-registry "$IMAGE_PULL_SECRET" ...
# kubectl --context "$STANDBY_CTX" -n "$NS" create secret docker-registry "$IMAGE_PULL_SECRET" ...
```

### Create TCPS certificates

Goal: one shared intermediate CA; a primary leaf Secret on the primary cluster; a standby leaf Secret on the standby cluster. Each leaf Secret must contain `tls.crt`, `tls.key`, and `ca.crt`.

### Separate bastions (typical cross-region)

1. On the **primary** cluster, create the root + intermediate CA and the primary leaf Secret `$PRIMARY_TLS_SECRET` for SANs that include `$PRIMARY_DNS` (and any required service DNS names).
2. Export the intermediate CA Secret from the primary cluster to a file.
3. On the **standby** cluster, import that intermediate CA Secret and Issuer. Do **not** create a new root.
4. Issue the standby leaf Secret `$STANDBY_TLS_SECRET` with SANs that include `$STANDBY_DNS`.
5. Compare intermediate CA certificate fingerprints on both clusters; they must match.

Automation for this handoff lives under `docs/sidb/tcps-cert-manager/SEPARATE_CLUSTER_ACCESS.md`. Dual-context helpers live under `docs/sidb/tcps-cert-manager/README.md`. Use those scripts if you want automated `cert-manager` issuance; the operational requirement is the shared intermediate CA and the two leaf Secrets named above.

## 3. Create the primary SIDB

1. Open and copy [`singleinstancedatabase_primary_cross_cluster_lb.yaml`](../../../config/samples/sidb/singleinstancedatabase_primary_cross_cluster_lb.yaml).
2. Keep the TCP NLB base, then add the TCPS / ExternalDNS fields below. Replace placeholders with your values from [Set the example variables](#set-the-example-variables). Same-cluster TDE+TCPS field patterns are also in [`singleinstancedatabase_dataguard_primary_tde_tcps.yaml`](../../../config/samples/sidb/singleinstancedatabase_dataguard_primary_tde_tcps.yaml).

| Field | Set to | Why it matters |
| --- | --- | --- |
| `createAs` | `primary` | Primary role |
| `flashBack` | `true` | Needed for FSFO / role-change paths that use flashback |
| `security.secrets.admin` / `tde` | `$ADMIN_SECRET`, `$TDE_SECRET` with `tde_wallet_pwd` | Primary admin + TDE password Secrets |
| `security.tcps` | `enabled: true`, `tlsSecret: $PRIMARY_TLS_SECRET` | Primary leaf TLS Secret from step 3 |
| `dataguard.prereqs.enabled` | `true` | Enables Data Guard prerequisites |
| `dataguard.standbySources[]` | `$STANDBY_SID` / `$STANDBY_DNS`, TCP `1521`, TCPS `2484` | Peer standby identity and ports |
| External Service | NLB + ExternalDNS `$PRIMARY_DNS`, TCP `1521` + TCPS `2484` | Bootstrap TCP and post-Ready TCPS on the same NLB |

Example of the critical fields after editing:

```yaml
createAs: primary
flashBack: true
security:
  secrets:
    admin:
      secretName: db-admin-secret
      secretKey: oracle_pwd
      keepSecret: true
    tde:
      secretName: tde-wallet-secret
      secretKey: tde_wallet_pwd
  tcps:
    enabled: true
    tlsSecret: <primary-sidb-name>-tcps-tls
dataguard:
  prereqs:
    enabled: true
  standbySources:
    - dbUniqueName: <standby-db-unique-name>
      host: <standby-private-dns-name>
      tcpPort: 1521
      tcpsEnabled: true
      tcpsPort: 2484
services:
  external:
    type: LoadBalancer
    publishNotReadyAddresses: true
    externalTrafficPolicy: Local
    annotations:
      external-dns.alpha.kubernetes.io/hostname: "<primary-private-dns-name>"
      oci.oraclecloud.com/load-balancer-type: "nlb"
      oci-network-load-balancer.oraclecloud.com/is-preserve-source: "false"
      oci-network-load-balancer.oraclecloud.com/internal: "true"
      oci-network-load-balancer.oraclecloud.com/subnet: "<primary-subnet-ocid>"
    tcp:
      enabled: true
      port: 1521
    # Expose TCPS 2484 on the same NLB (field names must match your SIDB API sample)
```

3. Save as `primary-sidb-tcps-nlb.yaml` and apply against the **primary** cluster:

```sh
kubectl --context "$PRIMARY_CTX" apply -f primary-sidb-tcps-nlb.yaml
kubectl --context "$PRIMARY_CTX" wait --for=jsonpath='{.status.status}'=Healthy \
  "singleinstancedatabase/$PRIMARY_DB" -n "$NS" --timeout=60m
kubectl --context "$PRIMARY_CTX" get singleinstancedatabase "$PRIMARY_DB" -n "$NS" -o wide
kubectl --context "$PRIMARY_CTX" get svc -n "$NS" -l "app=$PRIMARY_DB" -o wide
```

Expect role `PRIMARY`, status `Healthy`, and DNS/NLB for `$PRIMARY_DNS`.

## 4. Export the TDE wallet to the standby cluster

```sh
PRIMARY_POD=$(kubectl --context "$PRIMARY_CTX" get pod -n "$NS" -l "app=$PRIMARY_DB" \
  -o jsonpath='{.items[0].metadata.name}')

rm -rf /tmp/standby-wallet/tde
mkdir -p /tmp/standby-wallet/tde
kubectl --context "$PRIMARY_CTX" -n "$NS" cp \
  "${PRIMARY_POD}:/opt/oracle/oradata/${PRIMARY_SID}/tdewallet/tde/ewallet.p12" \
  /tmp/standby-wallet/tde/ewallet.p12 \
  -c "$PRIMARY_DB"

# Zip must run from the parent of `tde/` so the archive root is `tde/ewallet.p12`.
cd /tmp/standby-wallet
zip -qr /tmp/standby-wallet.zip tde
unzip -t /tmp/standby-wallet.zip
cd -

kubectl --context "$STANDBY_CTX" -n "$NS" create secret generic "$TDE_SECRET" \
  --from-file=standby-wallet.zip=/tmp/standby-wallet.zip \
  --dry-run=client -o yaml | kubectl --context "$STANDBY_CTX" apply -f -
```

For a **19c standby**, use the wallet ZIP only: omit `tde_wallet_pwd` and
`security.secrets.tde.secretKey`. For **26ai only**, add the password key to
this Secret and configure `secretKey: tde_wallet_pwd` in the standby manifest.
Use the wallet ZIP from the **current** primary. A stale ZIP commonly fails
after Duplicate networking succeeds.

## 5. Create the standby SIDB

1. Open and copy [`singleinstancedatabase_standby_cross_cluster_lb.yaml`](../../../config/samples/sidb/singleinstancedatabase_standby_cross_cluster_lb.yaml).
2. Keep the TCP NLB base, then add the TCPS fields below. Replace placeholders with your values from [Set the example variables](#set-the-example-variables). Use the same image tag as the primary.

| Field | Set to | Why it matters |
| --- | --- | --- |
| `createAs` | `standby` | Creates a physical standby |
| `primarySource.connectString` | `$PRIMARY_DNS:1521/$PRIMARY_SID` | **TCP** bootstrap to the primary NLB (required; TCPS-only bootstrap is not supported) |
| `primarySource.tcpsPort` | `2484` | Post-Ready TCPS peer port for aliases and Broker |
| `security.secrets.admin` | `$ADMIN_SECRET` / `oracle_pwd` | Same admin password as the primary |
| `security.secrets.tde` | `$TDE_SECRET` + `walletZipFileKey: standby-wallet.zip` | Current primary wallet zip from step 5. **19c:** omit `secretKey` (wallet zip only). **26ai only:** add `secretKey: tde_wallet_pwd` and the password key in the Secret |
| `security.tcps` | `enabled: true`, `tlsSecret: $STANDBY_TLS_SECRET` | Standby leaf TLS Secret from step 3 |
| `dataguard.prereqs.enabled` | `true` | Enables Duplicate / FAL prerequisites |
| External Service | NLB + ExternalDNS `$STANDBY_DNS`, TCP `1521` + TCPS `2484` | Cross-cluster reachability on the same NLB |

Example of the critical fields after editing:

```yaml
createAs: standby
primarySource:
  connectString: "<primary-private-dns-name>:1521/<primary-db-unique-name>"   # TCP for bootstrap
  tcpsPort: 2484
security:
  secrets:
    admin:
      secretName: db-admin-secret
      secretKey: oracle_pwd
    tde:
      secretName: tde-wallet-secret
      # 19c: omit secretKey and store only standby-wallet.zip.
      # 26ai only: add secretKey: tde_wallet_pwd and the password key.
      walletZipFileKey: standby-wallet.zip
  tcps:
    enabled: true
    tlsSecret: <standby-sidb-name>-tcps-tls
dataguard:
  prereqs:
    enabled: true
```

3. Save as `standby-sidb-tcps-nlb.yaml` and apply against the **standby** cluster:

```sh
kubectl --context "$STANDBY_CTX" apply -f standby-sidb-tcps-nlb.yaml
kubectl --context "$STANDBY_CTX" wait --for=jsonpath='{.status.status}'=Healthy \
  "singleinstancedatabase/$STANDBY_DB" -n "$NS" --timeout=60m
kubectl --context "$STANDBY_CTX" get singleinstancedatabase "$STANDBY_DB" -n "$NS" -o wide
```

Expect `PHYSICAL_STANDBY` / `Healthy`. Matching image tags on primary and standby are required.

## 6. Validate primary and standby before Broker

```sh
PRIMARY_POD=$(kubectl --context "$PRIMARY_CTX" get pod -n "$NS" -l "app=$PRIMARY_DB" \
  -o jsonpath='{.items[0].metadata.name}')
STANDBY_POD=$(kubectl --context "$STANDBY_CTX" get pod -n "$NS" -l "app=$STANDBY_DB" \
  -o jsonpath='{.items[0].metadata.name}')

kubectl --context "$PRIMARY_CTX" -n "$NS" exec "$PRIMARY_POD" -c "$PRIMARY_DB" -- bash -c 'sqlplus -s / as sysdba <<SQL
set lines 200 pages 50
select name, db_unique_name, database_role, open_mode from v\$database;
SQL'

kubectl --context "$STANDBY_CTX" -n "$NS" exec "$STANDBY_POD" -c "$STANDBY_DB" -- bash -c 'sqlplus -s / as sysdba <<SQL
set lines 200 pages 50
select name, db_unique_name, database_role, open_mode from v\$database;
SQL'
```

Standby Healthy alone is not full Data Guard. Configure Broker next.

### Copy the primary Data Guard client wallet

The Broker runner needs the **primary** SIDB TCPS client-wallet Secret in the standby (Broker) namespace. This Secret is separate from the TDE wallet and from server TLS Secrets.

```sh
kubectl --context "$PRIMARY_CTX" get singleinstancedatabase "$PRIMARY_DB" -n "$NS" \
  -o jsonpath='{.status.clientWalletSecret}{"\n"}'
kubectl --context "$STANDBY_CTX" get singleinstancedatabase "$STANDBY_DB" -n "$NS" \
  -o jsonpath='{.status.clientWalletSecret}{"\n"}'

# Param’s verified handoff (shared pipe). For isolated bastions, write JSON to a file and apply on standby.
kubectl --context "$PRIMARY_CTX" -n "$NS" get secret "$PRIMARY_WALLET_SECRET" -o json \
  | jq '{apiVersion,kind,metadata:{name:.metadata.name,namespace:"'"$NS"'"},type,data}' \
  | kubectl --context "$STANDBY_CTX" apply -f -

kubectl --context "$STANDBY_CTX" -n "$NS" get secret "$PRIMARY_WALLET_SECRET" "$STANDBY_WALLET_SECRET"
```

Do not use the portable client-wallet **zip** procedure for this Broker handoff. Broker needs the operator-published Secret objects in its namespace.

## 7. Generate DataguardBroker

From the repository root, on a host that can reach the standby cluster (and primary status when using multi-kubeconfig rendering):

```sh
cp config/samples/sidb/render-dg-broker-from-status.sh .
cp config/samples/sidb/gen_dg.sh .
chmod +x render-dg-broker-from-status.sh gen_dg.sh

./render-dg-broker-from-status.sh sidb "$STANDBY_DB" "$NS" "$DGB" > dataguardbroker.yaml
```

Prefer YAML. The script emits YAML when `yq` is installed; otherwise convert JSON:

```sh
command -v yq >/dev/null && yq -P . dataguardbroker.yaml > /tmp/dg.yaml \
  && mv /tmp/dg.yaml dataguardbroker.yaml
```

Review before apply:

- Member hosts use `$PRIMARY_DNS` and `$STANDBY_DNS` (current values for this deployment)
- Each member exposes Broker-reachable TCPS `:2484` endpoints
- The renderer may emit `replace-with-shared-client-wallet-secret` when the
  source status does not publish a wallet Secret. This is a placeholder, not a
  usable Secret name; do not apply the file while it is present.
- Set `tcps.clientWalletSecret` to a real Secret that exists in `$NS` on the
  standby cluster. For multiple members, set the member-specific Secret when
  the wallets are different.
- Keep `localRef` on the local standby member
- Replace image and admin Secret placeholders

Run this complete pre-apply check against the generated file:

```sh
BROKER_FILE=dataguardbroker.yaml
WALLET_SECRET=<wallet-secret-in-standby-namespace>

yq -e '.' "$BROKER_FILE" >/dev/null
echo "YAML syntax: OK"

if grep -qE 'replace-with|REPLACE_WITH|<[^>]+>' "$BROKER_FILE"; then
  echo 'ERROR: replace the generated TCPS wallet placeholder before applying' >&2
  exit 1
fi

kubectl --context "$STANDBY_CTX" -n "$NS" get secret "$WALLET_SECRET"
kubectl --context "$STANDBY_CTX" apply --dry-run=server -f "$BROKER_FILE"
```

The final command must report `created (server dry run)` or `configured
(server dry run)`. It validates the Broker schema, but it does not prove that
the wallet can authenticate or that TCPS connectivity works; perform the
DGMGRL validation in [8. Validate with DGMGRL](#validate-with-dgmgrl)
after applying the Broker.

If the generated file contains the placeholder, edit
`spec.topology.defaults.tcps.clientWalletSecret` and any member-level
`tcps.clientWalletSecret` values before applying. The Broker topology cannot be
changed after reconciliation starts; delete and recreate the Broker if an
invalid topology has already been applied.

Example wallet placement:

```yaml
members:
  - name: <primary-sidb-name>
    role: PRIMARY
    tcps:
      enabled: true
      clientWalletSecret: <primary-sidb-name>-dg-client-wallet
  - name: <standby-sidb-name>
    role: PHYSICAL_STANDBY
    tcps:
      enabled: true
      clientWalletSecret: <standby-sidb-name>-dg-client-wallet
```

## 8. Apply, verify, and run DGMGRL

Apply the generated manifest in the **standby** cluster:

```sh
kubectl --context "$STANDBY_CTX" apply -f dataguardbroker.yaml
kubectl --context "$STANDBY_CTX" get dataguardbroker "$DGB" -n "$NS" -o wide
kubectl --context "$STANDBY_CTX" describe dataguardbroker "$DGB" -n "$NS"
```

Wait until Broker is `Healthy` and all members are `Configured`.

### Validate with DGMGRL

```sh
RUNNER=$(kubectl --context "$STANDBY_CTX" get pods -n "$NS" \
  -o jsonpath='{range .items[*]}{.metadata.name}{"\n"}{end}' \
  | grep "^${DGB}-runner-" | head -n 1)

kubectl --context "$STANDBY_CTX" exec -it "$RUNNER" -n "$NS" -- dgmgrl /@<primary-tcps-alias>
```

Inside DGMGRL, run `SHOW CONFIGURATION`, `SHOW DATABASE VERBOSE`, and
`VALIDATE DATABASE VERBOSE` for both members. Confirm the configuration is
`SUCCESS`, all members are `SUCCESS`, and redo/Broker sessions use TCPS `:2484`
between the NLB addresses. Perform switchover and switchback only after these
checks pass and Broker is `Healthy`.

## 9. Switchover

Switchover is optional. Run it only after step 8 reports `SUCCESS` and all
members are healthy. Use DGMGRL from the Broker runner and confirm the target
standby is `PHYSICAL STANDBY` with apply enabled before switching back.

```text
SWITCHOVER TO <standby-db-unique-name>;
SHOW CONFIGURATION;
```

Switch back after validation:

```text
SWITCHOVER TO <primary-db-unique-name>;
SHOW CONFIGURATION;
```

## 10. FSFO and reinstate validation

FSFO is optional and disabled by default. Enable it only after successful
manual switchover and switchback validation. Configure an observer according
to the deployment’s availability design, then validate with:

```text
ENABLE FAST_START FAILOVER;
SHOW FAST_START FAILOVER;
VALIDATE DATABASE VERBOSE <standby-db-unique-name>;
```

If a failover leaves the former primary unavailable, reinstate it only after
the new primary is healthy and redo transport is working:

```text
REINSTATE DATABASE <former-primary-db-unique-name>;
SHOW CONFIGURATION;
```

## Troubleshooting

1. **Standby bootstrap still requires primary TCP `:1521`.** TCPS-only first contact is not available yet.
2. **Broker fails if the primary client-wallet Secret is missing in the Broker namespace.** Copy `$PRIMARY_WALLET_SECRET` before apply.
3. **Operator image must include the TCPS Broker fixes.** If wallets and endpoints are correct but Broker reports `dataguard broker configuration is missing after broker query returned no members`, upgrade the operator on both clusters to a build that includes the TCPS Broker changes.
4. **Generated Broker output may be JSON** when `yq` is absent. Convert to YAML before comparing with samples.
5. **Intermediate CA on standby is not auto-synced.** After primary intermediate CA rotation, re-export to every peer cluster and renew standby leaves.
6. **Stale TDE wallet zip** on the standby Secret causes post-Duplicate TDE failures. Re-export from the current primary.
7. **`FAL: Error fetching gap sequence, no FAL server specified` during bootstrap.**

   A brief standby alert-log message during Duplicate / recovery is expected.
   After the standby is Ready / Healthy, `fal_server` must be set from
   `primarySource.connectString` when `dataguard.prereqs.enabled` is `true`.
   If the FAL warning persists, verify the primary TCP endpoint
   (`$PRIMARY_DNS:1521/$PRIMARY_SID`), NLB reachability, then re-render and
   apply DataguardBroker. Full checklist:
   [`DATAGUARD_CROSS_CLUSTER_LB.md` troubleshooting A](./DATAGUARD_CROSS_CLUSTER_LB.md#a-fal-error-fetching-gap-sequence-no-fal-server-specified)
   and [SIDB Known Issues](../README.md#known-issues).

## Cleanup

Delete Broker before deleting SIDBs when you tear down the lab:

```sh
kubectl --context "$STANDBY_CTX" delete dataguardbroker "$DGB" -n "$NS" --wait=false
kubectl --context "$STANDBY_CTX" delete singleinstancedatabase "$STANDBY_DB" -n "$NS"
kubectl --context "$PRIMARY_CTX" delete singleinstancedatabase "$PRIMARY_DB" -n "$NS"
```

Retain or delete PVCs and Secrets according to your retention policy.

## Example: Ashburn–Mumbai Flannel profile

Lab reference only. The files below are sanitized structural samples, not the
exact private deployment. Replace every placeholder before applying them.
Do not combine these samples with stale lab IPs, DNS names, OCIDs, or Secret
names. If you need to reproduce the tested four-member profile, copy the
manifests from that deployment and update all endpoint and wallet values
together before applying the Broker.

- [Primary SIDB with TCP and TCPS NLB](../../../config/samples/sidb/singleinstancedatabase_primary_cross_cluster_lb.yaml)
- [Standby SIDB with TCP and TCPS NLB](../../../config/samples/sidb/singleinstancedatabase_standby_cross_cluster_lb.yaml)
- [DataguardBroker manifest](../../../config/samples/sidb/dataguardbroker_cross_cluster_lb.yaml)
- [Broker renderer](../../../config/samples/sidb/render-cross-cluster-dataguardbroker-lb.sh)
- [Four-member primary TCPS NLB sample](../../../config/samples/sidb/singleinstancedatabase_tcps_nlb_four_member_primary.yaml)
- [Four-member standby1 TCPS NLB sample](../../../config/samples/sidb/singleinstancedatabase_tcps_nlb_four_member_standby1.yaml)
- [Four-member standby2 TCPS NLB sample](../../../config/samples/sidb/singleinstancedatabase_tcps_nlb_four_member_standby2.yaml)
- [Four-member standby3 TCPS NLB sample](../../../config/samples/sidb/singleinstancedatabase_tcps_nlb_four_member_standby3.yaml)
- [Four-member TCPS Broker sample](../../../config/samples/sidb/dataguardbroker_tcps_nlb_four_member.yaml)

The primary and standby samples show the required `security.tcps`, TDE wallet,
NLB, DNS, subnet, and Data Guard fields together. Generate the Broker manifest
from live SIDB status after all databases are Healthy. The renderer output,
not a hand-copied example, is authoritative for live NLB endpoints and member
references.

## Related files

Samples used by this guide:

- Primary: [`singleinstancedatabase_tcps_nlb_four_member_primary.yaml`](../../../config/samples/sidb/singleinstancedatabase_tcps_nlb_four_member_primary.yaml)
- Same-cluster standby: [`singleinstancedatabase_tcps_nlb_four_member_standby1.yaml`](../../../config/samples/sidb/singleinstancedatabase_tcps_nlb_four_member_standby1.yaml)
- Cross-cluster standby 1: [`singleinstancedatabase_tcps_nlb_four_member_standby2.yaml`](../../../config/samples/sidb/singleinstancedatabase_tcps_nlb_four_member_standby2.yaml)
- Cross-cluster standby 2: [`singleinstancedatabase_tcps_nlb_four_member_standby3.yaml`](../../../config/samples/sidb/singleinstancedatabase_tcps_nlb_four_member_standby3.yaml)
- Broker: [`dataguardbroker_tcps_nlb_four_member.yaml`](../../../config/samples/sidb/dataguardbroker_tcps_nlb_four_member.yaml)
- Broker renderer: [`render-cross-cluster-dataguardbroker-lb.sh`](../../../config/samples/sidb/render-cross-cluster-dataguardbroker-lb.sh)
- Certificate workflow: [`docs/sidb/tcps-cert-manager/README.md`](../tcps-cert-manager/README.md)
