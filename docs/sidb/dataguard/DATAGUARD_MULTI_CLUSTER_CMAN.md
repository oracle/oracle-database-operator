# Multi-Cluster SIDB Data Guard with CMAN

This runbook documents the tested two-cluster topology using Oracle Connection Manager (CMAN) as the Data Guard and Broker network path:

| Cluster | Databases | CMAN endpoint |
| --- | --- | --- |
| Cluster 1 | `ORCLPRD`, `ORCLSTBY1` | `10.0.2.126:30189` |
| Cluster 2 | `ORCLSTBY2`, `ORCLSTBY3` | `10.2.1.131:30189` |

The tested order is important:

1. Create the Cluster 1 primary and same-cluster standby using ClusterIP.
2. Deploy and validate Cluster 1 CMAN.
3. Create Cluster 2 standbys through the Cluster 1 CMAN NodePort.
4. Deploy and validate Cluster 2 CMAN.
5. Configure Data Guard Broker using the two CMAN endpoints.
6. Validate all Broker paths, then test planned switchovers and switchback.

Replace the example IP addresses, image names, namespaces, and NodePorts with values for the target environment. The tested files were named `multi-sidb-create-v4-tcp-primary-cman.yaml`, `multi-sidb-create-v4-tcp-standby-cman.yaml`, `multi-sidb-standby-mumbai-1.yaml`, `multi-sidb-standby-mumbai-2.yaml`, `multi-cman-sidb-primary-standby-nodeport.yaml`, `multi-cman-sidb-standby3-nodeport.yaml`, and `multi-dataguardbroker-cman-nodeport.yaml`.

## Network design

- The primary and `ORCLSTBY1` use Kubernetes ClusterIP connectivity during same-cluster standby duplication.
- Cluster 1 CMAN exposes the primary and `ORCLSTBY1` services locally and provides the cross-cluster path used by `ORCLSTBY2` and `ORCLSTBY3` during duplication.
- Cluster 2 CMAN exposes `ORCLSTBY2` and `ORCLSTBY3` locally to the Broker runner and to database members that need to reach them.
- The Broker topology uses the CMAN NodePort for every member, not the individual database NodePorts.
- Each CMAN configuration must route both the normal database service and the static Broker service (`*_DGMGRL`).
- The same CMAN NodePort number can be used in both clusters because the cluster addresses differ.

## Prerequisites

1. Deploy the operator and CRDs in both clusters. Use the published 2.3 operator image `container-registry.oracle.com/database/operator:latest`.
2. Ensure the operator watches `default` in both clusters.
3. Ensure the ConnectionManager CRD and RBAC are installed.
4. Create matching `db-admin-secret` and TDE wallet Secrets in both clusters.
5. Ensure the database and CMAN images are pullable from both clusters.
6. Ensure TCP port `30189` is reachable between the clusters and from the Broker runner.

The databases should use archive logging, force logging, and flashback. Set `flashBack: true` on all standbys that will be used for switchovers, FSFO, or reinstatement.

## Create the primary and same-cluster standby

Create the primary SIDB with Data Guard prerequisite generation enabled. The primary's `standbySources` must point at reachable CMAN routes for the remote standbys. The tested lab used a primary CMAN route on `30189` and an additional route on `30190` during initial setup:

```yaml
spec:
  sid: ORCLPRD
  pdbName: APPPDB1
  createAs: primary
  archiveLog: true
  flashBack: true
  forceLog: true
  dataguard:
    prereqs:
      enabled: true
    standbySources:
      - dbUniqueName: ORCLSTBY1
        host: 10.2.1.131
        tcpPort: 30189
        tcpsEnabled: false
      - dbUniqueName: ORCLSTBY2
        host: 10.2.1.131
        tcpPort: 30190
        tcpsEnabled: false
```

If one CMAN routes all services, use the actual single CMAN endpoint consistently. Do not copy `30190` unless that second route exists and is configured for the requested standby service.

Expose the primary database NodePort for diagnostics, but do not use it as the Broker endpoint in this CMAN topology:

```yaml
services:
  endpoints:
    - name: nodeport
      type: NodePort
      tcp:
        enabled: true
        nodePort: 32330
```

Create `ORCLSTBY1` using the primary ClusterIP:

```yaml
primarySource:
  connectString: "sidb-primary.default.svc.cluster.local:1521/ORCLPRD"
```

Configure the standby with the shared TDE wallet Secret, `flashBack: true`, and:

```yaml
dataguard:
  prereqs:
    enabled: true
    standbyRedoSize: 1G
```

Wait for both SIDBs to become `Healthy`, and verify `ORCLSTBY1` reports `PHYSICAL STANDBY` with managed recovery running.

## Transfer the TDE wallet

Export the primary wallet and create the standby wallet Secret in both clusters before creating the remote standbys:

```sh
PRIMARY_POD=$(kubectl -n default get pod -l app=sidb-primary \
  -o jsonpath='{.items[0].metadata.name}')

rm -rf /tmp/standby-wallet /tmp/standby-wallet.zip
mkdir -p /tmp/standby-wallet/tde
kubectl -n default cp \
  "$PRIMARY_POD:/opt/oracle/oradata/ORCLPRD/tdewallet/tde/ewallet.p12" \
  /tmp/standby-wallet/tde/ewallet.p12
(cd /tmp/standby-wallet && zip -qr /tmp/standby-wallet.zip tde)
unzip -t /tmp/standby-wallet.zip
```

Apply the Secret in Cluster 1 and Cluster 2:

```sh
kubectl -n default create secret generic tde-wallet-secret \
  --from-file=standby-wallet.zip=/tmp/standby-wallet.zip \
  --from-literal=tde_wallet_pwd='<TDE_WALLET_PASSWORD>' \
  --dry-run=client -o yaml | kubectl apply -f -
```

## Deploy and validate Cluster 1 CMAN

Create a `ConnectionManager` (`kind: ConnectionManager`, `network.oracle.com/v4`) with a file-mode `cman.ora` ConfigMap. Do not use `TrafficManager` with `spec.type: cman` for new deployments. Use a CMAN runtime image such as `container-registry.oracle.com/database/client-cman:<tag>`. The ConfigMap must contain:

1. Local aliases for `ORCLPRD`, `ORCLPRD_DGMGRL`, `ORCLSTBY1`, and `ORCLSTBY1_DGMGRL`.
2. A CMAN configuration block for the CMAN service.
3. Rules accepting each of those four service names.

The local aliases use the database ClusterIP services:

```text
ORCLPRD =
(description=
  (address=(protocol=tcp)(host=sidb-primary.default.svc.cluster.local)(port=1521))
  (connect_data=(service_name=ORCLPRD))
)

ORCLPRD_DGMGRL =
(description=
  (address=(protocol=tcp)(host=sidb-primary.default.svc.cluster.local)(port=1521))
  (connect_data=(service_name=ORCLPRD_DGMGRL))
)
```

Add equivalent aliases for `sidb-standby1`. The CMAN rule list must include both normal and static services:

```text
(rule=(src=*)(dst=*)(srv=ORCLPRD)(act=accept) ...)
(rule=(src=*)(dst=*)(srv=ORCLPRD_DGMGRL)(act=accept) ...)
(rule=(src=*)(dst=*)(srv=ORCLSTBY1)(act=accept) ...)
(rule=(src=*)(dst=*)(srv=ORCLSTBY1_DGMGRL)(act=accept) ...)
```

Expose CMAN through a pinned external NodePort:

```yaml
service:
  internal:
    enabled: true
    ports:
      - name: cman
        port: 1521
        targetPort: 1521
  external:
    enabled: true
    serviceType: NodePort
    externalTrafficPolicy: Cluster
    ports:
      - name: cman
        port: 1521
        targetPort: 1521
        nodePort: 30189
```

Validate before creating remote standbys:

```sh
kubectl get connectionmanager cman-sidb \
  -o custom-columns='NAME:.metadata.name,STATUS:.status.status,READY:.status.readyReplicas'
kubectl get svc cman-sidb-ext -o wide
kubectl get endpoints cman-sidb-ext -o wide
kubectl exec deploy/cman-sidb -- lsnrctl services
```

The CMAN resource should be `Healthy`, have one ready replica, and show ready `cmgw`/`cmon` services. Confirm the generated `tnsnames.ora` contains the four expected aliases.

## Create the Cluster 2 standbys through Cluster 1 CMAN

Create `ORCLSTBY2` and `ORCLSTBY3` in Cluster 2. Their `primarySource` must use the Cluster 1 CMAN NodePort, not the primary database NodePort:

```yaml
primarySource:
  connectString: "10.0.2.126:30189/ORCLPRD"
```

Use unique database NodePorts for diagnostics, for example `30282` and `30283`, and enable Data Guard prerequisites on both standbys. Wait for both to become `Healthy` and verify local managed recovery.

From each Cluster 2 standby pod, validate the primary through CMAN:

```sh
sqlplus -s "sys/${ADMIN_PWD}@//10.0.2.126:30189/ORCLPRD as sysdba"
```

The query must report `ORCLPRD`, `PRIMARY`, and `READ WRITE`.

## Deploy and validate Cluster 2 CMAN

Create a second file-mode CMAN ConfigMap and ConnectionManager in Cluster 2. Its local aliases and rules should cover:

```text
ORCLSTBY2
ORCLSTBY2_DGMGRL
ORCLSTBY3
ORCLSTBY3_DGMGRL
```

The aliases target:

```text
sidb-standby2.default.svc.cluster.local:1521
sidb-standby3.default.svc.cluster.local:1521
```

Expose Cluster 2 CMAN on `10.2.1.131:30189`. Validate its ConnectionManager, endpoint, listener services, generated aliases, and both standby connections:

```sh
sqlplus -s "sys/${ADMIN_PWD}@//cman-sidb.default.svc.cluster.local:1521/ORCLSTBY2 as sysdba"
sqlplus -s "sys/${ADMIN_PWD}@//cman-sidb.default.svc.cluster.local:1521/ORCLSTBY3 as sysdba"
```

## Configure Broker using CMAN endpoints

Create the `DataguardBroker` in the cluster where the Broker runner and the selected CMAN endpoint are reachable. All four topology members use CMAN NodePort `30189`:

```yaml
spec:
  execution:
    authWallet:
      enabled: true
    image: <DATABASE_IMAGE>
  topology:
    defaults:
      adminSecretRef:
        secretName: db-admin-secret
        secretKey: oracle_pwd
    members:
      - dbUniqueName: ORCLPRD
        name: sidb-primary
        role: PRIMARY
        endpoints:
          - name: tcp
            host: 10.0.2.126
            port: 30189
            protocol: TCP
            serviceName: ORCLPRD
      - dbUniqueName: ORCLSTBY1
        name: sidb-standby1
        role: PHYSICAL_STANDBY
        endpoints:
          - name: tcp
            host: 10.0.2.126
            port: 30189
            protocol: TCP
            serviceName: ORCLSTBY1
      - dbUniqueName: ORCLSTBY2
        name: sidb-standby2
        role: PHYSICAL_STANDBY
        endpoints:
          - name: tcp
            host: 10.2.1.131
            port: 30189
            protocol: TCP
            serviceName: ORCLSTBY2
      - dbUniqueName: ORCLSTBY3
        name: sidb-standby3
        role: PHYSICAL_STANDBY
        endpoints:
          - name: tcp
            host: 10.2.1.131
            port: 30189
            protocol: TCP
            serviceName: ORCLSTBY3
    sourceKind: SingleInstanceDatabase
    sourceRef:
      apiVersion: database.oracle.com/v4
      kind: SingleInstanceDatabase
      name: sidb-standby3
      namespace: default
```

The `*_DGMGRL` services must resolve through the same CMAN endpoints. Validate the Broker before any role operation:

```text
show configuration;
validate database verbose ORCLPRD;
validate database verbose ORCLSTBY1;
validate database verbose ORCLSTBY2;
validate database verbose ORCLSTBY3;
validate network configuration for all;
validate static connect identifier for all;
```

Proceed only when the configuration is `SUCCESS`, apply is running, transport status is successful, and static connect validation succeeds.

## Switchover and switchback

Use planned switchovers to rotate through all members:

```text
switchover to ORCLSTBY1;
switchover to ORCLSTBY2;
switchover to ORCLSTBY3;
switchover to ORCLPRD;
```

After every operation, verify:

```text
show configuration;
show database verbose <CURRENT_PRIMARY>;
```

Also check the SIDB status roles and CMAN-routed SQL*Plus connection to the new primary. Keep the CMAN service aliases stable across role changes; only the database roles change.

## FSFO and reinstatement

Select a flashback-enabled standby with zero apply lag. Set the target and validate all CMAN paths before enabling FSFO:

```text
edit database ORCLPRD set property FastStartFailoverTarget='ORCLSTBY3';
show fast_start failover;
validate network configuration for all;
validate static connect identifier for all;
```

Confirm the Broker runner and observer are running. The observer may be created by the operator after the Broker resource reconciles, so verify it before enabling FSFO:

```sh
kubectl get pods -n default -l app=sidb-multi-dg
```

Then enable FSFO:

```text
enable fast_start failover;
show fast_start failover;
```

Preserve SIDB CRs, PVCs, Services, and both CMAN resources during outage testing. After automatic failover, inspect Broker status and observer logs. If the old primary is disabled, start it mounted, restore the SIDB readiness state as needed, and run:

```text
reinstate database ORCLPRD;
show database verbose ORCLPRD;
show configuration;
```

After all members synchronize, disable or retarget FSFO intentionally and perform a planned switchback if required.

## Troubleshooting gates

- `ORA-12514`: the CMAN rule or local alias is missing the requested service name.
- `ORA-12541`: the CMAN NodePort, CMAN listener, or endpoint path is unavailable.
- Static connect validation failure: the corresponding `*_DGMGRL` alias or CMAN rule is missing, or the database is not reachable while mounted.
- `ORA-16809`: inspect each member with `show database verbose`; do not proceed to the next switchover until warnings are understood.
- `ORA-16661`: the failed member needs Broker reinstatement after failover.
- `ORA-16818`/`ORA-16857`: FSFO is suspended or the target cannot be confirmed within the configured lag limit.
- If a CMAN ConfigMap is changed, wait for the ConnectionManager rollout and repeat alias, listener, and SQL*Plus validation before touching Broker state.
