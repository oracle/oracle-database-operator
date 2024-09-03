# Creating Long-Term Backups of an Oracle Autonomous Database

To create long-term backups of Autonomous Databases, use this procedure.

Oracle Cloud Infrastructure (OCI) automatically backs up your Autonomous Databases, and retains these backups for 60 days. You can restore and recover your database to any point-in-time in this retention period. Automatic backups are full backups taken every 60 days, with daily incremental backups. You can also create long-term backups for your database with a retention period between 3 months and up to 10 years. For more information, please visit [Create Long-Term Backups on Autonomous Database](https://docs.oracle.com/en/cloud/paas/autonomous-database/serverless/adbsb/backup-long-term.html) and [Backup and Restore Notes](https://docs.oracle.com/en-us/iaas/autonomous-database-serverless/doc/backup-restore-notes.html).

## Create Long-Term Backup

To back up an Autonomous Database, complete this procedure.

1. Add the following fields to the AutonomousDatabaseBackup resource definition. An example `.yaml` file is available here: [`config/samples/adb/autonomousdatabase_backup.yaml`](./../../config/samples/adb/autonomousdatabase_backup.yaml)
    | Attribute | Type | Description | Required? |
    |----|----|----|----|
    | `spec.displayName` | string | The user-friendly name for the backup. This name does not have to be unique. | Yes |
    | `spec.isLongTermBackup` | boolean | Indicates whether the backup is long-term. | Yes |
    | `spec.retentionPeriodInDays` | string | Retention period, in days, for long-term backups. Minimum retention period is 90 days. | Yes |
    | `spec.target.k8sADB.name` | string | The name of custom resource of the target Autonomous Database. Choose either the `spec.target.k8sADB.name` or the `spec.target.ociADB.ocid`, but not both. | Conditional |
    | `spec.target.ociADB.ocid` | string | The [OCID](https://docs.cloud.oracle.com/Content/General/Concepts/identifiers.htm) of the target AutonomousDatabase. Choose either the `spec.target.k8sADB.name` or the `spec.target.ociADB.ocid`, but not both. | Conditional |
    | `spec.ociConfig` | dictionary | Not required when the Operator is authorized with [Instance Principal](./ADB_PREREQUISITES.md#authorized-with-instance-principal). Otherwise, you will need the values from this section: [Authorized with API Key Authentication](./ADB_PREREQUISITES.md#authorized-with-api-key-authentication). | Conditional |
    | `spec.ociConfig.configMapName` | string | Name of the ConfigMap that holds the local OCI configuration | Conditional |
    | `spec.ociConfig.secretName`| string | Name of the Kubernetes (K8s) Secret that holds the private key value | Conditional |

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
      isLongTermBackup: true
      retentionPeriodInDays: 90
      ociConfig:
        configMapName: oci-cred
        secretName: oci-privatekey
    ```

2. Apply the yaml:

    ```sh
    kubectl apply -f config/samples/adb/autonomousdatabase_backup.yaml
    autonomousdatabasebackup.database.oracle.com/autonomousdatabasebackup-sample created
    ```
