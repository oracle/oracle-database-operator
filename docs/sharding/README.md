# Deploy Oracle Globally Distributed Database on Kubernetes with Oracle AI Database Operator

Oracle Globally Distributed Database distributes segments of a data set across many databases (shards) on different computers, either on-premises or in the cloud. This feature enables globally distributed, linearly scalable, multimodel databases. It requires no specialized hardware or software. Oracle Globally Distributed Database does all this while providing strong consistency, full power of SQL, support for structured and unstructured data, and the Oracle Database ecosystem. It meets data sovereignty requirements and supports applications that require low latency and high availability.

All of the shards together make up a single logical database, which is referred to as an Oracle Globally Distributed Database (GDD).

Kubernetes provides infrastructure building blocks, such as compute, storage, and networks. Kubernetes makes the infrastructure available as code. It enables rapid provisioning of multi-node topologies. Additionally, Kubernetes also provides StatefulSets, which are the workload API objects that are used to manage stateful applications. This provides us lifecycle management elasticity for databases as a stateful application for various database topologies, such as Oracle Globally Distributed Database, Oracle Real Application Clusters (Oracle RAC), single-instance Oracle Database, and other Oracle features and configurations.

The Sharding Controller in Oracle AI Database Operator deploys and manages Oracle Globally Distributed Database topology as a StatefulSets in the Kubernetes clusters, using Oracle Database and Global Data Services container images. Oracle AI Database Operator Sharding Controller manages the typical lifecycle of Oracle Globally Distributed Database topology in the Kubernetes cluster, as shown below:

* Create primary StatefulSets shards
* Create master and standby Global Data Services StatefulSets
* Create persistent storage, along with StatefulSet
* Create services
* Create load balancer service
* Provision Oracle Globally Distributed Database topology by creating and configuring the following:
  * Catalog database
  * Shard databases
  * GSMs
  * Shard scale up and scale down
* Shard topology cleanup

Oracle AI Database Operator Sharding Controller provides end-to-end automation of Oracle Globally Distributed Database topology deployment in Kubernetes clusters.

For API migration guidance, see [ShardingDatabase v4 Migration Strategy and FAQ](./SHARDINGDATABASE_V4_MIGRATION_FAQ.md).

## Getting Started with Oracle AI Database Operator Sharding Controller

The following sections describe how to deploy Oracle Globally Distributed Database using the Oracle AI Database Operator Sharding Controller for different use cases. Choose the deployment scenario that matches your Oracle GDD topology. Complete the prerequisites before starting any deployment.

- [Deploy Oracle Globally Distributed Database on Kubernetes with Oracle AI Database Operator](#deploy-oracle-globally-distributed-database-on-kubernetes-with-oracle-ai-database-operator)
  - [Getting Started with Oracle AI Database Operator Sharding Controller](#getting-started-with-oracle-ai-database-operator-sharding-controller)
  - [Prerequisites for running Oracle AI Database Operator Sharding Controller](#prerequisites-for-running-oracle-ai-database-operator-sharding-controller)
    - [Kubernetes cluster](#kubernetes-cluster)
    - [Required Kubernetes Roles and Privileges](#required-kubernetes-roles-and-privileges)
    - [Deploy Oracle AI Database Operator](#deploy-oracle-ai-database-operator)
    - [Oracle Database and Global Data Services container images](#oracle-database-and-global-data-services-container-images)
    - [Create the Oracle GDD Namespace](#create-the-oracle-gdd-namespace)
    - [Create a Kubernetes secret for the database installation owner for the Oracle Globally Distributed Database topology deployment](#create-a-kubernetes-secret-for-the-database-installation-owner-for-the-oracle-globally-distributed-database-topology-deployment)
    - [Provision a Persistent Volume Containing an Oracle Database Gold Image](#provision-a-persistent-volume-containing-an-oracle-database-gold-image)
  - [Supported Sharding and Replication Combinations](#supported-sharding-and-replication-combinations)
  - [Oracle GDD Template YAML](#oracle-gdd-template-yaml)
  - [Deploy Oracle AI Database 26ai Free](#deploy-oracle-ai-database-26ai-free)
  - [Deployment Scenarios for System-Managed Sharding with Data Guard Replication](#deployment-scenarios-for-system-managed-sharding-with-data-guard-replication)
  - [Deployment Scenarios for System-Managed Sharding with Native (Raft) Replication](#deployment-scenarios-for-system-managed-sharding-with-native-raft-replication)
  - [Deployment Scenarios for User-Defined Sharding with Data Guard Replication](#deployment-scenarios-for-user-defined-sharding-with-data-guard-replication)
  - [Deployment Scenarios for Composite Sharding with Data Guard Replication](#deployment-scenarios-for-composite-sharding-with-data-guard-replication)
  - [Deployment Scenarios for Composite Sharding with Native (Raft) Replication](#deployment-scenarios-for-composite-sharding-with-native-raft-replication)
  - [Additional Oracle GDD Deployment Scenarios](#additional-oracle-gdd-deployment-scenarios)
  - [GDD Deployment on OpenShift](#gdd-deployment-on-openshift)
    - [Create and Bind the SCC](#create-and-bind-the-scc)
    - [Configure the StatefulSet Controller Finalizer Permission](#configure-the-statefulset-controller-finalizer-permission)
    - [Reference the ServiceAccount in the GDD Manifest](#reference-the-serviceaccount-in-the-gdd-manifest)
    - [Deploy and Verify](#deploy-and-verify)
  - [Connecting to Oracle Globally Distributed Database](#connecting-to-oracle-globally-distributed-database)
  - [Frequently Asked Questions](#frequently-asked-questions)
  - [Debugging and Troubleshooting](#debugging-and-troubleshooting)
  - [Known Issues](#known-issues)

**Note:** Complete each prerequisite that applies to your environment before continuing.

## Prerequisites for running Oracle AI Database Operator Sharding Controller

**IMPORTANT:** You must make the changes specified in this section before you proceed to the next section.

### Kubernetes cluster

To deploy Oracle AI Database Operator Sharding Controller, you need a Kubernetes cluster that uses one of the following environments:

* A cloud-based Kubernetes cluster, such as [OCI on Container Engine for Kubernetes (OKE)](https://www.oracle.com/cloud-native/container-engine-kubernetes/) or  
* An on-premises Kubernetes Cluster, such as [Oracle Linux Cloud Native Environment (OLCNE)](https://docs.oracle.com/en/operating-systems/olcne/) cluster.

To use Oracle AI Database Operator Sharding Controller, ensure that your system is provisioned with a supported Kubernetes release. Refer to the [Oracle AI Database Operator releases](https://github.com/oracle/oracle-database-operator/releases).

### Required Kubernetes Roles and Privileges

  The Oracle AI Database Operator Sharding Controller requires access to the following Kubernetes resources:

  | Resource | Required verbs |
  | --- | --- |
  | Pods | `create`, `delete`, `get`, `list`, `patch`, `update`, `watch` |
  | Containers | `create`, `delete`, `get`, `list`, `patch`, `update`, `watch` |
  | PersistentVolumeClaims | `create`, `delete`, `get`, `list`, `patch`, `update`, `watch` |
  | Services | `create`, `delete`, `get`, `list`, `patch`, `update`, `watch` |
  | Secrets | `create`, `delete`, `get`, `list`, `patch`, `update`, `watch` |
  | Events | `create`, `patch` |  

### Deploy Oracle AI Database Operator

Follow [Install Oracle AI Database Operator](../../README.md#install-the-operator) to deploy the operator. If the operator is already installed, continue to the next section.

### Oracle Database and Global Data Services container images

Choose one of the following deployment options:

  **Use Oracle-Supplied container images:**

  Oracle AI Database Operator Sharding Controller uses Oracle Global Data Services and Oracle Database images to provision the sharding topology.

  You can download the pre-built Oracle Global Data Services and Oracle Database images from [Oracle Container Registry](https://container-registry.oracle.com). These images have been functionally tested for various Oracle GDD deployment scenarios on OKE and OLCNE. You can refer to [Oracle Container Registry Images for Oracle Globally Distributed Database Deployment](https://github.com/oracle/db-sharding/blob/master/container-based-sharding-deployment/README.md#oracle-container-registry-images-for-oracle-globally-distributed-database-deployment)
  
  You can either download the images and push them to your container image repository, or, if your Kubernetes cluster can reach Oracle Container Registry, you can download these images directly from Oracle Container Registry during the deployment.

  **Note:** You must accept the Oracle Container Registry license agreement before pulling the prebuilt images.

  OR

  **Build your own Oracle Database and Global Data Services container images:**
  
  You can build these images using instructions in Oracle's official GitHub repositories:

  * [Oracle Global Data Services Image](https://github.com/oracle/db-sharding/tree/master/container-based-sharding-deployment)
  * [Oracle Database Image](https://github.com/oracle/docker-images/tree/main/OracleDatabase/SingleInstance)

   After the images are ready, push them to your container image Repository, so that you can pull them while provisioning the Oracle Globally Distributed Database topology.

**Note**: The Oracle GDD topology examples in this document use Oracle GSM and Oracle Database images available on [Oracle Container Registry](https://container-registry.oracle.com).

**Note:** In case you want to use the `Oracle AI Database 26ai Free` Image for Database and GSM, refer to section [Deploy Oracle AI Database 26ai Free](#deploy-oracle-ai-database-26ai-free) for more details.

### Create the Oracle GDD Namespace

  Create a Kubernetes namespace named `shns`. All Oracle GDD topology resources will be provisioned in the `shns` namespace. For example:

  ```sh
  # Create the namespace
  kubectl create ns shns

  # Check the created namespace 
  kubectl get ns
  ```

### Create a Kubernetes secret for the database installation owner for the Oracle Globally Distributed Database topology deployment

Follow [Create Kubernetes Secret](./provisioning/create_kubernetes_secret_for_db_user.md) to create the `db-user-pass-pkutl` secret.

After completing these prerequisites, choose the deployment scenario that matches your environment.

### Provision a Persistent Volume Containing an Oracle Database Gold Image

This step is needed when you want to provision a Persistent Volume having an Oracle Database Gold Image for Database Cloning.

On an OCI OKE cluster, you can use this Persistent Volume during provisioning shard databases by cloning in the same Availability Domain or you can use a Full Backup of this Persistent Volume during provisioning shard databases by cloning in different Availability Domains.

See [Persistent Volume having database gold image](./provisioning/provisioning_persistent_volume_having_db_gold_image.md) for instructions.

**NOTE:** Provisioning the Oracle Globally Distributed Database using Cloning from Database Gold Image is `NOT` supported with Oracle AI Database 26ai Free. This step is therefore not required when deploying Oracle GDD with Oracle AI Database 26ai Free and GSM images.

## Supported Sharding and Replication Combinations

Oracle Globally Distributed Database supports the following sharding type and replication type combinations:

```text
Supported Deployment Combinations

├── System-Managed Sharding
│   ├── Data Guard Replication
│   └── Native (Raft) Replication
│
├── User-Defined Sharding
│   └── Data Guard Replication
│
└── Composite Sharding
    ├── Data Guard Replication
    └── Native (Raft) Replication
```

## Oracle GDD Template YAML

The complete Oracle GDD YAML template, including the available configuration options, is provided in the [Oracle GDD ShardingDatabase YAML template](./provisioning/oraclegdd.yaml).

## Deploy Oracle AI Database 26ai Free

For more information, see the [Oracle AI Database 26ai Free documentation](https://www.oracle.com/database/free/get-started/).

When using Oracle AI Database 26ai Free images for the database and GSM with the Oracle AI Database Operator Sharding Controller, note the following:

* Add the parameter `dbEdition: "free"` to the YAML manifest.
* See [Sample Oracle Globally Distributed Database deployment using Oracle AI Database 26ai Free and GSM images](./provisioning/free/sharding_provisioning_with_free_images.md) for an example.
* You can configure pod CPU and memory resources using the `cpu` and `memory` parameters. However, the `INIT_SGA_SIZE` and `INIT_PGA_SIZE` environment variables are not supported.
* Provisioning an Oracle GDD by cloning from a Database Gold Image is not supported.
* If `CATALOG_CHUNKS` is not specified, Oracle AI Database 26ai Free defaults to 12 chunks. This default is chosen to accommodate the 12 GB user data limit of Oracle AI Database 26ai Free.
* Oracle AI Database 26ai Free supports a maximum of three shards. For details, see the [Licensing Information](https://docs.oracle.com/en/database/oracle/oracle-database/26/dblic/Licensing-Information.html).

## Deployment Scenarios for System-Managed Sharding with Data Guard Replication

Deploy an Oracle Globally Distributed Database (GDD) topology with System-Managed Sharding and Data Guard replication on your cloud-based Kubernetes cluster.

The sample YAML manifests are based on an OCI Container Engine for Kubernetes (OKE) cluster. The following examples demonstrate different deployment scenarios for Oracle GDD with System-Managed Sharding and Data Guard replication:

| Scenario | Where to start |
| --- | --- |
| Minimal deployment | Deploy Oracle GDD with System-Managed Sharding and Data Guard replication using [minimal configuration](./provisioning/system_sharding/ssharding_provisioning_minimal_configuration.md) |
| Specify number of chunks | Deploy Oracle GDD with System-Managed Sharding and Data Guard replication [with number of chunks specified](./provisioning/system_sharding/ssharding_provisioning_with_chunks_specified.md) |
| Scale out | [Add shards](./provisioning/system_sharding/ssharding_scale_out_add_shards.md) to an existing Oracle GDD |
| Scale in | [Delete a shard](./provisioning/system_sharding/ssharding_scale_in_delete_an_existing_shard.md) from an existing Oracle GDD |

## Deployment Scenarios for System-Managed Sharding with Native (Raft) Replication

Deploy an Oracle Globally Distributed Database (GDD) topology with System-Managed Sharding and Native (Raft) replication on your cloud-based Kubernetes cluster.

**NOTE:** Native (Raft) replication is available starting with Oracle AI Database 26ai.

The sample YAML manifests are based on an OCI Container Engine for Kubernetes (OKE) cluster. The following examples demonstrate different deployment scenarios for Oracle GDD with System-Managed Sharding and Native (Raft) replication:

| Scenario | Where to start |
| --- | --- |
| Minimal deployment | Deploy Oracle GDD with System-Managed Sharding and Native (Raft) replication using [minimal configuration](./provisioning/system_sharding_native/ssharding_provisioning_minimal_configuration_native.md) |
| Specify number of chunks | Deploy Oracle GDD with System-Managed Sharding and Native (Raft) replication [with number of chunks specified](./provisioning/system_sharding_native/ssharding_provisioning_with_chunks_specified_native.md) |
| Scale out | [Add shards](./provisioning/system_sharding_native/ssharding_scale_out_add_shards_native.md) to an existing Oracle GDD |
| Scale in | [Delete a shard](./provisioning/system_sharding_native/ssharding_scale_in_delete_an_existing_shard_native.md) from an existing Oracle GDD |

## Deployment Scenarios for User-Defined Sharding with Data Guard Replication

Deploy an Oracle Globally Distributed Database (GDD) topology with User-Defined Sharding and Data Guard replication on your cloud-based Kubernetes cluster.

The sample YAML manifests are based on an OCI Container Engine for Kubernetes (OKE) cluster. The following examples demonstrate different deployment scenarios for Oracle GDD with User-Defined Sharding:

| Scenario | Where to start |
| --- | --- |
| Minimal deployment | Deploy Oracle GDD with User-Defined Sharding using [minimal configuration](./provisioning/user-defined-sharding/udsharding_provisioning_without_db_gold_image.md) |
| Scale out | [Add shards](./provisioning/user-defined-sharding/udsharding_scale_out_add_shards.md) to an existing Oracle GDD |
| Scale in | [Delete a shard](./provisioning/user-defined-sharding/udsharding_scale_in_delete_an_existing_shard.md) from an existing Oracle GDD |

## Deployment Scenarios for Composite Sharding with Data Guard Replication

Deploy an Oracle Globally Distributed Database (GDD) topology with Composite Sharding and Data Guard replication on your cloud-based Kubernetes cluster.

The sample YAML manifests are based on an OCI Container Engine for Kubernetes (OKE) cluster. The following examples demonstrate different deployment scenarios for Oracle GDD with Composite Sharding and Data Guard replication:

| Scenario | Where to start |
| --- | --- |
| Minimal deployment | Deploy Oracle GDD with Composite Sharding and Data Guard replication using [minimal configuration](./provisioning/composite_sharding/composite_provisioning_minimal_configuration.md) |
| Specify number of chunks | Deploy Oracle GDD with Composite Sharding and Data Guard replication [with number of chunks specified](./provisioning/composite_sharding/composite_provisioning_with_chunks_specified.md) |
| Scale out | [Add shards](./provisioning/composite_sharding/composite_scale_out_add_shards.md) to an existing Oracle GDD |
| Scale in | [Delete a shard](./provisioning/composite_sharding/composite_scale_in_delete_an_existing_shard.md) from an existing Oracle GDD |

## Deployment Scenarios for Composite Sharding with Native (Raft) Replication

Deploy an Oracle Globally Distributed Database (GDD) topology with Composite Sharding and Native (Raft) replication on your cloud-based Kubernetes cluster.

**NOTE:** Native (Raft) replication is available starting with Oracle AI Database 26ai.

The sample YAML manifests are based on an OCI Container Engine for Kubernetes (OKE) cluster. The following examples demonstrate different deployment scenarios for Oracle GDD with Composite Sharding and Native (Raft) replication:

| Scenario | Where to start |
| --- | --- |
| Minimal deployment | Deploy Oracle GDD with Composite Sharding and Native (Raft) replication using [minimal configuration](./provisioning/composite_sharding_native/composite_provisioning_minimal_configuration_native.md) |
| Specify number of chunks | Deploy Oracle GDD with Composite Sharding and Native (Raft) replication [with number of chunks specified](./provisioning/composite_sharding_native/composite_provisioning_with_chunks_specified_native.md) |
| Scale out | [Add shards](./provisioning/composite_sharding_native/composite_scale_out_add_shards_native.md) to an existing Oracle GDD |
| Scale in | [Delete a shard](./provisioning/composite_sharding_native/composite_scale_in_delete_an_existing_shard_native.md) from an existing Oracle GDD |

## Additional Oracle GDD Deployment Scenarios

These additional deployment scenarios can be used with any supported sharding configuration

The sample YAML manifests are based on an OCI Container Engine for Kubernetes (OKE) cluster. The following examples are for Oracle GDD with System-Managed Sharding, but you can also use them with other sharding cases.

| Scenario | Where to start |
| --- | --- |
| Specify CPU and memory | Deploy Oracle GDD using [custom CPU and memory settings](./provisioning/system_sharding/ssharding_provisioning_with_control_on_resources.md) |
| Shared mount point | Deploy Oracle GDD using a [shared mount point](./provisioning/system_sharding/ssharding_provisioning_shared_mount_point.md) |
| Node selection | Deploy Oracle GDD with [node selection](./provisioning/system_sharding/ssharding_provisioning_with_node_selection.md) for pod placement |

## GDD Deployment on OpenShift

Deploy Oracle Globally Distributed Database (GDD) on OpenShift by creating
an SCC, binding it to the database Pod ServiceAccount, and configuring the
StatefulSet controller permission described below.

Complete the standard operator, image, registry, storage, and database
Secret prerequisites first.

This example uses the tested System-Managed Sharding configuration:

| Setting | Value |
| --- | --- |
| Namespace | `shns` |
| ShardingDatabase resource | `sdb-system` |
| Database Pod ServiceAccount | `shns-sa` |
| SCC | `shns-oracle-user-scc` |
| Components | `catalog`, `gsm1`, `gsm2`, `pshard1`, and `pshard2` |

Run the commands from the GDD provisioning directory using an OpenShift
cluster administrator or an identity authorized to create the SCC and
grant the required RBAC permissions.

The examples assume that the namespace already exists:

```sh
NS=shns
GDD=sdb-system

oc get namespace "$NS"
```

If using another namespace or ShardingDatabase name, update the matching
values in the following manifests. Keep the StatefulSet controller
ServiceAccount namespace as `kube-system` for the controller identity
used in this example.

### Create and Bind the SCC

This example deployment uses a non-root SCC with UID `54321`, an `fsGroup`
of `54321`, and supplemental groups restricted to `54321`.

The SCC permits the capabilities requested by the database and GSM containers:

| Components | Requested capabilities |
| --- | --- |
| Catalog and shard databases | `NET_ADMIN`, `SYS_NICE` |
| GSM instances | `NET_RAW` |

The SCC drops default capabilities and permits the containers to
explicitly request the listed capabilities. It does not permit privileged
containers.

Apply the manifest: [openshift_gdd_rbac_scc.yaml](./provisioning/openshift/openshift_gdd_rbac_scc.yaml)

```sh
oc apply -f openshift_gdd_rbac_scc.yaml
```

Verify that the database Pod ServiceAccount can use the SCC:

```sh
oc auth can-i use securitycontextconstraints/shns-oracle-user-scc \
  --as="system:serviceaccount:${NS}:shns-sa" \
  -n "$NS"
```

Expected result: `yes`.

The RoleBinding authorizes Pods using `shns-sa` to use this SCC within
`shns`. It does not grant database access or general Kubernetes API
permissions.

### Configure the StatefulSet Controller Finalizer Permission

The operator configuration also requires a separate RBAC rule for
the StatefulSet controller.

The StatefulSet controller needs a separate RBAC rule to update
`shardingdatabases/finalizers`. Without this permission, it cannot create
PVCs that reference the `ShardingDatabase` with `blockOwnerDeletion: true`.

The permission must be granted to the identity creating the PVC (
  `system:serviceaccount:kube-system:statefulset-controller` in this case). Granting the
permission only to the database operator or `shns-sa` does not address
this admission failure.

Confirm the StatefulSet controller identity:

```sh
oc get clusterrolebinding system:controller:statefulset-controller \
  -o jsonpath='{range .subjects[*]}{.kind}{" "}{.namespace}{"/"}{.name}{"\n"}{end}'
```

Expected output for the tested cluster:

```text
ServiceAccount kube-system/statefulset-controller
```

Use the manifest [openshift_gdd_finalizer_rbac.yaml](./provisioning/openshift/openshift_gdd_finalizer_rbac.yaml).
If the cluster uses a different controller identity, adjust the RoleBinding subject accordingly.

Apply the manifest before creating the ShardingDatabase:

```sh
oc apply -f openshift_gdd_finalizer_rbac.yaml
```

Verify permission for the specific ShardingDatabase name:

```sh
oc auth can-i update "shardingdatabases.database.oracle.com/$GDD" \
  --subresource=finalizers \
  --as=system:serviceaccount:kube-system:statefulset-controller \
  -n "$NS"
```

Expected result: `yes`.

The Role grants only `update` on the `finalizers` subresource of
`shns/sdb-system`. Include the resource name in the verification command;
a permission check without a name can return `no` because this Role is
restricted by `resourceNames`.

This configuration grants an RBAC permission. It does not require
removing or changing finalizers on the ShardingDatabase or its storage
resources.

### Reference the ServiceAccount in the GDD Manifest

Add `spec.serviceAccountName: shns-sa` to the standard ShardingDatabase
manifest. Keep the existing database, GSM, shard, storage, and Secret
configuration.

The following fragment shows the required placement:

```yaml
apiVersion: database.oracle.com/v4
kind: ShardingDatabase
metadata:
  name: sdb-system
  namespace: shns
spec:
  serviceAccountName: shns-sa

  # Keep the remaining GDD configuration here.
```

`serviceAccountName` is a top-level field under `spec`, alongside fields
such as `dbImage`, `gsmImage`, and `storageClass`.

### Deploy and Verify

After applying both RBAC manifests, deploy the ShardingDatabase. The tested
deployment manifest was:

```sh
oc apply -f system_sharding/ssharding_shard_prov_sa.yaml
```

Verify that the generated StatefulSets use the intended ServiceAccount:

```sh
oc get statefulsets -n "$NS" \
  -o custom-columns='NAME:.metadata.name,SERVICEACCOUNT:.spec.template.spec.serviceAccountName'
```

For this example, `catalog`, `gsm1`, `gsm2`, `pshard1`, and `pshard2` should
all show `shns-sa`.

Check the Pods, PVCs, and selected SCC:

```sh
oc get pods,pvc -n "$NS"

oc get pods -n "$NS" \
  -o custom-columns='NAME:.metadata.name,STATUS:.status.phase,SA:.spec.serviceAccountName,SCC:.metadata.annotations.openshift\.io/scc'
```

The expected ServiceAccount is `shns-sa`, and the expected SCC for the
tested configuration is `shns-oracle-user-scc`.

Watch provisioning progress:

```sh
oc get pods -n "$NS" -w
```

Press Ctrl+C to stop watching. Pod creation confirms that admission has
progressed; continue the standard GDD workflow to verify database and
sharding topology readiness.

If Pods or PVCs are not created, inspect the StatefulSet events:

```sh
oc describe statefulset catalog -n "$NS"

oc get events -n "$NS" --sort-by=.metadata.creationTimestamp
```

For SCC capability errors, verify the SCC capability list, its RoleBinding,
and the StatefulSet ServiceAccount. For the `blockOwnerDeletion` error,
verify the StatefulSet controller's permission for the exact
ShardingDatabase name.

If an existing deployment is blocked by either permission issue, apply
the corresponding RBAC correction and allow the StatefulSet controller
to retry. Deleting the ShardingDatabase or restarting the operator is not
required for these permission corrections.

## Connecting to Oracle Globally Distributed Database

After provisioning the topology, follow [Database Connectivity](./provisioning/database_connection.md) to connect through GSM or directly to an individual shard database.

## Frequently Asked Questions

**What is Oracle Globally Distributed Database (GDD)?**

Oracle Globally Distributed Database (GDD) is a distributed database architecture that stores data across multiple Oracle databases (shards) while presenting them as a single logical database. It provides linear scalability, high availability, low-latency access, and data sovereignty.

---

**What is the Oracle AI Database Operator Sharding Controller?**

The Oracle AI Database Operator Sharding Controller automates the deployment and lifecycle management of Oracle Globally Distributed Database topologies on Kubernetes. It provisions catalog databases, shard databases, Global Service Managers (GSM), networking, storage, and supports scaling operations.

---

**Which sharding types are supported?**

The Oracle AI Database Operator supports the following sharding types:

- System-Managed Sharding
- User-Defined Sharding
- Composite Sharding

Choose the deployment guide that matches your application architecture and data distribution requirements.

---

**Which replication types are supported?**

Oracle Globally Distributed Database supports:

- Data Guard replication
- Native (Raft) replication

Native (Raft) replication is supported starting with Oracle AI Database 26ai.

---

**Can I use Native (Raft) replication with every sharding type?**

No. Native (Raft) replication is supported for System-Managed Sharding and Composite Sharding, but not for User-Defined Sharding. Native replication is available starting with Oracle AI Database 26ai.

---

**Where can I find the complete ShardingDatabase YAML template?**

A fully documented template containing all configurable parameters is available at:

[provisioning/oraclegdd.yaml](./provisioning/oraclegdd.yaml)

Use this template as the starting point for creating your own Oracle GDD deployment manifests.

---

**Can I deploy Oracle Globally Distributed Database using Oracle AI Database 26ai Free?**

Yes. Oracle AI Database 26ai Free is supported.

When using Oracle AI Database 26ai Free:

- Set `dbEdition: "free"` in the manifest.
- Native (Raft) replication is supported.
- A maximum of three shards is supported.
- Database Gold Image cloning is not supported.

---

**Can I scale the number of shards after deployment?**

Yes.

The Oracle AI Database Operator supports both:

- Scale-out by adding new shards
- Scale-in by deleting existing shards

Refer to the corresponding deployment scenario for your sharding type.

---

**Can I customize CPU, memory, and pod placement?**

Yes.

The deployment manifests support:

- CPU and memory resource requests and limits
- Kubernetes node selection
- Shared storage mount points
- Additional Persistent Volume Claims (PVCs)

See **Additional Deployment Scenarios** for examples.

---

**Can I expose Oracle GDD services outside the Kubernetes cluster?**

Yes.

Set the following parameter in the `ShardingDatabase` resource:

```yaml
isExternalSvc: true
```

This creates external LoadBalancer services for Catalog, Shards, and GSM components.

---

**How do I connect to Oracle Globally Distributed Database after deployment?**

After the topology is successfully provisioned, follow the instructions in [Database Connectivity](./provisioning/database_connection.md) to connect either to:

- the Oracle Globally Distributed Database through GSM, or
- individual shard databases.

---

**Where can I find troubleshooting information?**

See the **Debugging and Troubleshooting** section for common diagnostic procedures and the **Known Issues** section for current limitations and workarounds.

## Debugging and Troubleshooting

To debug the Oracle Globally Distributed Database topology provisioned using the Oracle AI Database Operator Sharding Controller, follow this document: [Debug Oracle GDD deployments](./provisioning/debugging.md)

## Known Issues

* For both Enterprise and Free images, stopping a Global Service Manager (GSM) pod with `crictl stopp` at the worker-node level can leave GSM in a failed state. The `gdsctl` commands fail with error **GSM-45034: Connection to GDS catalog is not established**.
* For both Enterprise and Free images, restart of the node running CATALOG using `/sbin/reboot -f` results in **GSM-45076: GSM IS NOT RUNNING**. After you encounter this issue, wait until database connectivity is restored and the `gdsctl` commands succeed. When the stack comes up again after the node restart, you can encounter an unexpected restart of the GSM pod.
* For both Enterprise and Free images, if the catalog database Pod is stopped from the worker node using the command `crictl stopp`, then it can leave the CATALOG in an error state. This error state results in GSM reporting the error message **GSM-45034: Connection to GDS catalog is not established.**
* For both Enterprise and Free images, either restart of node running the SHARD Pod using `/sbin/reboot -f` or stopping the shard database Pod from the worker node using `crictl stopp` command can leave the shard in an error state.
* For both Enterprise and Free images, after a forced restart of the node running a GSM pod, the GSM pod restarts multiple times, and then becomes stable. The GSM pod restarts itself because when the worker node comes up, the GSM pod is recreated, but does not obtain DB connection to the Catalog. The liveness probe then fails and restarts the pod. Be aware of this issue, and allow time for the GSM pod to stabilize
* **DDL Propagation from Catalog to Shards:** DDL Propagation from the catalog database to the shard databases can take several minutes to complete. To see faster propagation of DDLs such as the tablespace set from the catalog database to the shard databases, Oracle recommends that you set smaller chunk values by using the `CATALOG_CHUNKS` attribute in the YAML manifest while creating the Sharded Database Topology.
