# Quick Start: Oracle Database Observability with Prometheus and Grafana

This quick start shows the shortest path to a working observability setup for
`DatabaseObserver` on Kubernetes.

In this guide, `DatabaseObserver` is the Kubernetes custom resource, and the
operator uses it to deploy the Oracle AI Database Metrics Exporter (the
exporter).

By the end of this guide, you will:

1. Install the Prometheus and Grafana stack.
2. Configure Grafana with a Prometheus datasource.
3. Import the Oracle dashboard.
4. Create a `DatabaseObserver`.
5. Verify metrics, logs, and the Grafana UI.

## Example Environment

This example assumes:

1. The Oracle Database Operator is already installed.
2. A test namespace named `testcase` exists.
3. You have an Oracle Database reachable from the cluster.
4. You want to create a `DatabaseObserver` named `obs-quick`.
5. The database is reachable through a connect string such as `dbhost:1521/FREE`.

If your environment uses different names, update the commands and YAML
accordingly.

## Prerequisites

Make sure the following tools are available:

1. `kubectl`
2. `helm`
3. `curl`
4. `jq` for optional verification steps

This guide uses the Oracle dashboard published here:

- `https://raw.githubusercontent.com/oracle/oracle-db-appdev-monitoring/refs/heads/main/docker-compose/grafana/dashboards/oracle_rev3.json`

## 1. Install the Prometheus Stack

Install the Prometheus community chart and enable a longer startup timeout for
Prometheus.

```bash
helm repo add prometheus https://prometheus-community.github.io/helm-charts
helm repo update

kubectl get ns prometheus >/dev/null 2>&1 || kubectl create namespace prometheus

helm upgrade --install prometheus prometheus/kube-prometheus-stack \
  -n prometheus \
  --version 82.0.2 \
  --set prometheus.prometheusSpec.maximumStartupDurationSeconds=300 \
  --set grafana.adminPassword='<grafana-admin-password>'
```

Validate that the `ServiceMonitor` CRD is available:

```bash
kubectl api-resources | grep smon
```

## 2. Prepare a Monitoring User in the Database

Create a database user that the exporter can use for metrics queries.

Grant `SELECT_CATALOG_ROLE` and also explicitly grant access to
`SYS.V_$DIAG_ALERT_EXT`.

> Note: Bug **`38699234`** reports that `V_$DIAG_ALERT_EXT` is not granted through
> `SELECT_CATALOG_ROLE`, even though `SELECT_CATALOG_ROLE` should include this
> view. 

The exact command depends on how you access your database. The following SQL is
the minimum setup used by this quick start:

```sql
create user C##OBS identified by <db-password> container=all;
grant connect, SELECT_CATALOG_ROLE to C##OBS;
grant select on SYS.V_$DIAG_ALERT_EXT to C##OBS;
```

If your database is not a container database, adjust the username and `container=all`
clause to match your environment.

## 3. Create the Kubernetes Secret

Create a secret with the database credentials and connection string:

```bash
kubectl delete secret -n testcase obsdb-secret --ignore-not-found=true

kubectl create secret -n testcase generic obsdb-secret \
  --from-literal=username='C##OBS' \
  --from-literal=password='<db-password>' \
  --from-literal=connection='dbhost:1521/FREE'
```

## 4. Create the DatabaseObserver

Apply the following manifest. Update the namespace, secret name, or exporter
image if your environment uses different values:

```yaml
apiVersion: observability.oracle.com/v4
kind: DatabaseObserver
metadata:
  name: obs-quick
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
    image: "container-registry.oracle.com/database/observability-exporter:2.2.2"
  serviceMonitor:
    labels:
      release: prometheus
```

Save it as `obs-quick.yaml`, then apply it:

```bash
kubectl delete -f obs-quick.yaml --ignore-not-found=true
kubectl apply -f obs-quick.yaml
```

Wait for the observer pod and resource to become ready:

```bash
kubectl get databaseobserver -n testcase
kubectl get pods -n testcase -l app=obs-quick
kubectl describe databaseobserver -n testcase obs-quick
```

## 5. Connect to Grafana

Start a local port-forward to Grafana:

```bash
kubectl -n prometheus port-forward svc/prometheus-grafana 3080:80
```

Open:

```text
http://localhost:3080
```

Sign in with:

```text
username: admin
password: <grafana-admin-password>
```

## 6. Configure the Grafana Datasource

If the Prometheus datasource is not already present, add it manually in the
Grafana UI:

1. Go to `Connections` -> `Data sources`.
2. Click `Add data source`.
3. Choose `Prometheus`.
4. Set the URL to:

```text
http://prometheus-operated.prometheus.svc.cluster.local:9090
```

5. Click `Save & test`.

## 7. Import the Oracle Dashboard

Download the Oracle dashboard JSON:

```bash
curl -fsSL -o oracle_rev3.json \
  https://raw.githubusercontent.com/oracle/oracle-db-appdev-monitoring/refs/heads/main/docker-compose/grafana/dashboards/oracle_rev3.json
```

In Grafana:

1. Go to `Dashboards` -> `Import`.
2. Upload `oracle_rev3.json`.
3. When prompted, select the Prometheus datasource.
4. Finish the import.

## 8. Verify the Setup

In a separate terminal, start a local port-forward to Prometheus:

```bash
kubectl -n prometheus port-forward svc/prometheus-operated 9090:9090
```

Then run a simple Prometheus query locally and confirm that Oracle metrics are present:

```bash
curl -s -G http://localhost:9090/api/v1/query --data-urlencode 'query=oracledb_up' | jq
```

If the setup is working, the query returns a successful JSON response with at
least one metric result. The expected output should look similar to:

```json
{
  "status": "success",
  "data": {
    "resultType": "vector",
    "result": [
      {
        "metric": {
          "__name__": "oracledb_up",
          "database": "default",
          "endpoint": "metrics",
          "instance": "10.244.0.10:9161",
          "job": "obs-quick",
          "namespace": "testcase",
          "pod": "obs-quick-xxxxxxxxx-xxxxx",
          "service": "obs-quick"
        },
        "value": [
          1776171582.115,
          "1"
        ]
      }
    ]
  }
}
```

The key detail is that `oracledb_up` is present and its value is `"1"`.

## Cleanup

Remove the observer resources:

```bash
kubectl delete databaseobserver -n testcase obs-quick --ignore-not-found=true
kubectl delete secret -n testcase obsdb-secret --ignore-not-found=true
```

Remove the monitoring stack if you no longer need it:

```bash
helm uninstall prometheus -n prometheus
kubectl delete namespace prometheus --ignore-not-found=true
```
