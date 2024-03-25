# Managing Observability on Kubernetes for Oracle Databases

Oracle Database Operator for Kubernetes (`OraOperator`) includes the
Observability controller for Oracle Databases and adds the `DatabaseObserver` CRD, which enables users to observe 
Oracle Databases by scraping database metrics using SQL queries. The controller 
automates the deployment and maintenance of the metrics exporter container image,
metrics exporter service and a Prometheus servicemonitor.

The following sections explains the configuration and functionality
of the controller.

* [Prerequisites](#prerequisites)
* [The DatabaseObserver Custom Resource Definition](#the-databaseobserver-custom-resource)
* [Configuration of DatabaseObservers](#configuration)
  * [Create](#create-resource)
  * [List](#list-resource)
  * [Get Status](#get-detailed-status)
  * [Update](#patch-resource)
  * [Delete](#delete-resource)
* [Mandatory Roles and Privileges](#mandatory-roles-and-privileges-requirements-for-observability-controller)
* [Debugging and troubleshooting](#debugging-and-troubleshooting)

## Prerequisites
The `DatabaseObserver` custom resource has the following pre-requisites:

1. Prometheus and its `servicemonitor` custom resource definition must be installed on the cluster.

- The Observability controller creates multiple Kubernetes resources that include
  a Prometheus `servicemonitor`. In order for the controller
  to create ServiceMonitors, the ServiceMonitor custom resource must exist.

2. A pre-existing Oracle Database and the proper database grants and privileges.

- The controller exports metrics through SQL queries that the user can control 
   and specify through a _toml_ file. The necessary access privileges to the tables used in the queries
   are not provided and applied automatically.

### The DatabaseObserver Custom Resource
The Oracle Database Operator (__v1.1.0__) includes the Oracle Database Observability controller which automates
the deployment and setting up of the Oracle Database metrics exporter and the related resources to make Oracle databases observable.

In the sample YAML file found in 
[./config/samples/observability/databaseobserver.yaml](../../config/samples/observability/databaseobserver.yaml),
the databaseObserver custom resource offers the following properties to be configured:

| Attribute                                             | Type    | Default         | Required?    | Example                                                               |
|-------------------------------------------------------|---------|-----------------|--------------|-----------------------------------------------------------------------|
| `spec.database.dbUser.key`                            | string  | user            | Optional     | _username_                                                            |
| `spec.database.dbUser.secret`                         | string  | -               | Yes          | _db-secret_                                                           |
| `spec.database.dbPassword.key`                        | string  | password        | Optional     | _admin-password_                                                      |
| `spec.database.dbPassword.secret`                     | string  | -               | Conditional  | _db-secret_                                                           |
| `spec.database.dbPassword.vaultOCID`                  | string  | -               | Conditional  | _ocid1.vault.oc1..._                                                  |
| `spec.database.dbPassword.vaultSecretName`            | string  | -               | Conditional  | _db-vault_                                                            |
| `spec.database.dbWallet.secret`                       | string  | -               | Conditional  | _devsec-oradevdb-wallet_                                              |
| `spec.database.dbConnectionString.key`                | string  | connection      | Optional     | _connection_                                                          |
| `spec.database.dbConnectionString.secret`             | string  | -               | Yes          | _db-secretg_                                                          |
| `spec.exporter.image`                                 | string  | -               | Optional     | _container-registry.oracle.com/database/observability-exporter:1.0.2_ |
| `spec.exporter.configuration.configmap.key`           | string  | config.toml     | Optional     | _config.toml_                                                         |
| `spec.exporter.configuration.configmap.configmapName` | string  | -               | Optional     | _devcm-oradevdb-config_                                               |
| `spec.exporter.service.port`                          | number  | 9161            | Optional     | _9161_                                                                |
| `spec.prometheus.port`                                | string  | metrics         | Optional     | _metrics_                                                             |
| `spec.prometheus.labels`                              | map     | app: obs-{name} | Optional     | _app: oradevdb-apps_                                                  |
| `spec.replicas`                                       | number  | 1               | Optional     | _1_                                                                   |
| `spec.ociConfig.configMapName`                        | 	string | -               | 	Conditional | _oci-cred_                                                            |
| `spec.ociConfig.secretName`                           | 	string | -               | 	Conditional | _oci-privatekey_                                                      |





### Configuration
The `databaseObserver` custom resource has the following fields for all configurations that are <u>required</u>:
* `spec.database.dbUser.secret` - secret containing the database username. The corresponding key can be any value but must match the key in the secret provided.
* `spec.database.dbPassword.secret` - secret containing the database password (if vault is NOT used). The corresponding key field can be any value but must match the key in the secret provided
* `spec.database.dbConnectionString.secret` - secret containing the database connection string. The corresponding key field can be any value but must match the key in the secret provided i

If a database wallet is required to connect, the following field containing the secret is <u>required</u>:
* `spec.database.dbWallet.secret` - secret containing the database wallet. The filenames must be used as the keys

If vault is used to store the database password instead, the following fields are <u>required</u>:
* `spec.database.dbPassword.vaultOCID` - OCID of the vault used
* `spec.database.dbPassword.vaultSecretName` - Name of the secret inside the desired vault
* `spec.ociConfig.configMapName` - holds the rest of the information of the OCI API signing key. The following keys must be used: `fingerprint`, `region`, `tenancy` and `user`
* `spec.ociConfig.secretName` - holds the private key of the OCI API signing key. The key to the file containing the user private key must be: `privatekey`

The `databaseObserver` provides the remaining multiple fields that are <u>optional</u>:
* `spec.prometheus.labels` - labels to use for Service, ServiceMonitor and Deployment
* `spec.prometheus.port` - port to use for ServiceMonitor
* `spec.replicas` - number of replicas to deploy
* `spec.exporter.service.port` - port of service
* `spec.exporter.image` - image version of observability exporter to use


### Create Resource
Follow the steps below to create a new databaseObserver resource object.

1. To begin, creating a databaseObserver requires you to create and provide kubernetes Secrets to provide connection details:
```bash
kubectl create secret generic db-secret \
    --from-literal=username='username' \
    --from-literal=password='password_here' \
    --from-literal=connection='dbsample_tp'
```

2. (Conditional) Create a Kubernetes secret for the wallet (if a wallet is required to connect to the database). 

You can create this secret by using a command similar to the following example below. 
If you are connecting to an Autunomous Database and the operator is used to manage the Oracle Autonomous Database, 
a client wallet can also be downloaded as a secret through kubectl commands. You can find out how, [here](../../docs/adb/README.md#download-wallets).

Otherwise, you can create the wallet secret from a local directory containing the wallet files.
```bash
kubectl create secret generic db-wallet --from-file=wallet_dir
```

3. Finally, update the databaseObserver manifest with the resources you have created. You can use the example manifest 
inside config/samples/observability to specify and create your databaseObserver object with a 
YAML file.

```YAML
# example
apiVersion: observability.oracle.com/v1alpha1
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
NAME           EXPORTERCONFIG        STATUS
obs-sample     default               READY
```


To obtain a more detailed status, use the following command as an example:

```bash
kubectl describe databaseobserver obs-sample
```

This provides details of the current state of your databaseObserver resource object. A successful 
deployment of the databaseObserver resource object should display `READY` as the status and all conditions with a `True`
value for every ConditionType.


### Patch Resource
The Observability controller currently supports updates for most of the fields in the manifest. An example of patching the databaseObserver resource is as follows:
```bash
kubectl --type=merge -p '{"spec":{"exporter":{"image":"container-registry.oracle.com/database/observability-exporter:latest"}}}' patch databaseobserver obs-sample
```

The fields listed below can be updated with the given example command:

* spec.exporter.image
* spec.exporter.configuration.configmap.configmapName 
* spec.exporter.configuration.configmap.key
* spec.database.dbUser.secret
* spec.database.dbPassword.secret
* spec.database.dbConnectionString.secret
* spec.database.dbWallet.secret
* spec.ociConfig.configMapName
* spec.ociConfig.secretName
* spec.replicas
* spec.database.dbPassword.vaultOCID
* spec.database.dbPassword.vaultSecretName


### Delete Resource

To delete the DatabaseObserver custom resource and all related resources:

```bash
kubectl delete databaseobserver obs-sample
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
