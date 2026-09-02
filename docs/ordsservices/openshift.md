# OpenShift Deployment

If you are deploying the OrdsSrvs controller on OpenShift, configure the
appropriate Security Context Constraints (SCCs). Assign the custom SCCs to the
service accounts used by OrdsSrvs to permit the required operations.

## Create a Service Account

Create the service account used to assign the required SCCs to the OrdsSrvs
controller:

```yaml
apiVersion: v1
kind: ServiceAccount
metadata:
  name: ordssrvs-sa
  namespace: ordsnamespace
```

The same manifest is available as [ordssrvs-sa.yaml](./examples/ordssrvs-sa.yaml).

## Create a Custom Security Context Constraint

Use [ordssrvs-sa-scc.yaml](./examples/ordssrvs-sa-scc.yaml) to create the
required SCC and bind it to the `ordssrvs-sa` service account. This gives the
OrdsSrvs controller the permissions it needs to operate on OpenShift.

## Set `serviceAccountName` in OrdsSrvs

Set `serviceAccountName` to the dedicated service account in the OrdsSrvs
resource:

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

See the complete [OrdsSrvs example](./examples/ordssrvs.yaml).
