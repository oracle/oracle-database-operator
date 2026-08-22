<div align="center">
<h1 align="center">
  <br>
  <img src="./DBOperator.svg" alt="Oracle Database Operator">
</h1>
</div>

<div align="center">
<p align="center">
    <a href="https://github.com/oracle/oracle-database-operator">
    <img src="./oracledatabaseoperator.svg?style=flat-square&logo=github&logoColor=white"
         alt="Oracle Database Operator">
    </a>
    <a href="https://www.oracle.com/database/kubernetes-for-container-database">
    <img src="./oracledatabaseforcontainersandkubernetes.svg?style=flat-square&logo=github&logoColor=white"
         alt="Oracle Database For Containers and Kubernetes">
    </a>
</p>
</div>

<div align="center">
<p align="center">
    <a href="https://github.com/oracle/oracle-database-operator/commits/main/">
    <img src="./lastcommit.svg?style=flat-square&logo=github&logoColor=white"
         alt="GitHub last commit">
    </a>
    <a href="https://github.com/oracle/oracle-database-operator/issues">
    <img src="./issues.svg?style=flat-square&logo=github&logoColor=white"
         alt="GitHub issues">
    </a>
    <a href="https://github.com/oracle/oracle-database-operator/pulls">
    <img src="pullrequest.svg?style=flat-square&logo=github&logoColor=white"
         alt="GitHub pull requests">
    </a>
</p>
</div>

# Oracle Database Operator for Kubernetes

The Oracle Database Operator extends the Kubernetes API by introducing custom resources and controllers that enable the provisioning, management, and lifecycle automation of Oracle Database workloads and associated services on Kubernetes.

It simplifies database operations by leveraging Kubernetes-native constructs, allowing teams to manage Oracle databases using declarative configurations and standard tooling. This approach improves consistency, scalability, and operational efficiency across environments.

Use this repository to install and validate the operator, ensure it is running correctly, and then navigate to the relevant guides for managing specific controllers or database workloads. It also provides examples, configuration references, and best practices to help you get started quickly and operate reliably in production environments.

## Contents

- [What's New](#whats-new)
- [Platform Compatibility](#platform-compatibility)
- [Prerequisites](#prerequisites)
- [Quick Start](#quick-start)
- [Install the Operator](#install-the-operator)
- [Verify Installation](#verify-installation)
- [Choose Your Guide](#choose-your-guide)
- [Uninstall the Operator](#uninstall-the-operator)
- [Contributing](#contributing)
- [Help](#help)
- [Security](#security)
- [Troubleshooting](#troubleshooting)
- [License](#license)

## What's New

### v2.2.0

Version 2.2 introduces expanded database lifecycle automation, stronger reconciliation, new networking capabilities, and improved operator security and observability.

| Controller / area | Highlights |
| --- | --- |
| **AutonomousDatabase** | • OCI lifecycle reconciliation, finalizers, wallet validation and rotation<br>• Backup-resource synchronization and cleanup |
| **AutonomousContainerDatabase** | • OCI lifecycle-state validation and finalizer-based cleanup<br>• Kubernetes-to-OCI state synchronization |
| **AutonomousDatabaseBackup / Restore** | • Target validation and owner references<br>• Point-in-time restore and OCI work-request integration |
| **Multitenant controllers (LREST / LRPDB)** | • Internal comunication protocol improvement and cofiguration simplification - No need to specify https password <br>  • Creating application users on pdb using k8s secrets  <br> • Monitor pdb init parameters with reconciliation loop <br>  • Code optimization  <br> • Reset bitmask status simplification <br> • Load tnsname.ora topology |
| **OracleRestart** | • Phased reconciliation for validation, storage, workload, and finalization<br>• ASM disk lifecycle with static/dynamic PV/PVC handling |
| **ORDS Services (OrdsSrvs)** | • HTTP-only edge deployments, HTTP access-log forwarding/persistence, and Instance API bootstrap<br>• Configurable resource limits, metadata, and JVM options |
| **RacDatabase** | • Expanded RAC and ASM storage lifecycle<br>• Disk/PVC provisioning, and validation |
| **ShardingDatabase** | • Oracle GDD topology lifecycle and scaling<br>• Catalog/shard management, and status validation |
| **SingleInstanceDatabase** | • Service endpoints, TCPS, TrueCache, Data Guard prerequisites, clone/restore, and external PVCs<br>• Improved pod security, resource handling, recreation checks, and connection status |
| **DataguardBroker** | • Topology runtime, authentication wallets, validation/provisioning, FSFO observer management, and operation tracking<br>• Idempotent manual switchover support |
| **DatabaseObserver** | • Safer child-resource ownership and Server-Side Apply<br>• Improved deployment readiness and status handling |
| **PrivateAI** | • Phased dependency/workload reconciliation and update-lock status<br>• TLS secret lifecycle, support for vLLM and GPU, and rollout tracking |
| **TrafficManager** *(new)* *(preview mode)*| • Oracle Connection Manager (CMAN) endpoints<br>• generated rules, `cman.ora` file mode, and endpoint status |
| **Operator platform** | • New `network.oracle.com/v4` API and TrafficManager CRD<br>• Secure HTTPS metrics, hardened manager security context, expanded RBAC/webhooks, compatibility webhooks, network policy, samples, and test coverage |

## Platform Compatibility

This production release has been installed and tested on the following platforms:

| Platform | Version |
| --- | --- |
| [Oracle Container Engine for Kubernetes (OKE)](https://www.oracle.com/cloud-native/container-engine-kubernetes/) | Kubernetes 1.33 or later |
| [Red Hat OpenShift](https://www.redhat.com/en/technologies/cloud-computing/openshift) | 4.19 or later |
| [Oracle Linux Cloud Native Environment (OLCNE)](https://docs.oracle.com/en/operating-systems/olcne/) | 1.9 or later |
| [Google Kubernetes Engine](https://cloud.google.com/kubernetes-engine/docs) | Supported |
| [Azure Kubernetes Service](https://azure.microsoft.com/en-us/services/kubernetes-service/) | Supported |
| [Amazon Elastic Kubernetes Service](https://aws.amazon.com/eks/) | Supported |
| [Red Hat OKD](https://www.okd.io/) | Supported |
| [Minikube](https://minikube.sigs.k8s.io/docs/) | 1.29.0 or later |

## Prerequisites

Oracle strongly recommends reviewing [PREREQUISITES.md](./PREREQUISITES.md) before installation.

### Install cert-manager

The operator uses webhooks for validating user input before persisting it in `etcd`. Webhooks require TLS certificates that are generated and managed by a certificate manager.

> **OCNE note:** Before installing cert-manager on an OCNE cluster, review the [cert-manager supported releases](https://cert-manager.io/docs/releases/) and choose a cert-manager version that supports the Kubernetes version provided by your OCNE release. OCNE 1.9 uses Kubernetes 1.29, so replace the cert-manager version in the example below with a Kubernetes 1.29-compatible release.

```sh
kubectl apply -f https://github.com/cert-manager/cert-manager/releases/download/v1.20.2/cert-manager.yaml
```

### Choose Deployment Scope

The default installation is namespace-scoped. The operator runs in `oracle-database-operator-system` and watches that namespace by default. It does not watch other namespaces unless they are added to `WATCH_NAMESPACE` and the service account is granted access there.

The operator also supports an optional cluster-scoped mode. Apply the base RBAC manifest first so the operator namespace, service account, and packaged manager roles exist before the controller starts.

If you are planning to install the operator through an OKE add-on, OperatorHub.io, or Red Hat OpenShift, use the corresponding instructions under [Install The Operator](#install-the-operator): [Install as an OKE Add-on](#install-as-an-oke-add-on), [Install from OperatorHub.io](#install-from-operatorhubio), or [Install on Red Hat OpenShift](#install-on-red-hat-openshift). Those installation channels manage the deployment manifests and lifecycle, so do not apply the repository YAML scope procedures in addition to the selected platform installation. If you are deploying with repository YAML files, skip to [Default namespace-scoped deployment](#default-namespace-scoped-deployment).

#### Default namespace-scoped deployment

For a namespace-scoped deployment, generate the operator manifests from a comma-separated namespace list. The generated system manifest sets `WATCH_NAMESPACE`, and the generated RBAC manifest adds one manager `RoleBinding` for each watched namespace.

Example:

```sh
scripts/generate-namespace-install.sh oracle-database-operator-system,shns
```

Then apply the generated manifests:

```sh
kubectl apply -f dist/install/oracle-database-operator-rbac.yaml
kubectl apply -f dist/install/oracle-database-operator-system.yaml
```

The script keeps the operator service account in `oracle-database-operator-system` and grants access only in the namespaces listed in `WATCH_NAMESPACE`. This avoids manually editing `oracle-database-operator-system.yaml`, `oracle-database-operator-rbac.yaml`, and namespace role binding files separately.

The generated deployment contains the same namespace list:

```yaml
- name: WATCH_NAMESPACE
  value: "oracle-database-operator-system,shns"
```

Verify authorization one namespace at a time. The `-n`/`--namespace` option accepts only one namespace; do not pass the comma-separated `WATCH_NAMESPACE` value to it:

```sh
for namespace in oracle-database-operator-system shns; do
  echo "Checking ${namespace}"
  kubectl auth can-i list lrpdbs.database.oracle.com \
    --as=system:serviceaccount:oracle-database-operator-system:oracle-database-operator-controller-manager \
    -n "${namespace}"
done
```

#### Updating an existing namespace-scoped installation

Do not update only `WATCH_NAMESPACE`. Each namespace in the watch list must also have a manager `RoleBinding` for the existing operator ServiceAccount. The ServiceAccount itself does not change.

For the standard installation, use these values:

```sh
OPERATOR_NS=oracle-database-operator-system
SERVICE_ACCOUNT=oracle-database-operator-controller-manager
MANAGER_ROLE=oracle-database-operator-manager-role
```

To add namespaces, first create or update the RoleBinding in every new namespace:

```sh
for namespace in shns db-workloads; do
  kubectl create rolebinding oracle-database-operator-manager-rolebinding \
    --clusterrole="${MANAGER_ROLE}" \
    --serviceaccount="${OPERATOR_NS}:${SERVICE_ACCOUNT}" \
    --namespace="${namespace}" \
    --dry-run=client -o yaml | kubectl apply -f -
done
```

Verify access before changing the Deployment. Check each namespace separately; `kubectl auth can-i -n` does not accept a comma-separated namespace list:

```sh
for namespace in shns db-workloads; do
  kubectl auth can-i list singleinstancedatabases.database.oracle.com \
    --as="system:serviceaccount:${OPERATOR_NS}:${SERVICE_ACCOUNT}" \
    --namespace="${namespace}"
done
```

Then set the complete watch list. This replaces the previous value and triggers a rollout:

```sh
kubectl set env deployment/oracle-database-operator-controller-manager \
  -n "${OPERATOR_NS}" \
  WATCH_NAMESPACE="${OPERATOR_NS},shns,db-workloads"

kubectl rollout status deployment/oracle-database-operator-controller-manager \
  -n "${OPERATOR_NS}" --timeout=300s
```

To remove a namespace, first update `WATCH_NAMESPACE` without it and wait for the rollout. Then remove its obsolete RoleBinding:

```sh
kubectl set env deployment/oracle-database-operator-controller-manager \
  -n "${OPERATOR_NS}" \
  WATCH_NAMESPACE="${OPERATOR_NS},shns"

kubectl rollout status deployment/oracle-database-operator-controller-manager \
  -n "${OPERATOR_NS}" --timeout=300s

kubectl delete rolebinding oracle-database-operator-manager-rolebinding \
  -n db-workloads --ignore-not-found=true
```

For a fresh installation, continue using `scripts/generate-namespace-install.sh`; it generates the Deployment value and namespace RoleBindings together. For an existing installation, use the in-place procedure above instead of applying a full generated system manifest from a different operator version.

Before applying or changing the operator configuration, verify that the ServiceAccount can list resources in each watched namespace. For example:

```sh
kubectl auth can-i list lrpdbs.database.oracle.com \
  --as=system:serviceaccount:oracle-database-operator-system:oracle-database-operator-controller-manager \
  -n <watched-namespace>
```

#### Cluster-scoped deployment

To switch an existing operator from namespace scope to cluster scope, first complete the [Default namespace-scoped deployment](#default-namespace-scoped-deployment) section, including its RBAC, Deployment, and permission checks. After the namespace-scoped installation is healthy, follow the steps below to add the cluster binding and unset `WATCH_NAMESPACE`.


Namespace-scoped deployment is preferred because it follows least-privilege practice and limits the operator's access to selected Kubernetes namespaces. Cluster-scoped deployment grants the operator wider access across the cluster and should be used only when your operational requirements require it. If cluster scope is required for your environment, follow the steps below.

The base installation already creates the manager `ClusterRole`, the operator ServiceAccount, and a namespace-scoped manager `RoleBinding`. For an existing installation, do not recreate the ServiceAccount or replace the manager `ClusterRole`. Add a `ClusterRoleBinding` for the existing ServiceAccount, then remove `WATCH_NAMESPACE`. Keep the leader-election `RoleBinding` in `oracle-database-operator-system`.

Apply the cluster binding before changing the Deployment:

```sh
kubectl apply -f rbac/cluster-role-binding.yaml

kubectl set env deployment/oracle-database-operator-controller-manager \
  -n oracle-database-operator-system \
  WATCH_NAMESPACE-

kubectl rollout status deployment/oracle-database-operator-controller-manager \
  -n oracle-database-operator-system \
  --timeout=300s
```

The existing namespace-scoped manager `RoleBinding` may remain, but it is redundant after the `ClusterRoleBinding` is active. For a fresh installation or a full version upgrade, use the matching cluster-scoped manifest generated from that exact operator version; do not apply a cluster overlay from a different version to an existing Deployment.

Verify that it can manage resources outside its installation namespace:

```sh
kubectl auth can-i list singleinstancedatabases.database.oracle.com \
  --as=system:serviceaccount:oracle-database-operator-system:oracle-database-operator-controller-manager \
  -n default

kubectl get pods -n oracle-database-operator-system

kubectl logs -n oracle-database-operator-system \
  deployment/oracle-database-operator-controller-manager -c manager \
  | grep 'CLUSTER SCOPED'
```

### Optional Cluster-Scoped RBAC

Namespace-scoped deployment controls which namespaces the operator watches and where namespace `RoleBinding` objects are created. A namespace `RoleBinding` does not grant access to cluster-scoped Kubernetes resources such as `PersistentVolume`, `StorageClass`, or `Node`. Cluster scope grants the permissions already present in the manager `ClusterRole` across the cluster.

In the packaged manager role, `namespaces`, `persistentvolumes`, and `storageclasses` are already included. `nodes` is not included by default and requires the optional NodePort RBAC when a selected feature needs node access.

Apply the following optional RBAC manifests only when the selected controller feature requires them:

| Feature | Cluster-scoped resource | Apply |
| --- | --- | --- |
| NodePort service connect strings | `nodes` | `kubectl apply -f rbac/node-rbac.yaml` |
| Storage expansion for block volumes | `storageclasses.storage.k8s.io` | `kubectl apply -f rbac/storage-class-rbac.yaml` |
| SIDB custom scripts with existing PersistentVolumes | `persistentvolumes` read access | `kubectl apply -f rbac/persistent-volume-rbac.yaml` |
| RAC ASM PersistentVolume lifecycle | `persistentvolumes` create/delete access | `kubectl apply -f docs/rac/rbac/pv-rbac.yaml` |

For example, if you plan to expose services using `NodePort`, apply:

```sh
kubectl apply -f rbac/node-rbac.yaml
```

These optional manifests are cluster-scoped. Review them before applying, and avoid granting cluster-scoped PV access unless the workload needs that specific capability.

## Quick Start

Before using this quick start, complete the [prerequisites](#prerequisites):

- Install [cert-manager](#install-cert-manager), which provides the webhook TLS certificates.
- Use the default namespace-scoped deployment, or explicitly choose [cluster scope](#cluster-scoped-deployment).
- Apply the required RBAC before starting the operator.
- For namespace-scoped deployment, use `scripts/generate-namespace-install.sh` so the RBAC and `WATCH_NAMESPACE` values are generated from the same namespace list.
- Apply `oracle-database-operator-system.yaml` after RBAC to create the CRDs, webhooks, services, certificates, network policy, and operator Deployment.

1. Verify the operator ServiceAccount permissions in each watched namespace. The `-n`/`--namespace` option accepts one namespace only, so run the check separately for every namespace:

   ```sh
   for namespace in oracle-database-operator-system <additional-watched-namespace>; do
     echo "Checking ${namespace}"
     kubectl auth can-i list lrpdbs.database.oracle.com \
       --as=system:serviceaccount:oracle-database-operator-system:oracle-database-operator-controller-manager \
       -n "${namespace}"
   done
   ```

2. Verify the operator pods are healthy:

   ```sh
   kubectl get pods -n oracle-database-operator-system
   ```

3. [Apply a Controller Resource](#apply-a-controller-resource) in a namespace watched by the operator.


### Apply a Controller Resource

The operator deployment starts the controllers. To use a specific controller, apply a custom resource for that controller in a namespace the operator watches. Use the guide in the [Supported Controllers](#supported-controllers) table to choose the correct sample and prerequisites.

For example, after completing the Single Instance Database prerequisites and granting access to the target namespace, apply a Single Instance Database resource:

```sh
kubectl apply -f config/samples/sidb/singleinstancedatabase.yaml
```

Then watch the resource status in that namespace:

```sh
kubectl get singleinstancedatabase -n <watched-namespace>
```

## Install The Operator

The Oracle Database Operator can be installed in several ways, depending on your platform and operational model. If you already installed the operator by applying the YAML files in the [Quick Start](#quick-start), you can continue directly to [Supported Controllers](#supported-controllers).

### Install from YAML Manifests

If you want to deploy the operator with YAML manifests from this repository, first review the [Choose Deployment Scope](#choose-deployment-scope) section. Namespace scope is the default and recommended option: it limits the operator to the namespaces listed in <code>WATCH_NAMESPACE</code>, and generated RBAC grants access only in those namespaces. Cluster scope grants the manager <code>ClusterRole</code> across the cluster and should be used only when that broader access is required.

The combined [`oracle-database-operator.yaml`](./oracle-database-operator.yaml) remains available for compatibility. For new installations, follow the [Choose Deployment Scope](#choose-deployment-scope) instructions and use manifests generated for the same operator version.

#### Optional UID pinning for non-OpenShift clusters

The operator manifests do not pin the manager container to a fixed UID so that OpenShift Security Context Constraints can assign a namespace-valid UID automatically. On non-OpenShift Kubernetes clusters, if your security policy requires the manager container to run as UID/GID `1002`, use one of these options.

Before first deployment, add `runAsUser` and `runAsGroup` to the manager container security context in the operator YAML you plan to apply:

```yaml
securityContext:
  allowPrivilegeEscalation: false
  capabilities:
    drop:
    - ALL
  runAsNonRoot: true
  runAsUser: 1002
  runAsGroup: 1002
```

If the operator is already deployed, apply the same setting with a patch:

```sh
kubectl -n oracle-database-operator-system patch deployment oracle-database-operator-controller-manager \
  --type='json' \
  -p='[
    {"op":"add","path":"/spec/template/spec/containers/0/securityContext/runAsUser","value":1002},
    {"op":"add","path":"/spec/template/spec/containers/0/securityContext/runAsGroup","value":1002}
  ]'
```

Do not apply this UID patch on OpenShift. OpenShift assigns a valid UID from the namespace range when `runAsUser` is omitted.

The upstream project and pre-built manifest artifacts are available at [oracle/oracle-database-operator](https://github.com/oracle/oracle-database-operator).

### Install as an OKE Add-on

On Oracle Container Engine for Kubernetes (OKE), you can install and manage the Oracle Database Operator as an OKE add-on from the OCI Console or compatible automation flows. This is the recommended path when you want OKE to manage the add-on lifecycle and configuration.

For OKE add-on configuration options, see [Oracle Database Operator for Kubernetes add-on configuration](https://docs.oracle.com/en-us/iaas/Content/ContEng/Tasks/configuration-arguments-db-operator-for-k8s.htm).

### Install from OperatorHub.io

You can also install the operator from [OperatorHub.io](https://operatorhub.io/operator/oracle-database-operator).

1. Open the [Oracle Database Operator page](https://operatorhub.io/operator/oracle-database-operator).
2. Follow the installation instructions for your Kubernetes platform.

### Install on Red Hat OpenShift

For Red Hat OpenShift environments, use the partner-validated Oracle Database Operator listing and follow the platform-specific guidance for your cluster. This path is useful when you want to install through the Red Hat catalog and align with OpenShift validation and lifecycle practices.

For the partner-validated listing, see [Oracle Database Operator on Red Hat Ecosystem Catalog](https://catalog.redhat.com/en/software/container-stacks/detail/68c4502d45d3fc6d301a9980).

After installation, continue to [Verify Installation](#verify-installation). For OKE add-on, OperatorHub, or OpenShift catalog installations, also review any status and lifecycle checks recommended by that installation channel.

### Build and Deploy DB Operator (Optional)

We highly recommend using the prebuilt Oracle Database Operator container images published in Oracle Container Registry (OCR). These images are versioned, validated, and ready for deployment.

If your organization requires building the operator image manually in your own environment, follow the detailed [Oracle Database Operator Manual Build and Deployment Guide](./docs/installation/OPERATOR_INSTALLATION_README.md), which covers the build prerequisites, image build and tagging steps, registry handling, and deployment configuration.


## Verify Installation

Check that the operator pods are running:

```sh
kubectl get pods -n oracle-database-operator-system
```

Example output:

```sh
NAME                                                                 READY   STATUS    RESTARTS   AGE
oracle-database-operator-controller-manager-78666fdddb-s4xcm         1/1     Running   0          11d
oracle-database-operator-controller-manager-78666fdddb-5k6n4         1/1     Running   0          11d
oracle-database-operator-controller-manager-78666fdddb-t6bzb         1/1     Running   0          11d
```


For a complete installation check, run the following commands:

```sh
# Confirm the Deployment rollout
kubectl rollout status deployment/oracle-database-operator-controller-manager \
  -n oracle-database-operator-system --timeout=300s

# Check replica readiness and pod placement
kubectl get deployment oracle-database-operator-controller-manager \
  -n oracle-database-operator-system
kubectl get pods -n oracle-database-operator-system -o wide

# Verify permissions separately in every watched namespace
kubectl auth can-i list lrpdbs.database.oracle.com \
  --as=system:serviceaccount:oracle-database-operator-system:oracle-database-operator-controller-manager \
  -n <watched-namespace>

# Confirm a representative CRD is installed
kubectl get crd lrpdbs.database.oracle.com

# Confirm the admission webhooks exist
kubectl get mutatingwebhookconfiguration mutating-webhook-configuration
kubectl get validatingwebhookconfiguration validating-webhook-configuration

# Review recent controller logs
kubectl logs deployment/oracle-database-operator-controller-manager \
  -n oracle-database-operator-system -c manager --tail=100

# Review recent installation-namespace events when troubleshooting
kubectl get events -n oracle-database-operator-system --sort-by=.lastTimestamp
```

For cluster-scoped installations, repeat the permission check in a namespace outside `oracle-database-operator-system`, such as `default`. These checks validate the operator installation itself; verify workload-specific custom resources separately after you deploy them.


## Choose Your Guide

After the operator is installed, continue with the guide for your workload:

### Supported Controllers

| Controller | Primary use case | Common operations | Guide |
| --- | --- | --- | --- |
| Autonomous Database | Manage Oracle Autonomous Database resources on OCI | provision, bind, start, stop, scale, backup, restore, failover | [docs/adb/README.md](./docs/adb/README.md) |
| Autonomous Container Database | Manage the Autonomous Container Database infrastructure | provision, bind, restart, terminate | [docs/adb/ACD.md](./docs/adb/ACD.md) |
| Single Instance Database and Data Guard | Manage containerized Oracle single instance databases | provision, patch, clone, Data Guard, standby role conversion, ORDS, PDB operations | [docs/sidb/README.md](./docs/sidb/README.md) |
| Oracle GDD | Manage Oracle globally distributed  databases | provision topology, add shards, remove shards, Raft replication | [docs/sharding/README.md](./docs/sharding/README.md) |
| Multitenant | Manage CDB/PDB lifecycle | create, plug, unplug, clone, open, close, delete | [docs/multitenant/README.md](./docs/multitenant/README.md) |
| Oracle Base Database Service | Manage Oracle Base Database Service resources on OCI | provision, scale, clone, backup, restore, patch, Data Guard | [docs/dbcs/README.md](./docs/dbcs/README.md) |
| ORDS Services | Manage ORDS service deployments | provision, update, delete | [docs/ordsservices/README.md](./docs/ordsservices/README.md) |
| Oracle RAC | Manage Oracle Real Application Clusters | provision, scale, add or remove ASM disks | [docs/rac/README.md](./docs/rac/README.md) |
| Oracle Restart | Manage Oracle Restart deployments | provision, ASM disk operations, load balancer support | [docs/oraclerestart/README.md](./docs/oraclerestart/README.md) |
| Private AI | Manage Oracle Private AI Services Container | deploy, scale, configure networking, manage runtime updates | [docs/privateai/README.md](./docs/privateai/README.md) |
| Traffic Manager (CMAN) | Route database listener traffic | CMAN for Oracle listener connectivity | [docs/trafficmanager/README.md](./docs/trafficmanager/README.md) |

Traffic Manager (CMAN) works with Single Instance Database or Oracle RAC for CMAN-based listener access. See the [Traffic Manager guide](./docs/trafficmanager/README.md) for CMAN generated and file-mode configuration, and example manifests under [`docs/trafficmanager/samples/`](./docs/trafficmanager/samples/).

### Supporting Services

- [Oracle Database Observability](./docs/observability/README.md)
- [Oracle Database Operator Metrics](./docs/operator-metrics/README.md)

## Uninstall the Operator

Uninstall the operator in the reverse order of installation. Before removing the operator deployment or CRDs, decide whether you want to keep or delete the database custom resources that the operator manages.

### 1. Delete Custom Resources

Delete custom resources before deleting the operator deployment or CRDs. This allows the operator to run finalizers and clean up Kubernetes resources that it created.

To review Oracle Database Operator resources in a namespace:

```sh
kubectl api-resources --verbs=list --namespaced -o name \
  | grep -E 'database.oracle.com|observability.oracle.com|privateai.oracle.com|network.oracle.com'
```

Then delete the custom resources you no longer need in each namespace:

```sh
kubectl delete <resource-name> --all -n <namespace>
```

If you installed the operator in namespace-scoped mode, repeat this step for every namespace listed in `WATCH_NAMESPACE`.

### 2. Remove the Operator Installation

Use the uninstall method that matches how you installed the operator.

For YAML manifest installations, remove the system manifest first and then remove RBAC:

```sh
kubectl delete -f oracle-database-operator-system.yaml --ignore-not-found=true
kubectl delete -f oracle-database-operator-rbac.yaml --ignore-not-found=true
```

If you installed with the combined compatibility manifest, use:

```sh
kubectl delete -f oracle-database-operator.yaml --ignore-not-found=true
```

For OKE add-on, OperatorHub, or OpenShift catalog installations, uninstall the operator using the same channel you used to install it. Follow that platform's guidance to remove subscriptions, add-ons, or catalog-managed resources.

### 3. Remove CRDs Only When Appropriate

Removing CRDs deletes the API definitions for all Oracle Database Operator custom resources. Do this only after all custom resources have been deleted or intentionally preserved elsewhere.

To list Oracle Database Operator CRDs:

```sh
kubectl get crd | grep -E 'database.oracle.com|observability.oracle.com|privateai.oracle.com|network.oracle.com'
```

If you are certain the CRDs should be removed, delete them explicitly:

```sh
kubectl get crd -o name \
  | grep -E 'database.oracle.com|observability.oracle.com|privateai.oracle.com|network.oracle.com' \
  | xargs --no-run-if-empty kubectl delete
```

## Documentation for the Supported Oracle Database configurations

- [Oracle Autonomous Database](https://docs.oracle.com/en-us/iaas/Content/Database/Concepts/adboverview.htm)
- [Components of Dedicated Autonomous Database](https://docs.oracle.com/en-us/iaas/autonomous-database/doc/components.html)
- [Oracle Database Single Instance](https://docs.oracle.com/en/database/oracle/oracle-database/)
- [Oracle Globally Distributed Database](https://docs.oracle.com/en/database/oracle/oracle-database/21/shard/index.html)
- [Oracle Database Cloud Service](https://docs.oracle.com/en/database/database-cloud-services.html)

## Contributing

This project welcomes contributions from the community. Before submitting a pull request, please [review our contribution guide](./CONTRIBUTING.md)

## Help

You can submit a GitHub issue, or submit an issue and then file an [Oracle Support service](https://support.oracle.com/portal/) request. To file an issue or a service request, use the following product ID: 14430.

## Security

For information about responsible security vulnerability disclosure, see [Reporting security vulnerabilities](./docs/security/SECURITY.md).

For operator identity, ServiceAccount, RBAC, token, and pod security posture guidance, see [Oracle Database Operator Security Posture](./docs/security/operator-security-posture.md).

## Troubleshooting

For common Oracle Database Operator troubleshooting commands, including pod health, logs, webhook, certificate, RBAC, role binding, CRD, and event checks, see [TROUBLESHOOTING.md](./TROUBLESHOOTING.md).

## License

Copyright (c) 2022, 2026 Oracle and/or its affiliates.
Released under the Universal Permissive License v1.0 as shown at [https://oss.oracle.com/licenses/upl/](https://oss.oracle.com/licenses/upl/)
