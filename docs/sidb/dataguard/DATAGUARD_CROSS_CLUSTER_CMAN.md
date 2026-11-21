# Cross-Cluster Data Guard through CMAN (Primary + Standby + DataguardBroker)

This runbook covers a **two-cluster** SIDB Data Guard topology where:

- **Primary** and **standby** SIDBs run in separate Kubernetes clusters
- Each cluster runs **Oracle Connection Manager (CMAN)** via `kind: ConnectionManager`
  (`network.oracle.com/v4`). Do not create new `TrafficManager` resources with
  `spec.type: cman` — that path is deprecated on master.
- Cross-cluster clients and Broker reach databases through **CMAN NodePorts**
- CMAN uses **file-mode `cman.ora` with `next_hop`** to the local SIDB ClusterIP Service so static `*_DGMGRL` services remain reachable during switchover

Related runbooks:

- LoadBalancer path: [`DATAGUARD_CROSS_CLUSTER_LB.md`](./DATAGUARD_CROSS_CLUSTER_LB.md)
- Direct SIDB NodePort-only Data Guard is intentionally not documented; use this CMAN path or the LoadBalancer path for cross-cluster deployments.
- Multi-standby CMAN (1 primary + 3 standbys): [`DATAGUARD_MULTI_STANDBY_CMAN.md`](./DATAGUARD_MULTI_STANDBY_CMAN.md)
- General CMAN guide: [`../cman/README.md`](../../cman/README.md)

## Sample files (in this repository)

| Role | Sample |
|---|---|
| Primary SIDB | [`config/samples/sidb/singleinstancedatabase_primary_cross_cluster_cman.yaml`](../../../config/samples/sidb/singleinstancedatabase_primary_cross_cluster_cman.yaml) |
| Standby SIDB | [`config/samples/sidb/singleinstancedatabase_standby_cross_cluster_cman.yaml`](../../../config/samples/sidb/singleinstancedatabase_standby_cross_cluster_cman.yaml) |
| Primary CMAN (file-mode + next_hop) | [`config/samples/sidb/connectionmanager_cman_primary_cross_cluster_dg.yaml`](../../../config/samples/sidb/connectionmanager_cman_primary_cross_cluster_dg.yaml) |
| Standby CMAN (file-mode + next_hop) | [`config/samples/sidb/connectionmanager_cman_standby_cross_cluster_dg.yaml`](../../../config/samples/sidb/connectionmanager_cman_standby_cross_cluster_dg.yaml) |
| DataguardBroker | [`config/samples/sidb/dataguardbroker_cross_cluster_cman.yaml`](../../../config/samples/sidb/dataguardbroker_cross_cluster_cman.yaml) |
| Broker renderer | [`config/samples/sidb/render-dg-broker-from-status.sh`](../../../config/samples/sidb/render-dg-broker-from-status.sh) |

## Topology

```text
Primary cluster                                         Standby cluster
-----------------------------------------               -----------------------------------------
SIDB sidb-sample / ORCLPRD                              SIDB sidb-standby / ORCLSTBY
  ClusterIP sidb-sample:1521                              ClusterIP sidb-standby:1521
  NodePort (optional diagnostics)                         NodePort (optional diagnostics)
           ^                                                        ^
           | next_hop                                               | next_hop
CMAN cman-sidb (file-mode)                              CMAN cman-sidb (file-mode)
  NodePort <primary-cman-node-ip>:30189                   NodePort <standby-cman-node-ip>:30189
           |                                                        ^
           +-------------- Broker / redo / RMAN --------------------+

DataguardBroker (standby cluster runner)
  ORCLPRD  -> <primary-cman-node-ip>:30189/ORCLPRD
  ORCLSTBY -> <standby-cman-node-ip>:30189/ORCLSTBY
```

## Why file-mode + `next_hop` is required for switchover

> **Do not start this topology with generated-rules CMAN + `remote_listener`.**
> That path can look healthy (`SHOW CONFIGURATION` / `lsnrctl services` READY)
> and still fail switchover. Use **file-mode `next_hop` from the first CMAN
> apply** on both clusters. `remote_listener` is optional with `next_hop` and
> is **not** the recommended Broker path.

### Anti-pattern (loses hours): generated rules + `remote_listener`

1. Deploy CMAN with generated `rules:` (database registers via `remote_listener`)
2. Build DataguardBroker — config status `SUCCESS`
3. `SWITCHOVER TO <standby>` fails when Broker tries to restart the old primary:

```text
ORA-12514: ... Service ORCLPRD_DGMGRL is not registered with the listener
  at host <cman-node-ip> port 30189
```

During switchover the old primary shuts down, so its **dynamic** `*_DGMGRL`
registration disappears from CMAN. The database listener still has a **static**
`*_DGMGRL` entry, but generated-rules CMAN does not forward to that local
listener after registration is gone.

The same class of failure appears with empty/`hostname` `local_listener`
(`ORA-12564` / standby `DBT-05514`) when registration advertises an unreachable
address — see [`../cman/README.md`](../../cman/README.md#generated-rules-local_listener-must-advertise-a-reachable-address).

### Required path: file-mode `next_hop`

```text
CMAN listener
  -> next_hop local SIDB Service (ClusterIP:1521)
  -> database listener static *_DGMGRL / normal services
```

Ashburn: `next_hop` → `sidb-sample.default.svc.cluster.local:1521`  
Mumbai: `next_hop` → `sidb-standby.default.svc.cluster.local:1521`

One wildcard rule `(src=*)(dst=*)(srv=*)(act=accept)` is enough so normal
services and `*_DGMGRL` share the same path. Clients and Broker use the CMAN
NodePort; they do **not** depend on dynamic registration for switchover.

## Prerequisites

- Peered networks allowing each cluster to reach the other CMAN NodePort (`30189` in the samples)
- Operator with `ConnectionManager` and `DataguardBroker` CRDs installed in both clusters
- Matching admin Secret in both clusters
- Matching SIDB image tags
- TDE wallet zip copied to the standby cluster when primary uses TDE
- A CMAN image that supports file-mode `configSource.configMapRef`
- For cross-cluster standby create, the standby SIDB **NodePort** Service must keep
  `publishNotReadyAddresses: true` so primary RMAN can reach the auxiliary on
  `nodeIP:NodePort` while the pod is still NotReady (avoids `ORA-12541` during
  Duplicate). Current operators set this automatically for cross-cluster /
  flashback standbys; samples also set it explicitly.


## 1. Deploy operator

```bash
kubectl apply -f oracle-database-operator.yaml
kubectl -n oracle-database-operator-system get deploy,pods
```

## 2. Create primary SIDB (cluster A)

```bash
# Primary cluster
kubectl apply -f config/samples/sidb/singleinstancedatabase_primary_cross_cluster_cman.yaml
kubectl get sidb sidb-sample -o wide
kubectl get svc sidb-sample sidb-sample-nodeport -o wide
```

Wait until the primary is Healthy / PRIMARY.

## 3. Copy TDE wallet to the standby cluster

```bash
# Primary cluster
PRIMARY_POD=$(kubectl get pods -l app=sidb-sample -o jsonpath='{.items[0].metadata.name}')
rm -rf /tmp/standby-wallet /tmp/standby-wallet.zip
mkdir -p /tmp/standby-wallet/tde
kubectl cp -n <operator-watched-namespace> \
  "${PRIMARY_POD}:/opt/oracle/oradata/ORCLPRD/tdewallet/tde/ewallet.p12" \
  /tmp/standby-wallet/tde/ewallet.p12 \
  -c sidb-sample
(cd /tmp/standby-wallet && zip -qr /tmp/standby-wallet.zip tde)

# Standby cluster
kubectl create secret generic tde-wallet-secret \
  --from-file=standby-wallet.zip=/tmp/standby-wallet.zip \
  --from-literal=tde_wallet_pwd='<tde-wallet-password>' \
  --dry-run=client -o yaml | kubectl apply -f -
```

## 4. Deploy primary-cluster CMAN (file-mode + next_hop)

Use the **file-mode `next_hop` sample** (or an equivalent `ConnectionManager` +
ConfigMap). Do **not** deploy generated-rules CMAN for this topology.

Edit placeholders (`<cman-container-image>`, namespace, next_hop Service name if different), then:

```bash
# Primary cluster
kubectl apply -f config/samples/sidb/connectionmanager_cman_primary_cross_cluster_dg.yaml
# Prefer: kubectl get connectionmanager cman-sidb ...
kubectl get connectionmanager,trafficmanager cman-sidb \
  -o custom-columns='KIND:.kind,NAME:.metadata.name,STATUS:.status.status' 2>/dev/null
kubectl get svc cman-sidb cman-sidb-ext -o wide
```

Confirm the pod log shows **user-defined** `cman.ora` (file mode), not
"Generating CMAN file".

```bash
CMAN_POD=$(kubectl get pods -l app.kubernetes.io/name=cman-sidb -o jsonpath='{.items[0].metadata.name}')
kubectl logs "${CMAN_POD}" | grep -E 'user defined cman|Generating CMAN|READY TO USE'
kubectl exec "${CMAN_POD}" -- grep -A3 next_hop \
  /u01/app/oracle/product/23ai/client_1/network/admin/cman.ora
```

**Optional:** set `remote_listener` only if you want dynamic registration
visible in `lsnrctl services`. It is **not required** for Broker or Easy
Connect through file-mode `next_hop`, and it must **not** replace `next_hop`.

```bash
# Optional only — skip for the recommended Broker/switchover path
PRIMARY_POD=$(kubectl get pods -l app=sidb-sample -o jsonpath='{.items[0].metadata.name}')
kubectl exec -i "${PRIMARY_POD}" -c sidb-sample -- sqlplus -s / as sysdba <<'SQL'
ALTER SYSTEM SET remote_listener=
  '(ADDRESS=(PROTOCOL=TCP)(HOST=cman-sidb.default.svc.cluster.local)(PORT=1521))'
  SCOPE=BOTH;
ALTER SYSTEM REGISTER;
EXIT
SQL
```

Verify reachability through CMAN NodePort (Easy Connect / `sqlplus` to
`<primary-cman-node-ip>:30189/ORCLPRD` and later `ORCLPRD_DGMGRL`).

## 5. Create standby SIDB (cluster B)

Standby `primarySource.connectString` uses the **primary CMAN NodePort**:

```text
<primary-cman-node-ip>:30189/ORCLPRD
```

```bash
# Standby cluster
kubectl apply -f config/samples/sidb/singleinstancedatabase_standby_cross_cluster_cman.yaml
STANDBY_POD=$(kubectl get pods -l app=sidb-standby -o jsonpath='{.items[0].metadata.name}')
kubectl logs -f "${STANDBY_POD}" -c sidb-standby
```

Wait for DBCA / `DATABASE IS READY TO USE!` and MRP.

## 6. Deploy standby-cluster CMAN (file-mode + next_hop)

Same rule as primary: **file-mode `next_hop` only** (to
`sidb-standby.default.svc.cluster.local:1521`). Do not use generated-rules CMAN.

```bash
# Standby cluster
kubectl apply -f config/samples/sidb/connectionmanager_cman_standby_cross_cluster_dg.yaml
kubectl get connectionmanager,trafficmanager cman-sidb \
  -o custom-columns='KIND:.kind,NAME:.metadata.name,STATUS:.status.status' 2>/dev/null
kubectl get svc cman-sidb cman-sidb-ext -o wide
```

Confirm file mode + `next_hop` in the CMAN pod (same checks as section 4).

**Optional:** `remote_listener` / `local_listener` registration — only if you
want services listed on CMAN via dynamic registration. **Not required** for
Broker switchover when `next_hop` is in place. Skip the block below for the
recommended path.

```bash
# Optional only
STANDBY_POD=$(kubectl get pods -l app=sidb-standby -o jsonpath='{.items[0].metadata.name}')
STANDBY_POD_IP=$(kubectl get pod "${STANDBY_POD}" -o jsonpath='{.status.podIP}')

kubectl exec -i "${STANDBY_POD}" -c sidb-standby -- \
  sqlplus -s / as sysdba <<SQL
ALTER SYSTEM SET local_listener=
  '(ADDRESS=(PROTOCOL=TCP)(HOST=${STANDBY_POD_IP})(PORT=1521))'
  SCOPE=BOTH;
ALTER SYSTEM SET remote_listener=
  'cman-sidb.default.svc.cluster.local:1521'
  SCOPE=BOTH;
ALTER SYSTEM REGISTER;
select name, value from v\$parameter
 where name in ('local_listener','remote_listener');
exit
SQL
```

Verify standby services on CMAN:

```bash
CMAN_POD=$(kubectl get pods -l app.kubernetes.io/name=cman-sidb -o jsonpath='{.items[0].metadata.name}')
kubectl exec "${CMAN_POD}" -- lsnrctl services | grep -E 'Service|ORCLSTBY|apppdb1|READY'
```

## 7. Validate redo / roles

### Primary

```bash
PRIMARY_POD=$(kubectl get pods -l app=sidb-sample -o jsonpath='{.items[0].metadata.name}')
kubectl exec -i "${PRIMARY_POD}" -c sidb-sample -- sqlplus -s / as sysdba <<'SQL'
select name, db_unique_name, database_role, open_mode from v$database;
select dest_id, status, target, destination, error from v$archive_dest where dest_id <= 5;
select name, value from v$parameter where name in ('local_listener','remote_listener');
exit
SQL
```

### Standby

```bash
STANDBY_POD=$(kubectl get pods -l app=sidb-standby -o jsonpath='{.items[0].metadata.name}')
kubectl exec -i "${STANDBY_POD}" -c sidb-standby -- sqlplus -s / as sysdba <<'SQL'
select name, db_unique_name, database_role, open_mode from v$database;
select process, status, thread#, sequence# from v$managed_standby order by process;
select name, value from v$parameter where name in ('local_listener','remote_listener');
exit
SQL
```

Expect standby `PHYSICAL STANDBY` / `READ ONLY WITH APPLY` and `MRP0` applying.

Update primary `standbySources` / `LOG_ARCHIVE_DEST_n` to the standby CMAN NodePort if redo is not yet shipping through CMAN.

## 8. Create DataguardBroker

Generate with CMAN NodePort endpoints via multi-member mode (preferred):

```bash
# From repository root
bash config/samples/sidb/render-dg-broker-from-status.sh sidb \
  --broker-name sidb-standby-dg \
  --namespace <operator-watched-namespace> \
  --kubeconfig primary=/path/to/primary.kubeconfig \
  --kubeconfig standby=/path/to/standby.kubeconfig \
  --member primary:sidb-sample:host=<primary-cman-node-ip>:30189 \
  --member standby:sidb-standby:host=<standby-cman-node-ip>:30189 \
  --source-ref standby:sidb-standby \
  > dataguardbroker-generated.yaml
```

Single-standby render still works if you then edit endpoints to **CMAN NodePorts** (not ClusterIP, and usually not the SIDB diagnostic NodePort):

```bash
# Standby cluster
bash config/samples/sidb/render-dg-broker-from-status.sh \
  sidb sidb-standby <operator-watched-namespace> \
  > dataguardbroker-generated.yaml
```

Or apply the reviewed template:

```bash
kubectl apply -f config/samples/sidb/dataguardbroker_cross_cluster_cman.yaml
```

Expected endpoints:

```yaml
ORCLPRD:
  host: <primary-cman-node-ip>
  port: 30189
ORCLSTBY:
  host: <standby-cman-node-ip>
  port: 30189
```

Also set Broker static connect identifiers to the same CMAN endpoints with `SERVICE_NAME=<DB>_DGMGRL` if the controller/renderer did not already.

Watch the runner until Configuration Status is `SUCCESS`.

## 9. Validate and switchover

```bash
RUNNER=$(kubectl get pods --no-headers | awk '/dg-runner/ {print $1; exit}')
kubectl exec "${RUNNER}" -- bash -lc '
printf "%s\n" \
  "SHOW CONFIGURATION;" \
  "VALIDATE DATABASE VERBOSE ORCLPRD;" \
  "VALIDATE DATABASE VERBOSE ORCLSTBY;" \
| dgmgrl /@ORCLPRD as sysdba
'
```

Optional static checks through CMAN aliases:

```bash
kubectl exec "${RUNNER}" -- sqlplus -L '/@ORCLPRD_DGMGRL as sysdba'
kubectl exec "${RUNNER}" -- sqlplus -L '/@ORCLSTBY_DGMGRL as sysdba'
```

### Switchover to standby

```text
DGMGRL> SWITCHOVER TO ORCLSTBY;
```

### Switchback to primary

```text
DGMGRL> connect /@ORCLSTBY
DGMGRL> SWITCHOVER TO ORCLPRD;
```

### Post-switchover readiness note

If Broker reports a harmless `ORA-01100` (database already mounted) during startup races, confirm the former primary is open as standby and open the PDB read-only if readiness requires it:

```sql
ALTER PLUGGABLE DATABASE APPPDB1 OPEN READ ONLY;
```

## Troubleshooting

### A) Switchover fails with `ORA-12514` on `*_DGMGRL` through CMAN NodePort

Cause: CMAN used **generated rules + `remote_listener`**. Dynamic `*_DGMGRL`
registration disappeared when the old primary shut down. Broker config can
still show `SUCCESS` before switchover.

Fix:

1. Delete generated-rules CMAN and deploy **file-mode + `next_hop`** samples
   (sections 4 and 6) on **both** clusters
2. Confirm pod logs say user-defined `cman.ora` and `next_hop` points at the
   local SIDB ClusterIP Service (`sidb-sample` / `sidb-standby`)
3. Test `ORCLPRD_DGMGRL` / `ORCLSTBY_DGMGRL` through each CMAN NodePort **before**
   retrying switchover — `next_hop` must succeed even when the instance is down
   for dynamic registration
4. Do **not** treat setting `remote_listener` alone as the fix

### B) Broker connects to wrong port (`1521` on node IP, or SIDB NodePort instead of CMAN)

Edit DataguardBroker member endpoints to `<cman-node-ip>:30189` and update static connect identifiers accordingly.

### C) CMAN Healthy but no `ORCLPRD` / `ORCLSTBY` services

Confirm:

- `remote_listener` points at `cman-sidb.<ns>.svc.cluster.local:1521`
- `ALTER SYSTEM REGISTER` was run
- For standby, `local_listener` uses the current pod IP
- `next_hop` host/port matches the SIDB ClusterIP Service

### D) Lag / dual-standby recovery after a failed switchover

Reuse the recovery steps in [`DATAGUARD_CROSS_CLUSTER_LB.md`](./DATAGUARD_CROSS_CLUSTER_LB.md#b-failed-switchover-leaves-dual-physical-standby-no-primary).

## Related files

- Primary SIDB: [`config/samples/sidb/singleinstancedatabase_primary_cross_cluster_cman.yaml`](../../../config/samples/sidb/singleinstancedatabase_primary_cross_cluster_cman.yaml)
- Standby SIDB: [`config/samples/sidb/singleinstancedatabase_standby_cross_cluster_cman.yaml`](../../../config/samples/sidb/singleinstancedatabase_standby_cross_cluster_cman.yaml)
- Primary CMAN: [`config/samples/sidb/connectionmanager_cman_primary_cross_cluster_dg.yaml`](../../../config/samples/sidb/connectionmanager_cman_primary_cross_cluster_dg.yaml)
- Standby CMAN: [`config/samples/sidb/connectionmanager_cman_standby_cross_cluster_dg.yaml`](../../../config/samples/sidb/connectionmanager_cman_standby_cross_cluster_dg.yaml)
- Broker: [`config/samples/sidb/dataguardbroker_cross_cluster_cman.yaml`](../../../config/samples/sidb/dataguardbroker_cross_cluster_cman.yaml)
- Multi-standby CMAN: [`DATAGUARD_MULTI_STANDBY_CMAN.md`](./DATAGUARD_MULTI_STANDBY_CMAN.md)
- CMAN guide: [`../cman/README.md`](../../cman/README.md)
