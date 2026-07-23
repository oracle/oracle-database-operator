# HTTP Access Logs

`OrdsSrvs` can enable HTTP access logs and can optionally persist those HTTP access logs on external storage or forward them to container stdout.

Use HTTP access log persistence when HTTP access logs must survive pod restarts or when HTTP access logs must be retained on external storage for operational review.

Use HTTP access log forwarding when a log collector is configured to read pod stdout/stderr instead of reading files from a mounted directory.

## Enable HTTP Access Logs

Enable HTTP access logging with `spec.globalSettings."enable.standalone.access.log"`:

```yaml
apiVersion: database.oracle.com/v4
kind: OrdsSrvs
metadata:
  name: ordssrvs-logging
  namespace: ordsnamespace
spec:
  image: container-registry.oracle.com/database/ords:<ords-version>
  imagePullPolicy: IfNotPresent
  globalSettings:
    enable.standalone.access.log: true
    standalone.access.log.retainDays: 90
  accessLogPersistence:
    size: 20Gi
    accessMode: ReadWriteMany
    storageClass: ""
    volumeName: acclog-pv
  accessLogForwarder:
    enabled: true
    resources:
      requests:
        cpu: 10m
        memory: 32Mi
      limits:
        memory: 64Mi
  poolSettings:
    - poolName: default
      db.connectionType: customurl
      db.customURL: jdbc:oracle:thin:@//dbhost:1521/FREEPDB1
      db.username: ORDS_PUBLIC_USER
      db.secret:
        secretName: ordssrvs-auth
        passwordKey: dbAuth
  serviceAccountName: ordssrvs-sa
```

The controller writes the `standalone.access.log` setting to the global ORDS configuration:

```xml
<entry key="standalone.access.log">/opt/oracle/sa/log/global</entry>
```

ORDS writes HTTP access log files under:

```text
/opt/oracle/sa/log/global
```

The controller does not expose Jetty XML customization for HTTP access log file naming.

## Retention

Use `spec.globalSettings."standalone.access.log.retainDays"` to set HTTP access log retention:

```yaml
spec:
  globalSettings:
    enable.standalone.access.log: true
    standalone.access.log.retainDays: 90
```

The controller writes this value to ORDS global settings:

```xml
<entry key="standalone.access.log.retainDays">90</entry>
```

## HTTP Access Log Persistence

`spec.accessLogPersistence` controls the PVC used for HTTP access logs.

The controller creates the PVC only when persistence is requested by setting at least one `accessLogPersistence` attribute: `size`, `accessMode`, `storageClass`, or `volumeName`. An empty `accessLogPersistence: {}` block does not create a PVC.

When HTTP access log persistence is enabled, the controller creates a PVC named:

```text
<ordssrvs-name>-access-log-pvc
```

The PVC is mounted in the ORDS init and runtime containers at:

```text
/opt/oracle/sa/log/global
```

Example:

```yaml
spec:
  accessLogPersistence:
    size: 20Gi
    accessMode: ReadWriteMany
    storageClass: ""
    volumeName: acclog-pv
```

### Persistence Attributes

| Attribute | Description |
| --- | --- |
| `size` | Requested PVC size. Defaults to `2Gi` when omitted. |
| `accessMode` | PVC access mode. Defaults to `ReadWriteOnce` when omitted. Use `ReadWriteMany` for more than one pod. |
| `storageClass` | StorageClass name. If omitted or empty, the PVC is created with `storageClassName: ""` and does not use the cluster default StorageClass. |
| `volumeName` | Existing PersistentVolume name for static binding. |

These fields are applied when the PVC is created. Updating them later does not resize, replace, or rebind the existing PVC.

## Static PersistentVolume

For simple testing, you can create a static `hostPath` PersistentVolume and reference it with `volumeName`. This is useful for local or lab environments where you can inspect the host path directly.

For production, use a storage backend that matches your cluster and availability requirements, such as an RWX-capable CSI driver when running multiple replicas or a DaemonSet.

Example `hostPath` PersistentVolume for local testing:

```yaml
apiVersion: v1
kind: PersistentVolume
metadata:
  name: acclog-local-pv
spec:
  capacity:
    storage: 20Gi
  accessModes:
    - ReadWriteMany
  persistentVolumeReclaimPolicy: Retain
  hostPath:
    path: /mnt/acclog
```

Then reference it from the `OrdsSrvs` resource:

```yaml
spec:
  accessLogPersistence:
    volumeName: acclog-local-pv
    storageClass: ""
    size: 20Gi
    accessMode: ReadWriteMany
```

## Multiple Replicas and DaemonSet

When HTTP access log persistence is enabled, the controller mounts the shared PVC with a Pod object-specific subpath. ORDS keeps writing to the same path inside each container:

```text
/opt/oracle/sa/log/global/ords_YYYY_MM_DD.log
```

On the PersistentVolume, each Pod object writes into its own directory:

```text
/mnt/acclog/<pod-name>-<pod-uid>/ords_YYYY_MM_DD.log
```

This prevents multiple ORDS pods from writing to the same HTTP access log file on a shared volume. It also gives a recreated Pod object a new directory on the PersistentVolume.

When a Pod object is deleted or replaced, its previous access-log directory remains on the PersistentVolume. The operator does not delete these stale directories automatically because it cannot know whether the logs have already been archived, audited, or are still required for retention or compliance. Cleanup of old pod-specific log directories must be managed outside the operator according to your operational retention policy.

For `spec.replicas` greater than `1`, or when `spec.workloadType` is `DaemonSet`, HTTP access log persistence requires:

```yaml
accessMode: ReadWriteMany
```

If another access mode is used, the controller reports an `InvalidSpec` warning and does not continue with the deployment.

## HTTP Access Log Forwarder

`spec.accessLogForwarder` controls the sidecar that forwards HTTP access logs to container stdout.  
Use the forwarder when a log collector is configured to collect container stdout/stderr.  

> For collectors that support file scraping with offsets or state, configure the collector to read HTTP access log files directly from the mounted access-log volume.

Example:

```yaml
spec:
  globalSettings:
    enable.standalone.access.log: true
  accessLogForwarder:
    enabled: true
    resources:
      requests:
        cpu: 10m
        memory: 32Mi
      limits:
        memory: 64Mi
```

The forwarder uses the same ORDS image and image pull policy as the main ORDS container. No external log-forwarder image is required.

The sidecar mounts the HTTP access log directory read-only and tails the latest matching HTTP access log file to stdout.

With ORDS 26.1, the standalone HTTP access log file for this configuration uses the format:

```text
/opt/oracle/sa/log/global/ords_YYYY_MM_DD.log
```

For example:

```text
/opt/oracle/sa/log/global/ords_2026_05_25.log
```

The forwarder selects the latest matching `ords_*.log` file by filename order.

The forwarder reads the selected HTTP access log file from the beginning. This can lead to duplicate HTTP access log lines after a forwarder container restart in the same Pod object, or after log rollover.

When HTTP access log persistence is enabled, each recreated Pod object uses a new `<pod-name>-<pod-uid>` directory on the PersistentVolume.

## Verify HTTP Access Logs

Check the generated PVC:

```bash
kubectl get pvc -n ordsnamespace <ordssrvs-name>-access-log-pvc -o wide
```

Check the HTTP access log mount inside a pod:

```bash
kubectl exec -n ordsnamespace deploy/<ordssrvs-name> -c ordssrvs-main -- \
  ls -la /opt/oracle/sa/log/global
```

For a static hostPath PV, check the host path:

```bash
ls -l /mnt/acclog
ls -l /mnt/acclog/<pod-name>-<pod-uid>
```

For multiple replicas, expect one directory per Pod object on the PV.

Check the HTTP access log forwarder stdout:

```bash
kubectl logs -n ordsnamespace deploy/<ordssrvs-name> -c ordssrvs-access-log-forwarder
```
