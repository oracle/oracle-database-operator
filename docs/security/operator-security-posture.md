# Oracle Database Operator Security Posture

This page summarizes the default security posture of the Oracle Database Operator and the settings users should review before changing operator identity, RBAC, ServiceAccount token behavior, or pod security settings.

## Operator Identity

The operator controller manager runs as a dedicated Kubernetes ServiceAccount:

```text
system:serviceaccount:oracle-database-operator-system:oracle-database-operator-controller-manager
```

Keep the operator Deployment `serviceAccountName` aligned with this ServiceAccount unless you intentionally create and bind a replacement ServiceAccount with equivalent least-privilege permissions. Do not switch the operator to the namespace `default` ServiceAccount.

## ServiceAccount Token

The controller manager needs a Kubernetes API token to watch resources, update status, run leader election, serve admission webhooks, and reconcile custom resources. Kubernetes normally mounts this token through the pod ServiceAccount.

Do not set `automountServiceAccountToken: false` for the operator controller manager unless you provide another supported way for the controller to authenticate to the Kubernetes API. If token exposure is a concern, prefer least-privilege RBAC, namespace-scoped installs, and network policies over disabling the operator token.

## Namespace Scope

`WATCH_NAMESPACE` controls which namespaces the operator watches. For namespace-scoped installs, every namespace listed in `WATCH_NAMESPACE` must have a matching `RoleBinding` that grants the operator ServiceAccount access through the packaged manager `ClusterRole`.

Use the namespace install generator to keep `WATCH_NAMESPACE` and namespace RoleBindings aligned:

```sh
scripts/generate-namespace-install.sh oracle-database-operator-system,shns
```

Then verify access before creating custom resources:

```sh
kubectl auth can-i list lrpdbs.database.oracle.com \
  --as=system:serviceaccount:oracle-database-operator-system:oracle-database-operator-controller-manager \
  -n shns
```

## Roles And Bindings

RBAC uses separate objects for permissions and assignment:

- `Role` grants namespaced permissions.
- `ClusterRole` grants reusable permissions and can include cluster-scoped resources.
- `RoleBinding` grants a `Role` or `ClusterRole` inside one namespace.
- `ClusterRoleBinding` grants a `ClusterRole` across the cluster.

For namespace-scoped installs, prefer namespace `RoleBinding` objects over broad `ClusterRoleBinding` objects. Avoid using `cluster-admin` as an installation or troubleshooting shortcut.

## Optional Cluster-Scoped Permissions

Some Kubernetes resources are cluster-scoped and are not controlled by `WATCH_NAMESPACE`. Apply these optional RBAC manifests only when the selected controller feature requires them:

| Capability | Resource | Manifest |
| --- | --- | --- |
| NodePort service connect strings | `nodes` | `rbac/node-rbac.yaml` |
| Storage expansion for block volumes | `storageclasses.storage.k8s.io` | `rbac/storage-class-rbac.yaml` |
| SIDB custom scripts with existing PersistentVolumes | `persistentvolumes` read access | `rbac/persistent-volume-rbac.yaml` |
| RAC ASM PersistentVolume lifecycle | `persistentvolumes` create/delete access | `docs/rac/rbac/pv-rbac.yaml` |

Review optional cluster-scoped RBAC before applying it. PersistentVolume permissions are especially sensitive because `PersistentVolume` objects are cluster-scoped.

## Pod Security Context

The controller manager manifest uses a restricted pod posture:

- runs as a non-root user
- sets `allowPrivilegeEscalation: false`
- drops Linux capabilities with `capabilities.drop: ["ALL"]`
- uses `seccompProfile.type: RuntimeDefault`
- mounts metrics and webhook TLS certificates as read-only volumes

Keep these settings unless a platform-specific requirement has been reviewed and documented.

## Secure Configuration Guidance

Recommended:

- use namespace-scoped installs when the operator should manage only selected namespaces
- generate namespace-scoped install manifests instead of manually editing only `WATCH_NAMESPACE`
- apply optional cluster-scoped RBAC only when a selected feature requires it
- verify access with `kubectl auth can-i`
- keep the operator on the dedicated `oracle-database-operator-controller-manager` ServiceAccount

Avoid:

- binding the operator to `cluster-admin`
- binding operator permissions to the `default` ServiceAccount
- disabling ServiceAccount token automount for the controller manager
- granting PersistentVolume create/delete access unless RAC or Oracle Restart style PV lifecycle management requires it
