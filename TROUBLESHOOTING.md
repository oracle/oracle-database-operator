# Oracle Database Operator Troubleshooting

Use this guide to collect the first set of diagnostics for Oracle Database Operator installation, webhook, certificate, RBAC, role binding, and custom resource reconciliation issues.

Replace the placeholders before running the commands:

```bash
OPERATOR_NS=oracle-database-operator-system
TARGET_NS=<namespace-managed-by-the-operator>
RESOURCE=<custom-resource-kind-or-short-name>
NAME=<custom-resource-name>
```

## Operator Health

Check whether the operator pods are running:

```bash
kubectl get pods -n "${OPERATOR_NS}" -o wide
```

Check the controller manager deployment:

```bash
kubectl describe deployment oracle-database-operator-controller-manager -n "${OPERATOR_NS}"
```

Check recent controller manager logs:

```bash
kubectl logs -n "${OPERATOR_NS}" deployment/oracle-database-operator-controller-manager --all-containers=true --tail=200
```

Follow controller manager logs while reproducing the issue:

```bash
kubectl logs -n "${OPERATOR_NS}" deployment/oracle-database-operator-controller-manager --all-containers=true -f
```

Check recent operator namespace events:

```bash
kubectl get events -n "${OPERATOR_NS}" --sort-by=.lastTimestamp
```

Check rollout status after an install, upgrade, or restart:

```bash
kubectl rollout status deployment/oracle-database-operator-controller-manager -n "${OPERATOR_NS}"
```

## CRD And API Checks

Verify that Oracle Database Operator CRDs are installed:

```bash
kubectl get crd | grep -E 'database.oracle.com|observability.oracle.com|privateai.oracle.com|network.oracle.com'
```

List API resources served by the database.oracle.com API group:

```bash
kubectl api-resources --api-group=database.oracle.com
```

Check a custom resource in the target namespace:

```bash
kubectl get "${RESOURCE}" -n "${TARGET_NS}"
```

Describe a custom resource to review status, conditions, and events:

```bash
kubectl describe "${RESOURCE}" "${NAME}" -n "${TARGET_NS}"
```

## Webhook And Certificate Checks

Verify admission webhook configurations:

```bash
kubectl get validatingwebhookconfiguration,mutatingwebhookconfiguration | grep oracle-database-operator
```

Describe the validating webhook configuration:

```bash
kubectl describe validatingwebhookconfiguration oracle-database-operator-validating-webhook-configuration
```

Describe the mutating webhook configuration:

```bash
kubectl describe mutatingwebhookconfiguration oracle-database-operator-mutating-webhook-configuration
```

Verify the webhook Service and endpoints:

```bash
kubectl get service,endpoints oracle-database-operator-webhook-service -n "${OPERATOR_NS}"
```

Check the serving certificate and generated TLS Secret:

```bash
kubectl get certificate oracle-database-operator-serving-cert -n "${OPERATOR_NS}"
kubectl get secret webhook-server-cert -n "${OPERATOR_NS}"
```

Describe the serving certificate if webhook calls fail with TLS or CA bundle errors:

```bash
kubectl describe certificate oracle-database-operator-serving-cert -n "${OPERATOR_NS}"
```

Check cert-manager status if certificate resources are not ready:

```bash
kubectl get pods -n cert-manager
kubectl get clusterissuer,issuer -A
```

## RBAC And Role Binding Checks

Verify what the operator service account is allowed to do before changing permissions:

```bash
kubectl auth can-i list lrpdbs.database.oracle.com --as=system:serviceaccount:${OPERATOR_NS}:oracle-database-operator-controller-manager -n "${TARGET_NS}"
```

Check whether the generated manager role exists:

```bash
kubectl get clusterrole oracle-database-operator-manager-role -o yaml
```

Check operator role bindings in the target namespace:

```bash
kubectl get rolebinding -n "${TARGET_NS}" -o wide | grep oracle-database-operator
```

Describe the manager role binding in the target namespace. Use the role binding name returned by the previous command:

```bash
kubectl describe rolebinding <rolebinding-name> -n "${TARGET_NS}"
```

For cluster-scoped installs, check the cluster role binding:

```bash
kubectl get clusterrolebinding -o wide | grep oracle-database-operator
```

## ServiceAccount Checks

The operator Deployment runs under a Kubernetes ServiceAccount. The ServiceAccount name in the Deployment must match the ServiceAccount created by the RBAC manifest. If these names do not match, the ReplicaSet may show `DESIRED` replicas but `CURRENT` pods as `0` because Kubernetes cannot create the pods.

Check the ServiceAccount used by the operator Deployment:

```bash
kubectl get deployment oracle-database-operator-controller-manager -n "${OPERATOR_NS}" -o jsonpath='{.spec.template.spec.serviceAccountName}{"\n"}'
```

Check that the ServiceAccount exists:

```bash
kubectl get serviceaccount oracle-database-operator-controller-manager -n "${OPERATOR_NS}"
```

Describe the ReplicaSet when the Deployment shows `0/3`, `0/1`, or no pods are created:

```bash
kubectl describe rs -n "${OPERATOR_NS}" -l control-plane=controller-manager
```

Look for events such as `serviceaccount "controller-manager" not found`. That means the Deployment points to a ServiceAccount name that does not exist in the operator namespace.

Check the ServiceAccount subjects referenced by operator RoleBindings and ClusterRoleBindings:

```bash
kubectl get rolebinding,clusterrolebinding -A -o yaml | grep -A5 -B5 'oracle-database-operator-controller-manager'
```

If the Deployment points to the wrong ServiceAccount, update it to use the ServiceAccount created by the operator RBAC manifest:

```bash
kubectl patch deployment oracle-database-operator-controller-manager -n "${OPERATOR_NS}" --type='merge' -p '{"spec":{"template":{"spec":{"serviceAccountName":"oracle-database-operator-controller-manager"}}}}'
```

Then wait for the rollout:

```bash
kubectl rollout status deployment/oracle-database-operator-controller-manager -n "${OPERATOR_NS}"
```

For manifest-based installations, prefer fixing the YAML source and reapplying it instead of only patching the live Deployment:

```bash
kubectl apply -f oracle-database-operator-rbac.yaml
kubectl apply -f oracle-database-operator-system.yaml
```

Security guidance: do not switch the operator to the `default` ServiceAccount or grant broad permissions to make pod creation succeed. Keep the operator on `oracle-database-operator-controller-manager` and bind only the roles required for the namespaces or cluster scope being used.

## ServiceAccount Token Automount Checks

The operator needs a Kubernetes API token to watch resources, update status, run leader election, serve admission webhooks, and reconcile custom resources. Kubernetes normally mounts this token automatically for the pod's ServiceAccount.

Check whether token automount is disabled on the operator Deployment:

```bash
kubectl get deployment oracle-database-operator-controller-manager -n "${OPERATOR_NS}" -o jsonpath='{.spec.template.spec.automountServiceAccountToken}{"\n"}'
```

Check whether token automount is disabled on the operator ServiceAccount:

```bash
kubectl get serviceaccount oracle-database-operator-controller-manager -n "${OPERATOR_NS}" -o jsonpath='{.automountServiceAccountToken}{"\n"}'
```

If either command returns `false`, the operator may start but fail to authenticate to the Kubernetes API. Check the controller logs for authentication, authorization, leader election, watch, or status update errors:

```bash
kubectl logs -n "${OPERATOR_NS}" deployment/oracle-database-operator-controller-manager --all-containers=true --tail=200
```

To restore the default Kubernetes behavior, remove the explicit pod setting from the Deployment manifest or set it to `true`:

```bash
kubectl patch deployment oracle-database-operator-controller-manager -n "${OPERATOR_NS}" --type='merge' -p '{"spec":{"template":{"spec":{"automountServiceAccountToken":true}}}}'
```

If the ServiceAccount explicitly disables token automount, set it back to `true`:

```bash
kubectl patch serviceaccount oracle-database-operator-controller-manager -n "${OPERATOR_NS}" --type='merge' -p '{"automountServiceAccountToken":true}'
```

Security guidance: disabling `automountServiceAccountToken` is useful for application pods that do not need Kubernetes API access. It is usually not appropriate for the operator controller manager because the controller must authenticate to the API server. If token exposure is a concern, use least-privilege RBAC, namespace-scoped installs, short-lived projected tokens where supported by the cluster, and network policies rather than removing the operator's API token.

## Namespace Scope Checks

Check the operator watch namespace configuration:

```bash
kubectl get deployment oracle-database-operator-controller-manager -n "${OPERATOR_NS}" -o yaml | grep -A5 WATCH_NAMESPACE
```

Check resources in the namespace that the operator is expected to watch:

```bash
kubectl get all -n "${TARGET_NS}"
```

## Remediation Commands

Restart the controller manager after correcting configuration, certificates, or RBAC:

```bash
kubectl rollout restart deployment/oracle-database-operator-controller-manager -n "${OPERATOR_NS}"
kubectl rollout status deployment/oracle-database-operator-controller-manager -n "${OPERATOR_NS}"
```

Security guidance: prefer granting the minimum role or role binding required for the namespace and controller being used. Avoid using `cluster-admin` as a troubleshooting shortcut except for temporary, tightly controlled validation in a non-production environment.
