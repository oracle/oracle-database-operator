# Managing Oracle Autonomous Databases with Oracle Database Operator for Kubernetes

Before you use the Oracle Database Operator for Kubernetes (the operator), ensure that your system meets all of the Oracle Autonomous Database (ADB) Prerequisites [ADB_PREREQUISITES](./ADB_PREREQUISITES.md).

As indicated in the prerequisites (see above), to interact with OCI services, either the cluster must be authorized using Principal Instance, or the cluster must be authorized using the API Key Authentication by specifying the configMap and the secret under the `ociConfig` field.

## Required Permissions

The operator must be given the required type of access in a policy written by an administrator to manage the Autonomous Databases. For examples of Autonomous Database policies, see: [Let database and fleet admins manage Autonomous Databases](https://docs.oracle.com/en-us/iaas/Content/Identity/Concepts/commonpolicies.htm#db-admins-manage-adb)

Permissions to view the work requests are also required, so that the operator can update the resources when the work is done. For example work request policies, see: [Viewing Work Requests](https://docs.oracle.com/en-us/iaas/Content/ContEng/Tasks/contengviewingworkrequests.htm#contengviewingworkrequests) 

## Supported Features

After the operator is deployed, choose one of the following operations to create an `AutonomousDatabase` custom resource for Oracle Autonomous Database in your cluster.

* [Provision](#provision-an-autonomous-database) an Autonomous Database
* [Bind](#bind-to-an-existing-autonomous-database) to an existing Autonomous Database

After you create the resource, you can use the operator to perform the following tasks:

* [Scale the OCPU core count or storage](#scale-the-ocpu-core-count-or-storage) an Autonomous Database
* [Rename](#rename) an Autonomous Database
* [Manage ADMIN database user password](#manage-admin-password) of an Autonomous Database
* [Download instance credentials (wallets)](#download-wallets) of an Autonomous Database
* [Stop/Start/Terminate](#stopstartterminate) an Autonomous Database
* [Delete the resource](#delete-the-resource) from the cluster
* [Clone](#clone-an-existing-autonomous-database) an existing Autonomous Database
* [Switchover](#switchover-an-existing-autonomous-database) an existing Autonomous Database
* [Perform Manual Failover](#manually-failover-an-existing-autonomous-database) to an existing Autonomous Database

To debug the Oracle Autonomous Databases with Oracle Database Operator, see [Debugging and troubleshooting](#debugging-and-troubleshooting)

## Provision an Autonomous Database

To provision an Autonomous Database that will map objects in your cluster, complete the following steps:

1. Obtain the `Compartment OCID`.

    Log in to the Cloud Console and click `Compartment`.

    ![compartment-1](/images/adb/compartment-1.png)

    Click on the compartment name where you want to create your database, and **copy** the `OCID` of the compartment.

    ![compartment-2](/images/adb/compartment-2.png)

2. To create an Autonomous Database on Dedicated Exadata Infrastructure (ADB-D), the OCID of the Oracle Autonomous Container Database is required.

    You can skip this step if you want to create a Autonomous Database on Shared Exadata Infrastructure (ADB-S).

    Go to the Cloud Console and click `Autonomous Database`.

    ![acd-id-1](/images/adb/adb-id-1.png)

    Under `Dedicated Infrastructure`, click `Autonomous Container Database`.

    ![acd-id-2](/images/adb/acd-id-1.png)

    Click on the name of Autonomous Container Database and copy the `Autonomous Container Database OCID` from the Cloud Console.

    ![acd-id-3](/images/adb/acd-id-2.png)

3. Create a Kubernetes Secret to hold the password of the ADMIN user. **The key and the name of the secret must be the same.**

    You can create this secret by using a command similar to the following example:

    ```sh
    kubectl create secret generic admin-password --from-literal=admin-password='password_here'
    ```

4. Add the following fields to the Autonomous Database resource definition. An example `.yaml` file is available here: [`config/samples/adb/autonomousdatabase_create.yaml`](./../../config/samples/adb/autonomousdatabase_create.yaml)
    | Attribute | Type | Description | Required? |
    |----|----|----|----|
    | `spec.details.compartmentId` | string | The [OCID](https://docs.cloud.oracle.com/Content/General/Concepts/identifiers.htm) of the compartment of the Autonomous Database. | Yes |
    | `spec.details.dbName` | string | The database name. The name must begin with an alphabetic character and can contain a maximum of 14 alphanumeric characters. Special characters are not permitted. The database name must be unique in the tenancy. | Yes |
    | `spec.details.displayName` | string | The user-friendly name for the Autonomous Database. The name does not have to be unique. | Yes |
    | `spec.details.dbWorkload` | string | The Autonomous Database workload type. The following values are valid:<br> `OLTP` - indicates an Autonomous Transaction Processing database<br> `DW` - indicates an Autonomous Data Warehouse database<br> `AJD` - indicates an Autonomous JSON Database<br> `APEX` - indicates an Autonomous Database with the Oracle APEX Application Development workload type.<br> This cannot be updated in parallel with any of the following: licenseModel, cpuCoreCount, computeCount, computeModel, adminPassword, whitelistedIps, isMtlsConnectionRequired, privateEndpointLabel, nsgIds, dbVersion, dbName, or isFreeTier. | No |
    | `spec.details.licenseModel` | string | The Oracle license model that applies to the Oracle Autonomous Database. Bring your own license (BYOL) allows you to apply your current on-premises Oracle software licenses to equivalent, highly automated Oracle services in the cloud.License Included allows you to subscribe to new Oracle Database software licenses and the Oracle Database service. Note that when provisioning an [Autonomous Database on dedicated Exadata infrastructure](https://docs.oracle.com/en/cloud/paas/autonomous-database/index.html), this attribute must be null. It is already set at the Autonomous Exadata Infrastructure level. When provisioning an [Autonomous Database Serverless ](https://docs.oracle.com/en/cloud/paas/autonomous-database/index.html) database, if a value is not specified, the system defaults the value to `BRING_YOUR_OWN_LICENSE`. Bring your own license (BYOL) also allows you to select the DB edition using the optional parameter.<br> This cannot be updated in parallel with any of the following: cpuCoreCount, computeCount, dataStorageSizeInTBs, adminPassword, isMtlsConnectionRequired, dbWorkload, privateEndpointLabel, nsgIds, dbVersion, dbName, or isFreeTier. | No |
    | `spec.details.dbVersion` | string | A valid Oracle Database version for Autonomous Database. | No |
    | `spec.details.dataStorageSizeInTBs` | int | The size, in terabytes, of the data volume that will be created and attached to the database. This storage can later be scaled up if needed. For Autonomous Databases on dedicated Exadata infrastructure, the maximum storage value is determined by the infrastructure shape. See Characteristics of [Infrastructure Shapes](https://www.oracle.com/pls/topic/lookup?ctx=en/cloud/paas/autonomous-database&id=ATPFG-GUID-B0F033C1-CC5A-42F0-B2E7-3CECFEDA1FD1) for shape details. A full Exadata service is allocated when the Autonomous Database size is set to the upper limit (384 TB). | No |
    | `spec.details.cpuCoreCount` | int | The number of CPU cores to be made available to the database. For Autonomous Databases on dedicated Exadata infrastructure, the maximum number of cores is determined by the infrastructure shape. See [Characteristics of Infrastructure Shapes](https://www.oracle.com/pls/topic/lookup?ctx=en/cloud/paas/autonomous-database&id=ATPFG-GUID-B0F033C1-CC5A-42F0-B2E7-3CECFEDA1FD1) for shape details.<br>**Note:** This parameter cannot be used with the `ocpuCount` parameter. | Conditional |
    | `spec.details.computeModel` | string | The compute model of the Autonomous Database. This is required if using the `computeCount` parameter. If using `cpuCoreCount` then it is an error to specify `computeModel` to a non-null value. ECPU compute model is the recommended model and OCPU compute model is legacy. | Conditional |
    | `spec.details.computeCount` | float32 | The compute amount (CPUs) available to the database. Minimum and maximum values depend on the compute model and whether the database is an Autonomous Database Serverless instance or an Autonomous Database on Dedicated Exadata Infrastructure.<br> For an Autonomous Database Serverless instance, the 'ECPU' compute model requires a minimum value of one, for databases in the elastic resource pool and minimum value of two, otherwise. Required when using the `computeModel` parameter. When using `cpuCoreCount` parameter, it is an error to specify computeCount to a non-null value. Providing `computeModel` and `computeCount` is the preferred method for both OCPU and ECPU. | Conditional |
    | `spec.details.ocpuCount` | float32 | The number of OCPU cores to be made available to the database.<br>The following points apply:<br> - For Autonomous Databases on Dedicated Exadata infrastructure, to provision less than 1 core, enter a fractional value in an increment of 0.1. For example, you can provision 0.3 or 0.4 cores, but not 0.35 cores. (Note that fractional OCPU values are not supported for Autonomous Database Serverless instances.)<br> - To provision 1 or more cores, you must enter an integer between 1 and the maximum number of cores available for the infrastructure shape. For example, you can provision 2 cores or 3 cores, but not 2.5 cores. This applies to an Autonomous Database Serverless instance or an Autonomous Database on Dedicated Exadata Infrastructure.<br> - For Autonomous Database Serverless instances, this parameter is not used.<br> For Autonomous Databases on Dedicated Exadata infrastructure, the maximum number of cores is determined by the infrastructure shape. See [Characteristics of Infrastructure Shapes](https://www.oracle.com/pls/topic/lookup?ctx=en/cloud/paas/autonomous-database&id=ATPFG-GUID-B0F033C1-CC5A-42F0-B2E7-3CECFEDA1FD1) for shape details.<br> **Note:** This parameter cannot be used with the `cpuCoreCount` parameter. | Conditional |
    | `spec.details.adminPassword` | dictionary | The password for the ADMIN user. The password must be between 12 and 30 characters long, and must contain at least 1 uppercase, 1 lowercase, and 1 numeric character. It cannot contain the double quote symbol (") or the username "admin", regardless of casing.<br><br> Either `k8sSecret.name` or `ociSecret.id` must be provided. If both `k8sSecret.name` and `ociSecret.id` appear, the Operator reads the password from the K8s secret that `k8sSecret.name` refers to. | Yes |
    | `spec.details.adminPassword.k8sSecret.name` | string | The **name** of the K8s Secret where you want to hold the password for the ADMIN user. | Conditional |
    |`spec.details.adminPassword.ociSecret.id` | string | The **[OCID](https://docs.cloud.oracle.com/Content/General/Concepts/identifiers.htm)** of the [OCI Secret](https://docs.oracle.com/en-us/iaas/Content/KeyManagement/Tasks/managingsecrets.htm) where you want to hold the password for the ADMIN user. | Conditional |
    | `spec.details.dataStorageSizeInTBs`  | int | The size, in terabytes, of the data volume that will be created and attached to the database. This storage can later be scaled up if needed. | Yes |
    | `spec.details.isAutoScalingEnabled`  | boolean | Indicates if auto scaling is enabled for the Autonomous Database OCPU core count. The default value is `FALSE` | No |
    | `spec.details.isDedicated` | boolean | True if the database is on dedicated [Exadata infrastructure](https://docs.cloud.oracle.com/Content/Database/Concepts/adbddoverview.htm). `spec.details.autonomousContainerDatabase.k8sACD.name` or `spec.details.autonomousContainerDatabase.ociACD.id` has to be provided if the value is true. | No |
    | `spec.details.isFreeTier` | boolean | Indicates if this is an Always Free resource. The default value is false. Note that Always Free Autonomous Databases have 1 CPU and 20GB of memory. For Always Free databases, memory and CPU cannot be scaled.<br>This cannot be updated in parallel with any of the following: licenseModel, cpuCoreCount, computeCount, computeModel, adminPassword, whitelistedIps, isMtlsConnectionRequired, privateEndpointLabel, nsgIds, dbVersion, or dbName. | No |
    | `spec.details.isAccessControlEnabled` | boolean | Indicates if the database-level access control is enabled.<br>If disabled, database access is defined by the network security rules.<br>If enabled, database access is restricted to the IP addresses defined by the rules specified with the `whitelistedIps` property. While specifying `whitelistedIps` rules is optional, if database-level access control is enabled and no rules are specified, the database will become inaccessible.<br>When creating a database clone, the desired access control setting should be specified. By default, database-level access control will be disabled for the clone.<br>This property is applicable only to Autonomous Databases on the Exadata Cloud@Customer platform. For Autonomous Database Serverless instances, `whitelistedIps` is used. | No |
    | `spec.details.whitelistedIps` | []string | The client IP access control list (ACL). This feature is available for [Autonomous Database Serverless](https://docs.oracle.com/en/cloud/paas/autonomous-database/index.html) and on Exadata Cloud@Customer.<br>Only clients connecting from an IP address included in the ACL may access the Autonomous Database instance.<br>If `arePrimaryWhitelistedIpsUsed` is 'TRUE' then Autonomous Database uses this primary's IP access control list (ACL) for the disaster recovery peer called `standbywhitelistedips`.<br>For Autonomous Database Serverless, this is an array of CIDR (classless inter-domain routing) notations for a subnet or VCN OCID (virtual cloud network Oracle Cloud ID).<br>Multiple IPs and VCN OCIDs should be separate strings separated by commas. However, if other configurations require multiple pieces of information, then each piece is connected with semicolon (;) as a delimiter.<br>Example: `["1.1.1.1","1.1.1.0/24","ocid1.vcn.oc1.sea.<unique_id>","ocid1.vcn.oc1.sea.<unique_id1>;1.1.1.1","ocid1.vcn.oc1.sea.<unique_id2>;1.1.0.0/16"]`<br>For Exadata Cloud@Customer, this is an array of IP addresses or CIDR notations.<br>Example: `["1.1.1.1","1.1.1.0/24","1.1.2.25"]`<br>For an update operation, if you want to delete all the IPs in the ACL, use an array with a single empty string entry.<br>This cannot be updated in parallel with any of the following: licenseModel, cpuCoreCount, computeCount, computeModel, adminPassword, isMtlsConnectionRequired, dbWorkload, dbVersion, dbName, or isFreeTier. | No |
    | `spec.details.subnetId` | string | The [OCID](https://docs.cloud.oracle.com/Content/General/Concepts/identifiers.htm) of the subnet the resource is associated with.<br> **Subnet Restrictions:**<br> - For Autonomous Database, setting this will disable public secure access to the database.<br> These subnets are used by the Oracle Clusterware private interconnect on the database instance.<br> Specifying an overlapping subnet will cause the private interconnect to malfunction.<br> This restriction applies to both the client subnet and the backup subnet. | No |
    | `spec.details.nsgIds` | []string | The list of [OCIDs](https://docs.cloud.oracle.com/Content/General/Concepts/identifiers.htm) for the network security groups (NSGs) to which this resource belongs. Setting this to an empty list removes all resources from all NSGs. For more information about NSGs, see [Security Rules](https://docs.cloud.oracle.com/Content/Network/Concepts/securityrules.htm).<br> **NsgIds restrictions:**<br> - A network security group (NSG) is optional for Autonomous Databases with private access. The nsgIds list can be empty. | No |
    | `spec.details.privateEndpointLabel` | string | The resource's private endpoint label.<br> - Setting the endpoint label to a non-empty string creates a private endpoint database.<br> - Resetting the endpoint label to an empty string, after the creation of the private endpoint database, changes the private endpoint database to a public endpoint database.<br> - Setting the endpoint label to a non-empty string value, updates to a new private endpoint database, when the database is disabled and re-enabled.<br> This setting cannot be updated in parallel with any of the following: licenseModel, cpuCoreCount, computeCount, computeModel, adminPassword, whitelistedIps, isMTLSConnectionRequired, dbWorkload, dbVersion, dbName, or isFreeTier. | No |
    | `spec.details.isMtlsConnectionRequired` | boolean | Specifies if the Autonomous Database requires mTLS connections. | No |
    | `spec.details.autonomousContainerDatabase.k8sACD.name` | string | The **name** of the K8s Autonomous Container Database resource | No |
    | `spec.details.autonomousContainerDatabase.ociACD.id` | string | The Autonomous Container Database [OCID](https://docs.cloud.oracle.com/Content/General/Concepts/identifiers.htm). | No |
    | `spec.details.freeformTags` | dictionary | Free-form tags for this resource. Each tag is a simple key-value pair with no predefined name, type, or namespace. For more information, see [Resource Tag](https://docs.cloud.oracle.com/Content/General/Concepts/resourcetags.htm).<br><br> Example:<br> `freeformTags:`<br> &nbsp;&nbsp;&nbsp;&nbsp;`key1: value1`<br> &nbsp;&nbsp;&nbsp;&nbsp;`key2: value2`| No |
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
      action: Create
      details:
        compartmentId: ocid1.compartment...
        dbName: NewADB
        displayName: NewADB
        computeModel: ECPU
        computeCount: 1
        adminPassword:
          k8sSecret:
            name: admin-password # use the name of the secret from step 2
        dataStorageSizeInTBs: 1
      ociConfig:
        configMapName: oci-cred
        secretName: oci-privatekey
    ```

5. Choose the type of network access (optional):

   By default, the network access type is set to PUBLIC, which allows secure connections from anywhere. Uncomment the code block if you want configure the network access. For more information, see: [Configuring Network Access of Autonomous Database](./NETWORK_ACCESS_OPTIONS.md)

6. Apply the YAML:

    ```sh
    kubectl apply -f config/samples/adb/autonomousdatabase_create.yaml
    autonomousdatabase.database.oracle.com/autonomousdatabase-sample created
    ```

## Bind to an existing Autonomous Database

Other than provisioning a database, you can create the custom resource using an existing Autonomous Database.

The operator also generates the `AutonomousBackup` custom resources if a database already has backups. The operator syncs the `AutonomousBackups` in every reconciliation loop by getting the list of OCIDs of the AutonomousBackups from OCI, and then creates the `AutonomousDatabaseBackup` object automatically if it cannot find a resource that has the same `AutonomousBackupOCID` in the cluster.

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
    | `spec.details.id` | string | The [OCID](https://docs.cloud.oracle.com/Content/General/Concepts/identifiers.htm) of the Autonomous Database that you want to bind (create a reference) in your cluster. | Yes |
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
      action: Sync
      details:
        id: ocid1.autonomousdatabase...
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

> Note: this operation requires an `AutonomousDatabase` object to be in your cluster. To use this example, either the provision operation or the bind operation must be completed, and the operator must be authorized with API Key Authentication.

You can scale up or scale down the Oracle Autonomous Database OCPU core count or storage by updating the `computeCount` and `dataStorageSizeInTBs` parameters. The `isAutoScalingEnabled` indicates whether auto scaling is enabled. In this example, the CPU count and storage size (TB) are scaled up to 2 and the auto-scaling is turned off by updating the `autonomousdatabase-sample` custom resource.

1. An example YAML file is available here: [config/samples/adb/autonomousdatabase_scale.yaml](./../../config/samples/adb/autonomousdatabase_scale.yaml)

    ```yaml
    ---
    apiVersion: database.oracle.com/v1alpha1
    kind: AutonomousDatabase
    metadata:
      name: autonomousdatabase-sample
    spec:
      action: Update
      details:
        id: ocid1.autonomousdatabase...
        computeCount: 2
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
      action: Update
      details:
        id: ocid1.autonomousdatabase...
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

## Manage Admin Password

> Note: this operation requires an `AutonomousDatabase` object to be in your cluster. This example assumes the provision operation or the bind operation has been completed, and the operator is authorized with API Key Authentication.

1. Create a Kubernetes Secret to hold the new password of the ADMIN user.

    As an example, you can create this secret with the following command: *

    ```sh
    kubectl create secret generic new-adb-admin-password --from-literal=new-adb-admin-password='password_here'
    ```

    \* The password must be between 12 and 30 characters long, and must contain at least 1 uppercase, 1 lowercase, and 1 numeric character. It cannot contain the double quote symbol (") or the username "admin", regardless of casing.

2. Update the example [config/samples/adb/autonomousdatabase_update_admin_password.yaml](./../../config/samples/adb/autonomousdatabase_update_admin_password.yaml)

    ```yaml
    ---
    apiVersion: database.oracle.com/v1alpha1
    kind: AutonomousDatabase
    metadata:
      name: autonomousdatabase-sample
    spec:
      action: Update
      details:
        id: ocid1.autonomousdatabase...
        adminPassword:
          k8sSecret:
            name: new-admin-password
      ociConfig:
        configMapName: oci-cred
        secretName: oci-privatekey
    ```

    * `adminPassword.k8sSecret.name`: the **name** of the secret that you created in **step1**.

3. Apply the YAML.

    ```sh
    kubectl apply -f config/samples/adb/autonomousdatabase_update_admin_password.yaml
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

2. Update the example [config/samples/adb/autonomousdatabase_download_wallet.yaml](./../../config/samples/adb/autonomousdatabase_download_wallet.yaml)

    ```yaml
    ---
    apiVersion: database.oracle.com/v1alpha1
    kind: AutonomousDatabase
    metadata:
      name: autonomousdatabase-sample
    spec:
      action: Update
      details:
        id: ocid1.autonomousdatabase...
      wallet:
        name: instance-wallet
        password:
          k8sSecret:
            name: instance-wallet-password
      ociConfig:
        configMapName: oci-cred
        secretName: oci-privatekey
    ```

    * `wallet.name`: the name of the new Secret where you want the downloaded Wallet to be stored.
    * `wallet.password.k8sSecret.name`: the **name** of the secret you created in **step1**.

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

To start, stop, or terminate a database, use the `action` attribute.
Here's a list of the values you can set for `action`:

* `Start`: to start the database
* `Stop`: to stop the database
* `Terminate`: to terminate the database

1. An example .yaml file is available here: [config/samples/adb/autonomousdatabase_stop_start_terminate.yaml](./../../config/samples/adb/autonomousdatabase_stop_start_terminate.yaml)

    ```yaml
    ---
    apiVersion: database.oracle.com/v1alpha1
    kind: AutonomousDatabase
    metadata:
      name: autonomousdatabase-sample
    spec:
      action: Stop
      details:
        id: ocid1.autonomousdatabase...
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

To delete the resource and terminate the Autonomous Database, complete these steps:

1. Use the example [autonomousdatabase_delete_resource.yaml](./../../config/samples/adb/autonomousdatabase_delete_resource.yaml), which sets the attribute `hardLink` to true.

    ```yaml
    ---
    apiVersion: database.oracle.com/v1alpha1
    kind: AutonomousDatabase
    metadata:
      name: autonomousdatabase-sample
    spec:
      action: Update
      details:
        id: ocid1.autonomousdatabase...
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

## Clone an existing Autonomous Database

> Note: this operation requires an `AutonomousDatabase` object to be in your cluster. This example assumes the provision operation or the bind operation has been done by the users and the operator is authorized with API Key Authentication.

To clone an existing Autonomous Database, complete these steps:

1. Add the following fields to the AutonomousDatabase resource definition. An example YAML file is available here: [config/samples/adb/autonomousdatabase_clone.yaml](./../../config/samples/adb/autonomousdatabase_clone.yaml)
    | Attribute | Type | Description | Required? |
    |----|----|----|----|
    | `spec.details.id` | string | The [OCID](https://docs.cloud.oracle.com/Content/General/Concepts/identifiers.htm) of the source Autonomous Database that you will clone to create a new Autonomous Database. | Yes |
    | `spec.clone.cloneType` | string | The Autonomous Database clone type. Accepted values are: `FULL` and `METADATA`. | No |
    | `spec.clone.compartmentId` | string | The [OCID](https://docs.cloud.oracle.com/Content/General/Concepts/identifiers.htm) of the compartment of the Autonomous Database. | Yes |
    | `spec.clone.dbName` | string | The database name. The name must begin with an alphabetic character and can contain a maximum of 14 alphanumeric characters. Special characters are not permitted. The database name must be unique in the tenancy. | Yes |
    | `spec.clone.displayName` | string | The user-friendly name for the Autonomous Database. The name does not have to be unique. | Yes |
    | `spec.clone.dbWorkload` | string | The Autonomous Database workload type. The following values are valid:<br> `OLTP` - indicates an Autonomous Transaction Processing database<br> `DW` - indicates an Autonomous Data Warehouse database<br> `AJD` - indicates an Autonomous JSON Database<br> `APEX` - indicates an Autonomous Database with the Oracle APEX Application Development workload type.<br> This cannot be updated in parallel with any of the following: licenseModel, cpuCoreCount, computeCount, computeModel, adminPassword, whitelistedIps, isMtlsConnectionRequired, privateEndpointLabel, nsgIds, dbVersion, dbName, or isFreeTier. | No |
    | `spec.clone.licenseModel` | string | The Oracle license model that applies to the Oracle Autonomous Database. Bring your own license (BYOL) allows you to apply your current on-premises Oracle software licenses to equivalent, highly automated Oracle services in the cloud.License Included allows you to subscribe to new Oracle Database software licenses and the Oracle Database service. Note that when provisioning an [Autonomous Database on dedicated Exadata infrastructure](https://docs.oracle.com/en/cloud/paas/autonomous-database/index.html), this attribute must be null. It is already set at the Autonomous Exadata Infrastructure level. When provisioning an [Autonomous Database Serverless ](https://docs.oracle.com/en/cloud/paas/autonomous-database/index.html) database, if a value is not specified, the system defaults the value to `BRING_YOUR_OWN_LICENSE`. Bring your own license (BYOL) also allows you to select the DB edition using the optional parameter.<br> This cannot be updated in parallel with any of the following: cpuCoreCount, computeCount, dataStorageSizeInTBs, adminPassword, isMtlsConnectionRequired, dbWorkload, privateEndpointLabel, nsgIds, dbVersion, dbName, or isFreeTier. | No |
    | `spec.clone.dbVersion` | string | A valid Oracle Database version for Autonomous Database. | No |
    | `spec.clone.dataStorageSizeInTBs` | int | The size, in terabytes, of the data volume that will be created and attached to the database. This storage can later be scaled up if needed. For Autonomous Databases on dedicated Exadata infrastructure, the maximum storage value is determined by the infrastructure shape. See Characteristics of [Infrastructure Shapes](https://www.oracle.com/pls/topic/lookup?ctx=en/cloud/paas/autonomous-database&id=ATPFG-GUID-B0F033C1-CC5A-42F0-B2E7-3CECFEDA1FD1) for shape details. A full Exadata service is allocated when the Autonomous Database size is set to the upper limit (384 TB). | No |
    | `spec.clone.cpuCoreCount` | int | The number of CPU cores to be made available to the database. For Autonomous Databases on dedicated Exadata infrastructure, the maximum number of cores is determined by the infrastructure shape. See [Characteristics of Infrastructure Shapes](https://www.oracle.com/pls/topic/lookup?ctx=en/cloud/paas/autonomous-database&id=ATPFG-GUID-B0F033C1-CC5A-42F0-B2E7-3CECFEDA1FD1) for shape details.<br>**Note:** This parameter cannot be used with the `ocpuCount` parameter. | Conditional |
    | `spec.clone.computeModel` | string | The compute model of the Autonomous Database. This is required if using the `computeCount` parameter. If using `cpuCoreCount` then it is an error to specify `computeModel` to a non-null value. ECPU compute model is the recommended model and OCPU compute model is legacy. | Conditional |
    | `spec.clone.computeCount` | float32 | The compute amount (CPUs) available to the database. Minimum and maximum values depend on the compute model and whether the database is an Autonomous Database Serverless instance or an Autonomous Database on Dedicated Exadata Infrastructure.<br> For an Autonomous Database Serverless instance, the 'ECPU' compute model requires a minimum value of one, for databases in the elastic resource pool and minimum value of two, otherwise. Required when using the `computeModel` parameter. When using `cpuCoreCount` parameter, it is an error to specify computeCount to a non-null value. Providing `computeModel` and `computeCount` is the preferred method for both OCPU and ECPU. | Conditional |
    | `spec.clone.ocpuCount` | float32 | The number of OCPU cores to be made available to the database.<br>The following points apply:<br> - For Autonomous Databases on Dedicated Exadata infrastructure, to provision less than 1 core, enter a fractional value in an increment of 0.1. For example, you can provision 0.3 or 0.4 cores, but not 0.35 cores. (Note that fractional OCPU values are not supported for Autonomous Database Serverless instances.)<br> - To provision 1 or more cores, you must enter an integer between 1 and the maximum number of cores available for the infrastructure shape. For example, you can provision 2 cores or 3 cores, but not 2.5 cores. This applies to an Autonomous Database Serverless instance or an Autonomous Database on Dedicated Exadata Infrastructure.<br> - For Autonomous Database Serverless instances, this parameter is not used.<br> For Autonomous Databases on Dedicated Exadata infrastructure, the maximum number of cores is determined by the infrastructure shape. See [Characteristics of Infrastructure Shapes](https://www.oracle.com/pls/topic/lookup?ctx=en/cloud/paas/autonomous-database&id=ATPFG-GUID-B0F033C1-CC5A-42F0-B2E7-3CECFEDA1FD1) for shape details.<br> **Note:** This parameter cannot be used with the `cpuCoreCount` parameter. | Conditional |
    | `spec.clone.adminPassword` | dictionary | The password for the ADMIN user. The password must be between 12 and 30 characters long, and must contain at least 1 uppercase, 1 lowercase, and 1 numeric character. It cannot contain the double quote symbol (") or the username "admin", regardless of casing.<br><br> Either `k8sSecret.name` or `ociSecret.id` must be provided. If both `k8sSecret.name` and `ociSecret.id` appear, the Operator reads the password from the K8s secret that `k8sSecret.name` refers to. | Yes |
    | `spec.clone.adminPassword.k8sSecret.name` | string | The **name** of the K8s Secret where you want to hold the password for the ADMIN user. | Conditional |
    |`spec.clone.adminPassword.ociSecret.id` | string | The **[OCID](https://docs.cloud.oracle.com/Content/General/Concepts/identifiers.htm)** of the [OCI Secret](https://docs.oracle.com/en-us/iaas/Content/KeyManagement/Tasks/managingsecrets.htm) where you want to hold the password for the ADMIN user. | Conditional |
    | `spec.clone.dataStorageSizeInTBs`  | int | The size, in terabytes, of the data volume that will be created and attached to the database. This storage can later be scaled up if needed. | Yes |
    | `spec.clone.isAutoScalingEnabled`  | boolean | Indicates if auto scaling is enabled for the Autonomous Database OCPU core count. The default value is `FALSE` | No |
    | `spec.clone.isDedicated` | boolean | True if the database is on dedicated [Exadata infrastructure](https://docs.cloud.oracle.com/Content/Database/Concepts/adbddoverview.htm). `spec.clone.autonomousContainerDatabase.k8sACD.name` or `spec.clone.autonomousContainerDatabase.ociACD.id` has to be provided if the value is true. | No |
    | `spec.clone.isFreeTier` | boolean | Indicates if this is an Always Free resource. The default value is false. Note that Always Free Autonomous Databases have 1 CPU and 20GB of memory. For Always Free databases, memory and CPU cannot be scaled.<br>This cannot be updated in parallel with any of the following: licenseModel, cpuCoreCount, computeCount, computeModel, adminPassword, whitelistedIps, isMtlsConnectionRequired, privateEndpointLabel, nsgIds, dbVersion, or dbName. | No |
    | `spec.clone.isAccessControlEnabled` | boolean | Indicates if the database-level access control is enabled.<br>If disabled, database access is defined by the network security rules.<br>If enabled, database access is restricted to the IP addresses defined by the rules specified with the `whitelistedIps` property. While specifying `whitelistedIps` rules is optional, if database-level access control is enabled and no rules are specified, the database will become inaccessible.<br>When creating a database clone, the desired access control setting should be specified. By default, database-level access control will be disabled for the clone.<br>This property is applicable only to Autonomous Databases on the Exadata Cloud@Customer platform. For Autonomous Database Serverless instances, `whitelistedIps` is used. | No |
    | `spec.clone.whitelistedIps` | []string | The client IP access control list (ACL). This feature is available for [Autonomous Database Serverless](https://docs.oracle.com/en/cloud/paas/autonomous-database/index.html) and on Exadata Cloud@Customer.<br>Only clients connecting from an IP address included in the ACL may access the Autonomous Database instance.<br>If `arePrimaryWhitelistedIpsUsed` is 'TRUE' then Autonomous Database uses this primary's IP access control list (ACL) for the disaster recovery peer called `standbywhitelistedips`.<br>For Autonomous Database Serverless, this is an array of CIDR (classless inter-domain routing) notations for a subnet or VCN OCID (virtual cloud network Oracle Cloud ID).<br>Multiple IPs and VCN OCIDs should be separate strings separated by commas, but if it’s other configurations that need multiple pieces of information then its each piece is connected with semicolon (;) as a delimiter.<br>Example: `["1.1.1.1","1.1.1.0/24","ocid1.vcn.oc1.sea.<unique_id>","ocid1.vcn.oc1.sea.<unique_id1>;1.1.1.1","ocid1.vcn.oc1.sea.<unique_id2>;1.1.0.0/16"]`<br>For Exadata Cloud@Customer, this is an array of IP addresses or CIDR notations.<br>Example: `["1.1.1.1","1.1.1.0/24","1.1.2.25"]`<br>For an update operation, if you want to delete all the IPs in the ACL, use an array with a single empty string entry.<br>This cannot be updated in parallel with any of the following: licenseModel, cpuCoreCount, computeCount, computeModel, adminPassword, isMtlsConnectionRequired, dbWorkload, dbVersion, dbName, or isFreeTier. | No |
    | `spec.clone.subnetId` | string | The [OCID](https://docs.cloud.oracle.com/Content/General/Concepts/identifiers.htm) of the subnet the resource is associated with.<br> **Subnet Restrictions:**<br> - For Autonomous Database, setting this will disable public secure access to the database.<br> These subnets are used by the Oracle Clusterware private interconnect on the database instance.<br> Specifying an overlapping subnet will cause the private interconnect to malfunction.<br> This restriction applies to both the client subnet and the backup subnet. | No |
    | `spec.clone.nsgIds` | []string | The list of [OCIDs](https://docs.cloud.oracle.com/Content/General/Concepts/identifiers.htm) for the network security groups (NSGs) to which this resource belongs. Setting this to an empty list removes all resources from all NSGs. For more information about NSGs, see [Security Rules](https://docs.cloud.oracle.com/Content/Network/Concepts/securityrules.htm).<br> **NsgIds restrictions:**<br> - A network security group (NSG) is optional for Autonomous Databases with private access. The nsgIds list can be empty. | No |
    | `spec.clone.privateEndpointLabel` | string | The resource's private endpoint label.<br> - Setting the endpoint label to a non-empty string creates a private endpoint database.<br> - Resetting the endpoint label to an empty string, after the creation of the private endpoint database, changes the private endpoint database to a public endpoint database.<br> - Setting the endpoint label to a non-empty string value, updates to a new private endpoint database, when the database is disabled and re-enabled.<br> This setting cannot be updated in parallel with any of the following: licenseModel, cpuCoreCount, computeCount, computeModel, adminPassword, whitelistedIps, isMTLSConnectionRequired, dbWorkload, dbVersion, dbName, or isFreeTier. | No |
    | `spec.clone.isMtlsConnectionRequired` | boolean | Specifies if the Autonomous Database requires mTLS connections. | No |
    | `spec.clone.autonomousContainerDatabase.k8sACD.name` | string | The **name** of the K8s Autonomous Container Database resource | No |
    | `spec.clone.autonomousContainerDatabase.ociACD.id` | string | The Autonomous Container Database [OCID](https://docs.cloud.oracle.com/Content/General/Concepts/identifiers.htm). | No |
    | `spec.clone.freeformTags` | dictionary | Free-form tags for this resource. Each tag is a simple key-value pair with no predefined name, type, or namespace. For more information, see [Resource Tag](https://docs.cloud.oracle.com/Content/General/Concepts/resourcetags.htm).<br><br> Example:<br> `freeformTags:`<br> &nbsp;&nbsp;&nbsp;&nbsp;`key1: value1`<br> &nbsp;&nbsp;&nbsp;&nbsp;`key2: value2`| No |
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
      action: Clone
      details:
        id: ocid1.autonomousdatabase...
      clone:
        compartmentId: ocid1.compartment... OR ocid1.tenancy...
        dbName: ClonedADB
        displayName: ClonedADB
        computeModel: ECPU
        computeCount: 1
        adminPassword:
          k8sSecret:
            name: admin-password
        dataStorageSizeInTBs: 1
        dbWorkload: OLTP
        cloneType: METADATA
      ociConfig:
        configMapName: oci-cred
        secretName: oci-privatekey
    ```

2. Apply the yaml

    ```sh
    kubectl apply -f config/samples/adb/autonomousdatabase_clone.yaml
    autonomousdatabase.database.oracle.com/autonomousdatabase-sample configured
    ```

Now, you can verify that a cloned database with name "ClonedADB" is being provisioned on the Cloud Console.

## Switchover an existing Autonomous Database

> Note: this operation requires an `AutonomousDatabase` object to be in your cluster. This example assumes the provision operation or the bind operation has been done by the users and the operator is authorized with API Key Authentication.

To switchover an existing Autonomous Database, complete these steps:

1. Add the following fields to the AutonomousDatabase resource definition. An example YAML file is available here: [config/samples/adb/autonomousdatabase_switchover.yaml](./../../config/samples/adb/autonomousdatabase_switchover.yaml)
    | Attribute | Type | Description | Required? |
    |----|----|----|----|
    | `spec.details.id` | string | The [OCID](https://docs.cloud.oracle.com/Content/General/Concepts/identifiers.htm) of the source Autonomous Database that you will clone to create a new Autonomous Database. | Yes |
    | `spec.ociConfig` | dictionary | Not required when the Operator is authorized with [Instance Principal](./ADB_PREREQUISITES.md#authorized-with-instance-principal). Otherwise, you will need the values from the [Authorized with API Key Authentication](./ADB_PREREQUISITES.md#authorized-with-api-key-authentication) section. | Conditional |
    | `spec.ociConfig.configMapName` | string | Name of the ConfigMap that holds the local OCI configuration | Conditional |
    | `spec.ociConfig.secretName`| string | Name of the K8s Secret that holds the private key value | Conditional |

    ```yaml
    ---
    apiVersion: database.oracle.com/v4
    kind: AutonomousDatabase
    metadata:
      name: autonomousdatabase-sample
    spec:
      action: Switchover
      details:
        id: ocid1.autonomousdatabase...
      ociConfig:
        configMapName: oci-cred
        secretName: oci-privatekey
    ```

2. Apply the yaml

    ```sh
    kubectl apply -f config/samples/adb/autonomousdatabase_switchover.yaml
    autonomousdatabase.database.oracle.com/autonomousdatabase-sample configured
    ```

## Manually failover an existing Autonomous Database

> Note: this operation requires an `AutonomousDatabase` object to be in your cluster. This example assumes the provision operation or the bind operation has been done by the users and the operator is authorized with API Key Authentication.

To manually failover an existing Autonomous Database, complete these steps:

1. Add the following fields to the AutonomousDatabase resource definition. An example YAML file is available here: [config/samples/adb/autonomousdatabase_manual_failover.yaml](./../../config/samples/adb/autonomousdatabase_manual_failover.yaml)
    | Attribute | Type | Description | Required? |
    |----|----|----|----|
    | `spec.details.id` | string | The [OCID](https://docs.cloud.oracle.com/Content/General/Concepts/identifiers.htm) of the source Autonomous Database that you will clone to create a new Autonomous Database. | Yes |
    | `spec.ociConfig` | dictionary | Not required when the Operator is authorized with [Instance Principal](./ADB_PREREQUISITES.md#authorized-with-instance-principal). Otherwise, you will need the values from the [Authorized with API Key Authentication](./ADB_PREREQUISITES.md#authorized-with-api-key-authentication) section. | Conditional |
    | `spec.ociConfig.configMapName` | string | Name of the ConfigMap that holds the local OCI configuration | Conditional |
    | `spec.ociConfig.secretName`| string | Name of the K8s Secret that holds the private key value | Conditional |

    ```yaml
    ---
    apiVersion: database.oracle.com/v4
    kind: AutonomousDatabase
    metadata:
      name: autonomousdatabase-sample
    spec:
      action: Failover
      details:
        id: ocid1.autonomousdatabase...
      ociConfig:
        configMapName: oci-cred
        secretName: oci-privatekey
    ```

2. Apply the yaml

    ```sh
    kubectl apply -f config/samples/adb/autonomousdatabase_failover.yaml
    autonomousdatabase.database.oracle.com/autonomousdatabase-sample configured
    ```

## Roles and Privileges requirements for Oracle Autonomous Database Controller

Autonomous Database controller uses Kubernetes objects such as:

  | Resources | Verbs |
  | --- | --- |
  | Configmaps | get list watch create update patch delete | 
  | Secrets | get list watch create update patch delete | 
  | Events | create patch |

The defintion of all the Kubernetes Objects, which are to be used by the Oracle Autonomous Database Controller, comes from the `oracle-database-operator.yaml` file which is applied to deploy the **Oracle Database Operator**.

## OpenShift Support

The Autonomous Database (ADB) Controller has been tested on OpenShift clusters and verified to work with the following use cases:

* **Create** – Provision a new Autonomous Database
* **Sync (Binding)** – Synchronize with existing Autonomous Database resources
* **Update** – Apply configuration changes
* **Stop** – Stop an Autonomous Database instance
* **Start** – Start an Autonomous Database instance
* **Terminate** – Delete an Autonomous Database instance
* **Clone** – Create a clone from an existing Autonomous Database

## Debugging and troubleshooting

### Show the details of the resource

If you edit and reapply the `.yaml` file, then the Autonomous Database controller will only update the parameters that the file contains. The parameters that are not in the file will not be updated. To obtain the verbose output of the current spec, use the following command:

```sh
kubectl describe adb/autonomousdatabase-sample
```

If any error occurs during the reconciliation loop, then the operator reports the error using the resource's event stream, which shows up in kubectl describe output.

### Check the logs of the pod where the operator deploys

To check the logs, use these steps:

1. List the pod replicas

    ```sh
    kubectl get pods -n oracle-database-operator-system
    ```

2. Use the following command to check the logs of the Pod that has a failure

    ```sh
    kubectl logs -f pod/oracle-database-operator-controller-manager-78666fdddb-s4xcm -n oracle-database-operator-system
    ```

## Known Issues

### Failed to validate Wallet: "read-only file system"

In some environments, e.g. OKE using the Operator add-on, the operator fails to validate the wallet due to encountering a **read-only file system** error. This prevents successful wallet validation and can disrupt operator functionality.

For example, logs from the controller pod may show:

```text
"error": "Failed to validate Wallet: open /tmp/wallet1208873634.zip: read-only file system"
```

#### Workaround

* Ensure wallet directories and mounted volumes have correct **read-write** permissions.
* Confirm that file system mounts used by the operator are writable.

#### Reference

See GitHub issue **#193** in the [oracle-database-operator repository](https://github.com/oracle/oracle-database-operator/issues/193) for details and steps to work around.
