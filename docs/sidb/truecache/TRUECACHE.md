# True Cache deployment runbook

Use this runbook for detailed True Cache deployment paths, peered networking,
remote-host primaries, and final verification. For the shortest in-cluster TCP
path, use [`README.md` — True Cache Quick Start](../README.md#true-cache-quick-start).

##### Contents

| Order | Section |
| --- | --- |
| 1 | [Before any path](#before-any-path) |
| 2 | [Choose the path](#choose-the-path) |
| 3 | [In-cluster TCP](#1-in-cluster-tcp) |
| 4 | [In-cluster TCPS](#2-in-cluster-tcps) |
| 5 | [Remote primary TCP or TCPS](#3-remote-primary-tcp-or-tcps) |
| 6 | [Remote host SI or RAC](#4-remote-host-si-or-rac) |
| 7 | [Final verification](#final-verification-every-path) |
| 8 | [Fast troubleshooting gates](#fast-troubleshooting-gates) |
| 9 | [Manually publish the True Cache blob ConfigMap](#manually-publish-the-true-cache-blob-configmap) |
| 10 | [Advanced True Cache association](#advanced-true-cache-association) |
| 11 | [Set `primarySource.dbName`](#set-primarysourcedbname) |

Complete **Before any path**, choose exactly one deployment path, then run
**Final verification**. Use **Shared details** only when a selected path points
to it. Sample YAML files are the source of truth for image, Secret, hostname,
and service placeholders.

## Before any path

Set these shell values (they do not contain passwords):

```sh
export NS=default
export PRIMARY=sidb-sample
export TRUECACHE=truecache
```

Complete these before you apply a primary or True Cache SIDB:

1. [True Cache prerequisites](../prerequisites/PREREQUISITES.md#true-cache-prerequisites) —
   linked admin/TDE/image checks
2. [True Cache](../prerequisites/PREREQUISITES.md#true-cache) and
   [Primary source](../prerequisites/PREREQUISITES.md#primary-source) field tables when editing
   YAML
3. [TCPS TLS secret](../prerequisites/PREREQUISITES.md#tcps-tls-secret) only if your chosen path
   uses TCPS

Never put passwords in a committed YAML file. Continue to
[Choose the path](#choose-the-path).

## Choose the path

Pick **one** row that matches where your primary runs and which protocol you
need. Do not mix paths. Open only that linked section next.

| Primary location | True Cache source field | Blob preparation | Start here |
| --- | --- | --- | --- |
| SIDB, in-cluster, TCP | `primarySource.databaseRef` | Primary SIDB generates ConfigMap | [In-cluster TCP](#1-in-cluster-tcp) or [`README.md` Quick Start](../README.md#true-cache-quick-start) |
| SIDB, in-cluster, TCPS | `primarySource.databaseRef` | Primary SIDB generates ConfigMap | [In-cluster TCPS](#2-in-cluster-tcps) or [`README.md` TCPS](../README.md#true-cache-setup-with-tde-and-tcps) |
| Remote SIDB primary (another cluster) | `primarySource.connectString` | Generate on primary, copy ConfigMap | [Remote primary](#3-remote-primary-tcp-or-tcps) |
| Remote host SI or RAC | `primarySource.connectString` | DBCA on primary, create ConfigMap | [Remote host SI/RAC](#4-remote-host-si-or-rac) |

`databaseRef` is same-namespace only; a remote or non-Kubernetes primary always
uses `connectString`.

Every path has the same two gates before the True Cache YAML can be applied:

1. The True Cache blob is available as a ConfigMap in the True Cache namespace.
2. Primary connection method, endpoint, service name, and sample placeholders
   match your environment.

## Shared details (reference)

Use this only when a selected path needs field, service-association, remote
primary, or networking detail. It is not another prerequisite checklist.

### Fields to align before applying

On an operator-managed primary, confirm the primary YAML contains:

1. `edition: enterprise`, `createAs: primary`, and `archiveLog: true`
2. `security.secrets.admin` and `security.secrets.tde`
3. `trueCache.generateBlob: true` and `trueCache.createConfigMap: true`
4. `trueCache.generatePath` (normally `/tmp/tc_config_blob.tar.gz`)
5. `replicas: 1`

`trueCache.generateEnabled: true` is retained for compatibility and enables
both generation and ConfigMap publication, but new manifests should use the
separate `generateBlob` and `createConfigMap` fields.

On the True Cache YAML, confirm the following values match the primary and
your environment:

1. `createAs: truecache`, `edition: enterprise`, `sid`, admin/TDE secrets, image,
   storage, and any required `resources`, `initParams`, or `nodeSelector`. OCR
   `database/enterprise_ru` needs `spec.image.pullSecrets` (for example
   `ocr-reg-cred`); anonymous pull is not supported for that licensed image
2. `primarySource.databaseRef` for a primary in the same Kubernetes cluster and
   namespace (in-cluster samples), or `primarySource.connectString` for a
   remote primary (SIDB in another cluster, or external host)
3. `trueCache.blobConfigMapRef`, `blobConfigMapKey`, and `blobMountPath`; the
   default ConfigMap key is `tc_config_blob.tar.gz`
4. `trueCache.truedbUniqueName`
5. `trueCache.trueCacheServices` in the form
   `PRIMARY_PDB_NAME:PRIMARY_SERVICE_NAME:TRUECACHE_SERVICE_NAME`
6. `services.endpoints` for client exposure; `services.external` is legacy and
   deprecated. Set `services.endpoints[].isKeep: true` when a managed endpoint
   and fixed NLB address must survive SIDB recreation.
7. `hostAliases` when the primary hostname cannot be resolved by cluster DNS
8. `services.endpoints[].annotations`, including the subnet and optional fixed
   private NLB address, when a stable endpoint is required

The first value is the primary PDB name. The middle value is the exact
listener-registered primary PDB service used for the mapped workload; it is
not necessarily the PDB name. On RAC or on a primary with a `db_domain`, it
may be domain-qualified, such as `pdb1.example.com`. Confirm it with the
primary listener or `srvctl` before deploying. The last value is the True
Cache service for that mapped PDB. The DBCA/runtime association flow creates
and starts it in the mapped True Cache PDB; do not create it in the True Cache
CDB or pre-create it manually in the read-only True Cache PDB.

For automatic service registration, the default is manual association when
`autoTCServiceRegistration` is omitted or `false`. When it is `true`, the
helper must exist at `/home/oracle/configure-primary-truecache-service.sh` (or
the path in `PRIMARY_TC_SERVICE_SCRIPT_PATH`) on every RAC node, and the
`externaljob.ora`/DBMS_SCHEDULER checks in [`PREREQUISITES.md`](../prerequisites/PREREQUISITES.md)
must pass. OCR SIDB primaries that use the True Cache capable image (samples
set `image.pullFrom: container-registry.oracle.com/database/enterprise_ru:latest`)
already ship the helper at the default path—confirm it is present; do not copy
it again. Install or copy the sample script only for external host SI/RAC or
other non-OCR primaries. Set `PRIMARY_TC_SERVICE_SCRIPT_PATH` on the True Cache
SIDB `spec.envVars` only when that installed path differs from the default; the
operator forwards it to the True Cache image. `primarySchedulerCredentialName` is optional when a named
DBMS_SCHEDULER credential is used. Automatic registration submits an
executable `DBMS_SCHEDULER` job on the primary host and is security-sensitive;
use it only when the primary policy permits database-initiated OS execution.
Manual association is the supported fallback when the scheduler, helper, or
primary RAC environment does not satisfy these requirements.

### Primary service registration

There are these primary-side connection targets:

- `primarySource.connectString` is the primary CDB bootstrap target. It must be
  reachable while True Cache is created, for example
  `10.0.2.7:1521/ORCLPRD` or `sidb-sample.internal.example.com:1521/ORCLPRD`.
  Use TCP for this bootstrap path. When the primary also publishes TCPS and the
  remote True Cache path uses a NodePort or non-default TCPS port, set
  `primarySource.tcpsPort` to that reachable TCPS port for post-create TCPS
  aliases. Remote-primary “peered” setups are defined under
  [What “peered” means](#what-peered-means-in-true-cache-samples).
- The middle value in `trueCacheServices`, for example `tpdb_primary`, is the
  primary PDB service used for the mapped workload and the True Cache service
  association. It is not the CDB bootstrap connect string.

Complete association using one of the methods in
[Primary PDB service](../prerequisites/PREREQUISITES.md#primary-pdb-service):

- With `autoTCServiceRegistration: true` (sample default), complete the helper,
  `externaljob.ora`, and DBMS_SCHEDULER prerequisites. The helper creates and
  starts the primary PDB service if it is missing, then associates it with the
  True Cache service during provisioning.
- With `autoTCServiceRegistration: false` (or when it is omitted), create the
  primary PDB service yourself if you need a dedicated name, then complete
  association manually after the True Cache resource has been created.

For the manual path, the True Cache image prints a primary-host helper reminder
for the selected PDB, primary service, True Cache service, and True Cache
listener endpoint. After `DATABASE IS READY TO USE`, inspect the True Cache
logs for that reminder (search for the helper script name or
`AUTO_TC_SVC_REGISTRATION=false`):

```sh
kubectl logs -n "$NS" -l "app=$TRUECACHE" --tail=500 \
  | grep -E 'configure-primary-truecache-service\.sh|AUTO_TC_SVC_REGISTRATION=false'
```

The command shape is:

```sh
/home/oracle/configure-primary-truecache-service.sh \
  "<PRIMARY_SERVICE>" "<TRUECACHE_SERVICE>" "<PRIMARY_PDB>" \
  "<TC_HOST>:1521/<TC_CALLBACK_SERVICE>" \
  "<PRIMARY_DB_NAME>" "<PRIMARY_DB_UNIQUE_NAME>" "<PRIMARY_IS_RAC>" \
  '<PRIMARY_SYS_PASSWORD_OR_WALLET_SOURCE>'
```

Confirm the endpoint and service names, then run it on the primary host or
primary pod as the Oracle software owner. Do not put the SYS password in this
document or in shell history; use the password or wallet mechanism supported by
the image. Then run the association query in
[Final verification](#final-verification-every-path) on the primary.

For an external RAC primary, the helper runs on a RAC node selected by the
primary runtime. If automatic registration fails, inspect the helper debug log
on that node and verify that `ORACLE_HOME`, `ORACLE_SID`, and `PATH` are
available to the non-interactive job. The database-owner Oracle home should
provide `srvctl`; a `PRCR-1120` or `CRS-5702` message stating that a service is
already running must be treated as an idempotent state, not as a reason to
stop before the DBCA association. If the scheduler reports `ORA-27369`,
`No such file or directory`, `Operation not permitted`, or leaves the
association pending, switch to manual mode and run the printed helper command
as the Oracle software owner after confirming the endpoint and service names.

### Remote primary pre-checks

Before creating a remote True Cache, also verify the primary from its cluster:

```sh
# For a remote SIDB primary in another cluster, select that cluster first:
# export KUBECONFIG="$PRIMARY_KUBECONFIG"
PRIMARY_POD=$(kubectl get pod -n "$NS" -l "app=$PRIMARY" \
  -o jsonpath='{.items[0].metadata.name}')
kubectl exec -n "$NS" "$PRIMARY_POD" -- bash -lc 'lsnrctl status LISTENER'
kubectl get service -n "$NS" -o wide
```

Check the database state and wallet locally on the primary:

```sh
kubectl exec -n "$NS" "$PRIMARY_POD" -- bash -lc \
  'sqlplus -s / as sysdba <<EOF
set heading on feedback on pages 100 lines 200
select status, wallet_type, wrl_parameter from v\$encryption_wallet;
select name, open_mode from v\$pdbs;
exit
EOF'
```

The wallet must be open, the source PDB must be `READ WRITE`, and the listener
must advertise the primary CDB/service. For a remote primary, verify that the
ExternalDNS/private-DNS name resolves to the current endpoint and that the
ExternalDNS record is synchronized before applying the True Cache YAML.

For TCPS, the primary listener output must also show a TCPS endpoint on port
`2484`, and the primary Service must publish that port:

```sh
kubectl exec -n "$NS" "$PRIMARY_POD" -- bash -lc \
  "lsnrctl status LISTENER | grep -E 'tcps|2484|${PRIMARY}'"
kubectl get service -n "$NS" -o wide
```

## Connectivity and Service types

True Cache create and redo apply need **network reachability from the True
Cache pod to the primary listener** (TCP bootstrap, and TCPS when enabled).
Client apps separately need a path to the True Cache listener if they connect
to it. Choose `spec.services.endpoints[].type` so those paths work. Prefer
`services.endpoints` over deprecated `services.external`.

### What “peered” means in True Cache samples

Sample names such as `*_tcp_peered.yaml` / `*_tcps_peered.yaml` mean the
**primary SIDB and True Cache SIDB run in different Kubernetes clusters** (or
otherwise cannot use in-cluster `databaseRef` / ClusterIP DNS). “Peered”
refers to the **platform network and DNS setup you must provide** so the True
Cache pod can resolve and open TCP (and optional TCPS) to the primary endpoint
in `primarySource.connectString`. Typical OCI lab pieces:

1. **VCN peering (or equivalent routing)** between the primary cluster VCN and
   the True Cache cluster VCN so private traffic is allowed.
2. **Security lists / NSGs** that allow the True Cache worker CIDR to the
   primary listener ports (TCP `1521`, and TCPS `2484` or the published
   NodePort when using NodePort).
3. **Name resolution** for the hostname in `connectString`: shared private DNS,
   ExternalDNS into a zone both clusters use, or `spec.hostAliases` on the True
   Cache SIDB when DNS is not ready.
4. A **reachable Service frontend** on the primary side: usually an internal
   `LoadBalancer` (NLB) address, or a `NodePort` on a worker IP when you do not
   use a load balancer.

In-cluster TCP/TCPS samples are **not** peered: they stay on ClusterIP and
`databaseRef`. Remote host SI/RAC primaries are also not “peered SIDB”
samples; the primary listener is outside Kubernetes (host/SCAN), while the
True Cache SIDB still needs a route and DNS/IP to that host.

### ClusterIP vs NodePort vs LoadBalancer

| Type | When to use for True Cache | When not to use |
| --- | --- | --- |
| **ClusterIP** (or omit `services` and use the operator default) | Same Kubernetes cluster: primary and True Cache pods reach each other through cluster DNS (`<svc>.<ns>.svc`). | Remote primary or any client outside this cluster’s DNS — ClusterIP is not reachable there. |
| **NodePort** | Remote primary (or bare-metal) when you **do not** want a cloud LoadBalancer: publish TCP/TCPS on each node’s high port; put `NODE_IP:NodePort` (or a stable node hostname) in `connectString` / `tcpsPort`. Useful in labs without NLB quota, or when policy forbids LoadBalancer Services. | In-cluster only paths — unnecessary. Also avoid if node IPs are unstable and you have no DNS/VIP in front of them. |
| **LoadBalancer** (often internal NLB on OCI) | Remote peered SIDB primary, or True Cache that must be reached from another VCN/cluster/clients: stable hostname or IP via ExternalDNS / private DNS. | In-cluster TCP/TCPS between primary and True Cache — ClusterIP is enough; do not add LB only for that pod-to-pod path. |

**Rule of thumb:** if both SIDBs are in one cluster, use **ClusterIP**. If the
True Cache pod must leave the cluster network to hit the primary (or clients
must enter from outside), use **LoadBalancer** or **NodePort**, plus peering
and DNS as above. Prove connectivity with `nc`/`sqlplus` from the True Cache
pod to the exact host:port in `connectString` before apply.

| Topology | Sample YAML | Service type | Why |
| --- | --- | --- | --- |
| In-cluster (same namespace), TCP primary | [`singleinstancedatabase_truecache_primary.yaml`](../../../config/samples/sidb/singleinstancedatabase_truecache_primary.yaml) | Default / optional `ClusterIP` | True Cache uses `databaseRef`; in-cluster DNS is enough. |
| In-cluster (same namespace), TCP True Cache | [`singleinstancedatabase_truecache.yaml`](../../../config/samples/sidb/singleinstancedatabase_truecache.yaml) | Default / omit `services` | In-cluster clients use ClusterIP DNS. |
| In-cluster (same namespace), TCPS primary | [`singleinstancedatabase_truecache_primary_tcps.yaml`](../../../config/samples/sidb/singleinstancedatabase_truecache_primary_tcps.yaml) | `ClusterIP` TCP `1521` + TCPS `2484` | In-cluster; TLS SANs must cover in-cluster hostnames. |
| In-cluster (same namespace), TCPS True Cache | [`singleinstancedatabase_truecache_same_cluster_tcps.yaml`](../../../config/samples/sidb/singleinstancedatabase_truecache_same_cluster_tcps.yaml) | `ClusterIP` TCP `1521` + TCPS `2484` | Same as above. |
| Remote SIDB primary, TCP (peered) | [`singleinstancedatabase_truecache_primary_tcp_peered.yaml`](../../../config/samples/sidb/singleinstancedatabase_truecache_primary_tcp_peered.yaml) | `LoadBalancer` **or** `NodePort` | True Cache uses `connectString` to a reachable host:port after VCN peering + DNS. |
| Remote SIDB primary, TCPS (peered) | [`singleinstancedatabase_truecache_primary_tcps_peered.yaml`](../../../config/samples/sidb/singleinstancedatabase_truecache_primary_tcps_peered.yaml) | `LoadBalancer` **or** `NodePort` TCP + TCPS | Same as above; set `primarySource.tcpsPort` to the reachable TCPS port (service port `2484` on LB, or the TCPS NodePort). |
| Remote-primary True Cache SIDB | [`singleinstancedatabase_truecache_external.yaml`](../../../config/samples/sidb/singleinstancedatabase_truecache_external.yaml), [`*_external_rac.yaml`](../../../config/samples/sidb/singleinstancedatabase_truecache_external_rac.yaml) | Usually `LoadBalancer` (or `NodePort`) | Expose True Cache outside this cluster’s DNS when clients or peers need it. |
| Remote host SI/RAC primary | N/A (primary outside Kubernetes) | Host listener / SCAN | True Cache YAML uses `connectString` only; no primary SIDB Service. |

An in-cluster primary YAML that sets only `services.external.type: ClusterIP`
is the **legacy** shape. Prefer omitting Services (operator default ClusterIP)
or the commented `services.endpoints` block in the in-cluster primary sample.

<a id="1-in-cluster-tcp"></a>
<a id="1-same-cluster-tcp"></a>

## 1. In-cluster TCP

The primary and True Cache are in the **same Kubernetes cluster and namespace**.
The operator creates the blob ConfigMap on the primary. Pair
[`singleinstancedatabase_truecache_primary.yaml`](../../../config/samples/sidb/singleinstancedatabase_truecache_primary.yaml)
with [`singleinstancedatabase_truecache.yaml`](../../../config/samples/sidb/singleinstancedatabase_truecache.yaml)
(`primarySource.databaseRef`).

This path creates the sample primary, publishes its blob ConfigMap, creates the
True Cache, and verifies that the True Cache is ready for use.

This path uses the operator-created `ClusterIP` Services; it does not require a
LoadBalancer, NodePort, VCN peering, or Private DNS. Use the primary Service
DNS name for an in-cluster `primarySource.connectString` only when you choose
that form instead of `primarySource.databaseRef`.

### 1.1 Create or update the primary

For a new primary, edit and apply the
[in-cluster TCP primary YAML](../../../config/samples/sidb/singleinstancedatabase_truecache_primary.yaml):

```sh
kubectl apply -f config/samples/sidb/singleinstancedatabase_truecache_primary.yaml
kubectl wait --for=jsonpath='{.status.status}'=Healthy \
  "singleinstancedatabase/$PRIMARY" -n "$NS" --timeout=60m
```

While this waits, use a second terminal to watch the primary. Press Ctrl-C in
the watch terminal only; it does not stop the wait:

```sh
kubectl get singleinstancedatabase "$PRIMARY" -n "$NS" -w
```

For an existing Healthy primary, enable blob generation with a merge patch
instead of recreating the primary (supported on `createAs: primary` when
`edition: enterprise`, archive log is on, and the TDE secret is set):

```sh
kubectl patch singleinstancedatabase "$PRIMARY" -n "$NS" --type merge -p \
  '{"spec":{"trueCache":{"generateBlob":true,"createConfigMap":true,"generatePath":"/tmp/tc_config_blob.tar.gz"}}}'
```

Then wait for Healthy (needed after a new apply; after a patch on an already
Healthy primary this returns immediately) and for ConfigMap
`$PRIMARY-truecache-blob` to contain binary key `tc_config_blob.tar.gz` (this
is the blob-ready gate):

```sh
kubectl wait --for=jsonpath='{.status.status}'=Healthy \
  "singleinstancedatabase/$PRIMARY" -n "$NS" --timeout=60m
kubectl wait --for=jsonpath='{.binaryData.tc_config_blob\.tar\.gz}' \
  "configmap/$PRIMARY-truecache-blob" -n "$NS" --timeout=20m
```

While the blob is being published, watch the primary status in a second
terminal:

```sh
kubectl get singleinstancedatabase "$PRIMARY" -n "$NS" -w
```

### 1.2 Create True Cache

Keep `autoTCServiceRegistration: true` in the sample. The helper creates and
starts the primary PDB service (middle `trueCacheServices` value) if needed,
then associates it with the True Cache service. Manual
`DBMS_SERVICE.CREATE_SERVICE` is only for dedicated/manual paths — see
[Primary PDB service](../prerequisites/PREREQUISITES.md#primary-pdb-service).

Edit the [in-cluster TCP True Cache YAML](../../../config/samples/sidb/singleinstancedatabase_truecache.yaml)
for image, storage class, secret names, PDB/service names, and the True Cache
unique name, then apply it:

```sh
kubectl apply -f config/samples/sidb/singleinstancedatabase_truecache.yaml
kubectl wait --for=jsonpath='{.status.status}'=Healthy \
  "singleinstancedatabase/$TRUECACHE" -n "$NS" --timeout=60m
```

While this waits, watch the True Cache in a second terminal:

```sh
kubectl get singleinstancedatabase "$TRUECACHE" -n "$NS" -w
```

The sample uses `databaseRef: sidb-sample`. If your primary has another name,
change `spec.primarySource.databaseRef` and
`spec.trueCache.blobConfigMapRef` to match it before applying. Then complete
[Final verification](#final-verification-every-path).

<a id="2-in-cluster-tcps"></a>
<a id="2-same-cluster-tcps"></a>

## 2. In-cluster TCPS

Use this path when the primary and True Cache are in the **same** cluster and
namespace and both present a TCPS listener. For TCP-only in-cluster, use
[In-cluster TCP](#1-in-cluster-tcp) instead.

This path creates the sample primary and True Cache with TCPS enabled, then
verifies that the True Cache is ready for use.

1. Create the admin, TDE, and primary
   [TLS secrets](../prerequisites/PREREQUISITES.md#tcps-tls-secret) in `$NS`.
2. Edit the
   [in-cluster TCPS primary YAML](../../../config/samples/sidb/singleinstancedatabase_truecache_primary_tcps.yaml)
   so the primary has `edition: enterprise`, `archiveLog: true`, TDE, blob
   generation, `security.tcps.enabled: true`, and
   `tlsSecret: sidb-primary-tcps-tls` (or your Secret name).
3. Apply the primary and wait for Healthy plus the blob ConfigMap:

```sh
kubectl apply -f config/samples/sidb/singleinstancedatabase_truecache_primary_tcps.yaml
kubectl wait --for=jsonpath='{.status.status}'=Healthy \
  "singleinstancedatabase/$PRIMARY" -n "$NS" --timeout=60m
kubectl wait --for=jsonpath='{.binaryData.tc_config_blob\.tar\.gz}' \
  "configmap/$PRIMARY-truecache-blob" -n "$NS" --timeout=20m
```

While this waits, watch the primary in a second terminal:

```sh
kubectl get singleinstancedatabase "$PRIMARY" -n "$NS" -w
```

4. Confirm the True Cache TLS Secret exists in `$NS`. Both samples default to
   `tlsSecret: sidb-primary-tcps-tls` (shared when SANs cover both names) and
   expose **ClusterIP** TCP `1521` plus TCPS `2484`. Use a dedicated Secret
   such as `sidb-truecache-tcps-tls` when SANs differ, and set
   `spec.security.tcps.tlsSecret` to match before apply.
5. Edit and apply the
   [in-cluster TCPS True Cache YAML](../../../config/samples/sidb/singleinstancedatabase_truecache_same_cluster_tcps.yaml)
   with `autoTCServiceRegistration: true` (helper creates the primary PDB
   service if needed; see [Primary PDB service](../prerequisites/PREREQUISITES.md#primary-pdb-service)):

```sh
kubectl apply -f config/samples/sidb/singleinstancedatabase_truecache_same_cluster_tcps.yaml
kubectl wait --for=jsonpath='{.status.status}'=Healthy \
  "singleinstancedatabase/$TRUECACHE" -n "$NS" --timeout=60m
```

While this waits, watch the True Cache in a second terminal:

```sh
kubectl get singleinstancedatabase "$TRUECACHE" -n "$NS" -w
```

6. Use port `2484` for TCPS client connect strings. Enable TCPS on **both** the
   primary and True Cache SIDBs for this path; do not leave the primary
   TCP-only while True Cache has `security.tcps.enabled: true`.
7. Complete [Final verification](#final-verification-every-path).

<a id="3-remote-primary-tcp-or-tcps"></a>
<a id="3-cross-cluster-tcp-or-tcps"></a>

## 3. Remote primary TCP or TCPS

This is the **peered** path: primary and True Cache SIDBs are in different
Kubernetes clusters. Complete
[What “peered” means](#what-peered-means-in-true-cache-samples) first (VCN
peering or equivalent routing, security rules, DNS or `hostAliases`, and a
reachable primary `LoadBalancer` or `NodePort`). Same-cluster ClusterIP /
`databaseRef` does not apply here; the consumer uses
`primarySource.connectString`.

This path creates a blob on the primary cluster, copies it to the True Cache
cluster, then creates and verifies True Cache against the remote primary.

Run primary-cluster commands with the primary kubeconfig and True Cache-cluster
commands with the True Cache kubeconfig. Set these before starting:

```sh
export PRIMARY_KUBECONFIG=/path/to/primary.kubeconfig
export TRUECACHE_KUBECONFIG=/path/to/truecache.kubeconfig
```

### 3.1 Primary cluster: create the blob and endpoint

Edit the appropriate primary manifest before applying it:

- [Remote primary TCP YAML](../../../config/samples/sidb/singleinstancedatabase_truecache_primary_tcp_peered.yaml)
- [Remote primary TCPS YAML](../../../config/samples/sidb/singleinstancedatabase_truecache_primary_tcps_peered.yaml)

```sh
export KUBECONFIG="$PRIMARY_KUBECONFIG"
# TCP primary:
kubectl apply -f config/samples/sidb/singleinstancedatabase_truecache_primary_tcp_peered.yaml
# TCPS primary: use this command instead:
# kubectl apply -f config/samples/sidb/singleinstancedatabase_truecache_primary_tcps_peered.yaml
kubectl wait --for=jsonpath='{.status.status}'=Healthy \
  "singleinstancedatabase/$PRIMARY" -n "$NS" --timeout=60m
kubectl wait --for=jsonpath='{.binaryData.tc_config_blob\.tar\.gz}' \
  "configmap/$PRIMARY-truecache-blob" -n "$NS" --timeout=20m
kubectl get service "$PRIMARY-lb" -n "$NS" -o wide
```

While the primary waits, use a second terminal with the primary cluster context:

```sh
KUBECONFIG="$PRIMARY_KUBECONFIG" \
  kubectl get singleinstancedatabase "$PRIMARY" -n "$NS" -w
```

For TCP-only, expose the primary on port `1521`, then set
`primarySource.connectString` in the True Cache consumer YAML to that endpoint.
For TCPS, apply
`singleinstancedatabase_truecache_primary_tcps_peered.yaml` and keep TCP in
`primarySource.connectString` for DBCA bootstrap (LoadBalancer `:1521` or the
published TCP NodePort). Set `primarySource.tcpsPort` to the reachable primary
TCPS port (`2484` on a LoadBalancer service port, or the TCPS NodePort when the
peer uses NodePort/IP). Enable `security.tcps`, reference the TLS secret, and
publish True Cache `2484` for clients. Certificate SANs must cover the hostname
or IP used in `connectString` and peer endpoints.

Wait for the LoadBalancer and ExternalDNS/private-DNS record to converge before
continuing. This commonly takes a few minutes after the Service is created.
Proceed only when the hostname in `primarySource.connectString` resolves to the
current LoadBalancer address. The peered samples use
`sidb-sample.internal.example.com`:

```sh
getent hosts sidb-sample.internal.example.com
# or:
dig +short sidb-sample.internal.example.com
```

From the True Cache cluster, verify the TCP bootstrap port is reachable before
applying the consumer YAML (same host and TCP port as `connectString`):

```sh
export KUBECONFIG="$TRUECACHE_KUBECONFIG"
kubectl run truecache-netcheck --rm -i --restart=Never \
  --image=busybox:1.36 -- \
  sh -c 'nc -vz sidb-sample.internal.example.com 1521'
```

For TCPS, also check the reachable primary TCPS port from `primarySource.tcpsPort`
(or `2484` on a LoadBalancer service port):

```sh
kubectl run truecache-tcps-netcheck --rm -i --restart=Never \
  --image=busybox:1.36 -- \
  sh -c 'nc -vz sidb-sample.internal.example.com 2484'
```

### 3.2 Copy the blob ConfigMap to the True Cache cluster

Prefer copying the blob file and recreating the ConfigMap. That avoids carrying
primary-cluster `uid`, `resourceVersion`, or `ownerReferences` into the True
Cache cluster:

```sh
export KUBECONFIG="$PRIMARY_KUBECONFIG"
PRIMARY_POD=$(kubectl get pod -n "$NS" -l "app=$PRIMARY" \
  -o jsonpath='{.items[0].metadata.name}')
kubectl cp "$NS/$PRIMARY_POD:/tmp/tc_config_blob.tar.gz" \
  /tmp/tc_config_blob.tar.gz
export KUBECONFIG="$TRUECACHE_KUBECONFIG"
kubectl create configmap "$PRIMARY-truecache-blob" \
  --from-file=tc_config_blob.tar.gz=/tmp/tc_config_blob.tar.gz \
  -n "$NS" --dry-run=client -o yaml | kubectl apply -f -
kubectl get configmap "$PRIMARY-truecache-blob" -n "$NS"
rm -f /tmp/tc_config_blob.tar.gz
```

If you export the ConfigMap YAML instead, strip cluster-specific metadata before
applying it in the True Cache cluster:

```sh
export KUBECONFIG="$PRIMARY_KUBECONFIG"
kubectl get configmap "$PRIMARY-truecache-blob" -n "$NS" -o json \
  | jq 'del(.metadata.uid,
            .metadata.resourceVersion,
            .metadata.creationTimestamp,
            .metadata.generation,
            .metadata.managedFields,
            .metadata.ownerReferences,
            .metadata.annotations."kubectl.kubernetes.io/last-applied-configuration")
        | .metadata.namespace = env.NS' \
  > /tmp/truecache-blob.json
export KUBECONFIG="$TRUECACHE_KUBECONFIG"
kubectl apply -f /tmp/truecache-blob.json
kubectl get configmap "$PRIMARY-truecache-blob" -n "$NS"
rm -f /tmp/truecache-blob.json
```

The temporary files contain the blob and are not source files. Delete them after
successful ConfigMap verification, as shown above; the blob may contain
sensitive database configuration or wallet material.

### 3.3 Apply the remote-primary True Cache YAML

Edit the [remote-primary True Cache YAML](../../../config/samples/sidb/singleinstancedatabase_truecache_external.yaml):
`connectString`, image, secret names, and the primary PDB/service mapping. For
TCP-only (paired with `truecache_primary_tcp_peered.yaml`), leave
`primarySource.tcpsPort` and `security.tcps` commented as in the sample. Then
apply it:

```sh
kubectl apply -f config/samples/sidb/singleinstancedatabase_truecache_external.yaml
kubectl wait --for=jsonpath='{.status.status}'=Healthy \
  "singleinstancedatabase/$TRUECACHE" -n "$NS" --timeout=60m
```

While this waits, use a second terminal with the True Cache cluster context:

```sh
KUBECONFIG="$TRUECACHE_KUBECONFIG" \
  kubectl get singleinstancedatabase "$TRUECACHE" -n "$NS" -w
```

For a TCPS primary (paired with `truecache_primary_tcps_peered.yaml`), keep TCP
in `primarySource.connectString` for bootstrap and uncomment or set these
fields in the external sample before apply:

```yaml
primarySource:
  connectString: "sidb-sample.internal.example.com:1521/ORCLPRD"
  tcpsPort: 2484
security:
  tcps:
    enabled: true
    tlsSecret: sidb-truecache-tcps-tls   # True Cache leaf; not the primary secret name
services:
  endpoints:
    - name: loadbalancer
      tcps:
        enabled: true
        port: 2484
```

Create the True Cache peer TLS secret (`sidb-truecache-tcps-tls` by default)
before apply. Certificate SANs must cover the hostname or IP peers use. Then
complete [Final verification](#final-verification-every-path).

<a id="4-remote-host-si-or-rac"></a>
<a id="4-external-host-si-or-rac"></a>

## 4. Remote host SI or RAC

The primary is a remote host SI or RAC (not a Kubernetes SIDB), so DBCA must
create the blob.

This path prepares the blob on the primary host, publishes it as a ConfigMap
in the True Cache cluster, then creates and verifies True Cache.

### 4.1 Primary host: create the blob

Run as the Oracle software owner on the primary host (any RAC node that can
run DBCA against the primary database). Set `PRIMARY_DB` to the primary
source database identifier DBCA expects (SID or DB name for that home), not
the True Cache SID:

```sh
export ORACLE_HOME=/path/to/oracle/home
export PRIMARY_DB=ORCLPRD   # example; use the RAC primary source DB / SID
mkdir -p /tmp/truecache-blob
"$ORACLE_HOME/bin/dbca" -configureDatabase \
  -prepareTrueCacheConfigFile \
  -sourceDB "$PRIMARY_DB" \
  -trueCacheBlobLocation /tmp/truecache-blob \
  -silent
```

For a TDE-enabled database, provide the TDE wallet password option required by
the installed Oracle Database version and confirm that DBCA created a `.tar.gz`
archive:

```sh
find /tmp/truecache-blob -maxdepth 1 -type f -name '*.tar.gz' -print
```

See the Oracle DBCA reference for the version-specific True Cache blob
procedure:
<https://docs.oracle.com/en/database/oracle/oracle-database/26/odbtc/configuring-true-cache-dbca.html#GUID-A534C50F-3A84-4C04-9765-F85F99F0E52F>

### 4.2 True Cache cluster: create the blob ConfigMap

Copy the generated archive to the machine with `kubectl`, choose the ConfigMap
name, and use the same name in `spec.trueCache.blobConfigMapRef` in the True
Cache YAML before applying it:

```sh
export BLOB=/path/to/tc_config_blob.tar.gz
export BLOB_CONFIGMAP=orcl-primary-truecache-blob
kubectl create configmap "$BLOB_CONFIGMAP" \
  --from-file=tc_config_blob.tar.gz="$BLOB" \
  -n "$NS" --dry-run=client -o yaml | kubectl apply -f -
kubectl get configmap "$BLOB_CONFIGMAP" -n "$NS"
```

For example, if you keep `BLOB_CONFIGMAP=orcl-primary-truecache-blob`, set
`trueCache.blobConfigMapRef: orcl-primary-truecache-blob`. The external SI
sample currently uses `sidb-sample-truecache-blob`; change that field when using
the name above. The external RAC sample uses `orcl-primary-truecache-blob`.

### 4.3 Apply the True Cache YAML

For an external single-instance primary, edit and apply the
[external SI YAML](../../../config/samples/sidb/singleinstancedatabase_truecache_external.yaml).
Before applying, replace the sample peered-SIDB placeholders with host-primary
values: TCP `connectString` (`HOST:1521/SERVICE`), `blobConfigMapRef` (must
match `$BLOB_CONFIGMAP`), `trueCacheServices`, image, secrets, and storage.
Leave `tcpsPort` / `security.tcps` commented unless the host primary publishes
TCPS. If `autoTCServiceRegistration: true`, copy the helper onto the external
primary host and complete the remaining prerequisites
([Primary PDB service](../prerequisites/PREREQUISITES.md#primary-pdb-service)).

```sh
kubectl apply -f config/samples/sidb/singleinstancedatabase_truecache_external.yaml
kubectl wait --for=jsonpath='{.status.status}'=Healthy \
  "singleinstancedatabase/$TRUECACHE" -n "$NS" --timeout=60m
```

While this waits, use a second terminal to watch the True Cache:

```sh
kubectl get singleinstancedatabase "$TRUECACHE" -n "$NS" -w
```

For an external RAC primary, edit and apply the
[external RAC YAML](../../../config/samples/sidb/singleinstancedatabase_truecache_external_rac.yaml).
The sample resource name is `tck8node1` (set `TRUECACHE=tck8node1` or change
`metadata.name`). Before applying, replace the sample values for the RAC SCAN
`connectString`, short primary `dbName` (`PRIMDB` in the sample),
`blobConfigMapRef` (`orcl-primary-truecache-blob`, must match `$BLOB_CONFIGMAP`),
`trueCacheServices`, image, secrets, and storage class. For a domain-qualified
RAC SCAN service, set `dbName` to the short CDB `DB_NAME`; never use the full
SCAN service or `DB_UNIQUE_NAME`. Keep `connectString` on TCP SCAN bootstrap;
set `tcpsPort` / `security.tcps` only for TCPS. If
`autoTCServiceRegistration: true`, copy the helper onto every external RAC
node (same path on each) and complete
[Primary PDB service](../prerequisites/PREREQUISITES.md#primary-pdb-service).

```sh
kubectl apply -f config/samples/sidb/singleinstancedatabase_truecache_external_rac.yaml
kubectl wait --for=jsonpath='{.status.status}'=Healthy \
  "singleinstancedatabase/$TRUECACHE" -n "$NS" --timeout=60m
```

While this waits, use a second terminal to watch the True Cache:

```sh
kubectl get singleinstancedatabase "$TRUECACHE" -n "$NS" -w
```

Then complete [Final verification](#final-verification-every-path).

## Final verification: every path

Do not treat pod creation alone as success. Run all checks below:

```sh
kubectl get singleinstancedatabase "$TRUECACHE" -n "$NS" -o wide
kubectl describe singleinstancedatabase "$TRUECACHE" -n "$NS"
kubectl get pods -n "$NS" -l app="$TRUECACHE" -o wide
kubectl logs -n "$NS" -l app="$TRUECACHE" --tail=200
```

Then verify the Oracle state from inside the True Cache pod:

```sh
TRUECACHE_POD=$(kubectl get pod -n "$NS" -l "app=$TRUECACHE" \
  -o jsonpath='{.items[0].metadata.name}')
kubectl exec -n "$NS" "$TRUECACHE_POD" -- bash -lc \
  'sqlplus -s / as sysdba <<EOF
set heading on feedback on pages 100 lines 200
select name, db_unique_name, database_role, open_mode from v\$database;
select name, open_mode from v\$pdbs;
select * from v\$archive_gap;
exit
EOF'
kubectl exec -n "$NS" "$TRUECACHE_POD" -- bash -lc 'lsnrctl status LISTENER'
```

Expected results are `DATABASE_ROLE = TRUE CACHE` (with a space),
`OPEN_MODE = READ ONLY WITH APPLY`, the mapped True Cache PDB open read-only
(for example `APPPDB1`), no rows from `v$archive_gap`, and a listener service
for the configured True Cache service name (sample `tpdb_cache` often appears
as `TPDB_CACHE` in `lsnrctl`).

For a TCPS deployment, also validate the primary connection from the True Cache
pod. First confirm that the pod has a client wallet and SQL*Net configuration
that trusts the primary certificate; a raw TCPS descriptor alone is not enough
to establish trust. Replace the host, port, and service with your primary TCPS
endpoint. The command prompts for the SYS password; do not put the password in
the document or shell history:

```sh
kubectl exec -it "$TRUECACHE_POD" -n "$NS" -- bash -lc \
  "sqlplus 'sys@(DESCRIPTION=(ADDRESS=(PROTOCOL=TCPS)(HOST=primary.internal.example.com)(PORT=2484))(CONNECT_DATA=(SERVICE_NAME=ORCLPRD))) as sysdba'"
```

If the image creates client or server wallet mounts, confirm only that the
paths exist; do not print wallet contents:

```sh
kubectl exec -n "$NS" "$TRUECACHE_POD" -- \
  bash -lc 'ls -l /opt/oracle/oradata/clientWallet 2>/dev/null || true; \
            ls -l /run/secrets/tls_secret 2>/dev/null || true'
```

The required database-creation signal in the True Cache pod logs is
`DATABASE IS READY TO USE` (the image may print a trailing `!`):

```text
DATABASE IS READY TO USE!
```

Always verify the primary-to-True-Cache service association on the **primary**
(both when `autoTCServiceRegistration` is `true` and when association was
manual). True Cache `Healthy` / `DATABASE IS READY TO USE` does not prove
association. If automatic registration is disabled or omitted, run the primary
helper first, then check. Run the following as the Oracle software owner on the
primary; replace `APPPDB1` with your primary PDB. For an operator-managed
primary, first open SQL*Plus in the primary Pod. For a remote-host primary, run
the same SQL locally with `sqlplus / as sysdba`:

```sh
PRIMARY_POD=$(kubectl get pod -n "$NS" -l "app=$PRIMARY" \
  -o jsonpath='{.items[0].metadata.name}')
kubectl exec -it -n "$NS" "$PRIMARY_POD" -- sqlplus / as sysdba
```

```sql
ALTER SESSION SET CONTAINER=APPPDB1;
SELECT service_id, name, true_cache_service
FROM   v$active_services
ORDER BY service_id;
```

The mapped primary service (middle `trueCacheServices` value; sample
`tpdb_primary` / `TPDB_PRIMARY`) must show the expected True Cache service name
(sample `tpdb_cache` / `TPDB_CACHE`) in `TRUE_CACHE_SERVICE`.

## Fast troubleshooting gates

| Symptom | Check first |
| --- | --- |
| Blob not found | ConfigMap exists in the True Cache namespace and its key is exactly `tc_config_blob.tar.gz` |
| `PRIMARY_DB_NAME` error | [`primarySource.dbName`](#set-primarysourcedbname) is the short CDB `DB_NAME`, not a service or `DB_UNIQUE_NAME` |
| Connection timeout | Resolve and connect to the primary hostname and port from inside the True Cache pod |
| `ORA-12514` for a PDB service | Use the exact primary listener-registered PDB service in the middle `trueCacheServices` field; RAC and `db_domain` commonly produce a domain-qualified name |
| TCPS handshake failure | TLS secret exists and SAN matches the hostname or IP in `connectString`; for NodePort/IP peers confirm `primarySource.tcpsPort` |
| Stale DNS or old NLB IP after recreate | Resolve the ExternalDNS hostname to the current LoadBalancer address before apply; refresh `connectString` / `tcpsPort` to match |
| Database ready but no mapped service | Read `v$active_services` on the primary; complete the auto-registration prerequisites or perform the primary-side association manually |
| `ORA-27369`, `No such file or directory`, or pending association | Inspect the primary helper log and non-interactive environment, then use the manual helper command as the Oracle software owner |
| Recreate uses an old endpoint | Wait for the old SIDB, Service, LoadBalancer, PVC, and ExternalDNS records to converge before reusing the name; verify the DNS A and ExternalDNS owner records |

## Manually publish the True Cache blob ConfigMap

Use this fallback only when the primary does **not** publish the ConfigMap —
for example, `trueCache.generateBlob: true` and
`trueCache.createConfigMap: false`. The normal samples keep both fields enabled
and do not need this section.

`generateBlob: true` makes the operator create the archive in the primary Pod.
It does **not** copy that archive into the namespace where you create True
Cache. The steps below copy the archive and create the required ConfigMap in
the **True Cache namespace**.

Set the source and target values. They may be different for a remote primary:

```sh
export PRIMARY=sidb-sample
export PRIMARY_NS=default
export TRUECACHE_NS=default
export BLOB_PATH=/tmp/tc_config_blob.tar.gz
export BLOB=/tmp/tc_config_blob.tar.gz
export BLOB_CONFIGMAP="$PRIMARY-truecache-blob"
```

If the primary is in another cluster, first select its kubeconfig as shown in
[Remote primary TCP or TCPS](#3-remote-primary-tcp-or-tcps). Then confirm the
archive exists and copy it from the primary Pod. Replace `BLOB_PATH` when the
primary uses a custom `trueCache.generatePath`:

```sh
PRIMARY_POD=$(kubectl get pod -n "$PRIMARY_NS" -l "app=$PRIMARY" \
  -o jsonpath='{.items[0].metadata.name}')
kubectl exec -n "$PRIMARY_NS" "$PRIMARY_POD" -- test -s "$BLOB_PATH"
kubectl cp "$PRIMARY_NS/$PRIMARY_POD:$BLOB_PATH" "$BLOB"
```

Select the True Cache cluster kubeconfig when it differs, then create the
ConfigMap in the namespace where you will apply the True Cache YAML:

```sh
kubectl create configmap "$BLOB_CONFIGMAP" \
  --from-file=tc_config_blob.tar.gz="$BLOB" \
  -n "$TRUECACHE_NS" --dry-run=client -o yaml | kubectl apply -f -
kubectl get configmap "$BLOB_CONFIGMAP" -n "$TRUECACHE_NS"
rm -f "$BLOB"
```

Before applying True Cache, set
`spec.trueCache.blobConfigMapRef: <BLOB_CONFIGMAP>` and keep
`spec.trueCache.blobConfigMapKey: tc_config_blob.tar.gz`. Do not create an
empty or hand-written archive. If `generateBlob` is also disabled, enable it
on an SIDB primary first; for a non-Kubernetes primary, generate the archive
with [Remote host SI or RAC, step 4.1](#41-primary-host-create-the-blob).

## Advanced True Cache association

Use this section only when you need to change or troubleshoot how the primary
service is associated with True Cache. It is not part of the default deployment
path.

| When this applies | What you do | Details |
| --- | --- | --- |
| You want the default automatic association explained | Keep `autoTCServiceRegistration: true`. The helper creates or starts the primary PDB service and associates the True Cache service during provisioning. | [Automatic path](../prerequisites/PREREQUISITES.md#automatic-path-autotcserviceregistration-true) |
| Primary is an external host (SI or RAC) or uses a non-OCR image | Install `configure-primary-truecache-service.sh` on every primary node before automatic association can run. OCR `enterprise_ru` SIDB primaries already include it. | [Helper install](../prerequisites/PREREQUISITES.md#true-cache-helper-install) |
| You need a dedicated primary app service name, or RAC / `db_domain` needs the service created first | Create and start the primary PDB service yourself (`DBMS_SERVICE.CREATE_SERVICE`), then keep or turn off automatic association as needed. | [Create the primary PDB service manually](../prerequisites/PREREQUISITES.md#create-the-primary-pdb-service-manually-advanced) |
| You set `autoTCServiceRegistration: false` (or omit it) | After True Cache reports ready, run the association helper on the primary using the command printed in the True Cache pod logs. | [Manual True Cache service association](../prerequisites/PREREQUISITES.md#manual-true-cache-service-association) |

## Set `primarySource.dbName`

`primarySource.dbName` is optional. It does **not** create, rename, or select a
primary CDB. When set, it is the short primary CDB `DB_NAME` from
`SELECT name FROM v$database`.

| Primary path | What to do |
| --- | --- |
| Same-cluster SIDB using `primarySource.databaseRef` | Omit `dbName`. |
| Remote SIDB or host primary with a valid CDB service in `connectString` | Omit `dbName` unless your image or operator procedure explicitly requires the short CDB name. |
| External RAC primary with a domain-qualified SCAN service | Set `dbName` to the short CDB `DB_NAME` only when the image or operator procedure requires it. |

Never set it to a `DB_UNIQUE_NAME`, domain-qualified service, SCAN service, or
True Cache SID. `dbName` does not repair an invalid
`primarySource.connectString`; that value must always name a reachable primary
CDB service. If an explicit `dbName` makes bootstrap target an unregistered
service, remove it and retain the valid CDB service in the connect string.
Setting the short name can prevent the True Cache from inheriting the primary
`db_domain`; choose it only when that is required.
