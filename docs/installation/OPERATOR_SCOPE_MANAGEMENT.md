# Managing Oracle Database Operator Installation Scope

This guide explains how to change the scope of an existing Oracle Database Operator installation. It covers updating the watched namespaces and switching an existing namespace-scoped installation to cluster scope.

> [!CAUTION]
>
> These procedures modify the operator Deployment and RBAC. Follow the order of operations exactly and verify permissions before changing `WATCH_NAMESPACE`.

## Before you begin

- Confirm that `kubectl` is connected to the intended cluster.
- Back up or record the current operator Deployment and RBAC configuration.
- Use manifests and resource names from the same operator version as the running installation.

These commands use the standard YAML installation names. They discover the
ServiceAccount from the running Deployment; when that field is omitted,
Kubernetes uses the `default` ServiceAccount in the operator namespace.

```bash
OPERATOR_NS=oracle-database-operator-system
DEPLOYMENT=oracle-database-operator-controller-manager
SERVICE_ACCOUNT="$(kubectl get deployment "${DEPLOYMENT}" -n "${OPERATOR_NS}" \
  -o jsonpath='{.spec.template.spec.serviceAccountName}')"
SERVICE_ACCOUNT="${SERVICE_ACCOUNT:-default}"
MANAGER_ROLE=oracle-database-operator-manager-role
MANAGER_ROLE_BINDING=oracle-database-operator-oracle-database-operator-manager-rolebinding
```

If your installation uses different resource names, replace the variables with
the values from the running Deployment and its manager RBAC bindings. Inspect
them before changing scope:

```bash
kubectl get deployment "${DEPLOYMENT}" -n "${OPERATOR_NS}" \
  -o yaml

kubectl get rolebinding "${MANAGER_ROLE_BINDING}" \
  -n "${OPERATOR_NS}" -o yaml

kubectl get clusterrolebinding "${MANAGER_ROLE_BINDING}" -o yaml \
  2>/dev/null || true
```

## Updating an Existing Namespace-Scoped Installation

> **Important:**
>
> Do not update only `WATCH_NAMESPACE`. Every namespace in the watch list must also have a manager `RoleBinding` for the existing operator ServiceAccount. The ServiceAccount itself does not change.

The operator installation namespace is normally included in the watch list. Additional namespaces require their own manager `RoleBinding`.

## Adding Namespaces to the Watch List

Create the manager `RoleBinding` in every namespace that will be watched. If a RoleBinding with the same name already exists, confirm that it references the expected manager `ClusterRole` before applying this command. A RoleBinding's `roleRef` cannot be changed in place.

```bash
for namespace in shns db-workloads; do
  kubectl create rolebinding "${MANAGER_ROLE_BINDING}" \
    --clusterrole="${MANAGER_ROLE}" \
    --serviceaccount="${OPERATOR_NS}:${SERVICE_ACCOUNT}" \
    --namespace="${namespace}" \
    --dry-run=client -o yaml | kubectl apply -f -
done
```

The RoleBinding references the existing manager `ClusterRole` and the existing ServiceAccount. Do not create a second ServiceAccount for the additional namespaces.

## Verify namespace permissions

Verify access separately in every namespace before changing the Deployment:

```bash
for namespace in shns db-workloads; do
  kubectl auth can-i list singleinstancedatabases.database.oracle.com \
    --as="system:serviceaccount:${OPERATOR_NS}:${SERVICE_ACCOUNT}" \
    --namespace="${namespace}"
done
```

`kubectl auth can-i -n` does not accept a comma-separated namespace list. A successful check should return `yes` for each namespace.

You can also verify other resources required by your installation, for example:

```bash
kubectl auth can-i list lrpdbs.database.oracle.com \
  --as="system:serviceaccount:${OPERATOR_NS}:${SERVICE_ACCOUNT}" \
  --namespace="<watched-namespace>"
```

## Updating `WATCH_NAMESPACE`

After the RoleBindings and permission checks succeed, set the complete watch list. This replaces the previous value and triggers a rollout:

```bash
kubectl set env deployment/"${DEPLOYMENT}" \
  -n "${OPERATOR_NS}" \
  WATCH_NAMESPACE="${OPERATOR_NS},shns,db-workloads"

kubectl rollout status deployment/"${DEPLOYMENT}" \
  -n "${OPERATOR_NS}" --timeout=300s
```

Verify that the Deployment contains the intended value:

```bash
kubectl get deployment/"${DEPLOYMENT}" \
  -n "${OPERATOR_NS}" \
  -o jsonpath='{.spec.template.spec.containers[?(@.name=="manager")].env[?(@.name=="WATCH_NAMESPACE")].value}{"\n"}'
```

## Removing Namespaces from the Watch List

First remove the namespace from `WATCH_NAMESPACE` and wait for the rollout. Then remove its obsolete manager `RoleBinding`:

```bash
kubectl set env deployment/"${DEPLOYMENT}" \
  -n "${OPERATOR_NS}" \
  WATCH_NAMESPACE="${OPERATOR_NS},shns"

kubectl rollout status deployment/"${DEPLOYMENT}" \
  -n "${OPERATOR_NS}" --timeout=300s

kubectl delete rolebinding "${MANAGER_ROLE_BINDING}" \
  -n db-workloads --ignore-not-found=true
```

Remove a RoleBinding only after the operator has stopped watching that namespace. Review any remaining database resources in the namespace before removing access.

## Switching to a Cluster-Scoped Installation

> [!WARNING]
> Cluster-scoped operation grants the operator wider access across the cluster. Namespace-scoped operation is preferred because it follows least-privilege practice and limits access to selected namespaces.

To switch an existing operator from namespace scope to cluster scope, first ensure that the namespace-scoped installation is healthy. Then add the cluster binding before unsetting `WATCH_NAMESPACE`.

The existing installation already provides the manager `ClusterRole` and operator ServiceAccount. Do not recreate the ServiceAccount or replace the manager `ClusterRole`.

Create or update the manager cluster binding for the ServiceAccount that is
already used by the Deployment:

```bash
kubectl create clusterrolebinding "${MANAGER_ROLE_BINDING}" \
  --clusterrole="${MANAGER_ROLE}" \
  --serviceaccount="${OPERATOR_NS}:${SERVICE_ACCOUNT}" \
  --dry-run=client -o yaml | kubectl apply -f -
```

Unset `WATCH_NAMESPACE` and wait for the rollout:

```bash
kubectl set env deployment/"${DEPLOYMENT}" \
  -n "${OPERATOR_NS}" \
  WATCH_NAMESPACE-

kubectl rollout status deployment/"${DEPLOYMENT}" \
  -n "${OPERATOR_NS}" \
  --timeout=300s
```

Keep the leader-election `RoleBinding` in `oracle-database-operator-system`. The previous namespace-scoped manager `RoleBinding` may remain, but it is redundant once the `ClusterRoleBinding` is active.

## Verifying Cluster-Scoped Access

Verify that the existing ServiceAccount can manage resources outside the operator installation namespace:

```bash
kubectl auth can-i list singleinstancedatabases.database.oracle.com \
  --as="system:serviceaccount:${OPERATOR_NS}:${SERVICE_ACCOUNT}" \
  -n default

kubectl get pods -n "${OPERATOR_NS}"

kubectl logs -n "${OPERATOR_NS}" \
  deployment/"${DEPLOYMENT}" -c manager \
  | grep 'CLUSTER SCOPED'
```

The permission check should return `yes`, the operator pod should be running, and the logs should confirm cluster-scoped operation.

## Fresh Installation Versus Existing Installation

This guide changes an existing installation. For a fresh YAML installation,
follow the [default namespace-scoped deployment instructions](./INSTALLATION_METHODS.md#default-namespace-scoped-deployment).
For a version upgrade, first follow
[Upgrade the Oracle Database Operator](./UPGRADE.md), then make a deliberate
scope change with this guide if required.
