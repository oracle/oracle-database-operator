# Pool Probing

This opt-in feature checks whether the ORDS pool aliases configured in an
`OrdsSrvs` custom resource are reachable through the OrdsSrvs Service. 

> **Note:** Pool probing is available starting with OraOperator 2.3. It is
disabled by default.

Pool reachability is independent of Kubernetes lifecycle probes. Kubernetes
executes the lifecycle probes, while the OrdsSrvs controller executes the
pool-reachability probes and records their results. The two signals answer
different questions:

| Signal | Checks | Effect | Relevant status fields |
|---|---|---|---|
| Kubernetes lifecycle probes | Kubernetes checks whether the ORDS process responds on its local HTTP or HTTPS listener | Readiness failure removes the Pod from Service endpoints. Liveness failure restarts the container. | `status.status`, `status.conditions` |
| Pool reachability probes | The OrdsSrvs controller checks whether each configured pool alias responds through the OrdsSrvs Service | No Kubernetes action is taken; results are recorded in the OrdsSrvs status. | `status.poolProbes`, `status.poolsHealth`, `status.poolsReachable`, `status.poolsTotal`, `status.poolsOK`, `status.poolsFailed` |

A failed pool does not make an OrdsSrvs Pod unready or restart it.

## Enable Pool Probing

Pool probing applies only to pools listed in `spec.poolSettings`. Enable it by
setting a non-zero interval:

```yaml
spec:
  poolProbeIntervalSeconds: 60
```

The interval is in seconds. The controller first waits until the OrdsSrvs
workload is `Healthy`, then probes each configured pool. It repeats the probes
at the configured interval. Each HTTP request has a fixed three-second
timeout.

`poolProbeIntervalSeconds: 0` disables probing; this is the default. Pool
probing is also reported as `Disabled` when Central Configuration Server is in
use because the custom resource does not contain the definitive pool list.

## Probe URL and Outcomes

For each pool in `spec.poolSettings`, the controller sends a `GET` request to:

```text
Default pool: https://<ordssrvs>.<namespace>.svc:<port>/<context-path>/
Named pool:   https://<ordssrvs>.<namespace>.svc:<port>/<context-path>/<pool-alias>/
```

The default pool uses only the context path, for example
`/ords/`. A non-default pool adds its alias, for example `/ords/pdb1/`. The
trailing slash is required in both cases. HTTPS is enabled by default; the
controller uses HTTP only when HTTPS is disabled. The port and context path
come from the OrdsSrvs configuration.

The controller evaluates the initial response without following redirects.

| HTTP response or error | `outcome` | Reachable |
|---|---|---|
| Any `2xx` or `3xx` response, including a redirect | `OK` | Yes |
| `404 Not Found` | `POOL_NOT_FOUND` | No |
| Any `5xx` response | `SERVER_ERROR` | No |
| Connection error or timeout | `ERROR` | No |
| Any other HTTP response | `UNEXPECTED` | No |

When `standalone.https.host` is set, the request sends the configured hostname in the HTTP `Host` header (`Host: <standalone.https.host>`). When it is not set, or the deployment is HTTP-only, it uses `Host: localhost`.

For HTTPS, the controller disables certificate verification for this local
OrdsSrvs Service request so the default self-signed certificate does not block
the probe. This check is reachability validation, not certificate validation.

## Pool Probe Status

Probe results are exposed as status fields on the `OrdsSrvs` resource:

| Field | Description |
|---|---|
| `status.poolProbes` | Array containing the latest result for each configured pool. |
| `status.poolProbes[].poolName` | Configured pool alias. |
| `status.poolProbes[].outcome` | Probe result: `OK`, `POOL_NOT_FOUND`, `SERVER_ERROR`, `ERROR`, or `UNEXPECTED`. |
| `status.poolProbes[].httpStatusCode` | HTTP response code; omitted when no HTTP response was received. |
| `status.poolProbes[].lastChecked` | Timestamp of the latest probe for the pool. |
| `status.poolsHealth` | Aggregate pool health: `Healthy`, `Partial`, `Unhealthy`, `Disabled`, or `Unknown`. |
| `status.poolsReachable` | Reachable pools over total evaluated pools, formatted as `n/total`. |
| `status.poolsTotal` | Total number of pools evaluated in the latest probe cycle. |
| `status.poolsOK` | Number of pools with an `OK` outcome in the latest probe cycle. |
| `status.poolsFailed` | Number of pools with a non-`OK` outcome in the latest probe cycle. |

The numeric count fields are omitted when probing is disabled or no result is
available. An evaluated empty pool list reports zero counts. When workload
health prevents a new probe, previous results and counts are retained; use
`lastChecked` to determine result freshness.

## Inspect Pool Health

### Summary

The `STATUS` column is workload health. `POOLSHEALTH` and `POOLS` are the
separate pool-probe summary:

```bash
# OrdsSrvs resources: compare workload health with pool health
kubectl get ordssrvs -n $NAMESPACE
```

```text
NAME                STATUS   POOLSHEALTH   POOLS   WORKLOADTYPE   HTTPPORT   HTTPSPORT   MONGOPORT   AGE
ordssrvs-base       Healthy  Healthy       1/1     Deployment     8080       8443                    18m
ordssrvs-edgehttp   Healthy  Healthy       1/1     Deployment     8080       0                       7m
ordssrvs-wallets    Healthy  Healthy       4/4     Deployment     8080       8443                    9m
ordssrvs-cc         Healthy  Disabled              Deployment     8080       8443                    12m
ordssrvs-partial    Healthy  Partial       2/3     Deployment     8080       8443                    20m
ordssrvs-negative   Healthy  Unhealthy     0/1     Deployment     8080       8443                    2m
```

The example shows that `ordssrvs-negative` remains workload `Healthy` even
though no configured pool is reachable. The workload status is not derived
from pool health.

| `POOLSHEALTH` | `POOLS` | Meaning |
|---|---|---|
| `Healthy` | `n/n` | All configured pools passed their latest probe. |
| `Partial` | `n/total` | Some configured pools passed and some failed. |
| `Unhealthy` | `0/total` | No configured pool passed. |
| `Disabled` | empty | Probing is disabled or unsupported for the resource. |
| `Unknown` | empty | Probing is enabled but has not completed a probe yet. |

When probing is enabled with no configured pools, the summary is `Unknown` and
`POOLS` is `0/0` after the first probe attempt.

### Single-pool details

Use `kubectl describe` to view the single-pool results and aggregate fields:

```bash
kubectl describe -n $NAMESPACE ordssrvs/ordssrvs-partial
```

Example output excerpt:

```text
Status:
  Pool Probes:
    Http Status Code: 302
    Last Checked:      2026-09-29T10:03:24Z
    Outcome:           OK
    Pool Name:         pool1
    Http Status Code: 574
    Last Checked:      2026-09-29T10:03:24Z
    Outcome:           SERVER_ERROR
    Pool Name:         pool2
    Http Status Code: 302
    Last Checked:      2026-09-29T10:03:24Z
    Outcome:           OK
    Pool Name:         pool3
  Pools Failed:        1
  Pools Health:        Partial
  Pools OK:            2
  Pools Reachable:     2/3
  Pools Total:         3
  Status:              Healthy
```

## Empirical Service-Endpoint Test

This non-production test simulates pool unavailability by changing the Service
selector so that it has no matching OrdsSrvs Pods.

Use a resource with probing enabled and healthy pools, such as `ordssrvs-base`.
Confirm that both workload and pool health are `Healthy` before proceeding:

```bash
kubectl get ordssrvs ordssrvs-base -n "$NAMESPACE"
```

```text
NAME              STATUS   POOLSHEALTH   POOLS   WORKLOADTYPE   HTTPPORT   HTTPSPORT   MONGOPORT   AGE
ordssrvs-base     Healthy  Healthy       1/1     Deployment     8080       8443                    12m
```

For this example, the Service's `app` selector is `ordssrvs-base`. Change only
that value to stop matching Pods:

```bash
kubectl patch service ordssrvs-base -n "$NAMESPACE" --type=merge -p='{"spec":{"selector":{"app":"ordssrvs-base-no-endpoints"}}}'
```

```text
service/ordssrvs-base patched
```

After the next pool-probe interval, check that pool health is `Unhealthy` while
the workload remains `Healthy`:

```bash
# OrdsSrvs resource: check the reachability failure
kubectl get ordssrvs ordssrvs-base -n "$NAMESPACE"
```

```text
NAME              STATUS   POOLSHEALTH   POOLS   WORKLOADTYPE   HTTPPORT   HTTPSPORT   MONGOPORT   AGE
ordssrvs-base     Healthy  Unhealthy     0/1     Deployment     8080       8443                    13m
```

Restore the `app` selector immediately after the check:

```bash
# Service: restore the normal OrdsSrvs selector
kubectl patch service ordssrvs-base -n "$NAMESPACE" --type=merge -p='{"spec":{"selector":{"app":"ordssrvs-base"}}}'
```

```text
service/ordssrvs-base patched
```

After the next probe, verify that pool health returns to `Healthy`:

```bash
kubectl get ordssrvs ordssrvs-base -n "$NAMESPACE"
```

```text
NAME              STATUS   POOLSHEALTH   POOLS   WORKLOADTYPE   HTTPPORT   HTTPSPORT   MONGOPORT   AGE
ordssrvs-base     Healthy  Healthy       1/1     Deployment     8080       8443                    14m
```

## Limitations

* The controller waits for workload `Healthy` before probing. Before the first
  result, an enabled resource reports `Unknown`.
* With Central Configuration Server, pool probing reports `Disabled` because
  the controller does not have the definitive pool list in the custom
  resource.
* Pool health does not change lifecycle probes, Pod readiness, workload
  availability, `status.status`, or container restarts.
