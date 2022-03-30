# Managing Oracle Autonomous Container Databases on Dedicated Exadata Infrastructure

Before you use the Oracle Database Operator for Kubernetes (the operator), ensure your system meets all of the Oracle Autonomous Database (ADB) Prerequisites [ADB_PREREQUISITES](./../adb/ADB_PREREQUISITES.md).

To interact with OCI services, either the cluster has to be authorized using Principal Instance, or using the API Key Authentication by specifying the configMap and the secret under the `ociConfig` field.

## Required Permissions

The opeartor must be given the required type of access in a policy written by an administrator to manage the Autonomous Container Databases. See [Create an Autonomous Container Database](https://docs.oracle.com/en-us/iaas/autonomous-database/doc/create-acd.html) for the required policies.

The permission to view the workrequests is also required, so that the operator will update the resources when the work is done. See [Viewing Work Requests](https://docs.oracle.com/en-us/iaas/Content/ContEng/Tasks/contengviewingworkrequests.htm#contengviewingworkrequests) for sample work request policies.

## Supported Features

After the operator is deployed, choose either one of the following operations to create an `AutonomousContainerDatabase` custom resource for Oracle Autonomous Container Database in your cluster.

* [Provision](#provision-an-autonomous-container-database) an Autonomous Container Database
* [Bind](#bind-to-an-existing-autonomous-container-database) to an existing Autonomous Container Database

After you create the resource, you can use the operator to perform the following tasks:

* [Change the display name](#change-the-display-name) of an Autonomous Container Database
* [Restart/Terminate](#restartterminate) an Autonomous Container Database
* [Sync](#sync-the-resource-manually) of an Autonomous Container Database manually
* [Delete the resource](#delete-the-resource) from the cluster

## Provision an Autonomous Container Database

Follow the steps to provision an Autonomous Database that will map objects in your cluster.

1. Get the `Compartment OCID`.

    Login cloud console and click `Compartment`.

    ![compartment-1](/images/adb/compartment-1.png)

    Click on the compartment name where you want to create your database, and **copy** the `OCID` of the compartment.

    ![compartment-2](/images/adb/compartment-2.png)

2. Get the `AutonomousExadataVMCluster OCID`.

    Login cloud console. Go to `Autonomous Database`, and click the `Autonomous Exadata VM Cluster` under the Dedicated Infrastructure.

    ![aei-1](/images/adb/adb-id-1.png)

    Click on the name of the Autonomous Exadata VM Cluster, and copy the `OCID`.

    ![aei-2](/images/adb/aei-id-1.png)

    ![aei-3](/images/adb/aei-id-2.png)

3. Add the following fields to the AutonomousContainerDatabase resource definition. An example `.yaml` file is available here: [`config/samples/acd/autonomouscontainerdatabase_create.yaml`](./../../config/samples/acd/autonomouscontainerdatabase_create.yaml)
    | Attribute | Type | Description | Required? |
    |----|----|----|----|
    | `spec.compartmentOCID` | string | The [OCID](https://docs.cloud.oracle.com/Content/General/Concepts/identifiers.htm) of the compartment of the Autonomous Container Database. | Yes |
    | `spec.autonomousExadataVMClusterOCID` | string | The [OCID](https://docs.cloud.oracle.com/Content/General/Concepts/identifiers.htm) of the Autonomous Exadata Infrastructure. | Yes |
    | `spec.displayName` | string | The user-friendly name for the Autonomous Container Database. The name does not have to be unique. | Yes |
    | `spec.patchModel` | string | The Database Patch model preference. The following values are valid: RELEASE_UPDATES and RELEASE_UPDATE_REVISIONS. Currently, the Release Update Revision maintenance type is not a selectable option. | No |
    | `spec.freeformTags` | dictionary | Free-form tags for this resource. Each tag is a simple key-value pair with no predefined name, type, or namespace. For more information, see [Resource Tag](https://docs.cloud.oracle.com/Content/General/Concepts/resourcetags.htm).<br><br> Example:<br> `freeformTags:`<br> &nbsp;&nbsp;&nbsp;&nbsp;`key1: value1`<br> &nbsp;&nbsp;&nbsp;&nbsp;`key2: value2`| No |
    | `spec.ociConfig` | dictionary | Not required when the Operator is authorized with [Instance Principal](./../adb/ADB_PREREQUISITES.md#authorized-with-instance-principal). Otherwise, you will need the values from the [Authorized with API Key Authentication](./../adb/ADB_PREREQUISITES.md#authorized-with-api-key-authentication) section. | Conditional |
    | `spec.ociConfig.configMapName` | string | Name of the ConfigMap that holds the local OCI configuration | Conditional |
    | `spec.ociConfig.secretName`| string | Name of the K8s Secret that holds the private key value | Conditional |

    ```yaml
    ---
    apiVersion: database.oracle.com/v1alpha1
    kind: AutonomousContainerDatabase
    metadata:
      name: autonomouscontainerdatabase-sample
    spec:
      compartmentOCID: ocid1.compartment... OR ocid1.tenancy...
      autonomousExadataVMClusterOCID: ocid1.autonomousexainfrastructure...
      displayName: newACD
      ociConfig:
        configMapName: oci-cred
        secretName: oci-privatekey
    ```

4. Apply the yaml:

    ```sh
    kubectl apply -f config/samples/acd/autonomouscontainerdatabase_create.yaml
    autonomouscontainerdatabase.database.oracle.com/autonomouscontainerdatabase-sample created
    ```

## Bind to an existing Autonomous Container Database

Other than provisioning a container database, you can bind to an existing Autonomous Container Database in your cluster.

1. Clean up the resource you created in the earlier provision operation:

    ```sh
    kubectl delete adb/autonomouscontainerdatabase-sample
    autonomouscontainerdatabase.database.oracle.com/autonomouscontainerdatabase-sample deleted
    ```

2. Copy the `Autonomous Container Database OCID` from Cloud Console.

    ![acd-id-1](/images/adb/adb-id-1.png)

    ![acd-id-2](/images/adb/acd-id-1.png)

    ![acd-id-3](/images/adb/acd-id-2.png)

3. Add the following fields to the AutonomousContainerDatabase resource definition. An example `.yaml` file is available here: [`config/samples/acd/autonomouscontainerdatabase_bind.yaml`](./../../config/samples/acd/autonomouscontainerdatabase_bind.yaml)
    | Attribute | Type | Description | Required? |
    |----|----|----|----|
    | `spec.autonomousContainerDatabaseOCID` | string | The [OCID](https://docs.cloud.oracle.com/Content/General/Concepts/identifiers.htm) of the Autonomous Container Database you want to bind (create a reference) in your cluster. | Yes |
    | `spec.ociConfig` | dictionary | Not required when the Operator is authorized with [Instance Principal](./ADB_PREREQUISITES.md#authorized-with-instance-principal). Otherwise, you will need the values from the [Authorized with API Key Authentication](./ADB_PREREQUISITES.md#authorized-with-api-key-authentication) section. | Conditional |
    | `spec.ociConfig.configMapName` | string | Name of the ConfigMap that holds the local OCI configuration | Conditional |
    | `spec.ociConfig.secretName`| string | Name of the K8s Secret that holds the private key value | Conditional |

    ```yaml
    ---
    apiVersion: database.oracle.com/v1alpha1
    kind: AutonomousContainerDatabase
    metadata:
      name: autonomouscontainerdatabase-sample
    spec:
      autonomousContainerDatabaseOCID: ocid1.autonomouscontainerdatabase...
      ociConfig:
        configMapName: oci-cred
        secretName: oci-privatekey
    ```

4. Apply the yaml.

    ```sh
    kubectl apply -f config/samples/acd/autonomouscontainerdatabase_bind.yaml
    autonomouscontainerdatabase.database.oracle.com/autonomouscontainerdatabase-sample created
    ```

## Change the display name

> Note: this operation requires an `AutonomousContainerDatabase` object to be in your cluster. This example assumes the provision operation or the bind operation has been completed, and the operator is authorized with API Key Authentication.

You can change the display name of the database by modifying the value of the `displayName`, as follows:

1. An example YAML file is available here: [config/samples/acd/autonomouscontainerdatabase_change_displayname.yaml](./../../config/samples/acd/autonomouscontainerdatabase_change_displayname.yaml)

    ```yaml
    ---
    apiVersion: database.oracle.com/v1alpha1
    kind: AutonomousContainerDatabase
    metadata:
      name: autonomouscontainerdatabase-sample
    spec:
      compartmentOCID: ocid1.compartment... OR ocid1.tenancy...
      displayName: RenamedADB
      ociConfig:
        configMapName: oci-cred
        secretName: oci-privatekey
    ```

    * `displayNameName`: User-friendly name of the Autonomous Container Database. The name does not have to be unique.

2. Apply the change using `kubectl`.

    ```sh
    kubectl apply -f config/samples/acd/autonomouscontainerdatabase_change_displayname.yaml
    autonomouscontainerdatabase.database.oracle.com/autonomouscontainerdatabase-sample configured
    ```

## Restart/Terminate

> Note: this operation requires an `AutonomousContainerDatabase` object to be in your cluster. This example assumes the provision operation or the bind operation has been done by the users and the operator is authorized with API Key Authentication.

Users can restart/terminate a database using the `action` attribute. The value will be erased after the change is applied.
Here's a list of the values you can set for `action`:

* `RESTART`: to restart the database
* `TERMINATE`: to terminate the database
* `SYNC`: to sync the database, will describe in the next section

1. A sample .yaml file is available here: [config/samples/acd/autonomouscontainerdatabase_restart_terminate.yaml](./../../config/samples/acd/autonomouscontainerdatabase_restart_terminate.yaml)

    ```yaml
    ---
    apiVersion: database.oracle.com/v1alpha1
    kind: AutonomousContainerDatabase
    metadata:
      name: autonomouscontainerdatabase-sample
    spec:
      autonomousContainerDatabaseOCID: ocid1.autonomouscontainerdatabase...
      # Change the action to "TERMINATE" to terminate the database
      action: RESTART
      ociConfig:
        configMapName: oci-cred
        secretName: oci-privatekey
    ```

2. Apply the change to restart the database.

    ```sh
    kubectl apply -f config/samples/acd/autonomouscontainerdatabase_restart_terminate.yaml
    autonomouscontainerdatabase.database.oracle.com/autonomouscontainerdatabase-sample configured
    ```

## Sync the resource manually

> Note: this operation requires an `AutonomousContainerDatabase` object to be in your cluster. This example assumes the provision operation or the bind operation has been done by the users and the operator is authorized with API Key Authentication.

Users can sync the resource manually by setting the value of the `action` attribute to SYNC. The Operator may not response immediately if it is still waiting for a work to finish.

1. A sample .yaml file is available here: [config/samples/acd/autonomouscontainerdatabase_sync.yaml](./../../config/samples/acd/autonomouscontainerdatabase_sync.yaml)

    ```yaml
    ---
    apiVersion: database.oracle.com/v1alpha1
    kind: AutonomousContainerDatabase
    metadata:
      name: autonomouscontainerdatabase-sample
    spec:
      autonomousContainerDatabaseOCID: ocid1.autonomouscontainerdatabase...
      action: SYNC
      ociConfig:
        configMapName: oci-cred
        secretName: oci-privatekey
    ```

2. Apply the change to sync the database.

    ```sh
    kubectl apply -f config/samples/acd/autonomouscontainerdatabase_sync.yaml
    autonomouscontainerdatabase.database.oracle.com/autonomouscontainerdatabase-sample configured
    ```

## Delete the resource

> Note: this operation requires an `AutonomousContainerDatabase` object to be in your cluster. This example assumes the provision operation or the bind operation has been done by the users and the operator is authorized with API Key Authentication.

The `hardLink` defines the behavior when the resource is deleted from the cluster. If the `hardLink` is set to true, the Operator terminates the Autonomous Container Database in OCI when the resource is removed; otherwise, the Autonomous Container Database remains unchanged. By default the value is `false` if it is not explicitly specified.

Follow the steps to delete the resource and terminate the Autonomous Container Database.

1. Use the example [autonomouscontainerdatabase_delete_resource.yaml](./../../config/samples/acd/autonomouscontainerdatabase_delete_resource.yaml) which sets the attribute `hardLink` to true.

    ```yaml
    ---
    apiVersion: database.oracle.com/v1alpha1
    kind: AutonomousContainerDatabase
    metadata:
      name: autonomouscontainerdatabase-sample
    spec:
      autonomousContainerDatabaseOCID: ocid1.autonomouscontainerdatabase...
      hardLink: true
      ociConfig:
        configMapName: oci-cred
        secretName: oci-privatekey
    ```

2. Apply the yaml

    ```sh
    kubectl apply -f config/samples/acd/autonomouscontainerdatabase_delete_resource.yaml
    autonomouscontainerdatabase.database.oracle.com/autonomouscontainerdatabase-sample configured
    ```

3. Delete the resource in your cluster

    ```sh
    kubectl delete acd/autonomouscontainerdatabase-sample
    autonomouscontainerdatabase.database.oracle.com/autonomouscontainerdatabase-sample deleted
    ```

Now, you can verify that the Autonomous Container Database is in TERMINATING state.
