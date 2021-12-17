# Managing Oracle Autonomous Databases with Oracle Database Operator for Kubernetes

Before you use the Oracle Database Operator for Kubernetes (the operator), ensure your system meets all of the Oracle Autonomous Database (ADB) Prerequisites [ADB_PREREQUISITES](./ADB_PREREQUISITES.md).

## Required Permissions

The opeartor must be given the required type of access in a policy written by an administrator to manage the Autonomous Databases. See [Let database and fleet admins manage Autonomous Databases](https://docs.oracle.com/en-us/iaas/Content/Identity/Concepts/commonpolicies.htm#db-admins-manage-adb) for sample Autonomous Database policies.

The permission to view the workrequests is also required, so that the operator will update the resources when the work is done. See [Viewing Work Requests](https://docs.oracle.com/en-us/iaas/Content/ContEng/Tasks/contengviewingworkrequests.htm#contengviewingworkrequests) for sample work request policies.

## Supported Features

After the operator is deployed, choose either one of the following operations to create an `AutonomousDatabase` custom resource for Oracle Autonomous Database in your cluster.

* [Provision](#provision-an-autonomous-database) an Autonomous Database
* [Bind](#bind-to-an-existing-autonomous-database) to an existing Autonomous Database

After you create the resource, you can use the operator to perform the following tasks:

* [Scale the OCPU core count or storage](#scale-the-ocpu-core-count-or-storage) an Autonomous Database
* [Rename](#rename) an Autonomous Database
* [Manage ADMIN database user password](#manage-admin-passsword) of an Autonomous Database
* [Download instance credentials (wallets)](#download-wallets) of an Autonomous Database
* [Stop/Start/Terminate](#stopstartterminate) an Autonomous Database
* [Delete the resource](#delete-the-resource) from the cluster

To debug the Oracle Autonomous Databases with Oracle Database Operator, see [Debugging and troubleshooting](#debugging-and-troubleshooting)

## Provision an Autonomous Database

Follow the steps to provision an Autonomous Database that will bind objects in your cluster.

1. Get the `Compartment OCID`.

    Login cloud console and click `Compartment`.

    ![compartment-1](/images/adb/compartment-1.png)

    Click on the compartment name where you want to create your database, and **copy** the `OCID` of the compartment.

    ![compartment-2](/images/adb/compartment-2.png)

2. Create a Kubernetes Secret to hold the password of the ADMIN user.

    You can create this secret with the following command (as an example):

    ```sh
    kubectl create secret generic admin-password --from-literal=admin-password='password_here'
    ```

3. Add the following fields to the AutonomousDatabase resource definition. An example `.yaml` file is available here: [`config/samples/adb/autonomousdatabase_create.yaml`](./../../config/samples/adb/autonomousdatabase_create.yaml)
    | Attribute | Type | Description | Required? |
    |----|----|----|----|
    | `spec.details.compartmentOCID` | string | The [OCID](https://docs.cloud.oracle.com/Content/General/Concepts/identifiers.htm) of the compartment of the Autonomous Database. | Yes |
    | `spec.details.dbName` | string | The database name. The name must begin with an alphabetic character and can contain a maximum of 14 alphanumeric characters. Special characters are not permitted. The database name must be unique in the tenancy. | Yes |
    | `spec.details.displayName` | string | The user-friendly name for the Autonomous Database. The name does not have to be unique. | Yes |
    | `spec.details.cpuCoreCount` | int | The number of OCPU cores to be made available to the database. | Yes |
    | `spec.details.adminPassword` | dictionary | The password for the ADMIN user. The password must be between 12 and 30 characters long, and must contain at least 1 uppercase, 1 lowercase, and 1 numeric character. It cannot contain the double quote symbol (") or the username "admin", regardless of casing.<br><br> Either `k8sSecretName` or `ociSecretOCID` must be provided. If both `k8sSecretName` and `ociSecretOCID` appear, the Operator reads the password from the K8s secret that `k8sSecretName` refers to. | Yes |
    | `spec.details.adminPassword.k8sSecretName` | string | The **name** of the K8s Secret where you want to hold the password for the ADMIN user. | Conditional |
    |`spec.details.adminPassword.ociSecretOCID` | string | The **[OCID](https://docs.cloud.oracle.com/Content/General/Concepts/identifiers.htm)** of the [OCI Secret](https://docs.oracle.com/en-us/iaas/Content/KeyManagement/Tasks/managingsecrets.htm) where you want to hold the password for the ADMIN user. | Conditional |
    | `spec.details.dataStorageSizeInTBs`  | int | The size, in terabytes, of the data volume that will be created and attached to the database. This storage can later be scaled up if needed. | Yes |
    | `spec.details.isAutoScalingEnabled`  | boolean | Indicates if auto scaling is enabled for the Autonomous Database OCPU core count. The default value is `FALSE` | No |
    | `spec.details.isDedicated` | boolean | True if the database is on dedicated [Exadata infrastructure](https://docs.cloud.oracle.com/Content/Database/Concepts/adbddoverview.htm) | No |
    | `spec.details.freeformTags` | dictionary | Free-form tags for this resource. Each tag is a simple key-value pair with no predefined name, type, or namespace. For more information, see [Resource Tag](https://docs.cloud.oracle.com/Content/General/Concepts/resourcetags.htm).<br><br> Example:<br> `freeformTags:`<br> &nbsp;&nbsp;&nbsp;&nbsp;`key1: value1`<br> &nbsp;&nbsp;&nbsp;&nbsp;`key2: value2`| No |
    | `spec.details.dbWorkload` | string | The Oracle Autonomous Database workload type. The following values are valid:<br> - OLTP - indicates an Autonomous Transaction Processing database<br> - DW - indicates an Autonomous Data Warehouse database<br> - AJD - indicates an Autonomous JSON Database<br> - APEX - indicates an Autonomous Database with the Oracle APEX Application Development workload type. | No |
    | `spec.details.dbVersion` | string | A valid Oracle Database release for Oracle Autonomous Database. | No |
    | `spec.ociConfig` | dictionary | Not required when the Operator is authorized with [Instance Principal](./ADB_PREREQUISITES.md#authorized-with-instance-principal). Otherwise, you will need the values from the [Authorized with API Key Authentication](./ADB_PREREQUISITES.md#authorized-with-api-key-authentication) section. | Conditional |
    | `spec.ociConfig.configMapName` | string | Name of the ConfigMap that holds the local OCI configuration | Conditional |
    | `spec.ociConfig.secretName`| string | Name of the K8s Secret that holds the private key value | Conditional |

    ```yaml
    ---
    apiVersion: database.oracle.com/v1alpha1
    kind: AutonomousDatabase
    metadata:
      name: autonomousdatabase-sample
    spec:
      details:
        compartmentOCID: ocid1.compartment...
        dbName: NewADB
        displayName: NewADB
        cpuCoreCount: 1
        adminPassword:
          k8sSecretName: admin-password # use the name of the secret from step 2
        dataStorageSizeInTBs: 1
      ociConfig:
        configMapName: oci-cred
        secretName: oci-privatekey
    ```

4. Apply the yaml:

    ```sh
    kubectl apply -f config/samples/adb/autonomousdatabase_create.yaml
    autonomousdatabase.database.oracle.com/autonomousdatabase-sample created
    ```

## Bind to an existing Autonomous Database

Other than provisioning a database, you can bind to an existing database in your cluster.

1. Clean up the resource you created in the earlier provision operation:

    ```sh
    kubectl delete adb/autonomousdatabase-sample
    autonomousdatabase.database.oracle.com/autonomousdatabase-sample deleted
    ```

2. Copy the `Autonomous Database OCID` from Cloud Console.

    ![adb-id-1](/images/adb/adb-id-1.png)

    ![adb-id-2](/images/adb/adb-id-2.png)

3. Add the following fields to the AutonomousDatabase resource definition. An example `.yaml` file is available here: [`config/samples/adb/autonomousdatabase_bind.yaml`](./../../config/samples/adb/autonomousdatabase_bind.yaml)
    | Attribute | Type | Description | Required? |
    |----|----|----|----|
    | `spec.details.autonomousDatabaseOCID` | string | The [OCID](https://docs.cloud.oracle.com/Content/General/Concepts/identifiers.htm) of the Autonomous Database you want to bind (create a reference) in your cluster. | Yes |
    | `spec.ociConfig` | dictionary | Not required when the Operator is authorized with [Instance Principal](./ADB_PREREQUISITES.md#authorized-with-instance-principal). Otherwise, you will need the values from the [Authorized with API Key Authentication](./ADB_PREREQUISITES.md#authorized-with-api-key-authentication) section. | Conditional |
    | `spec.ociConfig.configMapName` | string | Name of the ConfigMap that holds the local OCI configuration | Conditional |
    | `spec.ociConfig.secretName`| string | Name of the K8s Secret that holds the private key value | Conditional |

    ```yaml
    ---
    apiVersion: database.oracle.com/v1alpha1
    kind: AutonomousDatabase
    metadata:
      name: autonomousdatabase-sample
    spec:
      details:
        autonomousDatabaseOCID: ocid1.autonomousdatabase...
      ociConfig:
        configMapName: oci-cred
        secretName: oci-privatekey
    ```

4. Apply the yaml.

    ```sh
    kubectl apply -f config/samples/adb/autonomousdatabase_bind.yaml
    autonomousdatabase.database.oracle.com/autonomousdatabase-sample created
    ```

## Scale the OCPU core count or storage

> Note: this operation requires an `AutonomousDatabase` object to be in your cluster. This example assumes either the provision operation or the bind operation has been done by the users and the operator is authorized with API Key Authentication.

Users can scale up or scale down the Oracle Autonomous Database OCPU core count or storage by updating the `cpuCoreCount` and `dataStorageSizeInTBs` parameters. The `isAutoScalingEnabled` indicates whether auto scaling is enabled. Here is an example of scaling the CPU count and storage size (TB) up to 2 and turning off the auto-scaling by updating the `autonomousdatabase-sample` custom resource.

1. An example YAML file is available here: [config/samples/adb/autonomousdatabase_scale.yaml](./../../config/samples/adb/autonomousdatabase_scale.yaml)

    ```yaml
    ---
    apiVersion: database.oracle.com/v1alpha1
    kind: AutonomousDatabase
    metadata:
      name: autonomousdatabase-sample
    spec:
      details:
        autonomousDatabaseOCID: ocid1.autonomousdatabase...
        cpuCoreCount: 2
        dataStorageSizeInTBs: 2
        isAutoScalingEnabled: false
      ociConfig:
        configMapName: oci-cred
        secretName: oci-privatekey
    ```

2. Apply the change using `kubectl`.

    ```sh
    kubectl apply -f config/samples/adb/autonomousdatabase_scale.yaml
    autonomousdatabase.database.oracle.com/autonomousdatabase-sample configured
    ```

## Rename

> Note: this operation requires an `AutonomousDatabase` object to be in your cluster. This example assumes the provision operation or the bind operation has been completed, and the operator is authorized with API Key Authentication.

You can rename the database by changing the values of the `dbName` and `displayName`, as follows:

1. An example YAML file is available here: [config/samples/adb/autonomousdatabase_rename.yaml](./../../config/samples/adb/autonomousdatabase_rename.yaml)

    ```yaml
    ---
    apiVersion: database.oracle.com/v1alpha1
    kind: AutonomousDatabase
    metadata:
      name: autonomousdatabase-sample
    spec:
      details:
        autonomousDatabaseOCID: ocid1.autonomousdatabase...
        dbName: RenamedADB
        displayName: RenamedADB
      ociConfig:
        configMapName: oci-cred
        secretName: oci-privatekey
    ```

    * `dbName`: The database name. It must begin with an alphabetic character. It can contain a maximum of 14 alphanumeric characters. Special characters are not permitted. The database name must be unique in the tenancy.
    * `displayNameName`: User-friendly name of the database. The name does not have to be unique.

2. Apply the change using `kubectl`.

    ```sh
    kubectl apply -f config/samples/adb/autonomousdatabase_rename.yaml
    autonomousdatabase.database.oracle.com/autonomousdatabase-sample configured
    ```

## Manage Admin Passsword

> Note: this operation requires an `AutonomousDatabase` object to be in your cluster. This example assumes the provision operation or the bind operation has been completed, and the operator is authorized with API Key Authentication.

1. Create a Kubernetes Secret to hold the new password of the ADMIN user.

    As an example, you can create this secret with the following command: *

    ```sh
    kubectl create secret generic new-adb-admin-password --from-literal=new-adb-admin-password='password_here'
    ```

    \* The password must be between 12 and 30 characters long, and must contain at least 1 uppercase, 1 lowercase, and 1 numeric character. It cannot contain the double quote symbol (") or the username "admin", regardless of casing.

2. Update the example [config/samples/adb/autonomousdatabase_change_admin_password.yaml](./../../config/samples/adb/autonomousdatabase_change_admin_password.yaml)

    ```yaml
    ---
    apiVersion: database.oracle.com/v1alpha1
    kind: AutonomousDatabase
    metadata:
      name: autonomousdatabase-sample
    spec:
      details:
        autonomousDatabaseOCID: ocid1.autonomousdatabase...
        adminPassword:
          k8sSecretName: new-admin-password
      ociConfig:
        configMapName: oci-cred
        secretName: oci-privatekey
    ```

    * `adminPassword.k8sSecretName`: the **name** of the secret that you created in **step1**.

3. Apply the YAML.

    ```sh
    kubectl apply -f config/samples/adb/autonomousdatabase_change_admin_password.yaml
    autonomousdatabase.database.oracle.com/autonomousdatabase-sample configured
    ```

## Download Wallets

> Note: this operation requires an `AutonomousDatabase` object to be in your cluster. This example assumes the provision operation or the bind operation has been done by the users and the operator is authorized with API Key Authentication.

A client Wallet is required to connect to a shared Oracle Autonomous Database. User has to provide a wallet password to download the Wallet. In the following example, the Operator will read the password from a Kubernetes Secret to download the Wallet. After that, the downloaded Wallet will be unzipped and stored as byte values in a new Kubernetes Secret `instance-wallet`.

1. Create a Kubernetes Secret to hold the wallet password.

    As an example, you can create this secret with the following command: *

    ```sh
    kubectl create secret generic instance-wallet-password --from-literal=instance-wallet-password='password_here'
    ```

    \* The password must be at least 8 characters long and must include at least 1 letter and either 1 numeric character or 1 special character.

2. Update the example [config/samples/adb/autonomousdatabase_wallet.yaml](./../../config/samples/adb/autonomousdatabase_wallet.yaml)

    ```yaml
    ---
    apiVersion: database.oracle.com/v1alpha1
    kind: AutonomousDatabase
    metadata:
      name: autonomousdatabase-sample
    spec:
      details:
        autonomousDatabaseOCID: ocid1.autonomousdatabase...
        wallet:
          name: instance-wallet
          password:
            k8sSecretName: instance-wallet-password
      ociConfig:
        configMapName: oci-cred
        secretName: oci-privatekey
    ```

    * `wallet.name`: the name of the new Secret where you want the downloaded Wallet to be stored.
    * `wallet.password.k8sSecretName`: the **name** of the secret you created in **step1**.

3. Apply the YAML

    ```sh
    kubectl apply -f config/samples/adb/autonomousdatabase_wallet.yaml
    autonomousdatabase.database.oracle.com/autonomousdatabase-sample configured
   ```

You should see a new Secret `instance-wallet` in your cluster:

```sh
$ kubectl get secrets
NAME                         TYPE                                  DATA   AGE
oci-privatekey               Opaque                                1      2d12h
instance-wallet-password     Opaque                                1      2d12h
instance-wallet              Opaque                                8      2d12h
```

To use the secret in a deployment, refer to [Using Secrets](https://kubernetes.io/docs/concepts/configuration/secret/#using-secrets) for the examples.

## Stop/Start/Terminate

> Note: this operation requires an `AutonomousDatabase` object to be in your cluster. This example assumes the provision operation or the bind operation has been done by the users and the operator is authorized with API Key Authentication.

Users can start/stop/terminate a database using the `lifecycleState` attribute.
Here's a list of the values you can set for `lifecycleState`:

* `AVAILABLE`: to start the database
* `STOPPED`: to stop the database
* `TERMINATED`: to terminate the database

1. A sample .yaml file is available here: [config/samples/adb/autonomousdatabase_stop_start_terminate.yaml](./../../config/samples/adb/autonomousdatabase_stop_start_terminate.yaml)

    ```yaml
    ---
    apiVersion: database.oracle.com/v1alpha1
    kind: AutonomousDatabase
    metadata:
      name: autonomousdatabase-sample
    spec:
      details:
        autonomousDatabaseOCID: ocid1.autonomousdatabase...
        lifecycleState: STOPPED
      ociConfig:
        configMapName: oci-cred
        secretName: oci-privatekey
    ```

2. Apply the change to stop the database.

    ```sh
    kubectl apply -f config/samples/adb/autonomousdatabase_stop_start_terminate.yaml
    autonomousdatabase.database.oracle.com/autonomousdatabase-sample configured
    ```

## Delete the resource

> Note: this operation requires an `AutonomousDatabase` object to be in your cluster. This example assumes the provision operation or the bind operation has been done by the users and the operator is authorized with API Key Authentication.

The `hardLink` defines the behavior when the resource is deleted from the cluster. If the `hardLink` is set to true, the Operator terminates the Autonomous Database in OCI when the resource is removed; otherwise, the database remains unchanged. By default the value is `false` if it is not explicitly specified.

Follow the steps to delete the resource and terminate the Autonomous Database.

1. Use the example [autonomousdatabase_delete_resource.yaml](./../../config/samples/adb/autonomousdatabase_delete_resource.yaml) which sets the attribute `hardLink` to true.

    ```yaml
    ---
    apiVersion: database.oracle.com/v1alpha1
    kind: AutonomousDatabase
    metadata:
      name: autonomousdatabase-sample
    spec:
      details:
        autonomousDatabaseOCID: ocid1.autonomousdatabase...
      hardLink: true
      ociConfig:
        configMapName: oci-cred
        secretName: oci-privatekey
    ```

2. Apply the yaml

    ```sh
    kubectl apply -f config/samples/adb/autonomousdatabase_delete_resource.yaml
    autonomousdatabase.database.oracle.com/autonomousdatabase-sample configured
    ```

3. Delete the resource in your cluster

    ```sh
    kubectl delete adb/autonomousdatabase-sample
    autonomousdatabase.database.oracle.com/autonomousdatabase-sample deleted
    ```

Now, you can verify that the database is in TERMINATING state on the Cloud Console.


## Debugging and troubleshooting

### Show the details of the resource

If you edit and re-apply the `.yaml` file, the Autonomous Database controller will only update the parameters that the file contains. The parameters which are not in the file will not be impacted. To get the verbose output of the current spec, use below command:

```sh
kubectl describe adb/autonomousdatabase-sample
```

### The resource is in UNAVAILABLE state

If an error occurs during the operation, the `lifecycleState` of the resoursce changes to UNAVAILABLE. Follow the steps to check the logs.

1. List the pod replicas

    ```sh
    kubectl get pods -n oracle-database-operator-system
    ```

2. Use the below command to check the logs of the Pod which has a failure

    ```sh
    kubectl logs -f pod/oracle-database-operator-controller-manager-78666fdddb-s4xcm -n oracle-database-operator-system
    ```
