# Installing the Oracle Database Operator

This guide describes the supported ways to install the Oracle Database Operator for Kubernetes. Choose one installation method and use manifests, scripts, and documentation from the same operator version.

> **Important:**
>
> Do not combine repository YAML manifests with an OKE add-on, OperatorHub.io, or Red Hat OpenShift catalog installation. These installation channels manage the operator Deployment and lifecycle differently.

## Installation methods

| Method | Recommended for |
| --- | --- |
| YAML manifests | Standard Kubernetes and GitOps installations |
| OKE add-on | OKE clusters managed through OCI |
| OperatorHub.io | Kubernetes platforms using Operator Lifecycle Manager |
| Red Hat OpenShift catalog | OpenShift installations using the Red Hat catalog |
| Manual build | Environments that require a custom operator image |

## Common prerequisites

Before installing the operator:

1. Confirm that the Kubernetes cluster meets the supported platform and version requirements.
2. Install a compatible version of [cert-manager](https://cert-manager.io/). The operator's admission webhooks require TLS certificates managed by cert-manager.
3. Confirm that the installation account can create the required namespaces, RBAC resources, CRDs, webhooks, Services, certificates, NetworkPolicy, and Deployment.
4. Review the prerequisites in the selected controller guide before deploying workloads.

## Install from YAML manifests

### Choose deployment scope

The default installation is namespace-scoped. The operator runs in `oracle-database-operator-system` and watches that namespace by default. It does not watch other namespaces unless they are added to `WATCH_NAMESPACE` and the service account is granted access there.

The operator also supports an optional cluster-scoped mode. Apply the base RBAC manifest first so the operator namespace, service account, and packaged manager roles exist before the controller starts.

If you plan to install the operator through an OKE add-on, OperatorHub.io, or the Red Hat Ecosystem Catalog, follow the corresponding installation section below. These channels manage the operator Deployment and lifecycle, so do not also apply the repository YAML manifests.

### Default namespace-scoped deployment

Namespace scope is the default and recommended option. The operator watches only the namespaces listed in `WATCH_NAMESPACE`.

Generate the manifests from the complete watch list:

```bash
scripts/generate-namespace-install.sh \
  oracle-database-operator-system,<watched-namespace>
```

Apply RBAC before the system manifest:

```bash
kubectl apply -f dist/install/oracle-database-operator-rbac.yaml
kubectl apply -f dist/install/oracle-database-operator-system.yaml
```

The generator creates the Deployment value and namespace manager RoleBindings from the same namespace list. This prevents the watch list and RBAC from becoming inconsistent.

Also, the generator keeps the operator service account in `oracle-database-operator-system` and grants access only in the namespaces listed in `WATCH_NAMESPACE`.

Verify access separately in every watched namespace one at a time. For example:

```bash
for namespace in oracle-database-operator-system <watched-namespace>; do
  kubectl auth can-i list lrpdbs.database.oracle.com \
    --as=system:serviceaccount:oracle-database-operator-system:oracle-database-operator-controller-manager \
    -n "${namespace}"
done
```

Each check should return `yes`.

### Optional feature-specific RBAC for cluster-scoped resources

Namespace-scoped deployment controls which namespaces the operator watches and where namespace `RoleBinding` objects are created. A namespace `RoleBinding` does not grant access to cluster-scoped Kubernetes resources such as `PersistentVolume`, `StorageClass`, or `Node`. Cluster scope grants the permissions already present in the manager `ClusterRole` across the cluster.

In the packaged manager role, `namespaces`, `persistentvolumes`, and `storageclasses` are already included. `nodes` is not included by default and requires the optional NodePort RBAC when a selected feature needs node access.

Apply the following optional RBAC manifests only when the selected controller feature requires them:

| Feature | Resource | Manifest |
| --- | --- | --- |
| NodePort service connect strings | `nodes` | `rbac/node-rbac.yaml` |
| Block-volume storage expansion | `storageclasses.storage.k8s.io` | `rbac/storage-class-rbac.yaml` |
| SIDB custom scripts using existing PVs | `persistentvolumes` | `rbac/persistent-volume-rbac.yaml` |
| RAC ASM PV lifecycle | `persistentvolumes` | `docs/rac/rbac/pv-rbac.yaml` |

Review each manifest before applying it. Avoid granting cluster-wide access when the workload does not require it.

### Switch to cluster scope

A direct cluster-scoped installation is not available. To run the operator with cluster scope:

1. Complete the default namespace-scoped installation.
2. Verify that the operator is healthy.
3. Follow [Managing Operator Installation Scope](OPERATOR_SCOPE_MANAGEMENT.md#switching-to-a-cluster-scoped-installation) to add cluster-wide RBAC and update `WATCH_NAMESPACE`.

Do not recreate the existing ServiceAccount or replace the manager `ClusterRole`.

### Optional UID pinning for non-OpenShift clusters

The operator manifests do not pin the manager container to a fixed UID so that OpenShift Security Context Constraints can assign a namespace-valid UID automatically.

On non-OpenShift Kubernetes clusters, if your security policy requires the manager container to run as UID/GID `1002`, use the following option.

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

Do not apply this UID pinning on OpenShift. OpenShift assigns a valid UID from the namespace range when `runAsUser` is omitted.

## Install as an OKE add-on

On Oracle Kubernetes Engine (OKE), install and manage the operator as an OKE add-on through the OCI Console or compatible automation flow.

Use the [OKE add-on configuration documentation](https://docs.oracle.com/en-us/iaas/Content/ContEng/Tasks/configuration-arguments-db-operator-for-k8s.htm) for supported configuration options. Do not apply repository YAML scope procedures in addition to the add-on installation.

## Install from OperatorHub.io

Install the operator through the [Oracle Database Operator listing on OperatorHub.io](https://operatorhub.io/operator/oracle-database-operator).

Follow the installation instructions for your Kubernetes platform and use the same channel for subsequent lifecycle operations.

## Install on Red Hat OpenShift

For OpenShift, use the partner-validated listing in the [Red Hat Ecosystem Catalog](https://catalog.redhat.com/en/software/container-stacks/detail/68c4502d45d3fc6d301a9980).

Follow the OpenShift-specific instructions and do not apply the repository YAML installation manifests on top of the catalog-managed installation.

## Build and deploy a custom operator image

Prebuilt Oracle Database Operator images from Oracle Container Registry are recommended for normal installations because they are versioned and validated.

If a custom image is required, follow the [Oracle Database Operator Manual Build and Deployment Guide](OPERATOR_INSTALLATION_README.md) for build prerequisites, image tagging, registry handling, and deployment configuration.

## Verify Installation

Use these checks after installing the operator through any supported installation method.

### Verify the operator Deployment

```bash
# Confirm the Deployment rollout
kubectl rollout status deployment/oracle-database-operator-controller-manager \
  -n oracle-database-operator-system --timeout=300s

# Check replica readiness and pod placement
kubectl get deployment oracle-database-operator-controller-manager \
  -n oracle-database-operator-system
kubectl get pods -n oracle-database-operator-system -o wide
```

### Verify permissions

For namespace-scoped installations, verify access separately in every watched namespace:

```bash
# Verify permissions separately in every watched namespace
kubectl auth can-i list lrpdbs.database.oracle.com \
  --as=system:serviceaccount:oracle-database-operator-system:oracle-database-operator-controller-manager \
  -n <watched-namespace>
```

For cluster-scoped installations, repeat the check in a namespace outside `oracle-database-operator-system`, such as `default`.

Each check should return `yes`.

### Verify CRDs and webhooks

```bash
kubectl get crd

# Confirm a representative CRD is installed
kubectl get crd lrpdbs.database.oracle.com

# Confirm the admission webhooks exist
kubectl get mutatingwebhookconfiguration mutating-webhook-configuration
kubectl get validatingwebhookconfiguration validating-webhook-configuration
```

### Review logs and events

```bash
# Review recent controller logs
kubectl logs deployment/oracle-database-operator-controller-manager \
  -n oracle-database-operator-system -c manager --tail=100

# Review recent installation-namespace events when troubleshooting
kubectl get events -n oracle-database-operator-system --sort-by=.lastTimestamp
```

These checks validate the operator installation. They do not verify that a database or other workload is healthy; use the relevant controller guide for workload verification.

For namespace-scoped installations, verify the ServiceAccount in every watched namespace. For cluster-scoped installations, repeat the check in a namespace outside `oracle-database-operator-system`.

## Existing installations

Do not use this guide to make in-place scope changes to an existing installation. Use [Managing Operator Installation Scope](OPERATOR_SCOPE_MANAGEMENT.md), which explains how to update RoleBindings, change `WATCH_NAMESPACE`, and switch between namespace and cluster scope safely.
