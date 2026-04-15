# OpenShift Example: Observability with User Workload Monitoring

This page provides a tested example of running Oracle Database observability on
OpenShift.

It is an example, not a universal OpenShift manual. Adapt the namespace names,
ServiceAccount names, labels, RBAC, and monitoring conventions to match your
cluster standards.

## Tested Setup

This example was validated with the following environment:

- Observability Controller `2.1`
- OpenShift `v1.31.7`
- Oracle AI Database Metrics Exporter image `container-registry.oracle.com/database/observability-exporter:2.2.0`

The validation covered these use cases:

1. Create a `DatabaseObserver`.
2. Check exporter logs.
3. Query Prometheus from the command line after enabling User Workload
   Monitoring (UWM).

## Overview

OpenShift includes cluster monitoring out of the box, but application metrics
are not scraped automatically. For the `DatabaseObserver` ServiceMonitor to be
picked up by OpenShift monitoring, enable User Workload Monitoring (UWM).

This example uses the following flow:

1. Enable User Workload Monitoring.
2. Optionally grant permissions for querying metrics from a pod.
3. Create a `DatabaseObserver`.
4. Query metrics through the Thanos Querier.

## Example Values Used Below

The commands and YAML in this page use example values:

- Namespace: `testcase`
- `DatabaseObserver` name: `obs`
- ServiceAccount used for metric queries: `metrics-query-sa`
- Example ServiceMonitor label: `release: prometheus`

Change these values to match your environment.

## 1. Enable User Workload Monitoring

If your organization already manages UWM centrally, do not patch cluster
monitoring settings again. Reuse the existing configuration and align the
ServiceMonitor labels with your cluster conventions.

This is a one-time cluster configuration and typically requires cluster-admin
privileges. It causes OpenShift to create the user workload monitoring
Prometheus components in the `openshift-user-workload-monitoring` namespace.

```bash
kubectl patch configmap cluster-monitoring-config \
  -n openshift-monitoring \
  --type merge \
  -p '{"data": {"config.yaml": "enableUserWorkload: true"}}'
```

After applying the patch, verify that the user workload monitoring pods are
running:

```bash
kubectl get pods -n openshift-user-workload-monitoring
```

## 2. Grant Permissions for Metric Queries

This step is only needed if you want to query OpenShift metrics from within a
pod by using a ServiceAccount token. It is not required for creating the
`DatabaseObserver` itself.

The following example grants the `cluster-monitoring-view` role to a
ServiceAccount named `metrics-query-sa` in the `testcase` namespace:

```bash
kubectl create clusterrolebinding uwm-view-binding \
  --clusterrole=cluster-monitoring-view \
  --serviceaccount=testcase:metrics-query-sa
```

If your environment uses a different namespace or ServiceAccount, update the
command accordingly.

## 3. Create the DatabaseObserver

Before applying the manifest below, create the database secret referenced as
`obsdb-secret`. For an example of creating that secret, see [Create the
Kubernetes Secret](./quickstart.md#3-create-the-kubernetes-secret) in the quick
start.

Apply a `DatabaseObserver` manifest similar to the following example:

```yaml
apiVersion: observability.oracle.com/v4
kind: DatabaseObserver
metadata:
  name: obs
  namespace: testcase
spec:
  database:
    dbUser:
      secret: obsdb-secret
    dbPassword:
      secret: obsdb-secret
    dbConnectionString:
      secret: obsdb-secret
  deployment:
    image: "container-registry.oracle.com/database/observability-exporter:2.2.0"
  serviceMonitor:
    labels:
      release: prometheus
```

About this example:

- The `serviceMonitor` section creates a ServiceMonitor that OpenShift User
  Workload Monitoring can discover.
- The labels shown above are example values only. Use labels that match your
  monitoring and namespace conventions.
- Depending on your OpenShift monitoring configuration, your cluster may expect
  a different label set or namespace selection policy.

Apply the manifest and verify that the observer pod is running:

```bash
kubectl apply -f databaseobserver-openshift.yaml
kubectl get databaseobserver -n testcase
kubectl get pods -n testcase -l app=obs
```

## 4. Query the Metrics

The Thanos Querier is a practical endpoint for validation because it provides a
single query interface over cluster and user workload metrics.

When querying from inside a pod, use the internal service address and the
ServiceAccount token for authentication:

```bash
PROMETHEUS_SVC="thanos-querier.openshift-monitoring.svc:9091"
PQUERY='oracledb_sessions_value{status="ACTIVE", database="default", type="USER"}'

kubectl exec -n testcase "$QUERY_POD" -- bash -c "
  TOKEN=\$(cat /var/run/secrets/kubernetes.io/serviceaccount/token)
  curl -s -k -H \"Authorization: Bearer \$TOKEN\" \
  -G \"https://$PROMETHEUS_SVC/api/v1/query\" \
  --data-urlencode \"query=$PQUERY\"
"
```

Replace `QUERY_POD` with the name of a pod that runs with the ServiceAccount to
which you granted access.

If the setup is working, the query returns Oracle database metrics scraped from
the exporter endpoint created for the `DatabaseObserver`.

## Notes

- OpenShift monitoring behavior, label selection, and RBAC vary between
  environments. Treat this page as a starting point, not a drop-in recipe.
- For the generic Kubernetes flow, see the [Quick Start](./quickstart.md).
