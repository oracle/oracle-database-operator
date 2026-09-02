# Pool Probing

Starting with OraOperator 2.3, this opt-in feature checks whether
the ORDS pool aliases configured in an `OrdsSrvs` custom resource are reachable
through the OrdsSrvs Service. It is disabled by default.

Pool reachability is independent of Kubernetes lifecycle probes. Kubernetes
executes the lifecycle probes, while the OrdsSrvs controller executes the
pool-reachability probes and records their results. The two signals answer
different questions:

| Signal | Checks | Can change |
|---|---|---|
| Kubernetes lifecycle probes | Kubernetes checks whether the ORDS process responds on its local HTTP or HTTPS listener | Pod readiness, container restarts, workload availability, and `status.status` |
| Pool reachability probes | The OrdsSrvs controller checks whether each configured pool alias responds through the OrdsSrvs Service | `status.poolProbes`, `status.poolsHealth`, and `status.poolsReachable` only |

A failed pool does not make a healthy ORDS Pod unready and does not restart the
container. See [Kubernetes Liveness, Readiness, and Startup Probes](./lifecycle_probes.md) for Pod
lifecycle behavior.

## Enable Pool Probing

Set a non-zero interval on an `OrdsSrvs` resource with pools defined directly in
`spec.poolSettings`:

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
use because the custom resource does not contain the authoritative pool list.

## Probe URL and Outcomes

For each pool in `spec.poolSettings`, the controller sends a `GET` request to:

```text
<scheme>://<ordssrvs>.<namespace>.svc:<port>/<context-path>[/<pool-alias>/]
```

The default pool uses only the context path, for example
`/ords/`. A non-default pool adds its alias, for example `/ords/pdb1/`. The
scheme, port, and context path come from the OrdsSrvs configuration. The
request uses `Host: localhost`, as do the Kubernetes lifecycle probes.

| Response or error | `outcome` | Counted as reachable |
|---|---|---|
| Any `3xx` response, including a redirect | `OK` | Yes |
| `404 Not Found` | `POOL_NOT_FOUND` | No |
| Any `5xx` response | `SERVER_ERROR` | No |
| Connection error or timeout | `ERROR` | No |
| Any other HTTP response | `UNEXPECTED` | No |

For HTTPS, the controller disables certificate verification for this local
OrdsSrvs Service request so the default self-signed certificate does not block
the probe. This check is reachability validation, not certificate validation.

## View Pool Health

The `STATUS` column is workload health. `POOLSHEALTH` and `POOLS` are the
separate pool-probe summary:

```bash
# OrdsSrvs resources: compare workload health with pool health
kubectl get ordssrvs -n $NAMESPACE

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

## Inspect Detailed Results

Read the latest per-pool result from the custom resource. The command prints
the pool alias, outcome, HTTP status code, and check time:

```bash
# Pool-probe details for an OrdsSrvs resource
kubectl get ordssrvs ordssrvs-partial -n $NAMESPACE \
  -o jsonpath='{range .status.poolProbes[*]}{"POOL "}{.poolName}{" OUTCOME="}{.outcome}{" HTTP_STATUS="}{.httpStatusCode}{" LAST_CHECKED="}{.lastChecked}{"\n"}{end}'

POOL positive OUTCOME=OK HTTP_STATUS=302 LAST_CHECKED=2026-08-31T11:54:51Z
POOL negative OUTCOME=SERVER_ERROR HTTP_STATUS=574 LAST_CHECKED=2026-08-31T11:54:51Z
```

`status.poolProbes` contains `poolName`, `outcome`, `httpStatusCode` when an
HTTP response was received, and `lastChecked`. A connection error or timeout
has no HTTP response and therefore has an HTTP status code of `0`.

For the complete OrdsSrvs resource details, including every pool-probe result,
use `kubectl describe`:

```bash
# OrdsSrvs resource: show pool results and aggregate pool health
kubectl describe ordssrvs ordssrvs-partial -n $NAMESPACE

...
Status:
  Pool Probes:
    Http Status Code: 302
    Last Checked:      2026-08-31T11:54:51Z
    Outcome:           OK
    Pool Name:         positive
    Http Status Code: 574
    Last Checked:      2026-08-31T11:54:51Z
    Outcome:           SERVER_ERROR
    Pool Name:         negative
  Pools Health:        Partial
  Pools Reachable:     1/2
  Status:              Healthy
```

## Empirical Service-Endpoint Test

The following non-production test deliberately changes the generated Service
selector so that it selects no Pods. It simulates the generic reachability
symptom that could result from a network error or a database error: the
controller cannot reach the pool through the Service. It validates the
controller's handling of a Service with no endpoints, but does not reproduce a
specific network or database failure.

Generate the reachability failure by changing the Service selector to a label
that no ORDS Pod has:

```bash
# Service: remove all selected endpoints for the test
kubectl patch service ordssrvs-negative -n $NAMESPACE --type merge -p '{"spec":{"selector":{"app":"ordssrvs-negative-no-endpoints"}}}'

service/ordssrvs-negative patched
```

After the next pool-probe interval, check that pool health is `Unhealthy` while
the workload remains `Healthy`:

```bash
# OrdsSrvs resource: check the reachability failure
kubectl get ordssrvs ordssrvs-negative -n $NAMESPACE

NAME              STATUS   POOLSHEALTH   POOLS   WORKLOADTYPE   HTTPPORT   HTTPSPORT   MONGOPORT   AGE
ordssrvs-negative Healthy  Unhealthy     0/1     Deployment     8080       8443                    13m
```

Restore the generated selector immediately after the check:

```bash
# Service: restore the normal OrdsSrvs selector
kubectl patch service ordssrvs-negative -n $NAMESPACE --type merge -p '{"spec":{"selector":{"app":"ordssrvs-negative","app.kubernetes.io/instance":"ordssrvs-negative","oracle.com/ords-operator-filter":"oracle-database-operator"}}}'

service/ordssrvs-negative patched
```

## Limitations

* Pool probing applies only to pools defined directly in `spec.poolSettings`.
* `poolProbeIntervalSeconds: 0` disables probing. This is the default.
* The controller waits for workload `Healthy` before probing. Before the first
  result, an enabled resource reports `Unknown`.
* With Central Configuration Server, pool probing reports `Disabled` because
  the controller does not have the authoritative pool list in the custom
  resource.
* Pool health does not change lifecycle probes, Pod readiness, workload
  availability, `status.status`, or container restarts.
