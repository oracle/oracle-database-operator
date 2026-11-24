# Oracle AI Database Operator Observability

## Contents

- [Prerequisites](#prerequisites)
- [Quick Start](#quick-start)
- [Architecture overview](#architecture-overview)
- [Advanced configuration](#advanced-configuration)
  - [Choose a backend path](#choose-a-backend-path)
  - [Grafana dashboard](#grafana-dashboard)
  - [Backend installation reference](#backend-installation-reference)
  - [1. Create user values](#1-create-user-values)
  - [2. Configure an HTTP or HTTPS proxy](#2-configure-an-http-or-https-proxy)
  - [3. Monitor multiple namespaces](#3-monitor-multiple-namespaces)
  - [4. Resolve, render, and inspect the chart](#4-resolve-render-and-inspect-the-chart)
  - [5. Install Alloy](#5-install-alloy)
  - [6. Verify Alloy readiness and generated configuration](#6-verify-alloy-readiness-and-generated-configuration)
  - [7. Required RBAC and security details](#7-required-rbac-and-security-details)
  - [8. Connect to Grafana](#8-connect-to-grafana)
  - [9. Verify the complete flow](#9-verify-the-complete-flow)
  - [10. Upgrade and uninstall](#10-upgrade-and-uninstall)
  - [Alloy dependency and existing collectors](#alloy-dependency-and-existing-collectors)
- [Production guidance summary](#production-guidance-summary)

This chart installs Grafana Alloy as a separate telemetry collector for the
Oracle AI Database Operator.

The chart supports:

- Controller-manager metrics from the operator HTTPS `/metrics` Service.
- Controller-manager stdout/stderr logs through the Kubernetes API.
- Optional workload stdout/stderr logs from configured Kubernetes namespaces.
- Optional Kubernetes container CPU and memory metrics through cAdvisor.
- Metrics forwarding to a Prometheus-compatible remote-write endpoint.
- Logs forwarding to a Loki push endpoint.

Prometheus, Loki, and Grafana are not installed or managed by this chart.

The Oracle AI Database Operator remains independent of this observability chart.
The operator exposes telemetry, while Grafana Alloy discovers, collects, labels,
and forwards that telemetry to the configured customer observability backends.

This Alloy chart collects **operator-level** controller metrics and logs. It
does not create a `DatabaseObserver` or collect database-level exporter metrics.
For `DatabaseObserver` database monitoring, use the separate
[DatabaseObserver quick start](../../docs/observability/quickstart.md). The two
approaches can share a monitoring backend but are configured independently.

## Prerequisites

### Required client tools

The machine or CI job running Helm needs the following tools. If a tool is not
already installed, use its official installation guide to download and install
the latest release:

- [`helm`](https://helm.sh/docs/intro/install/) version 3.12 or newer. Verify with:

  ```bash
  helm version
  ```

- [`kubectl`](https://kubernetes.io/docs/tasks/tools/) configured for the target cluster:

  ```bash
  kubectl version --client
  kubectl cluster-info
  ```

- `rg` is optional and is used only by examples that inspect rendered YAML.

The Kubernetes cluster does not need the Helm binary. Helm runs on the
administrator's machine or in CI and submits Kubernetes resources through the
Kubernetes API.

### Required Kubernetes components

The following are required for operator telemetry:

- An existing Oracle AI Database Operator installation.
- An operator controller-manager Service named by
  `operator.metrics.serviceName` in every namespace where operator metrics are
  expected.
- The operator metrics endpoint must be reachable from the Alloy Pod.
- The metrics Service must expose the port named by
  `operator.metrics.servicePortName`.
- Kubernetes API access for Alloy to discover Pods and read `pods/log` when log
  collection is enabled.

The default operator values are:

```yaml
operator:
  namespaces:
    - oracle-database-operator-system

  metrics:
    serviceName: oracle-database-operator-controller-manager-metrics-service
    servicePortName: https
```

Optional workload logging additionally requires Kubernetes API access to the
configured workload namespaces.

Optional cAdvisor collection additionally requires read access to Kubernetes
Nodes and access to `nodes/proxy`.

### Prometheus and Loki requirements

This chart does not install Prometheus, Loki, or Grafana.

Provide the correct endpoints for each enabled telemetry destination:

- Prometheus or a compatible backend:
  a remote-write endpoint, commonly ending in `/api/v1/write`.
- Loki:
  a push endpoint, commonly ending in `/loki/api/v1/push`.
- Grafana:
  a separately installed UI with Prometheus and Loki data sources.

The destination URLs must be reachable from the Alloy Pod.

A successful Helm install proves that the Kubernetes resources were created,
but it does not prove that the destination backends accept telemetry. Verify the
complete telemetry flow after installation.

For Alloy Helm dependency resolution (online, offline/air-gapped, or an existing Alloy install), see [Alloy dependency and existing collectors](#alloy-dependency-and-existing-collectors) under Advanced configuration.

## Quick Start

This copy-ready flow provisions a disposable Prometheus+Grafana stack and Loki,
then collects **both** controller-manager metrics and controller-manager logs.
It is for development and testing only. If approved backends already exist, do
not install duplicates; substitute their endpoints and use [Advanced configuration](#advanced-configuration) for production TLS, RBAC, and backend details.

This is operator telemetry, not database monitoring. To deploy an exporter for
database-level telemetry, use the separate
[DatabaseObserver quick start](../../docs/observability/quickstart.md).

### 0. Confirm the operator is installed

```bash
kubectl get deployment,svc -n oracle-database-operator-system
kubectl get svc oracle-database-operator-controller-manager-metrics-service \
  -n oracle-database-operator-system
```

### 1. Provision disposable backends

```bash
helm repo add prometheus-community https://prometheus-community.github.io/helm-charts
helm repo add grafana https://grafana.github.io/helm-charts
helm repo update

cat >/tmp/operator-monitoring-values.yaml <<'EOF'
prometheus:
  prometheusSpec:
    enableRemoteWriteReceiver: true
grafana:
  enabled: true
EOF

helm upgrade --install monitoring prometheus-community/kube-prometheus-stack \
  --namespace monitoring --create-namespace \
  --values /tmp/operator-monitoring-values.yaml \
  --wait --timeout 10m

cat >/tmp/operator-loki-values.yaml <<'EOF'
deploymentMode: SingleBinary
loki:
  auth_enabled: false
  commonConfig:
    replication_factor: 1
  storage:
    type: filesystem
  useTestSchema: true
singleBinary:
  replicas: 1
read:
  replicas: 0
write:
  replicas: 0
backend:
  replicas: 0
chunksCache:
  enabled: false
resultsCache:
  enabled: false
test:
  enabled: false
EOF

helm upgrade --install loki grafana/loki --version 7.3.0 \
  --namespace monitoring --create-namespace \
  --values /tmp/operator-loki-values.yaml \
  --wait --timeout 10m

kubectl get pods,svc --namespace monitoring
```

The temporary Loki configuration deliberately has no authentication and uses
test filesystem storage. Do not use it outside a disposable development
cluster. This quick start does not put any credentials in files or commands.

### 2. Trust the operator metrics certificate

The chart verifies the operator HTTPS certificate; it does not use
`insecureSkipVerify`. The following applies to the default self-signed
`metrics-server-cert` Secret. For another certificate issuer or name, use its
CA as described in [Operator metrics TLS](#operator-metrics-tls).

```bash
kubectl create namespace oracle-database-operator-observability \
  --dry-run=client -o yaml | kubectl apply -f -

metrics_ca_file="$(mktemp)"
kubectl get secret metrics-server-cert \
  --namespace oracle-database-operator-system \
  -o jsonpath='{.data.tls\.crt}' | base64 --decode >"$metrics_ca_file"
kubectl create secret generic operator-metrics-ca \
  --namespace oracle-database-operator-observability \
  --from-file=ca.crt="$metrics_ca_file" \
  --dry-run=client -o yaml | kubectl apply -f -
rm -f "$metrics_ca_file"
```

Only the certificate is copied: never copy `tls.key` into the collector
namespace.

### 3. Install the collector for metrics and controller logs

```bash
cd charts/oracle-database-operator-observability

cat >/tmp/operator-observability-values.yaml <<'EOF'
operator:
  namespaces:
    - oracle-database-operator-system
  metrics:
    enabled: true
    tls:
      insecureSkipVerify: false
      caFile: /etc/alloy/operator-metrics-ca/ca.crt
  logs:
    enabled: true
destinations:
  prometheus:
    enabled: true
    url: http://monitoring-kube-prometheus-prometheus.monitoring.svc.cluster.local:9090/api/v1/write
  loki:
    enabled: true
    url: http://loki-gateway.monitoring.svc.cluster.local/loki/api/v1/push
alloy:
  rbac:
    namespaces:
      - oracle-database-operator-system
  controller:
    volumes:
      extra:
        - name: operator-metrics-ca
          secret:
            secretName: operator-metrics-ca
  alloy:
    mounts:
      extra:
        - name: operator-metrics-ca
          mountPath: /etc/alloy/operator-metrics-ca
          readOnly: true
EOF

helm dependency build .
helm upgrade --install oracle-database-operator-observability . \
  --namespace oracle-database-operator-observability --create-namespace \
  --values /tmp/operator-observability-values.yaml --wait --timeout 10m

kubectl rollout status deployment/oracle-database-operator-observability-alloy \
  --namespace oracle-database-operator-observability --timeout=5m
kubectl logs deployment/oracle-database-operator-observability-alloy \
  --namespace oracle-database-operator-observability --container alloy --tail=100
```

Confirm the actual Prometheus and Loki Service names before use; release names
or chart versions can change them. This values file intentionally enables only
operator metrics and logs. See [Optional workload logs](#optional-workload-logs)
and [Optional cAdvisor CPU and memory metrics](#optional-cadvisor-cpu-and-memory-metrics)
for additional collection.

### 4. Verify in Grafana

Port-forward Grafana from a machine with cluster access:

```bash
kubectl port-forward service/monitoring-grafana --namespace monitoring 3000:80
```

If that command runs on a jump host, tunnel the local port over SSH from the
workstation instead:

```bash
ssh -N -L 3000:127.0.0.1:3000 USER@JUMP_HOST
```

Open `http://127.0.0.1:3000` and authenticate through the Grafana installation's
normal method; this chart does not create or document credentials. Grafana from
`kube-prometheus-stack` normally has a Prometheus data source. Add a Loki data
source with `http://loki-gateway.monitoring.svc.cluster.local` if it is not
already provisioned.

In Grafana Explore, query the Prometheus data source:

```promql
count(controller_runtime_reconcile_total{job="prometheus.scrape.operator_metrics"})
```

Then query the Loki data source:

```logql
{job="oracle-database-operator"}
```

A nonzero PromQL result and controller-manager log entries prove the two
pipelines. For useful follow-up queries, see [Operator metrics example](#operator-metrics-example)
and [Operator log examples](#operator-log-examples); for troubleshooting, see
[Verify the complete flow](#9-verify-the-complete-flow).

### 5. Import the Grafana dashboard (optional)

To import the dashboard:

1. Open Grafana and select **Dashboards** → **Import**.
2. Upload [`dashboards/oracle-database-operator.json`](dashboards/oracle-database-operator.json).
3. Map the dashboard's `prometheus` and `loki` data sources to the data sources
   configured in the target Grafana instance.
4. Select **Import** and choose values for the workload namespace and Pod
   variables when workload collection is enabled.

Operator metrics and operator log panels work with the default Quick Start
collector settings. Workload CPU, memory, log, and restart panels need optional
workload log and cAdvisor collection; see [Grafana dashboard](#grafana-dashboard)
for those values and panel details.

### 6. Cleanup

Remove only this chart's collector and its CA Secret; Prometheus, Loki, Grafana,
and their retained data remain:

```bash
helm uninstall oracle-database-operator-observability \
  --namespace oracle-database-operator-observability
kubectl delete secret operator-metrics-ca \
  --namespace oracle-database-operator-observability --ignore-not-found=true
```

After the collector-only cleanup, remove the complete disposable development
stack as well:

```bash
helm uninstall loki --namespace monitoring
helm uninstall monitoring --namespace monitoring
kubectl delete namespace oracle-database-operator-observability \
  --ignore-not-found=true
```

PVCs can outlive Helm releases. Before deleting the namespace, delete them only
when the development data is disposable: **this permanently destroys
Prometheus, Grafana, and Loki data.**

```bash
kubectl delete pvc --all --namespace monitoring
```

Then remove the backend namespace and verify that only the development releases
were removed:

```bash
kubectl delete namespace monitoring --ignore-not-found=true

helm list --all --namespace oracle-database-operator-observability
helm list --all --namespace monitoring
kubectl get namespace oracle-database-operator-observability monitoring
```

The cleanup commands do not delete the Oracle AI Database Operator, its namespace,
or its metrics certificate.

## Architecture overview

```text
Oracle AI Database Operator
  ├── HTTPS /metrics
  └── stdout/stderr
            │
            ▼
       Grafana Alloy  (this chart)
            ├── operator metrics / logs
            ├── optional workload logs
            └── optional cAdvisor CPU/memory
            │
     ┌──────┴──────┐
     ▼             ▼
 Prometheus      Loki
     └──────┬──────┘
            ▼
         Grafana
```

| Component | Owned by | Responsibility |
|---|---|---|
| Oracle AI Database Operator | Existing operator installation | Controller-manager, metrics Service, operator Pods, metrics, and application logs |
| Grafana Alloy | This Helm release | Discovery, collection, labeling, and forwarding |
| Prometheus or compatible backend | Customer monitoring stack | Metrics storage and PromQL |
| Loki | Customer logging stack | Log storage and LogQL |
| Grafana | Customer visualization stack | Dashboards, Explore, and visualization |

The chart uses the official Grafana Alloy Helm chart as a pinned remote
dependency. The observability chart is optional; the operator does not depend
on Alloy, Prometheus, Loki, or Grafana to run.

## Advanced configuration

Use this section after the Quick Start, or when connecting to existing
production backends instead of the disposable lab stack.

### Choose a backend path

This chart sends telemetry to existing Prometheus-compatible and Loki
backends; it does not install or manage those backends.

- For a disposable development environment, continue with [Quick Start](#quick-start).
- For an existing or production monitoring platform, use its approved
  Prometheus and Loki endpoints, authentication, TLS, retention, and storage
  settings. The [backend installation reference](#backend-installation-reference)
  later in this document contains development examples.

The Quick Start is self-contained: it provisions disposable backends, creates
the required values, resolves the Alloy dependency, installs the collector, and
verifies both telemetry pipelines in order.

## Grafana dashboard

The chart repository includes an optional [Oracle AI Database Operator dashboard](dashboards/oracle-database-operator.json)
for manual import into Grafana. The Quick Start covers the same import under
[5. Import the Grafana dashboard (optional)](#5-import-the-grafana-dashboard-optional).
The dashboard is not installed automatically: this chart collects and forwards
telemetry but does not own the Grafana installation or its dashboards.

The dashboard contains six panels:

- Operator metrics scrape health.
- Workload Pod CPU and memory.
- Workload Pod logs.
- Oracle AI Database Operator logs.
- Workload container restarts.

The operator metrics and operator log panels work with the default collector
configuration. The workload panels require the corresponding optional pipelines:

```yaml
workloadLogs:
  enabled: true
  namespaces:
    - sidb

cadvisor:
  enabled: true
```

The restart panel also requires `kube_pod_container_status_restarts_total`,
usually provided by kube-state-metrics or an equivalent Prometheus source.

To import the dashboard:

1. Open Grafana and select **Dashboards** → **Import**.
2. Upload [`dashboards/oracle-database-operator.json`](dashboards/oracle-database-operator.json).
3. Map the dashboard's `prometheus` and `loki` data sources to the data sources
   configured in the target Grafana instance.
4. Select **Import** and choose values for the workload namespace and Pod
   variables when workload collection is enabled.

The dashboard JSON has been sanitized for redistribution: Grafana instance
metadata, user identity, resource versions, timestamps, and environment-specific
variable selections were removed. The copied YAML is a Grafana v2 resource export
intended for Grafana Kubernetes resource management, not the portable manual
import artifact.

## Backend installation reference

The observability chart does not install telemetry storage or visualization
backends.

The [Quick Start](#quick-start) above is the single copy-ready path for a
disposable stack. The examples below are detailed backend reference material for
custom development or small-cluster setups; do not combine their installation
commands with the quick start. For production, use the organization's approved
monitoring stack and organization-specific values for:

- high availability,
- persistent storage,
- retention,
- authentication,
- authorization,
- TLS,
- backup,
- NetworkPolicy,
- ingress,
- and disaster recovery.

### Install Prometheus and Grafana for development

The `kube-prometheus-stack` chart installs Prometheus and Grafana together. It
also installs components such as Alertmanager, kube-state-metrics, and
node-exporter unless disabled in its values.

Create `monitoring-values.yaml` outside this chart:

```yaml
prometheus:
  prometheusSpec:
    # Alloy sends metrics through remote_write.
    enableRemoteWriteReceiver: true

    retention: 15d

    storageSpec:
      volumeClaimTemplate:
        spec:
          resources:
            requests:
              storage: 20Gi

grafana:
  enabled: true

  persistence:
    enabled: true
    size: 5Gi
```

Install it:

```bash
helm repo add prometheus-community \
  https://prometheus-community.github.io/helm-charts

helm repo update

helm upgrade --install monitoring \
  prometheus-community/kube-prometheus-stack \
  --namespace monitoring \
  --create-namespace \
  --values monitoring-values.yaml
```

Check the generated Services:

```bash
kubectl get pods,svc -n monitoring
kubectl get prometheus -n monitoring
```

With release name `monitoring`, a typical Prometheus remote-write endpoint is:

```text
http://monitoring-kube-prometheus-prometheus.monitoring.svc.cluster.local:9090/api/v1/write
```

Always confirm the actual Service name:

```bash
kubectl get svc -n monitoring
```

The remote-write URL is an ingestion URL used by Alloy.

Grafana uses the Prometheus query/base URL instead of `/api/v1/write`.

### Install Loki for development

Prometheus does not store logs.

Install Loki separately when:

```yaml
destinations:
  loki:
    enabled: true
```

The following example is for development/testing only.

Create `loki-values.yaml` outside this chart:

```yaml
deploymentMode: SingleBinary

loki:
  # Development only.
  auth_enabled: false

  commonConfig:
    replication_factor: 1

  storage:
    type: filesystem

  # Development only.
  useTestSchema: true

singleBinary:
  replicas: 1

  persistence:
    enabled: true
    size: 10Gi

read:
  replicas: 0

write:
  replicas: 0

backend:
  replicas: 0

chunksCache:
  enabled: false

resultsCache:
  enabled: false

test:
  enabled: false
```

Install Loki:

```bash
helm repo add grafana https://grafana.github.io/helm-charts
helm repo update

helm upgrade --install loki \
  grafana/loki \
  --version 7.3.0 \
  --namespace monitoring \
  --create-namespace \
  --values loki-values.yaml
```

Verify Loki:

```bash
kubectl get pods,svc -n monitoring | grep -i loki
```

A typical Alloy Loki push URL is:

```text
http://loki-gateway.monitoring.svc.cluster.local/loki/api/v1/push
```

The corresponding Grafana Loki data-source URL is typically:

```text
http://loki-gateway.monitoring.svc.cluster.local
```

Always use the actual Services in the target cluster.

For production Loki:

- use an organization-approved deployment mode,
- use supported persistent/object storage,
- enable authentication,
- configure TLS,
- and provide credentials through Kubernetes Secrets.

## 1. Create user values

Create a values file outside the chart, for example:

```text
user-values.yaml
```

A minimal production-oriented example is:

```yaml
operator:
  namespaces:
    - oracle-database-operator-system

  metrics:
    tls:
      insecureSkipVerify: false
      caFile: /etc/alloy/operator-metrics-ca/ca.crt

destinations:
  prometheus:
    enabled: true
    url: https://prometheus.example.com/api/v1/write

  loki:
    enabled: true
    url: https://loki.example.com/loki/api/v1/push

workloadLogs:
  enabled: false
  namespaces: []

cadvisor:
  enabled: false

alloy:
  rbac:
    namespaces:
      - oracle-database-operator-system

  controller:
    volumes:
      extra:
        - name: operator-metrics-ca
          secret:
            secretName: operator-metrics-ca

  alloy:
    mounts:
      extra:
        - name: operator-metrics-ca
          mountPath: /etc/alloy/operator-metrics-ca
          readOnly: true
```

Use the real destination URLs for the customer environment.

Prometheus must accept remote-write traffic.

Loki must accept its push API.

The chart does not require Prometheus and Loki to run in the same Kubernetes
cluster as Alloy, provided the destinations are reachable securely.

### Backend authentication

Do not store backend passwords or bearer tokens directly in committed
`values.yaml` files.

If a backend requires a bearer token, mount a Kubernetes Secret and configure
the token-file paths:

```yaml
destinations:
  prometheus:
    enabled: true
    url: https://prometheus.example.com/api/v1/write
    bearerTokenFile: /etc/alloy/backend-auth/prometheus.token

  loki:
    enabled: true
    url: https://loki.example.com/loki/api/v1/push
    bearerTokenFile: /etc/alloy/backend-auth/loki.token

alloy:
  controller:
    volumes:
      extra:
        - name: backend-auth
          secret:
            secretName: alloy-backend-auth

  alloy:
    mounts:
      extra:
        - name: backend-auth
          mountPath: /etc/alloy/backend-auth
          readOnly: true
```

The Secret must already exist in the Alloy release namespace.

### Backend TLS

Backend TLS verification is enabled by default.

When a backend uses a private CA, mount the CA and configure the corresponding
CA file.

Example:

```yaml
destinations:
  prometheus:
    tls:
      insecureSkipVerify: false
      caFile: /etc/alloy/backend-ca/prometheus-ca.crt

  loki:
    tls:
      insecureSkipVerify: false
      caFile: /etc/alloy/backend-ca/loki-ca.crt
```

Do not use:

```yaml
insecureSkipVerify: true
```

for normal production deployments.

### Enable or disable telemetry destinations

The destination flags control Alloy pipelines. They do not install or uninstall
Prometheus, Loki, or Grafana.

| Value | Effect when `false` |
|---|---|
| `destinations.prometheus.enabled` | Omits Prometheus remote write and Prometheus scrape pipelines |
| `destinations.loki.enabled` | Omits Loki write and Kubernetes log pipelines |
| `operator.metrics.enabled` | Disables operator metrics discovery and scraping |
| `operator.logs.enabled` | Disables operator Pod log discovery and collection |
| `workloadLogs.enabled` | Disables workload Pod log discovery and collection |
| `cadvisor.enabled` | Disables Kubernetes cAdvisor CPU and memory collection |
| `alloy.enabled` | Disables the Alloy dependency and generated Alloy ConfigMap |

Disable only Prometheus forwarding:

```bash
helm upgrade oracle-database-operator-observability . \
  --namespace oracle-database-operator-observability \
  --values user-values.yaml \
  --set destinations.prometheus.enabled=false
```

Disable only Loki forwarding:

```bash
helm upgrade oracle-database-operator-observability . \
  --namespace oracle-database-operator-observability \
  --values user-values.yaml \
  --set destinations.loki.enabled=false
```

Disable both pipelines while keeping Alloy installed:

```bash
helm upgrade oracle-database-operator-observability . \
  --namespace oracle-database-operator-observability \
  --values user-values.yaml \
  --set destinations.prometheus.enabled=false \
  --set destinations.loki.enabled=false
```

These flags do not affect separately managed Prometheus, Loki, or Grafana
installations.

### Optional workload logs

Workload log collection is disabled by default.

Enable it for namespaces containing workloads that should be observed:

```yaml
workloadLogs:
  enabled: true

  namespaces:
    - database-namespace

alloy:
  rbac:
    namespaces:
      - oracle-database-operator-system
      - database-namespace
```

Every namespace configured under `workloadLogs.namespaces` must also be granted
the required namespaced RBAC through `alloy.rbac.namespaces`.

The generated workload Loki streams can include the following labels:

```text
namespace
pod
container
job
```

The workload log job label is:

```text
oracle-database-kubernetes-logs
```

The operator log job label is:

```text
oracle-database-operator
```

Do not enable duplicate workload log collection when another collector already
sends the same logs to the same backend unless duplicate ingestion is intended.

### Optional cAdvisor CPU and memory metrics

cAdvisor collection is disabled by default.

Enable it only when the customer's existing Kubernetes monitoring platform does
not already collect equivalent container resource metrics:

```yaml
cadvisor:
  enabled: true

destinations:
  prometheus:
    enabled: true
    url: https://prometheus.example.com/api/v1/write
```

The initial cAdvisor pipeline forwards only:

```text
container_cpu_usage_seconds_total
container_memory_working_set_bytes
```

The generated Prometheus job label is:

```text
kubernetes-cadvisor
```

Alloy accesses kubelet cAdvisor through the Kubernetes API:

```text
/api/v1/nodes/<node-name>/proxy/metrics/cadvisor
```

Enabling cAdvisor requires additional cluster-scoped read permissions.

Keep cAdvisor disabled when the customer's existing Kubernetes monitoring
platform already supplies equivalent CPU and memory metrics.

## 2. Configure an HTTP or HTTPS proxy

A user can configure a proxy through chart values.

Do not place proxy passwords in a committed `values.yaml` file.

### Direct proxy URL

Use this when the proxy URL does not contain credentials:

```yaml
proxy:
  enabled: true
  url: http://proxy.example.com:8080

  noProxy:
    - localhost
    - 127.0.0.1
    - kubernetes.default.svc
    - .svc
    - .cluster.local
    - prometheus.monitoring.svc.cluster.local
    - loki.monitoring.svc.cluster.local
```

The chart passes proxy configuration to:

- Kubernetes discovery,
- operator metric scraping,
- cAdvisor discovery and scraping when enabled,
- Prometheus remote write,
- Loki writes.

The Kubernetes API and cluster-local Services should normally be included in
`noProxy`.

### Proxy through environment variables

Use this when proxy credentials must come from a Secret or when the platform
standardizes on `HTTP_PROXY`, `HTTPS_PROXY`, and `NO_PROXY`.

```yaml
proxy:
  enabled: true
  fromEnvironment: true

alloy:
  alloy:
    extraEnv:
      - name: HTTP_PROXY
        value: http://proxy.example.com:8080

      - name: HTTPS_PROXY
        value: http://proxy.example.com:8080

      - name: NO_PROXY
        value: kubernetes.default.svc,.svc,.cluster.local
```

For a credential-bearing proxy, use a Secret:

```yaml
proxy:
  enabled: true
  fromEnvironment: true

alloy:
  alloy:
    extraEnv:
      - name: HTTPS_PROXY
        valueFrom:
          secretKeyRef:
            name: alloy-proxy
            key: https-proxy

      - name: NO_PROXY
        valueFrom:
          secretKeyRef:
            name: alloy-proxy
            key: no-proxy
```

The Secret must already exist in the Alloy release namespace.

Example:

```bash
kubectl create secret generic alloy-proxy \
  --namespace oracle-database-operator-observability \
  --from-literal=https-proxy='https://user:password@proxy.example.com:8443' \
  --from-literal=no-proxy='kubernetes.default.svc,.svc,.cluster.local'
```

Configure either:

```text
proxy.url
```

or:

```text
proxy.fromEnvironment
```

but not both.

## 3. Monitor multiple namespaces

### Operator namespaces

Set operator namespaces under:

```yaml
operator:
  namespaces:
```

The same namespaces must be present under:

```yaml
alloy:
  rbac:
    namespaces:
```

because the Alloy dependency does not automatically derive namespaced RBAC
values from the parent chart.

Example:

```yaml
operator:
  namespaces:
    - oracle-database-operator-system
    - oracle-database-operator-prod
    - oracle-database-operator-test

alloy:
  rbac:
    namespaces:
      - oracle-database-operator-system
      - oracle-database-operator-prod
      - oracle-database-operator-test
```

The chart then:

1. Watches Services in the configured operator namespaces.
2. Watches operator Pods in the configured namespaces.
3. Creates namespaced RoleBindings through the Alloy dependency.
4. Sends selected metrics and logs to the configured destinations.

The default operator log selector is:

```yaml
operator:
  logs:
    podLabelMeta: __meta_kubernetes_pod_label_control_plane
    podLabelValue: controller-manager
```

If the operator Pods use a different label, update both values accordingly.

The configured operator metrics Service must exist in every namespace where
metrics are expected.

### Workload namespaces

Workload namespaces are configured separately:

```yaml
workloadLogs:
  enabled: true

  namespaces:
    - database-prod
    - database-test
```

Grant Alloy read access to those namespaces:

```yaml
alloy:
  rbac:
    namespaces:
      - oracle-database-operator-system
      - database-prod
      - database-test
```

After changing namespace configuration, upgrade the release:

```bash
helm upgrade oracle-database-operator-observability . \
  --namespace oracle-database-operator-observability \
  --values user-values.yaml
```

## 4. Resolve, render, and inspect the chart

From the chart directory:

```bash
cd charts/oracle-database-operator-observability

helm dependency build .

helm lint . \
  --values user-values.yaml

helm template \
  oracle-database-operator-observability \
  . \
  --namespace oracle-database-operator-observability \
  --values user-values.yaml \
  > /tmp/oracle-database-operator-observability.yaml
```

Inspect the important resources:

```bash
rg -n \
  'kind: (ConfigMap|Deployment|ServiceAccount|Role|RoleBinding|ClusterRole|ClusterRoleBinding)|prometheus\.scrape|prometheus\.remote_write|prometheus\.relabel|loki\.source\.kubernetes|loki\.write|nodes/proxy|endpointslices|proxy' \
  /tmp/oracle-database-operator-observability.yaml
```

Confirm that:

- The Alloy ConfigMap contains the expected enabled pipelines.
- Operator namespace discovery matches `operator.namespaces`.
- Workload namespace discovery appears only when `workloadLogs.enabled=true`.
- cAdvisor discovery appears only when `cadvisor.enabled=true`.
- The Alloy ServiceAccount has access to `/metrics` when operator metrics are
  enabled.
- `nodes`, `nodes/proxy`, and `nodes/metrics` RBAC appear only when cAdvisor is
  enabled.
- EndpointSlice RBAC uses the `discovery.k8s.io` API group.
- The Alloy workload uses the generated ConfigMap.
- Backend passwords or bearer-token values are not embedded in the generated
  ConfigMap.

For an additional Kubernetes API validation:

```bash
kubectl apply \
  --dry-run=server \
  -f /tmp/oracle-database-operator-observability.yaml
```

## 5. Install Alloy

For the first installation, use:

```bash
helm upgrade --install oracle-database-operator-observability . \
  --namespace oracle-database-operator-observability \
  --create-namespace \
  --values user-values.yaml \
  --wait \
  --timeout 10m
```

For later updates, the same command is safe to reuse.

If the release reports an unexpected failed or pending state:

```bash
helm list --all \
  --namespace oracle-database-operator-observability

helm history oracle-database-operator-observability \
  --namespace oracle-database-operator-observability
```

If this is a disposable failed first installation:

```bash
helm uninstall oracle-database-operator-observability \
  --namespace oracle-database-operator-observability

helm upgrade --install oracle-database-operator-observability . \
  --namespace oracle-database-operator-observability \
  --create-namespace \
  --values user-values.yaml \
  --wait \
  --timeout 10m
```

This does not uninstall the Oracle AI Database Operator or separately managed
Prometheus, Loki, or Grafana installations.

Check the release:

```bash
helm status oracle-database-operator-observability \
  --namespace oracle-database-operator-observability
```

Check the workload:

```bash
kubectl get pods,svc,configmap,serviceaccount \
  --namespace oracle-database-operator-observability
```

Check Alloy logs:

```bash
kubectl logs \
  deployment/oracle-database-operator-observability-alloy \
  --namespace oracle-database-operator-observability \
  --container alloy
```

If the Alloy dependency has a config-reloader container enabled, inspect it
separately when a configuration update does not take effect:

```bash
kubectl logs \
  deployment/oracle-database-operator-observability-alloy \
  --namespace oracle-database-operator-observability \
  --container config-reloader \
  --tail=100
```

The Deployment can also be discovered by label:

```bash
kubectl get deployment \
  --namespace oracle-database-operator-observability \
  -l app.kubernetes.io/name=alloy
```

## 6. Verify Alloy readiness and generated configuration

Find the Alloy Service:

```bash
kubectl get svc \
  --namespace oracle-database-operator-observability
```

Forward the Alloy HTTP port when required:

```bash
kubectl port-forward \
  service/oracle-database-operator-observability-alloy \
  --namespace oracle-database-operator-observability \
  12345:12345
```

In another terminal:

```bash
curl http://127.0.0.1:12345/-/ready
```

The response should indicate that Alloy is ready.

Configuration, discovery, RBAC, TLS, or destination errors are also visible in
the Alloy Pod logs.

The Alloy HTTP endpoint is an operational endpoint.

It is not Grafana and should not be treated as the customer visualization
interface.

Grafana is used to query telemetry stored in Prometheus and Loki.

## 7. Required RBAC and security details

### Chart-created identity

The Alloy dependency creates a ServiceAccount in the observability release
namespace.

The ServiceAccount token is automatically mounted because Alloy needs to:

- authenticate to the Kubernetes API,
- discover Kubernetes resources,
- read selected Pod logs,
- authenticate to the operator metrics endpoint,
- and optionally access cAdvisor through the Kubernetes API.

No manually created long-lived Kubernetes token is required.

### Namespaced permissions

For every namespace in:

```yaml
alloy:
  rbac:
    namespaces:
```

the Alloy identity receives read-only access required for enabled discovery
pipelines.

| API group | Resource | Verbs | Purpose |
|---|---|---|---|
| core | `pods` | `get`, `list`, `watch` | Discover operator/workload Pods |
| core | `pods/log` | `get`, `list`, `watch` | Read stdout/stderr logs |
| core | `services` | `get`, `list`, `watch` | Discover metrics Services |
| core | `endpoints` | `get`, `list`, `watch` | Resolve Service endpoints |
| `discovery.k8s.io` | `endpointslices` | `get`, `list`, `watch` | Resolve modern Service endpoints |

The chart does not grant create, update, patch, or delete permissions on these
resources.

### Operator metrics authorization

The operator's secure metrics server uses Kubernetes authentication and
authorization.

When required by the namespaced Alloy RBAC mode, the parent chart creates
cluster-scoped RBAC allowing the Alloy identity to perform:

```text
GET /metrics
```

Verify it:

```bash
kubectl auth can-i get \
  --non-resource-url=/metrics \
  --as=system:serviceaccount:oracle-database-operator-observability:oracle-database-operator-observability-alloy
```

Expected result:

```text
yes
```

Verify namespace access:

```bash
kubectl auth can-i list pods \
  --namespace oracle-database-operator-system \
  --as=system:serviceaccount:oracle-database-operator-observability:oracle-database-operator-observability-alloy

kubectl auth can-i get pods/log \
  --namespace oracle-database-operator-system \
  --as=system:serviceaccount:oracle-database-operator-observability:oracle-database-operator-observability-alloy
```

Repeat these checks for every workload namespace when workload log collection is
enabled.

### cAdvisor cluster permissions

When:

```yaml
cadvisor:
  enabled: true
```

the chart additionally grants read-only cluster-scoped permissions for:

| Resource | Verbs | Purpose |
|---|---|---|
| `nodes` | `get`, `list`, `watch` | Discover Kubernetes Nodes |
| `nodes/proxy` | `get` | Access kubelet cAdvisor through the Kubernetes API |
| `nodes/metrics` | `get` | Permit node metrics access where required |

These permissions are not required for normal operator metrics and logs.

Verify them:

```bash
kubectl auth can-i list nodes \
  --as=system:serviceaccount:oracle-database-operator-observability:oracle-database-operator-observability-alloy

kubectl auth can-i get nodes \
  --as=system:serviceaccount:oracle-database-operator-observability:oracle-database-operator-observability-alloy

kubectl auth can-i get nodes/proxy \
  --as=system:serviceaccount:oracle-database-operator-observability:oracle-database-operator-observability-alloy

kubectl auth can-i get nodes/metrics \
  --as=system:serviceaccount:oracle-database-operator-observability:oracle-database-operator-observability-alloy
```

Expected result when cAdvisor is enabled:

```text
yes
```

### Operator metrics TLS

The operator metrics endpoint is HTTPS.

Two modes are supported.

#### Smoke test only

```yaml
operator:
  metrics:
    tls:
      insecureSkipVerify: true
```

This disables certificate verification.

Do not use it for normal production deployments.

#### Production

Use a trusted CA:

```yaml
operator:
  metrics:
    tls:
      insecureSkipVerify: false
      caFile: /etc/alloy/operator-metrics-ca/ca.crt

alloy:
  controller:
    volumes:
      extra:
        - name: operator-metrics-ca
          secret:
            secretName: operator-metrics-ca

  alloy:
    mounts:
      extra:
        - name: operator-metrics-ca
          mountPath: /etc/alloy/operator-metrics-ca
          readOnly: true
```

For an operator installation using a self-signed metrics certificate, create the
trust Secret without copying the private key:

```bash
metrics_ca_file="$(mktemp)"

kubectl get secret metrics-server-cert \
  --namespace oracle-database-operator-system \
  -o jsonpath='{.data.tls\.crt}' \
  | base64 --decode > "$metrics_ca_file"

kubectl create secret generic operator-metrics-ca \
  --namespace oracle-database-operator-observability \
  --from-file=ca.crt="$metrics_ca_file" \
  --dry-run=client \
  -o yaml \
  | kubectl apply -f -

rm -f "$metrics_ca_file"
```

Never copy `tls.key` into the Alloy trust Secret.

If the operator is configured with a separate CA, mount that CA certificate
instead.

### Backend bearer tokens

Prometheus and Loki bearer tokens should be mounted from Kubernetes Secrets.

Do not place bearer tokens directly in:

- `values.yaml`,
- the Alloy ConfigMap,
- Git,
- or Helm command lines stored in shell history.

Example:

```yaml
destinations:
  prometheus:
    bearerTokenFile: /etc/alloy/backend-auth/prometheus.token

  loki:
    bearerTokenFile: /etc/alloy/backend-auth/loki.token

alloy:
  controller:
    volumes:
      extra:
        - name: backend-auth
          secret:
            secretName: alloy-backend-auth

  alloy:
    mounts:
      extra:
        - name: backend-auth
          mountPath: /etc/alloy/backend-auth
          readOnly: true
```

### Proxy credentials

Use:

```yaml
valueFrom:
  secretKeyRef:
```

for credential-bearing proxy values.

Avoid embedding proxy credentials directly in the Alloy ConfigMap.

### NetworkPolicy and egress

This chart does not create a NetworkPolicy.

If the cluster uses default-deny egress, allow Alloy to reach:

- the Kubernetes API server,
- the operator metrics Service,
- monitored operator namespaces,
- configured workload namespaces,
- the Prometheus remote-write endpoint,
- the Loki push endpoint,
- the configured HTTP/HTTPS proxy, if used,
- and cluster DNS.

When cAdvisor is enabled, Alloy also accesses kubelet cAdvisor through the
Kubernetes API server's node-proxy endpoint.

## 8. Connect to Grafana

Grafana is not installed or managed by this chart.

It must already be available or must be installed separately as part of the
customer's monitoring platform.

### Find Grafana

```bash
kubectl get services \
  --all-namespaces \
  | grep -i grafana
```

For a development environment, port-forward the Grafana Service.

Example:

```bash
kubectl port-forward \
  service/monitoring-grafana \
  --namespace monitoring \
  3000:80
```

Open:

```text
http://127.0.0.1:3000
```

Use the credentials supplied by the Grafana installation.

They are not created or managed by this chart.

### Configure data sources

Grafana needs:

- a Prometheus data source pointing to the Prometheus query endpoint,
- a Loki data source pointing to the Loki query endpoint.

The URLs used by Alloy for ingestion are not necessarily the same URLs Grafana
uses for queries.

For example:

```text
Alloy remote-write:
https://prometheus.example.com/api/v1/write

Grafana Prometheus data source:
https://prometheus.example.com
```

Similarly:

```text
Alloy Loki push:
https://loki.example.com/loki/api/v1/push

Grafana Loki data source:
https://loki.example.com
```

### Operator metrics example

In Grafana Explore, select the Prometheus data source:

```promql
controller_runtime_reconcile_total{
  job="prometheus.scrape.operator_metrics"
}
```

Reconcile rate by controller:

```promql
sum by (controller) (
  rate(
    controller_runtime_reconcile_total{
      job="prometheus.scrape.operator_metrics"
    }[5m]
  )
)
```

Operator process memory:

```promql
process_resident_memory_bytes{
  job="prometheus.scrape.operator_metrics"
}
```

### cAdvisor examples

When cAdvisor is enabled:

```promql
1000 *
sum by (namespace, pod, container) (
  rate(
    container_cpu_usage_seconds_total{
      job="kubernetes-cadvisor",
      container!="",
      container!="POD"
    }[5m]
  )
)
```

Memory:

```promql
sum by (namespace, pod, container) (
  container_memory_working_set_bytes{
    job="kubernetes-cadvisor",
    container!="",
    container!="POD"
  }
)
```

### Operator log examples

Select the Loki data source:

```logql
{job="oracle-database-operator"}
```

Operator manager:

```logql
{
  namespace="oracle-database-operator-system",
  container="manager"
}
```

Errors:

```logql
{
  namespace="oracle-database-operator-system",
  container="manager"
}
|~ "(?i)error"
```

### Workload log examples

When workload logging is enabled:

```logql
{job="oracle-database-kubernetes-logs"}
```

For a specific namespace:

```logql
{namespace="database-namespace"}
```

For a specific container:

```logql
{
  namespace="database-namespace",
  container="CONTAINER_NAME"
}
```

### Grafana dashboards

This chart does not create, update, or manage a customer Grafana installation.

Customers can:

- create dashboards from the documented PromQL and LogQL queries,
- import an organization-approved dashboard,
- or import an Oracle-provided reference dashboard when one is distributed
  with the product.

A reference dashboard should remain separate from Grafana lifecycle management
so that customers can use their existing Grafana installation and provisioning
process.

## 9. Verify the complete flow

### Operator metrics Service

```bash
kubectl get service \
  --namespace oracle-database-operator-system \
  oracle-database-operator-controller-manager-metrics-service
```

### Operator logs

```bash
kubectl logs \
  deployment/oracle-database-operator-controller-manager \
  --namespace oracle-database-operator-system
```

### Alloy

```bash
kubectl get pods \
  --namespace oracle-database-operator-observability

kubectl logs \
  deployment/oracle-database-operator-observability-alloy \
  --namespace oracle-database-operator-observability \
  --container alloy
```

Confirm that Alloy logs do not show unexpected:

```text
forbidden
unauthorized
certificate
x509
remote_write
loki
connection refused
timeout
```

errors.

### Prometheus

Confirm that operator metrics exist.

Example:

```promql
count(
  controller_runtime_reconcile_total{
    job="prometheus.scrape.operator_metrics"
  }
)
```

When cAdvisor is enabled:

```promql
count(
  container_cpu_usage_seconds_total{
    job="kubernetes-cadvisor"
  }
)
```

and:

```promql
count(
  container_memory_working_set_bytes{
    job="kubernetes-cadvisor"
  }
)
```

### Loki

Confirm operator logs:

```logql
{job="oracle-database-operator"}
```

When workload logs are enabled:

```logql
{job="oracle-database-kubernetes-logs"}
```

### Grafana

Confirm that Grafana can query both configured data sources.

The final expected flow is:

```text
Operator /metrics
        │
        ▼
      Alloy
        │
        ▼
Prometheus-compatible backend
        │
        ▼
      Grafana


Operator stdout/stderr
        │
        ▼
      Alloy
        │
        ▼
       Loki
        │
        ▼
      Grafana


Optional workload logs
        │
        ▼
      Alloy
        │
        ▼
       Loki
        │
        ▼
      Grafana


Optional cAdvisor
        │
        ▼
      Alloy
        │
        ▼
Prometheus-compatible backend
        │
        ▼
      Grafana
```

If Prometheus has no samples, check:

- Alloy logs,
- Kubernetes RBAC,
- operator Service discovery,
- TLS trust,
- backend authentication,
- backend connectivity,
- and remote-write configuration.

If Loki has no entries, check:

- Pod discovery,
- `pods/log` RBAC,
- workload namespace configuration,
- backend authentication,
- backend connectivity,
- and the Loki push endpoint.

If cAdvisor has no samples, check:

- `cadvisor.enabled`,
- `nodes` discovery permissions,
- `nodes/proxy` permission,
- Kubernetes API connectivity,
- and Prometheus remote-write status.

Useful health commands:

```bash
helm list \
  --namespace oracle-database-operator-observability

helm status oracle-database-operator-observability \
  --namespace oracle-database-operator-observability

kubectl get pods,svc,configmap,serviceaccount \
  --namespace oracle-database-operator-observability

kubectl rollout status \
  deployment/oracle-database-operator-observability-alloy \
  --namespace oracle-database-operator-observability \
  --timeout=5m

kubectl get services \
  --all-namespaces \
  | grep -Ei 'grafana|loki|prometheus'
```

## 10. Upgrade and uninstall

Upgrade the collector configuration with Helm:

```bash
helm upgrade oracle-database-operator-observability . \
  --namespace oracle-database-operator-observability \
  --values user-values.yaml \
  --wait \
  --timeout 10m
```

If configuration reload is disabled in the selected Alloy deployment values,
restart Alloy after changing the generated configuration:

```bash
kubectl rollout restart \
  deployment/oracle-database-operator-observability-alloy \
  --namespace oracle-database-operator-observability

kubectl rollout status \
  deployment/oracle-database-operator-observability-alloy \
  --namespace oracle-database-operator-observability
```

### Namespace recreation

When namespaced RBAC is used, deleting and recreating a monitored namespace also
deletes the Role and RoleBinding resources stored in that namespace.

After recreating a monitored namespace, reconcile the Helm release:

```bash
helm upgrade --install oracle-database-operator-observability . \
  --namespace oracle-database-operator-observability \
  --values user-values.yaml
```

This recreates required namespaced RBAC resources.

### Uninstall

Remove only the Alloy observability release:

```bash
helm uninstall oracle-database-operator-observability \
  --namespace oracle-database-operator-observability
```

Uninstalling this chart does not delete:

- the Oracle AI Database Operator,
- operator namespaces,
- customer Prometheus or compatible backends,
- customer Loki,
- or customer Grafana.

Those systems are managed independently.

## Alloy dependency and existing collectors

The parent chart declares this dependency:

```yaml
dependencies:
  - name: alloy
    version: "1.12.1"
    repository: https://grafana.github.io/helm-charts
```

The dependency is resolved on the machine running Helm, not by the Kubernetes
cluster.

There are two supported dependency workflows.

### Online installation

Use this when the Helm machine can reach the Grafana chart repository:

```bash
cd charts/oracle-database-operator-observability

helm repo add grafana https://grafana.github.io/helm-charts
helm repo update

helm dependency update .
```

`helm dependency update` downloads the pinned Alloy chart and creates or
updates `Chart.lock`.

The downloaded archive is generated under the chart's `charts/` directory.

For normal repeatable builds, use the lock file:

```bash
helm dependency build .
```

If the dependency archive is not already available locally,
`helm dependency build` also requires repository access.

### Offline or air-gapped installation

The air-gapped Kubernetes cluster does not need access to Grafana's Helm
repository.

A connected build machine must first resolve and package the dependency:

```bash
cd charts/oracle-database-operator-observability

helm repo add grafana https://grafana.github.io/helm-charts
helm repo update

helm dependency update .
helm package .
```

Transfer the generated parent chart archive to the air-gapped environment and
install the archive:

```bash
helm upgrade --install oracle-database-operator-observability \
  ./oracle-database-operator-observability-0.1.0.tgz \
  --namespace oracle-database-operator-observability \
  --create-namespace \
  --values user-values.yaml
```

The packaged parent chart contains its resolved Alloy dependency, so the target
cluster does not need to contact `grafana.github.io`.

An organization can also mirror the Alloy chart into an internal Helm or OCI
registry. Pin the approved mirrored chart version and regenerate `Chart.lock`
in the connected build environment.

### If Alloy is already installed

This chart does not adopt an Alloy Deployment owned by another Helm release.

Installing this chart with:

```yaml
alloy:
  enabled: true
```

creates a separate release-owned Alloy workload.

Running multiple collectors against the same targets and destinations can
duplicate metrics or logs.

If Alloy is already managed elsewhere, use one of these approaches:

1. Review the generated Alloy configuration and integrate the required
   pipelines into the organization's existing Alloy deployment.
2. Migrate Alloy ownership to this chart during a planned maintenance window.
3. Do not install this Alloy release if the existing collector already provides
   equivalent collection.

Setting:

```yaml
alloy:
  enabled: false
```

disables both the Alloy dependency and this chart's generated Alloy ConfigMap.
It is not an attach-to-existing-Alloy mode.

## Production guidance summary

For normal production use:

- Keep operator TLS verification enabled.
- Use a trusted CA instead of `insecureSkipVerify`.
- Store backend credentials in Kubernetes Secrets.
- Use authenticated HTTPS endpoints for external telemetry backends.
- Keep workload logging disabled unless required.
- Keep cAdvisor disabled when the customer's Kubernetes monitoring stack already
  provides equivalent container CPU and memory metrics.
- Grant only the RBAC needed for enabled features.
- Do not deploy duplicate Alloy collectors against the same targets unless
  duplicate collection is intended.
- Use the customer's existing Prometheus/Loki/Grafana infrastructure when
  available.
- Treat this chart as an optional telemetry collection and integration layer,
  not as a replacement for the customer's observability platform.
