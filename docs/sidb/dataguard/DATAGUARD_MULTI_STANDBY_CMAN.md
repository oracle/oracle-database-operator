# Multi-Standby Data Guard with CMAN (1 primary + 3 standbys across 2 clusters)

This runbook covers a validated multi-standby topology:

| Member | Cluster | Role | How peers reach it |
|---|---|---|---|
| `ORCLPRD` / `sidb-primary` | Cluster 1 | Primary | Cluster 1 CMAN NodePort |
| `ORCLSTBY1` / `sidb-standby1` | Cluster 1 | Physical standby | Same-cluster ClusterIP + Cluster 1 CMAN |
| `ORCLSTBY2` / `sidb-standby2` | Cluster 2 | Physical standby | Cluster 2 CMAN NodePort |
| `ORCLSTBY3` / `sidb-standby3` | Cluster 2 | Physical standby | Cluster 2 CMAN NodePort |

Recommended order:

1. Create Cluster 1 primary and standby1 using direct ClusterIP connectivity
2. Deploy Cluster 1 CMAN (file-mode + service-alias tnsnames)
3. Create Cluster 2 standbys through Cluster 1 CMAN
4. Deploy Cluster 2 CMAN
5. Configure a four-member DataguardBroker using CMAN endpoints
6. Test switchover rotation and optional Fast-Start Failover (FSFO)

For single primary/standby CMAN, see [`DATAGUARD_CROSS_CLUSTER_CMAN.md`](./DATAGUARD_CROSS_CLUSTER_CMAN.md). Direct SIDB NodePort-only Data Guard is intentionally not documented; use CMAN or the LoadBalancer path.

## Sample files (in this repository)

| Role | Sample |
|---|---|
| Primary | [`config/samples/sidb/singleinstancedatabase_multi_primary_cman.yaml`](../../../config/samples/sidb/singleinstancedatabase_multi_primary_cman.yaml) |
| Standby1 (same cluster) | [`config/samples/sidb/singleinstancedatabase_multi_standby1_same_cluster_cman.yaml`](../../../config/samples/sidb/singleinstancedatabase_multi_standby1_same_cluster_cman.yaml) |
| Standby2 (Cluster 2) | [`config/samples/sidb/singleinstancedatabase_multi_standby2_cross_cluster_cman.yaml`](../../../config/samples/sidb/singleinstancedatabase_multi_standby2_cross_cluster_cman.yaml) |
| Standby3 (Cluster 2) | [`config/samples/sidb/singleinstancedatabase_multi_standby3_cross_cluster_cman.yaml`](../../../config/samples/sidb/singleinstancedatabase_multi_standby3_cross_cluster_cman.yaml) |
| Cluster 1 CMAN | [`config/samples/sidb/connectionmanager_cman_cluster1_multi_standby_dg.yaml`](../../../config/samples/sidb/connectionmanager_cman_cluster1_multi_standby_dg.yaml) |
| Cluster 2 CMAN | [`config/samples/sidb/connectionmanager_cman_cluster2_multi_standby_dg.yaml`](../../../config/samples/sidb/connectionmanager_cman_cluster2_multi_standby_dg.yaml) |
| Four-member Broker | [`config/samples/sidb/dataguardbroker_multi_standby_cman.yaml`](../../../config/samples/sidb/dataguardbroker_multi_standby_cman.yaml) |
| Broker renderer | [`config/samples/sidb/render-dg-broker-from-status.sh`](../../../config/samples/sidb/render-dg-broker-from-status.sh) |

## Topology

```text
Cluster 1                                              Cluster 2
-----------------------------------------              -----------------------------------------
sidb-primary / ORCLPRD  <---ClusterIP---> sidb-standby1 / ORCLSTBY1
        \                                         /
         \                                       /
          +---- CMAN cman-sidb :30189 ----------+
                     |                                      CMAN cman-sidb :30189
                     |                                           |
                     +-------- RMAN / Broker / redo -------------+
                                     |                           |
                              sidb-standby2 / ORCLSTBY2   sidb-standby3 / ORCLSTBY3

DataguardBroker sidb-multi-dg (apply in Cluster 2)
  ORCLPRD, ORCLSTBY1 -> <cluster1-cman-node-ip>:30189
  ORCLSTBY2, ORCLSTBY3 -> <cluster2-cman-node-ip>:30189
```

CMAN uses **file-mode** `cman.ora` with:

- embedded `# CMAN_TNSNAMES_BEGIN ... # CMAN_TNSNAMES_END` aliases
- `use_service_as_tnsnames_alias=on`
- per-service accept rules for each DB service and `*_DGMGRL`

## Prerequisites

- Operator with `ConnectionManager`, `DataguardBroker`, and SIDB CRDs in both clusters
- Matching admin Secret and TDE wallet Secret in both clusters
- Matching SIDB image tags across all four databases
- Network path to each CMAN NodePort (`30189` in samples)
- CMAN image that supports embedded tnsnames / `use_service_as_tnsnames_alias`

## 1. Deploy operator 2.3

```bash
kubectl apply -f oracle-database-operator.yaml
```

## 2. Cluster 1 primary + same-cluster standby1

```bash
# Cluster 1
kubectl apply -f config/samples/sidb/singleinstancedatabase_multi_primary_cman.yaml
# Wait until Healthy / PRIMARY, then export TDE wallet into tde-wallet-secret

kubectl apply -f config/samples/sidb/singleinstancedatabase_multi_standby1_same_cluster_cman.yaml
```

Standby1 uses:

```text
primarySource.connectString: sidb-primary.default.svc.cluster.local:1521/ORCLPRD
```

Validate MRP on standby1 and primary role before continuing.

## 3. Deploy Cluster 1 CMAN

```bash
kubectl apply -f config/samples/sidb/connectionmanager_cman_cluster1_multi_standby_dg.yaml
kubectl get connectionmanager cman-sidb
kubectl get svc cman-sidb-ext -o wide
```

Verify CMAN-routed SQL\*Net:

```bash
# From a Cluster 1 pod, using admin password
sqlplus "sys/<pwd>@<cluster1-cman-node-ip>:30189/ORCLPRD as sysdba"
sqlplus "sys/<pwd>@<cluster1-cman-node-ip>:30189/ORCLSTBY1 as sysdba"
```

Confirm embedded aliases exist in the CMAN pod `tnsnames.ora` (`ORCLPRD`, `ORCLPRD_DGMGRL`, `ORCLSTBY1`, `ORCLSTBY1_DGMGRL`).

With this CMAN pattern, SIDB `remote_listener` may remain empty; routing is through service-alias tnsnames, not dynamic registration alone.

## 4. Cluster 2 standbys via Cluster 1 CMAN

Copy the TDE wallet Secret into Cluster 2, then:

```bash
# Cluster 2
kubectl apply -f config/samples/sidb/singleinstancedatabase_multi_standby2_cross_cluster_cman.yaml
kubectl apply -f config/samples/sidb/singleinstancedatabase_multi_standby3_cross_cluster_cman.yaml
```

Both use:

```text
primarySource.connectString: <cluster1-cman-node-ip>:30189/ORCLPRD
```

Wait for each standby to report `DATABASE IS READY TO USE!` and Healthy / PHYSICAL_STANDBY.

## 5. Deploy Cluster 2 CMAN

```bash
kubectl apply -f config/samples/sidb/connectionmanager_cman_cluster2_multi_standby_dg.yaml
```

Validate:

```bash
sqlplus "sys/<pwd>@//cman-sidb.default.svc.cluster.local:1521/ORCLSTBY2 as sysdba"
sqlplus "sys/<pwd>@//cman-sidb.default.svc.cluster.local:1521/ORCLSTBY3 as sysdba"
```

## 6. Build and apply the four-member DataguardBroker

Generate the four-member Broker with CMAN NodePort endpoints via multi-member mode:

```bash
# From repository root. Replace kubeconfigs and CMAN node IPs/ports as needed.
bash config/samples/sidb/render-dg-broker-from-status.sh sidb \
  --broker-name sidb-multi-dg \
  --namespace default \
  --kubeconfig c1=/path/to/cluster1.kubeconfig \
  --kubeconfig c2=/path/to/cluster2.kubeconfig \
  --member c1:sidb-sample:host=<cluster1-cman-node-ip>:30189 \
  --member c1:sidb-standby1:host=<cluster1-cman-node-ip>:30189 \
  --member c2:sidb-standby2:host=<cluster2-cman-node-ip>:30189 \
  --member c2:sidb-standby3:host=<cluster2-cman-node-ip>:30189 \
  --source-ref c2:sidb-standby3 \
  > dataguardbroker-generated.yaml

# Cluster 2
kubectl apply -f dataguardbroker-generated.yaml
# Or edit placeholders in the reviewed template:
# kubectl apply -f config/samples/sidb/dataguardbroker_multi_standby_cman.yaml
```

Expected members:

```text
ORCLPRD   -> <cluster1-cman-node-ip>:30189
ORCLSTBY1 -> <cluster1-cman-node-ip>:30189
ORCLSTBY2 -> <cluster2-cman-node-ip>:30189
ORCLSTBY3 -> <cluster2-cman-node-ip>:30189
```

Watch the Broker runner until Configuration Status is `SUCCESS` with all four members.

## 7. Validate

```text
DGMGRL> CONNECT /@ORCLPRD
DGMGRL> SHOW CONFIGURATION;
DGMGRL> VALIDATE DATABASE VERBOSE ORCLPRD;
DGMGRL> VALIDATE DATABASE VERBOSE ORCLSTBY1;
DGMGRL> VALIDATE DATABASE VERBOSE ORCLSTBY2;
DGMGRL> VALIDATE DATABASE VERBOSE ORCLSTBY3;
```

Gate: `Ready for Switchover: Yes` on the intended target and Configuration Status `SUCCESS`.

## 8. Switchover rotation

Pause operators during role changes if they cancel MRP / rewrite Broker files.

```text
DGMGRL> SWITCHOVER TO ORCLSTBY1;
DGMGRL> SWITCHOVER TO ORCLSTBY2;
DGMGRL> SWITCHOVER TO ORCLSTBY3;
DGMGRL> SWITCHOVER TO ORCLPRD;
```

`ORA-01100` (already mounted) during old-primary restart can be a harmless race; confirm the member is open in the expected role afterward.

## 9. Fast-Start Failover (optional)

Validated pattern from lab:

1. Ensure Flashback is ON on primary and FSFO target
2. `EDIT DATABASE 'ORCLPRD' SET PROPERTY FastStartFailoverTarget='orclstby3';`
3. `VALIDATE NETWORK CONFIGURATION FOR ALL;`
4. Ensure the operator-created observer pod is Running (`sidb-multi-dg-observer`)
5. Validate static connect identifiers **before** enabling FSFO

Important lab finding: static connect validation succeeded when identifiers used **SIDB NodePorts** (for example `32330` / `30281` / `30282` / `30283`), not only CMAN. If `VALIDATE STATIC CONNECT IDENTIFIER FOR ALL` fails against CMAN ports, point static connect identifiers at each member’s SIDB NodePort while keeping Broker DG connect identifiers on CMAN.

```text
DGMGRL> VALIDATE STATIC CONNECT IDENTIFIER FOR ALL;
DGMGRL> ENABLE FAST_START FAILOVER;
DGMGRL> SHOW FAST_START FAILOVER;
```

### Unplanned outage test notes

- Scale operators to `0` before killing the primary repeatedly, otherwise the controller may fight the outage simulation
- After FSFO, expect former primary `ORA-16661` (must reinstate) and possibly `ORA-16844` until a valid FSFO target is configured on the new primary
- Reinstate flow:
  1. `startup mount` on former primary (never open read/write)
  2. Restore the SIDB readiness lock if required by your image
  3. `REINSTATE DATABASE ORCLPRD;`
  4. Set a new FSFO target on the current primary and clear warnings

## Troubleshooting

### A) Renderer only produces two Broker members

Expected. Use multi-member `render-dg-broker-from-status.sh` with CMAN `host=<ip>:30189`, or start from [`dataguardbroker_multi_standby_cman.yaml`](../../../config/samples/sidb/dataguardbroker_multi_standby_cman.yaml).

### B) CMAN Healthy but SQL\*Net to `ORCLSTBYn` fails

Confirm embedded tnsnames aliases and `use_service_as_tnsnames_alias=on`, and that rule_list includes both the DB service and `*_DGMGRL`.

### C) Switchover works but FSFO static connect fails on CMAN ports

Use SIDB NodePorts for static connect identifiers; keep CMAN for DG/Broker runtime endpoints if that is the proven split from validation.

### D) Dual-standby / lag recovery

Reuse recovery steps in [`DATAGUARD_CROSS_CLUSTER_LB.md`](./DATAGUARD_CROSS_CLUSTER_LB.md#b-failed-switchover-leaves-dual-physical-standby-no-primary).

## Related files

- Single-standby CMAN: [`DATAGUARD_CROSS_CLUSTER_CMAN.md`](./DATAGUARD_CROSS_CLUSTER_CMAN.md)
- LB-only: [`DATAGUARD_CROSS_CLUSTER_LB.md`](./DATAGUARD_CROSS_CLUSTER_LB.md)
- CMAN guide: [`../cman/README.md`](../../cman/README.md)
