# Multi-Standby Data Guard over OCI NLB (1 primary + 3 standbys)

This runbook covers a validated multi-standby topology using **OCI Network Load Balancer (NLB)** TCP endpoints on port `1521` (no CMAN, no NodePort for Broker):

| Member | Cluster | CR | Example NLB |
|---|---|---|---|
| `ORCLPRD` | Cluster 1 | `sidb-sample` | `10.0.2.181:1521` |
| `ORCLSTBY1` | Cluster 1 (same cluster) | `sidb-standby1` | `10.0.2.77:1521` |
| `ORCLSTBY2` | Cluster 2 | `sidb-standby2` | `10.2.1.233:1521` |
| `ORCLSTBY3` | Cluster 2 | `sidb-standby3` | `10.2.1.6:1521` |

Recommended order:

1. Create Cluster 1 primary (`sidb-sample`) with NLB
2. Create Cluster 1 standby1 (same cluster) via primary NLB / ClusterIP
3. Create Cluster 2 standbys **one at a time** through the primary NLB
4. Ensure each Mumbai standby pod is created **after** its LB has an EXTERNAL-IP (`SVC_HOST=<LB-IP>`)
5. Patch primary `standbySources` with all standby NLB IPs
6. Generate/apply four-member DataguardBroker using NLB `:1521` endpoints
7. Validate and run switchover / switchback

Related:

- Single primary/standby LB: [`DATAGUARD_CROSS_CLUSTER_LB.md`](./DATAGUARD_CROSS_CLUSTER_LB.md)
- Direct SIDB NodePort-only Data Guard is intentionally not documented; use this NLB path, CMAN, or the documented TCP+TCPS NodePort path.
- Multi-standby CMAN: [`DATAGUARD_MULTI_STANDBY_CMAN.md`](./DATAGUARD_MULTI_STANDBY_CMAN.md)
- Mumbai NLB annotation reference: lab `dns-working-files-mumbai/05-truecache-nlb-service.yaml`

## Sample files (in this repository)

| Role | Sample |
|---|---|
| Primary | [`config/samples/sidb/singleinstancedatabase_multi_primary_lb.yaml`](../../../config/samples/sidb/singleinstancedatabase_multi_primary_lb.yaml) |
| Standby1 (same cluster) | [`config/samples/sidb/singleinstancedatabase_multi_standby1_same_cluster_lb.yaml`](../../../config/samples/sidb/singleinstancedatabase_multi_standby1_same_cluster_lb.yaml) |
| Standby2 (Cluster 2) | [`config/samples/sidb/singleinstancedatabase_multi_standby2_cross_cluster_lb.yaml`](../../../config/samples/sidb/singleinstancedatabase_multi_standby2_cross_cluster_lb.yaml) |
| Standby3 (Cluster 2) | [`config/samples/sidb/singleinstancedatabase_multi_standby3_cross_cluster_lb.yaml`](../../../config/samples/sidb/singleinstancedatabase_multi_standby3_cross_cluster_lb.yaml) |
| Four-member Broker | [`config/samples/sidb/dataguardbroker_multi_standby_lb.yaml`](../../../config/samples/sidb/dataguardbroker_multi_standby_lb.yaml) |
| Broker renderer | [`config/samples/sidb/render-dg-broker-from-status.sh`](../../../config/samples/sidb/render-dg-broker-from-status.sh) |

Lab copies used during validation (outside git): `multi-sidb-create-v4-tcp-*-lb.yaml`, `multi-sidb-standby-mumbai-*-lb.yaml`, `multi-dataguardbroker-lb.yaml`.

## Topology

```text
Cluster 1 (Ashburn)                              Cluster 2 (Mumbai)
-----------------------------------------        -----------------------------------------
sidb-sample / ORCLPRD
  NLB 10.0.2.181:1521
sidb-standby1 / ORCLSTBY1
  NLB 10.0.2.77:1521
        ^                                              sidb-standby2 NLB 10.2.1.233:1521
        |                                              sidb-standby3 NLB 10.2.1.6:1521
        +------------- NLB redo / Broker --------------+

DataguardBroker sidb-multi-dg (apply in Cluster 2)
  all members -> NLB host:1521
```

## Prerequisites

- Operator 2.3 in both clusters (image that sets `SVC_HOST=<LB-IP>` / `CROSS_CLUSTER_STANDBY=true` for external-primary standbys)
- Peered VCNs / routes for NLB TCP `1521`
- Matching admin Secret and TDE wallet Secret in both clusters
- Matching SIDB image tags on all four databases
- NLB annotations must use **`externalTrafficPolicy: Local`** with `is-preserve-source: "false"`  
  (`Cluster` + `is-preserve-source` is rejected by OCI and leaves EXTERNAL-IP `<pending>`)

## 1. Deploy operator 2.3

```bash
kubectl apply -f oracle-database-operator.yaml
```

## 2. Cluster 1 primary (`sidb-sample`)

```bash
# Cluster 1
kubectl apply -f config/samples/sidb/singleinstancedatabase_multi_primary_lb.yaml
kubectl get sidb sidb-sample -o wide
kubectl get svc sidb-sample-lb -o wide
```

Wait until Healthy / PRIMARY and NLB EXTERNAL-IP is assigned (example `10.0.2.181`).

Patch `standbySources` after each standby LB exists (use NLB IPs, not ClusterIP DNS):

```yaml
dataguard:
  prereqs:
    enabled: true
  standbySources:
    - dbUniqueName: ORCLSTBY1
      host: "<standby1-nlb-ip>"
      tcpPort: 1521
      tcpsEnabled: false
    - dbUniqueName: ORCLSTBY2
      host: "<standby2-nlb-ip>"
      tcpPort: 1521
      tcpsEnabled: false
    - dbUniqueName: ORCLSTBY3
      host: "<standby3-nlb-ip>"
      tcpPort: 1521
      tcpsEnabled: false
```

## 3. Copy TDE wallet

Always refresh from the **live** primary right before creating standbys (stale zip → `ORA-28374`):

```bash
PRIMARY_POD=$(kubectl get pods -l app=sidb-sample -o jsonpath='{.items[0].metadata.name}')
rm -rf /tmp/standby-wallet /tmp/standby-wallet.zip
mkdir -p /tmp/standby-wallet/tde
kubectl cp -n <ns> \
  "${PRIMARY_POD}:/opt/oracle/oradata/ORCLPRD/tdewallet/tde/ewallet.p12" \
  /tmp/standby-wallet/tde/ewallet.p12 -c sidb-sample
(cd /tmp/standby-wallet && zip -qr /tmp/standby-wallet.zip tde)

kubectl create secret generic tde-wallet-secret \
  --from-file=standby-wallet.zip=/tmp/standby-wallet.zip \
  --from-literal=tde_wallet_pwd='<tde-wallet-password>' \
  --dry-run=client -o yaml | kubectl apply -f -
```

Copy the same Secret into Cluster 2 before Mumbai standbys.

## 4. Cluster 1 standby1 (same cluster)

```bash
kubectl apply -f config/samples/sidb/singleinstancedatabase_multi_standby1_same_cluster_lb.yaml
```

Standby uses `primarySource` (not `standbySources`), for example:

```yaml
primarySource:
  details:
    host: "10.0.2.181"
    port: 1521
    sid: ORCLPRD
# or: connectString: "10.0.2.181:1521/ORCLPRD"
```

Wait for `DATABASE IS READY TO USE!` / PHYSICAL_STANDBY.

## 5. Cluster 2 standbys (sequential)

**Create one Mumbai standby at a time.** Parallel duplicates against the same primary can drop mid-restore (`ORA-03135` / `ORA-19849`).

### Critical: LB IP before duplicate

For cross-cluster standbys the controller sets:

```text
SVC_HOST=<standby-nlb-ip>
STANDBY_HOSTNAME=<standby-nlb-ip>
CROSS_CLUSTER_STANDBY=true
```

If the pod is created **before** the LB has an EXTERNAL-IP, `SVC_HOST` falls back to the Service name → ClusterIP → primary RMAN fails with:

```text
ORA-12170 ... host 10.96.x.x port 1521
```

Fix: wait for LB EXTERNAL-IP, then delete **pod + PVC** (keep LB Service) so the pod is recreated with `SVC_HOST=<LB-IP>`.

```bash
# Cluster 2 — standby2 first
kubectl apply -f config/samples/sidb/singleinstancedatabase_multi_standby2_cross_cluster_lb.yaml
kubectl get svc sidb-standby2-lb -w   # wait for EXTERNAL-IP
# verify env after Running:
kubectl exec <pod> -c sidb-standby2 -- printenv SVC_HOST CROSS_CLUSTER_STANDBY

# After standby2 is READY / PHYSICAL_STANDBY:
kubectl apply -f config/samples/sidb/singleinstancedatabase_multi_standby3_cross_cluster_lb.yaml
```

NLB annotations (validated):

```yaml
externalTrafficPolicy: Local
annotations:
  oci.oraclecloud.com/load-balancer-type: "nlb"
  oci-network-load-balancer.oraclecloud.com/is-preserve-source: "false"
  oci-network-load-balancer.oraclecloud.com/internal: "true"
  oci-network-load-balancer.oraclecloud.com/subnet: "<mumbai-subnet-ocid>"
```

## 6. Validate connectivity

```bash
# From each standby pod / primary pod using NLB IPs on :1521
sqlplus "sys/<pwd>@<primary-nlb>:1521/ORCLPRD as sysdba"
sqlplus "sys/<pwd>@<standbyN-nlb>:1521/ORCLSTBYn as sysdba"
```

Expect PHYSICAL_STANDBY / READ ONLY WITH APPLY and MRP running (FAL may warn until Broker/prereqs finish).

## 7. Generate and apply DataguardBroker

Generate a four-member Broker with **NLB :1521** endpoints (not NodePorts) using the sample renderer in multi-member mode. It unions `renderedBrokerSpec` from each SIDB, discovers each LB EXTERNAL-IP, and sets `sourceRef` / `localRef` on the apply cluster:

```bash
# From repository root. Replace kubeconfig paths and Service names as needed.
bash config/samples/sidb/render-dg-broker-from-status.sh sidb \
  --broker-name sidb-multi-dg \
  --namespace default \
  --kubeconfig ash=/path/to/cluster1.kubeconfig \
  --kubeconfig mum=/path/to/cluster2.kubeconfig \
  --member ash:sidb-sample:lb=sidb-sample-lb \
  --member ash:sidb-standby1:lb=sidb-standby1-lb \
  --member mum:sidb-standby2:lb=sidb-standby2-lb \
  --member mum:sidb-standby3:lb=sidb-standby3-lb \
  --source-ref mum:sidb-standby3 \
  > dataguardbroker-generated.yaml
```

Review the generated members, then apply in **Cluster 2**:

```bash
kubectl apply -f dataguardbroker-generated.yaml
# Or edit hosts in the reviewed sample template:
# kubectl apply -f config/samples/sidb/dataguardbroker_multi_standby_lb.yaml
```

Expected members:

```text
ORCLPRD   -> <primary-nlb>:1521
ORCLSTBY1 -> <standby1-nlb>:1521
ORCLSTBY2 -> <standby2-nlb>:1521  (+ localRef)
ORCLSTBY3 -> <standby3-nlb>:1521  (+ localRef)
```

Gate: runner shows Configuration Status `SUCCESS` with all four members.

## 8. Switchover rotation

Pause operators during role changes if they cancel MRP / rewrite Broker files.

```text
DGMGRL> CONNECT /@ORCLPRD
DGMGRL> SHOW CONFIGURATION;
DGMGRL> SWITCHOVER TO ORCLSTBY1;
DGMGRL> SWITCHOVER TO ORCLSTBY2;
DGMGRL> SWITCHOVER TO ORCLSTBY3;
DGMGRL> SWITCHOVER TO ORCLPRD;
```

Transient `ORA-16809` / lag warnings mid-rotation can clear after MRP catch-up. Client NLB endpoints are **role-specific** (no floating VIP).

## 9. ORA-16854 apply lag warning

Broker may show `WARNING` / `ORA-16854` even when SQL lag is `0` and MRP is applying. Mitigate before switchover if needed:

```text
EDIT CONFIGURATION SET PROPERTY OperationTimeout='120';
EDIT CONFIGURATION SET PROPERTY CommunicationTimeout='300';
EDIT DATABASE ORCLSTBY1 SET STATE='APPLY-OFF';
! sleep 12
EDIT DATABASE ORCLSTBY1 SET STATE='APPLY-ON';
EDIT DATABASE ORCLSTBY1 SET PROPERTY ApplyLagThreshold='0';
EDIT DATABASE ORCLSTBY2 SET PROPERTY ApplyLagThreshold='0';
EDIT DATABASE ORCLSTBY3 SET PROPERTY ApplyLagThreshold='0';
SHOW CONFIGURATION;
```

Planned switchover can still succeed when members are otherwise healthy; clear WARNING when possible.

## 10. FSFO caveats

Same practical limits as NodePort multi-standby:

- Observer may start (`sidb-multi-dg-observer`)
- Automatic FSFO often fails/suspends under listener-stop, transport-off, iptables, or pod-restart races (`ORA-16818`)
- Prefer **manual Broker failover** for predictable DR drills
- Scale operators to `0` before destructive FSFO tests
- After failover: reinstate disabled members (`ORA-16661`), set a new FSFO target (`ORA-16844`)

## Troubleshooting

### A) NLB EXTERNAL-IP stuck `<pending>`

```text
is-preserve-source cannot be set when externalTrafficPolicy is Cluster
```

Use `Local` + `is-preserve-source: "false"`. Delete/recreate the LB Service if needed.

### B) RMAN `ORA-12170` to `10.96.x.x`

Pod created before LB IP → `SVC_HOST=<service-name>`. Recreate pod+PVC after LB IP exists; confirm `SVC_HOST=<nlb-ip>`.

### C) RMAN `ORA-03135` / `ORA-19849` mid datafile restore

Usually parallel duplicates. Finish one Mumbai standby before starting the next; refresh TDE wallet from live primary.

### D) `ORA-28374` typed master key not found

Stale `standby-wallet.zip`. Re-export `ewallet.p12` from current primary and update the Secret.

### E) Operator Unhealthy but DB READY

Often missing FAL / archive dest until `standbySources` + Broker configure transport. Check `v$archive_dest` and MRP/`v$dataguard_stats`.

## Related files

- Single LB path: [`DATAGUARD_CROSS_CLUSTER_LB.md`](./DATAGUARD_CROSS_CLUSTER_LB.md)
- Multi CMAN: [`DATAGUARD_MULTI_STANDBY_CMAN.md`](./DATAGUARD_MULTI_STANDBY_CMAN.md)
- Multi TCPS NodePort: [`DATAGUARD_MULTI_STANDBY_TCPS_NODEPORT.md`](./DATAGUARD_MULTI_STANDBY_TCPS_NODEPORT.md)
