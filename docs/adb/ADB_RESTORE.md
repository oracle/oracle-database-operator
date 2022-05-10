# Restoring an Oracle Autonomous Database Manually

To restore an Autonomous Database from a backup, use this document.

You can either use any existing manual or automatic backup to restore your database, or you can restore and recover your database to any point in time in the 60-day retention period of your automatic backups. For point-in-time restores, you specify a timestamp. Your Autonomous Database identifies which backup to use for the fastest restore.

## Restore an Autonomous Database

To restore an Autonomous Database from a backup, or by using point-in-time restore, complete this procedure.

1. Add the following fields to the AutonomousDatabaseBackup resource definition. An example `.yaml` file is available here: [`config/samples/adb/autonomousdatabase_restore.yaml`](./../../config/samples/adb/autonomousdatabase_restore.yaml)
    | Attribute | Type | Description | Required? |
    |----|----|----|----|
    | `spec.target.k8sADB.name` | string | The name of custom resource of the target Autonomous Database (`AutonomousDatabase`). Choose either the `spec.target.k8sADB.name` or the `spec.target.ociADB.ocid`, but not both. | Conditional |
    | `spec.target.ociADB.ocid` | string | The [OCID](https://docs.cloud.oracle.com/Content/General/Concepts/identifiers.htm) of the target `AutonomousDatabase`. Choose either the `spec.target.k8sADB.name` or the `spec.target.ociADB.ocid`, but not both. | Conditional |
    | `spec.source.k8sADBBackup.name` | string | The name of custom resource of the `AutonomousDatabaseBackup` that you want to restore from. Choose either the `spec.source.k8sADBBackup.name` or the `spec.source.pointInTime.timestamp`, but not both. | Conditional |
    | `spec.source.pointInTime.timestamp` | string | The timestamp to specify the point in time to which you want the database restored. Your Autonomous Database identifies which backup to use for the fastest restore. The timestamp must follow this format: YYYY-MM-DD HH:MM:SS GMT. Choose either the `spec.source.k8sADBBackup.name` or the `spec.source.pointInTime.timestamp`, but not both. | Conditional |
    | `spec.ociConfig` | dictionary | Not required when the Operator is authorized with [Instance Principal](./ADB_PREREQUISITES.md#authorized-with-instance-principal). Otherwise, you will need the values from this section: [Authorized with API Key Authentication](./ADB_PREREQUISITES.md#authorized-with-api-key-authentication). | Conditional |
    | `spec.ociConfig.configMapName` | string | Name of the `ConfigMap` that holds the local OCI configuration | Conditional |
    | `spec.ociConfig.secretName`| string | Name of the Kubernetes (K8s) Secret that holds the private key value | Conditional |

    ```yaml
    ---
    apiVersion: database.oracle.com/v1alpha1
    kind: AutonomousDatabaseRestore
    metadata:
      name: autonomousdatabaserestore-sample
    spec:
      target:
        k8sADB:
          name: autonomousdatabase-sample
        # # Uncomment the below block if you use ADB OCID as the input of the target ADB
        # ociADB:
        #   ocid: ocid1.autonomousdatabase...
      source:
        k8sADBBackup: 
          name: autonomousdatabasebackup-sample
        # # Uncomment the following field to perform point-in-time restore
        # pointInTime: 
        #   timestamp: 2021-12-23 11:03:13 UTC
      ociConfig:
        configMapName: oci-cred
        secretName: oci-privatekey
    ```

2. Apply the yaml:

    ```sh
    kubectl apply -f config/samples/adb/autonomousdatabase_restore.yaml
    autonomousdatabaserestore.database.oracle.com/autonomousdatabaserestore-sample created
    ```
