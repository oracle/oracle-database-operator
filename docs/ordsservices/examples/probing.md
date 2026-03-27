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
    database.api.enabled: true
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
ordssrvs-adb       Healthy   Healthy       1/1          Deployment     latest        8080       8443                                      5h3m    true
ordssrvs-apexdl    Healthy   Healthy       1/1          Deployment     latest        8080       8443                                      5h4m    true
ordssrvs-apexpv    Healthy   Healthy       1/1          Deployment     latest        8080       8443                                      5h11m   true
ordssrvs-base      Healthy   Disabled      0/0          Deployment     latest        8080       8443        27017                         5h14m   true
ordssrvs-cc        Healthy   Disabled      0/0          Deployment     latest        8080       8443                                      5h12m   true
ordssrvs-enc       Healthy   Healthy       1/1          Deployment     latest        8080       8443                                      5h13m   true
ordssrvs-partial   Partial   Partial       2/3          Deployment     latest        8080       8443                                      5h12m   true
ordssrvs-wallets   Healthy   Healthy       4/4          Deployment     latest        8080       8443                                      5h14m   true
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

## Notes

* Pool probing only applies to pools defined directly in `spec.poolSettings`.
* If `spec.poolProbeIntervalSeconds` is omitted or set to `0`, probing remains disabled.
* When using Central Configuration Server, pool probing is disabled because the controller does not have the pool list locally.
