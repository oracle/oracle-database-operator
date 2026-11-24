# Pool Probing (Experimental)

This opt-in feature checks whether selected ORDS connection pools are reachable
through the OrdsSrvs Service.

Pool probing is a custom implementation in the OrdsSrvs controller. It reports
results in custom OrdsSrvs status attributes, including `status.poolProbes` and
`status.poolsHealth`.

To enable pool probing, set `poolProbeSettings.intervalSeconds` to a value greater than
`0`. This applies to both HTTP and HTTPS.

> **Note:** HTTPS pool probes do not validate server certificates or hostnames.
> Probes send no credentials or application data and only update reported pool
> health. Pool probing is experimental and disabled by default. Enable it only
> where this limitation is acceptable.

Pool reachability is independent of Kubernetes [container probes](./container_probing.md). Kubernetes
executes the container probes, while the OrdsSrvs controller executes the
pool-reachability probes and records their results. The two signals answer
different questions:

| Signal | Checks | Effect | Relevant status fields |
|---|---|---|---|
| Kubernetes container probes | Kubernetes checks whether the ORDS process responds on its local HTTP or HTTPS listener | Readiness failure removes the Pod from Service endpoints. Liveness failure restarts the container. | `status.status`, `status.conditions` |
| Pool reachability probes | The OrdsSrvs controller checks whether each configured pool alias responds through the OrdsSrvs Service | No Kubernetes action is taken; results are recorded in the OrdsSrvs status. | `status.poolProbes`, `status.poolsHealth`, `status.poolsReachable`, `status.poolsTotal`, `status.poolsOK`, `status.poolsFailed` |

A failed pool does not make an OrdsSrvs Pod unready or restart it.

## Enable Pool Probing

By default, pool probing uses the pool names in `spec.poolSettings`:

```yaml
spec:
  poolProbeSettings:
    intervalSeconds: 60
```

To select the pools to monitor, provide a non-empty `poolProbeSettings.poolList`:

```yaml
spec:
  poolProbeSettings:
    intervalSeconds: 60
    poolList: [poola, poolb]
```

This list overrides `poolSettings` for probing only. It does not configure pools
or trigger a Pod rollout. An empty or omitted list preserves the default
selection.

The interval is in seconds; `0` disables probing and is the default. The
controller waits until the OrdsSrvs workload is `Healthy`, then probes each
selected pool at that interval. Each request has a three-second timeout.

With Central Configuration Server, provide `poolProbeSettings.poolList` to enable probing.
Without a non-empty list, probing remains `Disabled`. The list can include pools
that ORDS has not loaded yet; probing can trigger their initialization.

Central Configuration must use URL-path pool mapping
(`security.externalMappingPathPrefix: true`). Repeat the listener ports,
`standalone.https.host` when set, and `standalone.context.path` in the CR so that
probe requests match the remote configuration. Host-based and custom-header
pool mapping are not supported by pool probing.

> **Note:** Pool probing is available starting with OraOperator 2.3.

## Probe URL and Outcomes

For each selected pool, the controller sends a `GET` request to:

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

## Pool Probe Status

Probe results are exposed as status fields on the `OrdsSrvs` resource:

| Field | Description |
|---|---|
| `status.poolProbes` | Array containing the latest result for each selected pool. |
| `status.poolProbes[].poolName` | Selected pool alias. |
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
ordssrvs-unhealthy  Healthy  Unhealthy     0/1     Deployment     8080       8443                    2m
```

The example shows that `ordssrvs-unhealthy` remains workload `Healthy` even
though no configured pool is reachable. The workload status is not derived
from pool health.

| `POOLSHEALTH` | `POOLS` | Meaning |
|---|---|---|
| `Healthy` | `n/n` | All selected pools passed their latest probe. |
| `Partial` | `n/total` | Some selected pools passed and some failed. |
| `Unhealthy` | `0/total` | No selected pool passed. |
| `Disabled` | empty | The interval is zero, or Central Configuration Server is used without a non-empty `poolProbeSettings.poolList`. |
| `Unknown` | empty | Probing is enabled but has not completed a probe yet. |

When probing is enabled with no selected pools, the summary is `Unknown` and
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

* HTTPS certificate and hostname validation are not supported.
* The controller waits for workload `Healthy` before probing. Before the first
  result, an enabled resource reports `Unknown`.
* Central Configuration Server requires a non-empty `poolProbeSettings.poolList` and
  URL-path pool mapping.
* Pool health does not change container probes, Pod readiness, workload
  availability, `status.status`, or container restarts.
