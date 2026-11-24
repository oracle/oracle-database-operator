# Upgrade the Oracle Database Operator

This guide describes how to upgrade an existing Oracle Database Operator installation to a supported release. Use the same target release for the CRDs, RBAC, webhooks, controller image, samples, and documentation.

Do not combine installation channels. Upgrade a YAML installation with the repository manifests, an OKE installation with the OKE add-on, an OperatorHub.io installation with OLM, and an OpenShift installation with the Red Hat catalog.

## Before you upgrade

1. Review the [supported releases](../../README.md#supported-releases) and the target release notes.
2. Confirm that the target release supports your Kubernetes platform, cert-manager version, database images, and controller features.
3. Confirm that the operator is healthy before starting the upgrade.
4. Record the current watched namespaces and keep the existing operator namespace unchanged.
5. Back up important custom resources and record any installation-specific image, RBAC, or security-context changes.

Check the current installation:

```bash
kubectl get deployment oracle-database-operator-controller-manager \
  -n oracle-database-operator-system
kubectl get pods -n oracle-database-operator-system
kubectl get crd | grep -E \
  'database.oracle.com|observability.oracle.com|privateai.oracle.com|network.oracle.com'
```

The Deployment should have available replicas and the operator pods should be `Running` and `Ready` before you continue.

## Upgrade a YAML installation

Run the following commands from the root of the repository at the target operator release. Do not run them from an older checkout.

### 1. Apply the target release CRDs

Apply the repository CRD kustomization:

```bash
kubectl apply -k config/crd
```

This updates the API definitions before the new controller starts. Do not delete the CRDs during an upgrade; deleting a CRD can delete the custom resources stored under that API.

Confirm that the CRDs are established:

```bash
kubectl get crd | grep -E \
  'database.oracle.com|observability.oracle.com|privateai.oracle.com|network.oracle.com'
```

### 2. Generate manifests with the existing watch list

Read the current `WATCH_NAMESPACE` value from the running Deployment before generating new manifests:

```bash
kubectl get deployment oracle-database-operator-controller-manager \
  -n oracle-database-operator-system \
  -o jsonpath='{range .spec.template.spec.containers[?(@.name=="manager")].env[?(@.name=="WATCH_NAMESPACE")]}{.value}{"\n"}{end}'
```

The command prints the current comma-separated namespace list. Save that list and reuse the same namespaces during the upgrade. An empty value indicates cluster-scoped operation; do not use the namespace-scoped generator in that case. Follow [Managing Operator Installation Scope](./OPERATOR_SCOPE_MANAGEMENT.md) for cluster-scoped installations.

For a namespace-scoped installation, include the operator namespace and every existing watched namespace exactly as reported by the command. For example, if the command returns `oracle-database-operator-system,application`, run:

```bash
scripts/generate-namespace-install.sh \
  oracle-database-operator-system,application
```

For multiple watched namespaces, include every namespace in the same comma-separated argument. Do not omit an existing namespace or use this step to change installation scope. If the watch list must change, complete the upgrade first and then follow [Managing Operator Installation Scope](./OPERATOR_SCOPE_MANAGEMENT.md) separately.

### 3. Apply RBAC and the operator system manifest

Apply RBAC first, followed by the generated system manifest:

```bash
kubectl apply -f dist/install/oracle-database-operator-rbac.yaml
kubectl apply -f dist/install/oracle-database-operator-system.yaml
```

The generated system manifest contains the target-release CRDs, admission
webhooks, Services, certificates, NetworkPolicy, manager configuration, and
controller Deployment. Applying it updates the Deployment to the target
release image. Kubernetes rolls out new pods when the Deployment pod template
changes, such as when the image or `WATCH_NAMESPACE` changes. If the pod
template is unchanged, applying the manifest does not force a new rollout. Do
not manually set an image from a different operator release.

### Exceptional image-only update

Use an image-only update only for a custom-image deployment or a controlled
recovery procedure. Do not run it after a normal manifest upgrade. The
preferred upgrade path is applying the complete target-release manifests
because the controller image must remain aligned with the CRDs, RBAC, admission
and compatibility webhooks, NetworkPolicy, Services, manager configuration,
and controller samples.

```bash
kubectl -n oracle-database-operator-system \
  set image deployment/oracle-database-operator-controller-manager \
  manager=<registry>/<repository>/controller:<target-release-tag>
```

Replace the image with the exact image built and approved for the target release. This command does not update CRDs or any other operator resources.

### 4. Wait for the rollout

```bash
kubectl rollout status \
  deployment/oracle-database-operator-controller-manager \
  -n oracle-database-operator-system \
  --timeout=300s
```

Expected result:

```text
deployment "oracle-database-operator-controller-manager" successfully rolled out
```

## Upgrade through another installation channel

Use the same channel that installed the operator:

| Installation channel | Upgrade action |
| --- | --- |
| OKE add-on | Upgrade the Oracle Database Operator add-on through the OCI Console or the OKE add-on automation used by your organization. |
| OperatorHub.io | Approve or configure the target CSV upgrade through OLM. Do not apply repository YAML manifests. |
| Red Hat OpenShift catalog | Upgrade the Subscription or Operator through the OpenShift catalog. Do not apply repository YAML manifests. |
| Custom image | Build and publish the target image, then update the installation using the same manifest and security settings used for the custom deployment. |

After a channel-managed upgrade, use the validation steps below. Follow that channel's documentation for approval, subscription, and rollback behavior.

## Validate the upgraded installation

Run every check in this section. Do not consider the upgrade complete until each check succeeds.

### 1. Verify the Deployment rollout

```bash
kubectl rollout status \
  deployment/oracle-database-operator-controller-manager \
  -n oracle-database-operator-system \
  --timeout=300s
```

Expected result:

```text
deployment "oracle-database-operator-controller-manager" successfully rolled out
```

### 2. Verify the running image and pod readiness

```bash
kubectl get deployment oracle-database-operator-controller-manager \
  -n oracle-database-operator-system \
  -o jsonpath='{.spec.template.spec.containers[?(@.name=="manager")].image}{"\n"}'

kubectl get pods -n oracle-database-operator-system -o wide
```

The image must be the approved target image, and every operator pod must be `Running` and `Ready`.

### 3. Verify CRDs and webhook configurations

```bash
kubectl get crd | grep -E \
  'database.oracle.com|observability.oracle.com|privateai.oracle.com|network.oracle.com'
kubectl get mutatingwebhookconfiguration
kubectl get validatingwebhookconfiguration
```

The target CRDs must be present and the mutating and validating webhook configurations must exist.

### 4. Verify access in every watched namespace

Use the exact namespace list reported in step 2. For example, if
`WATCH_NAMESPACE` is `oracle-database-operator-system,default`, run:

```bash
for namespace in oracle-database-operator-system default; do
  kubectl auth can-i list lrpdbs.database.oracle.com \
    --as=system:serviceaccount:oracle-database-operator-system:oracle-database-operator-controller-manager \
    -n "${namespace}"
done
```

Replace the example namespaces with every namespace in the existing
`WATCH_NAMESPACE` value. Do not leave out the operator namespace or any
additional watched namespace. Every check should return `yes`.

The command uses the standard operator ServiceAccount. If the existing
Deployment uses a different `serviceAccountName`, replace the ServiceAccount
name in `--as` with that value. For a cluster-scoped installation, skip this
namespace loop and verify the manager `ClusterRoleBinding` instead.

### 5. Verify operator logs

```bash
kubectl logs deployment/oracle-database-operator-controller-manager \
  -n oracle-database-operator-system -c manager --tail=100
```

Look for successful startup and reconciliation. Investigate webhook, RBAC, conversion, validation, or image-pull errors before continuing.

### 6. Validate existing workloads

```bash
kubectl get singleinstancedatabases.database.oracle.com \
  -A -o wide
kubectl get dataguardbrokers.database.oracle.com \
  -A -o wide
kubectl get events -n oracle-database-operator-system --sort-by=.lastTimestamp
```

For each controller, follow its controller guide and confirm that:

- Existing custom resources remain readable.
- Status reconciliation resumes.
- No unexpected validation or conversion errors appear.
- Expected finalizers remain present.
- Child Deployments, Services, Secrets, and PVCs remain healthy.
- Operator logs contain no webhook or RBAC errors.

## Upgrade cautions

- Back up important custom resources before upgrading.
- Apply CRDs before updating the controller Deployment.
- Do not delete CRDs as part of a normal upgrade.
- Do not use `kubectl replace --force` for CRDs or the operator Deployment.
- Do not mix YAML, OKE add-on, OperatorHub.io, and OpenShift installation channels.
- Do not change `WATCH_NAMESPACE` during a version upgrade unless you are intentionally changing scope.
- For namespace-scoped installations, preserve the existing watched namespace list and RoleBindings.
- Preserve the existing operator namespace, ServiceAccount, and security settings unless the target release documentation requires a change.
- Review the target release notes for deprecated fields, conversion changes, and controller-specific migration notes.
- Test the target release in a non-production cluster when possible.
- If the rollout fails, inspect the Deployment events and operator logs before taking corrective action. Follow the installation channel's rollback procedure rather than deleting the operator resources.
