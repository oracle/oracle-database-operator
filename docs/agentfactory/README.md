# Oracle AI Database Private Agent Factory Services

Oracle AI Database Operator for Kubernetes includes the `AgentFactory` controller for deploying the self-contained Oracle AI Database Private Agent Factory production image.

Use this guide when you need to deploy AgentFactory with:

- An Agent Factory image reference.
- A ready Oracle Database PDB, or a new SIDB created using the [SIDB Quick
  Start](../sidb/README.md#quick-start).
- Persistent storage for /mount. Use shared `ReadWriteMany` storage when running multiple replicas.

The PDB does not need to contain the PAF runtime users when you use automatic
schema provisioning. The users and required grants must already exist only
when you choose user-managed schema provisioning.

**Note:** Oracle Database version 23ai or later is required. The COMPATIBLE initialization parameter must be set to 23.0.0 or later.

## Contents
- [Before You Begin](#before-you-begin)
- [Quick Start](#quick-start)
- [Verify AgentFactory Deployment](#verify-agentfactory-deployment)
- [AgentFactory v4 Resource Model](#agentfactory-v4-resource-model)
- [OpenShift](#openshift)
- [Deployment Scenarios](#deployment-scenarios)
- [Database and Schema](#database-and-schema)
- [Runtime Configuration and LLM Providers](#runtime-configuration-and-llm-providers)
- [Storage](#storage)
- [Networking and TLS](#networking-and-tls)
- [Lifecycle and Updates](#lifecycle-and-updates)
- [Access Agent Factory](#access-agent-factory)
- [Operational Status and Cleanup](#operational-status-and-cleanup)
- [Troubleshooting and Known Issues](#troubleshooting-and-known-issues)
- [Additional Information](#additional-information)

## Before You Begin

> **Important — required prerequisite:** This section explains the shared
> requirements and configuration choices for AgentFactory deployment
> scenarios. Review every point and subsection before applying a manifest so
> you can tailor the YAML to the database, storage, networking, security, and
> lifecycle requirements of your environment.
>
> Each deployment workflow also identifies its mandatory, workflow-specific
> prerequisites. Complete those actions before applying that workflow's
> manifest.

The examples use OKE and OCI services. On another Kubernetes distribution or
cloud, provide equivalent storage, LoadBalancer, reserved-IP, DNS, IAM, and
TLS capabilities and replace OCI-specific annotations and commands.

The examples use namespace `paf`; replace it if your deployment uses a
different namespace. The commands are idempotent where possible.

**In this section:**

- [Create the namespace](#create-the-namespace)
- [Basic prerequisites](#basic-prerequisites)
- [Create the image pull secret](#create-the-image-pull-secret)
- [Create the SIDB Database Admin Secret](#create-the-sidb-database-admin-secret)
- [Create or Update the Runtime Secret](#create-or-update-the-runtime-secret)
- [Optional Wallet Password for an Encrypted Private Key](#optional-wallet-password-for-an-encrypted-private-key)
- [Create the Bootstrap Administrator Secret (Optional)](#create-the-bootstrap-administrator-secret-optional)
- [Secret Retention and Ownership](#secret-retention-and-ownership)
- [Storage and placement](#storage-and-placement)
- [Networking and access](#networking-and-access)
- [Resources and scaling](#resources-and-scaling)

### Create the namespace

```bash
PAF_NS=paf
kubectl create namespace "$PAF_NS" --dry-run=client -o yaml | kubectl apply -f -
```

### Basic prerequisites

Make sure the following are available:

1. Oracle AI Database Operator is installed and the operator watches the target
   namespace.
2. An Oracle Database 23ai or later PDB is available, with
   `COMPATIBLE` set to `23.0.0` or later and `MAX_STRING_SIZE` set to
   `EXTENDED`. AgentFactory 26.10.0 requires `MAX_STRING_SIZE=EXTENDED` in
   the target PDB. This is a database prerequisite, independent of schema
   provisioning and connectivity. The SIDB controller does not default this
   value; a new SIDB created without the following block reports
   `STANDARD`. The shared
   [`sidb_quickstart.yaml`](../../config/samples/sidb/sidb_quickstart.yaml)
   sample leaves `pdbParameters` commented so other SIDB use cases stay
   unchanged—uncomment it (or add the block below) when the SIDB backs
   AgentFactory:

   ```yaml
   spec:
     pdbParameters:
       - pdbName: orclpdb1
         name: MAX_STRING_SIZE
         value: EXTENDED
   ```

   Verify the value from the target PDB before applying AgentFactory. The
   query must return `EXTENDED`:

   ```sql
   SELECT name, value FROM v$parameter WHERE name = 'max_string_size';
   ```
3. An Agent Factory runtime image and credentials for its container registry.
4. Persistent storage for `/mount`. Use shared `ReadWriteMany` storage when
   running multiple Agent Factory replicas.
5. Decide how the database users will be prepared:
   - **Automatic schema provisioning:** provide the optional bootstrap administrator Secret
     and `spec.database.bootstrap.adminCredentials`. The controller creates or
     validates the PAF runtime user, its read-only companion, and required
     grants.
   - **User-managed setup:** do not provide administrator credentials. Create
     the PAF users and grants in the target PDB first, then provide only the
     runtime Secret to Agent Factory.
6. If you use the wallet sample, review [Storage](#storage) before
   continuing and pre-create the OCI FSS-backed shared PVC. The sample runs
   three replicas and references `agentfactory-shared-mount`, so the PVC must
   be `Bound` and provide shared `ReadWriteMany` access before you apply the
   sample.

### Create the image pull secret

**Recommended:** Use the published prebuilt images from [Oracle Container
Registry (OCR)](https://container-registry.oracle.com/). Configure registry
access by following [Pull Images from Oracle Container Registry](../registry/README.md),
then reference the image and its image-pull Secret in the AgentFactory
manifest.

Create a pull Secret in the same namespace as the `AgentFactory` resource. The
registry and credentials must match `spec.runtime.image.name`. This example is
for Oracle Container Registry:

```bash
PAF_NS=paf
PAF_IMAGE_PULL_SECRET=ocr-reg-cred

kubectl create secret docker-registry "$PAF_IMAGE_PULL_SECRET" \
  --docker-server=container-registry.oracle.com \
  --docker-username='<oracle-sso-email-address>' \
  --docker-password='<container-registry-auth-token>' \
  --docker-email='<oracle-sso-email-address>' \
  -n "$PAF_NS"
```

For an OCI Registry image, use the corresponding registry endpoint and
credentials instead. Set the same Secret name in the AgentFactory resource:

```yaml
spec:
  runtime:
    image:
      name: container-registry.oracle.com/database/applied-ai:latest
      pullSecret: ocr-reg-cred
```

### Create the SIDB Database Admin Secret

Create this Secret before applying `sidb_quickstart.yaml`. SIDB reads the
`oracle_pwd` key as the SYS password. The Secret must be in the same namespace
as the SIDB resource.

```bash
PAF_NS=paf

read -rsp 'SIDB SYS password: ' SIDB_SYS_PASSWORD
echo
kubectl create secret generic db-admin-secret -n "$PAF_NS" \
  --from-literal=oracle_pwd="$SIDB_SYS_PASSWORD" \
  --dry-run=client -o yaml | kubectl apply -f -
unset SIDB_SYS_PASSWORD
```

**Important:** When the Quick Start uses automatic AgentFactory schema
provisioning, create the separate
[Bootstrap Administrator Secret](#create-the-bootstrap-administrator-secret-optional)
with the same SYS password. Do not add AgentFactory bootstrap keys to this
SIDB Secret.

### Create or Update the Runtime Secret

The runtime Secret is always required. Choose the Secret layout that matches
the database connection mode. Each example below creates or updates the
Secret.

For the Quick Start and other Details-mode deployments without a wallet,
create the Secret with the UI administrator password and the PAF database
schema password. The UI administrator password must meet the AgentFactory
password policy (include at least one special character). Bootstrap fails
with `Password does not contain special character` when it does not:

```bash
PAF_NS=paf

kubectl create secret generic agentfactory-runtime-secret -n "$PAF_NS" \
  --from-literal=paf_ui_admin_password='<ui-admin-password>' \
  --from-literal=paf_db_password='<paf-user-password>' \
  --dry-run=client -o yaml | kubectl apply -f -
```

For Wallet mode, add the client wallet archive as `db_wallet.zip`:

```bash
WALLET_ZIP=<path-to-client-wallet.zip>

kubectl create secret generic agentfactory-runtime-secret -n "$PAF_NS" \
  --from-literal=paf_ui_admin_password='<ui-admin-password>' \
  --from-literal=paf_db_password='<paf-user-password>' \
  --from-file=db_wallet.zip="$WALLET_ZIP" \
  --dry-run=client -o yaml | kubectl apply -f -
```

#### Optional Wallet Password for an Encrypted Private Key

If the wallet archive contains an encrypted private key (for example,
`ewallet.pem`), add its password to the same runtime Secret. This field is
optional and is not the PAF schema password or the SIDB administrator password.
The key name in the Kubernetes Secret is user-selected; in this example it is
`wallet-password-prod`:

```bash
kubectl create secret generic agentfactory-runtime-secret -n "$PAF_NS" \
  --from-literal=paf_ui_admin_password='<ui-admin-password>' \
  --from-literal=paf_db_password='<paf-user-password>' \
  --from-file=db_wallet.zip="$WALLET_ZIP" \
  --from-literal=wallet-password-prod='<wallet-password>' \
  --dry-run=client -o yaml | kubectl apply -f -
```

For the required Wallet mode fields and `walletPasswordKey` configuration, see
[Wallet-Based Database Connectivity](#wallet-based-database-connectivity).

Do not add `db_wallet.zip` when using Details/TCP connectivity.

### Create the Bootstrap Administrator Secret (Optional)

Create this Secret only when the controller performs automatic schema
provisioning. The Quick Start uses a separate bootstrap Secret with the `sys`
and `password` keys:

```bash
read -rsp 'Database administrator password: ' DB_ADMIN_PASSWORD
echo
kubectl create secret generic paf-db-bootstrap-admin -n "$PAF_NS" \
  --from-literal=sys=SYS \
  --from-literal=password="$DB_ADMIN_PASSWORD" \
  --dry-run=client -o yaml | kubectl apply -f -
unset DB_ADMIN_PASSWORD
```

For this Quick Start, the AgentFactory administrator password must be the same
SYS password stored in the SIDB `db-admin-secret`. The Secrets remain separate:
SIDB reads `oracle_pwd`, while AgentFactory reads `sys` and `password`.

### Secret Retention and Ownership

The administrator Secret referenced by SIDB must be retained while the SIDB
exists because the SIDB controller may need it for later reconciliation. The
runtime Secret is also retained because the running application needs these
credentials.

### Storage and placement

AgentFactory requires persistent storage at `/mount`. Use `ReadWriteOnce` for
a single replica and shared `ReadWriteMany` storage, such as OCI FSS, when
multiple replicas must access the same content. Plan the storage class,
availability domain, PVC ownership, and `deletePvcOnDelete` behavior before
deployment. See [Storage](#storage) and
[Storage Allocation](#storage-allocation).

### Networking and access

The default ClusterIP Service supports in-cluster access and port-forwarding.
Enable a public or private LoadBalancer only when clients need access through
an external endpoint. A LoadBalancer may require provider-specific subnet or
reserved-IP annotations, and its IP is allocated asynchronously. See
[Networking and TLS](#networking-and-tls) and [Access Agent Factory](#access-agent-factory).

### Resources and scaling

Set `spec.runtime.resources` and `spec.runtime.replicas` only after
confirming that the selected nodes have the required capacity. Use either
`spec.runtime.workerNodeSelector` for a durable label-based placement policy or
`spec.runtime.workerNodes` for a fixed hostname allow-list; these fields are
mutually exclusive. Use shared `ReadWriteMany` storage before increasing
replicas. Changes to resources, image, placement, or replicas reconcile the
Deployment and may recycle Pods. See [Deployment Options](#lifecycle-and-updates).

## Quick Start

This path creates one AgentFactory deployment with a ClusterIP Service and a
controller-managed database schema setup Job. It creates the SIDB and
AgentFactory in the same namespace, uses Details/TCP database connectivity
without a wallet, and does not create a public LoadBalancer.

> **Before proceeding:** Review [Before You Begin](#before-you-begin),
> complete each required action, and review its linked reference before
> applying a manifest.
>
> **OpenShift:** finish the [OpenShift Quick Start checklist](#openshift-quick-start-checklist)
> first (SIDB SCC, AgentFactory SA/`anyuid`, `mountLocation`, StorageClass,
> and Secrets). Do not apply the samples below until those steps succeed.

**Required prerequisites and example names**

| Requirement | Values used by this Quick Start | Required action |
| --- | --- | --- |
| Target namespace | `paf` | **[Create the namespace](#create-the-namespace)** |
| OCR image-pull Secret | `ocr-reg-cred` | Configure OCR access by following **[Pull Images from OCR](../registry/README.md)**, then **[create the Secret](#create-the-image-pull-secret)**. |
| SIDB database-admin Secret | `db-admin-secret` with `oracle_pwd` | **[Create the SIDB administrator Secret](#create-the-sidb-database-admin-secret)** |
| AgentFactory runtime Secret | `agentfactory-runtime-secret` with `paf_ui_admin_password` and `paf_db_password` | **[Create or update the runtime Secret](#create-or-update-the-runtime-secret)** |
| AgentFactory bootstrap administrator Secret | `paf-db-bootstrap-admin` with `sys` and `password` | **[Create the bootstrap administrator Secret](#create-the-bootstrap-administrator-secret-optional)** |
| Operator and database | Database Operator watches `paf`; Oracle Database 23ai+ PDB with `COMPATIBLE >= 23.0.0` | **[Confirm the prerequisites](#basic-prerequisites)** |
| PDB parameter | `MAX_STRING_SIZE=EXTENDED` in the target PDB | Uncomment `pdbParameters` in [`sidb_quickstart.yaml`](../../config/samples/sidb/sidb_quickstart.yaml) (or add the block); see **[Basic prerequisites](#basic-prerequisites)** |
| Persistent `/mount` storage | `oci-bv`, `100Gi`, `ReadWriteOnce` for one replica | **[Review storage and placement](#storage-and-placement)**; on OpenShift see **[OpenShift sample field changes](#openshift-sample-field-changes)** |
| OpenShift only | SIDB write-permissions SCC + AgentFactory SA/`anyuid` + Secret `mountLocation` (for example `/mnt/af-secrets`) | Complete the **[OpenShift Quick Start checklist](#openshift-quick-start-checklist)** before applying SIDB or AgentFactory. UI admin password must include a special character. |

**In this section:**

- [Apply the SIDB Quick Start Manifest](#apply-the-sidb-quick-start-manifest)
- [Check the SIDB status](#check-the-sidb-status)
- [Apply the AgentFactory Quick Start Manifest](#apply-the-agentfactory-quick-start-manifest)

### Apply the SIDB Quick Start Manifest

Complete [Create the SIDB Database Admin Secret](#create-the-sidb-database-admin-secret),
then review and apply
[`sidb_quickstart.yaml`](../../config/samples/sidb/sidb_quickstart.yaml).

**Important:** Before applying the sample, change `metadata.namespace` from
`default` to `paf` (or your target namespace). Ensure that `db-admin-secret`
and `ocr-reg-cred` exist in that namespace and retain the sample's
`oracle_pwd` and image-pull Secret references. For AgentFactory, uncomment
the sample's `pdbParameters` block (or add it) so the target PDB is created
with `MAX_STRING_SIZE=EXTENDED` (required for AgentFactory 26.10.0+; omitting
it leaves `STANDARD`). If the cluster is not OKE, replace
`storageClass: oci-bv` with a supported RWO StorageClass for your
environment. On OpenShift, complete the
[OpenShift Quick Start checklist](#openshift-quick-start-checklist) first,
then apply your edited copy with `serviceAccountName: sidb-sa`,
`setWritePermissions: true` (when using the write-permissions SCC), a
supported RWO StorageClass, and uncommented `pdbParameters` for
`MAX_STRING_SIZE=EXTENDED`.

```bash
kubectl apply -f config/samples/sidb/sidb_quickstart.yaml
```

### Check the SIDB status

Do not apply AgentFactory until the SIDB reports `Healthy`. Set `PAF_NS` to
the namespace used in the SIDB manifest, then inspect and wait for the
`SingleInstanceDatabase` resource by type:

```bash
PAF_NS=paf
SIDB_NAME=sidb-sample

kubectl -n "$PAF_NS" get singleinstancedatabase "$SIDB_NAME" -o wide
kubectl -n "$PAF_NS" get pod,pvc -l app="$SIDB_NAME"
kubectl wait --for=jsonpath='{.status.status}'=Healthy \
  singleinstancedatabase/"$SIDB_NAME" -n "$PAF_NS" --timeout=30m
kubectl -n "$PAF_NS" get singleinstancedatabase "$SIDB_NAME" \
  -o jsonpath='{.status.status}{"\n"}'
```

On OpenShift, after the SIDB Pod exists, confirm it selected the SIDB SCC
sample (for write-permissions this is commonly
`sidb-write-permissions-scc`):

```bash
kubectl -n "$PAF_NS" get pod -l app="$SIDB_NAME" \
  -o custom-columns='NAME:.metadata.name,SA:.spec.serviceAccountName,SCC:.metadata.annotations.openshift\.io/scc'
```

### Apply the AgentFactory Quick Start Manifest

Review [`agentfactory_quickstart.yaml`](./provisioning/agentfactory_quickstart.yaml).
Its database host and service name match the SIDB Quick Start, and it uses a
ClusterIP Service for local port-forwarding.

**Important:** Before applying the sample, update its namespace, storage
class, image, and Secret names for your environment. The referenced runtime
and bootstrap administrator Secrets must already exist in the same namespace.
On OpenShift, after the
[AgentFactory ServiceAccount and SCC](#agentfactory-serviceaccount-and-scc)
checks succeed, set `spec.security.secret.mountLocation` (for example
`/mnt/af-secrets`) and, when the CRD serves the field,
`spec.runtime.serviceAccountName: agentfactory-sa`.

After the SIDB status check reports `Healthy`, apply AgentFactory:

```bash
kubectl apply -f docs/agentfactory/provisioning/agentfactory_quickstart.yaml
```

Continue with [Verify AgentFactory Deployment](#verify-agentfactory-deployment)
after applying the manifest. The controller creates and validates `paf_user`,
its read-only companion, and the required grants before reporting the database
ready.

## Verify AgentFactory Deployment

After applying an AgentFactory manifest, wait for the database setup Job (when
bootstrap is enabled), the generated Deployment, and the AgentFactory status
before accessing the UI.

```bash
PAF_NS=paf
AF=agentfactory-sample

kubectl -n "$PAF_NS" get agentfactory "$AF"
kubectl -n "$PAF_NS" get job,deploy,pod,svc,pvc \
  -l app.kubernetes.io/instance="$AF"
kubectl -n "$PAF_NS" get agentfactory "$AF" \
  -o jsonpath='phase={.status.phase} ready={.status.ready}{"\n"}'
kubectl -n "$PAF_NS" get agentfactory "$AF" \
  -o jsonpath='{range .status.conditions[*]}{.type}={.status} reason={.reason}{"\n"}{end}'
```

For automatic schema provisioning, the `<name>-db-setup` Job must be
`Complete`. AgentFactory is ready when `status.phase` is `Ready`, the
runtime Pod is `1/1 Running`, `DatabaseReady=True`, and `WorkloadReady=True`.
If it is not ready, inspect the resource, the setup Job, and recent events:

```bash
kubectl -n "$PAF_NS" describe agentfactory "$AF"
kubectl -n "$PAF_NS" logs job/"$AF"-db-setup -c paf-db-setup
kubectl -n "$PAF_NS" get events --sort-by=.lastTimestamp
```

On OpenShift, after the Deployment and Pods exist, confirm the ServiceAccount
and the SCC selected for the AgentFactory workload. Use the
`app.kubernetes.io/instance` label (the resource name). The
`app.kubernetes.io/name` label is the fixed value `agentfactory`:

```bash
kubectl get deployment "$AF" -n "$PAF_NS" \
  -o jsonpath='{.spec.template.spec.serviceAccountName}{"\n"}'
kubectl get pods -n "$PAF_NS" -l app.kubernetes.io/instance="$AF" \
  -o custom-columns='NAME:.metadata.name,READY:.status.containerStatuses[0].ready,SA:.spec.serviceAccountName,SCC:.metadata.annotations.openshift\.io/scc'
kubectl get pods -n "$PAF_NS" -l app.kubernetes.io/instance="$AF" -o wide
```

The Pod `SA` column must show `agentfactory-sa` when
`spec.runtime.serviceAccountName` is set and served by the CRD; otherwise it
shows `default`. The `SCC` column shows the constraint OpenShift selected
(expect `anyuid` for Applied AI / AgentFactory images). Confirm the runtime
Secret mount path is the value you set (for example `/mnt/af-secrets`), not
`/run/secrets`:

```bash
kubectl -n "$PAF_NS" get deploy "$AF" \
  -o jsonpath='{range .spec.template.spec.containers[0].volumeMounts[*]}{.name}={.mountPath}{"\n"}{end}'
```

If SIDB runs in the same namespace, also confirm the SIDB Pod SCC as shown in
[Check the SIDB status](#check-the-sidb-status). AgentFactory and SIDB use
different ServiceAccounts and different SCCs. If verification fails, use
[OpenShift: common failures](#openshift-common-failures).

## AgentFactory v4 Resource Model

Use the current grouped v4 fields to describe the desired AgentFactory
workload. The controller creates and reconciles the Deployment, Services,
managed PVCs, runtime ConfigMap, optional database setup Job, and status; do
not edit those generated resources directly.

| Spec area | Configure it for |
| --- | --- |
| `runtime` | Image, pull Secret, replicas, resources, placement, non-secret environment variables, and workload or Pod annotations. |
| `configuration` | UI administrator username, runtime behavior, client certificate identity, proxy settings, timeouts, and the optional LLM provider. |
| `database` | Details or Wallet database connectivity, runtime schema user, health path, and optional one-time schema bootstrap. |
| `security` | Runtime credentials, inbound HTTPS certificate and key, and outbound CA bundle. |
| `storage` | Persistent `/mount` and optional auxiliary volumes using an existing PVC, controller-managed PVC, or `emptyDir`. |
| `networking` | HTTP/HTTPS listeners plus generated ClusterIP, public LoadBalancer, and private LoadBalancer Services. |
| `upgrade` | Rollout, recreate, or one-off image upgrade behavior. |

**Important:** Start new manifests from the
[annotated AgentFactory CRD field reference](./reference/agentfactory-crd-reference.yaml).
Use only current fields. `spec.networking.service.portMappings`,
`spec.networking.service.name`, and `spec.networking.service.svcType` are
deprecated; use `service.ports` and the `cluster`, `publicLoadBalancer`, or
`privateLoadBalancer` blocks instead. `status` is controller-owned.

`spec.logging` remains present in the API but is not reconciled by the
AgentFactory controller. Do not rely on it to configure logging sidecars or
runtime behavior.

## OpenShift

Use this section when you deploy AgentFactory (with or without a new SIDB) on
OpenShift. Complete every checklist step before [Quick Start](#quick-start).
On plain Kubernetes or OKE, skip this section.

The operator does not create Security Context Constraints (SCCs). The
`AgentFactory` API has no `sccName` field. OpenShift customers must complete
**two separate SCC paths** when they also create SIDB, plus OpenShift-specific
Secret mount and StorageClass edits.

> **Secret path conflict (OpenShift only):** AgentFactory defaults
> `spec.security.secret.mountLocation` to `/run/secrets`. OpenShift and
> RHEL-based images already use `/run/secrets` for platform files (`rhsm`
> and related content). Leaving the default causes missing Secret files and
> CrashLoop. Set a free path such as `/mnt/af-secrets` before you apply
> AgentFactory.

| Workload | Required OpenShift setup | Details |
| --- | --- | --- |
| SIDB database Pod (when you create SIDB) | SIDB OpenShift SCC sample + `serviceAccountName: sidb-sa` | **[SIDB Deployment on OpenShift](../sidb/README.md#sidb-deployment-on-openshift)**. SIDB adds capability `SYS_NICE`. Binding only `anyuid` or `restricted-v2` fails SIDB admission. |
| AgentFactory runtime Pod | Dedicated SA (or `default` on older CRDs) + **`anyuid`** (typical) + Secret `mountLocation` outside `/run/secrets` | [AgentFactory ServiceAccount and SCC](#agentfactory-serviceaccount-and-scc). The image needs its own UID to write install paths; `restricted-v2` random UIDs often fail. |

If the target PDB already exists and is Healthy, skip the SIDB SCC steps and
still complete the AgentFactory SA/SCC and `mountLocation` steps.

**In this section:**

- [OpenShift Quick Start checklist](#openshift-quick-start-checklist)
- [OpenShift sample field changes](#openshift-sample-field-changes)
- [AgentFactory ServiceAccount and SCC](#agentfactory-serviceaccount-and-scc)
- [Optional: pin AgentFactory Pods to nodes](#optional-pin-agentfactory-pods-to-nodes)

### OpenShift Quick Start checklist

Follow this order. Do not apply SIDB or AgentFactory until each step succeeds.

1. **Create the namespace** and confirm the operator watches it.

```sh
PAF_NS=paf
OPERATOR_NS=oracle-database-operator-system

kubectl create namespace "$PAF_NS" --dry-run=client -o yaml | kubectl apply -f -

# Confirm paf (or your namespace) appears in the comma-separated list.
kubectl -n "$OPERATOR_NS" get deploy oracle-database-operator-controller-manager \
  -o jsonpath='{.spec.template.spec.containers[?(@.name=="manager")].env[?(@.name=="WATCH_NAMESPACE")].value}{"\n"}'
```

If the namespace is missing from `WATCH_NAMESPACE`, update the operator scope
before continuing:
[Managing Installation Scope](../installation/OPERATOR_SCOPE_MANAGEMENT.md).

2. **Prepare SIDB OpenShift RBAC/SCC** before applying any SIDB CR (skip this
   step only when you use an existing Healthy PDB).

   This AgentFactory OpenShift Quick Start uses `setWritePermissions: true`,
   so apply the write-permissions profile. If you keep
   `setWritePermissions` false or omitted, use
   [`openshift_rbac.yaml`](../../config/samples/sidb/openshift_rbac.yaml)
   instead. Apply **one** profile only. Full SIDB guidance:
   [SIDB Deployment on OpenShift](../sidb/README.md#sidb-deployment-on-openshift).

```sh
PAF_NS=paf
PROFILE=config/samples/sidb/openshift_rbac_write_permissions.yaml

kubectl delete rolebinding use-sidb-oracle-user-scc \
  use-sidb-write-permissions-scc -n "$PAF_NS" --ignore-not-found
sed "s/sidb-ns/${PAF_NS}/g" "$PROFILE" | kubectl apply -f -

kubectl auth can-i use securitycontextconstraints/sidb-write-permissions-scc \
  --as="system:serviceaccount:${PAF_NS}:sidb-sa" -n "$PAF_NS"
```

The `can-i` command must return `yes`. That SCC allows UID `0`-`54321` and
`SYS_NICE`.

3. **Prepare the AgentFactory ServiceAccount and SCC** using
   [AgentFactory ServiceAccount and SCC](#agentfactory-serviceaccount-and-scc)
   (create SA → `kubectl explain` for `serviceAccountName` → grant `anyuid` →
   set `mountLocation: /mnt/af-secrets`).
4. **Copy and edit the Quick Start YAMLs** for OpenShift using
   [OpenShift sample field changes](#openshift-sample-field-changes). Work from
   copies of:
   - [`config/samples/sidb/sidb_quickstart.yaml`](../../config/samples/sidb/sidb_quickstart.yaml)
   - [`docs/agentfactory/provisioning/agentfactory_quickstart.yaml`](./provisioning/agentfactory_quickstart.yaml)

   At minimum set: namespace, StorageClass, SIDB `serviceAccountName` /
   `setWritePermissions`, uncomment SIDB `pdbParameters` for
   `MAX_STRING_SIZE=EXTENDED`, AgentFactory Secret `mountLocation`, image-pull
   settings, and (when your CRD supports it) AgentFactory `serviceAccountName`.
5. **Create the Secrets** in `$PAF_NS` (links open the exact commands):
   - Image pull Secret — [Create the image pull secret](#create-the-image-pull-secret)
   - SIDB admin Secret — [Create the SIDB Database Admin Secret](#create-the-sidb-database-admin-secret)
   - AgentFactory runtime Secret — [Create or Update the Runtime Secret](#create-or-update-the-runtime-secret)
     (UI admin password **must** include a special character)
   - Bootstrap admin Secret — [Create the Bootstrap Administrator Secret](#create-the-bootstrap-administrator-secret-optional)
     (same SYS password as the SIDB admin Secret for this Quick Start)
6. **Apply and verify** using [Quick Start](#quick-start):
   apply SIDB → wait until `Healthy` → apply AgentFactory →
   [Verify AgentFactory Deployment](#verify-agentfactory-deployment).

**Success looks like:** SIDB `status=Healthy`; AgentFactory `status.phase=Ready`
with a Pod `1/1 Running`; OpenShift SCC annotations show
`sidb-write-permissions-scc` on the SIDB Pod and `anyuid` on the AgentFactory
Pod; Secret volume mount path is `/mnt/af-secrets` (or the path you set).

List objects by API resource type. `kubectl get all` omits custom resources,
PVCs, ServiceAccounts, and Secrets:

```sh
kubectl -n "$PAF_NS" get singleinstancedatabase,agentfactory,pod,pvc,sa,secret
```

### OpenShift sample field changes

Published samples keep OCR images and OKE example values so they stay
customer-portable. On OpenShift, copy the sample and change these fields
before apply.

| Sample field | Published example | Change on OpenShift |
| --- | --- | --- |
| `metadata.namespace` | `default` (SIDB) / `paf` (AgentFactory) | Use your target namespace. The operator `WATCH_NAMESPACE` list must include it. **[Create the namespace](#create-the-namespace)** |
| SIDB `spec.persistence.oradata.storageClass` | `oci-bv` | Replace with a supported RWO StorageClass (for example `rook-ceph-block`). **[Storage and placement](#storage-and-placement)** |
| AgentFactory `/mount` StorageClass | `oci-bv` (one replica) | Replace with a supported StorageClass. Use RWX/shared storage when `replicas > 1`. **[Storage](#storage)** |
| Image pull Secret | `ocr-reg-cred` | Keep OCR with `ocr-reg-cred`, or change both the image reference and Secret for your registry. The Secret must contain a non-empty registry auth. Omit `pullSecret` only when the registry allows anonymous pulls. **[Create the image pull secret](#create-the-image-pull-secret)** |
| SIDB `spec.serviceAccountName` | unset | Set `sidb-sa` after applying the SIDB OpenShift SCC sample. **[SIDB Deployment on OpenShift](../sidb/README.md#sidb-deployment-on-openshift)** |
| SIDB `spec.persistence.setWritePermissions` | unset / `false` in some samples | Set `true` only with [`openshift_rbac_write_permissions.yaml`](../../config/samples/sidb/openshift_rbac_write_permissions.yaml). **[OpenShift Quick Start checklist](#openshift-quick-start-checklist)** |
| AgentFactory `spec.runtime.serviceAccountName` | unset in the base sample | Set `agentfactory-sa` when the installed CRD serves this field. Older operator builds reject the field and use the namespace `default` ServiceAccount—authorize that SA instead. **[AgentFactory ServiceAccount and SCC](#agentfactory-serviceaccount-and-scc)** |
| AgentFactory `spec.security.secret.mountLocation` | unset (image default `/run/secrets`) | Set a path such as `/mnt/af-secrets` on OpenShift. Mounting Secrets at `/run/secrets` conflicts with RHEL/`rhsm` and platform files. **[AgentFactory ServiceAccount and SCC](#agentfactory-serviceaccount-and-scc)** |
| SIDB `spec.pdbParameters` | Commented example in [`sidb_quickstart.yaml`](../../config/samples/sidb/sidb_quickstart.yaml) | Uncomment / set `MAX_STRING_SIZE=EXTENDED` for AgentFactory. Omitting it leaves `STANDARD`. Keep the shared SIDB sample commented for non-AgentFactory SIDB users. **[Basic prerequisites](#basic-prerequisites)** |
| LoadBalancer annotations | OCI LB annotations in some samples | Replace with your cloud-provider annotations, or use ClusterIP plus an OpenShift Route. **[Networking and TLS](#networking-and-tls)** |

Example SIDB edits:

```yaml
spec:
  serviceAccountName: sidb-sa
  persistence:
    oradata:
      storageClass: <openshift-rwo-storageclass>
      accessMode: ReadWriteOnce
    # Requires openshift_rbac_write_permissions.yaml when true.
    setWritePermissions: true
```

Example AgentFactory edits:

```yaml
spec:
  runtime:
    # Include only when the installed AgentFactory CRD serves this field.
    serviceAccountName: agentfactory-sa
  security:
    secret:
      name: agentfactory-runtime-secret
      mountLocation: /mnt/af-secrets
      uiAdminPasswordKey: paf_ui_admin_password
      dbPasswordKey: paf_db_password
  storage:
    volumes:
      - name: app-mount
        mountPath: /mount
        source:
          pvc:
            storageClass: <openshift-storageclass>
```

### AgentFactory ServiceAccount and SCC

Complete these steps before you apply the `AgentFactory` resource. They do
not replace the SIDB OpenShift SCC steps.

1. Create a dedicated ServiceAccount:

```sh
PAF_NS=paf
kubectl create serviceaccount agentfactory-sa -n "$PAF_NS" \
  --dry-run=client -o yaml | kubectl apply -f -
```

2. Confirm whether the installed CRD serves `spec.runtime.serviceAccountName`:

```sh
kubectl explain agentfactory.spec.runtime.serviceAccountName
```

- If the field exists, edit the AgentFactory manifest to set it. This is a
  CR field, not a standalone object to apply with `kubectl apply`:

```yaml
spec:
  runtime:
    serviceAccountName: agentfactory-sa
```

- If `kubectl explain` reports that the field does not exist, the installed
  operator build is older than the current API. Pods use the namespace
  `default` ServiceAccount. Authorize `default` for the approved SCC in the
  next step, and omit `serviceAccountName` from the manifest.

3. Authorize the ServiceAccount that the Pods will use. For Applied AI /
AgentFactory images, ask an OpenShift administrator to approve **`anyuid`**
so the container can run as the image user and write its install paths.
Use `restricted-v2` only when your security team requires it **and** the
image can run under a random OpenShift UID.

```sh
# AF_SA is agentfactory-sa when the CRD serves serviceAccountName; otherwise default.
AF_SA=agentfactory-sa

kubectl auth can-i use scc/anyuid \
  --as=system:serviceaccount:${PAF_NS}:${AF_SA} -n "$PAF_NS"
```

If the command returns `no`, ask an OpenShift administrator to grant the
approved SCC, then re-check:

```sh
# Run only when can-i returned no. Requires oc and cluster-admin (or equivalent).
oc adm policy add-scc-to-user anyuid -z "$AF_SA" -n "$PAF_NS"
```

If Pods fail with permission errors on install paths while using
`restricted-v2`, switch to administrator-approved `anyuid` (or another
approved broader SCC).

4. Set Secret `mountLocation` away from `/run/secrets` on OpenShift:

```yaml
spec:
  security:
    secret:
      name: agentfactory-runtime-secret
      mountLocation: /mnt/af-secrets
      uiAdminPasswordKey: paf_ui_admin_password
      dbPasswordKey: paf_db_password
```

The operator mounts Secret keys as files under that directory and sets
`AGENT_FACTORY_RUNTIME_SECRETS_DIR` to match. Leaving the default
`/run/secrets` on OpenShift commonly fails because RHEL images and the
platform already use that path.

5. Do **not** verify the Deployment or Pods yet. Those objects exist only
after you apply AgentFactory. Use the OpenShift checks in
[Verify AgentFactory Deployment](#verify-agentfactory-deployment).

If a first OpenShift attempt fails bootstrap and leaves
`BOOTSTRAP_RETRYABLE=false` on the `/mount` PVC, delete the AgentFactory
resource and its app-mount PVC, fix SCC/`mountLocation`/password policy,
then apply again on a fresh volume. See
[OpenShift: common failures](#openshift-common-failures).

#### Optional: pin AgentFactory Pods to nodes

Only after the OpenShift checklist and Quick Start succeed, you may add
placement settings to the same AgentFactory manifest. Confirm node placement
only after Pods exist.

```yaml
spec:
  runtime:
    # Fixed node hostnames.
    workerNodes:
      - worker-a
      - worker-b

    # Or durable label-based placement.
    # workerNodeSelector:
    #   agentfactory.oracle.com/pool: paf
```

The operator renders required node affinity and reconciles changes to
`workerNodes` and `workerNodeSelector`.

## Deployment Scenarios

The Quick Start above is a complete end-to-end path. For other deployment
designs, first complete [Before You Begin](#before-you-begin), then select the
closest scenario from the matrix below. Before applying a production manifest,
review the sections that match that scenario:

- **[Database and Schema](#database-and-schema):** Select schema provisioning
  and Details or Wallet database connectivity.
- **[Runtime Configuration and LLM Providers](#runtime-configuration-and-llm-providers):**
  Confirm the runtime image, credentials, external provider, and proxy settings.
- **[Storage](#storage):** Plan `/mount` persistence, replica access mode, and
  any additional volumes.
- **[Networking and TLS](#networking-and-tls):** Select the client endpoint,
  LoadBalancer exposure, DNS, and TLS identity.
- **[Lifecycle and Updates](#lifecycle-and-updates):** Confirm resources,
  placement, replica count, and rollout strategy.

These are deployment patterns, not a fixed set of product limitations. Schema
provisioning, database connectivity, storage, replica count, and service
exposure are independent choices. Start with the closest YAML and change the
values for your namespace, database, image registry, StorageClass or PVC,
secrets, and network.

### Deployment scenario matrix

| # | Scenario | When to choose | Schema | Database connectivity | Storage and replicas | Service and access |
| --- | --- | --- | --- | --- | --- | --- |
| 1 | [Quick Start](#quick-start) | Use the standard one-replica deployment with automatic schema setup and local access. | Automatic | Details/TCP | Controller-managed RWO block volume; one replica | ClusterIP and port-forward |
| 2 | [User-managed schema](#deploy-agentfactory-with-a-user-managed-schema) | Use when the DBA has already created the PAF users and grants. | DBA-managed | Details/TCP | Controller-managed RWO block volume; one replica | ClusterIP and port-forward |
| 3 | [Dynamic shared storage](#deploy-agentfactory-with-dynamic-shared-storage) | Use for multiple replicas when the controller should create the shared FSS-backed storage. | Automatic or DBA-managed | Details/TCP | Controller-created RWX FSS; multiple replicas | ClusterIP or LoadBalancer |
| 4 | [Existing shared storage](#deploy-agentfactory-with-existing-shared-storage) | Use when a shared RWX PVC already exists and should remain user-owned. | Automatic or DBA-managed | Details/TCP | User-created RWX PVC; multiple replicas | ClusterIP or LoadBalancer |
| 5 | [Wallet/TCPS with automatic schema](#deploy-agentfactory-with-wallettcps-and-automatic-schema) | Use when database access requires a wallet/TCPS connection and the controller should create the schema. | Automatic | Wallet/TCPS | Existing shared RWX PVC; multiple replicas | ClusterIP or LoadBalancer |
| 6 | [Wallet/TCPS with user-managed schema](#deploy-agentfactory-with-wallettcps-and-user-managed-schema) | Use when wallet/TCPS is required and the DBA owns the schema and grants. | DBA-managed | Wallet/TCPS | Existing shared RWX PVC; multiple replicas | ClusterIP or LoadBalancer |
| 7 | [Public LoadBalancer](#deploy-agentfactory-with-a-public-loadbalancer) | Use when clients outside the private network require a public endpoint. | Automatic or DBA-managed | Details/TCP | RWO or RWX as required by replica count | Public LoadBalancer; optional reserved public IP |
| 8 | [Private LoadBalancer](#deploy-agentfactory-with-a-private-loadbalancer) | Use when clients require an externally reachable endpoint restricted to a private network. | Automatic or DBA-managed | Details/TCP | RWO or RWX as required by replica count | Private LoadBalancer; optional reserved private IP |

Before using any workflow below, review [Before You Begin](#before-you-begin),
[Database and Schema](#database-and-schema), [Storage](#storage), and
[Networking and TLS](#networking-and-tls). Update each sample's namespace,
image, Secrets, database endpoint, storage, and service exposure for the
cluster.

#### Deploy AgentFactory with a User-Managed Schema

Use this workflow when the DBA creates and owns the PAF schema. The controller
manages the Deployment, ClusterIP Service, and one RWO block volume, but does
not create database users or run a bootstrap Job.

**Required prerequisites**

| Requirement | Example value | Complete before applying |
| --- | --- | --- |
| Namespace | `paf` | [Create the namespace](#create-the-namespace) and confirm that the operator watches it. |
| Database schema | `paf_user` and `AAI_RO_paf_user` | Follow [Create Schema for User-Managed Provisioning](#create-schema-for-user-managed-provisioning). |
| Runtime Secret | `agentfactory-runtime-secret` | [Create or Update the Runtime Secret](#create-or-update-the-runtime-secret) with `paf_ui_admin_password` and `paf_db_password`. |
| Image pull Secret | `oracle-container-registry-secret` | [Create the image pull secret](#create-the-image-pull-secret) in the target namespace. |
| Database endpoint | `sidb-sample.sidb:1521/prodpdb` | Review [Details Connection](#details-connection) and update the host and PDB service. |
| StorageClass | `oci-bv` | Review [Storage Allocation](#storage-allocation) and verify RWO block-volume provisioning. |

Key fields are `spec.database.connectionType: Details`,
`spec.database.username: paf_user`, `spec.database.details`, the runtime
Secret under `spec.security.secret`, one replica, and the RWO PVC under
`spec.storage.volumes`. Omit `spec.database.bootstrap` and do not provide
administrator credentials.

Apply the reviewed [`agentfactory-sample.yaml`](./provisioning/agentfactory-sample.yaml):

```bash
kubectl apply -f docs/agentfactory/provisioning/agentfactory-sample.yaml
```

Follow [Verify AgentFactory Deployment](#verify-agentfactory-deployment). Since
bootstrap is disabled, do not expect a `db-setup` Job.

#### Deploy AgentFactory with Dynamic Shared Storage

Use this workflow when the controller should create the shared storage needed
by multiple replicas.

**Required prerequisites**

| Requirement | Example value | Complete before applying |
| --- | --- | --- |
| Namespace | `paf` | [Create the namespace](#create-the-namespace) and confirm that the operator watches it. |
| Image and runtime Secrets | OCR image, `oracle-container-registry-secret`, `agentfactory-runtime-secret` | [Create the image pull secret](#create-the-image-pull-secret) and [Create or Update the Runtime Secret](#create-or-update-the-runtime-secret). |
| Database connectivity | Details/TCP endpoint | Review [Details Connection](#details-connection) and update the host, port, and PDB service. |
| Schema provisioning | Automatic or DBA-managed | Follow [Automatic Schema Provisioning](#automatic-schema-provisioning) or [User-Managed Schema Provisioning](#user-managed-schema-provisioning). |
| Dynamic FSS storage | RWX StorageClass | Complete [New File System](#new-file-system), including OCI FSS CSI, IAM, availability-domain, mount-target, and network prerequisites. |
| Replicas | More than one | Confirm the selected StorageClass supports `ReadWriteMany` before increasing `spec.runtime.replicas`. |
| Service exposure | ClusterIP or LoadBalancer | Review [Service Exposure](#service-exposure) and configure the intended endpoint. |

Start with [`agentfactory-sample-create-fss-pvc.yaml`](./provisioning/agentfactory-sample-create-fss-pvc.yaml).
Review [New File System](#new-file-system), set the target namespace and
storage-class parameters, and use `ReadWriteMany` before setting
`spec.runtime.replicas` greater than one. Select automatic or user-managed
schema provisioning and configure the matching database fields.

Apply the manifest:

```bash
kubectl apply -f docs/agentfactory/provisioning/agentfactory-sample-create-fss-pvc.yaml
```

If an AgentFactory with the same name already exists, use a clean test name or
namespace. The controller does not replace an existing PVC when its
StorageClass or access mode changes, so verify that the PVC is newly created
with the requested FSS StorageClass and `ReadWriteMany` access.

Follow [Verify AgentFactory Deployment](#verify-agentfactory-deployment). Since
this sample omits `spec.database.bootstrap`, do not expect a `db-setup` Job.
For this workflow, also confirm that the generated PVC is `Bound`, uses
`fss-dyn-storage`, and provides `ReadWriteMany` access.

#### Deploy AgentFactory with Existing Shared Storage

Use this workflow when a shared RWX PVC already exists and remains owned by the
user.

**Required prerequisites**

| Requirement | Example value | Complete before applying |
| --- | --- | --- |
| Namespace | `paf` | [Create the namespace](#create-the-namespace) and confirm that the operator watches it. |
| Image and runtime Secrets | OCR image, `oracle-container-registry-secret`, `agentfactory-runtime-secret` | [Create the image pull secret](#create-the-image-pull-secret) and [Create or Update the Runtime Secret](#create-or-update-the-runtime-secret). |
| Database connectivity | Details/TCP endpoint | Review [Details Connection](#details-connection) and update the host, port, and PDB service. |
| Schema provisioning | Automatic or DBA-managed | Follow [Automatic Schema Provisioning](#automatic-schema-provisioning) or [User-Managed Schema Provisioning](#user-managed-schema-provisioning). |
| Existing shared PVC | Bound `ReadWriteMany` PVC | Review [Existing File System](#existing-file-system), pre-create the PVC, and set `existingPVC.claimName`; retain ownership of the PVC. |
| Replicas | More than one | Confirm the PVC supports concurrent access and size `spec.runtime.replicas` accordingly. |
| Service exposure | ClusterIP or LoadBalancer | Review [Service Exposure](#service-exposure) and configure the intended endpoint. |

Start with [`agentfactory-sample-use-existing-fss-update-replicas.yaml`](./provisioning/agentfactory-sample-use-existing-fss-update-replicas.yaml).
Review [Existing File System](#existing-file-system), set the namespace,
claim name, database mode, runtime Secret, image-pull Secret, and replica
count, then apply:

```bash
kubectl apply -f docs/agentfactory/provisioning/agentfactory-sample-use-existing-fss-update-replicas.yaml
```

Verify the PVC is `Bound` and that the Deployment uses the intended number of
replicas. Do not assume the controller deletes a user-provided PVC.

#### Deploy AgentFactory with Wallet/TCPS and Automatic Schema

Use this workflow for wallet-based TCPS connectivity when the controller must
create or validate the PAF schema.

**Required prerequisites**

| Requirement | Example value | Complete before applying |
| --- | --- | --- |
| Namespace | `paf` | [Create the namespace](#create-the-namespace) and confirm that the operator watches it. |
| Image and runtime Secrets | OCR image, `oracle-container-registry-secret`, `agentfactory-runtime-secret` | [Create the image pull secret](#create-the-image-pull-secret) and [Create or Update the Runtime Secret](#create-or-update-the-runtime-secret). |
| Wallet and TCPS | `db_wallet.zip`, valid TCPS alias and port | Review [Wallet-Based Database Connectivity](#wallet-based-database-connectivity) and [Configure Wallet Mode](#configure-wallet-mode); verify the alias and optional wallet-password key. |
| Bootstrap administrator Secret | `paf-db-bootstrap-admin` | [Create the Bootstrap Administrator Secret](#create-the-bootstrap-administrator-secret-optional) with credentials valid for the target PDB. |
| Shared storage | Existing `ReadWriteMany` PVC | Review [Existing File System](#existing-file-system), validate the PVC, and confirm it supports multiple replicas. |
| Service exposure | ClusterIP or LoadBalancer | Review [Service Exposure](#service-exposure) and configure the intended endpoint. |

Start with [`agentfactory-sample-wallet-bootstrap.yaml`](./provisioning/agentfactory-sample-wallet-bootstrap.yaml).
Review [Wallet-Based Database Connectivity](#wallet-based-database-connectivity)
and [Configure Wallet Mode](#configure-wallet-mode), verify the selected alias,
TCPS port, and optional encrypted-wallet password key, then apply:

```bash
kubectl apply -f docs/agentfactory/provisioning/agentfactory-sample-wallet-bootstrap.yaml
```

The bootstrap setup Job must complete before readiness is reported.

#### Deploy AgentFactory with Wallet/TCPS and User-Managed Schema

Use this workflow when the DBA has already created the PAF users and grants and
AgentFactory connects through a wallet and TCPS. Leave bootstrap disabled.

**Required prerequisites**

| Requirement | Example value | Complete before applying |
| --- | --- | --- |
| Namespace | `paf` | [Create the namespace](#create-the-namespace) and confirm that the operator watches it. |
| PAF schema | `paf_user` and `AAI_RO_paf_user` | Follow [Create Schema for User-Managed Provisioning](#create-schema-for-user-managed-provisioning). |
| Image and runtime Secrets | OCR image, `oracle-container-registry-secret`, `agentfactory-runtime-secret` | [Create the image pull secret](#create-the-image-pull-secret) and [Create or Update the Runtime Secret](#create-or-update-the-runtime-secret). |
| Wallet and TCPS | `db_wallet.zip`, valid TCPS alias and port | Review [Wallet-Based Database Connectivity](#wallet-based-database-connectivity) and [Configure Wallet Mode](#configure-wallet-mode); verify the alias and runtime password. |
| Shared storage | Existing `ReadWriteMany` PVC | Review [Existing File System](#existing-file-system), validate the PVC, and confirm it supports multiple replicas. |
| Service exposure | ClusterIP or LoadBalancer | Review [Service Exposure](#service-exposure) and configure the intended endpoint. |

Start with [`agentfactory-sample-wallet.yaml`](./provisioning/agentfactory-sample-wallet.yaml).
Review [Create Schema for User-Managed Provisioning](#create-schema-for-user-managed-provisioning),
[Wallet-Based Database Connectivity](#wallet-based-database-connectivity), and
[Configure Wallet Mode](#configure-wallet-mode), then apply:

```bash
kubectl apply -f docs/agentfactory/provisioning/agentfactory-sample-wallet.yaml
```

Follow [Verify AgentFactory Deployment](#verify-agentfactory-deployment). No
bootstrap Job should be created.

#### Deploy AgentFactory with a Public LoadBalancer

Use this workflow when clients require a public endpoint.

**Required prerequisites**

| Requirement | Example value | Complete before applying |
| --- | --- | --- |
| Namespace | `paf` | [Create the namespace](#create-the-namespace) and confirm that the operator watches it. |
| Image and runtime Secrets | OCR image, `oracle-container-registry-secret`, `agentfactory-runtime-secret` | [Create the image pull secret](#create-the-image-pull-secret) and [Create or Update the Runtime Secret](#create-or-update-the-runtime-secret). |
| Database connectivity | Details/TCP endpoint | Review [Details Connection](#details-connection) and verify the host, port, protocol, and PDB service. |
| Schema provisioning | Automatic or DBA-managed | Follow [Automatic Schema Provisioning](#automatic-schema-provisioning) or [User-Managed Schema Provisioning](#user-managed-schema-provisioning). |
| Storage | RWO for one replica or RWX for multiple replicas | Review [Storage Allocation](#storage-allocation) and use a StorageClass or PVC matching the replica count. |
| Public LoadBalancer | Public subnet and optional reserved public IP | Review [Service Exposure](#service-exposure), verify OCI subnet/address requirements, and set [Certificate Identity and TLS](#certificate-identity-and-tls-termination) only for the actual client address. |

Start with [`agentfactory-sample-public-lb.yaml`](./provisioning/agentfactory-sample-public-lb.yaml).
Review [Service Exposure](#service-exposure) and [Certificate Identity and TLS](#certificate-identity-and-tls-termination),
choose automatic or user-managed schema provisioning, then apply:

```bash
kubectl apply -f docs/agentfactory/provisioning/agentfactory-sample-public-lb.yaml
```

Verify the allocated public address, DNS, certificate identity, and client
routing before exposing the service to users.

#### Deploy AgentFactory with a Private LoadBalancer

Use this workflow when clients require a private endpoint.

**Required prerequisites**

| Requirement | Example value | Complete before applying |
| --- | --- | --- |
| Namespace | `paf` | [Create the namespace](#create-the-namespace) and confirm that the operator watches it. |
| Image and runtime Secrets | OCR image, `oracle-container-registry-secret`, `agentfactory-runtime-secret` | [Create the image pull secret](#create-the-image-pull-secret) and [Create or Update the Runtime Secret](#create-or-update-the-runtime-secret). |
| Database connectivity | Details/TCP endpoint | Review [Details Connection](#details-connection) and verify the host, port, protocol, and PDB service. |
| Schema provisioning | Automatic or DBA-managed | Follow [Automatic Schema Provisioning](#automatic-schema-provisioning) or [User-Managed Schema Provisioning](#user-managed-schema-provisioning). |
| Storage | RWO for one replica or RWX for multiple replicas | Review [Storage Allocation](#storage-allocation) and use a StorageClass or PVC matching the replica count. |
| Private LoadBalancer | Private subnet and optional reserved private IP | Review [Service Exposure](#service-exposure), verify private routing and DNS, and set [Certificate Identity and TLS](#certificate-identity-and-tls-termination) only for the actual client address. |

Start with [`agentfactory-sample-private-lb.yaml`](./provisioning/agentfactory-sample-private-lb.yaml).
Review [Service Exposure](#service-exposure) and [Certificate Identity and TLS](#certificate-identity-and-tls-termination),
choose automatic or user-managed schema provisioning, then apply:

```bash
kubectl apply -f docs/agentfactory/provisioning/agentfactory-sample-private-lb.yaml
```

Verify the private address, DNS, certificate identity, and client routing from
the intended network.

## Database and Schema

The namespace, image-pull Secret, runtime Secret, and optional bootstrap
administrator Secret are prepared in [Before You Begin](#before-you-begin).
Choose one schema provisioning mode and one database connectivity mode for
each deployment.

**In this section:**

- [Schema Provisioning](#schema-provisioning)
- [Automatic Schema Provisioning](#automatic-schema-provisioning)
- [User-Managed Schema Provisioning](#user-managed-schema-provisioning)
- [Create Schema for User-Managed Provisioning](#create-schema-for-user-managed-provisioning)
- [Database Connectivity](#database-connectivity)
- [Details Connection](#details-connection)
- [Wallet-Based Database Connectivity](#wallet-based-database-connectivity)
- [Configure Wallet Mode](#configure-wallet-mode)

### Schema Provisioning

Schema provisioning and database connectivity are independent choices. Select
one schema mode and one connectivity mode for each AgentFactory deployment:

| Schema mode | Administrator credentials | Result |
| --- | --- | --- |
| Automatic schema provisioning | Required through `spec.database.bootstrap.adminCredentials` | The controller creates or validates the PAF runtime and read-only users and applies the baseline account grants through a setup Job. Apply the AgentFactory runtime grants manually as documented below. `MAX_STRING_SIZE=EXTENDED` remains a DBA/SIDB prerequisite. |
| User-managed schema provisioning | Not supplied to AgentFactory | The DBA creates and maintains the PAF users, AgentFactory grants, and required database prerequisites, including `MAX_STRING_SIZE=EXTENDED`. |

#### Automatic Schema Provisioning

Use this mode when the PAF runtime users and grants have not yet been created.
Supply `spec.database.bootstrap.adminCredentials`, as shown in the Quick
Start. The controller:

1. Validates that the referenced Secret exists and contains the configured username and password keys, and validates the non-secret connection privilege (`NORMAL` or `SYSDBA`).
2. Creates the setup Job.
3. Connects to the target PDB using the supplied administrator credentials and selected privilege.
4. Creates or validates the PAF runtime and read-only users and applies the baseline account grants. Apply the AgentFactory runtime grants manually as documented below before the workload uses those features.
5. After the setup Job succeeds, creates or reconciles the workload and reports database and workload readiness only after the PAF readiness endpoint succeeds.
6. After a successful setup Job, deletes the Kubernetes bootstrap Secret when `cleanupPolicy` is `DeleteOnSuccess`.

The setup Job is a one-time account and grant provisioning and validation step;
it is not a continuous application-schema migration controller. The setup Job
itself does not create or upgrade PAF application tables. Handle later
application-schema changes through the PAF product's supported
initialization/upgrade procedure or an explicit DBA procedure.

To connect as `SYS AS SYSDBA`, store only `SYS` as the username and select the
connection privilege in the AgentFactory resource:

```yaml
# Kubernetes Secret data:
stringData:
  sys: SYS
  password: <sys-password>

# AgentFactory spec:
database:
  bootstrap:
    enabled: true
    adminCredentials:
      secretName: paf-db-bootstrap-admin
      usernameKey: sys
      passwordKey: password
      privilege: SYSDBA
```

Never set the Secret username to `SYS AS SYSDBA`. `AS SYSDBA` is an Oracle
connection privilege and is passed to the setup executable separately.

The setup Job does not apply the AgentFactory-specific runtime grants required
by the current 26.10.0 image. Apply those grants manually using the procedure
in [Create Schema for User-Managed Provisioning](#create-schema-for-user-managed-provisioning)
before applying AgentFactory. It does not change database initialization
parameters. Set `MAX_STRING_SIZE=EXTENDED` in the target PDB before applying
AgentFactory.

#### User-Managed Schema Provisioning

Use this mode when the DBA has already created the PAF users, AgentFactory
grants, and required database prerequisites. The target PDB must have
`MAX_STRING_SIZE=EXTENDED`. Complete any additional product-specific
prerequisites separately. Do not provide administrator credentials to the
AgentFactory resource. Omit
`database.bootstrap` or set `bootstrap.enabled: false`.

In this mode:

1. Create the PAF runtime user, read-only companion, and required grants in the target PDB before applying the AgentFactory resource. Complete any additional product-specific prerequisites separately.
2. Create the runtime Secret with the PAF database password and UI administrator password. For Wallet mode, also include `db_wallet.zip` and the optional encrypted-wallet password key required by the selected wallet configuration.
3. Apply the AgentFactory resource without `adminCredentials`.
4. The controller skips the setup Job.
5. The PAF HTTPS `/agentFactory/v1/getAppState` endpoint validates that the user-created schema is usable.
6. AgentFactory becomes ready only when the Deployment is available and the readiness probe succeeds.

The manual SQL procedure follows in [Create Schema for User-Managed Provisioning](#create-schema-for-user-managed-provisioning).

##### Create Schema for User-Managed Provisioning

This section applies only when `database.bootstrap.enabled` is false or `database.bootstrap` is omitted. Agent Factory needs an Oracle Database PDB to store essential runtime information. If you already have a database, create the Agent Factory users in that existing PDB. If you do not have a pre-existing database, you can provision one with the `SingleInstanceDatabase` controller first. For SIDB provisioning and connection details, see the [SIDB controller documentation](../sidb/README.md), especially [Verify SIDB Deployment](../sidb/README.md#verify-sidb-deployment).

Create the Agent Factory runtime schema user and the read-only companion user in the target PDB, not in `CDB$ROOT`. Replace `<db-password>` with the password you want to use. Do not paste the example placeholder as-is.

```sql
-- AgentFactory 26.10.0 requires EXTENDED in the target PDB.
SELECT name, value FROM v$parameter WHERE name = 'max_string_size';

CREATE USER appuser IDENTIFIED BY "<db-password>" DEFAULT TABLESPACE USERS QUOTA unlimited ON USERS ACCOUNT UNLOCK;

GRANT CREATE SESSION, CREATE TABLE, CREATE SEQUENCE, CREATE TRIGGER, CREATE TYPE,
  CREATE PROCEDURE, CREATE VIEW, CREATE SYNONYM TO appuser;

GRANT READ, WRITE ON DIRECTORY DATA_PUMP_DIR TO appuser;
GRANT SELECT ON V_$PARAMETER TO appuser;
GRANT CREATE JOB TO appuser;
GRANT CTXAPP TO appuser;
GRANT EXECUTE ON CTXSYS.CTX_DDL TO appuser;
GRANT SELECT ON SYS.V_$SQL_PLAN TO appuser;
GRANT EXECUTE ON SYS.DBMS_AQ TO appuser;
GRANT EXECUTE ON SYS.DBMS_AQADM TO appuser;

CREATE USER AAI_RO_appuser IDENTIFIED BY "<db-password>" ACCOUNT UNLOCK;
GRANT CREATE SESSION TO AAI_RO_appuser;
```

The read-only user name must use the `AAI_RO_<runtime-user>` pattern. For the runtime user `appuser`, the read-only user is `AAI_RO_appuser`.

The grants above are required by the AgentFactory 26.10.0 runtime. If the
`max_string_size` query returns `STANDARD`, configure the PDB with
`MAX_STRING_SIZE=EXTENDED` before applying AgentFactory. This setting is not a
schema grant and is not created by user-managed provisioning.

Gather the database connection details before you create the `AgentFactory` resource:

| AgentFactory field | What to provide |
|---|---|
| `spec.database.details.host` | Database listener host. For a SIDB, use the host and port from the PDB connect string: `status.pdbConnectString` for TCP or `status.tcpsPdbConnectString` for TCPS. If Agent Factory connects from outside the cluster or namespace routing boundary, use a reachable NodePort, LoadBalancer, or DNS name. |
| `spec.database.details.port` | Database listener port, normally `1521` for TCP unless your service exposes a different port. |
| `spec.database.details.serviceName` | PDB service name, for example `prodpdb`. This must identify the PDB where you created `appuser` and `AAI_RO_appuser`. |
| `spec.database.username` | Runtime schema user, for example `appuser`. |

For a SIDB-created database, use SIDB status to find the connection strings:

```bash
kubectl -n <db-namespace> get singleinstancedatabase <sidb-name> \
  -o jsonpath='{ "clusterConnectString=" }{.status.clusterConnectString}{"\n"}{ "connectString=" }{.status.connectString}{"\n"}{ "pdbConnectString=" }{.status.pdbConnectString}{"\n"}{ "tcpsConnectString=" }{.status.tcpsConnectString}{"\n"}{ "tcpsPdbConnectString=" }{.status.tcpsPdbConnectString}{"\n"}'
```

For Agent Factory Details mode, use the PDB-specific status field:

- TCP in the same cluster: `status.pdbConnectString`.
- TCP through a reachable NodePort, LoadBalancer, or DNS name: the externally reachable `status.pdbConnectString`.
- TCPS: `status.tcpsPdbConnectString`, using `protocol: TCPS`.

The `clusterConnectString` and `connectString` fields, and their TCPS
equivalents without `Pdb`, identify the SIDB/CDB service rather than the PDB
service. Do not use them for Agent Factory unless the PAF users were created
on that service. For Wallet mode, use the wallet alias and `db_wallet.zip`
instead of parsing these status strings.

For example, if the reachable `status.pdbConnectString` is:

```text
sidb-sample.sidb.svc.cluster.local:1521/prodpdb
```

then set:

```yaml
database:
  connectionType: Details
  username: appuser
  details:
    protocol: TCP
    host: sidb-sample.sidb.svc.cluster.local
    port: 1521
    serviceName: prodpdb
```

**Note:** `serviceName` is the PDB service name for the PDB where the Agent Factory users were created.

### Database Connectivity

Connectivity is independent of schema provisioning. Choose either a direct
Details connection or a portable client wallet:

Both modes also require `spec.database.username`, the `spec.database.airGapped`
field, and a runtime Secret containing the configured UI administrator and PAF
database password keys. Wallet mode additionally requires the wallet archive.
Wallet mode must set `spec.database.airGapped: false`, and its `wallet` block
must provide `selectedTnsAlias` and `hasOciCertificates`.

| Connectivity mode | Required configuration | Typical use |
| --- | --- | --- |
| Details | `database.connectionType: Details` and `database.details` | Direct TCP or TCPS connection using host, port, protocol, and service name. |
| Wallet | `database.connectionType: Wallet`, `database.wallet`, and the runtime Secret wallet archive (`db_wallet.zip` by default) | TCPS connection where the client wallet supplies the TNS alias and TLS configuration. |

#### Details Connection

For Details mode, set `airGapped`, the protocol, listener host and port, and
PDB service name. The runtime Secret contains the PAF schema password; no
wallet archive is required:

```yaml
spec:
  database:
    connectionType: Details
    username: paf_user
    airGapped: false
    details:
      protocol: TCP
      host: <database-host>
      port: 1521
      serviceName: <pdb-service>
  security:
    secret:
      name: agentfactory-runtime-secret
      dbPasswordKey: paf_db_password
      uiAdminPasswordKey: paf_ui_admin_password
```

#### Wallet-Based Database Connectivity

Wallet is a database connection option and can be combined with either
automatic or user-managed schema provisioning. It does not create the PAF
schema by itself.

The wallet must be a portable client wallet. For a SIDB-managed database, use
the [SIDB TCPS Client Wallet procedure](../sidb/tcps-cert-manager/TCPS_CLIENT_WALLET.md) to
create `db_wallet.zip`. Set `WALLET_ZIP` to that archive. If the schema was
provisioned manually and a portable client wallet ZIP already exists, set
`WALLET_ZIP` to that archive. In both cases, inspect `tnsnames.ora` and confirm
that the selected alias uses `PROTOCOL=TCPS` and port `2484`.

Create or update `agentfactory-runtime-secret` with the wallet archive and,
when required, its private-key password; see [Create or Update the Runtime
Secret](#create-or-update-the-runtime-secret).

##### Configure Wallet Mode

Configure Wallet mode and select an alias that exists in `tnsnames.ora`:

```yaml
spec:
  database:
    connectionType: Wallet
    username: paf_user
    airGapped: false
    wallet:
      selectedTnsAlias: ORCLPDB1
      hasOciCertificates: false
    bootstrap:
      enabled: false
  security:
    secret:
      name: agentfactory-runtime-secret
      walletZipKey: db_wallet.zip
      # Optional Secret data key for an encrypted wallet private key.
      walletPasswordKey: wallet-password-prod
      dbPasswordKey: paf_db_password
      uiAdminPasswordKey: paf_ui_admin_password
```

Set `walletPasswordKey` only when the wallet private key is encrypted. The
controller maps the selected key to `agent_factory_db_wallet_password` in the
runtime Secret directory. With the default mount, the file is
`/run/secrets/agent_factory_db_wallet_password`; a custom `mountLocation` uses
that directory. The controller does not add the wallet password to `runtime.env`,
command arguments, status, or logs.

The controller mounts the runtime Secret at the configured Secret mount
location, `/run/secrets` by default. The schema setup Job and the Agent
Factory runtime use the wallet archive from `db_wallet.zip`; the runtime
database password remains necessary to authenticate `paf_user`. Set
`bootstrap.enabled: true` and provide `adminCredentials` when the controller
should create the schema over the wallet connection.

## Runtime Configuration and LLM Providers

The controller renders non-secret runtime settings into the generated
`<name>-runtime` ConfigMap as `runtime.env` and mounts it at
`/etc/agent-factory/runtime.env`. The runtime Secret is mounted separately at
`/run/secrets` by default. Keep passwords, API keys, private keys, wallet
archives, and CA material in Secrets; they are never rendered into
`runtime.env` or AgentFactory status.

Use `spec.configuration` for the UI administrator username, certificate
identity, proxy settings, timeouts, and LLM provider. Image-based AgentFactory
deployments support only `mode: prod` and require `silentInstall: true`.
`uiAdminUsername` must be a valid email address. If you configure an HTTP or
HTTPS proxy, `noProxy` must include `localhost`, `127.0.0.1`, and `::1`.

### Configure an LLM Provider

Set `spec.configuration.llm.provider` and populate the matching provider
block. The optional `llm.name` defaults to `llm_model_entry`. The normalized
provider value `openai_compatible` uses the `openaiCompatible` block, and
`private_ai_services` uses the `privateAiServices` block. File fields identify
keys mounted from the runtime Secret. Specify either an absolute path or a
filename relative to the runtime Secret mount.

| Provider | Required provider settings |
| --- | --- |
| `oci` | `modelId`, `serviceEndpoint`, and `compartmentId`; when OCI API-key fields are used, also provide `userOcid`, `fingerprintFile`, `tenancyOcid`, and `region`. `privateKeyFile` is optional and defaults to `agent_factory_llm_oci_private_key.pem`. |
| `vllm` | `modelId`, `host`, and `port`; `apiKeyFile` is optional. |
| `openai` | `modelId` and `apiKeyFile`. |
| `openai_compatible` | `modelId` and `baseUrl`; `apiKeyFile` is optional. |
| `private_ai_services` | `modelId` and `baseUrl`; `apiKeyFile` is optional. |
| `ollama` | `modelId`, `host`, and `port`. |
| `google` | `modelId` plus either `credentialsType: API_KEY` and `apiKeyFile`, or `credentialsType: GOOGLE_SERVICE_ACCOUNT`, `region`, and `serviceAccountFile`. |

For example, an OpenAI API key stored in the runtime Secret can be selected
without putting its value in the manifest:

```yaml
spec:
  configuration:
    llm:
      provider: openai
      openai:
        modelId: <model-id>
        apiKeyFile: openai-api-key
  security:
    secret:
      name: agentfactory-runtime-secret
```

## Storage

AgentFactory storage is provisioned through OCI File Storage Service (FSS) in
the examples below. Choose an existing file system when its lifecycle is
managed outside Kubernetes, or choose dynamic provisioning when the FSS CSI
driver should create the file system and PV for the PVC.

**Important:** AgentFactory requires a persistent volume at `/mount`.
`emptyDir` is valid only for auxiliary paths and `volumeClaimTemplate` is not
supported by the AgentFactory Deployment controller. A controller-managed PVC
is shared by all replicas; use `ReadWriteMany` storage before scaling above one
replica. Existing PVCs remain user-owned.

Choose one of these approaches:

| Approach | What gets created |
|---|---|
| Existing file system | You create the FSS file system, mount target, and export in OCI, then bind a static PV/PVC to them. |
| New file system | You create a StorageClass. When AgentFactory creates a PVC using it, the FSS CSI driver creates the file system and PV. |

**In this section:**

- [Common Prerequisites](#common-prerequisites)
- [Existing File System](#existing-file-system)
- [Create the FSS Resources in OCI](#create-the-fss-resources-in-oci)
- [Configure Network Access](#configure-network-access)
- [Find the Mount Target IP and Export Path](#find-the-mount-target-ip-and-export-path)
- [Create the Static PV and PVC](#create-the-static-pv-and-pvc)
- [Reference the PVC from AgentFactory](#reference-the-pvc-from-agentfactory)
- [Inspect Existing Data Before Reuse](#inspect-existing-data-before-reuse)
- [New File System](#new-file-system)
- [Configure OKE Cluster-Principal IAM Policies](#configure-oke-cluster-principal-iam-policies)
- [Create the StorageClass](#create-the-storageclass)
- [Reference the StorageClass in AgentFactory](#reference-the-storageclass-in-agentfactory)
- [Troubleshoot a PVC That Stays Pending](#troubleshoot-a-pvc-that-stays-pending)

### Common Prerequisites

Verify that the FSS CSI driver is registered and its pods are running:

```bash
kubectl get csidriver fss.csi.oraclecloud.com
kubectl get daemonset -A | grep -Ei 'fss|oci.*csi|csi.*oci'
kubectl get pods -A | grep -Ei 'fss|oci.*csi|csi.*oci'
```

The worker nodes must be able to reach the mount target. Configure the
applicable NSGs or security lists for the worker nodes and mount target. For
standard NFS access, OCI documents TCP ports `111`, `2048`, `2049`, and `2050`,
and UDP ports `111` and `2048`; limit sources and destinations to the relevant
node and mount-target networks. See [Configuring VCN Security Rules for File
Storage](https://docs.oracle.com/en-us/iaas/Content/File/Tasks/securitylistsfilestorage.htm).

### Existing File System

Use this option when the file system already exists or must be managed
separately from Kubernetes.

#### Create the FSS Resources in OCI

Create the file system, create or select an active mount target, then create
an export that associates the file system with that mount target. Record the
file system OCID, mount target OCID, and export path.

The cluster-principal policies described in [New File System](#new-file-system)
are not needed solely to mount a statically provisioned file system. The person
or automation creating the OCI resources still needs permission to create them.

#### Configure Network Access

Before testing the mount, configure the required FSS network rules for the
mount target and worker nodes. The required rules depend on whether they are
in the same or different subnets. See [Configuring VCN Security Rules for
File Storage](https://docs.oracle.com/en-us/iaas/Content/File/Tasks/securitylistsfilestorage.htm).

#### Find the Mount Target IP and Export Path

Use OCI CLI to find the mount target private IP and export path:

```bash
OCI_REGION=us-phoenix-1
MOUNT_TARGET_OCID=<mount-target-ocid>
FILE_SYSTEM_OCID=<file-system-ocid>
FSS_COMPARTMENT_OCID=<fss-compartment-ocid>

MOUNT_TARGET_PRIVATE_IP_OCID=$( \
  oci fs mount-target get \
    --mount-target-id "$MOUNT_TARGET_OCID" \
    --region "$OCI_REGION" \
    --query 'data."private-ip-ids"[0]' \
    --raw-output
)

MOUNT_TARGET_PRIVATE_IP=$( \
  oci network private-ip get \
    --private-ip-id "$MOUNT_TARGET_PRIVATE_IP_OCID" \
    --region "$OCI_REGION" \
    --query 'data."ip-address"' \
    --raw-output
)

oci fs export list \
  --compartment-id "$FSS_COMPARTMENT_OCID" \
  --file-system-id "$FILE_SYSTEM_OCID" \
  --region "$OCI_REGION" \
  --query 'data[].{exportPath:path,mountTargetId:"mount-target-id"}'
```

From the export list, select the path for the intended mount target. The PV
`volumeHandle` format is:

```text
<file-system-ocid>:<mount-target-private-ip>:<export-path>
```

The export path begins with `/`.

#### Create the Static PV and PVC

Use the file system OCID, mount target private IP, and export path to
construct the `spec.csi.volumeHandle` value in the `PersistentVolume`.

**Note:** The `volumeHandle` format is `<file-system-ocid>:<mount-target-private-ip>:<export-path>`.

Example static FSS `PersistentVolume` and matching `PersistentVolumeClaim`:

```yaml
---
apiVersion: v1
kind: PersistentVolume
metadata:
  name: agentfactory-shared-pv
spec:
  capacity:
    storage: 100Gi
  volumeMode: Filesystem
  accessModes:
    - ReadWriteMany
  persistentVolumeReclaimPolicy: Retain
  csi:
    driver: fss.csi.oraclecloud.com
    volumeHandle: <file-system-ocid>:<mount-target-private-ip>:<export-path>
---
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: agentfactory-shared-mount
  namespace: paf
spec:
  accessModes:
    - ReadWriteMany
  storageClassName: ""
  resources:
    requests:
      storage: 100Gi
  volumeName: agentfactory-shared-pv
```

Replace the `volumeHandle` placeholders with the actual OCID, IP, and export
path. For new static resources, apply the manifest, then check that the PV and
PVC are bound. If either example name already exists, inspect it first rather
than applying a different static binding over it:

```bash
kubectl apply -f agentfactory-static-fss.yaml
kubectl get pv agentfactory-shared-pv
kubectl get pvc -n paf agentfactory-shared-mount
```

#### Reference the PVC from AgentFactory

The PVC must be in the same namespace as the AgentFactory resource. Use the
`existingPVC` reference for the static FSS example:

```yaml
storage:
  volumes:
  - name: app-mount
    mountPath: /mount
    source:
      existingPVC:
        claimName: agentfactory-shared-mount
```

#### Inspect Existing Data Before Reuse

If the file system may contain data from an earlier deployment, inspect it
before reusing the PVC. Use the repository sample manifest at
`docs/agentfactory/provisioning/sample-pod.yaml`, after confirming it
references the intended PVC. If an AgentFactory workload is still using the
PVC, stop or quiesce it before modifying files. Only remove files after
confirming they are stale and safe to delete.

- [docs/agentfactory/provisioning/sample-pod.yaml](./provisioning/sample-pod.yaml)

Apply and inspect the FSS:

```bash
kubectl apply -f docs/agentfactory/provisioning/sample-pod.yaml
kubectl wait --for=condition=Ready pod/ol9-sample-pod -n paf --timeout=5m
kubectl exec -it -n paf ol9-sample-pod -- /bin/sh
```

Inside the sample pod:

```bash
ls -la /mount
```

After reviewing and confirming the data is stale, remove only the confirmed
stale paths; do not delete unknown data. Then exit and remove the temporary
pod:

```bash
exit
kubectl delete pod -n paf ol9-sample-pod
```

### New File System

Use this option when each PVC should cause the FSS CSI driver to create a new
file system and PV. You do not manually create the file system or export for
this workflow.

#### Configure OKE Cluster-Principal IAM Policies

Before creating the StorageClass or AgentFactory resource, grant the OKE
cluster principal permission to manage FSS resources and use the relevant
network resources:

```text
Allow any-user to manage file-family in compartment id <FSS_COMPARTMENT_OCID> where request.principal.type = 'cluster'
Allow any-user to use virtual-network-family in compartment id <NETWORK_COMPARTMENT_OCID> where request.principal.type = 'cluster'
```

If the file system, mount target, or network resources are in different
compartments, scope the statements to the compartments containing those
resources. A separate `read mount-targets` statement is generally redundant
when `manage file-family` already covers the mount-target compartment.

If the cluster uses a dynamic group rather than the OKE cluster principal, use
the equivalent approved dynamic-group policies instead of the
`request.principal.type = 'cluster'` condition.

Create the IAM policy through the tenancy home region. Set the OCI CLI
profile or `OCI_CLI_REGION` to that region before running the command below.

For example:

```bash
POLICY_COMPARTMENT_OCID=<tenancy-or-policy-parent-compartment-ocid>
FSS_COMPARTMENT_OCID=<fss-compartment-ocid>
NETWORK_COMPARTMENT_OCID=<network-compartment-ocid>

FSS_POLICY="Allow any-user to manage file-family in compartment id ${FSS_COMPARTMENT_OCID} where request.principal.type = 'cluster'"
NETWORK_POLICY="Allow any-user to use virtual-network-family in compartment id ${NETWORK_COMPARTMENT_OCID} where request.principal.type = 'cluster'"

oci iam policy create \
  --compartment-id "$POLICY_COMPARTMENT_OCID" \
  --name PrivateAiFssCsiProvisioning \
  --description "Permissions for Oracle AI Database Private Agent Factory OKE FSS CSI provisioning" \
  --statements "[\"$FSS_POLICY\",\"$NETWORK_POLICY\"]"
```

Wait for the policy to reach `ACTIVE`, then allow time for IAM propagation
before testing provisioning.

#### Create the StorageClass

Use an OCI availability-domain name, such as `PHX-AD-1`, not a region name
such as `us-phoenix-1`. The example uses an existing active mount target. If
the CSI driver should create the mount target, use its supported
`mountTargetSubnetOcid` parameter instead of `mountTargetOcid`.

```yaml
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  name: fss-dyn-storage
provisioner: fss.csi.oraclecloud.com
parameters:
  availabilityDomain: PHX-AD-1
  mountTargetOcid: <active-mount-target-ocid>
  # Optional: compartment for the new file system.
  # Defaults to the cluster's compartment.
  # compartmentOcid: <fss-compartment-ocid>

  # Optional. If set, this path must be unique on the mount target.
  # exportPath: /agentfactory-unique-path
reclaimPolicy: Retain
volumeBindingMode: Immediate
```

Leaving out `exportPath` lets the CSI driver generate a path. If you set it,
make sure the path is unique on that mount target. Apply the StorageClass
before creating the AgentFactory resource:

```bash
kubectl apply -f fss-dyn-storageclass.yaml
kubectl get storageclass fss-dyn-storage
```

Verify the CSI driver before creating the AgentFactory resource:

```bash
kubectl get csidriver fss.csi.oraclecloud.com
kubectl get daemonset -A | grep -Ei 'fss|oci.*csi|csi.*oci'
kubectl get pods -A | grep -Ei 'fss|oci.*csi|csi.*oci'
```

#### Reference the StorageClass in AgentFactory

Reference the storage class from the `AgentFactory` spec to create a PVC on a
new file system dynamically:

```yaml
  storage:
    volumes:
      - name: app-mount
        mountPath: /mount
        source:
          pvc:
            storageClass: fss-dyn-storage
            size: 100Gi
            accessModes:
              - ReadWriteMany
            deletePvcOnDelete: false
```

When the PVC is created, the CSI driver provisions a new PV and file system.
The requested `size` is required by Kubernetes, but it does not set an FSS
capacity limit; FSS creates the file system with its service-defined capacity
behavior.

`deletePvcOnDelete: false` keeps the claim when the AgentFactory resource is
deleted. The StorageClass `reclaimPolicy: Retain` also keeps the PV and backend
file system if the PVC is later deleted; cleanup then requires a deliberate
manual process. If you reuse an existing StorageClass, verify that its actual
`reclaimPolicy` is `Retain` before relying on this behavior.

#### Troubleshoot a PVC That Stays Pending

If the PVC remains `Pending`, inspect the CSI provisioning event and
controller logs:

```bash
kubectl -n paf describe pvc <agentfactory-name>-<volume-name>
kubectl -n paf get events --sort-by='.lastTimestamp'
kubectl get daemonset -A | grep -Ei 'fss|oci.*csi|csi.*oci'
kubectl get pods -A | grep -Ei 'fss|oci.*csi|csi.*oci'
```

After identifying the CSI controller or driver Pod from the preceding
commands, inspect its recent logs. The namespace and Pod name vary by OKE
version and CSI deployment:

```bash
kubectl -n <csi-namespace> logs <csi-pod> --all-containers --since=15m
```

An OCI `404 NotAuthorizedOrNotFound` response for `GetMountTarget` usually
means that the mount-target OCID is wrong, is in a different region or
tenancy, or the cluster principal lacks permission. Correct the OCID or IAM
policies first; the CSI provisioner normally retries the pending claim. Do
not delete a PVC to force a retry if it may contain data.

## Networking and TLS

**In this section:**

- [Service Exposure](#service-exposure)
- [Use Preallocated OCI IP Addresses](#use-preallocated-oci-ip-addresses)
- [Use an Address Allocated by the LoadBalancer](#use-an-address-allocated-by-the-loadbalancer)
- [Certificate Identity and TLS Termination](#certificate-identity-and-tls-termination)

### Service Exposure

Use `networking.service.ports` to define generated Service ports, and enable
the `cluster`, `publicLoadBalancer`, or `privateLoadBalancer` endpoint that
matches the client access path. Do not use deprecated
`networking.service.portMappings`, `networking.service.name`, or
`networking.service.svcType`; service names and Service types are
controller-managed by the current v4 API.


The service model separates the cluster service, public LoadBalancer, and
private LoadBalancer.

Default behavior creates only the in-cluster service:

```yaml
networking:
  service:
    cluster:
      enabled: true
    publicLoadBalancer:
      enabled: false
    privateLoadBalancer:
      enabled: false
```

Expose through a Public LoadBalancer:

```yaml
networking:
  service:
    ports:
    - name: https
      port: 443
      targetPort: 8080
      protocol: TCP
    cluster:
      enabled: true
    publicLoadBalancer:
      enabled: true
      ip: <reserved-public-ip-address>
      externalTrafficPolicy: Cluster
      annotations:
        service.beta.kubernetes.io/oci-load-balancer-subnet1: <public-subnet-ocid>
```

Expose through a Private LoadBalancer:

```yaml
networking:
  service:
    ports:
    - name: https
      port: 443
      targetPort: 8080
      protocol: TCP
    cluster:
      enabled: true
    privateLoadBalancer:
      enabled: true
      ip: <reserved-private-ip-address>
      externalTrafficPolicy: Cluster
      annotations:
        service.beta.kubernetes.io/oci-load-balancer-subnet1: <private-subnet-ocid>
        oci.oraclecloud.com/reserved-private-ips: <reserved-private-ip-ocid>
```

For `privateLoadBalancer`, the controller adds these defaults when the
annotations are not already supplied:

```text
oci.oraclecloud.com/load-balancer-type: "lb"
service.beta.kubernetes.io/oci-load-balancer-internal: "true"
```

On OCI, specify `service.beta.kubernetes.io/oci-load-balancer-subnet1` with
the OCID of the subnet where the LoadBalancer should be created. Use a public
subnet for `publicLoadBalancer` and a private subnet for
`privateLoadBalancer`. A cluster or cloud-controller default subnet may make
this annotation optional, but an explicit annotation makes the endpoint
configuration portable and avoids a Service remaining pending when no default
subnet is configured.

Use `publicLoadBalancer.annotations` and `privateLoadBalancer.annotations` independently when public and private services require different cloud-provider settings.

#### Use Preallocated OCI IP Addresses

Use a preallocated address when the endpoint must remain stable across Service
reconciliation or LoadBalancer replacement. Reserve or select the public IP
in OCI as described in [Creating Load Balancers in OCI](https://docs.oracle.com/en-us/iaas/Content/Balance/Tasks/managingloadbalancer_topic-Creating_Load_Balancers.htm),
then record its address and OCID.

Use the reserved public IP address in `publicLoadBalancer.ip`. If the OCI
cloud provider requires an OCID annotation to bind the reserved address, add
the reserved public IP OCID as shown below. The `ip` value is the address;
`oci.oraclecloud.com/reserved-ips` is the OCI resource OCID:

```yaml
networking:
  service:
    publicLoadBalancer:
      enabled: true
      ip: <reserved-public-ip-address>
      annotations:
        service.beta.kubernetes.io/oci-load-balancer-subnet1: <public-subnet-ocid>
        oci.oraclecloud.com/reserved-ips: <reserved-public-ip-ocid>
```

For a private LoadBalancer, reserve a private IP in the LoadBalancer subnet
using the OCI Console or the equivalent OCI CLI/API operation. Record both the
private IP address and its OCID. A private LoadBalancer cannot use a reserved
public IP. Configure the private address and, where required by the OCI cloud
provider, its OCID annotation:

```yaml
networking:
  service:
    privateLoadBalancer:
      enabled: true
      ip: <reserved-private-ip-address>
      annotations:
        service.beta.kubernetes.io/oci-load-balancer-subnet1: <private-subnet-ocid>
        oci.oraclecloud.com/reserved-private-ips: <reserved-private-ip-ocid>
```

The same reserved address can be used as `configuration.certIpAddress` when
clients connect to that LoadBalancer by IP. Do not use one public address for
both a public and private LoadBalancer; use the address belonging to the
endpoint that clients actually reach.

#### Use an Address Allocated by the LoadBalancer

If no address is reserved, omit `publicLoadBalancer.ip` or
`privateLoadBalancer.ip` and the corresponding reserved-IP annotation. The
OCI provider allocates an address asynchronously. After the Service receives
an address, retrieve it and update the AgentFactory certificate identity if
clients will connect by IP:

```bash
AF=agentfactory-sample
NAMESPACE=paf

PUBLIC_SERVICE=$(kubectl get agentfactory "$AF" -n "$NAMESPACE" \
  -o jsonpath='{.status.publicLoadBalancerService}')

LB_IP=$(kubectl get service "$PUBLIC_SERVICE" -n "$NAMESPACE" \
  -o jsonpath='{.status.loadBalancer.ingress[0].ip}')

kubectl patch agentfactory "$AF" -n "$NAMESPACE" --type=merge \
  -p "{\"spec\":{\"configuration\":{\"certIpAddress\":\"$LB_IP\"}}}"
```

For a private LoadBalancer, use
`.status.privateLoadBalancerService` instead of
`.status.publicLoadBalancerService` and run the same Service lookup and
certificate update.

Wait for the AgentFactory Deployment to reconcile after changing the
certificate identity. If the LoadBalancer receives a different IP later,
update DNS and repeat this certificate update. Prefer a reserved address, or
use a stable DNS name with `certFqdn` and omit `certIpAddress`, when the
LoadBalancer address may change.

### Certificate Identity and TLS Termination

`certFqdn` and `certIpAddress` are optional application certificate identity
settings. The controller renders them into the runtime configuration; they do
not reserve an OCI address, configure an OCI LoadBalancer listener
certificate, or contain certificate or private-key material. `certIpAddress`
must be one IPv4 or IPv6 literal, without a CIDR, brackets, port, hostname, or
list notation:

```yaml
configuration:
  certFqdn: paf.example.com
  certIpAddress: <reserved-or-allocated-load-balancer-ip>
```

Use `certFqdn` for a DNS name and `certIpAddress` for an IP address. Set both
when clients use both forms. Keep the certificate identity aligned with each
TLS leg:

- If the LoadBalancer terminates TLS, its client-facing certificate must cover
  the DNS name or IP address used by clients. The AgentFactory certificate is
  not presented on that client-facing connection. If the LoadBalancer
  re-encrypts traffic to the backend, its backend connection must separately
  trust a backend certificate whose identity matches the backend name it uses.
- If TLS passes through to AgentFactory, the AgentFactory inbound certificate
  must cover the DNS name or IP address used by clients through the
  LoadBalancer.

The controller does not create or upload an OCI LoadBalancer certificate. To
provide a certificate and private key for inbound HTTPS served by AgentFactory,
configure `security.inboundTLS` with a matching Secret in the AgentFactory
namespace. The certificate Secret and these identity settings are separate
configuration inputs.

For the complete AgentFactory certificate and TLS behavior, refer to the
[Oracle AI Database Private Agent Factory documentation](https://docs.oracle.com/en/database/oracle/agent-factory/).

## Lifecycle and Updates

Use these procedures to provision an AgentFactory or update an existing
AgentFactory resource. The controller reconciles the generated Deployment,
Services, runtime ConfigMap, setup Job when schema bootstrap is enabled, and
controller-managed PVCs as required. A claim referenced through `existingPVC`
is user-managed: the controller mounts it but does not create or delete it.

**Before updating an existing resource:** Review the target image, Secret
references, storage access mode, and client endpoint. The controller
reconciles the generated Deployment, Services, runtime ConfigMap, setup Job,
and controller-managed PVCs; update the `AgentFactory` resource instead of
editing those generated resources directly. For an `existingPVC` claim, update
the PVC through its owner or storage workflow and update only the AgentFactory
volume reference here.

| Change | Main current fields | Guidance |
| --- | --- | --- |
| Image and one-time application upgrade | `spec.runtime.image`, `spec.upgrade` | [Image Update](#image-update) |
| CPU and memory | `spec.runtime.resources` | [Configure CPU and Memory](#configure-cpu-and-memory) |
| Replicas and placement | `spec.runtime.replicas`, `spec.runtime.workerNodes`, or `spec.runtime.workerNodeSelector` | [Scale Up and Down](#scale-up-and-down) |
| Persistent or auxiliary volumes | `spec.storage.volumes` | [Storage Allocation](#storage-allocation) |
| Cluster or LoadBalancer access | `spec.networking.service` | [Networking and TLS](#networking-and-tls) |
| Workload or Pod annotations | `spec.runtime.annotations` | [AgentFactory CRD Field Reference](#agentfactory-crd-field-reference) |

**In this section:**

- [Image Update](#image-update)
- [ImageUpgrade Specification](#imageupgrade-specification)
- [ImageUpgrade Steps](#imageupgrade-steps)
- [Configure CPU and Memory](#configure-cpu-and-memory)
- [Storage Allocation](#storage-allocation)
- [Use an existing PVC](#use-an-existing-pvc)
- [Create a PVC dynamically](#create-a-pvc-dynamically)
- [Storage for Non-Persistent Auxiliary Paths](#storage-for-non-persistent-auxiliary-paths)
- [Scale Up and Down](#scale-up-and-down)
- [Update LoadBalancer Services](#update-loadbalancer-services)
- [Expose Through a Public LoadBalancer](#expose-through-a-public-loadbalancer)
- [Expose Through a Private LoadBalancer](#expose-through-a-private-loadbalancer)
- [Tune External Traffic Policy](#tune-external-traffic-policy)

### Image Update

When you update `spec.runtime.image.name` on an existing AgentFactory, the
default `RollingUpdate` strategy updates the Deployment and Kubernetes
performs the rollout. The cluster must have capacity and storage access for a
temporary surge Pod. The one-hour termination grace period gives an old Pod
time to handle `SIGTERM`; it does not drain traffic automatically.

Use `ImageUpgrade` only when the new image must run a one-time schema or
application upgrade before the normal AgentFactory workload starts. This is a
planned-downtime operation for an existing deployment. The new image must
support the fixed `upgrade` container argument and the upgrade must be
idempotent. Do not use this strategy for initial provisioning: the controller
first creates the normal Deployment, and the one-shot flow is used when an
existing Deployment image changes.

| Setting | Default | Image-update effect |
| --- | --- | --- |
| `spec.upgrade.strategy` | `RollingUpdate` | Replaces Pods gradually. |
| `spec.upgrade.rollingUpdate.maxSurge` | `25%` | Allows temporary extra Pods during the rollout. |
| `spec.upgrade.rollingUpdate.maxUnavailable` | `0` | Does not terminate an available old Pod until the replacement Pod is Ready. |
| `spec.upgrade.terminationGracePeriodSeconds` | `3600` seconds | Allows an old Pod up to one hour to exit after `SIGTERM`. |
| `spec.upgrade.configChangePolicy` | `Rollout` | ConfigMap changes trigger a rollout. |
| `spec.upgrade.secretChangePolicy` | `Rollout` | Secret changes trigger a rollout. |
| `spec.runtime.image.pullPolicy` | `IfNotPresent` | Use an immutable tag or digest; use `Always` if a mutable tag is unavoidable. |

#### ImageUpgrade Specification

Set `spec.upgrade.strategy: ImageUpgrade` and provide the following fields:

| Field | Default or constraint | Description |
| --- | --- | --- |
| `spec.upgrade.strategy` | Allowed: `RollingUpdate`, `Recreate`, `ImageUpgrade`; default `RollingUpdate` | Set to `ImageUpgrade` to enable the one-shot upgrade Pod. |
| `spec.upgrade.imageUpgrade.activeDeadlineSeconds` | `3600`; minimum `1` | Maximum time allowed for the upgrade Pod to complete. |
| `spec.upgrade.imageUpgrade.preserveFailedPod` | `true` | Retain a failed upgrade Pod for diagnosis. A successful upgrade Pod is always deleted. |
| `spec.runtime.image.name` | Required target image | Image used by both the upgrade Pod and the restored normal Deployment. |
| `spec.runtime.image.pullPolicy` | Existing image setting | Pull policy applied to the upgrade Pod as well as the normal workload. |
| `spec.runtime.image.pullSecret` | Existing image setting | Registry Secret used by the upgrade Pod when the target image is private. |
| `spec.runtime.replicas` | Existing requested replica count | Replica count restored after a successful upgrade. |

`imageUpgrade` is valid only when `strategy` is `ImageUpgrade`. The controller
uses the normal AgentFactory image, resources, volumes, Secret mounts, and
image-pull settings for the dedicated Pod, but runs it with the fixed
`upgrade` argument, `restartPolicy: Never`, and no normal readiness or
liveness probes. The dedicated Pod has labels isolated from the normal
Deployment and Services.

#### ImageUpgrade Steps

1. Review the target image and make sure it contains the upgrade command,
   supports the configured mounts and Secrets, and can safely be run again.
2. Apply the AgentFactory update with `strategy: ImageUpgrade` and the new
   image. The controller records the source and target images in
   `status.imageUpgrade`.
3. The controller scales the existing Deployment to zero and waits for all
   normal AgentFactory Pods to terminate. During this step the application is
   unavailable.
4. The controller creates `<agentfactory-name>-image-upgrade` using the new
   image and runs the fixed `upgrade` argument. It waits up to
   `activeDeadlineSeconds` for the Pod to complete.
5. When the upgrade Pod succeeds, the controller deletes it, restores the
   normal Deployment with the target image and requested replica count, and
   waits for the workload to become Ready.
6. When the upgrade Pod fails, the controller sets `ImageUpgradeReady=False`
   and `ReconcileError=True`. The failed Pod is retained by default when
   `preserveFailedPod: true`; set it to `false` if the failed Pod should be
   deleted after diagnosis is no longer needed. The normal Deployment remains
   stopped until the image or upgrade configuration is corrected in a new
   resource generation; reapplying the same failed specification does not
   retry the operation.

Example:

```yaml
spec:
  runtime:
    image:
      name: container-registry.oracle.com/database/applied-ai:latest
      pullPolicy: IfNotPresent
      pullSecret: agentfactory-image-pull
    replicas: 3
  upgrade:
    strategy: ImageUpgrade
    imageUpgrade:
      activeDeadlineSeconds: 3600
      preserveFailedPod: true
```

Check the operation with:

```bash
kubectl get agentfactory <name> -n <namespace> -o yaml
kubectl get pod <name>-image-upgrade -n <namespace>
kubectl logs -n <namespace> <name>-image-upgrade -c agentfactory
```

The controller reports `phase`, `sourceImage`, `targetImage`, `podName`,
`replicas`, `observedGeneration`, and a human-readable `message` under
`status.imageUpgrade`. `ImageUpgradeReady=True` means the upgrade Pod
completed and the normal workload was restored successfully. The successful
upgrade Pod is deleted automatically, so a `NotFound` result from the Pod
lookup after completion is expected. If the operation fails, inspect the
retained Pod and its logs before changing the image or configuration to create
a new generation and retry. Do not manually edit the
generated Deployment or upgrade Pod.

After a successful operation, the status normally looks like this (image
names, generation, message, and condition timestamps are examples):

```yaml
status:
  phase: Ready
  appState: ready
  imageUpgrade:
    phase: Succeeded
    sourceImage: <old-image>
    targetImage: <new-image>
    podName: <agentfactory-name>-image-upgrade
    replicas: 3
    observedGeneration: 12
    message: image upgrade succeeded; restored AgentFactory Deployment...
  conditions:
  - type: ImageUpgradeReady
    status: "True"
    reason: Succeeded
  - type: WorkloadReady
    status: "True"
  - type: ReconcileComplete
    status: "True"
  - type: ReconcileError
    status: "False"
```

`status.imageUpgrade.phase` remains `Succeeded` as the record of the latest
completed image upgrade. A later image upgrade starts a new status transition
through `Stopping` and `Running`; a failed operation reports `Failed` and
leaves `ImageUpgradeReady=False`.

### Configure CPU and Memory

Use [agentfactory-sample.yaml](./provisioning/agentfactory-sample.yaml) as the
starting manifest for provisioning an AgentFactory with explicit CPU and
memory requests and limits. The sample currently requests 2 CPUs and 8 GiB of
memory, and limits the workload to 4 CPUs and 16 GiB of memory:

To provision with different values, edit the `resources` block before applying
the manifest. For example:

```yaml
spec:
  runtime:
    resources:
      requests:
        cpu: "4"
        memory: 16Gi
      limits:
        cpu: "8"
        memory: 32Gi
```

Before applying the sample, complete these checks:

1. Create the image pull Secret referenced by `spec.runtime.image.pullSecret`.
2. Create the PAF schema/runtime Secret referenced by
   `spec.security.secret.name`, including the keys configured by
   `uiAdminPasswordKey` and `dbPasswordKey`.
3. Review the database host, port, service name, namespace, image, Secret
   names, and storage values for your environment.
4. If the cluster is not OKE, replace `storageClass: oci-bv` with a
   StorageClass available on that cluster. Also confirm that the selected
   access mode supports the requested replica count.

Apply the reviewed YAML:

```bash
kubectl apply -f docs/agentfactory/provisioning/agentfactory-sample.yaml
kubectl -n paf rollout status deploy/agentfactory-sample --timeout=30m
```

**Note:** During provisioning, the controller creates or updates the
generated workload and its dependent resources. When the resource settings
are changed later, existing Pod(s) may be recycled.

### Storage Allocation

Agent Factory requires persistent `/mount`. The controller supports several deployment storage sources.

#### Use an existing PVC

You can create a PVC in advance, for example by provisioning one using OCI File Storage Service (FSS) and mount it into the Deployment:

An `existingPVC` must already exist in the AgentFactory namespace. The PVC and
its backing storage remain user-owned; the controller mounts the claim but does
not create or delete it.

```yaml
storage:
  volumes:
  - name: app-mount
    mountPath: /mount
    source:
      existingPVC:
        claimName: agentfactory-shared-mount
```

Samples:

- [docs/agentfactory/provisioning/create_pvc_in_advance.yaml](./provisioning/create_pvc_in_advance.yaml)
- [docs/agentfactory/provisioning/agentfactory-sample-use-existing-fss.yaml](./provisioning/agentfactory-sample-use-existing-fss.yaml)

#### Create a PVC dynamically

First, prepare a new storage class using OCI FSS. Create one controller-managed shared PVC and mount it into the Deployment:

The `pvc` source creates the claim through the controller. Set
`deletePvcOnDelete: false` when the claim must remain after the AgentFactory
resource is deleted; the StorageClass reclaim policy determines the behavior
of the bound volume if the claim is later removed.

```yaml
storage:
  volumes:
  - name: app-mount
    mountPath: /mount
    source:
      pvc:
        storageClass: fss-dyn-storage
        size: 100Gi
        accessModes:
        - ReadWriteMany
        deletePvcOnDelete: false
```

Samples:

- [docs/agentfactory/provisioning/storage-class.yaml](./provisioning/storage-class.yaml)
- [docs/agentfactory/provisioning/agentfactory-sample-create-fss-pvc.yaml](./provisioning/agentfactory-sample-create-fss-pvc.yaml)

#### Storage for Non-Persistent Auxiliary Paths

Use `emptyDir` only for non-persistent auxiliary paths such as temporary logs. Do not use `emptyDir` for `/mount`. `emptyDir` is temporary Pod storage and is deleted when the Pod is removed.

```yaml
storage:
  volumes:
  - name: app-mount
    mountPath: /mount
    source:
      existingPVC:
        claimName: agentfactory-shared-mount
  - name: logs
    mountPath: /var/log/agentfactory
    source:
      emptyDir:
        sizeLimit: 20Gi
```

The `/mount` volume must not use `emptyDir`.

Sample:

- [docs/agentfactory/provisioning/agentfactory-sample-with-non-persistent-storage.yaml](./provisioning/agentfactory-sample-with-non-persistent-storage.yaml)

### Scale Up and Down

You can scale the number of Agent Factory Pods up or down by changing
`spec.runtime.replicas` to a positive desired count. The controller mounts the
declared `/mount` volume into every replica, so use shared `ReadWriteMany`
storage for every multi-replica deployment. The `oci-bv` StorageClass is
suitable for a single replica with `ReadWriteOnce`; use FSS or another
StorageClass that explicitly supports `ReadWriteMany` before increasing the
replica count.

Set `spec.runtime.replicas` to the desired count:

```yaml
spec:
  runtime:
    replicas: 2
```

Edit the manifest you want to use so that its `replicas` value is the desired
count, then apply it and watch the Deployment:

```bash
kubectl apply -f docs/agentfactory/provisioning/agentfactory-sample-use-existing-fss.yaml
kubectl -n paf get deploy agentfactory-sample
kubectl -n paf get pod -l app.kubernetes.io/instance=agentfactory-sample
kubectl -n paf rollout status deployment/agentfactory-sample --timeout=10m
```

The supplied `agentfactory-sample-use-existing-fss-update-replicas.yaml`
example sets `replicas: 3`; change that value when you need a different count.

**Note:** For multi-replica deployments, `/mount` must be shared by all
replicas. `ReadWriteOnce` is a single-node access mode. It may allow multiple
Pods on the same node to use a volume, but that is not a reliable multi-replica
design; placing worker nodes in the same availability domain does not change
the access mode to `ReadWriteMany`. Use FSS or another StorageClass that
explicitly supports `ReadWriteMany`.

To scale back down:

```yaml
spec:
  runtime:
    replicas: 1
```

Use a positive value for `spec.runtime.replicas`; setting it to `0` does not
provide scale-to-zero behavior because the controller defaults non-positive
values to one replica.

Samples:

- [docs/agentfactory/provisioning/agentfactory-sample-use-existing-fss.yaml](./provisioning/agentfactory-sample-use-existing-fss.yaml)
- [docs/agentfactory/provisioning/agentfactory-sample-use-existing-fss-update-replicas.yaml](./provisioning/agentfactory-sample-use-existing-fss-update-replicas.yaml)

### Update LoadBalancer Services

You can expose the Oracle AI Database Private Agent Factory service through a public or private LoadBalancer.

#### Expose Through a Public LoadBalancer

Enable the public LoadBalancer when clients need to reach Agent Factory from outside the cluster:

Use `publicLoadBalancer.annotations` when your cloud provider requires subnet, shape, or other load balancer settings.

Sample:

- [docs/agentfactory/provisioning/agentfactory-sample-public-lb.yaml](./provisioning/agentfactory-sample-public-lb.yaml)

#### Expose Through a Private LoadBalancer

Enable the private LoadBalancer when clients should reach Agent Factory through a private subnet:

For `privateLoadBalancer`, the controller supplies these annotations when they
are not already present:

```text
oci.oraclecloud.com/load-balancer-type: "lb"
service.beta.kubernetes.io/oci-load-balancer-internal: "true"
```

Sample:

- [docs/agentfactory/provisioning/agentfactory-sample-private-lb.yaml](./provisioning/agentfactory-sample-private-lb.yaml)

### Tune External Traffic Policy

Use `Cluster` for the default Kubernetes load balancing behavior:

```yaml
externalTrafficPolicy: Cluster
```

Use `Local` when you need client source IP preservation and understand the node-local routing implications:

```yaml
externalTrafficPolicy: Local
```

The controller accepts only `Cluster` or `Local` for Public and Private LoadBalancer services.

## Access Agent Factory

**In this section:**

- [Access using in-cluster service and port forward](#access-using-in-cluster-service-and-port-forward)
- [Access using LoadBalancer service](#access-using-loadbalancer-service)

### Access using in-cluster service and port forward

The default in-cluster Service name is the AgentFactory resource name:

```bash
kubectl -n paf get svc agentfactory-sample
kubectl -n paf port-forward svc/agentfactory-sample 8443:443
```

Open the local UI:

```text
https://127.0.0.1:8443/
```

### Access using LoadBalancer service

When a public or private LoadBalancer is enabled, check the generated service and status:

```bash
kubectl -n paf get agentfactory agentfactory-sample \
  -o jsonpath='{.status.publicLoadBalancerService}{"\n"}{.status.privateLoadBalancerService}{"\n"}{.status.loadBalancerIP}{"\n"}'

kubectl -n paf get svc
```

Select the enabled LoadBalancer Service and wait until its ingress address is
assigned:

```bash
LB_SERVICE=agentfactory-sample-private-lb
LB_IP=$(kubectl -n paf get service "$LB_SERVICE" \
  -o jsonpath='{.status.loadBalancer.ingress[0].ip}')
echo "$LB_IP"
kubectl -n paf get service "$LB_SERVICE"
```

Use the assigned load balancer IP or DNS name:

```text
https://<load-balancer-ip>/
```

For a private LoadBalancer, run the browser or client from a network that can reach the private subnet.

## Operational Status and Cleanup

Use these commands to verify the AgentFactory resource and generated Kubernetes objects:

```bash
kubectl -n paf get agentfactory agentfactory-sample
kubectl -n paf describe agentfactory agentfactory-sample
kubectl -n paf get job,deploy,svc,pvc,pod -l app.kubernetes.io/instance=agentfactory-sample
kubectl -n paf logs deploy/agentfactory-sample
```

For automatic schema provisioning, inspect the setup Job separately:

```bash
kubectl -n paf get job agentfactory-sample-db-setup
kubectl -n paf describe job agentfactory-sample-db-setup
kubectl -n paf logs job/agentfactory-sample-db-setup
```

The setup Job must report `Complete` before the AgentFactory Deployment can become ready. The controller does not print database passwords or render them in the runtime ConfigMap.

Useful status fields:

| Status field | Meaning |
|---|---|
| `status.phase` | High-level reconcile phase such as `Ready`, `Pending`, `Updating`, `Blocked`, or `Error`. |
| `status.appState` | Runtime application state reported by the controller. |
| `status.replicas` | Observed Deployment replica count. |
| `status.readyReplicas` | Ready Deployment replica count. |
| `status.localService` | In-cluster Service name. |
| `status.publicLoadBalancerService` | Public LoadBalancer Service name when enabled. |
| `status.privateLoadBalancerService` | Private LoadBalancer Service name when enabled. |
| `status.loadBalancerIP` | Reported load balancer IP when available. |
| `status.runtimeConfig.name` | Generated runtime ConfigMap name. |
| `status.secret.name` | Referenced runtime secret name. |
| `status.storage[]` | Resolved storage mounts and claim names. |
| `status.database.phase` | Schema provisioning and database validation phase: `Disabled`, `Pending`, `Running`, `Ready`, or `Failed`. |
| `status.database.bootstrapJobName` | Name of the one-time database setup Job when bootstrap is enabled. |
| `status.database.pafUser` | Runtime PAF schema user configured in `spec.database.username`. |
| `status.database.message` | Human-readable schema provisioning or database validation message. |

The `DatabaseReady` condition is true only when schema provisioning is
complete, or when bootstrap is disabled and the runtime readiness probe
confirms the user-created schema.

### Cleanup

Before deleting an AgentFactory resource, review the storage ownership and
retention settings. Controller-created PVCs are removed only when their
configured `deletePvcOnDelete` policy permits it; user-provided PVCs and the
runtime Secret remain user-owned.

```bash
kubectl -n paf delete agentfactory agentfactory-sample
kubectl -n paf get pvc,secret,job,deploy,svc
```

If the bootstrap Secret uses `cleanupPolicy: DeleteOnSuccess`, it is removed
after a successful setup Job. Deleting the AgentFactory resource does not
delete the database users or revoke credentials in OCI Vault; handle those
through the database and Vault lifecycle procedures.

## Troubleshooting and Known Issues

**In this section:**

- [Deployment is not created](#deployment-is-not-created)
- [Pods remain Pending](#pods-remain-pending)
- [Runtime does not become Ready](#runtime-does-not-become-ready)
- [Bootstrap Secret is recreated](#bootstrap-secret-is-recreated)
- [OpenShift: common failures](#openshift-common-failures)

### Deployment is not created

If the AgentFactory resource exists but no Deployment is created:

```bash
kubectl -n paf get agentfactory agentfactory-sample -o yaml
kubectl -n paf get deploy,pod,svc,pvc
kubectl -n paf get events --sort-by='.lastTimestamp'
```

Verify that the Oracle AI Database Operator and AgentFactory controller are running, then inspect the controller logs.

```bash
kubectl get pods -A | grep -Ei 'oracle-database-operator|agentfactory|operator'
```

### Pods remain Pending

Inspect the Pod and PVC:

```bash
kubectl -n paf describe pod <pod-name>
kubectl -n paf get pvc
kubectl -n paf describe pvc <pvc-name>
```

### Runtime does not become Ready

Inspect the generated runtime configuration and application logs:

```bash
kubectl -n paf get configmap agentfactory-sample-runtime -o yaml
kubectl -n paf logs deploy/agentfactory-sample
```

If automatic schema provisioning is enabled, also inspect the setup Job:

```bash
kubectl -n paf get job agentfactory-sample-db-setup -o yaml
kubectl -n paf logs job/agentfactory-sample-db-setup
```

Common causes are an unavailable PDB service, incorrect `serviceName`, a missing bootstrap Secret key, insufficient administrator privileges, an image without Bash or SQL*Plus, or an image without `unzip` when Wallet mode is used. In Wallet mode, also verify that the runtime Secret contains the wallet ZIP and that it contains the selected TNS alias. If bootstrap is disabled, verify that `spec.database.username` exists in the target PDB and that the PAF image exposes the configured HTTPS `spec.database.healthPath` endpoint. The endpoint must return a successful status only after the application and database schema are ready.

### Bootstrap Secret is recreated

`cleanupPolicy: DeleteOnSuccess` deletes the Kubernetes bootstrap Secret only. If an external-secret or CSI synchronization object is still active, it may recreate the Secret. Suspend or delete that synchronization object after the setup Job succeeds, then verify:

```bash
kubectl -n paf get secret paf-db-bootstrap-admin
```

### OpenShift: common failures

| Symptom | Likely cause | Fix |
| --- | --- | --- |
| SIDB Pod rejected; events mention `SYS_NICE` or capabilities | Only `anyuid` / `restricted-v2` bound for SIDB | Apply the SIDB write-permissions SCC sample and set `serviceAccountName: sidb-sa`. See [OpenShift Quick Start checklist](#openshift-quick-start-checklist). |
| AgentFactory Pod CrashLoop; missing Secret files under `/run/secrets` | Default Secret mount conflicts with RHEL/`rhsm` on OpenShift | Set `spec.security.secret.mountLocation` to a free path such as `/mnt/af-secrets`. |
| AgentFactory Pod permission errors on install paths; SCC is `restricted-v2` | Random UID cannot write image paths | Grant administrator-approved `anyuid` to the ServiceAccount the Pods use. |
| `serviceAccountName` rejected on apply (`BadRequest` / unknown field) | Installed CRD is older than the current API | Omit the field; authorize the namespace `default` ServiceAccount instead. |
| Bootstrap fails: `Password does not contain special character` | UI admin password policy | Recreate `agentfactory-runtime-secret` with a UI password that includes a special character. |
| Bootstrap fails with `BOOTSTRAP_RETRYABLE=false` after an earlier failed attempt | Stale `/mount` PVC state | Delete the AgentFactory resource and its app-mount PVC, fix SCC/`mountLocation`/password, then reapply on a fresh volume. |
| `ImagePullBackOff` / `Unauthorized` for the registry | Empty or wrong pull Secret | Use a Secret with valid registry auth, or omit `pullSecret` only when the registry allows anonymous pulls. |

## Additional Information

**In this section:**

- [AgentFactory CRD Field Reference](#agentfactory-crd-field-reference)
- [Samples](#samples)

### AgentFactory CRD Field Reference

The [annotated AgentFactory CRD field reference](./reference/agentfactory-crd-reference.yaml)
shows all user-configurable fields in the resource, including field
descriptions, defaults, allowed values, mutually exclusive alternatives,
Secret requirements, resource ownership, rollout behavior, and idempotency
notes.

The reference is documentation-only and must not be applied unchanged. Replace
placeholders, remove unused connection or volume alternatives, and follow the
comments before creating an `AgentFactory` resource. The generated CRD at
`config/crd/bases/privateai.oracle.com_agentfactories.yaml` remains the
authoritative Kubernetes schema, and `status` fields are controller-owned.

### Samples

- [Wallet mode](./provisioning/agentfactory-sample-wallet.yaml)
- [Wallet mode with automatic schema provisioning](./provisioning/agentfactory-sample-wallet-bootstrap.yaml)
- [Details mode](./provisioning/agentfactory-sample-details.yaml)
