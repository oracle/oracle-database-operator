# Managing Observability on Kubernetes for Oracle Databases

Oracle Database Operator for Kubernetes (`OraOperator`) includes the
Observability controller for Oracle Databases and adds the `DatabaseObserver` CRD, which enables users to observe 
Oracle Databases by scraping database metrics using SQL queries and observe logs in the Database _alert.log_. The controller 
automates the deployment and maintenance of the metrics exporter container image,
metrics exporter service and Prometheus servicemonitor.

The following sections explains the configuration and functionality
of the controller.

* [Prerequisites](#prerequisites)
* [The DatabaseObserver Custom Resource Definition](#the-databaseobserver-custom-resource)
  * [Configuration Options](#configuration-options)
  * [Resources Managed by the Controller](#resources-managed-by-the-controller)
* [DatabaseObserver Operations](#databaseobserver-operations)
  * [Create](#create-resource)
  * [List](#list-resource)
  * [Get Status](#get-detailed-status)
  * [Update](#patch-resource)
  * [Delete](#delete-resource)
* [Configuration Options for Scraping Metrics](#scraping-metrics)
  * [Custom Metrics Config](#custom-metrics-config)
  * [Prometheus Release](#prometheus-release)
* [Configuration Options for Scraping Logs](#scraping-logs)
  * [Custom Log Location with PersistentVolumes](#custom-log-location-with-persistentvolumes)
  * [Example Working with Sidecars and Promtail](#working-with-sidecars-to-deploy-promtail)
  * [Promtail Config Example](#Promtail-Config-Example)
* [Other Configuration Options](#other-configuration-options)
    * [Labels](#labels)
    * [Custom Exporter Image or Version](#custom-exporter-image-or-version)
* [Mandatory Roles and Privileges](#mandatory-roles-and-privileges-requirements-for-observability-controller)
* [Debugging and troubleshooting](#debugging-and-troubleshooting)
* [Known Issues](#known-issues)

## Prerequisites
The `DatabaseObserver` custom resource has the following prerequisites:

1. Installation of Prometheus `servicemonitor` custom resource definition (CRD) on the cluster.

- The Observability controller creates multiple Kubernetes resources that include
  a Prometheus `servicemonitor`. For the controller
  to create ServiceMonitors, the ServiceMonitor custom resource must exist. For example, to install
  Prometheus CRDs using the [Kube Prometheus Stack helm chart](https://prometheus-community.github.io/helm-charts/), run the following helm commands:
    ```bash
    helm repo add prometheus https://prometheus-community.github.io/helm-charts
    helm repo update
    helm upgrade --install prometheus prometheus/kube-prometheus-stack -n prometheus --create-namespace
    ```

2. A pre-existing Oracle Database and the proper database grants and privileges.


- The controller exports metrics through SQL queries that the user can control 
   and specify through a _toml_ file. The necessary access privileges to the tables used in the queries
   are not provided and applied automatically.

## The DatabaseObserver Custom Resource
The Oracle Database Operator (__v2.0.0__ or later) includes the Oracle Database Observability controller, which automates
the deployment and setting up of the Oracle Database exporter and the related resources to make Oracle Databases observable.

In the example YAML file found in 
[`./config/samples/observability/v4/databaseobserver.yaml`](../../config/samples/observability/v4/databaseobserver.yaml),
the databaseObserver custom resource provides the following configurable properties:

| Attribute                                              | Type   | Default                                                             | Required?   | Example                                                               |
|--------------------------------------------------------|--------|---------------------------------------------------------------------|:------------|-----------------------------------------------------------------------|
| `spec.database.dbUser.key`                             | string | user                                                                | Optional    | _username_                                                            |
| `spec.database.dbUser.secret`                          | string | -                                                                   | Yes         | _db-secret_                                                           |
| `spec.database.dbPassword.key`                         | string | password                                                            | Optional    | _admin-password_                                                      |
| `spec.database.dbPassword.secret`                      | string | -                                                                   | Conditional | _db-secret_                                                           |
| `spec.database.dbPassword.vaultOCID`                   | string | -                                                                   | Conditional | _ocid1.vault.oc1..._                                                  |
| `spec.database.dbPassword.vaultSecretName`             | string | -                                                                   | Conditional | _db-vault_                                                            |
| `spec.database.dbWallet.secret`                        | string | -                                                                   | Conditional | _devsec-oradevdb-wallet_                                              |
| `spec.database.dbConnectionString.key`                 | string | connection                                                          | Optional    | _connection_                                                          |
| `spec.database.dbConnectionString.secret`              | string | -                                                                   | Yes         | _db-secretg_                                                          |
| `spec.sidecars`                                        | array  | -                                                                   | Optional    | -                                                                     |
| `spec.sidecarVolumes`                                  | array  | -                                                                   | Optional    | -                                                                     |
| `spec.exporter.deployment.securityContext`             | object |                                                                     | Optional    | _                                                                     |
| `spec.exporter.deployment.env`                         | map    | -                                                                   | Optional    | _DB_ROLE: "SYSDBA"_                                                   |
| `spec.exporter.deployment.image`                       | string | container-registry.oracle.com/database/observability-exporter:1.5.1 | Optional    | _container-registry.oracle.com/database/observability-exporter:1.3.0_ |
| `spec.exporter.deployment.args`                        | array  | -                                                                   | Optional    | _[ "--log.level=info" ]_                                              |
| `spec.exporter.deployment.commands`                    | array  | -                                                                   | Optional    | _[ "/oracledb_exporter" ]_                                            |
| `spec.exporter.deployment.labels`                      | map    | -                                                                   | Optional    | _environment: dev_                                                    |
| `spec.exporter.deployment.podTemplate.labels`          | map    | -                                                                   | Optional    | _environment: dev_                                                    |
| `spec.exporter.deployment.podTemplate.securityContext` | object | -                                                                   | Optional    | _                                                                     |
| `spec.exporter.service.ports`                          | array  | -                                                                   | Optional    | -                                                                     |
| `spec.exporter.service.labels`                         | map    | -                                                                   | Optional    | _environment: dev_                                                    |                                                                     |
| `spec.configuration.configMap.key`                     | string | config.toml                                                         | Optional    | _config.toml_                                                         |
| `spec.configuration.configMap.name`                    | string | -                                                                   | Optional    | _devcm-oradevdb-config_                                               |
| `spec.prometheus.serviceMonitor.labels`                | map    | -                                                                   | Yes         | _release: prometheus_                                                 |
| `spec.prometheus.serviceMonitor.namespaceSelector`     | -      | -                                                                   | Yes         | -                                                                     |
| `spec.prometheus.serviceMonitor.endpoints`             | array  | -                                                                   | Optional    | -                                                                     |
| `spec.log.destination`                                 | string | alert.log                                                           | Optional    | _alert.log_                                                           |
| `spec.log.filename`                                    | string | /log                                                                | Optional    | _/log_                                                                |
| `spec.log.disable`                                     | bool   | -                                                                   | Optional    | true                                                                  |
| `spec.log.volume.persistentVolumeClaim.claimName`      | string | -                                                                   | Optional    | _my-pvc_                                                              |
| `spec.replicas`                                        | number | 1                                                                   | Optional    | _1_                                                                   |
| `spec.inheritLabels`                                   | array  | -                                                                   | Optional    | _- environment: dev_<br/>- app.kubernetes.io/name: observer           |
| `spec.ociConfig.configMapName`                         | string | -                                                                   | Conditional | _oci-cred_                                                            |
| `spec.ociConfig.secretName`                            | string | -                                                                   | Conditional | _oci-privatekey_                                                      |


### Configuration Options
The `databaseObserver` Custom resource has the following fields for all configurations that are <u>required</u>:
* `spec.database.dbUser.secret` - Secret containing the database username. The corresponding key can be any value but must match the key in the secret provided.
* `spec.database.dbPassword.secret` - Secret containing the database password (if `vault` is NOT used). The corresponding key field can be any value, but must match the key in the Secret provided
* `spec.database.dbConnectionString.secret` - Secret containing the database connection string. The corresponding key field can be any value but must match the key in the Secret provided
* `spec.prometheus.serviceMonitor.labels` - Custom labels to add to the service monitors labels. A label is required for your serviceMonitor to be discovered. This label must match what is set in the serviceMonitorSelector of your Prometheus configuration

If a database wallet is required to connect, then the following field containing the wallet secret is <u>required</u>:
* `spec.database.dbWallet.secret` - Secret containing the database wallet. The filenames inside the wallet must be used as keys

If vault is used to store the database password instead, then the following fields are <u>required</u>:
* `spec.database.dbPassword.vaultOCID` - OCID of the vault used
* `spec.database.dbPassword.vaultSecretName` - Name of the secret inside the desired vault
* `spec.ociConfig.configMapName` - Holds the rest of the information of the OCI API signing key. The following keys must be used: `fingerprint`, `region`, `tenancy` and `user`
* `spec.ociConfig.secretName` - Holds the private key of the OCI API signing key. The key to the file containing the user private key must be: `privatekey`

The `databaseObserver` Resource provides the remaining multiple fields that are <u>optional</u>:
* `spec.prometheus.serviceMonitor.endpoints` - ServiceMonitor endpoints
* `spec.prometheus.serviceMonitor.namespaceSelector` - ServiceMonitor namespace selector
* `spec.sidecars` - List of containers to run as a sidecar container with the observability exporter container image
* `spec.sidecarVolumes` - Volumes of any sidecar containers
* `spec.log.disable` - Disables Log volume creation
* `spec.log.filename` - Custom filename for the log file
* `spec.log.destination` - Custom destination for the log volume
* `spec.log.volume.persistentVolumeClaim.claimName` - A volume in which to place the log to be shared by the containers. If not specified, an EmptyDir is used by default.
* `spec.configuration.configMap.key` - Configuration filename inside the container and the configmap
* `spec.configuration.configMap.name` - Name of the `configMap` that holds the custom metrics configuration
* `spec.replicas` - Number of replicas to deploy
* `spec.exporter.service.ports` - Port number for the generated service to use
* `spec.exporter.service.labels` - Custom labels to add to service labels
* `spec.exporter.deployment.image` - Image version of observability exporter to use
* `spec.exporter.deployment.env` - Custom environment variables for the observability exporter
* `spec.exporter.deployment.labels` - Custom labels to add to deployment labels
* `spec.exporter.deployment.podTemplate.labels` - Custom labels to add to pod labels
* `spec.exporter.deployment.podTemplate.securityContext` - Configures pod securityContext
* `spec.exporter.deployment.args` - Additional arguments to provide the observability-exporter
* `spec.exporter.deployment.commands` - Commands to supply to the observability-exporter
* `spec.exporter.deployment.securityContext` - Configures container securityContext
* `spec.inheritLabels` - Keys of inherited labels from the databaseObserver resource. These labels are applied to generated resources.

### Resources Managed by the Controller
When you create a `DatabaseObserver` resource, the controller creates and manages the following resources:

1. __Deployment__ - The deployment will have the same name as the `databaseObserver` resource
    - Deploys a container named `observability-exporter`
    - The default container image version of the `container-registry.oracle.com/database/observability-exporter` supported is __[v1.5.1](https://github.com/oracle/oracle-db-appdev-monitoring/releases/tag/1.5.1)__

2. __Service__ - The service will have the same name as the databaseObserver
    - The service is of type `ClusterIP`

3. __Prometheus ServiceMonitor__ - The serviceMonitor will have the same name as the `databaseObserver`

## DatabaseObserver Operations
### Create Resource
Follow the steps below to create a new `databaseObserver` resource object.

1. To begin, creating a `databaseObserver` requires you to create and provide Kubernetes Secrets to provide connection details:
```bash
kubectl create secret generic db-secret \
    --from-literal=username='username' \
    --from-literal=password='password_here' \
    --from-literal=connection='dbsample_tp'
```

2. (Conditional) Create a Kubernetes Secret for the wallet (if a wallet is required to connect to the database). 

You can create this Secret by using a command similar to the example that follows. 
If you are connecting to an Autunomous Database, and the operator is used to manage the Oracle Autonomous Database, then a client wallet can also be downloaded as a Secret through `kubectl` commands. See the ADB README section on [Download Wallets](../../docs/adb/README.md#download-wallets).

You can also choose to create the wallet secret from a local directory containing the wallet files:
```bash
kubectl create secret generic db-wallet --from-file=wallet_dir
```

3. Finally, update the `databaseObserver` manifest with the resources you have created. You can use the example _minimal_ manifest 
inside [config/samples/observability/v4](../../config/samples/observability/v4/databaseobserver_minimal.yaml) to specify and create your databaseObserver object with a 
YAML file.

```YAML
# example
apiVersion: observability.oracle.com/v4
kind: DatabaseObserver
metadata:
  name: obs-sample
spec:
  database:
    dbUser:
      key: "username"
      secret: db-secret
 
    dbPassword:
      key: "password"
      secret: db-secret
    
    dbConnectionString:
      key: "connection"
      secret: db-secret
     
    dbWallet:
      secret: db-wallet

  prometheus:
    serviceMonitor:
      labels:
        release: prometheus
```

```bash
   kubectl apply -f databaseobserver.yaml
```

### List Resource
To list the Observability custom resources, use the following command as an example:
```bash
kubectl get databaseobserver -A
```

### Get Detailed Status
To obtain a quick status, use the following command as an example: 

> Note: The databaseobserver custom resource is named `obs-sample` in the next following sections. 
> We will use this name as an example.

```sh
$ kubectl get databaseobserver obs-sample
NAME         EXPORTERCONFIG   STATUS   VERSION
obs-sample   DEFAULT          READY    1.6.0
```


To obtain a more detailed status, use the following command as an example:

```bash
kubectl describe databaseobserver obs-sample
```

This command displays details of the current state of your `databaseObserver` resource object. A successful 
deployment of the `databaseObserver` resource object should display `READY` as the status, and all conditions should display with a `True` value for every ConditionType.


### Patch Resource
The Observability controller currently supports updates for most of the fields in the manifest. The following is an example of patching the `databaseObserver` resource:
```bash
kubectl --type=merge -p '{"spec":{"exporter":{"image":"container-registry.oracle.com/database/observability-exporter:1.6.0"}}}' patch databaseobserver obs-sample
```

### Delete Resource

To delete the `databaseObserver` custom resource and all related resources, use this command:

```bash
kubectl delete databaseobserver obs-sample
```

## Scraping Metrics
The `databaseObserve`r resource deploys the Observability exporter container. This container connects to an Oracle Database and
scrapes metrics using SQL queries. By default, the exporter provides standard metrics, which are listed in the [official GitHub page of the Observability Exporter](https://github.com/oracle/oracle-db-appdev-monitoring?tab=readme-ov-file#standard-metrics).

To define custom metrics in Oracle Database for scraping, a TOML file that lists your custom queries and properties is required.
The file will have metric sections with the following parts:
- a context
- a request, which contains the SQL query
- a map between the field(s) in the request and comment(s)

For example, the code snippet that follows shows how you can define custom metrics:
```toml
[[metric]]
context = "test"
request = "SELECT 1 as value_1, 2 as value_2 FROM DUAL"
metricsdesc = { value_1 = "Simple example returning always 1.", value_2 = "Same but returning always 2." }
```
This file produces the following entries:
```
# HELP oracledb_test_value_1 Simple example returning always 1.
# TYPE oracledb_test_value_1 gauge
oracledb_test_value_1 1
# HELP oracledb_test_value_2 Same but returning always 2.
# TYPE oracledb_test_value_2 gauge
oracledb_test_value_2 2
```

You can find more information in the [__Custom Metrics__](https://github.com/oracle/oracle-db-appdev-monitoring?tab=readme-ov-file#custom-metrics) section of the Official GitHub page.



### Custom Metrics Config
When configuring a `databaseObserver` resource, you can use the field `spec.configuration.configMap` to provide a 
custom metrics file as a `configMap`.

You can create the `configMap` by running the following command:
```bash
kubectl create cm custom-metrics-cm --from-file=metrics.toml
```

Finally, when creating or updating a `databaseObserver` resource, if we assume using the example above, you can set the fields in your YAML file as follows:
```yaml
spec:
  configuration:
    configMap:
      key: "metrics.toml"
      name: "custom-metrics-cm"
```

### Prometheus Release
To enable your Prometheus configuration to find and include the `ServiceMonitor` created by the `databaseObserver` resource, the field `spec.prometheus.serviceMonitor.labels` is an __important__ and __required__ field. The label on the ServiceMonitor
must match the `spec.serviceMonitorSelector` field in your Prometheus configuration.

```yaml
  prometheus:
    serviceMonitor:
      labels:
        release: stable
```

## Scraping Logs
Currently, the observability exporter provides the `alert.log` from Oracle Database, which provides important information about errors and exceptions during database operations. 

By default, the logs are stored in the pod filesystem, inside `/log/alert.log`. Note that the log can also be placed in a custom path with a custom filename, You can also place a volume available to multiple pods with the use of `PersistentVolumes` by specifying a `persistentVolumeClaim`. 
Because the logs are stored in a file, scraping the logs must be pushed to a log aggregation system, such as _Loki_. 
In the following example, `Promtail` is used as a sidecar container that ships the contents of local logs to the Loki instance.


To configure the `databaseObserver` resource with a sidecar, two fields can be used:
```yaml
spec:
  sidecars: []
  sidecarVolumes: []
```

You can find an example in the `samples` directory, which deploys a Promtail sidecar container as an example:
[`config/samples/observability/v4/databaseobserver_logs_promtail.yaml`](../../config/samples/observability/v4/databaseobserver_logs_promtail.yaml)

### Custom Log Location with PersistentVolumes

The fields `spec.log.filename` and `spec.log.destination` enable you to configure a custom location and filename for the log.
Using a custom location enables you to control where to place the logfile, such as a `persistentVolume`.

```yaml
  log:
    filename: "alert.log"
    destination: "/log"
```

To configure the `databaseObserver` resource to put the log file in a `persistentVolume`, you can set the following fields 
in your `databaseObserver` YAML file. The field `spec.log.volume.name` is provided to control the name of the volume used
for the log, while the field `spec.log.volume.persistentVolumeClaim.claimName` is used to specify the claim to use. 
These details can be used with any sidecar containers, or with other containers.

If `spec.log.volume.persistentVolumeClaim.claimName` is not specified, then an `EmptyDir` volume is automatically used.

> Important Note: the volume name must match all references of the volume, such as in any sidecar containers that use and mount this volume.

```yaml
  log:
    volume:
      persistentVolumeClaim:
        claimName: "my-pvc"
```

The security context defines privilege and access control settings for a pod container, If these privileges and access control settingrs need to be updated in the pod, then the same field is available on the `databaseObserver` spec. You can set this object under deployment: `spec.exporter.deployment.securityContext`.

```yaml
spec:
  exporter:
    deployment:
      runAsUser: 1000
```

Configuring security context under the PodTemplate is also possible. You can set this object under: `spec.exporter.deployment.podTemplate.securityContext`

```yaml
spec:
  exporter:
    deployment:
      podTemplate:
          securityContext:
            supplementalGroups: [1000]
```


### Working with Sidecars to deploy Promtail
The fields `spec.sidecars` and `spec.sidecarVolumes` provide the ability to deploy container images as a sidecar container
alongside the `observability-exporter` container.

You can specify container images to deploy inside `spec.sidecars` as you would normally define a container in a deployment. The field
`spec.sidecars` is of an array of containers (`[]corev1.Container`).

For example, to deploy a Grafana Promtail image, you can specify the container and its details as an element to the array, `spec.sidecars`.
```yaml
  sidecars:
    - name: promtail
      image: grafana/promtail
      args:
        - -config.file=/etc/promtail/config.yaml
      volumeMounts:
        - name: promtail-config-volume
          mountPath: /etc/promtail
        - name: my-log-volume
          mountPath: /log  
```

> Important Note: Make sure the volumeMount name matches the actual name of the volumes referenced. In this case, `my-log-volume` is referenced in `spec.log.volume.name`.

In the field `spec.sidecarVolumes`, you can specify and list the volumes you need in your sidecar containers. The field
`spec.sidecarVolumes` is an array of Volumes (`[]corev1.Volume`).

For example, when deploying the Promtail container, you can specify in the field any volume that needs to be mounted in the sidecar container above.

```yaml
  sidecarVolumes:
    - name: promtail-config-volume
      configMap:
        name: promtail-config-file
```

In this example, the `promtail-config-file` `configMap` contains the Promtail configuration, which specifies where to find
the target and the path to the file, as well as the endpoint where Loki is listening for any push API requests.

__Promtail Config Example__

```yaml
# config.yaml
server:
  http_listen_port: 9080
  grpc_listen_port: 0
positions:
  filename: /tmp/positions.yaml

clients:
  - url: http://{loki-endpoint}:3100/loki/api/v1/push

scrape_configs:
  - job_name:  "alert-log"
    static_configs:
      - targets:
          - localhost
        labels:
          app: {my-database-observer-label}
          __path__: /log/*.log
 ```

To create the `configmap`, you can run the following command:
```bash
kubectl create cm promtail-config-file --from-file=config.yaml
```


## Other Configuration Options

### Labels

__About the Default Label__ - The resources created by the Observability Controller will automatically be labelled with:
- `app`: `<database-observer-resource-name>`


For example, if the `databaseObserver` instance is named: `metrics-exporter`, then resources such as the deployment will be labelled
with `app: metrics-exporter`. This label `cannot be overwritten` as this label is used by multiple resources created. Selectors used by the deployment, service and servicemonitor use this label.

The following configuration shows an example:

```yaml
apiVersion: observability.oracle.com/v4
kind: DatabaseObserver
metadata:
  name: metrics-exporter
  labels:
    app: my-db-metrics
    some: non-inherited-label
spec:

  # inheritLabels
  inheritLabels:
    - some

  # ...
```

Meanwhile, you can provide extra labels to the resources created by the `databaseObserver` controller, such as the Deployment, Pods, Service and ServiceMonitor.
```yaml
spec:
  exporter:
    deployment:
      labels:
      podTemplate:
        labels:
    service:
      labels:
  prometheus:
    serviceMonitor:
      labels:
```

### Custom Exporter Image or Version
The field `spec.exporter.deployment.image` is provided to enable you to make use of a newer or older version of the [observability-exporter](https://github.com/oracle/oracle-db-appdev-monitoring)
container image.

```yaml
spec:
  exporter:
    deployment:
      image: "container-registry.oracle.com/database/observability-exporter:1.6.0"
```

### Custom Environment Variables, Arguments and Commands
The fields `spec.exporter.deployment.env`, `spec.exporter.deployment.args` and `spec.exporter.deployment.commands` are provided for adding custom environment variables, arguments (`args`) and commands to the containers. 
Any custom environment variable will overwrite environment variables set by the controller.

```yaml
spec:
  exporter:
    deployment:
      env:
        DB_ROLE: ""
        TNS_ADMIN: ""
      args:
        - "--log.level=info"
      commands:
        - "/oracledb_exporter"
```


### Custom Service Ports
The field `spec.exporter.service.ports` is provided to enable setting the ports of the service. If not set, then the following definition is set by default.

```yaml
spec:
  exporter:
    service:
      ports:
        - name: metrics
          port: 9161
          targetPort: 9161
      
```

### Custom ServiceMonitor Endpoints
The field `spec.prometheus.serviceMonitor.endpoints` is provided for providing custom endpoints for the ServiceMonitor resource created by the `databaseObserver`:

```yaml
spec:
  prometheus:
    serviceMonitor:
      endpoints:
        - bearerTokenSecret:
            key: ''
          interval: 20s
          port: metrics
          relabelings:
            - action: replace
              sourceLabels:
                - __meta_kubernetes_endpoints_label_app
              targetLabel: instance
```

## Mandatory roles and privileges requirements for Observability Controller

The Observability controller issues the following policy rules for the following resources. Besides
databaseobserver resources, the controller manages its own service, deployment, pods and servicemonitor 
and gets and lists configmaps and secrets.

| Resources                                             | Verbs                                     |
|-------------------------------------------------------|-------------------------------------------|
| services                                              | create delete get list patch update watch |
| deployments                                           | create delete get list patch update watch |
| pods                                                  | create delete get list patch update watch |
| events                                                | create delete get list patch update watch |
| services.apps                                         | create delete get list patch update watch |
| deployments.apps                                      | create delete get list patch update watch |
| pods.apps                                             | create delete get list patch update watch |
| servicemonitors.monitoring.coreos.com                 | create delete get list patch update watch |
| databaseobservers.observability.oracle.com            | create delete get list patch update watch |
| databaseobservers.observability.oracle.com/status     | get patch update                          |
| configmaps                                            | get list                                  |
| secrets                                               | get list                                  |
| configmaps.apps                                       | get list                                  |
| databaseobservers.observability.oracle.com/finalizers | update                                    |

## Debugging and troubleshooting

### Show the details of the resource
To obtain the verbose output of the current spec, use the following command:

```sh
kubectl describe databaseobserver/database-observer-sample
```

If any error occurs during the reconciliation loop, then the Operator either reports
the error using the resource's event stream, or it will show the error under conditions.

### Check the logs of the pod where the operator deploys
Follow these steps to check the logs.

1. List the pod replicas

    ```sh
    kubectl get pods -n oracle-database-operator-system
    ```

2. Use the following command to check the logs of the deployment

    ```sh
    kubectl logs deployment.apps/oracle-database-operator-controller-manager -n oracle-database-operator-system
    ```

## Resources
- [GitHub - Unified Observability for Oracle Database Project](https://github.com/oracle/oracle-db-appdev-monitoring)
