# Oracle AI Database Operator for Kubernetes: Managing Single Instance Databases (SIDB)

Oracle AI Database Operator for Kubernetes (`OraOperator`) provides the SingleInstanceDatabase (SIDB) controller for deploying, managing, patching, cloning, and operating Oracle Database on Kubernetes. This guide covers Oracle Single Instance Database deployments, Data Guard, True Cache, ORDS, TCPS, and day-to-day lifecycle management using the `database.oracle.com/v4` API.

Use this document when you want to:

- Create a new single-instance database
- Provision clone, standby, or True Cache databases
- Configure Data Guard Broker, TCPS, service endpoints, or custom scripts
- Patch, resize, or delete a database
- Enable Oracle REST Data Services (ORDS) and Oracle APEX

## Contents
- [Quick Start](#quick-start)
- [Verify SIDB Deployment](#verify-sidb-deployment)
- [Before You Begin](#before-you-begin)
- [SIDB v4 Resource Model](#sidb-v4-resource-model)
- [SIDB Deployment on OpenShift](#sidb-deployment-on-openshift)
- [SIDB Deployment and Lifecycle](#sidb-deployment-and-lifecycle)
- [Data Guard](#data-guard)
- [True Cache](#true-cache)
- [ORDS and APEX](#ords-and-apex)
- [Networking](#networking)
- [Storage](#storage)
- [Lifecycle Management](#lifecycle-management)
- [Troubleshooting and Known Issues](#troubleshooting-and-known-issues)
- [Additional Information](#additional-information)

## Quick Start

Use this Quick Start to deploy and verify a primary SIDB.

> **Before proceeding:** Review [Before You Begin](#before-you-begin) and understand its image, registry, storage, and namespace requirements.

**Required prerequisites and example names**

> **Required before deployment**
>
> The `ocr-reg-cred` image-pull Secret and `db-admin-secret` database-admin Secret are mandatory for this Quick Start. Complete every **linked action** in the **Complete before applying** column before applying the manifest.

| Requirement | Values used by this Quick Start | Complete before applying |
| --- | --- | --- |
| Shared deployment prerequisites | Before You Begin | **[Review Before You Begin](#before-you-begin)** |
| Target namespace | `default` | Create the Secrets in this namespace and set `metadata.namespace` to match. |
| Database image / image-pull Secret | `container-registry.oracle.com/database/enterprise_ru:latest` / `ocr-reg-cred` | **[Create the `ocr-reg-cred` image-pull Secret](./prerequisites/PREREQUISITES.md#image-pull-secret)** |
| Database admin Secret | `db-admin-secret`, key `oracle_pwd` | **[Create the `db-admin-secret` database-admin Secret](./prerequisites/PREREQUISITES.md#database-admin-secret)** |
| Persistent storage | StorageClass `oci-bv`, `100Gi`, `ReadWriteOnce` | **[Prepare persistent storage](./prerequisites/PREREQUISITES.md#set-up-kubernetes-and-volumes-for-database-persistence)** |

1. Review [`sidb_quickstart.yaml`](../../config/samples/sidb/sidb_quickstart.yaml), then update its namespace, storage class, image, and Secret names for your environment.
2. Apply the manifest.
3. Continue with [Verify SIDB Deployment](#verify-sidb-deployment) to confirm readiness and retrieve the connection strings.

**Important:** The sample uses the `default` namespace. Replace it with the actual namespace for your deployment before applying the file.

Apply the manifest:

```sh
kubectl apply -f config/samples/sidb/sidb_quickstart.yaml
```

Continue with [Verify SIDB Deployment](#verify-sidb-deployment) after applying the manifest.

## Verify SIDB Deployment

After applying the manifest, confirm that the SIDB is ready before using its connection strings.

### Verify SIDB Status and Connection Strings

Use SIDB status as the source of truth for readiness and connection strings:

```sh
NS=default
SIDB=sidb-sample

kubectl get singleinstancedatabase -n $NS -o wide
kubectl get singleinstancedatabase $SIDB -n $NS \
  -o jsonpath='{"status="}{.status.status}{"\n"}{"role="}{.status.role}{"\n"}{"clusterConnectString="}{.status.clusterConnectString}{"\n"}{"connectString="}{.status.connectString}{"\n"}{"tcpsConnectString="}{.status.tcpsConnectString}{"\n"}'
```

Typical output:

```text
status=Healthy
role=PRIMARY
clusterConnectString=<in-cluster-connect-string>
connectString=<tcp-connect-string>
tcpsConnectString=Not enabled
```

- `clusterConnectString` is for in-cluster clients.
- `connectString` is for externally reachable TCP clients.
- `tcpsConnectString` is for TCPS clients and reports `Not enabled` when TCPS is disabled.

### Verify Supporting Resources

```sh
kubectl get pods,svc,pvc -n $NS
kubectl get secret -n $NS
```

### Inspect Details When Needed

If the SIDB does not report `Healthy`, inspect its status and recent events:

```sh
kubectl describe singleinstancedatabase $SIDB -n $NS
kubectl get events -n $NS --sort-by=.lastTimestamp
```

## Before You Begin

This section describes the requirements and configuration choices for SIDB
deployment scenarios. Review every point and subsection
before applying a manifest so that you can tailor the YAML to your deployment
requirements.

> **Important:** Each deployment scenario has a **Required before deployment**
> section. Complete every mandatory action in that section before applying the
> scenario's YAML.

**Always required**

1. [Oracle Database container images](./prerequisites/PREREQUISITES.md#prepare-oracle-database-container-images)
2. [Kubernetes worker-node disk space](./prerequisites/PREREQUISITES.md#ensure-sufficient-disk-space-in-kubernetes-worker-nodes)
3. [Kubernetes cluster and persistent storage / StorageClass](./prerequisites/PREREQUISITES.md#set-up-kubernetes-and-volumes-for-database-persistence)
4. [Database admin secret](./prerequisites/PREREQUISITES.md#database-admin-secret)
5. [Operator-watched namespace and access](../../README.md#install-from-yaml-manifests)

**When your path needs them**

1. [Image pull secret](./prerequisites/PREREQUISITES.md#image-pull-secret) — private registry or licensed OCR images (for example `enterprise_ru`)
2. [TDE secret](./prerequisites/PREREQUISITES.md#tde-secret) — when the manifest sets `spec.security.secrets.tde` (**required for True Cache**)
3. [TCPS TLS secret](./prerequisites/PREREQUISITES.md#tcps-tls-secret) — when the manifest enables `spec.security.tcps`
4. [ORDS password secret](./prerequisites/PREREQUISITES.md#ords-password-secret) — when deploying ORDS / OrdsSrvs
5. [SIDB Pod kernel sysctls](./prerequisites/PREREQUISITES.md#configure-sidb-pod-kernel-sysctls) — when the manifest specifies Oracle sysctls or requests HugePages
6. [SIDB HugePages](./prerequisites/PREREQUISITES.md#configure-sidb-hugepages) — when requesting `hugepages-2Mi`

**Review Best Practices for Deployment Scenarios**

Before deploying SIDB to production, review the following sections and confirm
that the selected scenario satisfies its networking, storage, lifecycle, and
operational requirements:

- [Networking](#networking) — service endpoints, LoadBalancer exposure, TCPS,
  and TNS aliases for Oracle client connectivity.
- [Storage](#storage) — persistent `oradata` and FRA volumes, additional PVCs
  for diagnostics or application data, and the required write-permissions
  behavior.
- [Lifecycle Management](#lifecycle-management) — resource sizing, custom
  scripts, immutable fields, and maintenance operations.
- [Known Issues](#known-issues) — current limitations and operational cautions.

Complete the guidance that applies to your deployment before applying the
manifest. In particular, verify the required additional PVCs, TNS aliases,
LoadBalancer Services, and TCPS configuration rather than adding them after
the database is in use.

**Command-line tools**

Use Bash 4 or later and `kubectl` with access to the target cluster. Broker generation also requires `jq`; `yq` is optional for YAML output. The TDE wallet steps use `jq`, `zip`, `unzip`, `base64`, and `cmp`. The certificate helper also uses `openssl` and `timeout`.

Full checklist and field tables:
[PREREQUISITES.md — Quick Checklist](./prerequisites/PREREQUISITES.md#quick-checklist).

After completing the prerequisites, review the current full-field SIDB reference template before authoring a manifest:

- [`config/samples/sidb/singleinstancedatabase.yaml`](../../config/samples/sidb/singleinstancedatabase.yaml)

The template starts as a valid primary baseline and documents every current
SIDB-specific v4 field as an active value or a commented, mode-specific option.
It deliberately excludes deprecated fields and does not combine mutually
exclusive primary-source, restore, or True Cache options. Use a
scenario-specific sample for a runnable clone, standby, True Cache, or restore
manifest.

### Mandatory Resource Privileges

The SIDB controller requires the following Kubernetes resource privileges:

| Resource | Privileges |
| --- | --- |
| Pods | `create delete get list patch update watch` |
| Containers | `create delete get list patch update watch` |
| PersistentVolumeClaims | `create delete get list patch update watch` |
| Services | `create delete get list patch update watch` |
| Secrets | `create delete get list patch update watch` |
| Events | `create patch` |

For access management, see [`../../README.md`](../../README.md).

### Optional Resource Privileges

Some Oracle AI Database Operator features require additional Kubernetes RBAC permissions. Apply the corresponding RBAC manifest only if you plan to use the associated feature.

| Feature | When Required | Resource | Privileges |
| --- | --- | --- | --- |
| NodePort service connect strings | When using NodePort services to generate database connect strings | Nodes | `list`, `watch` |
| Storage expansion for block volumes | When using block volume expansion for database storage | StorageClasses | `get`, `list`, `watch` |
| Custom script execution | When executing custom scripts that require PersistentVolume information | PersistentVolumes | `get`, `list`, `watch` |

The optional RBAC manifests are located in the repository `rbac` directory:

- [`rbac/node-rbac.yaml`](../../rbac/node-rbac.yaml)
- [`rbac/storage-class-rbac.yaml`](../../rbac/storage-class-rbac.yaml)
- [`rbac/persistent-volume-rbac.yaml`](../../rbac/persistent-volume-rbac.yaml)

If you are running commands from the repository root, apply them as follows:

```sh
kubectl apply -f rbac/node-rbac.yaml
kubectl apply -f rbac/storage-class-rbac.yaml
kubectl apply -f rbac/persistent-volume-rbac.yaml
```

If you are running commands from another directory, provide the correct relative or absolute path to the same files.

## SIDB v4 Resource Model

The most important documentation change in v4 is the grouped parameter model. When authoring new manifests, prefer the v4 grouped layout described below.

### Key v4 Groups

| Area | v4 fields | Notes |
| --- | --- | --- |
| Admin credentials | `spec.security.secrets.admin` | Replaces the flat password-secret style for new manifests |
| Source database | [`spec.primarySource`](./prerequisites/PREREQUISITES.md#primary-source) | Used for `clone`, `standby`, and `truecache` |
| Data Guard | [`spec.dataguard`](./prerequisites/PREREQUISITES.md#data-guard) | Standby prerequisites, standby sources, and broker-related SIDB fields |
| True Cache | [`spec.trueCache`](./prerequisites/PREREQUISITES.md#true-cache) | Covers blob generation and truecache-only options |
| TCPS | `spec.security.tcps` | Enables TCPS and TLS secret handling |
| Services Endpoints | `spec.services.endpoints` | Defines external TCP and TCPS service endpoints |
| Storage | `spec.persistence.oradata` | Main datafiles volume definition |
| Resources | `spec.resources` | Standard Kubernetes resource requests and limits |

## SIDB Deployment on OpenShift

Deploy SIDB on OpenShift by creating an SCC profile and binding it to the
ServiceAccount used by the SIDB Pod. Complete [Before You Begin](#before-you-begin)
first, including the image, registry, storage, and Secret requirements.

> **Important:** Apply **one** profile to `sidb-sa`; do not bind both profiles
> to the same ServiceAccount. Use the write-permissions profile when the operator
> needs to run a permissions init container as root. Setting
> `spec.persistence.setWritePermissions: true` does not by itself confirm
> that this container was generated; inspect the resulting Pod.

### Select an SCC Profile

| SIDB setting | SCC profile | Sample manifest |
| --- | --- | --- |
| `setWritePermissions: false` or omitted | Non-root containers (`UID 54321`, `fsGroup 54321`) | [`openshift_rbac.yaml`](../../config/samples/sidb/openshift_rbac.yaml) |
| `setWritePermissions: true` | Permits a root permissions init container when generated by the operator; verify the Pod configuration. | [`openshift_rbac_write_permissions.yaml`](../../config/samples/sidb/openshift_rbac_write_permissions.yaml) |

The non-root profile permits UID `54321`. The write-permissions profile
permits UIDs from `0` through `54321`, allowing a permissions init container
to run as root when required by the storage configuration. Applying this
SCC does not create the init container. Verify the generated Pod to confirm
whether the operator added a permissions init container and which user IDs
the containers use.

Each sample contains the SCC, `sidb-sa` ServiceAccount, a namespace-scoped
Role granting `use` on exactly that SCC, and a RoleBinding from the Role to
`sidb-sa`. The SCC defines the permitted security settings. The RoleBinding is
what authorizes Pods that use `sidb-sa` to be admitted under that SCC; it does
not grant database access or broader Kubernetes API permissions.

### Create and Bind the Profile

The samples use `sidb-ns`. Set `NS` to the target namespace, then apply only
the file selected in the preceding table. Creating an SCC requires an
OpenShift cluster administrator or an identity authorized to manage SCCs.

```sh
NS=sidb-ns

# Use this profile when setWritePermissions is false or omitted.
PROFILE=config/samples/sidb/openshift_rbac.yaml

# For setWritePermissions: true, use this instead:
# PROFILE=config/samples/sidb/openshift_rbac_write_permissions.yaml

# This keeps a profile switch mutually exclusive; it is harmless before the
# first SIDB deployment.
oc delete rolebinding use-sidb-oracle-user-scc \
  use-sidb-write-permissions-scc -n "$NS" --ignore-not-found

sed "s/sidb-ns/${NS}/g" "$PROFILE" | oc apply -f -
```

Confirm that the selected SCC is usable by the SIDB ServiceAccount. Substitute
the write-permissions SCC name when that profile is selected:

```sh
oc auth can-i use securitycontextconstraints/sidb-oracle-user-scc \
  --as="system:serviceaccount:${NS}:sidb-sa" \
  -n "$NS"
```

The command must return `yes` before you create the SIDB. If it returns `no`,
verify that the RoleBinding and the `subjects[].namespace` value use the same
namespace as the SIDB.

### Reference the ServiceAccount in the SIDB Manifest

Set `spec.serviceAccountName` to the bound ServiceAccount. Select the matching
`setWritePermissions` setting:

```yaml
spec:
  serviceAccountName: sidb-sa
  persistence:
    # Omit this field or set it to false for the non-root SCC profile.
    setWritePermissions: false
```

Use the write-permissions profile only when the storage backend needs the
controller to change ownership of the mounted volumes:

```yaml
spec:
  serviceAccountName: sidb-sa
  persistence:
    setWritePermissions: true
```

If you change `setWritePermissions` after deployment, remove the previous
profile RoleBinding before applying the matching profile, then update the SIDB
manifest. If you set a custom `spec.securityContext` or request additional
capabilities, have the OpenShift administrator provide a separate
least-privilege SCC that permits those exact settings.

After the Pod is created, confirm the SCC selected by OpenShift:

```sh
SIDB=sidb-sample
POD=$(oc get pod -n "$NS" -l app="$SIDB" -o jsonpath='{.items[0].metadata.name}')
oc get pod -n "$NS" "$POD" \
  -o jsonpath='{.metadata.annotations.openshift\.io/scc}{"\n"}'
```

## SIDB Deployment and Lifecycle

This section provides task-oriented deployment and lifecycle procedures. Each procedure points to the recommended sample and highlights the main parameters to review.

Before using any workflow in this section, complete [Before You Begin](#before-you-begin). Verify that the namespace, secrets, image pull secret, and storage class used by the selected YAML exist.

For a standard primary SIDB deployment, follow [Quick Start](#quick-start). It creates the primary database, verifies it is `Healthy`, and retrieves its connection strings. Use the procedures below for specialized deployment types such as TDE/TCPS, prebuilt, clone, standby, restore, patch, or delete.

- [Quick Start: Create a New Database](#quick-start)
- [Create a Primary Database with TDE and TCPS](#create-a-primary-database-with-tde-and-tcps)
- [Create a Prebuilt Database](#create-a-prebuilt-database)
- [Create Express, Free, or Free Lite Databases](#create-express-free-or-free-lite-databases)
- [Clone a Database](#clone-a-database)
- [Restore a Database from an RMAN Backup](#restore-a-database-from-an-rman-backup)
- [Patch a Database](#patch-a-database)
- [Delete a Database](#delete-a-database)

#### Create a Primary Database with TDE and TCPS

Use this workflow to create a new primary SIDB with TDE encryption and a TCPS listener.

> **Before proceeding:** Review [Before You Begin](#before-you-begin) to understand the namespace, image, registry, storage, credential, TDE, and TCPS requirements for this deployment.

**Required prerequisites and example names**

> **Required before deployment**
>
> Complete the required actions in the **Complete before applying** column before applying the manifest.

| Requirement | Values used by this workflow | Complete before applying |
| --- | --- | --- |
| Shared deployment prerequisites | Before You Begin | **[Review Before You Begin](#before-you-begin)** |
| Target namespace | `<operator-watched-namespace>` | Create all referenced Secrets in this namespace and set `metadata.namespace` to match. |
| Database admin Secret | `db-admin-secret`, key `oracle_pwd` | **[Create the database-admin Secret](./prerequisites/PREREQUISITES.md#database-admin-secret)** |
| TDE password Secret | `tde-wallet-secret`, key `tde_wallet_pwd` | **[Create the TDE password Secret](./prerequisites/PREREQUISITES.md#tde-secret)** |
| TCPS TLS Secret | `sidb-tcps-tls` | **[Create the TCPS TLS Secret](./prerequisites/PREREQUISITES.md#tcps-tls-secret)** |
| Database image / image-pull Secret | `container-registry.oracle.com/database/enterprise_ru:latest` / `ocr-reg-cred` | **[Create the image-pull Secret](./prerequisites/PREREQUISITES.md#image-pull-secret)** |
| Persistent storage | StorageClass `oci-bv`, `100Gi`, `ReadWriteOnce` | **[Prepare persistent storage](./prerequisites/PREREQUISITES.md#set-up-kubernetes-and-volumes-for-database-persistence)** |
| TCPS listener endpoint | NodePort with TCPS enabled | **[Configure TCPS endpoints and clients](#enable-tcps)** |

1. Start with [`config/samples/sidb/singleinstancedatabase_tcps_tde.yaml`](../../config/samples/sidb/singleinstancedatabase_tcps_tde.yaml). Update the namespace, database admin Secret, image, image-pull Secret, and persistence settings for your environment. The following `security` block configures TDE and TCPS. Set `spec.security.secrets.tde` to the TDE password Secret identified in the table. Keep `secretKey: tde_wallet_pwd` unless the Secret uses a different key. Set `spec.security.tcps.tlsSecret` to the TCPS TLS Secret identified in the table. You can enable TCPS by adding the `spec.security.tcps` fields or disable it by removing them:

   ```yaml
   security:
     secrets:
       tde:
         secretName: tde-wallet-secret
         secretKey: tde_wallet_pwd
     tcps:
       enabled: true
       tlsSecret: sidb-tcps-tls
   ```

2. The sample enables `tcps` on a NodePort service endpoint. Change the endpoint type or add endpoints as needed.
3. Apply the manifest and wait for the primary to become healthy:

   ```sh
   kubectl apply -f singleinstancedatabase_tcps_tde.yaml
   kubectl get singleinstancedatabase sidb-sample -n <namespace> --watch
   ```

4. Retrieve the TCPS connect string after the primary is healthy:

   ```sh
   kubectl get singleinstancedatabase sidb-sample -n <namespace> -o jsonpath='{.status.tcpsConnectString}{"\n"}'
   ```

#### Create a Prebuilt Database

Use when the image already contains a prebuilt database.

> **Before proceeding:** Review [Before You Begin](#before-you-begin) to understand the namespace, image, registry, credential, and storage requirements for this deployment.

**Required prerequisites and example names**

> **Required before deployment**
>
> Complete the required actions in the **Complete before applying** column before applying the manifest.

| Requirement | Values used by this workflow | Complete before applying |
| --- | --- | --- |
| Shared deployment prerequisites | Before You Begin | **[Review Before You Begin](#before-you-begin)** |
| Target namespace | `default` | Create referenced Secrets in this namespace and set `metadata.namespace` to match. |
| Database admin Secret | `prebuiltdb-admin-secret`, key `oracle_pwd` | **[Create the database-admin Secret](./prerequisites/PREREQUISITES.md#database-admin-secret)** |
| Prebuilt database image | `container-registry.oracle.com/database/free:latest` with `prebuiltDB: true` | **[Prepare the database image](./prerequisites/PREREQUISITES.md#prepare-oracle-database-container-images)** |
| Image-pull Secret (if required) | Not set in the sample | **[Create an image-pull Secret](./prerequisites/PREREQUISITES.md#image-pull-secret)** |
| Persistent storage (optional) | StorageClass `oci-bv`, `50Gi`, `ReadWriteOnce` | **[Prepare persistent storage](./prerequisites/PREREQUISITES.md#set-up-kubernetes-and-volumes-for-database-persistence)** |

Start with [`config/samples/sidb/singleinstancedatabase_prebuiltdb.yaml`](../../config/samples/sidb/singleinstancedatabase_prebuiltdb.yaml).

Key fields:

- `spec.image.prebuiltDB: true`
- prebuilt database image selection

#### Create Express, Free, or Free Lite Databases

Use these edition-specific or image-specific samples when you want lighter-weight database distributions. Check the sample manifest for the exact `spec.edition` value supported by the installed CRD.

> **Before proceeding:** Review [Before You Begin](#before-you-begin) to understand the namespace, image, registry, credential, resource, and storage requirements for this deployment.

**Required prerequisites and example names**

> **Required before deployment**
>
> Complete the required actions in the **Complete before applying** column before applying the manifest.

| Requirement | Values used by this workflow | Complete before applying |
| --- | --- | --- |
| Shared deployment prerequisites | Before You Begin | **[Review Before You Begin](#before-you-begin)** |
| Target namespace | `default` | Create referenced Secrets in this namespace and set `metadata.namespace` to match. |
| Database admin Secret | `xedb-admin-secret` for Express; `freedb-admin-secret` for Free and Free Lite; key `oracle_pwd` | **[Create the database-admin Secret](./prerequisites/PREREQUISITES.md#database-admin-secret)** |
| Database image and edition | Express, Free, or Free Lite image with the matching `spec.edition` | **[Prepare the database image](./prerequisites/PREREQUISITES.md#prepare-oracle-database-container-images)** |
| Image-pull Secret (if required) | Not set in the samples | **[Create an image-pull Secret](./prerequisites/PREREQUISITES.md#image-pull-secret)** |
| Persistent storage | StorageClass `oci-bv`, `50Gi`, `ReadWriteOnce` | **[Prepare persistent storage](./prerequisites/PREREQUISITES.md#set-up-kubernetes-and-volumes-for-database-persistence)** |
| Pod resources | Requests and limits in the selected sample | Size CPU and memory for the selected edition and workload. |

Samples:

- [`config/samples/sidb/singleinstancedatabase_express.yaml`](../../config/samples/sidb/singleinstancedatabase_express.yaml)
- [`config/samples/sidb/singleinstancedatabase_free.yaml`](../../config/samples/sidb/singleinstancedatabase_free.yaml)
- [`config/samples/sidb/singleinstancedatabase_free-lite.yaml`](../../config/samples/sidb/singleinstancedatabase_free-lite.yaml)

Review:

- `spec.edition`
- image selection
- resource sizing
- storage sizing

#### Clone a Database

Use when you want a new SIDB created from an existing primary database.

> **Before proceeding:** Review [Before You Begin](#before-you-begin) to understand the namespace, image, registry, credential, source connectivity, and storage requirements for this deployment.

**Required prerequisites and example names**

> **Required before deployment**
>
> Complete the required actions in the **Complete before applying** column before applying the manifest.

| Requirement | Values used by this workflow | Complete before applying |
| --- | --- | --- |
| Shared deployment prerequisites | Before You Begin | **[Review Before You Begin](#before-you-begin)** |
| Target namespace | `default` | Create referenced Secrets in this namespace and set `metadata.namespace` to match. |
| Source primary | Healthy SIDB with `archiveLog: true` | Verify that the source is healthy and has archive logging enabled. |
| Source reference | `primarySource.databaseRef: sidb-sample` | **[Configure the primary source](./prerequisites/PREREQUISITES.md#primary-source)** |
| Database admin Secret | `db-admin-secret`, key `oracle_pwd` | **[Create the database-admin Secret](./prerequisites/PREREQUISITES.md#database-admin-secret)** |
| Compatible database image / image-pull Secret | `container-registry.oracle.com/database/enterprise_ru:latest` / `ocr-reg-cred` | **[Prepare the database image](./prerequisites/PREREQUISITES.md#prepare-oracle-database-container-images)** and **[create the image-pull Secret](./prerequisites/PREREQUISITES.md#image-pull-secret)** |
| Persistent storage | StorageClass `oci-bv`, `100Gi`, `ReadWriteOnce` | **[Prepare persistent storage](./prerequisites/PREREQUISITES.md#set-up-kubernetes-and-volumes-for-database-persistence)** |

**Note:** Enabling force logging (`spec.forceLog: true`) on the source before starting the clone is also recommended to ensure that changes from operations normally performed with `NOLOGGING` can be recovered in the duplicated database. DBCA reports warning `DBT-16071` when the source does not have force logging enabled.

Start with [`config/samples/sidb/singleinstancedatabase_clone.yaml`](../../config/samples/sidb/singleinstancedatabase_clone.yaml).

Key fields:

- `spec.createAs: clone`
- `spec.primarySource`
- `spec.security.secrets.admin`
- image compatible with the source database major version

#### Restore a Database from an RMAN Backup

Use this option to provision a new Single Instance Database (SIDB) by restoring an existing RMAN backup. The backup can be restored from either an object storage service or a mounted file system.

> **Before proceeding:** Review [Before You Begin](#before-you-begin) to understand the namespace, image, registry, credential, backup-access, and storage requirements for this deployment.

**Required prerequisites and example names**

> **Required before deployment**
>
> Complete the required actions in the **Complete before applying** column before applying the manifest.

| Requirement | Values used by this workflow | Complete before applying |
| --- | --- | --- |
| Shared deployment prerequisites | Before You Begin | **[Review Before You Begin](#before-you-begin)** |
| Restore scenario | CDB or non-CDB from OCI Object Storage or a mounted file system | **[Complete the RMAN restore prerequisites](./restore/RESTORE_DATABASE_FROM_RMAN_BACKUP.md#before-you-begin)** |
| Target namespace | `sidb` in the restore guide | Create the namespace and all referenced Secrets in the same namespace as the SIDB. |
| Restore Secrets and ConfigMaps | `sidb-admin-secret`; source wallet and decrypt Secrets when required; backup access configuration | **[Create restore Secrets and ConfigMaps](./restore/RESTORE_DATABASE_FROM_RMAN_BACKUP.md#prepare-secrets-and-configmaps)** |
| Target database image / image-pull Secret | Image compatible with the source database; registry Secret when required | **[Prepare the database image](./prerequisites/PREREQUISITES.md#prepare-oracle-database-container-images)** and **[create the image-pull Secret](./prerequisites/PREREQUISITES.md#image-pull-secret)** |
| Backup location and target storage | Object Storage details or a mounted backup volume; persistent storage for the restored SIDB | **[Configure the restore manifest](./restore/RESTORE_DATABASE_FROM_RMAN_BACKUP.md#configure-the-restore-manifest)** and **[prepare persistent storage](./prerequisites/PREREQUISITES.md#set-up-kubernetes-and-volumes-for-database-persistence)** |

Use the linked actions in the table for the complete restore procedure and scenario-specific settings.

#### Patch a Database

Use this workflow to update an SIDB to a newer RU-compatible image.

> **Important:** For a primary SIDB, the operator owns `datapatch`. Do not run `datapatch` manually in the database Pod after an image update.

1. Set the namespace, SIDB name, and approved target image. Confirm the image is compatible with the database release and that its registry is reachable:

   ```sh
   NS=default
   DB=sidb-sample
   NEW_IMAGE=container-registry.oracle.com/database/enterprise_ru:<approved-tag>
   ```

2. Update `spec.image.pullFrom` directly. This replaces the SIDB Pod:

   ```sh
   kubectl patch singleinstancedatabase "$DB" -n "$NS" \
     --type=merge \
     -p "{\"spec\":{\"image\":{\"pullFrom\":\"${NEW_IMAGE}\"}}}"
   ```

   To update `spec.image.version` instead, use the same command with `version` in place of `pullFrom`.

3. Wait for the replacement Pod and SIDB to become ready:

   ```sh
   kubectl get singleinstancedatabase "$DB" -n "$NS" --watch
   ```

4. Inspect the controller decision and related events:

   ```sh
   kubectl get singleinstancedatabase "$DB" -n "$NS" \
     -o jsonpath='{.status.datafilesPatched}{"\n"}{.status.datapatch.image}{"\n"}{.status.datapatch.imageID}{"\n"}{.status.datapatch.oracleHomePatchFingerprint}{"\n"}{.status.datapatch.sqlPatchFingerprint}{"\n"}'

   kubectl get events -n "$NS" \
     --field-selector involvedObject.kind=SingleInstanceDatabase,involvedObject.name="$DB" \
     --sort-by=.lastTimestamp
   ```

5. Verify the SQL patch inventory in the database.

   Connect as SYSDBA to `CDB$ROOT` in the replacement database Pod and run:

   ```sql
   SET LINESIZE 240
   SET PAGESIZE 100
   COLUMN action FORMAT A10
   COLUMN status FORMAT A15
   COLUMN description FORMAT A75

   SELECT con_id, patch_id, action, status, action_time, description
   FROM   cdb_registry_sqlpatch
   ORDER  BY con_id, patch_id, action_time DESC;
   ```

   Confirm that the latest record for the expected RU patch in each
   applicable container shows `ACTION = APPLY` and `STATUS = SUCCESS`.
   Review the datapatch output to confirm which containers were processed.

##### Image Update and Datapatch

1. Changing `spec.image.pullFrom` or `spec.image.version` replaces the SIDB Pod. The database container starts with `SKIP_DATAPATCH=true`, so startup cannot apply SQL patches concurrently with the controller.

2. For a primary SIDB, after the replacement Pod is ready, the operator compares the result in `status.datapatch` with two fingerprints:

   - The Oracle Home fingerprint is a sorted hash of patch IDs from `opatch lspatches`.
   - The database fingerprint is a sorted hash of the latest state records in `DBA_REGISTRY_SQLPATCH`.

3. The operator acts as follows:

   - On the first image update after installing an operator version that records this status, no prior fingerprint exists, so the operator runs `datapatch` once.
   - When both fingerprints match the recorded result, the patch inventories are unchanged. The operator emits `Datapatch Skipped`, records the image and image ID, and sets `status.datafilesPatched` to `true` without running `datapatch`.
   - When either fingerprint differs, the operator runs `datapatch`, requires a successful command exit and SQL patch verification, then records the new fingerprints.
   - When inventory collection, `datapatch`, or SQL patch verification fails, `status.datafilesPatched` remains `false` and reconciliation retries.

4. The operator does not run `datapatch` on a physical standby or True Cache instance because they are read-only. Their `status.datafilesPatched` value can remain `false` after an image update; this does not mean the image update failed. If a physical standby is later promoted to `PRIMARY`, the operator evaluates and runs or skips `datapatch` for that role.

5. Do not use image tags or `status.releaseUpdate` as patch identity: either can omit one-off patches. Use the controller-recorded fingerprints to determine whether the Oracle Home and SQL patch inventories changed.

##### Recover a Missed Primary Datapatch

Use this recovery only when a **primary** SIDB has a changed Oracle Home patch inventory but `DBA_REGISTRY_SQLPATCH` does not show the corresponding SQL patch. Do not use it for a physical standby or True Cache instance.

1. Confirm that the SIDB is `PRIMARY` and review the controller patch state:

   ```sh
   kubectl get singleinstancedatabase "$DB" -n "$NS" \
     -o jsonpath='role={.status.role}{"\n"}datafilesPatched={.status.datafilesPatched}{"\n"}'
   ```

2. Only when the role is `PRIMARY` and `datafilesPatched=true`, mark datapatch pending:

   ```sh
   kubectl patch singleinstancedatabase "$DB" -n "$NS" \
     --subresource=status --type=merge \
     -p '{"status":{"datafilesPatched":"false"}}'
   ```

3. Watch for `Datapatch Executing` and `Datapatch Done` events. Then verify that `DBA_REGISTRY_SQLPATCH` contains the new release update.

#### Delete a Database

Delete the SIDB deployment:

```sh
NS=default
DB=sidb-sample

kubectl delete singleinstancedatabase $DB -n $NS
```

Before deleting:

- delete OrdsSrvs (or a legacy `OracleRestDataService`) first if the SIDB is referenced by an ORDS resource
- delete Data Guard Broker first if the SIDB is part of a Data Guard configuration
- review PVCs before deleting database storage

Check PVCs:

```sh
kubectl get pvc -n $NS
```

> Do not delete PVCs unless you intend to delete the database files.

## Data Guard

#### Overview

[Oracle Data Guard](https://docs.oracle.com/en/database/oracle/oracle-database/26/haovw/oracle-data-guard.html)
is a native high-availability, data protection, and disaster recovery solution
included with Oracle Database Enterprise Edition. It automates the creation,
maintenance, and monitoring of one or more standby databases so enterprise
applications can survive outages, data corruptions, and disasters without
losing information. Architecture and concepts:
[Oracle Data Guard documentation](https://docs.oracle.com/en/database/oracle/oracle-database/26/haovw/oracle-data-guard.html).

This OraOperator guide deploys Data Guard as SIDB resources plus a Broker, in
order:

1. A **primary** SIDB (`createAs: primary` or an equivalent primary role).
2. A **physical standby** SIDB (`createAs: standby`) that connects to the
   primary with `primarySource.databaseRef` or `primarySource.connectString`.
3. A **Data Guard Broker** (`DataguardBroker`) that manages the configuration
   after the primary and standby are Healthy.

Choose one deployment scenario below, create the primary before the standby,
then configure Broker. Same-cluster workflows use the example resource names in
[Before You Begin for Data Guard](#before-you-begin-for-data-guard); apply only
one path to a new database pair. Cross-cluster paths use their linked runbooks.

#### Data Guard Contents

- [Deployment Scenarios](#deployment-scenarios)
- [Helpers and Operations](#helpers-and-operations)
- [Before You Begin for Data Guard](#before-you-begin-for-data-guard)
- [Data Guard Quick Start](#data-guard-quick-start)
- [Data Guard Setup with TDE and TCPS](#data-guard-setup-with-tde-and-tcps)
- [Create the Data Guard Broker Configuration](#create-the-data-guard-broker-configuration)
- [Perform Data Guard Operations](#perform-data-guard-operations)
- [Delete the Data Guard Configuration](#delete-the-data-guard-configuration)

#### Deployment Scenarios

> **Before proceeding:** Review [Before You Begin for Data Guard](#before-you-begin-for-data-guard) and understand its requirements. Each item below is a deployment path; complete that scenario’s prerequisites before applying its manifests.

| When you need to... | Open |
| --- | --- |
| Create a same-cluster primary and physical standby over TCP | [Data Guard Quick Start](#data-guard-quick-start) |
| Create a same-cluster primary and physical standby with TDE and TCPS | [Data Guard Setup with TDE and TCPS](#data-guard-setup-with-tde-and-tcps) |
| Create a cross-cluster primary and standby over TCP with NLB | [Cross-Cluster Data Guard over TCP with NLB](./dataguard/DATAGUARD_CROSS_CLUSTER_LB.md) |
| Create a cross-cluster primary and standby over TCPS with NLB (ExternalDNS) | [Cross-Cluster Data Guard over TCPS with NLB](./dataguard/DATAGUARD_CROSS_CLUSTER_TCPS_NLB.md) |

#### Helpers and Operations

| When you need to... | Open |
| --- | --- |
| Prepare for any Data Guard deployment | [Before You Begin for Data Guard](#before-you-begin-for-data-guard) |
| Create cross-cluster TCPS certificates with separate bastions | [Separate cluster TCPS certificate access](./tcps-cert-manager/SEPARATE_CLUSTER_ACCESS.md) |
| Copy a remote primary TCPS client wallet into the Broker namespace | [TCPS Client Wallet Placement for the Broker Runner](#tcps-client-wallet-placement-for-the-broker-runner) |
| Generate or apply a Data Guard Broker manifest | [Create the Data Guard Broker Configuration](#create-the-data-guard-broker-configuration) |
| Run DGMGRL through the Broker runner | [Connect to the Broker Runner](#connect-to-the-broker-runner) |
| Verify Broker, database, transport, and apply status | [Watch Data Guard Status](#watch-data-guard-status) |
| Perform a Broker operation | [Perform Data Guard Operations](#perform-data-guard-operations) |
| Perform a planned role reversal | [Switchover](#switchover) |
| Change data protection | [Change Protection Mode](#change-protection-mode) |
| Use a temporarily writable standby | [Convert Between Physical and Snapshot Standby](#convert-between-physical-and-snapshot-standby) |
| Enable or disable automatic failover | [Enable and Disable Fast-Start Failover](#enable-and-disable-fast-start-failover) |
| Build a static Data Guard connect string | [Static Data Guard Connect String](#static-data-guard-connect-string) |
| Create a custom Data Guard service | [Create Sample Custom Service](#create-sample-custom-service) |
| Delete the Broker configuration | [Delete the Data Guard Configuration](#delete-the-data-guard-configuration) |

#### Before You Begin for Data Guard

Before proceeding, review every point in **[Before You Begin](#before-you-begin)** to understand the overall prerequisites, including storage and credential requirements. Select one deployment scenario above and complete the mandatory scenario-specific prerequisites in its **Required Before Deployment** section before applying the related manifest.

##### Set the example variables

In this Data Guard guide, we use the following resource names. If you change names in the YAML files, update them here as well:

```sh
export NS=default
export PRIMARY_DB=sidb-sample
export STANDBY_DB=standbydatabase-sample
export DGB=standbydatabase-sample-dg
export BROKER_NAME="$DGB"
```

Run the commands from the repository root. If you change a name or namespace, update the corresponding YAML references, Service DNS names, certificate SANs, and commands. Create Secrets in `$NS` and adjust the storage settings for your cluster.

#### Data Guard Quick Start

This **same-cluster** workflow creates a primary and physical standby in the same Kubernetes cluster and namespace over TCP. The standby uses `primarySource.databaseRef`. Complete [Before You Begin for Data Guard](#before-you-begin-for-data-guard) first; the primary sample already enables explicit Data Guard prerequisites, and the standby uses `databaseRef`.

<a id="tcp-workflow-settings"></a>

**Required Before Deployment**

> **Important:** Complete every action in the **Complete before applying** column before applying a related manifest. These requirements are mandatory for this deployment scenario.

| Requirement | Example value | Complete before applying |
| --- | --- | --- |
| Shared Data Guard requirements | Before You Begin for Data Guard | **[Review the shared requirements](#before-you-begin-for-data-guard)** |
| Namespace | `default` | Create the resources and referenced Secrets in this namespace. |
| Database admin Secret | `db-admin-secret`, key `oracle_pwd` | **[Create admin Secret](./prerequisites/PREREQUISITES.md#database-admin-secret)** |
| Database image and image-pull Secret | `container-registry.oracle.com/database/enterprise_ru:latest` / `ocr-reg-cred` | **[Create image-pull Secret](./prerequisites/PREREQUISITES.md#image-pull-secret)** |
| Persistent storage | StorageClass and capacity for both SIDBs | **[Prepare persistent storage](./prerequisites/PREREQUISITES.md#set-up-kubernetes-and-volumes-for-database-persistence)** |

**Review Manifest Requirements**

> The sample YAMLs already contain these values. If you change any example value, update every dependent reference (YAML fields, `$PRIMARY_DB` / `$STANDBY_DB` / `$DGB`, Service DNS names, and Broker members) before applying.

Primary SIDB — [`singleinstancedatabase_dataguard_primary.yaml`](../../config/samples/sidb/singleinstancedatabase_dataguard_primary.yaml)

| Setting | Example value | Required relationship |
| --- | --- | --- |
| `metadata.name` | `sidb-sample` | Matches `$PRIMARY_DB` |
| `metadata.namespace` | `default` | Matches `$NS` and the Secrets namespace |
| `sid` | `ORCL1` | Unique primary SID; Broker uses this as `dbUniqueName` |
| `pdbName` | `orclpdb1` | PDB created on the primary |
| `createAs` | `primary` | Primary role for this workflow |
| `archiveLog` | `true` | Required for Data Guard redo transport |
| `forceLog` | `true` | Required for Data Guard redo transport |
| `dataguard.prereqs.enabled` | `true` | Enables database-side Data Guard prerequisites |
| Admin Secret | `db-admin-secret` / `oracle_pwd` | Same Secret used by standby and Broker |

Standby SIDB — [`singleinstancedatabase_dataguard_standby.yaml`](../../config/samples/sidb/singleinstancedatabase_dataguard_standby.yaml)

| Setting | Example value | Required relationship |
| --- | --- | --- |
| `metadata.name` | `standbydatabase-sample` | Matches `$STANDBY_DB` |
| `metadata.namespace` | `default` | Same namespace as the primary |
| `sid` | `ORCLS` | Unique standby SID; Broker uses this as `dbUniqueName` |
| `createAs` | `standby` | Physical standby role for this workflow |
| `primarySource.databaseRef` | `sidb-sample` | Must equal the primary `metadata.name` |
| Admin Secret | `db-admin-secret` / `oracle_pwd` | Same Secret as the primary |

Data Guard Broker — [`dataguardbroker_same_cluster_tcp.yaml`](../../config/samples/sidb/dataguardbroker_same_cluster_tcp.yaml)

| Setting | Example value | Required relationship |
| --- | --- | --- |
| `metadata.name` | `standbydatabase-sample-dg` | Matches `$DGB`; create only after both SIDBs are Healthy |
| `metadata.namespace` | `default` | Same namespace as both SIDBs |
| `topology.sourceRef.name` | `standbydatabase-sample` | Standby SIDB that owns the Broker runner |
| Primary member `name` / `dbUniqueName` | `sidb-sample` / `ORCL1` | Match primary `metadata.name` and `sid` |
| Standby member `name` / `dbUniqueName` | `standbydatabase-sample` / `ORCLS` | Match standby `metadata.name` and `sid` |
| Primary endpoint host | `sidb-sample.default.svc.cluster.local` | Primary Service DNS name in `$NS` |
| Standby endpoint host | `standbydatabase-sample.default.svc.cluster.local` | Standby Service DNS name in `$NS` |
| Admin Secret | `db-admin-secret` / `oracle_pwd` | Same Secret as both SIDBs |

**1. Create the primary**

Use [`singleinstancedatabase_dataguard_primary.yaml`](../../config/samples/sidb/singleinstancedatabase_dataguard_primary.yaml).

```sh
kubectl apply -f config/samples/sidb/singleinstancedatabase_dataguard_primary.yaml
kubectl wait --for=jsonpath='{.status.status}'=Healthy \
  "singleinstancedatabase/$PRIMARY_DB" -n "$NS" --timeout=60m
kubectl get singleinstancedatabase "$PRIMARY_DB" -n "$NS" -o wide
```

Verify status `Healthy` and role `PRIMARY` before continuing.

**2. Create the standby**

Use [`singleinstancedatabase_dataguard_standby.yaml`](../../config/samples/sidb/singleinstancedatabase_dataguard_standby.yaml). Its `primarySource.databaseRef` already matches the primary resource name.

```sh
kubectl apply -f config/samples/sidb/singleinstancedatabase_dataguard_standby.yaml
kubectl wait --for=jsonpath='{.status.status}'=Healthy \
  "singleinstancedatabase/$STANDBY_DB" -n "$NS" --timeout=60m
kubectl get singleinstancedatabase "$STANDBY_DB" -n "$NS" -o wide
```

Verify status `Healthy` and role `PHYSICAL_STANDBY`. For bootstrap failures, inspect the [SIDB diagnostics](#collect-diagnostics).

**3. Configure Data Guard Broker**

Both SIDBs must be `Healthy`, with roles `PRIMARY` and `PHYSICAL_STANDBY`, before creating Broker. Choose one option:

- **If you used the primary and standby sample YAMLs with the same names and values:** apply [`dataguardbroker_same_cluster_tcp.yaml`](../../config/samples/sidb/dataguardbroker_same_cluster_tcp.yaml) using the commands below. Skip [Create the Data Guard Broker Configuration](#create-the-data-guard-broker-configuration).
- **If you changed any name or value:** follow [Create the Data Guard Broker Configuration](#create-the-data-guard-broker-configuration) to generate, review, and apply a Broker YAML for your deployment. Skip the sample apply commands below.

```sh
kubectl apply -f config/samples/sidb/dataguardbroker_same_cluster_tcp.yaml
kubectl get dataguardbroker "$DGB" -n "$NS" -o wide
kubectl describe dataguardbroker "$DGB" -n "$NS"
```

When Broker is `Healthy`, creation is complete. Continue to [Connect to the Broker Runner](#connect-to-the-broker-runner) and [Watch Data Guard Status](#watch-data-guard-status) to verify synchronization. Create only one Broker resource for the pair.

<a id="dataguard-setup-enabled-with-tde-and-tcps"></a>

#### Data Guard Setup with TDE and TCPS

This **same-cluster** workflow creates the same database pair with [TDE encryption](./prerequisites/PREREQUISITES.md#tde-secret) and [TCPS listeners](./tcps-cert-manager/README.md). The standby uses `primarySource.databaseRef` to reach the primary in the same Kubernetes cluster and namespace. The primary YAML enables `dataguard.standbySources` for standby `ORCLS` at TCP `1521` and TCPS `2484`.

<a id="tde-tcps-workflow-settings"></a>

**Required Before Deployment**

> **Important:** Complete every action in the **Complete before applying** column before applying a related manifest. These requirements are mandatory for this deployment scenario.

| Requirement | Example value | Complete before applying |
| --- | --- | --- |
| Shared Data Guard requirements | Before You Begin for Data Guard | **[Review the shared requirements](#before-you-begin-for-data-guard)** |
| Namespace | `default` | Create the resources and referenced Secrets in this namespace. |
| Database admin Secret | `db-admin-secret`, key `oracle_pwd` | **[Create admin Secret](./prerequisites/PREREQUISITES.md#database-admin-secret)** |
| Database image and image-pull Secret | `container-registry.oracle.com/database/enterprise_ru:latest` / `ocr-reg-cred` | **[Create image-pull Secret](./prerequisites/PREREQUISITES.md#image-pull-secret)** |
| Persistent storage | StorageClass and capacity for both SIDBs | **[Prepare persistent storage](./prerequisites/PREREQUISITES.md#set-up-kubernetes-and-volumes-for-database-persistence)** |
| Primary TDE password Secret | `tde-wallet-secret`, key `tde_wallet_pwd` | **[Create primary TDE Secret](./prerequisites/PREREQUISITES.md#tde-secret)** |
| TCPS certificates and TLS Secrets | `sidb-primary-tcps-tls` and `sidb-standby-tcps-tls` | **[Create certificates and TLS Secrets](./tcps-cert-manager/README.md)** |
| Standby TDE wallet Secret | `sidb-standby-tde-wallet`, password key `tde_wallet_pwd`, archive key `wallet.zip` | **[Create the standby wallet Secret](./prerequisites/PREREQUISITES.md#tde-secret)** after exporting the current primary wallet in step 5 and before applying the standby manifest. |

**Review Manifest Requirements**

> The sample YAMLs already contain these values. If you change any example value, update every dependent reference (YAML fields, `$PRIMARY_DB` / `$STANDBY_DB` / `$DGB`, Service DNS names, certificate SANs, ports, and Secret names) before applying.

Primary SIDB — [`singleinstancedatabase_dataguard_primary_tde_tcps.yaml`](../../config/samples/sidb/singleinstancedatabase_dataguard_primary_tde_tcps.yaml)

| Setting | Example value | Required relationship |
| --- | --- | --- |
| `metadata.name` | `sidb-sample` | Matches `$PRIMARY_DB` |
| `metadata.namespace` | `default` | Matches `$NS` and the Secrets namespace |
| `sid` | `ORCL1` | Unique primary SID; Broker uses this as `dbUniqueName` |
| `pdbName` | `orclpdb1` | PDB created on the primary |
| `createAs` | `primary` | Primary role for this workflow |
| `archiveLog` / `forceLog` | `true` / `true` | Required for Data Guard redo transport |
| `dataguard.prereqs.enabled` | `true` | Enables database-side Data Guard prerequisites |
| `dataguard.standbySources[].dbUniqueName` | `ORCLS` | Must equal the standby `sid` |
| `dataguard.standbySources[].host` | `standbydatabase-sample.default.svc.cluster.local` | Standby Service DNS name |
| `dataguard.standbySources[].tcpPort` / `tcpsPort` | `1521` / `2484` | Standby TCP and TCPS listener ports |
| Admin Secret | `db-admin-secret` / `oracle_pwd` | Same admin Secret as standby and Broker |
| TDE Secret | `tde-wallet-secret` / `tde_wallet_pwd` | Primary TDE password Secret |
| TCPS TLS Secret | `sidb-primary-tcps-tls` | Primary certificate Secret from cert-manager |

Standby SIDB — [`singleinstancedatabase_dataguard_standby_tde_tcps.yaml`](../../config/samples/sidb/singleinstancedatabase_dataguard_standby_tde_tcps.yaml)

| Setting | Example value | Required relationship |
| --- | --- | --- |
| `metadata.name` | `standbydatabase-sample` | Matches `$STANDBY_DB` |
| `metadata.namespace` | `default` | Same namespace as the primary |
| `sid` | `ORCLS` | Unique standby SID; must match primary `standbySources.dbUniqueName` |
| `createAs` | `standby` | Physical standby role for this workflow |
| `primarySource.databaseRef` | `sidb-sample` | Must equal the primary `metadata.name` |
| Admin Secret | `db-admin-secret` / `oracle_pwd` | Same admin Secret as the primary |
| Standby TDE wallet Secret | `sidb-standby-tde-wallet` / `tde_wallet_pwd` + `wallet.zip` | Created from the exported primary wallet before apply |
| TCPS TLS Secret | `sidb-standby-tcps-tls` | Standby certificate Secret from cert-manager |

Data Guard Broker — [`dataguardbroker_same_cluster_tde_tcps.yaml`](../../config/samples/sidb/dataguardbroker_same_cluster_tde_tcps.yaml)

| Setting | Example value | Required relationship |
| --- | --- | --- |
| `metadata.name` | `standbydatabase-sample-dg` | Matches `$DGB`; create only after both SIDBs are Healthy |
| `metadata.namespace` | `default` | Same namespace as both SIDBs |
| `topology.sourceRef.name` | `standbydatabase-sample` | Standby SIDB that owns the Broker runner |
| Primary member `name` / `dbUniqueName` | `sidb-sample` / `ORCL1` | Match primary `metadata.name` and `sid` |
| Standby member `name` / `dbUniqueName` | `standbydatabase-sample` / `ORCLS` | Match standby `metadata.name` and `sid` |
| Primary TCP / TCPS endpoints | `sidb-sample.default.svc.cluster.local:1521` / `:2484` | Primary Service DNS name and ports |
| Standby TCP / TCPS endpoints | `standbydatabase-sample.default.svc.cluster.local:1521` / `:2484` | Standby Service DNS name and ports |
| Admin Secret | `db-admin-secret` / `oracle_pwd` | Same Secret as both SIDBs |

**1. Prepare the Secrets and certificates**

Create the [admin Secret](./prerequisites/PREREQUISITES.md#database-admin-secret) and [primary TDE password Secret](./prerequisites/PREREQUISITES.md#tde-secret) using the [settings in the table above](#tde-tcps-workflow-settings). The standby TDE wallet Secret is created in step 5, after exporting the primary wallet.

With cert-manager installed, follow the [certificate helper procedure](./tcps-cert-manager/README.md#run) to create the shared CA, both certificates, and their TLS Secrets.

Use the same Kubernetes context for both databases. Set the helper's namespace, TLS Secret names, and certificate DNS names to match the [settings in the table above](#tde-tcps-workflow-settings), rather than using its default database DNS names. Include each database's Service DNS names in its certificate SANs.

Verify certificate readiness and Secret types:

```sh
kubectl wait -n "$NS" --for=condition=Ready \
  certificate/sidb-primary-tcps certificate/sidb-standby-tcps --timeout=5m
kubectl get secret sidb-primary-tcps-tls sidb-standby-tcps-tls -n "$NS" \
  -o custom-columns=NAME:.metadata.name,TYPE:.type
```

Continue when the wait succeeds and both Secrets have type `kubernetes.io/tls`.

**2. Create the primary**

Use [`singleinstancedatabase_dataguard_primary_tde_tcps.yaml`](../../config/samples/sidb/singleinstancedatabase_dataguard_primary_tde_tcps.yaml). It references the [primary TDE password Secret](./prerequisites/PREREQUISITES.md#tde-secret) and [primary TLS Secret](./tcps-cert-manager/README.md#verify-tls-secrets), and uses the operator-created default ClusterIP Service.

```sh
kubectl apply -f config/samples/sidb/singleinstancedatabase_dataguard_primary_tde_tcps.yaml
kubectl wait --for=jsonpath='{.status.status}'=Healthy \
  "singleinstancedatabase/$PRIMARY_DB" -n "$NS" --timeout=60m
kubectl get singleinstancedatabase "$PRIMARY_DB" -n "$NS" -o wide
kubectl get svc "$PRIMARY_DB" -n "$NS"
```

Verify `Healthy` / `PRIMARY` and that the Service exposes both listener ports before exporting the wallet.

**3. Locate the primary TDE wallet**

The standby must receive the wallet exported from the current primary, not a wallet from an earlier database creation. See the [TDE Secret reference](./prerequisites/PREREQUISITES.md#tde-secret) for password handling.

Capture the wallet path into `WALLET_ROOT` in the same shell used for the next steps. Printing the SQL result alone does not set this shell variable.

```sh
PRIMARY_POD=$(kubectl get pod -n "$NS" -l "app=$PRIMARY_DB" \
  -o jsonpath='{.items[0].metadata.name}')

WALLET_ROOT=$(kubectl exec -i "$PRIMARY_POD" -n "$NS" -c "$PRIMARY_DB" -- \
  sqlplus -L -s / as sysdba <<'SQL'
whenever sqlerror exit failure
set heading off feedback off pages 0 verify off echo off linesize 400
select value from v$parameter where name = 'wallet_root';
exit
SQL
)
WALLET_ROOT=$(printf '%s\n' "$WALLET_ROOT" | \
  sed '/^[[:space:]]*$/d; s/^[[:space:]]*//; s/[[:space:]]*$//')

test -n "$WALLET_ROOT" && kubectl exec "$PRIMARY_POD" -n "$NS" -c "$PRIMARY_DB" -- \
  test -d "$WALLET_ROOT/tde"
```

Continue only if the directory check succeeds. For the sample primary, the wallet root is normally `/opt/oracle/oradata/ORCL1/tdewallet`; use the value returned by the database.

**4. Export and validate the wallet archive**

Copy the wallet to a private temporary directory and create the archive locally. This uses the `zip` utility on your workstation; it does not require `zip` in the database image. `kubectl cp` requires `tar` in the source container.

```sh
WALLET_DIR=$(mktemp -d)
chmod 700 "$WALLET_DIR"
test -n "$WALLET_ROOT" &&
kubectl cp -n "$NS" -c "$PRIMARY_DB" \
  "$PRIMARY_POD:$WALLET_ROOT" "$WALLET_DIR/wallet-root" &&
(cd "$WALLET_DIR/wallet-root" && zip -qr "$WALLET_DIR/standby-wallet.zip" tde) &&
unzip -t "$WALLET_DIR/standby-wallet.zip" &&
unzip -Z1 "$WALLET_DIR/standby-wallet.zip"
```

Verify the archive contains the `tde/` directory and its wallet files, including `ewallet.p12` and any required `cwallet.sso`. Preserve the directory layout; do not zip only the parent directory or use an archive from a different primary.

**5. Create the standby TDE wallet Secret**

Create or update the [standby TDE Secret](./prerequisites/PREREQUISITES.md#tde-secret) with the wallet archive exported from the primary. This Secret is separate from the [standby TLS Secret](./tcps-cert-manager/README.md#verify-tls-secrets).

**Oracle Database 19c standby:** include only the wallet zip (`wallet.zip` / `walletZipFileKey`). Do **not** set `spec.security.secrets.tde.secretKey` on the standby SIDB. The validating webhook rejects `secretKey` for 19c standby images (including OCR tags such as `:128786` when that tag family is used).

**Oracle Database 23ai and later standby:** the Secret must also include the password that opens the primary wallet, and the standby SIDB must set `secretKey` to that password key.

Copy the password bytes from the primary's `tde-wallet-secret` instead of retyping a password or leaving a placeholder. If you changed the primary TDE Secret name or key in its YAML, use that same reference below.

```sh
(
  set -euo pipefail
  umask 077
  test -s "$WALLET_DIR/standby-wallet.zip"

  kubectl get secret tde-wallet-secret -n "$NS" -o json | \
    jq -er '.data.tde_wallet_pwd // error("Primary TDE password key is missing")' | \
    base64 -d > "$WALLET_DIR/tde-wallet-password"
  test -s "$WALLET_DIR/tde-wallet-password"

  kubectl create secret generic sidb-standby-tde-wallet -n "$NS" \
    --from-file=tde_wallet_pwd="$WALLET_DIR/tde-wallet-password" \
    --from-file=wallet.zip="$WALLET_DIR/standby-wallet.zip" \
    --dry-run=client -o yaml | kubectl apply -f -

  kubectl get secret sidb-standby-tde-wallet -n "$NS" -o json | \
    jq -er '.data.tde_wallet_pwd // error("Standby TDE password key is missing")' | \
    base64 -d > "$WALLET_DIR/standby-wallet-password"
  cmp -s "$WALLET_DIR/tde-wallet-password" "$WALLET_DIR/standby-wallet-password"
  printf '%s\n' 'TDE wallet passwords match.'
  rm -f "$WALLET_DIR/tde-wallet-password" "$WALLET_DIR/standby-wallet-password"
)
kubectl describe secret sidb-standby-tde-wallet -n "$NS"
```

Continue only after the password comparison succeeds and the Secret contains `tde_wallet_pwd` and `wallet.zip`. The commands do not print the password. Keep the [TDE Secret](./prerequisites/PREREQUISITES.md#tde-secret) and wallet archive from the same primary deployment. The samples use `container-registry.oracle.com/database/enterprise_ru:latest`; when adapting to another image or database release, follow that image's standby-wallet requirements.

**6. Create the standby**

Use [`singleinstancedatabase_standby_tcps_connectstring.yaml`](../../config/samples/sidb/singleinstancedatabase_standby_tcps_connectstring.yaml). The [TDE wallet](./prerequisites/PREREQUISITES.md#tde-secret) and [TLS Secret](./tcps-cert-manager/README.md#verify-tls-secrets) references are already enabled. `primarySource.connectString` points at the primary Service; the alternative `databaseRef` remains commented so exactly one source selector is active. Its wallet root uses the standby SID `ORCLS`. For **19c**, remove `secretKey` from the sample `tde` block before apply; for **23ai+**, keep `secretKey`.

```sh
kubectl apply -f config/samples/sidb/singleinstancedatabase_standby_tcps_connectstring.yaml
kubectl wait --for=jsonpath='{.status.status}'=Healthy \
  "singleinstancedatabase/$STANDBY_DB" -n "$NS" --timeout=60m
kubectl get singleinstancedatabase "$STANDBY_DB" -n "$NS" -o wide
kubectl get svc "$STANDBY_DB" -n "$NS"
```

Verify `Healthy` / `PHYSICAL_STANDBY` and TCP/TCPS listener ports. If bootstrap fails, inspect the Pod's current and previous logs and verify the [TDE wallet Secret](./prerequisites/PREREQUISITES.md#tde-secret), archive layout, and [TLS certificate chain](./tcps-cert-manager/README.md#verify-the-certificate-chain) before retrying.

After the Secret is created and no local copy is needed, remove the temporary wallet directory:

```sh
rm -rf "$WALLET_DIR"
```

**7. Configure Data Guard Broker**

Both SIDBs must be `Healthy`, with roles `PRIMARY` and `PHYSICAL_STANDBY`. Verify their [Broker client-wallet Secrets](#tcps-client-wallet-placement-for-the-broker-runner), then choose one option:

- **If you used the primary and standby sample YAMLs with the same names and values, including the published wallet names:** apply [`dataguardbroker_same_cluster_tde_tcps.yaml`](../../config/samples/sidb/dataguardbroker_same_cluster_tde_tcps.yaml) using the commands below. Skip [Create the Data Guard Broker Configuration](#create-the-data-guard-broker-configuration).
- **If you changed any name, value, or wallet reference:** follow [Create the Data Guard Broker Configuration](#create-the-data-guard-broker-configuration) to generate, review, and apply a Broker YAML for your deployment. Skip the sample apply commands below.

```sh
kubectl apply -f config/samples/sidb/dataguardbroker_same_cluster_tde_tcps.yaml
kubectl get dataguardbroker "$DGB" -n "$NS" -o wide
kubectl describe dataguardbroker "$DGB" -n "$NS"
```

When Broker is `Healthy`, creation is complete. Continue to [Connect to the Broker Runner](#connect-to-the-broker-runner) and [Watch Data Guard Status](#watch-data-guard-status) to verify synchronization. Create only one Broker resource for the pair.

#### Create the Data Guard Broker Configuration

Follow this section only if you changed the sample values or need to generate a Broker manifest from current standby status. Both SIDBs must be `Healthy` before generation. If you already applied a supplied Broker YAML, skip to [Connect to the Broker Runner](#connect-to-the-broker-runner).

##### Generate the Data Guard Broker YAML from Standby Status

The scripts read SIDB `status.dataguard.renderedBrokerSpec`. Use Bash 4 or later, `kubectl`, and `jq` (`yq` is optional for YAML output). Keep the namespace and resource variables from [Set the example variables](#set-the-example-variables) set in your terminal.

Copy the helpers from the sample directory:

```sh
cp config/samples/sidb/render-dg-broker-from-status.sh .
cp config/samples/sidb/gen_dg.sh .
chmod +x render-dg-broker-from-status.sh gen_dg.sh
```

Run **one** command, based on the standby's source selector:

**For `primarySource.databaseRef`** — use the wrapper. It discovers matching same-namespace standbys automatically:

```sh
./gen_dg.sh
```

**For `primarySource.connectString`** — select the standby directly:

```sh
./render-dg-broker-from-status.sh sidb "$STANDBY_DB" "$NS" "$DGB" > dataguardbroker.yaml
```

Either command creates `dataguardbroker.yaml`. The renderer prefers YAML when `yq` is installed and otherwise writes JSON. Prefer YAML so the reviewed file matches the sample manifests; convert JSON with `yq -P` before review when needed. Review the generated `metadata.name` and use that name for `DGB` in subsequent commands if it differs.

##### TCPS Client Wallet Placement for the Broker Runner

Complete this check for every **TDE and TCPS** Broker workflow, including same-cluster and cross-cluster deployments, before applying either a sample or generated Broker manifest. The operator publishes each SIDB's client-wallet Secret in `status.clientWalletSecret`:

```sh
kubectl get singleinstancedatabase "$PRIMARY_DB" "$STANDBY_DB" -n "$NS" \
  -o custom-columns=NAME:.metadata.name,WALLET:.status.clientWalletSecret
kubectl get secret sidb-sample-dg-client-wallet \
  standbydatabase-sample-dg-client-wallet -n "$NS"
```

Use the published Secret names if they differ. All referenced client-wallet Secrets must exist in the Broker namespace, `$NS`.

The [TDE and TCPS Broker sample](../../config/samples/sidb/dataguardbroker_same_cluster_tde_tcps.yaml) sets the primary's `tcps.clientWalletSecret` to `sidb-sample-dg-client-wallet`. The standby inherits `standbydatabase-sample-dg-client-wallet` from `topology.defaults.tcps.clientWalletSecret`. Both members have `localRef` entries for their SIDBs.

These Data Guard client-wallet Secrets are separate from the [TDE wallet](./prerequisites/PREREQUISITES.md#tde-secret), server TLS Secrets created by the certificate script, and Broker `spec.execution.authWallet`. The portable zip steps in [TCPS Client Wallet](./tcps-cert-manager/TCPS_CLIENT_WALLET.md) are for external clients; do not use that zip path for Broker Secret placement.

**Cross-cluster / separate-bastion Broker handoff:** when Broker runs in the standby cluster and the primary SIDB runs in another cluster, copy the primary client-wallet Secret into the Broker namespace before apply:

```sh
kubectl --context "$PRIMARY_CTX" -n "$NS" get secret "$PRIMARY_DB-dg-client-wallet" -o json \
  | jq '{apiVersion,kind,metadata:{name:.metadata.name,namespace:"'"$NS"'"},type,data}' \
  | kubectl --context "$STANDBY_CTX" apply -f -
```

If the contexts cannot share a pipe, write the JSON on the primary side, copy the file, and apply it on the standby side. Set each Broker member `tcps.clientWalletSecret` to the Secret name that exists in the Broker namespace. For the full TCPS + NLB path, see [Cross-Cluster Data Guard over TCPS with NLB](./dataguard/DATAGUARD_CROSS_CLUSTER_TCPS_NLB.md).

##### Review the Generated Data Guard Broker YAML

```sh
cat dataguardbroker.yaml
```

Compare the structure with the matching sample: [TCP Broker](../../config/samples/sidb/dataguardbroker_same_cluster_tcp.yaml) or [TDE and TCPS Broker](../../config/samples/sidb/dataguardbroker_same_cluster_tde_tcps.yaml). Confirm your generated values match your deployment:

- Broker name and namespace match `DGB` and `NS`.
- Both members have the correct database unique names, current roles, Service endpoints, and same-cluster SIDB `localRef` entries.
- Execution image, image pull Secret, and admin Secret name/key match your SIDBs.
- For TDE and TCPS, verify the [effective client-wallet references](#tcps-client-wallet-placement-for-the-broker-runner), including the primary's own wallet override. Replace any remaining placeholders before applying.

##### Apply the Generated Broker YAML

```sh
kubectl apply -f dataguardbroker.yaml
kubectl get dataguardbroker "$DGB" -n "$NS" -o wide
kubectl describe dataguardbroker "$DGB" -n "$NS"
```

When Broker is `Healthy`, creation is complete. Continue to [Connect to the Broker Runner](#connect-to-the-broker-runner) and [Watch Data Guard Status](#watch-data-guard-status) for verification. If Broker remains unhealthy, see [Troubleshoot Broker YAML Generation](#troubleshoot-broker-yaml-generation).

##### Connect to the Broker Runner

Find the runner Pod using the Broker resource name:

```sh
RUNNER=$(kubectl get pods -n "$NS" \
  -o jsonpath='{range .items[*]}{.metadata.name}{"\n"}{end}' | \
  grep "^${DGB}-runner-" | head -n 1)
test -n "$RUNNER" && kubectl get pod "$RUNNER" -n "$NS"
```

Once the Pod is ready, list its Oracle Net aliases:

```sh
kubectl exec "$RUNNER" -n "$NS" -- bash -c \
  'grep -E "^[[:space:]]*[A-Za-z][A-Za-z0-9_.-]*[[:space:]]*=" "$TNS_ADMIN/tnsnames.ora"'
```

For the TDE and TCPS sample, connect to the initial primary using `ORCL1TCPS`. For TCP, replace `ORCL1TCPS` below with the current primary's TCP alias listed above.

```sh
kubectl exec -it "$RUNNER" -n "$NS" -- dgmgrl /@ORCL1TCPS
```

After a switchover, select the new primary's alias. For example, use `ORCLSTCPS` when `ORCLS` is the primary in the TDE and TCPS workflow.

##### Watch Data Guard Status

```sh
kubectl get dataguardbroker "$DGB" -n "$NS" -o wide
kubectl get singleinstancedatabase "$PRIMARY_DB" "$STANDBY_DB" -n "$NS" -o wide
kubectl get pods -n "$NS" -o wide
```

From the [authenticated runner session](#connect-to-the-broker-runner), verify the configuration, database roles, transport/apply state, and lag before operations:

```text
SHOW CONFIGURATION;
SHOW DATABASE 'ORCL1';
SHOW DATABASE 'ORCLS';
VALIDATE DATABASE 'ORCLS';
```

Expect Broker `SUCCESS`, the intended primary/standby roles, and healthy redo transport/apply. Investigate gaps or stale/unknown lag before a planned switchover.

##### Troubleshoot Broker YAML Generation

Check that both SIDBs are healthy and that the standby publishes `status.dataguard.readyForBroker: true` and `status.dataguard.renderedBrokerSpec`. The primary may not publish either field in this workflow. In that case, a wrapper message about synthesizing/merging the primary from the standby spec is not itself a failure; verify that the generated manifest contains both correct members. Single-member rendering checks readiness on the selected standby. Confirm that the wrapper's exported namespace and primary name match the manifests; a connect-string standby requires the direct renderer command above.

For the TDE and TCPS workflow, also verify the [admin Secret](./prerequisites/PREREQUISITES.md#database-admin-secret), [TDE wallet](./prerequisites/PREREQUISITES.md#tde-secret), [TLS Secrets](./tcps-cert-manager/README.md#verify-tls-secrets), and [Broker client wallets](#tcps-client-wallet-placement-for-the-broker-runner) are the correct, distinct resources.

```sh
kubectl get singleinstancedatabase "$STANDBY_DB" -n "$NS" \
  -o jsonpath='{.status.dataguard.readyForBroker}{"\n"}{.status.dataguard.renderedBrokerSpec}{"\n"}'
kubectl describe dataguardbroker "$DGB" -n "$NS"
kubectl get events -n "$NS" --sort-by=.lastTimestamp
```

#### Perform Data Guard Operations

Choose one operation below and use either **A. kubectl commands (Recommended approach)** or **B. DGMGRL** for that operation. Do not issue both at the same time. Keep the operator running for kubectl commands and wait for each operation to complete before starting another.

| Operation | Procedure |
| --- | --- |
| Planned role reversal | [Switchover](#switchover) |
| Change data protection | [Protection mode](#change-protection-mode) |
| Use a temporarily writable standby | [Physical / snapshot conversion](#convert-between-physical-and-snapshot-standby) |
| Configure automatic failover | [FSFO](#enable-and-disable-fast-start-failover) |

Set the namespace and actual Broker resource name before operations:

```sh
NS=default
DG=standbydatabase-sample-dg
kubectl get dataguardbroker "$DG" -n "$NS" -o wide
kubectl get singleinstancedatabase -n "$NS" \
  -o custom-columns=NAME:.metadata.name,STATUS:.status.status,ROLE:.status.role
```

The kubectl operation examples apply only to an operator/CRD release that exposes the shown topology-based fields. Confirm support before using patches; otherwise use the DGMGRL procedure supported by your release:

```sh
kubectl explain dataguardbroker.spec.operations --api-version=database.oracle.com/v4
```

Each operation needs a new `requestId`; examples below generate a time-based value. Targets are database `DB_UNIQUE_NAME` values (`ORCL1`, `ORCLS`), not SIDB resource names. FSFO is a desired-state setting and does not use `requestId`.

For DGMGRL, use [Connect to the Broker Runner](#connect-to-the-broker-runner) and select the current primary's published alias. Verify `SHOW CONFIGURATION;` before each operation. Resource names stay the same after a role change; use the reported database roles to select the target.

##### Switchover

Use switchover for planned role reversal. Both databases and Broker must be healthy, the target must be synchronized, and no operation may be in progress. With FSFO enabled, only its configured target is eligible and the required synchronization/lag limit must be met.

**A. Using kubectl commands (Recommended approach)**

Switch to `ORCLS` when it is the current physical standby:

```sh
REQUEST_ID="switchover-$(date +%s)"
kubectl patch dataguardbroker "$DG" -n "$NS" --type=merge \
  -p "{\"spec\":{\"operations\":{\"switchover\":{\"target\":\"ORCLS\",\"requestId\":\"$REQUEST_ID\"}}}}"
```

To switch back, use `target: ORCL1` with a new request ID after it is healthy as a physical standby.

**B. Using DGMGRL**

```text
SHOW CONFIGURATION;
VALIDATE DATABASE 'ORCLS';
SWITCHOVER TO 'ORCLS';
```

For switchback, validate `ORCL1` as the current standby, then run `SWITCHOVER TO 'ORCL1';`.

**Verify after either method**

Reconnect to the current primary using [the runner procedure](#connect-to-the-broker-runner), then run:

```text
SHOW CONFIGURATION;
SHOW DATABASE 'ORCL1';
SHOW DATABASE 'ORCLS';
```

After switching to `ORCLS`, expect `ORCLS` to be primary and `ORCL1` to be physical standby; switchback reverses these roles. Confirm configuration status `SUCCESS`, standby state `APPLY-ON`, and current transport/apply lag before another operation.


> If SIDB or DataguardBroker remains unhealthy after switchover, see [Transient Reconciliation Errors During Data Guard Switchover](#transient-reconciliation-errors-during-data-guard-switchover).

##### Change Protection Mode

For the examples below, disable FSFO first using [the FSFO procedure](#enable-and-disable-fast-start-failover) and confirm it is disabled. Prepare appropriate standby redo logs and transport settings before increasing protection. For the direct primary/standby examples, use `SYNC` for maximum protection; maximum availability supports `SYNC` or, where supported, `FASTSYNC`. Configure the future standby as well so role reversal can preserve protection.

**A. Using kubectl commands (Recommended approach)**

Set `MaxAvailability`:

```sh
REQUEST_ID="availability-$(date +%s)"
kubectl patch dataguardbroker "$DG" -n "$NS" --type=merge \
  -p "{\"spec\":{\"operations\":{\"protectionMode\":{\"mode\":\"MaxAvailability\",\"requestId\":\"$REQUEST_ID\"}}}}"
```

From `MaxAvailability`, set `MaxProtection` after validating synchronized transport:

```sh
REQUEST_ID="protection-$(date +%s)"
kubectl patch dataguardbroker "$DG" -n "$NS" --type=merge \
  -p "{\"spec\":{\"operations\":{\"protectionMode\":{\"mode\":\"MaxProtection\",\"requestId\":\"$REQUEST_ID\"}}}}"
```

Set `MaxPerformance`:

```sh
REQUEST_ID="performance-$(date +%s)"
kubectl patch dataguardbroker "$DG" -n "$NS" --type=merge \
  -p "{\"spec\":{\"operations\":{\"protectionMode\":{\"mode\":\"MaxPerformance\",\"requestId\":\"$REQUEST_ID\"}}}}"
```

Do not assume a patch also changes `LogXptMode` unless the deployed operator documents that behavior. Verify the effective transport mode and Broker result.

**B. Using DGMGRL**

For `ORCLS` as the current standby, prepare synchronous transport before increasing protection:

```text
EDIT DATABASE 'ORCLS' SET PROPERTY LogXptMode='SYNC';
EDIT CONFIGURATION SET PROTECTION MODE AS MaxAvailability;
```

To move from maximum availability to maximum protection:

```text
EDIT CONFIGURATION SET PROTECTION MODE AS MaxProtection;
```

To reduce protection, change the protection mode before reducing transport guarantees:

```text
EDIT CONFIGURATION SET PROTECTION MODE AS MaxPerformance;
EDIT DATABASE 'ORCLS' SET PROPERTY LogXptMode='ASYNC';
```

Broker does not allow a direct upgrade from `MaxPerformance` to `MaxProtection`; go through `MaxAvailability`. Maximum protection can shut down the primary if no synchronized standby can accept redo. See [Oracle protection-mode documentation](https://docs.oracle.com/en/database/oracle/oracle-database/26/dgbkr/managing-data-protection-modes.html).

**Verify after either method**

After each mode change, run these checks in the authenticated DGMGRL session:

```text
SHOW CONFIGURATION;
SHOW DATABASE 'ORCL1' 'LogXptMode';
SHOW DATABASE 'ORCLS' 'LogXptMode';
```

Confirm `Protection Mode` matches the requested mode and configuration status is `SUCCESS`. Check the current standby's transport mode is appropriate for that protection mode; also check the other member's setting before a future role reversal.

##### Convert Between Physical and Snapshot Standby

For this workflow, disable FSFO and wait for the disabled state before conversion. The target is a standby database unique name. Verify the effective database prerequisites before conversion: a fast recovery area (FRA) must be configured with adequate space for the guaranteed restore point and accumulated redo. Enable any optional database setting required by your installed operator/image before requesting the conversion. Snapshot changes are discarded when returning to physical standby. See [Oracle snapshot standby documentation](https://docs.oracle.com/en/database/oracle/oracle-database/26/sbydb/managing-oracle-data-guard-physical-standby-databases.html).

**A. Using kubectl commands (Recommended approach)**

Convert the physical standby to snapshot standby:

```sh
REQUEST_ID="snapshot-$(date +%s)"
kubectl patch dataguardbroker "$DG" -n "$NS" --type=merge \
  -p "{\"spec\":{\"operations\":{\"roleConversion\":{\"target\":\"ORCLS\",\"role\":\"SNAPSHOT_STANDBY\",\"requestId\":\"$REQUEST_ID\"}}}}"
```

Convert it back to physical standby:

```sh
REQUEST_ID="physical-$(date +%s)"
kubectl patch dataguardbroker "$DG" -n "$NS" --type=merge \
  -p "{\"spec\":{\"operations\":{\"roleConversion\":{\"target\":\"ORCLS\",\"role\":\"PHYSICAL_STANDBY\",\"requestId\":\"$REQUEST_ID\"}}}}"
```

Use one conversion mechanism; do not also patch the legacy SIDB `spec.convertToSnapshotStandby` field for the same request.

**B. Using DGMGRL**

```text
CONVERT DATABASE 'ORCLS' TO SNAPSHOT STANDBY;
```

To return it to physical standby:

```text
CONVERT DATABASE 'ORCLS' TO PHYSICAL STANDBY;
```

**Verify after either method**

After each conversion, run these checks in the authenticated DGMGRL session:

```text
SHOW CONFIGURATION;
SHOW DATABASE 'ORCLS';
```

Confirm the role is `SNAPSHOT STANDBY` or `PHYSICAL STANDBY`, as requested, and configuration status is `SUCCESS`. Redo is not applied while the database is a snapshot standby. After returning to physical standby, confirm `APPLY-ON` and wait for apply to catch up before switchover or enabling FSFO again.

##### Enable and Disable Fast-Start Failover

Validate the primary, physical standby, static connect identifiers, observer connectivity, transport mode, and lag policy before enabling automatic failover. A successful planned switchover does not by itself validate FSFO. A snapshot standby cannot be the FSFO target.

**A. Using kubectl commands (Recommended approach)**

For a release exposing `spec.topology.policy.fastStartFailover` and `spec.topology.observer`, configure desired state and let the operator manage its observer. No operation `requestId` is required.

```sh
OBSERVER_NAME="$DG-observer"
OBSERVER_IMAGE=$(kubectl get dataguardbroker "$DG" -n "$NS" \
  -o jsonpath='{.spec.execution.image}')
test -n "$OBSERVER_IMAGE" && kubectl patch dataguardbroker "$DG" -n "$NS" --type=merge \
  -p "{\"spec\":{\"topology\":{\"policy\":{\"fastStartFailover\":true},\"observer\":{\"enabled\":true,\"name\":\"$OBSERVER_NAME\",\"image\":\"$OBSERVER_IMAGE\"}}}}"
```

Disable FSFO and remove the observer configuration:

```sh
kubectl patch dataguardbroker "$DG" -n "$NS" --type=merge \
  -p '{"spec":{"topology":{"policy":{"fastStartFailover":false},"observer":null}}}'
```

Verify Broker's effective FSFO state after reconciliation and confirm the operator-managed observer is observing when enabled, or has stopped as expected when disabled. Do not rely only on the requested specification.

**B. Using DGMGRL**

Use this method for a configuration whose FSFO/observer lifecycle is managed through DGMGRL. Configure an authenticated observer using the procedure for your database version, then set the two-member targets and enable FSFO:

```text
EDIT DATABASE 'ORCLS' SET PROPERTY FastStartFailoverTarget='ORCL1';
EDIT DATABASE 'ORCL1' SET PROPERTY FastStartFailoverTarget='ORCLS';
ENABLE FAST_START FAILOVER;
```

To disable:

```text
DISABLE FAST_START FAILOVER;
```

For topology-based resources using method A, keep the desired state consistent rather than toggling FSFO independently through both interfaces.

**Verify after either method**

After enabling or disabling FSFO, run these checks in the authenticated DGMGRL session:

```text
SHOW FAST_START FAILOVER;
SHOW OBSERVER;
SHOW CONFIGURATION;
```

When enabled, confirm the intended active target, an observing master observer with recent primary/standby pings, and configuration status `SUCCESS`. When disabled, confirm `Fast-Start Failover: Disabled`; observer registration alone does not mean FSFO is enabled. With method A, also confirm the operator-managed observer has stopped as expected. Before a DR drill, confirm the protection mode and acceptable lag/data-loss policy.

#### Static Data Guard Connect String

Validate Broker's restart path from an authenticated DGMGRL session. The static connect identifiers must reach the appropriate listener's `*_DGMGRL` service even when a database is mounted or restarting:

```text
SHOW DATABASE VERBOSE 'ORCL1';
SHOW DATABASE VERBOSE 'ORCLS';
VALIDATE STATIC CONNECT IDENTIFIER FOR ALL;
```

For application-facing connection strings, inspect Broker status separately:

```sh
kubectl get dataguardbroker "$DGB" -n "$NS" \
  -o jsonpath='{.status.externalConnectString}{"\n"}{.status.clusterConnectString}{"\n"}'
```

Application connection strings do not substitute for static-identifier validation.

#### Create sample custom service

For a PDB service that follows the primary role after switchover or failover, see [Create sample custom service](./custom-service/CUSTOM_SERVICE.md). That document is a reference example; adapt and validate the service for your application before use.

#### Delete the Data Guard Configuration

Check current roles before deleting: the resource originally created as standby may now be primary. Disable FSFO using [the matching management method](#enable-and-disable-fast-start-failover) and verify the observer state first. Delete Broker before the database resource selected for removal.

```sh
kubectl get singleinstancedatabase "$PRIMARY_DB" "$STANDBY_DB" -n "$NS" -o wide
kubectl delete dataguardbroker "$DGB" -n "$NS" --wait=true
```

If the intended standby is still `$STANDBY_DB` and it is no longer needed:

```sh
kubectl delete singleinstancedatabase "$STANDBY_DB" -n "$NS"
```

Do not delete shared [admin](./prerequisites/PREREQUISITES.md#database-admin-secret), [TDE](./prerequisites/PREREQUISITES.md#tde-secret), or [TLS Secrets](./prerequisites/PREREQUISITES.md#tcps-tls-secret) that remain in use.

## True Cache

#### Overview

[Oracle True Cache](https://www.oracle.com/database/truecache/)
is a mostly diskless, read-only, in-memory SQL and JSON cache for Oracle
Database. It offloads read activity from the primary, keeps cached data
consistent through automatic redo apply (similar to Active Data Guard), and can
route read-only application queries to the cache. Architecture detail:
[True Cache documentation](https://docs.oracle.com/en/database/oracle/oracle-database/23/odbtc/overview-oracle-true-cache.html).

This OraOperator guide deploys True Cache as two SIDB resources, in order:

1. A **primary** SIDB that publishes a True Cache configuration blob as a
   ConfigMap (`<primary>-truecache-blob`).
2. A **True Cache** SIDB (`createAs: truecache`) that mounts that ConfigMap and
   connects to the primary.

#### True Cache Contents

- [Deployment Scenarios](#deployment-scenarios-1)
- [Helpers and Operations](#helpers-and-operations-1)
- [Before You Begin for True Cache](#before-you-begin-for-true-cache)
- [True Cache Quick Start](#true-cache-quick-start)
- [True Cache Setup with TDE and TCPS](#true-cache-setup-with-tde-and-tcps)
- [Create True Cache for a Remote Primary](#create-true-cache-for-a-remote-primary)
- [Enable blob generation on an existing primary](#enable-blob-generation-on-an-existing-primary)

#### Deployment Scenarios

> **Before proceeding:** Review [Before You Begin for True Cache](#before-you-begin-for-true-cache) and understand its requirements. Each item below is a deployment path; complete that scenario’s prerequisites before applying its manifests.

| When you need to... | Open |
| --- | --- |
| Create a same-cluster primary and True Cache over TCP | [True Cache Quick Start](#true-cache-quick-start) |
| Create a same-cluster primary and True Cache with TDE and TCPS | [True Cache Setup with TDE and TCPS](#true-cache-setup-with-tde-and-tcps) |
| Create a True Cache for a cross-cluster or remote-host primary | [Create True Cache for a Remote Primary](#create-true-cache-for-a-remote-primary) |

#### Helpers and Operations

| When you need to... | Open |
| --- | --- |
| Prepare for any True Cache deployment | [Before You Begin for True Cache](#before-you-begin-for-true-cache) |
| Create TCPS certificates for a cross-cluster or remote-host primary | [TCPS certificate manager](./tcps-cert-manager/README.md) or [separate cluster access](./tcps-cert-manager/SEPARATE_CLUSTER_ACCESS.md) |
| Enable blob generation on an existing primary | [Enable blob generation on an existing primary](#enable-blob-generation-on-an-existing-primary) |
| Configure advanced service association behavior | [Advanced True Cache association](./truecache/TRUECACHE.md#advanced-true-cache-association) |
| Set the optional primary CDB name | [Set `primarySource.dbName`](./truecache/TRUECACHE.md#set-primarysourcedbname) |

#### Before You Begin for True Cache

Before proceeding, review every point in **[Before You Begin](#before-you-begin)** to understand the overall prerequisites, including storage and credential requirements. Select one deployment scenario above and complete the mandatory scenario-specific prerequisites in its workflow before applying the related manifests.

#### True Cache Quick Start

Use this **same-cluster** TCP Quick Start when the primary and True Cache use
the same Kubernetes cluster and namespace. It creates the sample primary and its
True Cache, connects them over TCP, and verifies that the True Cache is ready
for use.

**Required Before Deployment**

> **Important:** Complete every action in the **Complete before applying** column before applying a related manifest. These requirements are mandatory for this deployment scenario.

| Requirement | Example value | Complete before applying |
| --- | --- | --- |
| Shared True Cache requirements | Before You Begin for True Cache | **[Review the shared requirements](#before-you-begin-for-true-cache)** |
| Namespace and operator access | `default` | **[Confirm namespace access](../../README.md#install-from-yaml-manifests)** |
| Persistent storage | `oci-bv` / `60Gi` for each SIDB | **[Prepare persistent storage](./prerequisites/PREREQUISITES.md#set-up-kubernetes-and-volumes-for-database-persistence)** |
| Database admin and TDE Secrets | `db-admin-secret` / `tde-wallet-secret` | **[Create the admin Secret](./prerequisites/PREREQUISITES.md#database-admin-secret)** and **[TDE Secret](./prerequisites/PREREQUISITES.md#tde-secret)** |
| Image-pull Secret | `ocr-reg-cred` | **[Create image-pull Secret](./prerequisites/PREREQUISITES.md#image-pull-secret)** |
| True Cache blob generation | Primary blob ConfigMap and archive | **[Configure blob generation](./prerequisites/PREREQUISITES.md#true-cache)** |

**Review Manifest Requirements**

> The sample YAMLs already contain these values. If you change any example value, update every dependent reference (YAML fields, `$PRIMARY` / `$TRUECACHE`, blob ConfigMap name, and Secrets) before applying.

Primary SIDB — [`singleinstancedatabase_truecache_primary.yaml`](../../config/samples/sidb/singleinstancedatabase_truecache_primary.yaml)

| Setting | Example value | Required relationship |
| --- | --- | --- |
| `metadata.name` | `sidb-sample` | Matches `$PRIMARY` |
| `metadata.namespace` | `default` | Matches `$NS` and the Secrets namespace |
| `sid` | `ORCLPRD` | Unique primary SID |
| `pdbName` | `APPPDB1` | PDB created on the primary |
| `createAs` | `primary` | Primary role for this workflow |
| Admin Secret | `db-admin-secret` / `oracle_pwd` | Same admin Secret as True Cache |
| TDE Secret | `tde-wallet-secret` / `tde_wallet_pwd` | Same TDE Secret as True Cache |
| `trueCache.generateBlob` | `true` | Publishes the True Cache blob archive |
| `trueCache.createConfigMap` | `true` | Creates ConfigMap `sidb-sample-truecache-blob` |
| `trueCache.generatePath` | `/tmp/tc_config_blob.tar.gz` | Blob archive path inside the primary Pod |

True Cache SIDB — [`singleinstancedatabase_truecache.yaml`](../../config/samples/sidb/singleinstancedatabase_truecache.yaml)

| Setting | Example value | Required relationship |
| --- | --- | --- |
| `metadata.name` | `truecache` | Matches `$TRUECACHE` |
| `metadata.namespace` | `default` | Same namespace as the primary |
| `sid` | `ORCLTC` | Unique True Cache SID |
| `createAs` | `truecache` | True Cache role for this workflow |
| `primarySource.databaseRef` | `sidb-sample` | Must equal the primary `metadata.name` |
| `trueCache.blobConfigMapRef` | `sidb-sample-truecache-blob` | ConfigMap created by the Healthy primary |
| `trueCache.blobConfigMapKey` | `tc_config_blob.tar.gz` | Key inside the blob ConfigMap |
| `trueCache.blobMountPath` | `/stage/tc_config_blob.tar.gz` | Mount path inside the True Cache Pod |
| Admin Secret | `db-admin-secret` / `oracle_pwd` | Same admin Secret as the primary |
| TDE Secret | `tde-wallet-secret` / `tde_wallet_pwd` | Same TDE Secret as the primary |

**1. Configure the primary manifest**
([`singleinstancedatabase_truecache_primary.yaml`](../../config/samples/sidb/singleinstancedatabase_truecache_primary.yaml)).

Review the **Primary SIDB** table above and update only the fields required for your environment.


**2. Apply the primary**

```sh
export NS=default          # must match metadata.namespace
export PRIMARY=sidb-sample # must match metadata.name
kubectl apply -f config/samples/sidb/singleinstancedatabase_truecache_primary.yaml
kubectl wait --for=jsonpath='{.status.status}'=Healthy \
  "singleinstancedatabase/$PRIMARY" -n "$NS" --timeout=60m
kubectl wait --for=jsonpath='{.binaryData.tc_config_blob\.tar\.gz}' \
  "configmap/$PRIMARY-truecache-blob" -n "$NS" --timeout=20m
```

`kubectl wait` reports only completion or a timeout. To see progress while it
runs, open a second terminal in the same Kubernetes context and run:

```sh
kubectl get singleinstancedatabase "$PRIMARY" -n "$NS" -w
```

Press Ctrl-C in that second terminal when you are done watching; it does not
stop the `kubectl wait` command in the first terminal.

**3. Configure the True Cache manifest**
([`singleinstancedatabase_truecache.yaml`](../../config/samples/sidb/singleinstancedatabase_truecache.yaml)).

Review the **True Cache SIDB** table above. Use the same namespace as the primary and keep the primary name, blob ConfigMap reference, and service mapping aligned.


**4. Apply True Cache**

```sh
export TRUECACHE=truecache # must match metadata.name
kubectl apply -f config/samples/sidb/singleinstancedatabase_truecache.yaml
kubectl wait --for=jsonpath='{.status.status}'=Healthy \
  "singleinstancedatabase/$TRUECACHE" -n "$NS" --timeout=60m
```

While this waits, use a second terminal to watch the True Cache status:

```sh
kubectl get singleinstancedatabase "$TRUECACHE" -n "$NS" -w
```

**5. Verify:**

```sh
kubectl logs -n "$NS" -l "app=$TRUECACHE" --tail=200
kubectl get singleinstancedatabase "$TRUECACHE" -n "$NS" -o wide
```

Expect role `TRUE_CACHE`, status `Healthy`, and `DATABASE IS READY TO USE` in
the logs. Confirm `TRUE_CACHE_SERVICE` on the primary
([Final verification](./truecache/TRUECACHE.md#final-verification-every-path)).

#### True Cache Setup with TDE and TCPS

Use this **same-cluster** TCPS Quick Start when the primary and True Cache use
the same Kubernetes cluster and namespace. It creates the sample primary and
True Cache with TCPS enabled, then verifies that the True Cache is ready for
use.

**Required Before Deployment**

> **Important:** Complete every action in the **Complete before applying** column before applying a related manifest. These requirements are mandatory for this deployment scenario.

| Requirement | Example value | Complete before applying |
| --- | --- | --- |
| Shared True Cache requirements | Before You Begin for True Cache | **[Review the shared requirements](#before-you-begin-for-true-cache)** |
| Namespace and operator access | `default` | **[Confirm namespace access](../../README.md#install-from-yaml-manifests)** |
| Persistent storage | `oci-bv` / `60Gi` for each SIDB | **[Prepare persistent storage](./prerequisites/PREREQUISITES.md#set-up-kubernetes-and-volumes-for-database-persistence)** |
| Database admin and TDE Secrets | `db-admin-secret` / `tde-wallet-secret` | **[Create the admin Secret](./prerequisites/PREREQUISITES.md#database-admin-secret)** and **[TDE Secret](./prerequisites/PREREQUISITES.md#tde-secret)** |
| TCPS certificates and TLS Secrets | `sidb-primary-tcps-tls` or separate Secrets with the required SANs | **[Create certificates and TLS Secrets](./tcps-cert-manager/README.md)** |
| Image-pull Secret | `ocr-reg-cred` | **[Create image-pull Secret](./prerequisites/PREREQUISITES.md#image-pull-secret)** |
| True Cache blob generation | Primary blob ConfigMap and archive | **[Configure blob generation](./prerequisites/PREREQUISITES.md#true-cache)** |

**Review Manifest Requirements**

> The sample YAMLs already contain these values. If you change any example value, update every dependent reference (YAML fields, `$PRIMARY` / `$TRUECACHE`, blob ConfigMap name, certificate SANs, and Secrets) before applying.

Primary SIDB — [`singleinstancedatabase_truecache_primary_tcps.yaml`](../../config/samples/sidb/singleinstancedatabase_truecache_primary_tcps.yaml)

| Setting | Example value | Required relationship |
| --- | --- | --- |
| `metadata.name` | `sidb-sample` | Matches `$PRIMARY` |
| `metadata.namespace` | `default` | Matches `$NS` and the Secrets namespace |
| `sid` | `ORCLPRD` | Unique primary SID |
| `createAs` | `primary` | Primary role for this workflow |
| Admin Secret | `db-admin-secret` / `oracle_pwd` | Same admin Secret as True Cache |
| TDE Secret | `tde-wallet-secret` / `tde_wallet_pwd` | Same TDE Secret as True Cache |
| `security.tcps.enabled` | `true` | Publishes the primary TCPS listener |
| `security.tcps.tlsSecret` | `sidb-primary-tcps-tls` | TLS Secret SANs must cover primary client hostnames |
| `trueCache.generateBlob` / `createConfigMap` | `true` / `true` | Creates ConfigMap `sidb-sample-truecache-blob` |
| `trueCache.generatePath` | `/tmp/tc_config_blob.tar.gz` | Blob archive path inside the primary Pod |

True Cache SIDB — [`singleinstancedatabase_truecache_same_cluster_tcps.yaml`](../../config/samples/sidb/singleinstancedatabase_truecache_same_cluster_tcps.yaml)

| Setting | Example value | Required relationship |
| --- | --- | --- |
| `metadata.name` | `truecache` | Matches `$TRUECACHE` |
| `metadata.namespace` | `default` | Same namespace as the primary |
| `sid` | `ORCLTC` | Unique True Cache SID |
| `createAs` | `truecache` | True Cache role for this workflow |
| `primarySource.databaseRef` | `sidb-sample` | Must equal the primary `metadata.name` |
| `trueCache.blobConfigMapRef` | `sidb-sample-truecache-blob` | ConfigMap created by the Healthy primary |
| `trueCache.blobConfigMapKey` | `tc_config_blob.tar.gz` | Key inside the blob ConfigMap |
| `trueCache.blobMountPath` | `/stage/tc_config_blob.tar.gz` | Mount path inside the True Cache Pod |
| Admin Secret | `db-admin-secret` / `oracle_pwd` | Same admin Secret as the primary |
| TDE Secret | `tde-wallet-secret` / `tde_wallet_pwd` | Same TDE Secret as the primary |
| `security.tcps.enabled` | `true` | Publishes the True Cache TCPS listener |
| `security.tcps.tlsSecret` | `sidb-primary-tcps-tls` | Share only when SANs cover both names; otherwise use a dedicated Secret such as `sidb-truecache-tcps-tls` |

**1. Configure the primary manifest**
([`singleinstancedatabase_truecache_primary_tcps.yaml`](../../config/samples/sidb/singleinstancedatabase_truecache_primary_tcps.yaml)).

Review the **Primary SIDB** table above and update only the fields required for your environment.


**2. Apply the primary**

```sh
export NS=default
export PRIMARY=sidb-sample
kubectl apply -f config/samples/sidb/singleinstancedatabase_truecache_primary_tcps.yaml
kubectl wait --for=jsonpath='{.status.status}'=Healthy \
  "singleinstancedatabase/$PRIMARY" -n "$NS" --timeout=60m
kubectl wait --for=jsonpath='{.binaryData.tc_config_blob\.tar\.gz}' \
  "configmap/$PRIMARY-truecache-blob" -n "$NS" --timeout=20m
```

`kubectl wait` reports only completion or a timeout. To see progress, open a
second terminal in the same Kubernetes context and watch the primary status:

```sh
kubectl get singleinstancedatabase "$PRIMARY" -n "$NS" -w
```

Press Ctrl-C in that second terminal when you are done watching; it does not
stop the `kubectl wait` command in the first terminal.

**3. Configure the True Cache manifest**
([`singleinstancedatabase_truecache_same_cluster_tcps.yaml`](../../config/samples/sidb/singleinstancedatabase_truecache_same_cluster_tcps.yaml)).

Review the **True Cache SIDB** table above. Use the same namespace as the primary and keep the primary name, blob ConfigMap reference, and TCPS TLS Secret selection aligned.


**4. Apply True Cache**

```sh
export TRUECACHE=truecache
kubectl apply -f config/samples/sidb/singleinstancedatabase_truecache_same_cluster_tcps.yaml
kubectl wait --for=jsonpath='{.status.status}'=Healthy \
  "singleinstancedatabase/$TRUECACHE" -n "$NS" --timeout=60m
```

While this waits, use a second terminal to watch the True Cache status:

```sh
kubectl get singleinstancedatabase "$TRUECACHE" -n "$NS" -w
```

**5. Verify:**

```sh
kubectl logs -n "$NS" -l "app=$TRUECACHE" --tail=200
kubectl get singleinstancedatabase -n "$NS" -o wide
```

Expect `sidb-sample` / `PRIMARY` and `truecache` / `TRUE_CACHE`, both
`Healthy`, plus `DATABASE IS READY TO USE` in the True Cache logs. Confirm
`TRUE_CACHE_SERVICE` on the primary and TCPS on port `2484`
([Final verification](./truecache/TRUECACHE.md#final-verification-every-path)).

<a id="create-true-cache-for-a-remote-primary"></a>
<a id="create-true-cache-for-a-cross-cluster-or-external-primary"></a>
<a id="create-true-cache-for-an-external-primary"></a>

#### Create True Cache for a Remote Primary

Use this path for a **cross-cluster** SIDB primary or a **remote-host** primary
outside Kubernetes. This path uses `primarySource.connectString` rather than
`databaseRef`.

Prefer stable ExternalDNS or private DNS hostnames for LoadBalancer endpoints
when the primary and True Cache span clusters or regions. True Cache bootstrap
still uses the primary TCP listener in `primarySource.connectString`; set
`primarySource.tcpsPort` when the primary publishes TCPS.

Choose the procedure that matches your primary:

- **Cross-cluster SIDB primary:** Follow steps 1–3 below.
- **Remote-host single-instance or RAC primary:** Follow
  [`TRUECACHE.md` §4](./truecache/TRUECACHE.md#4-remote-host-si-or-rac). Do not
  use steps 1–3 below.

**Required Before Deployment**

> **Important:** Complete every action in the **Complete before applying** column before applying a related manifest. These requirements are mandatory for this deployment scenario.

| Requirement | Example value | Complete before applying |
| --- | --- | --- |
| Shared True Cache requirements | Before You Begin for True Cache in each SIDB cluster and namespace | **[Review the shared requirements](#before-you-begin-for-true-cache)** |
| Namespace and operator access | `default` in the primary and True Cache clusters | **[Confirm namespace access](../../README.md#install-from-yaml-manifests)** in each cluster |
| Persistent storage | `oci-bv` / `60Gi` for every operator-managed SIDB | **[Prepare persistent storage](./prerequisites/PREREQUISITES.md#set-up-kubernetes-and-volumes-for-database-persistence)** |
| Database admin and TDE Secrets | `db-admin-secret` / `tde-wallet-secret` in each SIDB namespace | **[Create the admin Secret](./prerequisites/PREREQUISITES.md#database-admin-secret)** and **[TDE Secret](./prerequisites/PREREQUISITES.md#tde-secret)** |
| TCPS certificates and TLS Secrets | TLS Secret in every namespace that presents a certificate | **[Create certificates and TLS Secrets](./tcps-cert-manager/README.md)** when using TCPS |
| Image-pull Secret | `ocr-reg-cred` in every namespace that pulls from a private registry | **[Create image-pull Secret](./prerequisites/PREREQUISITES.md#image-pull-secret)** |
| Remote primary reachability | TCP listener and, when used, TCPS listener reachable from the True Cache cluster | **[Provide a reachable primary connection](./prerequisites/PREREQUISITES.md#primary-source)** |
| True Cache blob | Primary-generated blob ConfigMap and archive | **[Configure blob generation](./prerequisites/PREREQUISITES.md#true-cache)** and copy the blob ConfigMap in step 2 |

**Review Manifest Requirements**

> The sample YAMLs already contain these values. If you change any example value, update every dependent reference (connect string, ports, blob ConfigMap name, ExternalDNS hostnames, and Secrets) before applying.

Peered primary SIDB — [`singleinstancedatabase_truecache_primary_tcp_peered.yaml`](../../config/samples/sidb/singleinstancedatabase_truecache_primary_tcp_peered.yaml) or [`singleinstancedatabase_truecache_primary_tcps_peered.yaml`](../../config/samples/sidb/singleinstancedatabase_truecache_primary_tcps_peered.yaml)

| Setting | Example value | Required relationship |
| --- | --- | --- |
| `metadata.name` | `sidb-sample` | Matches `$PRIMARY` |
| `metadata.namespace` | `default` | Primary-cluster namespace for Secrets and the blob ConfigMap |
| `sid` | `ORCLPRD` | Trailing service name in the True Cache `connectString` |
| `createAs` | `primary` | Primary role on the primary cluster |
| LoadBalancer / ExternalDNS hostname | `sidb-sample.internal.example.com` | Hostname True Cache uses in `primarySource.connectString` |
| TCP listener port | `1521` | Reachable from the True Cache cluster for bootstrap |
| TCPS listener port (TCPS sample) | `2484` | Set as `primarySource.tcpsPort` on True Cache when TCPS is used |
| `trueCache.generateBlob` / `createConfigMap` | `true` / `true` | Creates ConfigMap `sidb-sample-truecache-blob` to copy |
| Admin / TDE Secrets | `db-admin-secret` / `tde-wallet-secret` | Present in the primary namespace |

True Cache SIDB — [`singleinstancedatabase_truecache_external.yaml`](../../config/samples/sidb/singleinstancedatabase_truecache_external.yaml)

| Setting | Example value | Required relationship |
| --- | --- | --- |
| `metadata.name` | `truecache` | Matches `$TRUECACHE` |
| `metadata.namespace` | `default` | True Cache cluster namespace |
| `sid` | `ORCLTC` | Unique True Cache SID |
| `createAs` | `truecache` | True Cache role for this workflow |
| `primarySource.connectString` | `sidb-sample.internal.example.com:1521/ORCLPRD` | Primary TCP listener reachable from the True Cache Pod |
| `primarySource.tcpsPort` (when TCPS) | `2484` | Reachable primary TCPS port; keep `connectString` on TCP |
| `trueCache.blobConfigMapRef` | `sidb-sample-truecache-blob` | Blob ConfigMap copied into the True Cache cluster |
| `trueCache.blobConfigMapKey` | `tc_config_blob.tar.gz` | Key inside the copied blob ConfigMap |
| `trueCache.blobMountPath` | `/stage/tc_config_blob.tar.gz` | Mount path inside the True Cache Pod |
| Admin / TDE Secrets | `db-admin-secret` / `tde-wallet-secret` | Present in the True Cache namespace |

**1. Configure the peered primary manifest** (on the primary cluster).

TCP primary: [`singleinstancedatabase_truecache_primary_tcp_peered.yaml`](../../config/samples/sidb/singleinstancedatabase_truecache_primary_tcp_peered.yaml)

TCPS primary: [`singleinstancedatabase_truecache_primary_tcps_peered.yaml`](../../config/samples/sidb/singleinstancedatabase_truecache_primary_tcps_peered.yaml)

Review the **Peered primary SIDB** table above and update only the fields required for your environment.


**2. Apply the peered primary and copy its blob ConfigMap into the True Cache cluster.**

```sh
export PRIMARY_KUBECONFIG=/path/to/primary.kubeconfig
export TRUECACHE_KUBECONFIG=/path/to/truecache.kubeconfig
export NS=default
export PRIMARY=sidb-sample

export KUBECONFIG="$PRIMARY_KUBECONFIG"
kubectl apply -f config/samples/sidb/singleinstancedatabase_truecache_primary_tcp_peered.yaml
# TCPS: use singleinstancedatabase_truecache_primary_tcps_peered.yaml instead
kubectl wait --for=jsonpath='{.status.status}'=Healthy \
  "singleinstancedatabase/$PRIMARY" -n "$NS" --timeout=60m
kubectl wait --for=jsonpath='{.binaryData.tc_config_blob\.tar\.gz}' \
  "configmap/$PRIMARY-truecache-blob" -n "$NS" --timeout=20m

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

`kubectl wait` reports only completion or a timeout. To see progress, open a
second terminal and use the primary cluster context to watch its status:

```sh
KUBECONFIG="$PRIMARY_KUBECONFIG" \
  kubectl get singleinstancedatabase "$PRIMARY" -n "$NS" -w
```

Press Ctrl-C in that second terminal when you are done watching; it does not
stop the `kubectl wait` command in the first terminal.

**3. Configure the True Cache manifest** (on the True Cache cluster)
([`singleinstancedatabase_truecache_external.yaml`](../../config/samples/sidb/singleinstancedatabase_truecache_external.yaml)).

Review the **True Cache SIDB** table above. Update the primary endpoint and port values for the selected Service type, then keep the copied blob ConfigMap reference aligned with the primary.


Choose the primary Service type first, then set `connectString` to match.
`ClusterIP` is only for same-cluster Quick Start / TCPS paths — it is **not**
reachable from another cluster.

| Primary `services.endpoints` type | When to use | `connectString` HOST | TCP PORT in `connectString` | `primarySource.tcpsPort` |
| --- | --- | --- | --- | --- |
| `LoadBalancer` | Cross-cluster with a stable hostname or LB DNS (peered samples default) | LB hostname / private DNS name | Service port `1521` | `2484` when primary publishes TCPS |
| `NodePort` | Cross-cluster without a LoadBalancer (labs / no NLB) | Node IP or node hostname | TCP NodePort (for example `31100`) | TCPS NodePort (for example `32526`) |

Examples:

```yaml
# LoadBalancer + TCP only
primarySource:
  connectString: "sidb-sample.internal.example.com:1521/ORCLPRD"

# LoadBalancer + TCPS (TCP bootstrap, then TCPS peer port)
primarySource:
  connectString: "sidb-sample.internal.example.com:1521/ORCLPRD"
  tcpsPort: 2484

# NodePort + TCPS (HOST is a node address; ports are NodePorts)
primarySource:
  connectString: "10.0.2.182:31100/ORCLPRD"
  tcpsPort: 32526
```

Before you apply True Cache, verify all of the following:

- The `connectString` host resolves from the True Cache pod. Use cluster DNS,
  private DNS / ExternalDNS, or `hostAliases` for name resolution. VCN peering
  (or equivalent routing) must also allow the pod to reach that host and port.
- Keep `connectString` on the primary's **TCP** listener, even for a TCPS
  deployment. Set `primarySource.tcpsPort` separately only when the primary
  publishes TCPS.
- Omit optional `primarySource.dbName` unless your selected scenario requires
  it; when set, use only the short primary CDB `DB_NAME`. See
  [Set `primarySource.dbName`](./truecache/TRUECACHE.md#set-primarysourcedbname).

For the complete cross-cluster peering procedure, see
[`TRUECACHE.md` — Remote primary TCP or TCPS](./truecache/TRUECACHE.md#3-remote-primary-tcp-or-tcps).

**4. Apply True Cache**

Check reachability, then apply:

```sh
export KUBECONFIG="$TRUECACHE_KUBECONFIG"
export TRUECACHE=truecache
kubectl run truecache-netcheck --rm -i --restart=Never \
  --image=busybox:1.36 -- \
  sh -c 'nc -vz sidb-sample.internal.example.com 1521'
kubectl apply -f config/samples/sidb/singleinstancedatabase_truecache_external.yaml
kubectl wait --for=jsonpath='{.status.status}'=Healthy \
  "singleinstancedatabase/$TRUECACHE" -n "$NS" --timeout=60m
```

While this waits, open a second terminal and use the True Cache cluster
context to watch the resource:

```sh
KUBECONFIG="$TRUECACHE_KUBECONFIG" \
  kubectl get singleinstancedatabase "$TRUECACHE" -n "$NS" -w
```

**5. Verify:**

```sh
kubectl logs -n "$NS" -l "app=$TRUECACHE" --tail=200
kubectl get singleinstancedatabase "$TRUECACHE" -n "$NS" -o wide
```

Expect role `TRUE_CACHE`, status `Healthy`, and `DATABASE IS READY TO USE` in
the logs. Confirm `TRUE_CACHE_SERVICE` on the primary
([Final verification](./truecache/TRUECACHE.md#final-verification-every-path)).

#### Enable blob generation on an existing primary

Use this task only when the primary SIDB is already `Healthy` **and does not
yet generate a True Cache blob**. It enables blob generation on that existing
primary without recreating it. Do not run it when you apply a supplied new
primary sample: the [True Cache Quick Start](#true-cache-quick-start) primary
YAML already enables blob generation.

Confirm the primary already has enterprise edition, `archiveLog: true`, and the
TDE secret. Then enable blob generation with a merge patch. The operator
reconciles those `trueCache` fields and creates ConfigMap
`$PRIMARY-truecache-blob` with binary key `tc_config_blob.tar.gz`:

```sh
kubectl patch singleinstancedatabase "$PRIMARY" -n "$NS" --type merge -p \
  '{"spec":{"trueCache":{"generateBlob":true,"createConfigMap":true,"generatePath":"/tmp/tc_config_blob.tar.gz"}}}'
```

The Healthy wait returns immediately on an already Healthy primary. Keep
waiting on the ConfigMap — that is what proves blob generation finished:

```sh
kubectl wait --for=jsonpath='{.status.status}'=Healthy \
  "singleinstancedatabase/$PRIMARY" -n "$NS" --timeout=60m
kubectl wait --for=jsonpath='{.binaryData.tc_config_blob\.tar\.gz}' \
  "configmap/$PRIMARY-truecache-blob" -n "$NS" --timeout=20m
```

`kubectl wait` reports only completion or a timeout. To see progress, open a
second terminal in the same Kubernetes context and watch the existing primary:

```sh
kubectl get singleinstancedatabase "$PRIMARY" -n "$NS" -w
```

Press Ctrl-C in that second terminal when you are done watching; it does not
stop the `kubectl wait` command in the first terminal.

Continue at **Apply True Cache** in the deployment procedure selected from the
[Deployment Scenarios](#deployment-scenarios-1) table above.

## ORDS and APEX

#### Overview

[Oracle REST Data Services (ORDS)](https://docs.oracle.com/en/database/oracle/oracle-rest-data-services/)
is a Java-based middleware and web gateway that exposes Oracle Database objects,
tables, PL/SQL, Database API, REST-enabled schemas, and Database Actions through
secure HTTP/REST interfaces.

This SIDB page keeps the basic SIDB-plus-ORDS workflow focused. APEX
installation or upgrade and other OrdsSrvs capabilities are not duplicated
here; see the OrdsSrvs documentation for their current support and procedures.

This OraOperator guide deploys ORDS with the `OrdsSrvs` controller against a
Healthy SIDB. Use `OrdsSrvs` for new work.

> **Deprecated as of version 2.3:** The `OracleRestDataService` controller is
> retained only for existing deployments and migration. See
> [Legacy OracleRestDataService Controller](#legacy-oraclerestdataservice-controller).

#### ORDS and APEX Contents

- [Deployment Scenarios](#deployment-scenarios-2)
- [Helpers and Operations](#helpers-and-operations-2)
- [Before You Begin for ORDS and APEX](#before-you-begin-for-ords-and-apex)
- [ORDS Quick Start](#ords-quick-start)
- [Legacy OracleRestDataService Controller](#legacy-oraclerestdataservice-controller)
- [Delete ORDS](#delete-ords)

#### Deployment Scenarios

> **Before proceeding:** Review [Before You Begin for ORDS and APEX](#before-you-begin-for-ords-and-apex), then follow the SIDB-plus-OrdsSrvs Quick Start.

| When you need to... | Open |
| --- | --- |
| Create ORDS against a same-cluster SIDB | [ORDS Quick Start](#ords-quick-start) |

#### Helpers and Operations

| When you need to... | Open |
| --- | --- |
| Read the OrdsSrvs controller documentation | [`docs/ordsservices/README.md`](../ordsservices/README.md) |
| Migrate or operate a legacy `OracleRestDataService` resource | [Legacy OracleRestDataService Controller](#legacy-oraclerestdataservice-controller) |
| Delete OrdsSrvs | [Delete ORDS](#delete-ords) |

#### Before You Begin for ORDS and APEX

Use the same-cluster Quick Start below for an operator-managed SIDB and
`OrdsSrvs`. Install OraOperator with the OrdsSrvs controller enabled, and make
sure it watches both namespaces used by the example. The Quick Start uses
`default` for the SIDB and `ordsnamespace` for OrdsSrvs:

```sh
export SIDB_NS=default
export ORDS_NS=ordsnamespace
```

If you choose different namespaces, update every `metadata.namespace`, Secret
namespace, RoleBinding, and `WATCH_NAMESPACE` value consistently.

1. [OrdsSrvs prerequisites](../ordsservices/README.md#prerequisites)
2. [Namespace-scoped deployment](../ordsservices/README.md#namespace-scoped-deployment),
   including the [RoleBinding](../ordsservices/examples/ordsnamespace-role-binding.yaml)
3. `ordssrvs-auth` Secrets with `adminAuth` and `dbAuth` in the namespaces
   where they are referenced
4. A Healthy SIDB and its published PDB connection string

#### ORDS Quick Start

Use this **same-cluster** Quick Start when OrdsSrvs connects to a
**containerized SIDB** managed by OraOperator in the same Kubernetes cluster.
It creates the sample SIDB and OrdsSrvs, connects them over TCP, and verifies
ORDS access. Create the SIDB first and wait until it is `Healthy` before
applying `OrdsSrvs`.

The detailed companion walkthrough is
[`docs/ordsservices/examples/sidb_container.md`](../ordsservices/examples/sidb_container.md).
It provides the complete OrdsSrvs resource reference. If you already have a
Healthy SIDB, skip the SIDB creation step and use its verified PDB connection
string.

**1. Set the resource names and namespaces**

This example keeps the SIDB in `default` and runs ORDS in a separate
`ordsnamespace`. Both namespaces must be watched by the operator.

```sh
export SIDB_NS=default
export ORDS_NS=ordsnamespace
export SIDB=oraoper-sidb
export ORDS=ords-sidb

kubectl create namespace "$ORDS_NS"
```

If either namespace is outside the operator namespace, apply the
[RoleBinding](../ordsservices/examples/ordsnamespace-role-binding.yaml) and
add both namespaces to `WATCH_NAMESPACE` before continuing. The `default`
namespace is valid only if the operator is configured to watch it.

**2. Create the SIDB and ORDS credentials Secrets**

Replace the placeholders with real passwords before running the command:

- `oracle_pwd` is the SIDB administrator password used by the Free database
  container.
- `dbAuth` is the password for the ORDS runtime database user
  (`ORDS_PUBLIC_USER` in the manifest).
- `adminAuth` is the database administrator password (`SYS` in the ORDS
  manifest). It must be the same password as `freedb-admin-secret.oracle_pwd`.

```sh
kubectl create secret generic freedb-admin-secret \
  --from-literal=oracle_pwd='<database-admin-credential>' \
  --namespace "$SIDB_NS" \
  --dry-run=client -o yaml | kubectl apply -f -

kubectl create secret generic ordssrvs-auth \
  --from-literal=dbAuth='<ords-db-credential>' \
  --from-literal=adminAuth='<database-admin-credential>' \
  --namespace "$ORDS_NS" \
  --dry-run=client -o yaml | kubectl apply -f -
```

Kubernetes Secrets are namespace-scoped. The SIDB and OrdsSrvs use different
Secrets: the SIDB reads `freedb-admin-secret.oracle_pwd` from `$SIDB_NS`, while
OrdsSrvs reads `ordssrvs-auth.dbAuth` and `ordssrvs-auth.adminAuth` from
`$ORDS_NS`. Do not omit the SIDB Secret or substitute `ordssrvs-auth`; the
SIDB pod will remain in `Init` with a `secret not found` mount error.

The administrator password in both Secrets must match the password used when
the SIDB database was initialized. Updating a Kubernetes Secret does not
change an existing database password; if the database was initialized with a
different password, use that password or recreate the test SIDB before
starting ORDS.

**3. Configure the SIDB manifest**

Start from the [Oracle Database Free SIDB sample
YAML](../../config/samples/sidb/singleinstancedatabase_free.yaml). Set
`metadata.name` to `$SIDB`, `metadata.namespace` to `$SIDB_NS`, and the admin
Secret reference to `freedb-admin-secret` / `oracle_pwd`. Update the image and
persistence values for your environment, then save the file as
`oraoper-sidb.yaml`.

**4. Apply the SIDB**

```sh
kubectl apply -f oraoper-sidb.yaml
kubectl wait --for=jsonpath='{.status.status}'=Healthy \
  "singleinstancedatabase/$SIDB" \
  -n "$SIDB_NS" \
  --timeout=60m
```

`kubectl wait` reports only completion or a timeout. To see progress, open
a second terminal in the same Kubernetes context:

```sh
kubectl get singleinstancedatabase "$SIDB" -n "$SIDB_NS" -w
```

Press Ctrl-C in that second terminal when you are done watching; it does
not stop the `kubectl wait` command in the first terminal.

**5. Read the SIDB PDB connect string, then configure OrdsSrvs**

A `Healthy` SIDB does not by itself guarantee that the PDB connection string
has been published. Wait for that status field before reading it:

```sh
kubectl wait --for=jsonpath='{.status.pdbConnectString}' \
  "singleinstancedatabase/$SIDB" \
  -n "$SIDB_NS" \
  --timeout=10m

CONN_STRING=$(kubectl get singleinstancedatabase "$SIDB" \
  -n "$SIDB_NS" \
  -o jsonpath='{.status.pdbConnectString}')
```

Use the checked-in [`ords-sidb.yaml`](./ords-sidb.yaml) manifest and replace
its placeholder PDB connection string if needed. The same manifest is shown
inline below for reference.

The YAML file does not expand shell variables. Replace
`jdbc:oracle:thin:@//<pdb-connect-string>` with the actual value of
`jdbc:oracle:thin:@//$CONN_STRING`, and replace `ordsnamespace` if
`$ORDS_NS` has another value. Keep `autoUpgradeORDS: true` when ORDS must be
installed or upgraded in the database. Omit `autoUpgradeAPEX` or leave it
false for this focused SIDB-plus-ORDS procedure. If APEX installation or
upgrade is required, use the [OrdsSrvs APEX auto-upgrade
procedure](../ordsservices/autoupgrade.md), which documents the required
`apex.zip` copy while the init container is waiting.

```yaml
apiVersion: database.oracle.com/v4
kind: OrdsSrvs
metadata:
  name: ords-sidb
  namespace: ordsnamespace
spec:
  image: container-registry.oracle.com/database/ords:latest
  forceRestart: true
  globalSettings:
    database.api.enabled: true
  poolSettings:
    - poolName: default
      autoUpgradeORDS: true
      restEnabledSql.active: true
      plsql.gateway.mode: direct
      db.connectionType: customurl
      db.customURL: jdbc:oracle:thin:@//<pdb-connect-string>
      db.username: ORDS_PUBLIC_USER
      db.secret:
        secretName: ordssrvs-auth
        passwordKey: dbAuth
      db.adminUser: SYS
      db.adminUser.secret:
        secretName: ordssrvs-auth
        passwordKey: adminAuth
  replicas: 1
```

**6. Apply OrdsSrvs**

```sh
kubectl apply -f ords-sidb.yaml
kubectl wait --for=jsonpath='{.status.status}'=Healthy \
  "ordssrvs/$ORDS" -n "$ORDS_NS" --timeout=30m
```

While this waits, use a second terminal to watch the ORDS resource:

```sh
kubectl get ordssrvs "$ORDS" -n "$ORDS_NS" -w
```

**7. Verify**

Port-forward the OrdsSrvs Service and open the ORDS URL. Use the hostname
`localhost` rather than a raw IP such as `127.0.0.1`; the default HTTPS
listener can reject a mismatched SNI with `HTTP 400 Invalid SNI`:

```sh
kubectl port-forward "service/$ORDS" -n "$ORDS_NS" 8443:8443
```

Open `https://localhost:8443/ords`. If `standalone.https.host` is configured,
use that hostname instead of `localhost`.

#### Legacy OracleRestDataService Controller

> **Deprecated as of version 2.3:** `OracleRestDataService` is retained for existing deployments and migration only. Do not use it for new work. Migrate to `OrdsSrvs`.

Legacy samples kept for compatibility:

- [`config/samples/sidb/oraclerestdataservice.yaml`](../../config/samples/sidb/oraclerestdataservice.yaml)
- [`config/samples/sidb/oraclerestdataservice_create.yaml`](../../config/samples/sidb/oraclerestdataservice_create.yaml)
- [`config/samples/sidb/oraclerestdataservice_secrets.yaml`](../../config/samples/sidb/oraclerestdataservice_secrets.yaml)

If you still operate a legacy resource, delete it before deleting the referenced SIDB:

```sh
kubectl delete oraclerestdataservice ords-sample
```

#### Delete ORDS

Delete OrdsSrvs before deleting the referenced SIDB when the SIDB exists only to serve that ORDS deployment:

```sh
kubectl delete ordssrvs ords-sidb -n ordsnamespace
```

## Networking

Configure SIDB listener exposure, TCPS, and optional hostname overrides. Keep
Data Guard and True Cache topology settings in their respective
[Data Guard](#data-guard) and [True Cache](#true-cache) sections.

- [Service Endpoints](#service-endpoints)
- [Enable TCPS](#enable-tcps)
- [Use a TCPS Client Wallet](#use-a-tcps-client-wallet)
- [TNS Aliases](#tns-aliases)
- [Host Aliases](#host-aliases)

### Service Endpoints

Every SIDB has an in-cluster `ClusterIP` Service named after the SIDB. By
default, it exposes TCP on `1521`; enabling TCPS also adds `2484`. Use
`spec.services.endpoints` to add or customize client-facing Service endpoints.

**Important:** Configure at most one endpoint of each valid name/type pair:
`cluster` / `ClusterIP`, `nodeport` / `NodePort`, and `loadbalancer` /
`LoadBalancer`. You can omit `name` only when `type` identifies the pair.

| Access scope | `name` / `type` | Managed Service | Configure |
| --- | --- | --- | --- |
| In-cluster | `cluster` / `ClusterIP` | `<sidb-name>` | TCP Service port is always `1521`; `nodePort` is not supported. |
| External node address | `nodeport` / `NodePort` | `<sidb-name>-nodeport` | Use `tcp.nodePort` and `tcps.nodePort`; each pinned value must be in `30000`-`32767`. |
| External load balancer | `loadbalancer` / `LoadBalancer` | `<sidb-name>-lb` | Use `tcp.port` and `tcps.port`; use provider annotations when required. |

NodePort and LoadBalancer Services also expose the XDB port `5500`. Endpoint
annotations and `externalTrafficPolicy` apply only to NodePort and
LoadBalancer endpoints; `externalTrafficPolicy` accepts `Cluster` or `Local`.

Use `isKeep: true` when the Service must survive SIDB deletion and recreation.
Changing an endpoint Service type or ports recreates that Service, so plan for
client reconnects and a possible external-address change.

```yaml
# NodePort: optionally pin the allocated ports.
services:
  endpoints:
    - name: nodeport
      type: NodePort
      tcp:
        enabled: true
        nodePort: 31521
```

```yaml
# LoadBalancer: configure Service ports and provider-specific annotations.
services:
  endpoints:
    - name: loadbalancer
      type: LoadBalancer
      annotations:
        oci-network-load-balancer.oraclecloud.com/internal: "true"
      tcp:
        enabled: true
        port: 1521
```

**Deprecated:** Do not use `spec.services.external`, `spec.loadBalancer`,
`spec.listenerPort`, `spec.tcpsListenerPort`, or `spec.serviceAnnotations` in
new manifests. Use `spec.services.endpoints` instead.

For every available endpoint field, see
[`config/samples/sidb/singleinstancedatabase.yaml`](../../config/samples/sidb/singleinstancedatabase.yaml).

### Enable TCPS

Use TCPS when clients, Data Guard, or True Cache peers must connect over TLS.
Create the [TCPS TLS secret](./prerequisites/PREREQUISITES.md#tcps-tls-secret)
before creating the SIDB. Its certificate SANs must cover every client and
remote-peer hostname; use IP SANs when clients use IP addresses.

1. Enable `spec.security.tcps` and reference the TLS Secret.
2. For in-cluster clients, the default ClusterIP Service exposes TCPS on `2484`.
   For external clients, add a NodePort or LoadBalancer endpoint with TCPS
   enabled.
3. Keep TCP on `1521` when bootstrap or mixed clients still require it.
4. Apply the SIDB with TCPS already configured. Enabling TCPS by patching an
   existing `SingleInstanceDatabase` is not supported.

```yaml
security:
  tcps:
    enabled: true
    tlsSecret: sidb-tcps-tls
services:
  endpoints:
    - name: loadbalancer
      type: LoadBalancer
      tcp:
        enabled: true
        port: 1521
      tcps:
        enabled: true
        port: 2484
```

Review [Before You Begin](#before-you-begin) and use the
[`tcps-cert-manager` procedure](./tcps-cert-manager/README.md) when
cert-manager manages the certificate. Start from
[`config/samples/sidb/singleinstancedatabase_tcps.yaml`](../../config/samples/sidb/singleinstancedatabase_tcps.yaml)
for a primary-only TCPS example.

### Use a TCPS Client Wallet

After a TCPS-enabled SIDB is ready, identify the wallet Secret from status:

```sh
NS=default
SIDB=sidb-sample

kubectl get singleinstancedatabase "$SIDB" -n "$NS" \
  -o jsonpath='tcps={.status.isTcpsEnabled} walletDir={.status.clientWalletLoc} walletSecret={.status.clientWalletSecret}{"\n"}'
```

Confirm that `status.isTcpsEnabled` is `true`. When
`spec.security.tcps.clientWalletSecret` is omitted, the operator publishes a
generated client-wallet Secret for Data Guard and records its name in
`status.clientWalletSecret`. When you set that field, the Broker uses the
supplied Secret and the operator does not publish a generated one.
`status.clientWalletLoc` is valid only inside the SIDB Pod. This is a TCPS
client wallet, not a TDE wallet.

For inspecting the Secret and creating a portable `db_wallet.zip`, see
[TCPS Client Wallet](./tcps-cert-manager/TCPS_CLIENT_WALLET.md). For Data
Guard, the Broker runner must have the referenced client-wallet Secret in its
own namespace; see [TCPS Client Wallet Placement for the Broker
Runner](#tcps-client-wallet-placement-for-the-broker-runner).

### TNS Aliases

Use `spec.tnsAliases` to have the operator manage named entries in the SIDB
Pod's `tnsnames.ora`. Each alias supplies the Oracle Net host, listener port,
protocol, and service name used by Oracle client tools.

Set `tnsAliases` as a direct list under `spec`; do not nest it under another
`tnsAliases` key:

```yaml
spec:
  tnsAliases:
    - name: TEST_ORCLP
      host: sidb-addlpvc-test.shns.svc.cluster.local
      port: 1521
      protocol: TCP
      serviceName: ORCLP
    - name: TEST_ORCLP_PDB
      host: sidb-addlpvc-test.shns.svc.cluster.local
      port: 1521
      protocol: TCP
      serviceName: ORCLPDB
```

The operator writes and reconciles these aliases in the effective Oracle Net
administration directory in the SIDB Pod. Alias names are case-insensitively
unique; use `TCP` or `TCPS`, and specify the listener port and service name
that the target database exposes. For TCPS, use port `2484` when applicable
and set `sslServerDN` when the client must verify the server certificate DN.

This setting does not create a Kubernetes Service or distribute
`tnsnames.ora` to other application Pods. An application outside the SIDB Pod
must use the same alias definition in its own `TNS_ADMIN` directory or client
wallet before it connects with `TEST_ORCLP` or `TEST_ORCLP_PDB`.

### Host Aliases

Use `spec.hostAliases` only when a required hostname cannot resolve through
cluster or private DNS. It adds static entries to the database Pod; it does not
create a Service or replace DNS management.

```yaml
hostAliases:
  - ip: 10.0.0.10
    hostnames:
      - primary.example.com
```

Keep the IP address current, and ensure the hostname is covered by the TLS
certificate SAN when the connection uses TCPS.

## Storage

Use persistent storage for database files, recovery files, and diagnostics. For production SIDBs, Oracle recommends separate PVCs for `oradata` and the Fast Recovery Area (FRA), and a persistent diagnostics PVC mounted at `/opt/oracle/diag`. This keeps datafiles, recovery files, and diagnostic output independently sized and retained across Pod replacement or container restart, subject to the PVC and storage retention policies.

- [Recommended Storage Layout](#recommended-storage-layout)
- [Dynamic Persistence](#dynamic-persistence)
- [Existing PVCs](#existing-pvcs)
- [Additional PVC Volumes](#additional-pvc-volumes)
- [Storage Expansion](#storage-expansion)
- [Write Permissions and Scripts Volume](#write-permissions-and-scripts-volume)

### Recommended Storage Layout

| Purpose | SIDB field | Recommended mount path | Use |
| --- | --- | --- | --- |
| Database datafiles | `persistence.oradata` | `/opt/oracle/oradata` | Required database datafiles volume. |
| Fast Recovery Area | `persistence.fra` | `/opt/oracle/fra` | Separate volume for archived redo logs, backups, and other recovery files. |
| Diagnostics | `persistence.additionalPVCs` | `/opt/oracle/diag` | Persistent alert logs, trace files, and incident diagnostics. |
| Database audit files | `persistence.additionalPVCs` | `/opt/oracle/admin/<SID>/adump` | Persistent audit output; replace `<SID>` with the value of `spec.sid`. |
| DBCA, datapatch, and operational logs | `persistence.additionalPVCs` | `/opt/oracle/cfgtoollogs` | Persistent operational logs after the image paths are configured to use this location. |
| Application or backup files | `persistence.additionalPVCs` | A dedicated absolute path | Optional supplemental storage that does not replace `oradata` or FRA. |

The following example dynamically provisions separate `oradata` and FRA PVCs,
then mounts pre-created diagnostics, audit, operational-log, and application
PVCs. The sample SID is `ORCL1`; replace it when `spec.sid` is different. All
referenced PVCs must be in the same namespace as the SIDB.

```yaml
spec:
  persistence:
    oradata:
      size: 50Gi
      storageClass: oci-bv
      accessMode: ReadWriteOnce

    fra:
      size: 100Gi
      storageClass: oci-bv
      accessMode: ReadWriteOnce
      mountPath: /opt/oracle/fra
      recoveryAreaSize: 80Gi

    additionalPVCs:
      - pvcName: sidb-diag-existing
        mountPath: /opt/oracle/diag
      - pvcName: sidb-audit-existing
        mountPath: /opt/oracle/admin/ORCL1/adump
      - pvcName: sidb-cfgtoollogs-existing
        mountPath: /opt/oracle/cfgtoollogs
      - pvcName: sidb-addlpvc-existing-a
        mountPath: /opt/oracle/additional-data-a
      - pvcName: sidb-addlpvc-existing-b
        mountPath: /opt/oracle/additional-data-b

    retainOnDelete: true
```

Do not use an additional PVC as the main datafiles or FRA volume. Use
`persistence.oradata` and `persistence.fra` for those paths so the controller
can configure and reconcile the database storage correctly. Avoid overlapping
mount paths.

The image cannot be completely read-only today. Database creation and DBCA,
Oracle Home relinking, image updates and OPatch inventory operations, and
primary-database datapatch can write outside the database data volumes. Use
additional PVCs for diagnostics, audit output, DBCA logs, and datapatch logs to
keep persistent operational data outside the container layer. A future image
refactoring should also store `/etc/oratab` in a supported persistent
configuration location, move DBCA temporary files to a dedicated writable
temporary volume, and redirect `audit_file_dest` to the persistent audit path.
These changes reduce the writable image surface, but the Oracle Home still
requires a supported writable path until relinking, patching, and datapatch
are redesigned.

### Dynamic Persistence

For an operator-managed `oradata` or FRA PVC, omit `pvcName` and set the storage request:

- `persistence.oradata.size`, `storageClass`, and `accessMode`
- `persistence.fra.size`, `storageClass`, `accessMode`, optional `mountPath`, and `recoveryAreaSize`

Set `fra.mountPath: /opt/oracle/fra` for the recommended separate layout. When `fra.mountPath` is omitted, the controller mounts the FRA PVC at `/opt/oracle/oradata/fast_recovery_area`. Set `fra.recoveryAreaSize` explicitly to control the database recovery-area limit; it is translated to `db_recovery_file_dest_size`.

Set `persistence.retainOnDelete: true` when operator-created PVCs must remain after the SIDB is deleted.

### Static Persistence

Use `pvcName` for a PVC created outside the operator, including volumes materialized by a backup or restore workflow. The operator mounts a referenced PVC but does not create, adopt, resize, delete, or add an owner reference to it.

```yaml
spec:
  persistence:
    oradata:
      pvcName: sidb-existing-oradata
    fra:
      pvcName: sidb-existing-fra
      mountPath: /opt/oracle/fra
      recoveryAreaSize: 80Gi
```

The claim must already exist in the SIDB namespace with the required capacity, access mode, storage class, and binding state. For `oradata`, do not set `size` or `storageClass` when `pvcName` is set. For FRA, do not set `size`, `storageClass`, or `accessMode` with `pvcName`, and always set `recoveryAreaSize`.

For backup and restore workflows, create or materialize the datafiles PVC before creating the SIDB, then set `persistence.oradata.pvcName` to that claim name. Do not select the operator-managed path with `size` and `storageClass` for a same-name pre-created PVC; the controller does not adopt it and reports a `Managed PVC conflict`.

`spec.persistence.datafilesVolumeName` is deprecated. Use `spec.persistence.oradata.pvcName` for a pre-created datafiles claim.

### Additional PVC Volumes

Use `persistence.additionalPVCs` for diagnostics and supplemental storage. Each entry requires a unique absolute `mountPath` and uses one of the following modes:

- **Existing PVC:** set `pvcName`. This is the preferred mode for a diagnostics PVC at `/opt/oracle/diag` when the storage lifecycle is managed outside SIDB.
- **Operator-created PVC:** omit `pvcName` and set both `storageSizeInGb` and `storageClass`. The controller creates a `ReadWriteOnce` PVC named `<sidb-name>-additional-<index>`.

```yaml
spec:
  persistence:
    additionalPVCs:
      - pvcName: sidb-diag-existing
        mountPath: /opt/oracle/diag
      - pvcName: sidb-addlpvc-existing-a
        mountPath: /opt/oracle/additional-data-a
      - pvcName: sidb-addlpvc-existing-b
        mountPath: /opt/oracle/additional-data-b
      - mountPath: /opt/oracle/exports
        storageSizeInGb: 50
        storageClass: oci-bv
```

Do not combine `pvcName` with `storageSizeInGb` or `storageClass` in one additional-PVC entry. Changing additional PVC mounts causes the controller to recreate the database Pod so the desired mounts are applied.

### Storage Expansion

For operator-managed PVCs, increase `persistence.oradata.size` or `persistence.fra.size` only when the StorageClass supports volume expansion. Shrinking is not supported. Resize pre-created PVCs and additional PVCs through the storage administrator workflow; do not treat an `additionalPVCs.storageSizeInGb` change as a resize operation.

### Write Permissions and Scripts Volume

- `persistence.setWritePermissions` — enable when the storage backend requires an init container to grant the `oracle` user write access. It applies to configured `oradata`, FRA, and additional PVC mount paths.
- `spec.scripts.setup.pvcName` and `spec.scripts.startup.pvcName` — optional PVC references that supply controller-supported `setup` and `startup` scripts.

Script execution steps are under [Execute Custom Scripts](#execute-custom-scripts).

## Lifecycle Management

Day-2 sizing, scripts, field-change caution, and pod-level maintenance. Create, clone, patch, and delete procedures stay in [SIDB Deployment and Lifecycle](#sidb-deployment-and-lifecycle); standby / True Cache / ORDS stay in their workflow sections. Role changes use `spec.createAs` in those workflows (there is no separate “mode switch” procedure here).

- [Database Pod Resources and Init Parameters](#database-pod-resources-and-init-parameters)
- [Pod Placement](#pod-placement)
- [Execute Custom Scripts](#execute-custom-scripts)
- [Immutable or Sensitive Areas](#immutable-or-sensitive-areas)
- [Maintenance Operations](#maintenance-operations)
- [Update an Existing SIDB](#update-an-existing-sidb)

### Database Pod Resources and Init Parameters

Use `spec.resources` for Kubernetes CPU/memory requests and limits, and `spec.initParams` for supported database initialization settings such as:

- `cpuCount`
- `processes`
- `sgaTarget`
- `pgaAggregateTarget`
- `memoryTarget`

`memoryTarget` configures Oracle Automatic Memory Management. The controller sets `MEMORY_MAX_TARGET`, clears explicit SGA/PGA targets, restarts the database, and then applies the requested `MEMORY_TARGET`. It accepts quantities such as `20Gi` or `20GB`. Size the pod memory limit substantially above the target to leave room for Oracle processes and OS overhead. Do not combine `memoryTarget` with `sgaTarget` or `pgaAggregateTarget`.

The [Oracle Database 26ai memory management guidance](https://docs.oracle.com/en/database/oracle/oracle-database/26/admin/managing-memory.html#GUID-24312D78-522D-4C08-8934-820BE18D34C7)
recommends Automatic Memory Management when the total SGA and PGA is up to
4GB, and Automatic Shared Memory Management for larger databases. This is a
memory-management boundary, not an Oracle minimum-memory requirement. SIDB
uses a 4GiB regular container-memory floor when its memory validation path is
active.

**SGA/PGA memory sizing**

`spec.initParams.sgaTarget` and `spec.initParams.pgaAggregateTarget` are
integer values in MiB. For example, `14336` represents a 14GiB SGA.

For the SGA/PGA memory-validation path, SIDB uses:

- `M`: `spec.resources.limits.memory`.
- `S`: effective SGA size.
- `P`: effective PGA size.
- `H`: `spec.resources.limits.hugepages-2Mi`, falling back to
  `spec.resources.requests.hugepages-2Mi` when the limit is omitted.

When both `spec.initParams.sgaTarget` and `spec.initParams.pgaAggregateTarget` are specified, SIDB interprets their values in MiB and uses them as the effective SGA and PGA sizes for memory calculation i.e.

- `S = sgaTarget × 1MiB`
- `P = pgaAggregateTarget × 1MiB`

When neither HugePages nor `spec.initParams.memoryTarget` is configured,
both SGA/PGA parameters are omitted, and `spec.resources.limits.memory`
is specified, SIDB derives the effective SGA size as 50% of the regular
memory limit and the effective PGA size as 25% of the derived SGA size:

- `S = M / 2`
- `P = S / 4`

The following checks apply when validating an SGA/PGA configuration against
a regular memory limit:

| Configuration | Validation requirements |
| --- | --- |
| Without HugePages (H = 0) | `M >= S + P + 1GiB` and `M >= 4GiB` |
| With HugePages (H > 0)| `H >= S`, `M >= P + 1GiB`, and `M >= 4GiB` |

The `1GiB` allowance and `4GiB` floor are operator validation thresholds.
Size production deployments for the workload: PGA usage can exceed its
target, and Oracle processes require additional regular memory.

**HugePages configuration**

SIDB supports the Kubernetes resource `hugepages-2Mi`. Configure explicit
`sgaTarget` and `pgaAggregateTarget` values, and set equal HugePages
requests and limits. Do not combine HugePages with `memoryTarget`.

With HugePages, the SGA is covered by the HugePages allocation and is not
added to the regular container-memory requirement.

The following fragment configures a 14GiB SGA and a 4GiB PGA target, reserves
16GiB of HugePages, and requests 8GiB of regular memory:

```yaml
spec:
  initParams:
    sgaTarget: 14336
    pgaAggregateTarget: 4096
  resources:
    requests:
      cpu: "4"
      memory: 8Gi
      hugepages-2Mi: 16Gi
    limits:
      cpu: "4"
      memory: 8Gi
      hugepages-2Mi: 16Gi
```

The configuration satisfies the operator's memory-validation requirements:

| Check | Calculation | Result |
| --- | --- | --- |
| HugePages cover the SGA | `16GiB >= 14GiB` | Pass |
| Regular memory covers the PGA target plus allowance | `8GiB >= 4GiB + 1GiB` | Pass |
| Regular memory meets the operator minimum | `8GiB >= 4GiB` | Pass |

Without HugePages, the same SGA and PGA targets require a regular memory
limit of at least **19GiB** (`14GiB + 4GiB + 1GiB`).

Complete [Configure SIDB HugePages](./prerequisites/PREREQUISITES.md#configure-sidb-hugepages)
before deploying this configuration. That procedure covers worker
allocation, runtime memory-lock limits, and verification. For dedicated
worker placement, see [Pod Placement](#pod-placement).

**Shared-memory sysctls**

An explicit `kernel.shmmax` takes precedence and must be at least the
effective SGA size. With HugePages, SIDB uses the effective HugePages
allocation as `kernel.shmmax` when that sysctl is omitted.

In regular-memory mode, SIDB preserves user-provided sysctls and does not
automatically add `kernel.shmmax` or `kernel.shmall`, including when
SGA/PGA targets are explicitly configured.

See [Configure SIDB Pod Kernel Sysctls](./prerequisites/PREREQUISITES.md#configure-sidb-pod-kernel-sysctls)
for calculation rules and worker configuration.

AMM configuration example, without HugePages:

```yaml
spec:
  initParams:
    memoryTarget: 20Gi
  resources:
    limits:
      memory: 24Gi
```

After deployment, patching `spec.initParams.memoryTarget` and restarting the pod causes the controller to reconcile the new target. The database may restart during this operation; wait for the SIDB to return to `Healthy` before validating `memory_target`.

### Pod Placement

Use `spec.nodeSelector` to restrict the SIDB Pod to nodes with a matching label.
The selector does not create or label nodes, so apply the label before creating
the SIDB:

```bash
kubectl label node <node-name> workload=oracle
```

```yaml
spec:
  nodeSelector:
    workload: "oracle"
```

Use `spec.tolerations` when the selected nodes have a taint. A toleration allows
the SIDB Pod to run on a tainted node; it does not select a node by itself.
For predictable placement, use a matching `nodeSelector` and toleration:

```bash
kubectl label node <node-name> sidb-hugepages=true
kubectl taint node <node-name> sidb-hugepages=true:NoSchedule
```

```yaml
spec:
  nodeSelector:
    sidb-hugepages: "true"
  tolerations:
    - key: sidb-hugepages
      operator: Equal
      value: "true"
      effect: NoSchedule
```

`spec.tolerations` is optional. When it is omitted, the controller adds no
user-defined tolerations. Apply the taint only after the manifest includes the
matching toleration, or the SIDB Pod remains Pending. 

For HugePages workloads, complete
[Configure SIDB HugePages](./prerequisites/PREREQUISITES.md#configure-sidb-hugepages)
on every selected worker. This includes sufficient HugePages capacity,
Oracle group access, and container-runtime memory-lock limits.

To remove the example taint later, run:

```bash
kubectl taint node <node-name> sidb-hugepages=true:NoSchedule-
```

### Execute Custom Scripts

Mount a scripts PVC by setting `spec.scripts.setup.pvcName` and/or
`spec.scripts.startup.pvcName` (see [Write Permissions and Scripts Volume](#write-permissions-and-scripts-volume)), then place scripts under the controller-supported
`setup` and `startup` directories.

If you use custom scripts, apply the optional persistent-volume RBAC described in [Before You Begin](#before-you-begin).

### Immutable or Sensitive Areas

Treat these as carefully managed after the database is running:

- storage layout and bound volumes
- source database references for clone, standby, and True Cache (`spec.primarySource`)
- image family and edition combinations
- TCPS and TDE configuration that affects bootstrap paths

Prefer the create/clone/standby/True Cache workflows when changing role or source identity rather than ad hoc edits to those fields.

### Maintenance Operations

If manual inspection inside the database pod is required:

1. Enter the pod:

   ```sh
   kubectl exec -it <pod-name> -- /bin/bash
   ```

2. Inspect environment and Oracle paths:

   ```sh
   env
   ```

3. Connect as SYSDBA if needed:

   ```sh
   sqlplus "/ as sysdba"
   ```

For operator-managed changes (image patch, delete, storage size), use the matching workflow or [Storage Expansion](#storage-expansion) instead of manual pod edits alone.

### Update an Existing SIDB

For step-by-step configuration updates and verification, see
[Update an Existing SIDB](./lifecycle/CONFIGURATION_UPDATES.md).
The guide covers container resources, database initialization parameters,
storage retention and reuse, PVC expansion, and TNS aliases.

## Troubleshooting and Known Issues

Use this section for SIDB diagnostics, common operator errors, and current product limitations (including Data Guard Broker gaps that also affect True Cache and standby workflows where applicable).

### Collect Diagnostics

```sh
kubectl get events -n $NS --sort-by=.lastTimestamp
```

```sh
kubectl describe singleinstancedatabase $DB -n $NS
```

```sh
kubectl get pods -n $NS
```

```sh
kubectl describe pod <pod-name> -n $NS
```

```sh
kubectl logs -f <pod-name> -n $NS
```

```sh
kubectl logs -n <operator-namespace> deployment/oracle-database-operator-controller-manager -c manager
```

### Common SIDB Errors

#### Operator does not watch this namespace

If applying the SIDB manifest fails with an error similar to:

```text
metadata.namespace: Invalid value: "default": operator does not watch this namespace
```

Then the SIDB resource was created in a namespace not watched by the operator.

Do not use `default` unless the operator is configured to watch `default`.

Update `metadata.namespace` in the SIDB YAML to the operator-watched namespace.

Example:

```yaml
metadata:
  name: sidb-sample
  namespace: default
```

Then apply again:

```sh
kubectl apply -f sidb.yaml
```

#### Referenced secret not found

If the SIDB enters `Error` state and reports a missing secret, for example:

```text
Secret "ocr-reg-cred" not found
```

Then the SIDB YAML references a secret that does not exist in the target namespace.

Check existing secrets:

```sh
kubectl get secret -n $NS
```

Then either create the missing secret or update the SIDB YAML to use an existing secret.

Example:

```yaml
security:
  secrets:
    admin:
      secretName: db-admin-secret
      secretKey: oracle_pwd

image:
  pullSecrets: ocr-reg-cred
```

Verify:

```sh
kubectl get secret db-admin-secret -n $NS
kubectl get secret ocr-reg-cred -n $NS
```

#### Pod is Running but Not Ready

A database pod can be in `Running` state while the database is still starting.

Check the pod:

```sh
kubectl describe pod <pod-name> -n $NS
```

Check the database logs:

```sh
kubectl logs -f <pod-name> -n $NS
```

A successful startup may show log output similar to:

```text
DATABASE IS READY TO USE!
```

If the pod remains `Running` but `Ready: False`, check the readiness probe message in `kubectl describe pod`.

#### ORA-00800: Set Priority Failed

Oracle background processes may report a failure to obtain their requested
scheduling priority. For example, the database alert log may contain:

```text
ORA-00800: soft external error, arguments: [Set Priority Failed], [VKTM], [Check traces and OS configuration], [Check Oracle document and MOS notes], []
```

This error can be observed when `spec.allowPrivilegeEscalation` was omitted and the generated database
container uses `allowPrivilegeEscalation: false`. This restriction can
prevent image-provided helper executables from acquiring the privileges
needed to adjust process priority.

For affected deployments, explicitly enable privilege escalation in the
SIDB manifest:

```yaml
spec:
  allowPrivilegeEscalation: true
```

For an existing SIDB, update the source manifest and apply it, or patch the
resource:

```sh
NS=<namespace used for SIDB deployment>
SIDB=sidb-sample

kubectl patch singleinstancedatabase "$SIDB" -n "$NS" \
  --type=merge \
  -p '{"spec":{"allowPrivilegeEscalation":true}}'
```

The updated setting requires a newly created database Pod; restarting the
database inside the existing container is insufficient. Schedule the change
during a maintenance window and verify that the operator replaces the Pod.
If it does not, use the supported Pod replacement procedure.

Verify the effective setting on the replacement Pod:

```sh
POD=<replacement-pod-name>

kubectl get pod "$POD" -n "$NS" \
  -o jsonpath='{range .spec.containers[*]}{.name}{" allowPrivilegeEscalation="}{.securityContext.allowPrivilegeEscalation}{"\n"}{end}'
```

The database container must report `allowPrivilegeEscalation=true`.
After startup, inspect new alert-log entries and the associated process
trace to confirm that the priority-setting error no longer occurs.
Historical errors remain in the persisted alert log.

This setting allows privilege escalation within the container; it does
not itself grant `SYS_NICE` or enable privileged-container mode. The
cluster's admission policy, including the selected OpenShift SCC, must
permit it. Kubernetes Restricted Pod Security requires
`allowPrivilegeEscalation: false`.

#### Listener fails after deleting and recreating an SIDB

If a recreated SIDB listener reports `TNS-12545` or logs `Error listening on` with a hostname such as `sidb1-1yvn7` or `sidb2-tusy9`, the persisted `listener.ora` contains the old generated pod name. Pod names are ephemeral, while `listener.ora` is retained on the SIDB PVC.

The SIDB startup path now rewrites stale listener `HOST` entries to the local bind address `0.0.0.0` on every startup. Stable SIDB service names are used in `tnsnames.ora` and Data Guard endpoints, where they are advertised to clients. The controller also invokes the idempotent repair during ready-database reconciliation, for both primary and standby databases, and reloads or restarts a running listener when a repair is needed. PVC deletion is not required.

Verify the repair with:

```sh
kubectl exec <sidb-pod> -- grep -n "HOST=" /opt/oracle/oradata/dbconfig/<SID>/listener.ora
kubectl exec <sidb-pod> -- lsnrctl status
```
#### Transient Reconciliation Errors During Data Guard Switchover

During switchover, SIDB or DataguardBroker resources may temporarily report `Unhealthy` or `Error` due to Kubernetes exec failures such as `unable to upgrade connection: error dialing backend: EOF`, even when DGMGRL reports `SUCCESS`.

Verify the actual database roles and Broker status using `SHOW CONFIGURATION;`. If Broker reports `SUCCESS` but the Kubernetes resources remain unhealthy, a planned switchback using DGMGRL may help restore a healthy state. Confirm both SIDBs and DataguardBroker return to `Healthy`. If the issue persists, collect diagnostics before attempting further operations.

### Known Issues

1. Data synchronization between the primary and standby databases does not occur until the Data Guard Broker is configured.

   After the primary and standby databases reach the `Healthy` state, changes made to the primary database are not propagated to the standby database until a Data Guard Broker configuration is created and reaches the `Healthy` state.

2. Standby and True Cache bootstrap still use the primary TCP listener for first contact.

   Even when the end state is TCPS, `primarySource.connectString` must reach the primary on TCP (commonly `:1521` or a TCP NodePort/NLB port). Set `primarySource.tcpsPort` for post-Ready TCPS aliases and Broker/redo over TCPS. TCPS-only bootstrap is not available yet. See [Data Guard Setup with TDE and TCPS](#data-guard-setup-with-tde-and-tcps) and [Cross-Cluster Data Guard over TCPS with NLB](./dataguard/DATAGUARD_CROSS_CLUSTER_TCPS_NLB.md).

3. Cross-cluster TCPS Broker requires the primary client-wallet Secret in the Broker namespace.

   When DataguardBroker runs in a different cluster from the primary SIDB, copy the primary `status.clientWalletSecret` into the Broker namespace and reference it from the primary member `tcps.clientWalletSecret`. Missing wallets commonly surface as Broker `Error` / `dataguard broker configuration is missing after broker query returned no members`. See [TCPS Client Wallet Placement for the Broker Runner](#tcps-client-wallet-placement-for-the-broker-runner).

4. Separate-cluster TCPS certificate paths must reuse the primary intermediate CA.

   Do not bootstrap an independent CA on the standby cluster. Use dual-context helpers when one admin host reaches both clusters, or [SEPARATE_CLUSTER_ACCESS.md](./tcps-cert-manager/SEPARATE_CLUSTER_ACCESS.md) when bastions are isolated. Intermediate CA Secrets on peer clusters are not auto-synced after rotation.

5. Standby alert logs may show `FAL: Error fetching gap sequence, no FAL server specified` during bootstrap.

   During the short Duplicate / recovery transition, the standby alert log can briefly report:

   ```text
   FAL: Error fetching gap sequence, no FAL server specified
   ```

   When `dataguard.prereqs.enabled` is `true`, the operator configures `FAL_SERVER` from `primarySource.connectString` (or the primary reference used by that path). After the standby is Ready / Healthy, confirm `fal_server` is set and points at a reachable primary endpoint. A **brief** message during bootstrap is expected. A **persistent** FAL warning after Ready means the primary endpoint is missing, stale, or unreachable—fix `primarySource`, NLB/DNS reachability, then apply or re-render DataguardBroker. See [Cross-Cluster Data Guard over TCP with NLB](./dataguard/DATAGUARD_CROSS_CLUSTER_LB.md#a-fal-error-fetching-gap-sequence-no-fal-server-specified) and [Cross-Cluster Data Guard over TCPS with NLB](./dataguard/DATAGUARD_CROSS_CLUSTER_TCPS_NLB.md#known-issues-for-this-path).

## Additional Information

Detailed hands-on setup instructions are also available in LiveLab format:

- <https://oracle.github.io/cloudtestdrive/AppDev/database-operator/workshops/freetier/?lab=introduction>
