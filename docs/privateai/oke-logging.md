# Read Private AI Logs From stdout On OKE

This guide explains the recommended way to read Oracle Private AI logs on OKE:
send the main container `stdout` and `stderr` stream to a cluster logging
backend.

For OKE, the primary documented backend is OCI Logging.

This guide also notes how the same `stdout` and `stderr` stream can be consumed
by Loki if your cluster already uses a Loki-based logging stack.

## Recommendation

As of today, the recommended default path is:

1. Private AI writes logs to `stdout` and `stderr`
2. Kubernetes exposes those logs through `kubectl logs`
3. OKE workload logging forwards them to OCI Logging

This should be the default documentation path.

## What This Means For The Private AI CR

You do not need any extra Private AI logging configuration for the default OKE
logging path.

You also do not need to configure file-based log shipping in the `PrivateAi`
resource just to get logs into OKE telemetry.

The only `PrivateAi` field that may still matter for logging behavior is:

- `spec.runtime.env`

Use `spec.runtime.env` only if the Private AI container image supports
environment variables that change log level, log format, or application logging
behavior.

## Before You Begin

Make sure:

1. Oracle Database Operator is installed on OKE
2. Your `PrivateAi` resource is deployed and healthy
3. You can access the cluster with `kubectl`
4. OKE workload logging is available for your cluster in OCI

## Step 1: Verify The Main Container Logs

List the Private AI Pods:

```bash
kubectl get pods -n pai
```

Read the logs from the main container:

```bash
kubectl logs -n pai pod/<pod-name>
```

If the logs you want are visible here, this is enough to use OKE workload
logging. No extra Private AI log collector configuration is needed.

## Step 2: Generate A Test Request

Use one of the following paths to check service connectivity. In each case, use the Service port or Load Balancer port configured for your deployment.

### Port Forwarding

Check the Service port:

```sh
kubectl get service "$NAME" -n "$NAMESPACE"
```

Start port forwarding in one terminal. Set `SERVICE_PORT` to the Service port shown by the command above. For example, use `443` if the Service exposes `443`, or `8443` if it exposes `8443`:

```sh
export SERVICE_PORT=443
kubectl port-forward -n "$NAMESPACE" "svc/$NAME" "8443:$SERVICE_PORT"
```

In another terminal, request the health endpoint through the local forwarded port:

```sh
curl -k -i -v https://127.0.0.1:8443/health
```

The local port is `8443`; the remote port in the port-forward command must match the Service port. For example, if the Service exposes port `443`, use `8443:443`. If it exposes port `8443`, use `8443:8443`.

### Internal Load Balancer

From a client that can reach the private network, request the endpoint using the reserved private IP. This example uses `xx.xx.xx.xx` and the Load Balancer's HTTPS port `443`:

```sh
curl -k -i -v https://xx.xx.xx.xx/health
```

Replace the example IP and port with the values configured for your deployment.

### Public Load Balancer

From a client that can reach the public endpoint, set `PUBLIC_ENDPOINT` to the public Load Balancer IP or DNS name. This example assumes the Load Balancer listens on HTTPS port `443`:

```sh
export PUBLIC_ENDPOINT="xxx.xxx.xxx.xxx"
curl -k -i -v "https://$PUBLIC_ENDPOINT/health"
```

Replace the example address with the public IP or DNS name configured for the deployment. 

The `/health` request checks service health. It may not create an application log entry. After generating a request, check `kubectl logs` again to see whether the application emitted a corresponding line.

## Step 3: Enable OKE Workload Logging

In OCI, enable workload logging for the OKE cluster and send the Kubernetes
container logs to an OCI Log Group.

At a minimum, ensure that you can filter or search by the following:

- namespace: `pai`
- Pod name
- container name

For the default path, search the main Private AI application container logs.

## Step 4: Read The Logs In OCI Logging

After workload logging is enabled and log traffic exists, search for the
Private AI logs in OCI Logging.

Use these filters:

1. the OKE cluster
2. namespace `pai`
3. the relevant Pod name
4. the main application container

If the same lines are visible in both `kubectl logs` and OCI Logging, then the OKE
telemetry path is working.

## Step 5: Troubleshoot Missing Logs

If logs are missing in OCI Logging, then check for the cause in this order.

### 1. Verify `kubectl logs` first

```bash
kubectl logs -n pai pod/<pod-name>
```

If the log line is not visible here, then OKE workload logging cannot collect it.

### 2. Verify the application is really writing to stdout/stderr

The recommended path assumes the application logs go to the main container log
stream.

If the application writes only to files, then those file logs will not automatically
appear in OKE workload logging unless a separate cluster-side or application-side
solution is introduced.

### 3. Verify cluster-side logging is enabled

Confirm that workload logging is enabled for the OKE cluster and that logs are
being delivered to the expected OCI Log Group.

### 4. Verify you are searching the correct container

Search the main Private AI application container, not just the Pod name or
service name.

## Alternative Backend: Loki

If your cluster already uses Loki, then you can use the same `stdout` and `stderr`
stream as the source for Loki ingestion.

The pattern is:

1. Private AI writes logs to `stdout` and `stderr`
2. A cluster logging agent such as Promtail or Grafana Agent scrapes container
   logs from Kubernetes
3. The agent pushes those logs to Loki

This is a cluster logging architecture choice, not a feature managed directly by
the Private AI controller in this repo.

So the documentation boundary is:

- `PrivateAi`: produce useful logs on `stdout` and `stderr`
- OKE / cluster logging stack: route those container logs to OCI Logging or Loki

## Summary

The default and recommended flow is:

1. Confirm the Private AI main container logs are visible with `kubectl logs`
2. Generate a test request
3. Enable OKE workload logging
4. Read the same log lines in OCI Logging

If your organization uses Loki instead of OCI Logging, use the same main
container `stdout` and `stderr` stream and let the cluster logging agent ship it
to Loki.
