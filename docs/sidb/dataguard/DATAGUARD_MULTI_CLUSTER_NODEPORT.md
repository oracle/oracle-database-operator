# Multi-Cluster SIDB Data Guard with Direct NodePorts

This runbook documents the tested four-database topology without CMAN and without TCPS:

| Cluster | Database | Role | Reachable endpoint |
| --- | --- | --- | --- |
| Cluster 1 | `ORCLPRD` / `sidb-primary` | Primary | `10.0.2.126:32330/ORCLPRD` |
| Cluster 1 | `ORCLSTBY1` / `sidb-standby1` | Physical standby | `10.0.2.126:30281/ORCLSTBY1` |
| Cluster 2 | `ORCLSTBY2` / `sidb-standby2` | Physical standby | `10.2.1.131:30282/ORCLSTBY2` |
| Cluster 2 | `ORCLSTBY3` / `sidb-standby3` | Physical standby | `10.2.1.131:30283/ORCLSTBY3` |

The endpoint addresses in this example are lab values. Replace them with addresses reachable from every database pod and the Broker runner pod.

## Why the topology uses two endpoint types

- The same-cluster standby uses the primary ClusterIP service (`sidb-primary.default.svc.cluster.local:1521`) for initial duplication. Cluster DNS is the simplest and most stable path inside one Kubernetes cluster.
- Cross-cluster standbys cannot use the primary ClusterIP. They use the primary NodePort (`10.0.2.126:32330`) for initial duplication.
- The Broker topology uses NodePorts for all members. Broker DMON traffic must be reachable in both directions between every database and the Broker runner, including the static `*_DGMGRL` services.
- NodePorts are role-specific, not a transparent client failover address. After a switchover, clients must use the NodePort belonging to the new primary.

## Prerequisites

1. Deploy the operator and CRDs using the operator release manifest.
2. Ensure the operator watches `default` in both clusters.
3. Create `db-admin-secret` in each cluster with the same `oracle_pwd` key.
4. Create the primary TDE secret with `tde_wallet_pwd`.
5. Ensure the database image and `oci-bv` storage class are available in both clusters.
6. Ensure the listed node addresses and NodePorts are reachable from both clusters.

The primary must have `archiveLog`, `forceLog`, and `flashBack` enabled. Every standby must have `flashBack: true` when it will be used for FSFO or later reinstatement.

## Create the primary

Create the primary SIDB with Data Guard prerequisite generation enabled. Its `standbySources` entries describe the endpoints that the primary uses to ship redo. For this direct-NodePort topology, set `tcpsEnabled: false` and set `tcpPort` to each standby's NodePort.

The essential shape is:

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
        host: 10.0.2.126
        tcpPort: 30281
        tcpsEnabled: false
      - dbUniqueName: ORCLSTBY2
        host: 10.2.1.131
        tcpPort: 30282
        tcpsEnabled: false
      - dbUniqueName: ORCLSTBY3
        host: 10.2.1.131
        tcpPort: 30283
        tcpsEnabled: false
```

Expose the primary's database listener through a pinned NodePort:

```yaml
services:
  endpoints:
    - name: nodeport
      type: NodePort
      tcp:
        enabled: true
        nodePort: 32330
```

Wait for `Healthy` and `PRIMARY` before creating standbys.

## Transfer the TDE wallet

For 23ai and later, the standby creation flow requires the primary wallet zip and its password. Export the wallet from the primary pod, validate the archive, and create or update the standby wallet Secret in each target cluster.

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

Create the Secret in Cluster 1 and Cluster 2. Use the same wallet password as the primary:

```sh
kubectl -n default create secret generic tde-wallet-secret \
  --from-file=standby-wallet.zip=/tmp/standby-wallet.zip \
  --from-literal=tde_wallet_pwd='<TDE_WALLET_PASSWORD>' \
  --dry-run=client -o yaml | kubectl apply -f -
```

## Create the standbys

For the same-cluster standby, use the primary ClusterIP:

```yaml
primarySource:
  connectString: "sidb-primary.default.svc.cluster.local:1521/ORCLPRD"
```

For each cross-cluster standby, use the primary NodePort:

```yaml
primarySource:
  connectString: "10.0.2.126:32330/ORCLPRD"
```

Every standby should include:

```yaml
createAs: standby
flashBack: true
dataguard:
  prereqs:
    enabled: true
    standbyRedoSize: 1G
security:
  secrets:
    admin:
      secretName: db-admin-secret
      secretKey: oracle_pwd
      keepSecret: true
    tde:
      secretName: tde-wallet-secret
      secretKey: tde_wallet_pwd
      walletZipFileKey: standby-wallet.zip
      walletRoot: /opt/oracle/oradata/dbconfig/<DB_UNIQUE_NAME>/.wallet
```

Expose each standby with its own pinned NodePort:

```yaml
services:
  endpoints:
    - name: nodeport
      type: NodePort
      tcp:
        enabled: true
        nodePort: <STANDBY_NODEPORT>
```

Do not create the Broker resource until every standby is `Healthy`, reports `PHYSICAL_STANDBY`, and has redo apply running.

## Validate database connectivity

Check status and endpoints in both clusters:

```sh
kubectl get singleinstancedatabase -n default \
  -o custom-columns='NAME:.metadata.name,STATUS:.status.status,ROLE:.status.role,CONNECT:.status.connectString,DATAFILES:.status.datafilesCreated'

kubectl get svc -n default
```

From a pod in each cluster, test direct SQL*Plus connections to every NodePort:

```sh
sqlplus -s "sys/${ADMIN_PWD}@10.0.2.126:32330/ORCLPRD as sysdba"
sqlplus -s "sys/${ADMIN_PWD}@10.0.2.126:30281/ORCLSTBY1 as sysdba"
sqlplus -s "sys/${ADMIN_PWD}@10.2.1.131:30282/ORCLSTBY2 as sysdba"
sqlplus -s "sys/${ADMIN_PWD}@10.2.1.131:30283/ORCLSTBY3 as sysdba"
```

The standby query must report `PHYSICAL STANDBY` and normally `READ ONLY WITH APPLY`. Also verify `MRP0` and zero or converging transport/apply lag from `v$managed_standby` and `v$archived_log`.

## Configure the Data Guard Broker

Create one `DataguardBroker` resource in the cluster that will host the Broker runner. Use direct NodePort endpoints for all members:

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
            port: 32330
            protocol: TCP
            serviceName: ORCLPRD
      # Add ORCLSTBY1, ORCLSTBY2, and ORCLSTBY3 using their NodePorts.
    sourceKind: SingleInstanceDatabase
    sourceRef:
      apiVersion: database.oracle.com/v4
      kind: SingleInstanceDatabase
      name: sidb-primary
      namespace: default
```

The `name`, `dbUniqueName`, `role`, `host`, `port`, `protocol`, and `serviceName` values must be correct. The Broker runner must be able to reach every endpoint. After applying, wait for `Configuration Status: SUCCESS` from DGMGRL and `Healthy` from the Broker resource.

Validate from the Broker runner:

```sh
DGMGRL> show configuration
DGMGRL> validate database verbose ORCLPRD
DGMGRL> validate database verbose ORCLSTBY1
DGMGRL> validate database verbose ORCLSTBY2
DGMGRL> validate database verbose ORCLSTBY3
DGMGRL> validate network configuration for all;
DGMGRL> validate static connect identifier for all;
```

Do not proceed to switchover or FSFO while static connect validation fails.

## Test planned switchovers and switchback

Before each planned switchover:

```text
show configuration;
show database verbose <CURRENT_PRIMARY>;
show database verbose <TARGET_STANDBY>;
```

The target must be healthy and synchronized. Run:

```text
switchover to ORCLSTBY1;
show configuration;
```

Repeat for `ORCLSTBY2`, `ORCLSTBY3`, and finally `ORCLPRD` to test the complete rotation. After every operation, verify:

```sh
kubectl get singleinstancedatabase -n default \
  -o custom-columns='NAME:.metadata.name,STATUS:.status.status,ROLE:.status.role,CONNECT:.status.connectString'
```

The new primary's NodePort becomes the active client endpoint. The former primary's old NodePort is not automatically a role-independent client endpoint.

## Configure and test FSFO

Use a standby with flashback enabled and zero apply lag as the target. From the current primary connection:

```text
edit database ORCLPRD set property FastStartFailoverTarget='ORCLSTBY2';
show fast_start failover;
```

Before enabling FSFO, verify:

```text
show configuration;
validate database verbose ORCLPRD;
validate database verbose ORCLSTBY2;
validate network configuration for all;
validate static connect identifier for all;
show fast_start failover;
```

The Broker operator creates and maintains an observer pod when FSFO is enabled or when it observes FSFO enabled through DGMGRL. Confirm the observer is running before enabling FSFO:

```sh
kubectl get pod -n default -l app=sidb-multi-dg
```

Then enable FSFO:

```text
enable fast_start failover;
show fast_start failover;
```

Record the active target, observer name, lag limit, and FSFO mode. For an outage test, preserve the SIDB CR, PVC, and Service. After the test, inspect `show configuration` and `show fast_start failover` before restoring anything.

If FSFO promotes the target, the old primary generally requires reinstatement:

```text
reinstate database ORCLPRD;
show database verbose ORCLPRD;
show configuration;
```

If the old primary is managed by SIDB, let the pod return mounted and restore its readiness state before asking Broker to reinstate it. Do not recreate the database or delete its PVC as part of a normal FSFO test.

After all members are synchronized, optionally switch back with a planned switchover and confirm Broker `SUCCESS`.

## Operational cautions

- Keep the operator running during normal Broker reconciliation, but avoid changing Data Guard prerequisite settings while FSFO is actively managing the configuration unless the deployed operator version contains the FSFO reconciliation guard.
- Treat `ORA-16532`, `ORA-16573`, `ORA-16661`, `ORA-16818`, and `ORA-16857` as Broker-state or recovery signals; capture `show configuration`, `show database verbose`, observer logs, and SIDB controller logs before changing resources.
- Preserve database pods, PVCs, and Services during switchover/FSFO tests. The Broker operation changes database roles; Kubernetes resource recreation is a separate recovery action.
