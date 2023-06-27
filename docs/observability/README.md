# Cloud Native Observability on Kubernetes for Oracle Databases

Oracle Database Operator for Kubernetes (`OraOperator`) includes, as a preview, the
Cloud Native Observability Controller for Oracle Databases, which automates the
deployment of the database observability exporter, creates Service Monitors
for Prometheus and provides a configmap containing a JSON for a sample dashboard
in Grafana. The following sections explain the setup and functionality
of the operator

- [Requirements](#requirements)
    - [The Observability Custom Resource](#observability-custom-resource)
        - [Create](#create)
        - [Delete](#delete)
        - [Patch](#patch)

## Requirements
Oracle recommends that you follow the [requirements](./REQUIREMENTS.md).

## DatabaseObserver Custom Resource
The Oracle Database Operator __v1.0.0__ includes the Observability controller, as a Preview (i.e., not a production quality feature). The Observability controller automates
the deployment and setting up of the Oracle Database exporter and related resources to make databases observable.

### Resource Details
#### Observability List
To list the Observability custom resources, use the following command as an example:
```sh
$ kubectl get databaseobserver
```

#### Quick Status
To obtain a quick status, use the following command as an example: 

> Note: The databaseobserver custom resource is named `obs-sample`. 
> We will use this name only in the following examples

```sh
$ kubectl get databaseobserver obs-sample
NAME           EXPORTERCONFIG        STATUS
obs-sample     obs-cm-obs-sample     READY
```

#### Detailed Status
To obtain a detailed database status, use the following command as an example:

```sh
$ kubectl describe databaseobserver obs-sample
Name:         obs-sample
Namespace:    ...
Labels:       ...
Annotations:  ...
API Version:  observability.oracle.com/v1alpha1
Kind:         DatabaseObserver
Metadata: ...
Spec: ...
Status:
  Conditions:
    Last Transition Time:  2023-05-08T15:23:05Z
    Message:               Observability exporter deployed successfully
    Reason:                ResourceReady
    Status:                True
    Type:                  READY
  Exporter Config:         obs-cm-obs-sample
  Status:                  READY
Events:
  Type    Reason                     Age                From           Message
  ----    ------                     ----               ----           -------
  Normal  InitializationSucceeded    16s                Observability  Initialization of observability resource completed
  Normal  CreateResourceSucceeded    16s                Observability  Succeeded creating configmap: obs-cm-obs-sample
  Normal  CreateResourceSucceeded    16s                Observability  Succeeded creating Deployment: obs-deploy-obs-sample
  Normal  CreateResourceSucceeded    16s                Observability  Succeeded creating Service: obs-svc-obs-sample
  Normal  CreateResourceSucceeded    16s                Observability  Succeeded creating ServiceMonitor: obs-servicemonitor-obs-sample
  Normal  CreateResourceSucceeded    16s                Observability  Succeeded creating ConfigMap: obs-json-dash-obs-sample
```


## Create
To provision a new Observability custom resource on the Kubernetes cluster, use the example **[config/samples/observability/observability_create.yaml](../../config/samples/observability/observability_create.yaml)**.

In the example YAML file, the Observability custom resource checks the following properties:

| Attribute                                             | Type   | Default         | Required?   | Example                                                               |
|-------------------------------------------------------|--------|-----------------|-------------|-----------------------------------------------------------------------|
| `spec.database.dbName`                                | string | -               | Yes         | _oradevdb_                                                            |
| `spec.database.dbPassword.key`                        | string | password        | Optional    | _admin-password_                                                      |
| `spec.database.dbPassword.secret`                     | string | -               | Required    | _devsec-dbpassword_                                                   |
| `spec.database.dbWallet.secret`                       | string | -               | Conditional | _devsec-oradevdb-wallet_                                              |
| `spec.database.dbConnectionString.key`                | string | access          | Optional    | _connection_                                                          |
| `spec.database.dbConnectionString.secret`             | string | -               | Required    | _devsec-dbconnectionstring_                                           |
| `spec.exporter.image`                                 | string | -               | No          | _container-registry.oracle.com/database/observability-exporter:1.0.2_ |
| `spec.exporter.configuration.configmap.key`           | string | config.toml     | No          | _config.toml_                                                         |
| `spec.exporter.configuration.configmap.configmapName` | string | -               | No          | _devcm-oradevdb-config_                                               |
| `spec.exporter.service.port`                          | number | 9161            | No          | _9161_                                                                |
| `spec.prometheus.port`                                | string | metrics         | No          | _metrics_                                                             |
| `spec.prometheus.labels`                              | map    | app: obs-{name} | No          | _app: oradevdb-apps_                                                  |
| `spec.replicas`                                       | number | 1               | No          | _1_                                                                   |

To provide the remaining references above, follow the guide provided below.

To enable the exporter to make a connection and observe your database, whether it is in the cloud or it is
a containerized database, you need to provide a working set of database credentials.


1. Create a Kubernetes secret for the Database Connection String.
    ```bash
    kubectl create secret generic db-connect-string --from-literal=access=admin/password@sampledb_tp
    ```

2. Create a Kubernetes secret for the Database Password.
    ```bash
    kubectl create secret generic db-password --from-literal=password='password'
    ```

3. (Conditional) If your database is an Autonomous Database, create a Kubernetes secret for the Autonomous Database Wallet

   If you have an Autonomous database, you can create a secret for the wallet by following this [step](../adb/README.md#download-wallets).


Once the secrets are created, to create the Observability resource, apply the YAML
```bash
   kubectl apply -f observability_create.yaml
```

### Configurations

The Observability controller provides multiple other fields for configuration but
are optional:

- `spec.exporter.image` - You can update this field to override the exporter image used
  by the controller, and is useful for using the latest exporter version ahead of what the
  Observability controller supports.
- `spec.epxorter.configuration.configmap.configmapName` and `spec.epxorter.configuration.configmap.key` - You can
  set this field to the name of a custom configmap that you want to provide the controller to use
  instead of the default configmap. This field can be updated and the deployment will replace the configmap being used.
- `spec.exporter.service.port` - You can change the port used by the exporter service created by the controller.
- `spec.prometheus.port` - You can change the port used by the service monitor created by the controller.
- `spec.prometheus.labels` - You can set the labels that is specific to your usage. This field defines the labels
  and selector labels used by the resources. This field cannot be updated after the custom resource is created.
- `spec.replicas` - You can set this number to the replica count of the exporter deployment.

## Delete

To delete the DatabaseObserver custom resource and all related resources:

```bash
kubectl delete databaseobserver obs-sample
```

## Patch

The Observability controller currently supports updates for only the following fields:

- `spec.exporter.image` - If there is a specific release of the observability exporter you require, you can update
  the _image_ field and the controller will attempt to update the exporter deployment image. The command below
  shows how to patch the field to the latest image of the exporter.
    ```bash
    kubectl --type=merge -p '{"spec":{"exporter":{"image":"container-registry.oracle.com/database/observability-exporter:latest"}}}' patch databaseobserver obs-sample
    ```

- `spec.exporter.configuration.configmap.configmapName` - The exporter configuration `.toml` file defines the metrics
  are scraped by the exporter. This field provides the ability for you to supply your own configmap containing
  the custom configurations that you require. The command below shows how to patch the field with your own configmap.
    ```bash
      kubectl --type=merge -p '{"spec":{"exporter":{"configuration":{"configmap": {"configmapName": "my-custom-configmap", "key": "config.toml"}}}' patch databaseobserver obs-sample
    ```


## Debugging and troubleshooting

### Show the details of the resource

To get the verbose output of the current spec, use the command below:

```sh
kubectl describe databaseobserver/database-observer-sample
```

If any error occurs during the reconciliation loop, the Operator either reports
the error using the resource's event stream, or will show the error under conditions.

### Check the logs of the pod where the operator deploys

Follow the steps to check the logs.

1. List the pod replicas

    ```sh
    kubectl get pods -n oracle-database-operator-system
    ```

2. Use the below command to check the logs of the deployment

    ```sh
    kubectl logs deployment.apps/oracle-database-operator-controller-manager -n oracle-database-operator-system
    ```
