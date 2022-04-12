# Backing up an Autonomous Database Manually

This document describes how to create manual backups of Autonomous Databases.

Oracle Cloud Infrastructure automatically backs up your Autonomous Databases and retains these backups for 60 days. You can restore and recover your database to any point-in-time in this retention period. Automatic backups are full backups taken every 60 days and daily incremental backups. You can also create manual backups to supplement your automatic backups. Manual backups are stored in an Object Storage bucket that you create, and are retained for 60 days. For more information, please visit [this page](https://docs.oracle.com/en-us/iaas/Content/Database/Tasks/adbbackingup.htm).

The operator gets the list of the `AutonomousBackupOCIDs` from OCI in every reconciliation loop, and creates the `AutonomousDatabaseBackup` resource automatically if the resource with the same `AutonomousBackupOCID` doesn't exist in the cluster.

## Prerequisites

You must create an Oracle Cloud Infrastructure Object Storage bucket to hold your Autonomous Database manual backups and configure your database to connect to it. Please follow the steps [in this page](https://docs.oracle.com/en-us/iaas/Content/Database/Tasks/adbbackingup.htm#creatingbucket) to finish the setup. This is a one-time operation.

## Create Manual Backup

Follow the steps to back up an Autonomous Database.

1. Add the following fields to the AutonomousDatabaseBackup resource definition. An example `.yaml` file is available here: [`config/samples/adb/autonomousdatabase_backup.yaml`](./../../config/samples/adb/autonomousdatabase_backup.yaml)
    | Attribute | Type | Description | Required? |
    |----|----|----|----|
    | `spec.target.k8sADB.name` | string | The name of custom resource of the target AutonomousDatabase. Choose either the `spec.target.k8sADB.name` or the `spec.target.ociADB.ocid`, but not both. | Conditional |
    | `spec.target.ociADB.ocid` | string | The [OCID](https://docs.cloud.oracle.com/Content/General/Concepts/identifiers.htm) of the target AutonomousDatabase. Choose either the `spec.target.k8sADB.name` or the `spec.target.ociADB.ocid`, but not both. | Conditional |
    | `spec.displayName` | string | The user-friendly name for the backup. The name does not have to be unique. | Yes |
    | `spec.ociConfig` | dictionary | Not required when the Operator is authorized with [Instance Principal](./ADB_PREREQUISITES.md#authorized-with-instance-principal). Otherwise, you will need the values from the [Authorized with API Key Authentication](./ADB_PREREQUISITES.md#authorized-with-api-key-authentication) section. | Conditional |
    | `spec.ociConfig.configMapName` | string | Name of the ConfigMap that holds the local OCI configuration | Conditional |
    | `spec.ociConfig.secretName`| string | Name of the K8s Secret that holds the private key value | Conditional |

    ```yaml
    ---
    apiVersion: database.oracle.com/v1alpha1
    kind: AutonomousDatabaseBackup
    metadata:
      name: autonomousdatabasebackup-sample
    spec:
      target:
        k8sADB:
          name: autonomousdatabase-sample
        # # Uncomment the below block if you use ADB OCID as the input of the target ADB
        # ociADB:
        #   ocid: ocid1.autonomousdatabase...
      displayName: autonomousdatabasebackup-sample
      ociConfig:
        configMapName: oci-cred
        secretName: oci-privatekey
    ```

2. Apply the yaml:

    ```sh
    kubectl apply -f config/samples/adb/autonomousdatabase_backup.yaml
    autonomousdatabasebackup.database.oracle.com/autonomousdatabasebackup-sample created
    ```
