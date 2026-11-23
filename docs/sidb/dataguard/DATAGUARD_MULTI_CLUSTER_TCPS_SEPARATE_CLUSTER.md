# Multi-cluster Data Guard over TCPS (separate cluster access)

End-to-end guide for SIDB Data Guard when primary and standby members run on different Kubernetes clusters, TCPS is required, and clusters are administered independently (no dual-context kubectl from one host).

Certificate runbook for this access model:

[`tcps-cert-manager/SEPARATE_CLUSTER_ACCESS.md`](../tcps-cert-manager/SEPARATE_CLUSTER_ACCESS.md)

If one admin client can reach both clusters, you may instead use `tcps-cert-manager/run-all.sh` from [`tcps-cert-manager/README.md`](../tcps-cert-manager/README.md). Both certificate paths share the same CA design.

---

## Scope

This guide covers:

1. Matching operator/CRD/RBAC on every cluster
2. Shared-CA TCPS certificate generation for primary and standby members
3. Primary SIDB create
4. TDE wallet distribution to standby clusters
5. Standby SIDB create on local and remote clusters
6. TCPS connectivity checks between members
7. DataguardBroker configuration using TCPS endpoints

---

## Example topology

| Member | Cluster | SID | TCP | TCPS | Peer address example |
| --- | --- | --- | --- | --- | --- |
| sidb-primary | cluster-a | ORCLPRD | 32331 | 32484 | 10.0.2.240 |
| sidb-standby1 | cluster-a | ORCLSTBY1 | 30281 | 30284 | 10.0.2.240 |
| sidb-standby2 | cluster-b | ORCLSTBY2 | 30282 | 30285 | 10.2.1.131 |
| sidb-standby3 | cluster-b | ORCLSTBY3 | 30283 | 30286 | 10.2.1.131 |

TLS Secrets in namespace `default`:

- Primary leaf: `sidb-primary-tcps-tls`
- Standby leaf name used by standby members: `sidb-standby-tcps-tls` (issued on each cluster that hosts standbys)

---

## 1. Deploy a matching operator on every cluster

Use the same operator image and CRD/RBAC revision on all clusters.

```bash
kubectl apply -f oracle-database-operator.yaml
```

Verify SIDB v4 includes `primarySource.tcpsPort` when your manifests use it:

```bash
kubectl get crd singleinstancedatabases.database.oracle.com -o json \
| jq '.spec.versions[] | select(.name=="v4") | .schema.openAPIV3Schema.properties.spec.properties.primarySource.properties | has("tcpsPort")'
```

If `false`, apply the current CRD from source:

```bash
kubectl apply -f config/crd/bases/database.oracle.com_singleinstancedatabases.yaml
```

If the operator CrashLoops with RBAC `forbidden` errors, refresh the manager ClusterRole from `config/rbac/role.yaml` (deployed name is commonly `oracle-database-operator-manager-role`) and restart the controller.

---

## 2. Create shared-CA TCPS certificates

Follow [`tcps-cert-manager/SEPARATE_CLUSTER_ACCESS.md`](../tcps-cert-manager/SEPARATE_CLUSTER_ACCESS.md) completely:

1. Primary cluster: create root + intermediate CA and primary leaf
2. Export intermediate CA Secret
3. Import intermediate CA + Issuer on every peer cluster
4. Issue standby leaves locally with `BOOTSTRAP_CA=false`
5. Confirm intermediate fingerprints match across clusters

Do not create an independent CA on a standby cluster.

---

## 3. Create the primary database

Ensure `db-admin-secret` and TDE password secret exist, then create the primary SIDB with:

```yaml
security:
  tcps:
    enabled: true
    tlsSecret: sidb-primary-tcps-tls
dataguard:
  prereqs:
    enabled: true
  standbySources:
    - dbUniqueName: ORCLSTBY1
      host: <standby1-peer-address>
      tcpPort: 30281
      tcpsEnabled: true
      tcpsPort: 30284
    # additional standbySources as needed
```

Wait until status is Healthy / PRIMARY and TCPS connect string is populated.

---

## 4. Distribute the TDE wallet

Export the primary TDE wallet, package it, and create `tde-wallet-secret` with `standby-wallet.zip` on every cluster that will host a standby.

Also ensure `db-admin-secret` on peer clusters uses the same SYS/admin password as primary.

---

## 5. Create standby databases

### Same cluster as primary

Issue/confirm local `sidb-standby-tcps-tls`, then create the standby SIDB with:

```yaml
createAs: standby
primarySource:
  connectString: "<primary-peer-address>:<tcpPort>/<SID>"
  tcpsPort: <primary-tcps-port>
security:
  tcps:
    enabled: true
    tlsSecret: sidb-standby-tcps-tls
```

### Remote cluster

Repeat on the remote cluster after shared CA import and local standby leaf issuance.

Validate member health:

```bash
kubectl get singleinstancedatabase \
  -o custom-columns='NAME:.metadata.name,STATUS:.status.status,ROLE:.status.role,TCPS:.status.tcpsConnectString,DATAFILES:.status.datafilesCreated'
```

---

## 6. Validate TCPS between members

From a standby pod to primary and from primary to standby, use the auto-generated `*TCPS` aliases (or explicit TCPS connect descriptors) and confirm `v$database` roles.

Example pattern:

```bash
ADMIN_PWD=$(kubectl get secret db-admin-secret -o jsonpath='{.data.oracle_pwd}' | base64 -d)
kubectl exec -i "$STANDBY_POD" -c <container> -- \
  timeout 25 sqlplus -L -s "sys/\"${ADMIN_PWD}\"@ORCLPRDTCPS as sysdba" \
  <<<'select db_unique_name, database_role from v$database; exit'
```

---

## 7. Configure DataguardBroker over TCPS

After all SIDBs are Healthy:

1. Collect each member’s TCPS endpoint (host/port/service)
2. Ensure DG client wallet Secrets exist for every member the Broker runner must contact
3. Copy wallet Secrets across clusters when the Broker pod runs in one cluster and must reach remote members
4. Apply a `DataguardBroker` CR with `tcps.enabled: true` and TCPS endpoints for all members
5. Validate with DGMGRL `SHOW CONFIGURATION` and switchover tests

Member ports and wallet secret names must stay aligned with the SIDB services created above.

---

## Failure modes tied to certificates

| Symptom | Likely cause |
| --- | --- |
| TCPS handshake / peer auth failures | Different intermediate CA on primary vs standby clusters |
| `unknown field spec.primarySource.tcpsPort` | Peer cluster CRD older than manifests |
| Operator CrashLoop `forbidden` | Stale manager ClusterRole vs operator image |
| Standby duplicate `ORA-12541` on NodePort | Auxiliary listener/NodePort not ready yet (often not TLS) |

---

## Related docs

- [`tcps-cert-manager/SEPARATE_CLUSTER_ACCESS.md`](../tcps-cert-manager/SEPARATE_CLUSTER_ACCESS.md) — certificate method
- [`DATAGUARD_MULTI_CLUSTER_TCPS_NODEPORT.md`](./DATAGUARD_MULTI_CLUSTER_TCPS_NODEPORT.md) — NodePort-oriented DG examples
- [`PREREQUISITES.md`](../prerequisites/PREREQUISITES.md)
