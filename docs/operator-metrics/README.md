# Oracle Database Operator Metrics

This guide explains how to read the metrics exposed by the Oracle Database
Operator controller manager itself.

These are operator metrics such as:

- `controller_runtime_*`
- `workqueue_*`
- `go_*`
- `process_*`

This guide is different from [`docs/observability/README.md`](../observability/README.md),
which documents `DatabaseObserver` metrics for Oracle databases.

## What Is Enabled By Default

When you install the operator from the generated
[`oracle-database-operator.yaml`](../../oracle-database-operator.yaml), the
metrics endpoint is exposed by default:

- A metrics `Service` is created as
  `oracle-database-operator-controller-manager-metrics-service`
- A TLS certificate is requested through cert-manager
- The manager serves metrics on HTTPS port `8443`
- The webhook server remains on port `9443`
- The metrics certificate is mounted automatically into the manager Pod

You do not need to create the metrics certificate manually.

## Prerequisites

Before testing operator metrics, make sure:

1. `cert-manager` is installed and healthy
2. The operator is installed from the updated `oracle-database-operator.yaml`
3. The operator Pods are running in namespace `oracle-database-operator-system`

## 1. Install The Operator

Apply the operator manifest:

```bash
kubectl apply -f oracle-database-operator.yaml
```

Verify the operator Pods are running:

```bash
kubectl get pods -n oracle-database-operator-system
```

## 2. Verify The Metrics Service And Certificate

Check that the metrics Service exists:

```bash
kubectl get svc -n oracle-database-operator-system
```

Expected Service name:

```text
oracle-database-operator-controller-manager-metrics-service
```

Check that cert-manager created the certificate and secret:

```bash
kubectl get certificate,secret -n oracle-database-operator-system | grep metrics
```

Expected objects:

```text
oracle-database-operator-metrics-certs
metrics-server-cert
```

If the Service exists but the `metrics-server-cert` secret does not, check the
cert-manager installation before continuing.

## 3. Create A ServiceAccount For Manual Testing

Create a ServiceAccount in the operator namespace:

```bash
kubectl create sa metrics-reader -n oracle-database-operator-system
```

Bind it to the operator metrics reader ClusterRole:

```bash
kubectl create clusterrolebinding operator-metrics-reader-binding \
  --clusterrole=oracle-database-operator-metrics-reader \
  --serviceaccount=oracle-database-operator-system:metrics-reader
```

Create a token for that ServiceAccount:

```bash
kubectl create token metrics-reader -n oracle-database-operator-system
```

Save the returned token for the next step.

## 4. Read The Metrics Manually

Port-forward the metrics Service:

```bash
kubectl port-forward -n oracle-database-operator-system \
  svc/oracle-database-operator-controller-manager-metrics-service 8443:8443
```

In another terminal, query the endpoint with the ServiceAccount token:

```bash
curl -k -H "Authorization: Bearer <TOKEN>" \
  https://127.0.0.1:8443/metrics
```

Use `-k` because the metrics certificate is self-signed by the example issuer.

If the endpoint is working, the output will include metrics similar to:

```text
controller_runtime_reconcile_total
workqueue_depth
go_goroutines
process_cpu_seconds_total
```

## 5. Optional: Quick Validation Checks

The following commands are useful for a quick test:

```bash
curl -k -H "Authorization: Bearer <TOKEN>" \
  https://127.0.0.1:8443/metrics
```

```bash
curl -k -H "Authorization: Bearer <TOKEN>" \
  https://127.0.0.1:8443/metrics
```

## 6. Prometheus Scrape Example

For Prometheus, you still do not need to create certificates manually. The main
requirements are:

1. Prometheus must be allowed to call `/metrics`
2. Prometheus must scrape the Service over HTTPS
3. For a simple test setup, TLS verification can be skipped

The following example `ServiceMonitor` is suitable for testing:

```yaml
apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
metadata:
  name: oracle-database-operator-controller-manager
  namespace: oracle-database-operator-system
  labels:
    release: prometheus
spec:
  selector:
    matchLabels:
      control-plane: controller-manager
  namespaceSelector:
    matchNames:
      - oracle-database-operator-system
  endpoints:
    - port: https
      path: /metrics
      scheme: https
      bearerTokenFile: /var/run/secrets/kubernetes.io/serviceaccount/token
      tlsConfig:
        insecureSkipVerify: true
```

For this scrape to succeed, bind the Prometheus ServiceAccount to the
`oracle-database-operator-metrics-reader` ClusterRole.

After applying the `ServiceMonitor`, verify metrics in Prometheus with queries
such as:

```promql
controller_runtime_reconcile_total
```

```promql
workqueue_depth
```

## Troubleshooting

If metrics do not work as expected:

1. Check the operator Service:
   ```bash
   kubectl get svc -n oracle-database-operator-system
   ```
2. Check the certificate and secret:
   ```bash
   kubectl get certificate,secret -n oracle-database-operator-system
   ```
3. Check the operator Pod:
   ```bash
   kubectl get pods -n oracle-database-operator-system
   kubectl describe pod -n oracle-database-operator-system <operator-pod-name>
   ```
4. Check the operator logs:
   ```bash
   kubectl logs -n oracle-database-operator-system <operator-pod-name>
   ```
5. Confirm the caller has RBAC permission to `GET /metrics`

## Summary

To test operator metrics:

1. Install the operator manifest
2. Verify the metrics Service and certificate
3. Create a reader ServiceAccount and bind it to the metrics reader ClusterRole
4. Port-forward the metrics Service
5. Query `https://127.0.0.1:8443/metrics` with a bearer token

No manual certificate creation is required when using the updated
`oracle-database-operator.yaml`.
