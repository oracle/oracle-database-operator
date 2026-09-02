# Kubernetes Liveness, Readiness, and Startup Probes

The OrdsSrvs controller automatically configures startup, readiness, and liveness HTTP probes on the main ORDS container.  
It configures the endpoint, scheme, port, and timing for each probe. They are enabled by default and use the same local endpoint with different failure thresholds.   
*(Available since OraOperator 2.3.)*  

A probe request has this form:
```text
<scheme>://<pod-ip>:<port>/<path>
```

For example, the default HTTPS configuration is:
```text
https://<pod-ip>:8443/favicon.ico
```

Kubernetes executes the probe request and evaluates the result. The controller only configures the probes.

| Probe | Path | Timeout | Period | Consecutive failures before action | Failed result |
|---|---|---:|---:|---:|---|
| Startup | `/favicon.ico` | 3 seconds | 10 seconds | 30 | Kubernetes restarts the ORDS container. |
| Readiness | `/favicon.ico` | 3 seconds | 10 seconds | 3 | The Pod is not `Ready` and is removed from ready Service endpoints. |
| Liveness | `/favicon.ico` | 3 seconds | 10 seconds | 18 | Kubernetes restarts the ORDS container. |

`/ords/` could be used as a probe path, but ORDS redirects that request. Kubernetes accepts the redirect as a positive result but reports a noisy warning in `kubectl describe pod`. `/favicon.ico` is a static ORDS file that returns HTTP `200 OK`.  

Lifecycle probes check the Kubernetes-facing ORDS listener only. They do not test database or pool reachability: an unavailable database pool must not make a Pod unready or cause ORDS to restart. See [Pool Probing](./pool_probing.md) for the separate pool-health feature.  

## Optional Lifecycle-Probe Configuration

The controller uses the default values above when `probePath` and `probeSettings` are omitted. To override them, set only the fields required by your environment.

> Set `probePath: ""` to disable Kubernetes lifecycle probes.

For example, reduce the startup allowance to three minutes while retaining the default ten-second probe period:

```yaml
spec:
  probeSettings:
    startupFailureThreshold: 18
```

## Test Configuration

The following resource creates three ORDS replicas for lifecycle-probe testing. Replace `ORDSIMG` and `CONNECTSTRING` before applying it. The shortened timeout, period, and failure-threshold values are for the failure/recovery test; use values appropriate for your environment otherwise.

```yaml
apiVersion: database.oracle.com/v4
kind: OrdsSrvs
metadata:
  name: ordssrvs-probes
  namespace: ordsnamespace
spec:
  image: ORDSIMG
  replicas: 3
  probeSettings:
    timeoutSeconds: 1
    periodSeconds: 3
    startupFailureThreshold: 30
    readinessFailureThreshold: 2
    livenessFailureThreshold: 5
  poolSettings:
    - poolName: default
      db.connectionType: customurl
      db.customURL: jdbc:oracle:thin:@//CONNECTSTRING
      db.username: ORDS_PUBLIC_USER
      db.secret:
        secretName: ordssrvs-auth
        passwordKey: dbAuth
```

## OrdsSrvs Workload Status

The OrdsSrvs `status.status` value reports workload availability. For Deployments, the controller uses `availableReplicas`. For StatefulSets and DaemonSets, it uses ready replica counts.

| Status | Lifecycle stage | Meaning |
|---|---|---|
| `Preparing` | Initial startup | Zero replicas are available or ready. |
| `Progressing` | Initial startup | Some replicas are available or ready. |
| `Healthy` | Running | All desired replicas are available or ready. |
| `Degraded` | Previously healthy | Some replicas are unavailable or not ready. |
| `Unhealthy` | Previously healthy | Zero replicas are available or ready. |

## Inspect OrdsSrvs Workload Status

Use the custom resource together with its child workload objects to inspect lifecycle-probe effects. The OrdsSrvs resource reports the aggregate workload status; the Deployment and ReplicaSet report replica availability; Pods and EndpointSlices show which replicas are ready to receive Service traffic.

The following representative outputs show a healthy three-replica resource and its child objects:

### OrdsSrvs resource

Displays the controller-level workload status and configured workload type.

```bash
# OrdsSrvs resource
kubectl get ordssrvs ordssrvs-probes -n ordsnamespace

NAME              STATUS    POOLSHEALTH   POOLS   WORKLOADTYPE   HTTPPORT   HTTPSPORT   MONGOPORT   AGE
ordssrvs-probes   Healthy   Disabled              Deployment     8080       8443                    29m
```

### Deployment

Displays the desired, ready, updated, and available replica counts.

```bash
# Deployment
kubectl get deployment ordssrvs-probes -n ordsnamespace

NAME              READY   UP-TO-DATE   AVAILABLE   AGE
ordssrvs-probes   3/3     3            3           29m
```

### ReplicaSet

Displays the replica counts maintained by the Deployment's current ReplicaSet.

```bash
# ReplicaSet
kubectl get replicaset -n ordsnamespace -l app.kubernetes.io/instance=ordssrvs-probes

NAME                        DESIRED   CURRENT   READY   AGE
ordssrvs-probes-d7c9998f8   3         3         3       29m
```

### Pods

Displays each Pod's readiness and runtime state.

```bash
# Pods
kubectl get pods -n ordsnamespace -l app.kubernetes.io/instance=ordssrvs-probes

NAME                              READY   STATUS    RESTARTS   AGE
ordssrvs-probes-d7c9998f8-4cmmf   1/1     Running   0          29m
ordssrvs-probes-d7c9998f8-5xscz   1/1     Running   0          29m
ordssrvs-probes-d7c9998f8-b72g2   1/1     Running   0          29m
```

### EndpointSlices

Displays whether each Pod endpoint is ready to receive Service traffic.

```bash
# EndpointSlices
kubectl get endpointslice -n ordsnamespace \
  -l kubernetes.io/service-name=ordssrvs-probes \
  -o jsonpath='{range .items[*].endpoints[*]}{"ENDPOINT "}{.addresses[0]}{" POD="}{.targetRef.name}{" ready="}{.conditions.ready}{" serving="}{.conditions.serving}{" terminating="}{.conditions.terminating}{"\n"}{end}'

ENDPOINT 10.244.1.119 POD=ordssrvs-probes-d7c9998f8-b72g2 ready=true serving=true terminating=false
ENDPOINT 10.244.1.118 POD=ordssrvs-probes-d7c9998f8-4cmmf ready=true serving=true terminating=false
ENDPOINT 10.244.1.117 POD=ordssrvs-probes-d7c9998f8-5xscz ready=true serving=true terminating=false
```

## Empirical Testing in a Non-Production Environment

In this example, we use `SIGSTOP` as a controlled simulation of generic ORDS unavailability. It suspends the ORDS Java process without terminating the container, allowing readiness and liveness behavior to be observed. Use `SIGSTOP` only in a non-production environment.

Before each test, confirm that all three replicas and Service endpoints are ready. Use the inspection commands in [Inspect OrdsSrvs Workload Status](#inspect-ordssrvs-workload-status) before and after suspending Java.

### One-Pod Failure

Suspend Java in one of the three Pods. Readiness removes that Pod from Service traffic before liveness restarts its container.

```bash
# Initial Pods
kubectl get pods -n ordsnamespace -l app.kubernetes.io/instance=ordssrvs-probes

NAME                              READY   STATUS    RESTARTS   AGE
ordssrvs-probes-d7c9998f8-4cmmf   1/1     Running   0          29m
ordssrvs-probes-d7c9998f8-5xscz   1/1     Running   0          29m
ordssrvs-probes-d7c9998f8-b72g2   1/1     Running   0          29m
```

Replace `POD` with one of these Pod names, then suspend its Java process:

```bash
# Suspend Java in one Pod
kubectl exec -n ordsnamespace POD -c ordssrvs-main -- bash -c \
  'for proc in /proc/[0-9]*; do read -r name < "$proc/comm" 2>/dev/null || continue; [ "$name" = java ] || continue; pid=${proc##*/}; echo "Stopping Java PID $pid"; kill -STOP "$pid"; exit 0; done; echo "Java process not found" >&2; exit 1'

Stopping Java PID 8
```

After the readiness failure threshold:

```bash
# OrdsSrvs resource
kubectl get ordssrvs ordssrvs-probes -n ordsnamespace

NAME              STATUS     POOLSHEALTH   POOLS   WORKLOADTYPE   HTTPPORT   HTTPSPORT   MONGOPORT   AGE
ordssrvs-probes   Degraded   Disabled              Deployment     8080       8443                    29m
```

```bash
# Deployment
kubectl get deployment ordssrvs-probes -n ordsnamespace

NAME              READY   UP-TO-DATE   AVAILABLE   AGE
ordssrvs-probes   2/3     3            2           29m
```

```bash
# Pods
kubectl get pods -n ordsnamespace -l app.kubernetes.io/instance=ordssrvs-probes

NAME                              READY   STATUS    RESTARTS   AGE
ordssrvs-probes-d7c9998f8-4cmmf   0/1     Running   0          29m
ordssrvs-probes-d7c9998f8-5xscz   1/1     Running   0          29m
ordssrvs-probes-d7c9998f8-b72g2   1/1     Running   0          29m
```

```bash
# EndpointSlices
kubectl get endpointslice -n ordsnamespace \
  -l kubernetes.io/service-name=ordssrvs-probes \
  -o jsonpath='{range .items[*].endpoints[*]}{"ENDPOINT "}{.addresses[0]}{" POD="}{.targetRef.name}{" ready="}{.conditions.ready}{" serving="}{.conditions.serving}{" terminating="}{.conditions.terminating}{"\n"}{end}'

ENDPOINT 10.244.1.119 POD=ordssrvs-probes-d7c9998f8-b72g2 ready=true serving=true terminating=false
ENDPOINT 10.244.1.118 POD=ordssrvs-probes-d7c9998f8-4cmmf ready=false serving=false terminating=false
ENDPOINT 10.244.1.117 POD=ordssrvs-probes-d7c9998f8-5xscz ready=true serving=true terminating=false
```

One endpoint is no longer serving Service traffic.

After the liveness failure threshold, Kubernetes restarts the suspended container:

```bash
# OrdsSrvs resource after liveness recovery
kubectl get ordssrvs ordssrvs-probes -n ordsnamespace

NAME              STATUS    POOLSHEALTH   POOLS   WORKLOADTYPE   HTTPPORT   HTTPSPORT   MONGOPORT   AGE
ordssrvs-probes   Healthy   Disabled              Deployment     8080       8443                    30m
```

```bash
# Deployment after liveness recovery
kubectl get deployment ordssrvs-probes -n ordsnamespace

NAME              READY   UP-TO-DATE   AVAILABLE   AGE
ordssrvs-probes   3/3     3            3           30m
```

### All-Pods Failure

Suspend Java in every ORDS Pod by running the one-Pod `kubectl exec ... kill -STOP` command once for each Pod. No endpoint remains ready, and the workload is reported as `Unhealthy` until liveness restarts the containers.

```bash
# Initial Pods
kubectl get pods -n ordsnamespace -l app.kubernetes.io/instance=ordssrvs-probes

NAME                              READY   STATUS    RESTARTS   AGE
ordssrvs-probes-d7c9998f8-4cmmf   1/1     Running   1          30m
ordssrvs-probes-d7c9998f8-5xscz   1/1     Running   0          30m
ordssrvs-probes-d7c9998f8-b72g2   1/1     Running   0          30m
```

After every Pod has failed readiness:

```bash
# OrdsSrvs resource
kubectl get ordssrvs ordssrvs-probes -n ordsnamespace

NAME              STATUS      POOLSHEALTH   POOLS   WORKLOADTYPE   HTTPPORT   HTTPSPORT   MONGOPORT   AGE
ordssrvs-probes   Unhealthy   Disabled              Deployment     8080       8443                    30m
```

```bash
# Deployment
kubectl get deployment ordssrvs-probes -n ordsnamespace

NAME              READY   UP-TO-DATE   AVAILABLE   AGE
ordssrvs-probes   0/3     3            0           30m
```

```bash
# Pods
kubectl get pods -n ordsnamespace -l app.kubernetes.io/instance=ordssrvs-probes

NAME                              READY   STATUS    RESTARTS   AGE
ordssrvs-probes-d7c9998f8-4cmmf   0/1     Running   1          30m
ordssrvs-probes-d7c9998f8-5xscz   0/1     Running   0          30m
ordssrvs-probes-d7c9998f8-b72g2   0/1     Running   0          30m
```

```bash
# EndpointSlices
kubectl get endpointslice -n ordsnamespace \
  -l kubernetes.io/service-name=ordssrvs-probes \
  -o jsonpath='{range .items[*].endpoints[*]}{"ENDPOINT "}{.addresses[0]}{" POD="}{.targetRef.name}{" ready="}{.conditions.ready}{" serving="}{.conditions.serving}{" terminating="}{.conditions.terminating}{"\n"}{end}'

ENDPOINT 10.244.1.119 POD=ordssrvs-probes-d7c9998f8-b72g2 ready=false serving=false terminating=false
ENDPOINT 10.244.1.118 POD=ordssrvs-probes-d7c9998f8-4cmmf ready=false serving=false terminating=false
ENDPOINT 10.244.1.117 POD=ordssrvs-probes-d7c9998f8-5xscz ready=false serving=false terminating=false
```

After the liveness failure threshold, Kubernetes restarts the containers:

```bash
# OrdsSrvs resource after liveness recovery
kubectl get ordssrvs ordssrvs-probes -n ordsnamespace

NAME              STATUS    POOLSHEALTH   POOLS   WORKLOADTYPE   HTTPPORT   HTTPSPORT   MONGOPORT   AGE
ordssrvs-probes   Healthy   Disabled              Deployment     8080       8443                    31m
```

```bash
# Deployment after liveness recovery
kubectl get deployment ordssrvs-probes -n ordsnamespace

NAME              READY   UP-TO-DATE   AVAILABLE   AGE
ordssrvs-probes   3/3     3            3           31m
```
