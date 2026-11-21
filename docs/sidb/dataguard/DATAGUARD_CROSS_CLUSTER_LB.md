# Cross-Cluster Data Guard over TCP with NLB (Primary + Standby + DataguardBroker)

Use this document for a **two-cluster** TCP Data Guard topology where:

- **Primary** runs in cluster A
- **Standby** runs in cluster B
- Both databases expose an **OCI Network Load Balancer (NLB)** on TCP `1521`
- **DataguardBroker** is applied in the **standby** cluster and uses NLB
  IPs/DNS for both members

For the recommended deployment, each region has its own private DNS zone and
ExternalDNS is installed in each OKE cluster. The `external-dns.alpha.kubernetes.io/hostname`
annotation on each SIDB LoadBalancer Service then publishes the hostname and
points it to the current NLB address. Configure cross-region DNS resolution so
the primary resolves the standby hostname and the standby resolves the primary
hostname. This removes the need to manually create DNS records or copy NLB IPs.
NLB IPs remain a troubleshooting fallback only.

## Contents

| Step | Task | Link |
| --- | --- | --- |
| 1 | Prepare requirements | [Before You Begin](#before-you-begin) |
| 2 | Set cluster access and values | [Set the example variables](#set-the-example-variables) |
| 3 | Check deployment gates | [Required Before Deployment](#required-before-deployment) |
| 4 | Edit the manifests | [Review Manifest Requirements](#review-manifest-requirements) |
| 5 | Deploy the operator | [Deploy the operator](#1-deploy-the-operator-in-both-clusters) |
| 6 | Create namespaces and Secrets | [Create namespaces and Secrets](#2-create-namespaces-and-secrets) |
| 7 | Create the primary | [Create the primary SIDB](#3-create-the-primary-sidb) |
| 8 | Copy the TDE wallet | [Export the TDE wallet](#4-export-the-tde-wallet-to-the-standby-cluster) |
| 9 | Create the standby | [Create the standby SIDB](#5-create-the-standby-sidb) |
| 10 | Verify the pair | [Validate before Broker](#6-validate-primary-and-standby-before-broker) |
| 11 | Generate Broker manifest | [Generate DataguardBroker](#7-generate-dataguardbroker) |
| 12 | Apply and verify Broker | [Apply, verify, and run DGMGRL](#8-apply-verify-and-run-dgmgrl) |
| 13 | Optional role change | [Switchover](#9-switchover) |
| 14 | Optional FSFO validation | [FSFO and reinstate validation](#10-fsfo-and-reinstate-validation) |
| — | Troubleshoot | [Troubleshooting](#troubleshooting) |
| — | Example reference | [Example: Ashburn–Mumbai Flannel profile](#example-ashburnmumbai-flannel-profile) |

## Before You Begin

Complete every item in **both** clusters unless a row says otherwise.

| Requirement | What to prepare | How to confirm |
| --- | --- | --- |
| Matching OraOperator | Same operator image, CRDs, and RBAC on both clusters | **[Deploy and verify the operator in both clusters](#1-deploy-the-operator-in-both-clusters)** |
| Watched namespace | Operator watches `$NS` | **[Create the namespaces and Secrets](#2-create-namespaces-and-secrets)**; verify the watched namespace in operator logs |
| Storage | StorageClass that can provision SIDB PVCs (example: `oci-bv`) | **[Review the StorageClass in the SIDB manifests](#review-manifest-requirements)** |
| Networking | Peered VCNs / routes so workers reach the peer NLB on TCP `1521`; prefer stable DNS (ExternalDNS / private DNS) for NLB hostnames | **[Test NLB traffic policy and TCP reachability](#nlb-traffic-policy-short)** |
| Admin password | Same SYS/admin password in both clusters | **[Create the admin Secret](#2-create-namespaces-and-secrets)**; confirm key `oracle_pwd` |
| TDE password | Password for the TDE wallet on the primary | **[Create the TDE Secret](#2-create-namespaces-and-secrets)**; confirm key `tde_wallet_pwd` on primary before create |
| Database image | Same image tag on primary and standby; supports `CROSS_CLUSTER_STANDBY=true` rewrite for overlay CNIs | **[Review the image in both SIDB manifests](#review-manifest-requirements)** |
| Image pull | OCR or private registry Secret when required | **[Create the image-pull Secret](#2-create-namespaces-and-secrets)** in `$NS` on both clusters |
| Tools | `kubectl`, `jq`, `zip`, `unzip`, Bash 4+; `yq` recommended | **[Set the example variables and helper commands](#set-the-example-variables)** |

### NLB traffic policy (short)

| Pattern | Typical `externalTrafficPolicy` | `is-preserve-source` |
| --- | --- | --- |
| VCN-native / peered (common default) | `Local` | `"false"` |
| Overlay CNI (for example Flannel) when remote cannot hit Local VIP | `Cluster` | **omit** (OCI rejects Cluster + preserve-source) |

The connectivity test is performed after the standby NLB is created in
[step 5](#5-create-the-standby-sidb). If it fails, see
[NLB connectivity fails with `Local`](#c-nlb-connectivity-fails-with-local).

## Set the example variables

This section tells you exactly what to provide and what the runbook discovers
later. The shell variables do **not** substitute values into YAML automatically;
use them as a checklist while editing the three sample manifests.

### 1. Provide these values before starting

You need access to both Kubernetes clusters, the same administrator password for
both databases, a TDE wallet password, and the OCI subnet OCID used by each
cluster's LoadBalancer Service. Choose names and values, or keep the examples:

```sh
export PRIMARY_CTX=<primary-kube-context>
export STANDBY_CTX=<standby-kube-context>

export NS=default
export PRIMARY_DB=sidb-sample
export STANDBY_DB=sidb-standby
export PRIMARY_SID=ORCLPRD
export STANDBY_SID=ORCLSTBY
export DGB=${STANDBY_DB}-dg

export PRIMARY_SUBNET_OCID=<primary-oke-svclb-subnet-ocid>
export STANDBY_SUBNET_OCID=<standby-oke-svclb-subnet-ocid>

export ADMIN_SECRET=db-admin-secret
export TDE_SECRET=tde-wallet-secret
export IMAGE_PULL_SECRET=ocr-reg-cred
export DB_IMAGE=container-registry.oracle.com/database/enterprise_ru:latest
export STORAGE_CLASS=oci-bv
export STORAGE_SIZE=100Gi
```

`PRIMARY_CTX` and `STANDBY_CTX` must identify different clusters. If you do not
use kubeconfig files, replace the explicit `--context` options below with
`KUBECONFIG=/path/to/primary.kubeconfig` or
`KUBECONFIG=/path/to/standby.kubeconfig` as appropriate. Do not set `PRIMARY_NLB` or
`STANDBY_NLB` yet; those values are discovered after the corresponding
LoadBalancer Services are created.

### 2. Confirm both cluster contexts

```sh
kubectl --context "$PRIMARY_CTX" config current-context
kubectl --context "$STANDBY_CTX" config current-context
kubectl --context "$PRIMARY_CTX" get nodes
kubectl --context "$STANDBY_CTX" get nodes
```

### Sample files

The following files are edited or applied later in the runbook. Their values
are explained in [Review Manifest Requirements](#review-manifest-requirements).

| Role | Sample |
| --- | --- |
| Primary SIDB | [`singleinstancedatabase_primary_cross_cluster_lb.yaml`](../../../config/samples/sidb/singleinstancedatabase_primary_cross_cluster_lb.yaml) |
| Standby SIDB | [`singleinstancedatabase_standby_cross_cluster_lb.yaml`](../../../config/samples/sidb/singleinstancedatabase_standby_cross_cluster_lb.yaml) |
| Broker template | [`dataguardbroker_cross_cluster_lb.yaml`](../../../config/samples/sidb/dataguardbroker_cross_cluster_lb.yaml) |
| Broker renderer | [`render-cross-cluster-dataguardbroker-lb.sh`](../../../config/samples/sidb/render-cross-cluster-dataguardbroker-lb.sh) |

## Required Before Deployment

> **Important:** Complete every action in the **Complete before applying** column before applying the related manifest.

| Requirement | Example value | Complete before applying |
| --- | --- | --- |
| Operator in both clusters | Same image/CRD/RBAC | [1. Deploy the operator in both clusters](#1-deploy-the-operator-in-both-clusters) |
| Namespace + admin/TDE Secrets | `$NS`, `$ADMIN_SECRET`, `$TDE_SECRET` | [2. Create namespaces and Secrets](#2-create-namespaces-and-secrets) |
| Bidirectional NLB reachability on TCP `1521` | `$PRIMARY_NLB`, `$STANDBY_NLB` | Confirm before standby create |
| Primary Healthy | `$PRIMARY_DB` status `Healthy` / `PRIMARY` | [3. Create the primary SIDB](#3-create-the-primary-sidb) |
| Current primary TDE wallet zip in standby | `standby-wallet.zip` in `$TDE_SECRET` on standby | [4. Export the TDE wallet to the standby cluster](#4-export-the-tde-wallet-to-the-standby-cluster) |
| Standby Healthy / PHYSICAL STANDBY | `$STANDBY_DB` | [5. Create the standby SIDB](#5-create-the-standby-sidb) |
| Current NLB hosts in Broker YAML | `$PRIMARY_NLB`, `$STANDBY_NLB` | [7. Generate DataguardBroker](#7-generate-dataguardbroker) |

## Review Manifest Requirements

> The samples use placeholders. Replace them with the example values below (or your chosen equivalents) before apply. If you change a value, update every dependent field.

Primary SIDB — [`singleinstancedatabase_primary_cross_cluster_lb.yaml`](../../../config/samples/sidb/singleinstancedatabase_primary_cross_cluster_lb.yaml)

| Setting | Example value | Required relationship |
| --- | --- | --- |
| `metadata.name` | `sidb-sample` | Matches `$PRIMARY_DB` |
| `metadata.namespace` | `default` | Matches `$NS` |
| `sid` | `ORCLPRD` | Matches `$PRIMARY_SID`; Broker `dbUniqueName` |
| `pdbName` | `APPPDB1` | PDB on the primary |
| `createAs` | `primary` | Primary role |
| `dataguard.prereqs.enabled` | `true` | Enables Data Guard prerequisites |
| `dataguard.standbySources[].dbUniqueName` | `ORCLSTBY` | Must equal standby `sid` |
| `dataguard.standbySources[].host` | `$STANDBY_NLB` | Set after standby NLB exists (or update then) |
| Admin / TDE Secrets | `db-admin-secret` / `tde-wallet-secret` | Present in primary `$NS` |
| External Service type / port | `LoadBalancer` / `1521` | Publishes `$PRIMARY_NLB` |

Standby SIDB — [`singleinstancedatabase_standby_cross_cluster_lb.yaml`](../../../config/samples/sidb/singleinstancedatabase_standby_cross_cluster_lb.yaml)

| Setting | Example value | Required relationship |
| --- | --- | --- |
| `metadata.name` | `sidb-standby` | Matches `$STANDBY_DB` |
| `metadata.namespace` | `default` | Matches `$NS` |
| `sid` | `ORCLSTBY` | Matches `$STANDBY_SID` |
| `createAs` | `standby` | Physical standby role |
| `primarySource.connectString` | `$PRIMARY_NLB:1521/ORCLPRD` | Reachable from the standby Pod |
| `dataguard.prereqs.enabled` | `true` | Required for Duplicate / FAL setup |
| Admin Secret | `db-admin-secret` / `oracle_pwd` | Same password as primary |
| TDE Secret | `tde-wallet-secret` with `standby-wallet.zip` | Current primary wallet zip |
| External Service type / port | `LoadBalancer` / `1521` | Publishes `$STANDBY_NLB` |

DataguardBroker — generate with the renderer, or edit [`dataguardbroker_cross_cluster_lb.yaml`](../../../config/samples/sidb/dataguardbroker_cross_cluster_lb.yaml)

| Setting | Example value | Required relationship |
| --- | --- | --- |
| `metadata.name` | `sidb-standby-dg` | Matches `$DGB`; apply only after both SIDBs are Healthy |
| `metadata.namespace` | `default` | Standby-cluster `$NS` |
| `topology.sourceRef.name` | `sidb-standby` | Local standby SIDB |
| Primary member host / service | `$PRIMARY_NLB:1521` / `ORCLPRD` | Current primary NLB |
| Standby member host / service | `$STANDBY_NLB:1521` / `ORCLSTBY` | Current standby NLB |
| Admin Secret | `db-admin-secret` / `oracle_pwd` | Same Secret as both SIDBs |

## 1. Deploy the operator in both clusters

Use the **same** `oracle-database-operator.yaml` file for both clusters. The
first command targets the primary cluster; the second command targets the
standby cluster. Do not apply both commands to the same cluster.

### Primary cluster (cluster A)

```sh
kubectl --context "$PRIMARY_CTX" apply -f oracle-database-operator.yaml
kubectl --context "$PRIMARY_CTX" -n oracle-database-operator-system get deploy,pods
```

Wait until the primary operator deployment is Ready.

### Standby cluster (cluster B)

```sh
kubectl --context "$STANDBY_CTX" apply -f oracle-database-operator.yaml
kubectl --context "$STANDBY_CTX" -n oracle-database-operator-system get deploy,pods
```

Wait until the standby operator deployment is Ready. Continue only after the
operator is Ready in **both** clusters.


## 2. Create namespaces and Secrets

Create the application namespace in both clusters. Run the primary block while
connected to cluster A, then run the standby block while connected to cluster B.

### Primary cluster (cluster A)

```sh
kubectl --context "$PRIMARY_CTX" create namespace "$NS" --dry-run=client -o yaml | kubectl --context "$PRIMARY_CTX" apply -f -
```

### Standby cluster (cluster B)

```sh
kubectl --context "$STANDBY_CTX" create namespace "$NS" --dry-run=client -o yaml | kubectl --context "$STANDBY_CTX" apply -f -
```

Create the same admin Secret, with the same password, in both clusters.

### Admin Secret in the primary cluster

```sh
kubectl --context "$PRIMARY_CTX" -n "$NS" create secret generic "$ADMIN_SECRET" \
  --from-literal=oracle_pwd='<admin-password>' \
  --dry-run=client -o yaml | kubectl --context "$PRIMARY_CTX" apply -f -
```

### Admin Secret in the standby cluster

```sh
kubectl --context "$STANDBY_CTX" -n "$NS" create secret generic "$ADMIN_SECRET" \
  --from-literal=oracle_pwd='<admin-password>' \
  --dry-run=client -o yaml | kubectl --context "$STANDBY_CTX" apply -f -
```

Create the TDE password Secret in the primary cluster only. The standby TDE
Secret, including the wallet ZIP, is created in [step 4](#4-export-the-tde-wallet-to-the-standby-cluster)
after the primary database is Healthy.

### TDE password Secret in the primary cluster

```sh
kubectl --context "$PRIMARY_CTX" -n "$NS" create secret generic "$TDE_SECRET" \
  --from-literal=tde_wallet_pwd='<tde-wallet-password>' \
  --dry-run=client -o yaml | kubectl --context "$PRIMARY_CTX" apply -f -
```

If the database image requires registry authentication, create the image-pull
Secret separately in **both** `$NS` namespaces. The Secret name must be
`$IMAGE_PULL_SECRET`, and the same registry credentials must be valid in both
clusters.

## 3. Create the primary SIDB

1. Edit [`singleinstancedatabase_primary_cross_cluster_lb.yaml`](../../../config/samples/sidb/singleinstancedatabase_primary_cross_cluster_lb.yaml):
   set `metadata.namespace`, Secrets, image, storage, subnet OCID, and NLB
   annotations for the primary cluster.
2. You can leave `standbySources[].host` as a placeholder until the standby NLB
   exists; update it before Broker if the sample requires it.
3. Apply against the **primary** cluster:

```sh
kubectl --context "$PRIMARY_CTX" apply -f config/samples/sidb/singleinstancedatabase_primary_cross_cluster_lb.yaml

kubectl --context "$PRIMARY_CTX" wait --for=jsonpath='{.status.status}'=Healthy \
  "singleinstancedatabase/$PRIMARY_DB" -n "$NS" --timeout=60m

kubectl --context "$PRIMARY_CTX" -n "$NS" get sidb "$PRIMARY_DB" -o wide
kubectl --context "$PRIMARY_CTX" -n "$NS" get svc "${PRIMARY_DB}-lb" -o wide
```

4. Record the primary NLB:

```sh
export PRIMARY_NLB=$(kubectl --context "$PRIMARY_CTX" -n "$NS" get svc "${PRIMARY_DB}-lb" \
  -o jsonpath='{.status.loadBalancer.ingress[0].hostname}{.status.loadBalancer.ingress[0].ip}')
echo "PRIMARY_NLB=$PRIMARY_NLB"
```

Continue when the primary is Healthy / PRIMARY, the Pod shows
`DATABASE IS READY TO USE!`, and `$PRIMARY_NLB` is set.

## 4. Export the TDE wallet to the standby cluster

```sh
PRIMARY_POD=$(kubectl --context "$PRIMARY_CTX" -n "$NS" get pods -l "app=$PRIMARY_DB" \
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

For a **19c standby**, this Secret contains the wallet ZIP only. Do not add
`tde_wallet_pwd` or configure `security.secrets.tde.secretKey` on the standby;
the stock 19c image can fail when that password key is mounted. For a **26ai
standby only**, add `--from-literal=tde_wallet_pwd='<tde-wallet-password>'`
above and configure `security.secrets.tde.secretKey: tde_wallet_pwd` in the
standby manifest. The primary TDE Secret still contains `tde_wallet_pwd`.
Always use the wallet ZIP from the **current** primary. A stale wallet causes
`ORA-28374` after Duplicate networking works.

## 5. Create the standby SIDB

1. Edit [`singleinstancedatabase_standby_cross_cluster_lb.yaml`](../../../config/samples/sidb/singleinstancedatabase_standby_cross_cluster_lb.yaml):
   - `primarySource.connectString: "$PRIMARY_NLB:1521/$PRIMARY_SID"`
   - same image tag as the primary
   - standby subnet OCID / NLB annotations
   - Secrets and namespace
2. Apply against the **standby** cluster:

```sh
kubectl --context "$STANDBY_CTX" apply -f config/samples/sidb/singleinstancedatabase_standby_cross_cluster_lb.yaml

kubectl --context "$STANDBY_CTX" wait --for=jsonpath='{.status.status}'=Healthy \
  "singleinstancedatabase/$STANDBY_DB" -n "$NS" --timeout=90m

kubectl --context "$STANDBY_CTX" -n "$NS" get sidb "$STANDBY_DB" -o wide
kubectl --context "$STANDBY_CTX" -n "$NS" get svc "${STANDBY_DB}-lb" -o wide
```

3. Record the standby NLB and confirm primary→standby reachability:

```sh
export STANDBY_NLB=$(kubectl --context "$STANDBY_CTX" -n "$NS" get svc "${STANDBY_DB}-lb" \
  -o jsonpath='{.status.loadBalancer.ingress[0].hostname}{.status.loadBalancer.ingress[0].ip}')
echo "STANDBY_NLB=$STANDBY_NLB"

kubectl --context "$PRIMARY_CTX" -n "$NS" exec -it deploy/"$PRIMARY_DB" -- \
  bash -c "timeout 5 bash -c 'echo >/dev/tcp/${STANDBY_NLB}/1521' && echo OK || echo FAIL"
```

4. Update the primary `dataguard.standbySources[].host` to `$STANDBY_NLB` if it
   was still a placeholder, then re-apply the primary manifest.

Notes:

- Set `publishNotReadyAddresses: true` on the external endpoint when Duplicate
  must reach the auxiliary listener before the database is Ready.
- Expect DBCA to run through `Duplicate_database`, `Create_TDE_wallet`, and
  finish with `DATABASE IS READY TO USE!`.
- Standby Healthy / `PHYSICAL_STANDBY` alone is **not** full Data Guard. Apply
  Broker next.

## 6. Validate primary and standby before Broker

### Primary cluster

```sh
PRIMARY_POD=$(kubectl --context "$PRIMARY_CTX" -n "$NS" get pods -l "app=$PRIMARY_DB" \
  -o jsonpath='{.items[0].metadata.name}')

kubectl --context "$PRIMARY_CTX" -n "$NS" exec "${PRIMARY_POD}" -c "$PRIMARY_DB" -- bash -c 'sqlplus -s / as sysdba <<SQL
set lines 200 pages 100
select name, db_unique_name, database_role, open_mode, force_logging, log_mode from v\$database;
select thread#, sequence#, archived, status from v\$log order by thread#, sequence#;
SQL'

kubectl --context "$PRIMARY_CTX" -n "$NS" exec "${PRIMARY_POD}" -c "$PRIMARY_DB" -- \
  lsnrctl status LISTENER | grep -E "Service|${PRIMARY_SID}|DGMGRL|READY|1521"
```

Expect:

- `DATABASE_ROLE=PRIMARY`, `OPEN_MODE=READ WRITE`
- Listener services include `$PRIMARY_SID` and `${PRIMARY_SID}_DGMGRL`

### Standby cluster

```sh
STANDBY_POD=$(kubectl --context "$STANDBY_CTX" -n "$NS" get pods -l "app=$STANDBY_DB" \
  -o jsonpath='{.items[0].metadata.name}')

kubectl --context "$STANDBY_CTX" -n "$NS" exec "${STANDBY_POD}" -c "$STANDBY_DB" -- bash -c 'sqlplus -s / as sysdba <<SQL
set lines 220 pages 100
select name, db_unique_name, database_role, open_mode, force_logging, log_mode from v\$database;
select process, status, thread#, sequence# from v\$managed_standby order by process;
select name, value from v\$dataguard_stats where name in ('\''apply lag'\'','\''transport lag'\'');
SQL'

kubectl --context "$STANDBY_CTX" -n "$NS" exec "${STANDBY_POD}" -c "$STANDBY_DB" -- \
  lsnrctl status LISTENER | grep -E "Service|${STANDBY_SID}|DGMGRL|READY|1521"
```

Expect:

- `DATABASE_ROLE=PHYSICAL STANDBY`, `OPEN_MODE=READ ONLY WITH APPLY`
- Listener services include `$STANDBY_SID` and `${STANDBY_SID}_DGMGRL`
- With `dataguard.prereqs.enabled: true`, `fal_server` points at the primary NLB

## 7. Generate DataguardBroker

Prefer generating Broker YAML from current status with the repository wrapper.
It discovers the current NLB endpoints and keeps the cluster mapping explicit:

```sh
PRIMARY_KUBECONFIG=/path/to/primary.kubeconfig
STANDBY_KUBECONFIG=/path/to/standby.kubeconfig
OUT=dataguardbroker-generated.yaml

PRIMARY_DB="$PRIMARY_DB" \
STANDBY_DB="$STANDBY_DB" \
BROKER_NAME="$DGB" \
NS="$NS" \
PRIMARY_KUBECONFIG="$PRIMARY_KUBECONFIG" \
STANDBY_KUBECONFIG="$STANDBY_KUBECONFIG" \
OUT="$OUT" \
bash config/samples/sidb/render-cross-cluster-dataguardbroker-lb.sh
```

Review `$OUT` before proceeding. If the script reports that
`status.dataguard.readyForBroker` is not true, finish standby readiness and
prerequisites first, then run the renderer again.

Review the generated members:

- `$PRIMARY_SID` → `$PRIMARY_NLB:1521/$PRIMARY_SID`, role `PRIMARY`
- `$STANDBY_SID` → `$STANDBY_NLB:1521/$STANDBY_SID`, role `PHYSICAL_STANDBY`, with `localRef` to `$STANDBY_DB`

Do not reuse stale lab IPs.

## 8. Apply, verify, and run DGMGRL

Apply the generated manifest in the **standby** cluster:

```sh
kubectl --context "$STANDBY_CTX" apply -f "$OUT"
# or edit and apply config/samples/sidb/dataguardbroker_cross_cluster_lb.yaml

kubectl --context "$STANDBY_CTX" -n "$NS" get dataguardbroker "$DGB" -o wide
RUNNER=$(kubectl --context "$STANDBY_CTX" -n "$NS" get pods --no-headers | awk '/dg-runner/ {print $1; exit}')
kubectl --context "$STANDBY_CTX" -n "$NS" logs -f "${RUNNER}"
```

Expected Broker outcome:

```text
Configuration - dg_config
  Protection Mode: MaxPerformance
  Members:
  ORCLPRD  - Primary database
    orclstby - Physical standby database
Fast-Start Failover: Disabled
Configuration Status: SUCCESS
```

### Validate with DGMGRL

```sh
RUNNER=$(kubectl --context "$STANDBY_CTX" -n "$NS" get pods --no-headers | awk '/dg-runner/ {print $1; exit}')

kubectl --context "$STANDBY_CTX" -n "$NS" exec "${RUNNER}" -- bash -lc "
printf '%s\n' \
  'SHOW CONFIGURATION;' \
  'VALIDATE DATABASE VERBOSE ${PRIMARY_SID};' \
  'VALIDATE DATABASE VERBOSE ${STANDBY_SID};' \
| dgmgrl /@${PRIMARY_SID} as sysdba
"
```

Gate before switchover:

- Configuration Status: `SUCCESS`
- `Ready for Switchover: Yes` on primary and standby
- Gap Status: No Gap; transport/apply lag `0 seconds`
- `MRP0` is applying
- Static `*_DGMGRL` services exist on both listeners
- Broker connect identifiers use NLB hosts, not pod overlay IPs

## 9. Switchover

Optional. Pause operators during switchover if your image rewrites Broker files
or cancels MRP during role change:

```sh
kubectl --context "$PRIMARY_CTX" -n oracle-database-operator-system scale \
  deploy/oracle-database-operator-controller-manager --replicas=0
kubectl --context "$STANDBY_CTX" -n oracle-database-operator-system scale \
  deploy/oracle-database-operator-controller-manager --replicas=0
```

### Switchover to standby (`ORCLSTBY`)

```text
DGMGRL> connect /@ORCLPRD
DGMGRL> SHOW CONFIGURATION;
DGMGRL> SWITCHOVER TO ORCLSTBY;
```

Expected:

```text
Switchover succeeded, new primary is "orclstby"
```

### Switchback to original primary (`ORCLPRD`)

```text
DGMGRL> connect /@ORCLSTBY
DGMGRL> SHOW CONFIGURATION;
DGMGRL> SWITCHOVER TO ORCLPRD;
```

Expected:

```text
Switchover succeeded, new primary is "orclprd"
```

After success, confirm roles and lag, then scale operators back to `1` if desired.

## 10. FSFO and reinstate validation

Optional. Configure FSFO only after section 8 gates are clean:

```text
SHOW CONFIGURATION;
VALIDATE NETWORK CONFIGURATION FOR ALL;
VALIDATE STATIC CONNECT IDENTIFIER FOR ALL;
VALIDATE DATABASE VERBOSE ORCLPRD;
VALIDATE DATABASE VERBOSE ORCLSTBY;
SHOW FAST_START FAILOVER;
```

Require `Ready for Switchover: Yes`, `Ready for Failover: Yes`, no transport
gap, zero transport/apply lag, Flashback enabled on both databases, and a
reachable observer.

```text
EDIT DATABASE ORCLPRD SET PROPERTY FastStartFailoverTarget='ORCLSTBY';
ENABLE FAST_START FAILOVER;
SHOW FAST_START FAILOVER;
```

Tested outage sequence:

1. Stop the primary database or its listener using a controlled test method.
2. Confirm the observer promotes `ORCLSTBY` and Broker reports FSFO success.
3. Stop the outage loop before recovery work.
4. Start `ORCLPRD` mounted, never read/write, and run `REINSTATE DATABASE ORCLPRD`.
5. Confirm `ORCLPRD` is physical standby, apply is running, and Broker is `SUCCESS`.
6. Validate static identifiers again, then perform a planned switchback to `ORCLPRD`.

Treat a post-reinstate `ORA-01017` as a failed complete FSFO lifecycle until
password files and Broker wallet credentials are synchronized and the entire
sequence passes again.

## Troubleshooting

### A) `FAL: Error fetching gap sequence, no FAL server specified`

**Symptom** (standby alert log):

```text
FAL: Error fetching gap sequence, no FAL server specified
```

**When it is expected:** Briefly during Duplicate / recovery, before the standby
is Ready and before DataguardBroker finishes configuring transport.

**When it is a problem:** The message continues after the standby is Ready /
Healthy, or `fal_server` is still empty.

**Cause:** The standby prerequisite phase did not set `FAL_SERVER`, or
`primarySource.connectString` / the primary NLB DNS is stale or unreachable.

**Fix:**

1. Confirm `dataguard.prereqs.enabled: true` on the standby (and primary sample
   when used).
2. Confirm `primarySource.connectString` uses the **current** `$PRIMARY_NLB`
   (`host:1521/$PRIMARY_SID`).
3. On the standby:

   ```sh
   STANDBY_POD=$(kubectl --context "$STANDBY_CTX" -n "$NS" get pods -l "app=$STANDBY_DB" \
     -o jsonpath='{.items[0].metadata.name}')
   kubectl --context "$STANDBY_CTX" -n "$NS" exec "${STANDBY_POD}" -c "$STANDBY_DB" -- bash -c \
     'sqlplus -s / as sysdba <<< "show parameter fal_server"'
   ```

4. From the standby Pod, verify TCP to `$PRIMARY_NLB:1521`.
5. Re-render and apply DataguardBroker with current NLB hosts; confirm primary
   `LOG_ARCHIVE_DEST_2` is `VALID` and gaps clear.

### B) Duplicate `ORA-12170` to a pod overlay IP (for example `10.244/10.245/10.246.x.x`)

**Cause:** DBCA/RMAN built the auxiliary string from the local pod address, which the remote primary cannot reach.

**Fix:** Use an operator + database image that sets `CROSS_CLUSTER_STANDBY=true` for IP-based external primaries and rewrites the RMAN auxiliary endpoint to `STANDBY_HOSTNAME:STANDBY_LISTENER_PORT` (the standby NLB). Confirm the primary pod can open TCP to the standby NLB `:1521` (see [NLB traffic policy (short)](#nlb-traffic-policy-short)).

### C) NLB connectivity fails with `Local`

If the TCP test above fails while the standby Service uses
`externalTrafficPolicy: Local`, change the standby Service to
`externalTrafficPolicy: Cluster`, remove `is-preserve-source`, and retest TCP
port `1521` from the primary Pod. OCI rejects the combination of `Cluster` and
`is-preserve-source`.

For the complete lab configuration and network-specific notes, see the
[Example: Ashburn–Mumbai Flannel profile](#example-ashburnmumbai-flannel-profile).

### D) `ORA-28374` after networking works

**Cause:** Stale standby `tde-wallet-secret` zip vs current primary wallet, or dirty PVC from earlier failed attempts.

**Fix:** Re-export wallet from the current primary, recreate the standby-cluster Secret, wipe/recreate the standby PVC if needed, and recreate the standby SIDB.

### E) Standby Pod restarts intermittently during provisioning

Capture the first failed attempt before deleting or recreating the Pod:

```sh
kubectl --context "$STANDBY_CTX" -n "$NS" describe pod "$STANDBY_POD"
kubectl --context "$STANDBY_CTX" -n "$NS" logs "$STANDBY_POD" -c "$STANDBY_DB" --previous
kubectl --context "$STANDBY_CTX" -n "$NS" get events --sort-by=.lastTimestamp
kubectl --context "$STANDBY_CTX" -n "$NS" get sidb "$STANDBY_DB" -o yaml
kubectl --context "$STANDBY_CTX" -n "$NS" get svc "${STANDBY_DB}-lb" -o yaml
```

Also record NLB backend health and timestamps for DBCA/RMAN failure, Pod
restart, Service endpoint publication, and the next successful retry.
`publishNotReadyAddresses: true` addresses the pre-Ready NLB publication race.

### F) Broker WARNING / `ORA-16854` / switchover `ORA-16608`

**Symptom**

```text
Warning: ORA-16854: apply lag could not be determined
Error: ORA-16608: one or more members have warnings
Unable to switchover, primary database is still "..."
```

SQL may still show healthy transport/apply. Broker health checks can lag on
cross-cluster LB setups.

**Mitigation before retrying switchover**

```sh
RUNNER=$(kubectl --context "$STANDBY_CTX" -n "$NS" get pods --no-headers | awk '/dg-runner/ {print $1; exit}')

kubectl --context "$STANDBY_CTX" -n "$NS" exec "${RUNNER}" -- bash -lc "
printf '%s\n' \
  \"EDIT CONFIGURATION SET PROPERTY OperationTimeout='120';\" \
  \"EDIT CONFIGURATION SET PROPERTY CommunicationTimeout='300';\" \
  \"EDIT DATABASE ${PRIMARY_SID} SET STATE='APPLY-OFF';\" \
| dgmgrl /@${STANDBY_SID} as sysdba
sleep 15
printf '%s\n' \
  \"EDIT DATABASE ${PRIMARY_SID} SET STATE='APPLY-ON';\" \
  \"EDIT DATABASE ${PRIMARY_SID} SET PROPERTY ApplyLagThreshold='0';\" \
  \"EDIT DATABASE ${STANDBY_SID} SET PROPERTY ApplyLagThreshold='0';\" \
  \"EDIT DATABASE ${STANDBY_SID} SET PROPERTY TransportLagThreshold='0';\" \
  'SHOW CONFIGURATION;' \
| dgmgrl /@${STANDBY_SID} as sysdba
"
```

Only retry switchover when `SHOW CONFIGURATION` is `SUCCESS` with no member
WARNING/ERROR for several refresh cycles.

### G) Failed switchover leaves dual PHYSICAL STANDBY (no primary)

**Symptom**

- Alert log on old primary shows controlfile converted to standby, then target convert fails (`ORA-16471` / `ORA-1154`)
- Both databases report `PHYSICAL STANDBY`
- Intended new primary often shows `SWITCHOVER_STATUS=TO PRIMARY`

**Recovery**

1. Pause operators in both clusters (see [9. Switchover](#9-switchover)).
2. Promote the intended new primary (example toward `ORCLPRD` on the primary-cluster SIDB):

```sh
PRIMARY_POD=$(kubectl --context "$PRIMARY_CTX" -n "$NS" get pods -l "app=$PRIMARY_DB" \
  -o jsonpath='{.items[0].metadata.name}')

kubectl --context "$PRIMARY_CTX" -n "$NS" exec "${PRIMARY_POD}" -c "$PRIMARY_DB" -- bash -lc '
export ORACLE_SID=ORCLPRD
sqlplus -s / as sysdba <<SQL
whenever sqlerror continue
alter database recover managed standby database cancel;
shutdown immediate
startup mount
alter database commit to switchover to primary with session shutdown;
alter database open;
alter system set dg_broker_start=true;
select database_role, open_mode, switchover_status from v\$database;
SQL
'
```

3. Ensure the other member is standby with MRP, then resync Broker:

```sh
RUNNER=$(kubectl --context "$STANDBY_CTX" -n "$NS" get pods --no-headers | awk '/dg-runner/ {print $1; exit}')
kubectl --context "$STANDBY_CTX" -n "$NS" exec "${RUNNER}" -- bash -lc "
printf '%s\n' \
  'DISABLE CONFIGURATION;' \
  'ENABLE CONFIGURATION;' \
  'SHOW CONFIGURATION;' \
| dgmgrl /@${PRIMARY_SID} as sysdba
"
```

### H) Operator cancels MRP / toggles `dg_broker_start` during role change

If standby alert logs show repeated `ORA-16037` cancel of managed recovery around
Broker/prereq scripts, scale the operator to `0` for the switchover window,
complete Broker operations, then scale back to `1` after Configuration Status is
stable `SUCCESS`.

### I) Static connect identifier warnings

Listeners must register `${DB_UNIQUE_NAME}_DGMGRL` on both sides. Static connect
identifiers in Broker should point at the reachable NLB address, not a ClusterIP
or pod overlay IP.

### J) FSFO reinstate followed by `ORA-01017`

Credential drift between the database password file and the Broker authentication
wallet. Capture validation output, perform an approved credential
synchronization, rebuild the wallet if required, then rerun:

```text
VALIDATE STATIC CONNECT IDENTIFIER FOR ALL;
SHOW CONFIGURATION;
SWITCHOVER TO ORCLPRD;
```

## Example: Ashburn–Mumbai Flannel profile

Lab reference only. Do **not** copy these VIPs into your manifests.

Exercised on 22 September 2026 with separate private OKE clusters using Flannel:

| Item | Ashburn | Mumbai |
| --- | --- | --- |
| Database | `ORCLPRD` / `sidb-sample` | `ORCLSTBY` / `sidb-standby` |
| NLB service | `sidb-sample-lb` | `sidb-standby-lb` |
| Example NLB VIP (lab only) | `10.0.2.172` | `10.2.1.52` |
| NLB TCP port | `1521` | `1521` |
| Data Guard state | `Healthy / PRIMARY` | `Healthy / PHYSICAL_STANDBY` |

Working Service settings for that overlay profile:

- Primary: `externalTrafficPolicy: Local` and
  `oci-network-load-balancer.oraclecloud.com/is-preserve-source: "false"`.
- Standby: `externalTrafficPolicy: Cluster`, with `is-preserve-source` omitted.

Re-read both LoadBalancer Services after every recreate and update
`primarySource`, Broker member endpoints, and DNS records with the current
values.

## Related files

Samples used by this guide:

- Primary: [`config/samples/sidb/singleinstancedatabase_primary_cross_cluster_lb.yaml`](../../../config/samples/sidb/singleinstancedatabase_primary_cross_cluster_lb.yaml)
- Standby: [`config/samples/sidb/singleinstancedatabase_standby_cross_cluster_lb.yaml`](../../../config/samples/sidb/singleinstancedatabase_standby_cross_cluster_lb.yaml)
- Broker template: [`config/samples/sidb/dataguardbroker_cross_cluster_lb.yaml`](../../../config/samples/sidb/dataguardbroker_cross_cluster_lb.yaml)
- Broker renderer: [`config/samples/sidb/render-cross-cluster-dataguardbroker-lb.sh`](../../../config/samples/sidb/render-cross-cluster-dataguardbroker-lb.sh)
