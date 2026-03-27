# OrdsSrvs Controller: Additional Labels, Annotations, and Resource Settings

This example shows how to customize labels, annotations, and container resource requirements for `OrdsSrvs`.

These options are useful when you want to:

* Integrate with cluster tooling that relies on labels or annotations, such as log collectors, service discovery, or custom probes
* Add ownership or environment metadata for cost management and operational tracking
* Influence dynamic worker node allocation by defining CPU and memory requests and limits that the Kubernetes scheduler can use

Before testing this example, please verify the prerequisites: [OrdsSrvs prerequisites](../README.md#prerequisites)

## Example Manifest

```yaml
apiVersion: database.oracle.com/v4
kind: OrdsSrvs
metadata:
  name: ordssrvs-base
  namespace: NAMESPACE
spec:
  image: ORDSIMG
  jdkJavaOptions: "-Xms384M -Xmx640M"

  globalSettings:
    standalone.http.port: 8080

  poolSettings:
    - poolName: default
      db.connectionType: customurl
      db.customURL: jdbc:oracle:thin:@//CONNECTSTRING
      db.username: ORDS_PUBLIC_USER
      db.secret:
        secretName: db-secret

  resources:
    requests:
      cpu: "1"
      memory: "512Mi"
    limits:
      cpu: "2"
      memory: "1Gi"

  serviceAccountName: ordssrvs-sa

  commonMetadata:
    additionalLabels:
      environment: dev
      app: reserved-will-be-ignored
    additionalAnnotations:
      contact: "platform-team"

  workload:
    metadata:
      additionalLabels:
        workload-tier: backend
        app.kubernetes.io/name: reserved-will-be-ignored
      additionalAnnotations:
        example.com/change-reason: "config-update-2026-03-23"

  podTemplate:
    metadata:
      additionalLabels:
        logging: enabled
      additionalAnnotations:
        example.com/diagnostics-port: "8080"

  service:
    metadata:
      additionalLabels:
        exposure: internal
      additionalAnnotations:
        example.com/dns-name: "app.internal.example.com"
```

## How Metadata Is Applied

The controller builds labels and annotations in a predictable order.

### Labels

1. System labels
2. `spec.commonMetadata.additionalLabels`
3. Resource-specific additional labels

Resource-specific means:

* `spec.workload.metadata.additionalLabels` for the workload
* `spec.podTemplate.metadata.additionalLabels` for pods
* `spec.service.metadata.additionalLabels` for the Service

If the same non-system label key appears more than once, the later value overrides the earlier one.

### Annotations

1. System annotations
2. `spec.commonMetadata.additionalAnnotations`
3. Resource-specific additional annotations

Resource-specific means:

* `spec.workload.metadata.additionalAnnotations`
* `spec.podTemplate.metadata.additionalAnnotations`
* `spec.service.metadata.additionalAnnotations`

If the same annotation key appears more than once, the later value overrides the earlier one.

## Reserved Label Keys

The controller reserves a small set of label keys for its own operation. If you try to set them in additional labels, the values are treated as reserved and ignored.

Reserved label keys:

* `app`
* `app.kubernetes.io/name`
* `app.kubernetes.io/instance`
* `app.kubernetes.io/managed-by`
* `app.kubernetes.io/component`
* `oracle.com/ords-operator-filter`

In the example above, these custom labels are reserved and ignored:

* `spec.commonMetadata.additionalLabels.app`
* `spec.workload.metadata.additionalLabels.app.kubernetes.io/name`

This ensures the operator keeps control of selector and system identity labels.

## Resource Requirements

The `spec.resources` section uses the standard Kubernetes `ResourceRequirements` structure.

These resource settings are applied to both:

* the ORDS init container
* the main ORDS container

Example:

```yaml
resources:
  requests:
    cpu: "1"
    memory: "512Mi"
  limits:
    cpu: "2"
    memory: "1Gi"
```

This is useful when you want to:

* Reserve enough CPU and memory for ORDS startup and runtime
* Influence placement on worker nodes through Kubernetes scheduling
* Support cost management and capacity planning in shared clusters

You can configure `spec.resources` together with `jdkJavaOptions` to reserve memory for the Java process. For example, if you increase `-Xms` or `-Xmx`, set container memory requests and limits high enough to accommodate the Java heap and remaining native or non-heap memory used by ORDS and the JVM. Make sure the requested memory can actually be allocated on cluster nodes, or the pod may remain pending.

## What This Example Produces

From the example manifest:

* All generated resources receive the common annotation:
  * `contact: platform-team`
* The workload receives:
  * label `workload-tier: backend`
  * annotation `example.com/change-reason: config-update-2026-03-23`
* The pod template receives:
  * label `logging: enabled`
  * annotation `example.com/diagnostics-port: 8080`
* The Service receives:
  * label `exposure: internal`
  * annotation `example.com/dns-name: app.internal.example.com`
* The reserved label keys are ignored:
  * `app`
  * `app.kubernetes.io/name`

## Notes

* Use `spec.commonMetadata` for metadata that should be shared across generated resources.
* Use `spec.workload.metadata`, `spec.podTemplate.metadata`, and `spec.service.metadata` when you need different metadata on each resource type.
* Labels reserved by the operator are treated as reserved and ignored even if specified by the user.
* Annotations are not filtered in the same way as labels.
* `spec.resources` uses standard Kubernetes resource syntax and applies to both init and main ORDS containers.
* When `jdkJavaOptions` increases Java memory usage, make sure the requested and limited container memory still fits available node capacity.

