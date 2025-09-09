# Managing Observability on Kubernetes for Oracle Databases

Oracle Database Operator for Kubernetes (`OraOperator`) includes the
Observability controller for Oracle Databases and adds the `DatabaseObserver` CRD, which enables users to observe 
Oracle Databases by scraping database metrics using SQL queries and observe logs in the Database _alert.log_. 
The controller automates the deployment and maintenance of the [database observability exporter](https://github.com/oracle/oracle-db-appdev-monitoring),
metrics exporter service and Prometheus ServiceMonitor.

The following sections explains the configuration and functionality
of the controller.

* [Prerequisites](#prerequisites)
* [The DatabaseObserver Custom Resource Definition](#the-databaseobserver-custom-resource)
  * [Configuring Database Credentials](#configuration-fields-related-to-managing-database-credentials)
  * [Configuring Cloud Provider Vaults for Database credentials](#configuration-fields-related-to-vault-usage)
  * [Configuring the Managed Resources](#configuration-fields-related-to-the-deployment-pod-service-and-servicemonitor-resources)
  * [Configuring Export of Database Metrics](#configuration-fields-related-to-metrics-export)
  * [Configuring Export of Database Logs](#configuration-fields-related-to-logs-export)
  * [Configuring the Exporter Config File](#configuration-fields-related-to-the-exporter-config-file)

* [DatabaseObserver Operations](#databaseobserver-operations)
  * [Create](#create-resource)
  * [List](#list-resource)
  * [Get Status](#get-detailed-status)
  * [Update](#patch-resource)
  * [Delete](#delete-resource)

* [Connecting to the Database](#connecting-to-the-database)
  * [Default DB Configuration](#default-database-configuration)
  * [Multiple DB Configuration](#multiple-database-configuration)

* [Vault Configuration](#database-authentication-with-vaults-in-the-cloud)
    * [Using OCI Vault](#oci-vault-configuration)
    * [Using Azure Vault](#azure-vault-configuration)

* [Setting the Exporter Config File](#defining-an-exporter-config-file)

* [Scraping Metrics](#scraping-metrics)
  * [Custom Metrics Config](#custom-metrics-config)
  * [Prometheus Release](#prometheus-release)
  
* [Scraping Logs](#scraping-logs)
  * [Custom Log Location with PersistentVolumes](#custom-log-location-with-persistentvolumes)
  * [Example Working with Sidecars and Promtail](#working-with-sidecars-to-deploy-promtail)

  
* [Customizing Resources](#customizing-resources-and-available-configuration-options)
    * [Environment Variables and Default Values](#environment-variables-and-default-values)
    * [Managing Labels](#managing-labels)
    * [Custom Exporter Image or Version](#custom-environment-variables-arguments-and-commands)
    * [Security Contexts](#security-contexts)
    * [Custom Service Ports](#custom-service-ports)
    * [Custom ServiceMonitor](#custom-servicemonitor-endpoints)
  
* [Mandatory Roles and Privileges](#mandatory-roles-and-privileges-requirements-for-observability-controller)

* [Debugging and troubleshooting](#debugging-and-troubleshooting)
* [Known Issues](#known-issues)
* [Resources](#resources)

## Prerequisites
The `DatabaseObserver` custom resource has the following prerequisites:

1. Installation of Prometheus `servicemonitor` custom resource definition (CRD) on the cluster.

   - The Observability controller creates multiple Kubernetes resources that include
     a Prometheus `servicemonitor`. For the controller
     to create ServiceMonitors, the ServiceMonitor custom resource must exist. For __example__, to install
     Prometheus CRDs using the [Kube Prometheus Stack helm chart](https://prometheus-community.github.io/helm-charts/), run the following helm commands:
       ```bash
       helm repo add prometheus https://prometheus-community.github.io/helm-charts
       helm repo update
       helm upgrade --install prometheus prometheus/kube-prometheus-stack -n prometheus --create-namespace
       ```
     - You can check if the ServiceMonitor API exists in your cluster by running the following command:
     ```bash
     kubectl api-resources | grep smon
     ```

2. A preexisting Oracle Database, and the proper database grants and privileges.

    - The controller exports metrics through SQL queries that the user can control 
       and specify through the _toml_ files. The necessary access privileges to the tables used in the queries
       are not provided and applied automatically.

## The DatabaseObserver Custom Resource
Oracle Database Operator (__v1.0.0__ or later) includes the Oracle Database Observability controller, which automates
the deployment and configuration of the Oracle Database exporter and the related resources to make Oracle Databases observable.
The Observability Controller introduces the `databaseobserver` APIs.

To list the available APIs included in the
Database Operator, you can run the following command:

```bash
kubectl api-resources | grep oracle
```

Learn about the different and configurable fields available in this release of the DatabaseObserver APIs in the following sections.

> In this release, the controller deploys the Database Observability Exporter ([v2.0.2](https://github.com/oracle/oracle-db-appdev-monitoring/releases/tag/2.0.2)).



### Configuration Fields Related to Managing Database Credentials
The following fields are available for configuring the exporter to successfully connect to databases:

| Attribute                                         | Type   | Required?   | Example              |
|---------------------------------------------------|--------|:------------|----------------------|
| `spec.database.dbUser.key`                        | string | No          | _username_           |
| `spec.database.dbPassword.key`                    | string | No          | _password_           |
| `spec.database.dbConnectionString.key`            | string | No          | _connection_         |
| `spec.database.dbUser.secret`                     | string | Yes         | _db-secret_          |
| `spec.database.dbPassword.secret`                 | string | Yes         | _db-secret_          |
| `spec.database.dbConnectionString.secret`         | string | Yes         | _db-secret_          |
| `spec.database.dbUser.envName`                    | string | No          | _DB_USERNAME_        |
| `spec.database.dbPassword.envName`                | string | No          | _DB_PASSWORD_        |
| `spec.database.dbConnectionString.envName`        | string | No          | _DB_CONN_STRING_     |
| `spec.databases.<key>.dbUser.key`                 | string | No          | _username_           |
| `spec.databases.<key>.dbPassword.key`             | string | No          | _password_           |
| `spec.databases.<key>.dbConnectionString.key`     | string | No          | _connection_         |
| `spec.databases.<key>.dbUser.secret`              | string | Yes         | _db02-secret_        |
| `spec.databases.<key>.dbPassword.secret`          | string | Yes         | _db02-secret_        |
| `spec.databases.<key>.dbConnectionString.secret`  | string | Yes         | _db02-secret_        |
| `spec.databases.<key>.dbUser.envName`             | string | No          | _DB2_USERNAME_       |
| `spec.databases.<key>.dbPassword.envName`         | string | No          | _DB2_PASSWORD_       |
| `spec.databases.<key>.dbConnectionString.envName` | string | No          | _DB2_CONN_STRING_    |
| `spec.wallet.secret`                              | string | Conditional | _combined-wallet_    |
| `spec.wallet.mountPath`                           | string | No          | _"/wallet/combined"_ |
| `spec.wallet.additional[].name`                   | string | Conditional | _db02_               |
| `spec.wallet.additional[].secret`                 | string | Conditional | _db02-wallet_        |
| `spec.wallet.additional[].mountPath`              | string | Conditional | _"/wallet/db02"_     |

These fields enable you to define connection details for a single database, or for multiple databases. For default values, environment variables and default behavior, see [defaults](#environment-variables-and-default-values).

1. `spec.database` - Use to configure the database username, password and connection string. 
The environment variables set by the controller can be customized through the `envName` field.
Both `envName` and `key` fields are optional.


2. `spec.databases` - Use to configure multiple database credentials. The keys are used as a prefix for environment
variables. For example, a key of `MYDB` will produce the environment variables `MYDB_USERNAME` and `MYDB_PASSWORD`.


3. `spec.wallet` - Use to configure the wallet with which you want to connect to the Oracle Database, if applicable. The field `mountPath`
enables you to control where the wallet is to be mounted. Meanwhile, additional wallets can be mounted in the array `additional`, used for multi
database configuration.

To learn more about configuring the database connection, see [Connecting to the Database](#connecting-to-the-database).

### Configuration Fields Related to Vault Usage
The following fields are available for configuring the exporter to use the vault when connecting to databases.

| Attribute                                 | Type   | Required?   | Example                                 |
|-------------------------------------------|--------|:------------|-----------------------------------------|
| `spec.database.oci.vaultID`               | string | Conditional | _ocid1.vault.oc1.<region>.<vault-ocid>_ |
| `spec.database.oci.vaultPasswordSecret`   | string | Conditional | _sample_secret_                         |
| `spec.database.azure.vaultID`             | string | Conditional | -                                       |
| `spec.database.azure.vaultUsernameSecret` | string | Conditional | _sample_usn_secret                      |
| `spec.database.azure.vaultPasswordSecret` | string | Conditional | _sample_pwd_secret_                     |
| `spec.ociConfig.configMap.key`            | string | No          | _config_                                |
| `spec.ociConfig.configMap.name`           | string | Conditional | _oci-config-file_                       |
| `spec.ociConfig.privateKey.key`           | string | No          | _private.pem_                           |
| `spec.ociConfig.privateKey.secret`        | string | Conditional | _oci-privatekey_                        |
| `spec.ociConfig.mountPath`                | string | No          | _"/.oci"_                               |
| `spec.azureConfig.configMap.name`         | string | Conditional | _azure-configmap_                       |


These fields enable you to define vault details for OCI or Azure. For default values, environment variables and default behavior, see [defaults](#environment-variables-and-default-values).
1. About `spec.database.oci` - for configuring the OCI Vault details for the default database.

> Note: For multiple database configuration with Vault integration, use the exporter config file. See instructions on the [Exporter Config File](#defining-an-exporter-config-file).

2. `spec.database.azure.` - Use to configure the Azure Vault details for the default database.


3. `spec.ociConfig` - Use to configure the authentication of requests made to the Oracle Cloud performed by the exporter. The configMap should contain
the actual _config_ file use by the OCI CLI (usually found in ~/.oci/config). See the Oracle Cloud Infrastructure documentation on [configuring the OCI CLI (external link)](https://docs.oracle.com/en-us/iaas/Content/API/SDKDocs/cliconfigure.htm#Configuring_the_CLI).
> Note: The OCI profile used is [DEFAULT]. Under the profile DEFAULT, set `key_file=/.oci/<your-file-name>.pem`.

4. About `spec.azureConfig` - for configuring the authentication of requests made to the Azure Cloud done by the exporter. 
The configMap should contain the keys `tenantId`, `clientId` and `clientSecret`, which are used to create the related environment variables.

To learn more about configuring an integration with a vault for database authentication, see [Authenticating with Vaults](#database-authentication-with-vaults-in-the-cloud).

### Configuration Fields Related to the Deployment, Pod, Service and ServiceMonitor Resources
When you create a `DatabaseObserver` resource, the controller creates and manages the following resources:
1. Deployment
2. Service 
3. Prometheus ServiceMonitor
The following fields below are available for configuring the controller-managed resources for a custom deployment, pod, service and servicemonitor.

| Attribute                               | Type   | Required? | Example                                                               |
|-----------------------------------------|--------|:----------|-----------------------------------------------------------------------|
| `spec.deployment.securityContext`       | object | No  | -                                                                     |
| `spec.deployment.podSecurityContext`    | object | No  | -                                                                     |
| `spec.deployment.env`                   | map    | No  | _DB_ROLE: "SYSDBA"_                                                   |
| `spec.deployment.image`                 | string | No  | _container-registry.oracle.com/database/observability-exporter:1.3.0_ |
| `spec.deployment.args`                  | array  | No  | _[ "--log.level=info" ]_                                              |
| `spec.deployment.commands`              | array  | No  | _[ "/oracledb_exporter" ]_                                            |
| `spec.deployment.labels`                | map    | No  | _environment: dev_                                                    |
| `spec.deployment.podTemplate.labels`    | map    | No  | _environment: dev_                                                    |
| `spec.service.ports`                    | array  | No  | -                                                                     |
| `spec.service.labels`                   | map    | No  | _environment: dev_                                                    |                                                                   
| `spec.serviceMonitor.labels`            | map    | Yes       | _release: prometheus_                                                 |
| `spec.serviceMonitor.endpoints`         | array  | No  | -                                                                     |
| `spec.serviceMonitor.namespaceSelector` | -      | No  | -                                                                     |
| `spec.replicas`                         | number | No  | _1_                                                                   |
| `spec.inheritLabels`                    | array  | No  | _- environment: dev_<br/>- app.kubernetes.io/name: observer           |

These fields enable you to define deployment, service and serviceMonitor details. For default values, environment variables and default behavior, see [defaults](#environment-variables-and-default-values).

1. `spec.deployment` - Use to configure deployment details such as custom environment variables through `env` and arguments through `args`.
The container image can also be replaced with a later or older version of the exporter using the `image` field. For security related configurations,
the `securityContext` and `podSecurityContext` are available.

2. `spec.service` - Use to configure a customized service resource.


3. `spec.serviceMonitor` - Use to configure the serviceMonitor resource.

> Note: It is an essential requirement that you set the Prometheus label inside `serviceMonitor.labels`. Ensure that the label set is included in the serviceMonitorSelector field of your Prometheus CR. 


5. `spec.replicas` - Use to configure the number of replicas to deploy


6. `spec.inheritLabels` - Use to configure all resources created and managed so that they inherit the labels from databaseobserver CR.


To learn more about configuring managed resources, such as the Deployment, Service and other services, see [Customizing Resources and Available Configuration Options](#customizing-resources-and-available-configuration-options)

### Configuration Fields Related to Metrics Export
The following fields are available for configuring the metrics exported from the database.

| Attribute                       | Type   | Required?   | Example                 |
|---------------------------------|--------|:------------|-------------------------|
| `spec.metrics.configMap[].key`  | string | No          | _config.toml_           ||                                              |        |             |                                                                       |
| `spec.metrics.configMap[].name` | string | Conditional | _custom-metrics-config_ |

These fields enable you to define configMap sources for metrics. For default values, environment variables and default behavior, see [defaults](#environment-variables-and-default-values).
1. `metrics.configMap[]` - Use to configure an array of configs that will contain the TOML files. This field creates a volume with multiple source files mounted in the same directory.

To learn more about configuring the metrics export, see [Defining an Exporter Config File](#defining-an-exporter-config-file).

### Configuration Fields Related to Logs Export
The following fields are available for configuring how the `alert.log` is exported from the database.

| Attribute                                        | Type   | Required?   | Example      |
|--------------------------------------------------|--------|:------------|--------------|
| `spc.log.destination`                            | string | No          | _alert.log_  |
| `spc.log.filename`                               | string | No          | _/log_       |
| `spc.log.disable`                                | bool   | No          | true         |
| `spc.log.volume.name`                            | string | No          | _log-volume_ |                                                                |
| `spc.log.volume.persistentVolumeClaim.claimName` | string | Conditional | _my-pvc_     |
| `spc.sidecar.containers[]`                       | array  | Conditional | -            |
| `spc.sidecar.volumes[]`                          | array  | Conditional | -            |

These fields enable you to define log details and sidecar resources. For default values, environment variables and default behavior, see [defaults](#environment-variables-and-default-values).
1. `spec.sidecar.containers` - Use to configure an array of containers as a sidecar to the observability 
exporter container, such as promtail. The field `sidecar.containers` enables you to list containers as you would normally for deployments.

2. `spec.sidecar.volumes` - Use to configure extra volumes related to your sidecar containers.


3. `spec.log.disable` - Use to disable the log volume creation


4. `spec.log.filename` - Use to specify a custom filename for the log file


5. `spec.log.destination` - Use to configure a custom destination for the log volume.


6. `spec.log.volume` - Use to configure the log volume into which the exporter extracts the logs, and from which the logs are read. 
If a persistentVolumeClaim is not provided, then an emptyDir is created instead. Meanwhile, the field `log.volume.name` can be used to define the name of the volume
to reference.

To learn more about configuring the log export, see [Scraping Logs](#scraping-logs).

### Configuration Fields Related to the Exporter Config File
The following fields are available for configuring the exporter through a config-file:

| Attribute                           | Type   | Required?   | Example           |
|-------------------------------------|--------|:------------|-------------------|
| `spec.exporterConfig.configMapName` | string | Conditional | _exporter-config_ |
| `spec.exporterConfig.mountPath`     | string | No          | _/config_         |

These fields enable you to define the exporter config-file. For default values, environment variables and default behavior, see [defaults](#environment-variables-and-default-values).

1. `spec.exporterConfig` - Use to configure the exporter with a config file containing database, log and metrics details. You can use
the `mountPath` field to define a custom location for the config file.

> Note: The CONFIG_FILE environment variable or the --config.file args must be set to the desired location of the config file.

To learn more about configuring the config-file for the exporter, see [Defining an Exporter Config File](#defining-an-exporter-config-file).

## DatabaseObserver Operations
### Create Resource
Use the following steps to create a new `databaseObserver` resource object.

1. When you create a `databaseObserver`, you are required to create and to provide Kubernetes Secrets containing your database connection details. Replace the values and
create a single secret by running the following command:
```bash
kubectl create secret generic db-secret \
    --from-literal=username='username' \
    --from-literal=password='password_here' \
    --from-literal=connection='dbsample_tp'
```

2. (Conditional) Create a Kubernetes Secret for the wallet (if a wallet is required to connect to the database).

If you are connecting to an Autonomous Database, and the operator is used to manage the Oracle Autonomous Database, 
then a client wallet can also be downloaded as a Secret through `kubectl` commands. 
See the ADB README section on [Download Wallets](../../docs/adb/README.md#download-wallets).

You can also choose to create the wallet secret from a local directory containing the wallet files:
```bash
kubectl create secret generic db-wallet --from-file=<wallet_dir>
```

3. Update the `databaseObserver` manifest with the resources that you have created. You can use the example _minimal_ manifest 
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
     
  wallet:
    secret: db-wallet

  serviceMonitor:
    labels:
      release: prometheus
```

To create the resource:
```bash
   kubectl apply -f databaseobserver.yaml
```

### List Resource
To list the Observability custom resources, use the following command as an example:
```bash
kubectl get dbobserver -A
```

### Get Detailed Status
To obtain a quick status, use the following command as an example:

> Note: The databaseobserver custom resource is named `obs-sample` in the next following sections. 
> We will use this name as an example.

```sh
$ kubectl get databaseobserver obs-sample
NAME         METRICSCONFIG   STATUS   VERSION
obs-sample   DEFAULT          READY    2.0.2
```


To obtain a more detailed status, use the following command as an example:

```bash
kubectl describe databaseobserver obs-sample
```

This command displays details of the current state of your `databaseObserver` resource object. A successful 
deployment of the `databaseObserver` resource object should display `READY` as the status, and all conditions should 
display with a `True` value for every ConditionType.


### Patch Resource
The Observability controller currently supports updates for most of the fields in the manifest. The following is an example 
of patching the `databaseObserver` resource:

```bash
kubectl --type=merge -p '{"spec":{"exporter":{"image":"container-registry.oracle.com/database/observability-exporter:2.0.1"}}}' patch databaseobserver obs-sample
```

### Delete Resource

To delete the `databaseObserver` custom resource and all related resources, use this command:

```bash
kubectl delete databaseobserver obs-sample
```

## Connecting to the Database


### Default Database Configuration
To configure the observability exporter to export from a single Oracle Database, use the field `spec.database`
to define the details of the database. If the wallet is applicable, `spec.wallet` allows you to define a secret containing the wallet
and where the wallet is to be mounted as a volume. 

```yaml
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
    
  wallet:
    secret: db-wallet
    mountPath: /oracle/wallet
  #...
```
Alternatively, the default database can be defined and configured through the exporter config-file. In this case, the config-file must be defined, and the
credentials are set only through the exporter YAML file as kubernetes secrets. As an example, using the same secret with the same keys, you can define the relevant secrets
in the following YAML file:

```yaml

spec:
  database:
    dbUser:
      secret: db-secret
    dbPassword:
      secret: db-secret
      
  wallet:
    secret: db-wallet
    mountPath: /oracle/wallet
    
  deployment:
    env:
      CONFIG_FILE: "/oracle/exporter/config.yaml"
  
  exporterConfig:
    configMap:
      name: config-file
```

In the config-file, we reference the environment variables DB_USERNAME and DB_PASSWORD which are set by default by the observability controller
when `spec.database.dbUser` and `spec.database.dbPassword` are defined. The TNS_ADMIN is also set through the config-file, and the
connection string provided through the config file. 

```yaml
# config.yaml

# Environment variables of the form ${VAR_NAME} will be expanded.
databases:
  default:
    username: ${DB_USERNAME}
    password: ${DB_PASSWORD}
    url: oracledb_tp
    
    ## Path to Oracle Database wallet, if using wallet
    tnsAdmin: /oracle/wallet
```
To create the configmap:
```bash
kubectl create cm config-file --from-file=config.yaml
```

### Multiple Database Configuration
To configure the observability exporter to export metrics and logs from multiple Oracle Databases, __instead__ of `spec.database`, you must use an exporter config file, 
configure the _databaseobserver_ YAML file with a combined wallet (if applicable), and then use the
`spec.databases` field . The field `spec.databases` is a map with keys used for naming environment variables
and identifying groups of credentials.

For example, to define two databases, in the _databaseobserver_ YAML file, you can have the following configuration:

```yaml
spec:
  databases:
    db01:
      dbUser:
        secret: adb01-secret
      dbPassword:
        secret: adb01-secret
    db02:
      dbUser:
        secret: adb02-secret
        envName: "DB2_USN"
      dbPassword:
        secret: adb02-secret
        envName: "DB2_PWD"

  wallet:
    secret: combined
    mountPath: "/example_dbwallet/combined"
    additional:
      - name: db01
        secret: db01-wallet
        mountPath: "/example_dbwallet/db01"
      - name: db02
        secret: db02-wallet
        mountPath: "/example_dbwallet/db02"

  deployment:
    env:
      TNS_ADMIN: /example_dbwallet/combined
      CONFIG_FILE: "/config/config.yaml"

  exporterConfig:
    configMap:
      name: config-file
```
Each database is configured under `spec.databases`, and multiple wallets are defined in the shared directory `/dbwallet` 
as an example. To configure a combined wallet for multiple databases, see [configuring a combined wallet](#configuring-wallets-for-multiple-databases).

In the configuration file _config-file_, db1 and db2 are configured with the credentials provided as environment variables through the
__databaseobserver__ YAML file as secrets.

```yaml
# config.yaml
# Environment variables of the form ${VAR_NAME} will be expanded.
databases:
  db1:
    username: ${db01_USERNAME}
    password: ${db01_PASSWORD}
    url: db1_tp
  
  db2:
    username: ${DB2_USN}
    password: ${DB2_PWD}
    url: db2_tp
```

To create the configMap, run the following command:

```bash
kubectl create cm config-file --from-file=config.yaml
```

To learn more about the config file, you can consult the [official documentations of the exporter](https://github.com/oracle/oracle-db-appdev-monitoring?tab=readme-ov-file#standalone-binary).

#### Configuring Wallets for Multiple Databases
In configuring multiple databases where each connection requires a database wallet, a combined wallet is required and can be configured through the databaseobserver YAML file. To
create a combined wallet:

1. Copy TNS aliases from every tnsnames.ora and combined them into one tnsnames.ora file.


2. Set the wallet directory for the aliases inside security, with the following snippet pointing to each database wallet location: `(MY_WALLET_DIRECTORY=/example_dbwallet/db01)`, for example:

```
...)(security=(MY_WALLET_DIRECTORY=)(ssl_server_dn_match=...)))
```


3. Place the combined `tnsnames.ora` file and one of the `sqlnet.ora` files inside a combined directory.


4. Take wallet files (`.sso`, `.p12`, `.pem`) and place them in separate directories.


The resulting wallet directory structure should look similar to the following, where wallet files for each database are in separate directories:
```bash
#
example_dbwallet
├── combined
│   ├── sqlnet.ora
│   └── tnsnames.ora # Combined tnsnames.ora
├── db01
│   ├── cwallet.sso
│   ├── ewallet.p12
│   └── ewallet.pem
└── db02
    ├── cwallet.sso
    ├── ewallet.p12
    └── ewallet.pem
```
Return to the YAML file for the next example.

5. Set the combined _tnsnames.ora_ under `spec.wallet.secret` and set the 
specific database wallet files (.sso, .p12, .pem) under `.spec.wallet.additional`.

6. Finally, set the TNS_ADMIN to the location of the `tnsnames.ora`.

> Note: When setting the name under `spec.wallets.additional[].name`, you must provide a unique name other than `creds`,  because this is the default volume name.

To learn more about this requirement, you can consult the [official documentation of the exporter](https://github.com/oracle/oracle-db-appdev-monitoring?tab=readme-ov-file#configuring-connections-for-multiple-databases-using-oracle-database-wallets).


## Database Authentication with Vaults in the Cloud
You can use Cloud Vault resources to store sensitive database credentials. In this release, the following vaults are supported:
- OCI Vault
- Azure Vault

### OCI Vault Configuration
The OCI Vault can be used to store the database credentials. This release supports storing the Oracle Database password in the OCI Vault.

When you configure the Vault, you must provide the following:
- Vault details:
  - The string OCID of the Vault used
  - The string name of the OCI Vault Secret containing the password
- Authentication details (if applicable):
  - Kubernetes Secret containing the [OCI CLI Config file](https://docs.oracle.com/en-us/iaas/Content/API/SDKDocs/cliconfigure.htm)
  - Kubernetes Secret containing the user's OCI CLI Private Key

The observability exporter needs to authenticate requests to retrieve the database password from the OCI Vault. When configuring API Key authentication, 
the OCI CLI config file and the __DEFAULT profile is used__. 
> Note: The exporter uses the DEFAULT profile.

For example, the OCI CLI config file can appear as follows:
```bash
[DEFAULT]
user=<ocid1.user.oc1..>
fingerprint=<your-fingerprint>
key_file=/.oci/private.pem
tenancy=<ocid1.tenancy.oc1..>
region=<us-ashburn-1>
```

> Note: Ensure that the key_file is in `/.oci` and that the private key file matches.

You can create the configmap with the following command:
```bash
kubectl create cm oci-cred --from-file=config
```

You can create the secret with the following command:
```bash
kubectl create secret generic oci-privatekey --from-file=private.pem
```

Finally, to configure the exporter to use an OCI Vault for retrieving the password, you can configure the following fields in the YAML file:
```yaml
spec:
    database:
      oci:
        vaultID: ocid1.vault.oc1.<region>.<vault-ocid>
        vaultPasswordSecret: sample_secret

    # ...
    
    ociConfig:
      configMap:
        name: oci-cred
      privateKey:
        name: oci-privatekey
```

### Azure Vault Configuration
The Azure Vault can be used to store the database credentials. This release supports storing the database username and password in the Azure Vault.

To configure the exporter to use Azure Vault for retrieving database credentials, you can configure the following fields:
```yaml
spec:
    database:
      azure:
        vaultID: "..."
        vaultUsernameSecret: sample_usn_secret
        vaultPasswordSecret: sample_pwd_secret

    # ...
    
    azureConfig:
      configMap:
        name: azure-cred
```

The `spec.azureConfig` field allows you to provide the environment variables through a configMap:
- AZURE_TENANT_ID
- AZURE_CLIENT_ID
- AZURE_CLIENT_SECRET

You can then create the following configmap referenced in the YAML file above with your desired values:
```bash
kubectl create configmap azure-cred \
--from-literal=tenantId=<AZURE_TENANT_ID> \
--from-literal=clientId=<AZURE_CLIENT_ID> \
--from-literal=clientSecret=<AZURE_CLIENT_SECRET>
```

## Defining an Exporter Config File
A YAML configuration file for the exporter can be provided by setting the `--config.file=`
command-line argument. It is recommended to use the configuration file from the 2.0.0 release of the exporter
and onwards.

To configure the exporter config file, set the path to the YAML file 
under `spec.deployment.args` from which to read the config from. The configMap
containing the exporter settings and configurations is set through `spec.exporterConfig`.
Specifying `spec.exporterConfig.mountPath` allows you to control the location where
the volume will be mounted.
```yaml
spec:
  # ...
  
  deployment:
    args: "--config.file=/config/exporter-config.yaml"

  exporterConfig:
    mountPath: "/config"
    configMap:
      key: exporter-config.yaml
      name: exporter-config-file
```

Create the _exporter-config-file_ configMap. For example, using the following example configuration.
```yaml
# exporter-config.yaml

# Example Oracle Database Metrics Exporter Configuration file.
# Environment variables of the form ${VAR_NAME} will be expanded.
databases:
  default:
    ## Database username
    username: ${DB_USERNAME}
    ## Database password
    password: ${DB_PASSWORD}
    ## Database connection url
    url: localhost:1521/freepdb1

    ## Metrics query timeout for this database, in seconds
    queryTimeout: 5
    
    ### Connection pooling settings for the go-sql connection pool
    ## Max open connections for this database using go-sql connection pool
    maxOpenConns: 10
    ## Max idle connections for this database using go-sql connection pool
    maxIdleConns: 10
```
For more information on the configuration fields, see the following [examples](https://github.com/oracle/oracle-db-appdev-monitoring/blob/main/example-config.yaml). You can then create the configMap that is referenced in the _databaseobserver_ YAML file with the following command:
```bash
kubectl create cm exporter-config-file --from-file=exporter-config.yaml
```

Note that in the above configuration file, the environment variables `DB_USERNAME` and `DB_PASSWORD` will be expanded
by the exporter. These environment variables
are one of the default environment variables set by the DatabaseObserver controller. In the DatabaseObserver YAML file, you
can set the following details:

```yaml
spec:
  database:
    dbUser:
      secret: db-secret
    dbPassword:
      secret: db-secret
    
  # ...
```

## Scraping Metrics
The `databaseObserver` resource deploys the Observability exporter container. This container connects to an Oracle Database and
scrapes metrics using SQL queries. By default, the exporter provides standard metrics, which are listed in the [official GitHub page of the Observability Exporter](https://github.com/oracle/oracle-db-appdev-monitoring?tab=readme-ov-file#standard-metrics).

To define custom metrics in Oracle Database for scraping, a TOML file that lists your custom queries and properties is required.
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
When configuring a `databaseObserver` resource, you can use the field `spec.metrics.configMap[]` to provide one or more
custom metrics files as a `configMap`.

You can create the `configMap` by running the following command:
```bash
kubectl create cm custom-metrics --from-file=metrics.toml
```

Finally, when creating or updating a `databaseObserver` resource, if we assume using the example above, you can set the fields in your YAML file as follows:
```yaml
spec:
  metrics:
    configMap:
      - name: custom-metrics
        key: "metrics.toml"
```

When configuring multiple configurations with different configmaps, you can do so by listing them under `spec.metrics.configMap`:
```yaml
spec:
  metrics:
    configMap:
      - name: custom-metrics
        key: "metrics.toml"
      - name: txeventq-metrics
        key: "txeventq.toml"
```

> Note: This mounts a volume named `metrics-volume` and in one mounted directory located inside the container `/oracle/observability` will include all
the provided TOML files.

### Prometheus Release
To enable your Prometheus configuration to find and include the `ServiceMonitor` created by the `databaseObserver` resource, the field `spec.serviceMonitor.labels` is an __important__ and __required__ field. The label on the ServiceMonitor
must match the `spec.serviceMonitorSelector` field in your Prometheus configuration.

```yaml
  serviceMonitor:
    labels:
      release: prometheus
```

## Scraping Logs
Currently, the observability exporter provides the `alert.log` from Oracle Database, which provides important information about errors and exceptions during database operations. 

By default, the logs are stored in the pod filesystem, inside `/log/alert.log`. Note that the log can also be placed in a custom path with a custom filename, You can also place a volume available to multiple pods with the use of `PersistentVolumes` by specifying a `persistentVolumeClaim`. 
Because the logs are stored in a file, scraping the logs must be pushed to a log aggregation system, such as _Loki_. 
In the following example, `Promtail` is used as a sidecar container that ships the contents of local logs to the Loki instance.


To configure the `databaseObserver` resource with a sidecar, two fields can be used:
```yaml
spec:
  sidecar: 
    containers: []
    volumes: []
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
      name: log-volume
      persistentVolumeClaim:
        claimName: "my-pvc"
```

### Working with Sidecars to deploy Promtail
The fields `spec.sidecars` and `spec.sidecarVolumes` provide the ability to deploy container images as a sidecar container
alongside the `observability-exporter` container.

You can specify container images to deploy inside `spec.sidecars` as you would normally define a container in a deployment. The field
`spec.sidecars` is of an array of containers (`[]corev1.Container`).

For example, to deploy a Grafana Promtail image, you can specify the container and its details as an element to the array, `spec.sidecars`.
```yaml
  sidecar:
    containers:
    - name: promtail
      image: grafana/promtail
      args:
        - -config.file=/etc/promtail/config.yaml
      volumeMounts:
        - name: promtail-config-volume
          mountPath: /etc/promtail
        - name: log-volume
          mountPath: /log  
```

> Important Note: The log volume is set by the controller with the name `log-volume` by default, unless set in `spec.log.volume.name`.

In the field `spec.sidecar.volumes`, you can specify and list the volumes you need in your sidecar containers. The field
`spec.sidecar.volumes` is an array of Volumes (`[]corev1.Volume`).

For example, when deploying the Promtail container, you can specify in the field any volume that needs to be mounted in the sidecar container above.

```yaml
  volumes:
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

## Customizing Resources and Available Configuration Options

### Environment Variables and Default Values
The following environment variables are set and provided by the controller by default:

| Environment Variable       | Related Field                           | Default                                   | Details                                                                                                         |
|----------------------------|-----------------------------------------|-------------------------------------------|-----------------------------------------------------------------------------------------------------------------|
| `ORACLE_HOME`              | -                                       | /lib/oracle/21/client64/lib               | Location of the Oracle Instant Client                                                                           |
| `TNS_ADMIN`                | -                                       | /lib/oracle/21/client64/lib/network/admin | Location of your (unzipped) wallet                                                                              |
| `DB_USERNAME`              | database.dbUser                         | -                                         | Database username, retrieved from a Kubernetes secret, not set or used for multi-databases                      |
| `DB_PASSWORD`              | database.dbPassword                     | -                                         | Database password, retrieved from a Kubernetes secret,  not set or used for multi-databases                     |
| `DB_CONNECT_STRING`        | database.dbConnectionString             | -                                         | Database connection string, retrieved from a Kubernetes secret,  not set or used for multi-databases            |
| `<key>_USERNAME`           | databases.&lt;key&gt;.dbUser            | -                                         | Database username, Environment Variable can be completely renamed through `dbUser.envName`                      |
| `<key>_PASSWORD`           | databases.&lt;key&gt;.dbPassword        | -                                         | Database password, Environment Variable can be completely renamed through `dbPassword.envName`                  |
| `<key>_CONNECT_STRING`     | database.&lt;key&gt;.dbConnectionString | -                                         | Database connection string, Environment Variable can be completely renamed through `dbConnectionString.envName` |
| `OCI_VAULT_ID`             | database.oci.vaultID                    | -                                         | Vault OCID containing the OCI Vault Secret referenced                                                           |
| `OCI_VAULT_SECRET_NAME`    | database.oci.vaultPasswordSecret        | -                                         | Vault Secret name containing the DB Password secret                                                             |
| `AZ_VAULT_ID`              | database.azure.vaultID                  | -                                         | Vault ID containing the Azure Vault Secret referenced                                                           |
| `AZ_VAULT_PASSWORD_SECRET` | database.azure.vaultPasswordSecret      | -                                         | Vault Secret name containing the DB Password secret                                                             |
| `AZ_VAULT_USERNAME_SECRET` | database.azure.vaultUsernameSecret      | -                                         | Vault Secret name containing the DB Username secret                                                             |
| `AZURE_TENANT_ID`          | azureConfig.configMap.name              | -                                         | Your Azure cloud Tenant ID                                                                                      |
| `AZURE_CLIENT_ID`          | azureConfig.configMap.name              | -                                         | Your Azure cloud client ID                                                                                      |
| `AZURE_CLIENT_SECRET`      | azureConfig.configMap.name              | -                                         | Your Azure cloud client secret                                                                                  |
| `LOG_DESTINATION`          | log.destination                         | /log/alert.log                            | Location of where alert.log will be placed                                                                      |
| `CUSTOM_METRICS`           | metrics.ConfigMap[]                     | -                                         | List of paths where the TOML files are located                                                                  |

The following default values or behavior is set:

| Usage                                       | Related Field(s)                             | Default                                                             | Details |
|---------------------------------------------|----------------------------------------------|---------------------------------------------------------------------|---------|
| Database Username Key                       | `database.dbUser.key`                        | username                                                            |         |
| Database Password Key                       | `database.dbPassword.key`                    | password                                                            |         |
| Database Connection String Key              | `database.dbConnectionString.key`            | connection                                                          |         |
| Wallet MountPath                            | `wallet.MountPath`                           | /lib/oracle/21/client64/lib/network/admin                           |         |
| Env Var for Database Username               | `databases.<key>.dbUser.envName`             | &lt;key&gt;_USERNAME                                                |         |
| Env Var for Database Password               | `databases.<key>.dbPassword.envName`         | &lt;key&gt;_PASSWORD                                                |         |
| Env Var for Database Connection String      | `databases.<key>.dbConnectionString.envName` | &lt;key&gt;_CONNECT_STRING                                          |         |
| Exporter Image                              | `deployment.image`                           | container-registry.oracle.com/database/observability-exporter:2.0.2 |         |
| Volume name of OCI Config file mounted      | `ociConfig.configMap.name`                   | oci-config-volume                                                   |         |
| Volume name of Metrics Config files mounted | `metrics.configMap.name`                     | metrics-volume                                                      |         |
| Volume name of Log volume mounted           | `log.volume.name`                            | log-volume                                                          |         |
| Volume name of Exporter Config mounted      | `exporterConfig.configMap.name`              | config-volume                                                       |         |
| Volume name of Wallet                       | `wallet`                                     |                                                                     |         |
| Log config filename                         | `log.filename`                               | alert.log                                                           |         |
| Log config file location for mounting       | `log.destination`                            | /log                                                                |         |
| Metrics config files location for mounting  | `metrics`                                    | /oracle/observability                                               |         |
| Exporter config file location for mounting  | `exporterConfig.mountPath`                   | /config                                                             |         |
| OCI Config file location for mounting       | `ociConfig.mountPath`                        | /.oci                                                               |         |

Overwriting environment variables can be managed by configuring the fields `spec.deployment.envs` or `spec.deployment.args` or through the [exporter config file](#defining-an-exporter-config-file):
```yaml
spec:
  deployment:
    args:
      - "--config.file=/location"
    env:
      TNS_ADMIN: /path/to/new/location
```
These variables and configuration values can be set explicitly, variables such as:
- DB_ROLE
- DATABASE_MAXIDLECONNS
- DATABASE_MAXOPENCONNS
- DATABASE_POOLINCREMENT
- DATABASE_POOLMAXCONNECTIONS
- DATABASE_POOLMINCONNECTIONS

### Managing Labels

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
  deployment:
    labels:
    podTemplate:
      labels:
  service:
    labels:
  serviceMonitor:
    labels:
```

### Custom Exporter Image or Version
The field `spec.deployment.image` is provided to enable you to make use of a newer or older version of the [observability-exporter](https://github.com/oracle/oracle-db-appdev-monitoring)
container image.

```yaml
spec:
  deployment:
    image: "container-registry.oracle.com/database/observability-exporter:2.0.1"
```

### Custom Environment Variables, Arguments and Commands
The fields `spec.deployment.env`, `spec.deployment.args` and `spec.deployment.commands` are provided for adding custom environment variables, arguments (`args`) and commands to the containers. 
Any custom environment variable will overwrite environment variables set by the controller.

```yaml
spec:
  deployment:
    env:
      DB_ROLE: "SYSDBA"
      TNS_ADMIN: "/path/to/wallet"
    args:
      - "--log.level=info"
    commands:
      - "/oracledb_exporter"
```


### Security Contexts
The security context defines privilege and access control settings for a pod container. If these privileges and access control setting need to be updated in the pod, then the same field is available on the `databaseObserver` spec. You can set this object under deployment: `spec.deployment.securityContext`.

```yaml
spec:
  deployment:
    securityContext:
        runAsUser: 1000
```

Configuring security context under the PodTemplate is also possible. You can set this object under: `spec.deployment.podTemplate.securityContext`

```yaml
spec:
  deployment:
    podSecurityContext:
      supplementalGroups: [1000]
          
```

### Custom Service Ports
The field `spec.service.ports` is provided to enable setting the ports of the service. If not set, then the following definition is set by default.

```yaml
spec:
  service:
    ports:
      - name: metrics
        port: 9161
        targetPort: 9161
      
```

### Custom ServiceMonitor Endpoints
The field `spec.serviceMonitor.endpoints` is provided for providing custom endpoints for the ServiceMonitor resource created by the `databaseObserver`:

```yaml
spec:
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
the error using the resource's event stream, or the resource's Conditions.

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

## Known Issues

### Using the OCI Vault (or Azure Vault) causes an error

> The development team has identified an issue with v2.0.2 of the exporter when using the OCI vault to store only the username or only the password, leading to an error.
> Using the Azure Vault to store only the username or the password will likely produce the same error.

__WORKAROUND:__<br/>
Because the OCI Vault feature is not functioning for v2 to v2.0.2 of the exporter, Oracle recomends that you use Kubernetes secrets. 
For Azure users, only retrieval of both username and password from the Vault is supported. 
Retrieving only one (username or password) of the credentials will lead to an error.

__WORKAROUND AFTER V2.0.2__:
When a new version of the exporter is released with the fix, set the field `deployment.image` to the new version of the exporter.

## Resources
For further information about the Oracle Databases logs and metrics Exporter container image, 
consult the official repository documentations:
- [GitHub - Unified Observability for Oracle Database Project](https://github.com/oracle/oracle-db-appdev-monitoring)


