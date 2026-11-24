# Multi-Standby Data Guard over TCP+TCPS NodePort (1 primary + 3 standbys)

This runbook covers a validated multi-standby topology with **both TCP and TCPS NodePorts** (no CMAN):

| Member | Cluster | TCP NodePort | TCPS NodePort |
|---|---|---|---|
| `ORCLPRD` / `sidb-primary` | Cluster 1 | `32331` | `32484` |
| `ORCLSTBY1` / `sidb-standby1` | Cluster 1 | `30281` | `30284` |
| `ORCLSTBY2` / `sidb-standby2` | Cluster 2 | `30282` | `30285` |
| `ORCLSTBY3` / `sidb-standby3` | Cluster 2 | `30283` | `30286` |

Recommended order:

1. Issue TCPS certificates/secrets (cert-manager)
2. Create Cluster 1 primary + standby1
3. Create Cluster 2 standbys through primary TCP NodePort (`connectString`) + `tcpsPort`
4. Validate TCP and TCPS SQL\*Plus connectivity
5. Copy Broker TCPS client-wallet Secrets into the Broker cluster
6. Apply a four-member DataguardBroker with TCP+TCPS endpoints
7. Test switchover rotation; treat FSFO as best-effort (same caveats as TCP multi-standby)

Related:

- Direct TCP-only SIDB NodePort Data Guard is intentionally not documented; use CMAN or LoadBalancer for TCP-only deployments.
- TCPS cert-manager: [`tcps-cert-manager/README.md`](../tcps-cert-manager/README.md)
- Single-standby cross-cluster deployments should use the CMAN or LoadBalancer runbook.

## Sample files (in this repository)

| Role | Sample |
|---|---|
| Primary | [`config/samples/sidb/singleinstancedatabase_multi_primary_tcps_nodeport.yaml`](../../../config/samples/sidb/singleinstancedatabase_multi_primary_tcps_nodeport.yaml) |
| Standby1 | [`config/samples/sidb/singleinstancedatabase_multi_standby1_same_cluster_tcps_nodeport.yaml`](../../../config/samples/sidb/singleinstancedatabase_multi_standby1_same_cluster_tcps_nodeport.yaml) |
| Standby2 | [`config/samples/sidb/singleinstancedatabase_multi_standby2_cross_cluster_tcps_nodeport.yaml`](../../../config/samples/sidb/singleinstancedatabase_multi_standby2_cross_cluster_tcps_nodeport.yaml) |
| Standby3 | [`config/samples/sidb/singleinstancedatabase_multi_standby3_cross_cluster_tcps_nodeport.yaml`](../../../config/samples/sidb/singleinstancedatabase_multi_standby3_cross_cluster_tcps_nodeport.yaml) |
| Four-member Broker | [`config/samples/sidb/dataguardbroker_multi_standby_tcps_nodeport.yaml`](../../../config/samples/sidb/dataguardbroker_multi_standby_tcps_nodeport.yaml) |
| Broker renderer | [`config/samples/sidb/render-dg-broker-from-status.sh`](../../../config/samples/sidb/render-dg-broker-from-status.sh) |

## Topology

```text
Cluster 1                                              Cluster 2
-----------------------------------------              -----------------------------------------
sidb-primary
  TCP  :<node>:32331/ORCLPRD
  TCPS :<node>:32484/ORCLPRD
sidb-standby1
  TCP  :<node>:30281/ORCLSTBY1
  TCPS :<node>:30284/ORCLSTBY1

        ^                                                 sidb-standby2 :30282/:30285
        |                                                 sidb-standby3 :30283/:30286
        +-------- TCP bootstrap + TCPS redo/Broker -------+

DataguardBroker sidb-multi-tcps-dg (Cluster 2)
  members each have TCP + TCPS endpoints and tcps.clientWalletSecret
```

Bootstrap note: standby `primarySource.connectString` uses the **TCP** primary NodePort for DBCA/RMAN. `primarySource.tcpsPort` supplies the primary TCPS NodePort used for TCPS aliases / redo paths. Primary `standbySources[].tcpsPort` must match each standby’s TCPS NodePort.

## Prerequisites

- Operator 2.3 in both clusters
- cert-manager and TCPS certificates/secrets ready in each cluster ([`tcps-cert-manager/README.md`](../tcps-cert-manager/README.md))
- Matching admin Secret and TDE wallet Secret in both clusters
- Matching DG/TCPS-capable SIDB image tags on all four databases
- Network path for TCP and TCPS NodePorts both ways

## 1. Deploy operator 2.3

```bash
kubectl apply -f oracle-database-operator.yaml
```

## 2. Generate and validate TCPS certificates

Follow [`tcps-cert-manager/README.md`](../tcps-cert-manager/README.md) (or your org’s Confluence TCPS certificate guide) so each SIDB has a Ready TLS Secret, for example:

```bash
kubectl -n <ns> get certificate sidb-primary-tcps
kubectl -n <ns> get secret sidb-primary-tcps-tls
# likewise for standby TLS secrets used by standby manifests
```

## 3. Cluster 1 primary + standby1

```bash
# Cluster 1
kubectl apply -f config/samples/sidb/singleinstancedatabase_multi_primary_tcps_nodeport.yaml
# Wait Healthy / PRIMARY, export TDE wallet into tde-wallet-secret

kubectl apply -f config/samples/sidb/singleinstancedatabase_multi_standby1_same_cluster_tcps_nodeport.yaml
```

Confirm Services expose both ports:

```bash
kubectl get svc sidb-primary-nodeport sidb-standby1-nodeport -o wide
# expect 1521:<tcpNodePort> and 2484:<tcpsNodePort>
```

Validate TCPS aliases / SQL\*Plus:

```bash
# On standby1: ORCLPRDTCPS -> primary host:32484
# On primary:  ORCLSTBY1TCPS -> standby1 host:30284
sqlplus "sys/<pwd>@ORCLPRDTCPS as sysdba"
sqlplus "sys/<pwd>@ORCLSTBY1TCPS as sysdba"
```

## 4. Cluster 2 standbys

Copy TDE wallet Secret into Cluster 2, ensure standby TLS Secrets exist, then:

```bash
# Cluster 2
kubectl apply -f config/samples/sidb/singleinstancedatabase_multi_standby2_cross_cluster_tcps_nodeport.yaml
kubectl apply -f config/samples/sidb/singleinstancedatabase_multi_standby3_cross_cluster_tcps_nodeport.yaml
```

Both use:

```text
primarySource.connectString: <cluster1-node-ip>:32331/ORCLPRD
primarySource.tcpsPort: 32484
```

Wait for Healthy PHYSICAL_STANDBY and MRP.

## 5. Copy Broker TCPS client wallets into the Broker cluster

Broker members reference per-database `tcps.clientWalletSecret` values (for example `sidb-primary-dg-client-wallet`). Secrets created in Cluster 1 must be copied into Cluster 2 before applying Broker:

```bash
# Example: copy Cluster 1 wallets into Cluster 2
for SECRET in sidb-primary-dg-client-wallet sidb-standby1-dg-client-wallet; do
  kubectl --context=<cluster1> -n <ns> get secret "$SECRET" -o json |
    jq 'del(.metadata.uid,.metadata.resourceVersion,.metadata.creationTimestamp,.metadata.managedFields,.metadata.ownerReferences)' |
    kubectl --context=<cluster2> -n <ns> apply -f -
done

kubectl --context=<cluster2> get secret \
  sidb-primary-dg-client-wallet \
  sidb-standby1-dg-client-wallet \
  sidb-standby2-dg-client-wallet \
  sidb-standby3-dg-client-wallet
```

Create/generate Cluster 2 standby wallet secrets locally if the controller has already produced them there.

## 6. Apply four-member DataguardBroker

Generate the four-member Broker with NodePort hosts (TCP and TCPS endpoint ports are preserved from each SIDB `renderedBrokerSpec` when you pass `host=<node-ip>` without a port). For TCP-only port overrides use `host=<ip>:<port>` on a TCP topology, or edit the generated YAML for per-protocol NodePorts:

```bash
# From repository root. host=<node-ip> keeps per-endpoint ports from status (TCP + TCPS).
bash config/samples/sidb/render-dg-broker-from-status.sh sidb \
  --broker-name sidb-multi-tcps-dg \
  --namespace default \
  --kubeconfig c1=/path/to/cluster1.kubeconfig \
  --kubeconfig c2=/path/to/cluster2.kubeconfig \
  --member c1:sidb-sample:host=<cluster1-node-ip> \
  --member c1:sidb-standby1:host=<cluster1-node-ip> \
  --member c2:sidb-standby2:host=<cluster2-node-ip> \
  --member c2:sidb-standby3:host=<cluster2-node-ip> \
  --source-ref c2:sidb-standby3 \
  > dataguardbroker-generated.yaml

# Review TCP/TCPS NodePorts, then apply in Cluster 2:
kubectl apply -f dataguardbroker-generated.yaml
# Or edit placeholders in the reviewed template:
# kubectl apply -f config/samples/sidb/dataguardbroker_multi_standby_tcps_nodeport.yaml
```

Each member should include:

- TCP endpoint (`1521` NodePort)
- TCPS endpoint (`2484` NodePort)
- `tcps.enabled: true` + `clientWalletSecret`

Watch the runner until Configuration Status is `SUCCESS`.

Optional: set Broker DG connect identifiers to TCPS aliases after configuration exists:

```text
EDIT DATABASE ORCLPRD   SET PROPERTY DGConnectIdentifier='ORCLPRDTCPS';
EDIT DATABASE ORCLSTBY1 SET PROPERTY DGConnectIdentifier='ORCLSTBY1TCPS';
EDIT DATABASE ORCLSTBY2 SET PROPERTY DGConnectIdentifier='ORCLSTBY2TCPS';
EDIT DATABASE ORCLSTBY3 SET PROPERTY DGConnectIdentifier='ORCLSTBY3TCPS';
```

## 7. Validate

```text
DGMGRL> CONNECT /@ORCLPRDTCPS
DGMGRL> SHOW CONFIGURATION;
DGMGRL> VALIDATE DATABASE VERBOSE ORCLPRD;
DGMGRL> VALIDATE DATABASE VERBOSE ORCLSTBY1;
DGMGRL> VALIDATE DATABASE VERBOSE ORCLSTBY2;
DGMGRL> VALIDATE DATABASE VERBOSE ORCLSTBY3;
```

Lab-validated pass criteria:

- Configuration Status: `SUCCESS`
- Ready for Switchover: Yes
- Apply Running, transport Success, lag 0, no gap

Known gaps (do not block planned switchover in lab, but document them):

- Standby-to-standby network validation may fail when database sessions cannot resolve `ORCLSTBYnTCPS` (`TNS_ADMIN` empty in those sessions)
- Static connect identifier validation under OS auth may warn; use password/wallet auth for a definitive check
- Prefer TCP NodePorts for static `*_DGMGRL` identifiers unless TCPS static services are explicitly validated

## 8. Switchover rotation

Pause operators during role changes if needed.

```text
DGMGRL> SWITCHOVER TO ORCLSTBY1;
DGMGRL> SWITCHOVER TO ORCLSTBY2;
DGMGRL> SWITCHOVER TO ORCLSTBY3;
DGMGRL> SWITCHOVER TO ORCLPRD;
```

Validated: planned switchover / switchback across members succeeds when Configuration Status is clean.

**Client endpoint note:** TCP/TCPS NodePorts are role-specific. After role change, clients must use the new primary’s NodePorts (not the old `ORCLPRD` ports).

## 9. Manual failover / reinstate

Same pattern as TCP multi-standby:

1. Confirm target has latest redo
2. Crash/shutdown primary (do not delete SIDB CR/PVC)
3. `FAILOVER TO <standby> IMMEDIATE;`
4. Expect `ORA-16661` on disabled members until reinstated
5. `startup mount` former primary, restore readiness lock if required, then `REINSTATE DATABASE ...` / `ENABLE DATABASE ...`
6. Optional switchback

## 10. FSFO caveats

Same practical FSFO limitations apply to this NodePort topology:

- Observer may start (`sidb-multi-*-observer`)
- Listener-stop / transport-off / iptables / pod-restart races often fail to trigger clean automatic FSFO (`ORA-16818` timer resets)
- Prefer **manual Broker failover** for predictable DR drills
- Scale operators to `0` before destructive FSFO tests

## Troubleshooting

### A) Standby bootstrap cannot reach primary TCPS

Confirm `primarySource.connectString` is TCP NodePort for DBCA, and `tcpsPort` matches primary TCPS NodePort. Validate `ORCLPRDTCPS` alias on the standby.

### B) Broker runner cannot connect over TCPS

Confirm each `clientWalletSecret` exists in the Broker namespace and endpoints use TCPS NodePorts (`32484` / `30284` / `30285` / `30286`), not container port `2484` on the node IP.

### C) TCPS aliases missing between standbys

Expected lab gap when `TNS_ADMIN` is empty in database sessions. Broker runner wallets/aliases are separate from in-database session resolution. Fix session `TNS_ADMIN` / tnsnames if standby-to-standby TCPS validation is required.

### D) Renderer only produces two Broker members

Use multi-member mode (`--member` / `--source-ref`) so all four databases are united in one Broker YAML, or start from [`dataguardbroker_multi_standby_tcps_nodeport.yaml`](../../../config/samples/sidb/dataguardbroker_multi_standby_tcps_nodeport.yaml).

## Related files

- Multi-standby CMAN: [`DATAGUARD_MULTI_STANDBY_CMAN.md`](./DATAGUARD_MULTI_STANDBY_CMAN.md)
- TCPS cert-manager: [`tcps-cert-manager/README.md`](../tcps-cert-manager/README.md)
- General SIDB Data Guard: [`README.md`](../README.md#data-guard)
