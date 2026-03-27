# OrdsSrvs Controller: Automatic Pool Probing (Experimental)

> **⚠️Warning⚠️**  
> Automatic pool probing is currently experimental. It is disabled by default, not available with Central Configuration Server, and the current implementation uses HTTPS probing without certificate verification.

This example shows how to enable automatic pool probing in the `OrdsSrvs` controller and how to read the probe results from the `OrdsSrvs` resource status.

Pool probing verifies configured pools by checking the ORDS pool landing URL and reporting the aggregated health in the custom resource status.

Before testing this example, please verify the prerequisites: [OrdsSrvs prerequisites](../README.md#prerequisites)

## How It Works

For each configured pool, the controller probes:

```text
https://<ORDSSRVS>:8443/ords/<POOLNAME>/
```

The result is exposed through:

* `status.poolProbes`
* `status.poolsHealth`
* `status.poolsValid`

## Enable Pool Probing

Add `poolProbeIntervalSeconds` to your `OrdsSrvs` resource. For example:

```yaml
apiVersion: database.oracle.com/v4
kind: OrdsSrvs
metadata:
  name: ordssrvs-probe
  namespace: ordsnamespace
spec:
  image: container-registry.oracle.com/database/ords:latest
  forceRestart: true
  poolProbeIntervalSeconds: 300
  globalSettings:
    standalone.http.port: 8080
  poolSettings:
    - poolName: default
      autoUpgradeORDS: true
      db.connectionType: customurl
      db.customURL: jdbc:oracle:thin:@//<host>:<port>/<service_name>
      db.username: ORDS_PUBLIC_USER
      db.secret:
        secretName: db-auth
      db.adminUser: SYS
      db.adminUser.secret:
        secretName: db-admin-auth
```

Apply the manifest:

```bash
kubectl apply -f ordssrvs-probe.yaml
```

## View Probe Status

Watch the `OrdsSrvs` resource:

```bash
kubectl get -n testcase ordssrvs
```

Example output:

```text
NAME               STATUS    POOLSHEALTH   POOLSVALID   WORKLOADTYPE   ORDSVERSION   HTTPPORT   HTTPSPORT   MONGOPORT   RESTARTREQUIRED   AGE     ORDSINSTALLED
ordssrvs-base      Healthy   Healthy       1/1          Deployment     latest        8080       8443        27017                         3h26m   true
ordssrvs-cc        Healthy   Disabled      0/0          Deployment     latest        8080       8443                                      3h23m   true
ordssrvs-partial   Partial   Partial       2/3          Deployment     latest        8080       8443                                      3h24m   true
ordssrvs-wallets   Healthy   Healthy       4/4          Deployment     latest        8080       8443                                      3h25m   true
```

## Interpreting the Status

* `POOLSHEALTH=Healthy` means all configured pools are currently passing probes.
* `POOLSHEALTH=Partial` means only some configured pools are passing probes.
* `POOLSHEALTH=Disabled` means probing is not active.
* `POOLSVALID` reports the number of healthy pools over the total number of configured pools.
* `STATUS=Partial` can indicate that the ORDS workload itself is healthy, but one or more pools are failing probes.

In the example output:

* `ordssrvs-wallets` shows `4/4`, so all four pools are healthy.
* `ordssrvs-partial` shows `2/3`, so one pool is failing.
* `ordssrvs-cc` shows `Disabled 0/0`, because probing is disabled when Central Configuration Server is enabled.

## Inspect Detailed Probe Results

To see which pool is failing and why, describe the `OrdsSrvs` resource:

```bash
kubectl describe -n testcase ordssrvs ordssrvs-partial
```

Example excerpt:

```text
Status:
  Conditions:
    Message:  Some pools failing probes
    Reason:   SomePoolsFailing
    Status:   False
    Type:     PoolsHealthy
  Pool Probes:
    Display:   positive|https://ordssrvs-partial.testcase.svc:8443/ords/positive/|(302)|OK
    Outcome:   OK
    Pool Name: positive
    Display:   badpool|https://ordssrvs-partial.testcase.svc:8443/ords/badpool/|(574)|SERVER_ERROR
    Outcome:   SERVER_ERROR
    Pool Name: badpool
    Display:   goodpool|https://ordssrvs-partial.testcase.svc:8443/ords/goodpool/|(302)|OK
    Outcome:   OK
    Pool Name: goodpool
  Pools Health: Partial
  Pools Valid:  2/3
  Status:       Partial
```

What to look for:

* `Status: Partial` shows the overall `OrdsSrvs` status when the workload is available but not all pools are passing probes.
* `Pools Health: Partial` and `Pools Valid: 2/3` show that two out of three configured pools are healthy.
* The `PoolsHealthy` condition with `Reason: SomePoolsFailing` confirms that probing is running and at least one pool is currently unhealthy.
* `status.poolProbes` lists each pool separately, including the probe URL, returned HTTP status, and the final outcome.
* In this example, the `badpool` pool returns `SERVER_ERROR`, while `positive` and `goodpool` both return `OK`.

## Notes

* Pool probing only applies to pools defined directly in `spec.poolSettings`.
* If `spec.poolProbeIntervalSeconds` is omitted or set to `0`, probing remains disabled.
* When using Central Configuration Server, pool probing is disabled because the controller does not have the pool list locally.
