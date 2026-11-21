# Multi-Cluster SIDB Data Guard with TCPS NodePorts

This runbook records the tested four-member TCPS Data Guard deployment through
planned switchover. It uses direct NodePorts for Data Guard and Broker
connectivity (no CMAN).

## Plan

Deploy these manifests:

```text
$WORK_DIR
$WORK_DIR
$WORK_DIR
$WORK_DIR
$WORK_DIR
```

### Ports

| Member | Cluster | Node | TCP NodePort | TCPS NodePort |
| --- | --- | --- | ---: | ---: |
| `ORCLPRD` / `sidb-primary` | Ashburn | `10.0.2.240` | `32331` | `32484` |
| `ORCLSTBY1` / `sidb-standby1` | Ashburn | `10.0.2.240` | `30281` | `30284` |
| `ORCLSTBY2` / `sidb-standby2` | Mumbai | `10.2.1.131` | `30282` | `30285` |
| `ORCLSTBY3` / `sidb-standby3` | Mumbai | `10.2.1.131` | `30283` | `30286` |

### Images

| Component | Image |
| --- | --- |
| Operator | `container-registry.oracle.com/database/operator:latest` |
| SIDB / Broker runner | `container-registry.oracle.com/database/enterprise_ru:latest` |

### Kubeconfigs

```bash
ASHBURN=/path/to/primary-cluster.kubeconfig
MUMBAI=/path/to/standby-cluster.kubeconfig
```

### Mandatory order

1. Deploy the 2.3 operator in both clusters.
2. Create matching `db-admin-secret` in both clusters.
3. Create TCPS TLS Secrets before any TCPS SIDB.
4. Deploy Ashburn primary; wait for `Healthy` / `PRIMARY`.
5. Export primary TDE wallet; create `tde-wallet-secret` in Ashburn and Mumbai.
6. Deploy Ashburn standby1; wait for `Healthy` / `PHYSICAL_STANDBY`.
7. Deploy Mumbai standby2 and standby3; wait for both healthy with apply running.
8. Copy Ashburn `*-dg-client-wallet` Secrets to Mumbai; confirm all four wallets exist in Mumbai.
9. Apply Broker in Mumbai; wait for configuration `SUCCESS`.
10. Validate, then run planned switchovers.

### TCPS rules that must stay in YAML

- Set explicit `tcpsPort` on IP/NodePort peers:
  - primary `spec.dataguard.standbySources[].tcpsPort`
  - standby `spec.primarySource.tcpsPort`
- Operator-owned TNS aliases come from those fields. Do not hand-edit database `tnsnames.ora` for the customer path.
- Mumbai `db-admin-secret` password must match Ashburn.
- NodePorts are role-specific. After switchover, clients must use the new primary endpoint.

## Deploy 2.3 operator

Use operator image `container-registry.oracle.com/database/operator:latest`
from the 2.3 release (or a build that contains the TCPS NodePort /
`tcpsPort` support under test).

```bash
kubectl apply -f oracle-database-operator.yaml
```

Apply in both Ashburn and Mumbai. Confirm the controller Deployment is ready:

```bash
kubectl --kubeconfig="$ASHBURN" -n oracle-database-operator-system get deploy,pods
kubectl --kubeconfig="$MUMBAI" -n oracle-database-operator-system get deploy,pods
```

## Generate certificates and Secrets for TLS/TCPS

Refer to:

- Confluence: `TCPS - TLS Certificate Management`
- Repo helper: [`tcps-cert-manager/README.md`](../tcps-cert-manager/README.md)

Example cert-manager flow:

```bash
PRIMARY_CTX="$(KUBECONFIG="$ASHBURN" kubectl config current-context)"
STANDBY_CTX="$(KUBECONFIG="$MUMBAI" kubectl config current-context)"

export KUBECONFIG="$ASHBURN:$MUMBAI"
export PRIMARY_CTX STANDBY_CTX NS=default
export PRIMARY_DNS_NAMES='sidb-primary,sidb-primary.default.svc.cluster.local'
export STANDBY_DNS_NAMES='sidb-standby1,sidb-standby1.default.svc.cluster.local,sidb-standby2,sidb-standby2.default.svc.cluster.local,sidb-standby3,sidb-standby3.default.svc.cluster.local'

cd $WORK_DIR
./docs/sidb/tcps-cert-manager/run-all.sh
```

### Validate certificates and Secrets

```bash
kubectl -n default get certificate sidb-primary-tcps
# READY=True, SECRET=sidb-primary-tcps-tls

kubectl -n default describe certificate sidb-primary-tcps
kubectl -n default get secret sidb-primary-tcps-tls
```

Expected Secret type `kubernetes.io/tls` with data keys `tls.crt`, `tls.key`, and `ca.crt`.

Also verify:

```bash
kubectl --kubeconfig="$ASHBURN" -n default get secret \
  db-admin-secret tde-wallet-secret sidb-primary-tcps-tls sidb-standby-tcps-tls

kubectl --kubeconfig="$MUMBAI" -n default get secret \
  db-admin-secret tde-wallet-secret sidb-standby-tcps-tls
```

## Setup SIDB in primary cluster (Ashburn)

```bash
kubectl --kubeconfig="$ASHBURN" apply -f \
  $WORK_DIR
```

Key primary fields:

```yaml
spec:
  sid: ORCLPRD
  createAs: primary
  security:
    tcps:
      enabled: true
      tlsSecret: sidb-primary-tcps-tls
  dataguard:
    prereqs:
      enabled: true
    standbySources:
      - dbUniqueName: ORCLSTBY1
        host: 10.0.2.240
        tcpPort: 30281
        tcpsEnabled: true
        tcpsPort: 30284
      - dbUniqueName: ORCLSTBY2
        host: 10.2.1.131
        tcpPort: 30282
        tcpsEnabled: true
        tcpsPort: 30285
      - dbUniqueName: ORCLSTBY3
        host: 10.2.1.131
        tcpPort: 30283
        tcpsEnabled: true
        tcpsPort: 30286
  image:
    pullFrom: container-registry.oracle.com/database/enterprise_ru:latest
  services:
    endpoints:
      - name: nodeport
        type: NodePort
        tcp:
          enabled: true
          nodePort: 32331
        tcps:
          enabled: true
          nodePort: 32484
  nodeSelector:
    kubernetes.io/hostname: 10.0.2.240
```

Wait until the primary pod is ready and logs show:

```text
DATABASE IS READY TO USE!
```

Confirm:

```bash
kubectl --kubeconfig="$ASHBURN" -n default get sidb sidb-primary
kubectl --kubeconfig="$ASHBURN" -n default get svc sidb-primary sidb-primary-nodeport
```

Expected NodePort Service includes `1521:32331/TCP` and `2484:32484/TCP`.

## Download TDE wallet from primary

```bash
export KUBECONFIG=/path/to/primary-cluster.kubeconfig
PRIMARY_POD="$(kubectl -n default get pod -l app=sidb-primary -o jsonpath='{.items[0].metadata.name}')"

rm -rf /tmp/standby-wallet /tmp/standby-wallet.zip
mkdir -p /tmp/standby-wallet/tde

kubectl -n default cp \
  "${PRIMARY_POD}:/opt/oracle/oradata/ORCLPRD/tdewallet/tde/ewallet.p12" \
  /tmp/standby-wallet/tde/ewallet.p12 \
  -c sidb-primary

cd /tmp/standby-wallet
zip -qr /tmp/standby-wallet.zip tde
unzip -t /tmp/standby-wallet.zip
ls -l /tmp/standby-wallet/tde/ewallet.p12
```

### Create TDE wallet Secret in Ashburn and Mumbai

```bash
# Ashburn
kubectl --kubeconfig="$ASHBURN" -n default delete secret tde-wallet-secret --ignore-not-found
kubectl --kubeconfig="$ASHBURN" -n default create secret generic tde-wallet-secret \
  --from-file=standby-wallet.zip=/tmp/standby-wallet.zip \
  --from-literal=tde_wallet_pwd='<TDE_WALLET_PASSWORD>'

# Mumbai
kubectl --kubeconfig="$MUMBAI" -n default delete secret tde-wallet-secret --ignore-not-found
kubectl --kubeconfig="$MUMBAI" -n default create secret generic tde-wallet-secret \
  --from-file=standby-wallet.zip=/tmp/standby-wallet.zip \
  --from-literal=tde_wallet_pwd='<TDE_WALLET_PASSWORD>'
```

## Deploy standby in same cluster (Ashburn)

```bash
kubectl --kubeconfig="$ASHBURN" apply -f \
  $WORK_DIR
```

Key standby1 fields:

```yaml
spec:
  sid: ORCLSTBY1
  createAs: standby
  flashBack: true
  primarySource:
    connectString: "10.0.2.240:32331/ORCLPRD"
    tcpsPort: 32484
  security:
    tcps:
      enabled: true
      tlsSecret: sidb-standby-tcps-tls
  services:
    endpoints:
      - name: nodeport
        type: NodePort
        tcp:
          enabled: true
          nodePort: 30281
        tcps:
          enabled: true
          nodePort: 30284
```

Watch DBCA:

```bash
kubectl --kubeconfig="$ASHBURN" -n default logs -f pod/<sidb-standby1-pod>
```

Expected progression includes `Duplicate_database`, then readiness and:

```text
DATABASE IS READY TO USE!
```

Confirm:

```bash
kubectl --kubeconfig="$ASHBURN" -n default get sidb
# sidb-primary  Healthy PRIMARY ... 10.0.2.240:32331/ORCLPRD ... 10.0.2.240:32484/ORCLPRD
# sidb-standby1 Healthy PHYSICAL_STANDBY ... 10.0.2.240:30281/ORCLSTBY1 ... 10.0.2.240:30284/ORCLSTBY1
```

## Deploy standbys in Mumbai

```bash
kubectl --kubeconfig="$MUMBAI" apply -f \
  $WORK_DIR

kubectl --kubeconfig="$MUMBAI" apply -f \
  $WORK_DIR
```

Both use:

```yaml
primarySource:
  connectString: "10.0.2.240:32331/ORCLPRD"
  tcpsPort: 32484
```

and NodePorts `30282/30285` for standby2, `30283/30286` for standby3.

Cross-cluster RMAN duplicate can remain at DBCA `46% Duplicate_database` for a long time. That is expected over WAN. Wait until both report `Healthy` / `PHYSICAL_STANDBY`.

Confirm Services:

```bash
kubectl --kubeconfig="$MUMBAI" -n default get svc sidb-standby2-nodeport sidb-standby3-nodeport -o wide
```

## Copy Data Guard client wallets to Mumbai

After all four SIDBs are healthy and have generated wallets:

```bash
ASHBURN=/path/to/primary-cluster.kubeconfig
MUMBAI=/path/to/standby-cluster.kubeconfig

for SECRET in sidb-primary-dg-client-wallet sidb-standby1-dg-client-wallet; do
  kubectl --kubeconfig="$ASHBURN" -n default get secret "$SECRET" -o json |
    jq 'del(.metadata.uid,
             .metadata.resourceVersion,
             .metadata.creationTimestamp,
             .metadata.managedFields,
             .metadata.ownerReferences)' |
    kubectl --kubeconfig="$MUMBAI" -n default apply -f -
done

kubectl --kubeconfig="$MUMBAI" -n default get secret \
  sidb-primary-dg-client-wallet \
  sidb-standby1-dg-client-wallet \
  sidb-standby2-dg-client-wallet \
  sidb-standby3-dg-client-wallet
```

Expected: all four Secrets present. Standby2/3 wallets are created in Mumbai by those SIDBs; only Ashburn wallets are copied.

## Configure DataguardBroker

Apply the Broker CR in Mumbai:

```bash
kubectl --kubeconfig="$MUMBAI" apply -f \
  $WORK_DIR
```

Key Broker fields from the tested manifest:

```yaml
metadata:
  name: sidb-multi-tcps-dg
spec:
  execution:
    authWallet:
      enabled: true
    image: container-registry.oracle.com/database/enterprise_ru:latest
  topology:
    defaults:
      adminSecretRef:
        secretName: db-admin-secret
        secretKey: oracle_pwd
    members:
      # ORCLPRD / ORCLSTBY1 / ORCLSTBY2 / ORCLSTBY3 with TCP+TCPS NodePorts
      # and tcps.clientWalletSecret per member
    sourceKind: SingleInstanceDatabase
    sourceRef:
      apiVersion: database.oracle.com/v4
      kind: SingleInstanceDatabase
      name: sidb-standby3
      namespace: default
```

Watch the runner:

```bash
kubectl --kubeconfig="$MUMBAI" -n default get pods
kubectl --kubeconfig="$MUMBAI" -n default logs -f pod/<sidb-multi-tcps-dg-runner-...>
```

Initial runner output may show `ORA-16532` on each member, then:

```text
Configuration "dg_config" created with primary database "orclprd"
Database "orclstby1" added
Database "orclstby2" added
Database "orclstby3" added
...
Configuration Status:
SUCCESS
```

## Broker validation

From the Broker runner:

```bash
export TNS_ADMIN=/opt/oracle/dg-net
dgmgrl /@ORCLPRDTCPS
```

```text
SHOW CONFIGURATION;
SHOW DATABASE VERBOSE ORCLPRD;
SHOW DATABASE VERBOSE ORCLSTBY1;
SHOW DATABASE VERBOSE ORCLSTBY2;
SHOW DATABASE VERBOSE ORCLSTBY3;
VALIDATE DATABASE ORCLSTBY1;
VALIDATE DATABASE ORCLSTBY2;
VALIDATE DATABASE ORCLSTBY3;
```

### Observed validation result from the tested lab

Passed:

- Broker configuration: `SUCCESS`
- Databases ready for switchover: `Yes`
- Apply state: running
- Transport: success
- Apply/transport lag: `0` seconds
- Gap status: no gap

Known gaps observed in lab validation (do not block basic planned switchover in this tested path, but record them):

- network validation between some standby members
- static connect identifier validation for all members
- some standby DB sessions cannot resolve peer `ORCLSTBYxTCPS` aliases when `TNS_ADMIN` is empty in those sessions

Operator-owned aliases on the primary / Broker runner remain the required path for Broker connect identifiers. Do not paper over that with manual database TNS edits during customer-like testing.

## Planned switchovers

Preserve SIDB pods and PVCs. Use DGMGRL from the Broker runner with the current primary TCPS alias.

### Switchover to standby1

```text
CONNECT /@ORCLPRDTCPS
SHOW CONFIGURATION;
SWITCHOVER TO ORCLSTBY1;
SHOW CONFIGURATION;
```

Tested result:

```text
Switchover succeeded, new primary is "orclstby1"
Switchover processing complete, broker ready.
```

Verify new primary:

```bash
STBY1_POD="$(kubectl --kubeconfig="$ASHBURN" -n default get pod -l app=sidb-standby1 -o jsonpath='{.items[0].metadata.name}')"
ADMIN_PWD="$(kubectl --kubeconfig="$ASHBURN" -n default get secret db-admin-secret -o jsonpath='{.data.oracle_pwd}' | base64 -d)"

kubectl --kubeconfig="$ASHBURN" -n default exec -i "$STBY1_POD" -c sidb-standby1 -- \
  sqlplus -s "sys/${ADMIN_PWD}@10.0.2.240:30281/ORCLSTBY1 as sysdba" <<'EOF'
select name, db_unique_name, database_role, open_mode from v$database;
exit
EOF
```

Expected:

```text
NAME      DB_UNIQUE_NAME  DATABASE_ROLE  OPEN_MODE
ORCLPRD   ORCLSTBY1       PRIMARY        READ WRITE
```

After this switchover, application/client endpoints for the current primary are
`10.0.2.240:30281` / `30284`. The old primary NodePorts `32331` / `32484` are
not transparent failover addresses.

### Optional continued rotation

If validating full rotation:

```text
CONNECT /@ORCLSTBY1TCPS
SWITCHOVER TO ORCLSTBY2;
SHOW CONFIGURATION;

CONNECT /@ORCLSTBY2TCPS
SWITCHOVER TO ORCLSTBY3;
SHOW CONFIGURATION;

CONNECT /@ORCLSTBY3TCPS
SWITCHOVER TO ORCLPRD;
SHOW CONFIGURATION;
```

After each switchover, confirm:

```bash
kubectl --kubeconfig="$ASHBURN" -n default get sidb \
  -o custom-columns='NAME:.metadata.name,STATUS:.status.status,ROLE:.status.role'
kubectl --kubeconfig="$MUMBAI" -n default get sidb \
  -o custom-columns='NAME:.metadata.name,STATUS:.status.status,ROLE:.status.role'
```

and DGMGRL:

```text
SHOW CONFIGURATION;
```

Final expected configuration after switchback to primary:

```text
ORCLPRD   - Primary database
  orclstby1 - Physical standby database
  orclstby2 - Physical standby database
  orclstby3 - Physical standby database
Configuration Status: SUCCESS
```

## Operational cautions

- Keep YAML `tcpsPort` values aligned with Service NodePorts so operator alias generation stays correct.
- Do not manually rewrite database or runner TNS aliases during customer-like validation.
- Mumbai standby creation can take much longer than Ashburn standby1 because of WAN RMAN duplicate.
- Capture `SHOW CONFIGURATION`, `SHOW DATABASE VERBOSE`, runner logs, and SIDB status before deleting resources if Broker reports `ERROR` after a role change.
- This runbook stops after planned switchover validation. FSFO and reinstatement are covered in the TCP NodePort / CMAN Data Guard runbooks.
