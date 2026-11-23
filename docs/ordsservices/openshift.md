# OpenShift Deployment

When using the `OrdsSrvs` controller on OpenShift, configure Security Context
Constraints (SCCs) for the ServiceAccount specified in the `OrdsSrvs` custom
resource. The controller assigns this ServiceAccount to the Pods it creates.

## Create a Service Account

Create the ServiceAccount to reference in the `OrdsSrvs` custom resource:

```yaml
apiVersion: v1
kind: ServiceAccount
metadata:
  name: ordssrvs-sa
  namespace: ordsnamespace
```

Save the manifest as `ordssrvs-sa.yaml`, or download
[ordssrvs-sa.yaml](./examples/ordssrvs-sa.yaml), then apply it:

```bash
kubectl apply -f ordssrvs-sa.yaml
```

## Create a Custom Security Context Constraint

Use [ordssrvs-sa-scc.yaml](./examples/ordssrvs-sa-scc.yaml) to create the
required SCC and bind it to the `ordssrvs-sa` ServiceAccount. This allows the
Pods created by the `OrdsSrvs` controller to run under that SCC. The operator
controller uses its own ServiceAccount. Download the linked SCC manifest and
apply it:

```bash
kubectl apply -f ordssrvs-sa-scc.yaml
```

## Set `serviceAccountName` in OrdsSrvs

Merge `spec.serviceAccountName` into your `OrdsSrvs` custom resource. This is a
partial manifest; retain your existing image, connection, and credential settings:

```yaml
apiVersion: database.oracle.com/v4
kind: OrdsSrvs
metadata:
  name: ordssrvs
  namespace: ordsnamespace
spec:
  ...
  globalSettings:
    ...
  poolSettings:
    ...
  serviceAccountName: ordssrvs-sa
```

A complete manifest is also available as [ordssrvs.yaml](./examples/ordssrvs.yaml).
