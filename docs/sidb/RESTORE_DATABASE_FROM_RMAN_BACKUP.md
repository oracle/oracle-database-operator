# Restore a Database from an RMAN Backup

## Introduction

Use the Single Instance Database (SIDB) controller in Oracle Database Operator to provision a primary database by restoring an existing RMAN backup. The backup can be read from either:

- An object storage service (for example, an `Oracle Cloud Infrastructure (OCI) Object Storage bucket`)
- A file system mounted in the SIDB pod (for example, `OCI File Storage (FSS)`)

The restore flow supports backups of both `non-CDB` and `CDB` source databases.

## Supported Restore Scenarios

| Source Database Type | Backup Location | Target Database Type |
| --- | --- | --- |
| Non-CDB | Object Storage | Non-CDB |
| Non-CDB | Mounted File System | Non-CDB |
| CDB | Object Storage | CDB |
| CDB | Mounted File System | CDB |

## Before You Begin

Before restoring a database from an RMAN backup, ensure that the following prerequisites have been completed.

### Environment

- Oracle Database Operator is deployed, and the `SingleInstanceDatabase` custom resource is available.
- `kubectl` is configured for the target Kubernetes cluster.
- The database container image supports the restore operation.
- An appropriate storage class is available for `spec.persistence.oradata`.

### Source Database

- The source database is running in `ARCHIVELOG` mode.
- Transparent Data Encryption (TDE) is enabled on the source database.
- The source database DBID is available.
- The source database TDE wallet files have been collected and packaged into a compressed archive (for example, `tde_wallet_<timestamp>.tgz`) for use during the restore.

  **Note:** The restore operation expects the source database TDE wallet to be provided as a compressed archive containing the `tde` directory. The archive must preserve the `tde` directory and all wallet files within it, such as `ewallet.p12` and any versioned wallet files (for example, `ewallet_<timestamp>.p12`).

### RMAN Backup

- All RMAN backup pieces required to restore and recover the source database are available.
- The RMAN backup includes the database, archived redo logs, and any required metadata for recovery.
- RMAN control file autobackup is enabled to ensure that the control file and SPFILE can be restored if required.
- If the RMAN backup is encrypted, the corresponding RMAN decryption password and source database TDE wallet are available.

### Backup Storage

Depending on the restore method, one of the following storage options must be available.

**Object Storage**

- The RMAN backup and all associated backup files are available in an object storage bucket.

**Mounted File System**

- The RMAN backup and all associated backup files are available on the mounted file system.
- The mounted file system is accessible from the target Kubernetes cluster through a Kubernetes `PersistentVolume` and `PersistentVolumeClaim`.
- If the RMAN backup is transferred to the mounted file system as a compressed archive, it has been extracted before starting the restore.
- The extracted backup files have the appropriate ownership and permissions to be accessed by the SIDB pod.

### Namespace

The examples in this document use the `sidb` namespace.

```sh
export NS=sidb
kubectl create namespace "$NS" --dry-run=client -o yaml | kubectl apply -f -
```

> **Security:** Do not commit passwords, private keys, wallet files, or other secret values to source control. Create the Kubernetes Secrets from secure local files, and remove the local files when they are no longer required.

## Prepare Secrets and ConfigMaps

Run the commands in this section from a machine that has `kubectl` access to the cluster.

### Create Password Files

Create the files required by the restore. Use a secure method to provide each value. The following example avoids placing the passwords directly in the command history:

```sh
read -rsp "Source database administrator password: " SIDB_ADMIN_PASSWORD; echo
printf '%s' "$SIDB_ADMIN_PASSWORD" > sidb-admin-password
unset SIDB_ADMIN_PASSWORD

read -rsp "Source database wallet password: " SOURCE_WALLET_PASSWORD; echo
printf '%s' "$SOURCE_WALLET_PASSWORD" > source-wallet-password
unset SOURCE_WALLET_PASSWORD
```

For an encrypted RMAN backup, also create the decrypt-password file:

```sh
read -rsp "RMAN backup decrypt password: " RMAN_DECRYPT_PASSWORD; echo
printf '%s' "$RMAN_DECRYPT_PASSWORD" > rman-decrypt-password
unset RMAN_DECRYPT_PASSWORD
```

### Create Kubernetes Secrets

Create or update the database administrator password Secret:

```sh
kubectl create secret generic sidb-admin-secret \
  --from-file=oracle_pwd=./sidb-admin-password \
  -n "$NS" \
  --dry-run=client -o yaml | kubectl apply -f -
```

Create or update the source database wallet password Secret:

```sh
kubectl create secret generic source-db-wallet-password \
  --from-file=sourcedb_wallet_pwd=./source-wallet-password \
  -n "$NS" \
  --dry-run=client -o yaml | kubectl apply -f -
```

Create or update the source TDE wallet Secret. Replace the wallet archive path with the archive for the source database being restored:

```sh
kubectl create secret generic source-db-tde-wallet \
  --from-file=sourcedb_tde_wallet_files=./source-tde-wallet.tgz \
  -n "$NS" \
  --dry-run=client -o yaml | kubectl apply -f -
```

For encrypted RMAN backups, create or update the decrypt-password Secret:

```sh
kubectl create secret generic rman-decrypt-password \
  --from-file=decrypt_pwd=./rman-decrypt-password \
  -n "$NS" \
  --dry-run=client -o yaml | kubectl apply -f -
```

### Prepare OCI Object Storage Access

This subsection is required only for an OCI Object Storage restore.

Create a Kubernetes Secret containing the OCI API signing private key (`oci_api_key.pem`) for the OCI user configured in the OCI configuration file.

```sh
kubectl create secret generic sidb-oci-privatekey \
  --type=kubernetes.io/ssh-auth \
  --from-file=ssh-privatekey=./oci_api_key.pem \
  -n "$NS" \
  --dry-run=client -o yaml | kubectl apply -f -
```

Create an OCI configuration file:

```sh
cat > config.txt <<'EOF_CONFIG'
USER_OCID=<user-ocid>
FINGERPRINT=<api-key-fingerprint>
OPC_HOST=https://objectstorage.<region>.oraclecloud.com
TENANCY_OCID=<tenancy-ocid>
EOF_CONFIG
```

Create or update the ConfigMap containing the OCI configuration:

```sh
kubectl create configmap sidb-oci-config \
  --from-file=oci.env=./config.txt \
  -n "$NS" \
  --dry-run=client -o yaml | kubectl apply -f -
```

If the restore flow requires the OCI Backup Module installer, download the [Oracle Database Cloud Backup Module](https://docs.oracle.com/en/cloud/paas/db-backup-cloud/mgdbb/downloading-and-installing-oracle-database-cloud-backup-module-oci.html), extract the downloaded `opc_installer.zip`, and create a ZIP archive containing the `oci_installer` directory.

For example:

```sh
unzip opc_installer.zip
cd opc_installer
zip -r oci_installer.zip ./oci_installer
```

Create the ConfigMap referenced by `restore.objectStore.opcInstallerZip` using the generated `oci_installer.zip` file:

```sh
kubectl create configmap opc-installer-zipfile \
  --from-file=opc_installer_zipfile=./oci_installer.zip \
  -n "$NS" \
  --dry-run=client -o yaml | kubectl apply -f -
```

**Note:** The downloaded `opc_installer.zip` from Oracle Technology Network contains both the `oci_installer` and `opc_installer` directories. The Oracle Database Operator expects a ZIP archive containing only the `oci_installer` directory when creating the `opc-installer-zipfile` ConfigMap.

### Prepare OCI File Storage Access

This subsection is required only for an FSS restore.

Ensure that a PVC containing the backup exists in the target namespace. The common manifest uses the following example values:

```yaml
additionalPVCs:
  - mountPath: /FileSystem_SIDB_BACKUP
    pvcName: pvc-sidb-deployment
```

The value of `restore.fileSystem.backupPath` must point to the backup directory under the mounted path.

#### Prepare the Backup on the Mounted File System

If the RMAN backup is available as a compressed archive, extract it on the mounted file system before starting the restore.

For example:

```sh
# On the source host
cd /u01
tar -cvzf sidb_backup.tar.gz sidb_backup

# Copy sidb_backup.tar.gz to the mounted file system

# On the mounted file system
cd /FileSystem_SIDB_BACKUP
tar -xvzf sidb_backup.tar.gz

# Set the ownership expected by the SIDB container
chown -R 54321:54321 sidb_backup
```

## Configure the Restore Manifest

Use [`singleinstancedatabase_restore_from_backup_template.yaml`](../../config/samples/sidb/singleinstancedatabase_restore_from_backup_template.yaml) as the common manifest for all four restore scenarios.

Before applying the manifest:

1. Set `metadata.name`, `metadata.namespace`, `spec.sid`, and `spec.image.pullFrom`.
2. For a CDB restore, uncomment `spec.pdbName` and set the target PDB name. Leave it commented for a non-CDB restore.
3. Set `restore.options.sourceDbName` to the database name recorded in the source backup.
4. Set `restore.target.walletRoot` using the target SID.
5. Select exactly one backup source:
   - Keep `restore.objectStore` for an OCI Object Storage restore and leave `restore.fileSystem` commented.
   - Comment out `restore.objectStore`, uncomment `restore.fileSystem`, and enable the FSS PVC settings for a file-system restore.
6. Set the source DBID:
   - Use `restore.objectStore.backupIdentity.dbid` for an Object Storage restore.
   - Use `restore.objectStore.backupIdentity.bucketName` for the Object Storage restore bucket having the backup.
   - Use `restore.objectStore.backupIdentity.compartmentOcid` for an Object Storage bucket compartment OCID.
7. Set the source wallet and password Secret references.

### Scenario-Specific Samples

| Restore Scenario | Sample YAML |
| --- | --- |
| Non-CDB from Object Storage | [`singleinstancedatabase_restore_noncdb_objectstore.yaml`](../../config/samples/sidb/singleinstancedatabase_restore_noncdb_objectstore.yaml) |
| Non-CDB from Mounted File System | [`singleinstancedatabase_restore_noncdb_filesystem.yaml`](../../config/samples/sidb/singleinstancedatabase_restore_noncdb_filesystem.yaml) |
| CDB from Object Storage | [`singleinstancedatabase_restore_cdb_objectstore.yaml`](../../config/samples/sidb/singleinstancedatabase_restore_cdb_objectstore.yaml) |
| CDB from Mounted File System | [`singleinstancedatabase_restore_cdb_filesystem.yaml`](../../config/samples/sidb/singleinstancedatabase_restore_cdb_filesystem.yaml) |

## Deploy the SIDB Resource

Apply the completed manifest:

```sh
kubectl apply -f singleinstancedatabase_restore_from_backup_template.yaml
```

Monitor the SIDB resource and pod:

```sh
kubectl get singleinstancedatabase -n "$NS" -w
```

In another terminal:

```sh
kubectl get pods -n "$NS" -w
```

Set the pod name and follow the logs:

```sh
POD=$(kubectl get pods -n "$NS" \
  -l app=sidb-restore-db \
  -o jsonpath='{.items[0].metadata.name}')

kubectl logs -f "$POD" -n "$NS"
```

Replace `sidb-restore-db` in the label selector if `metadata.name` uses a different value.

Wait until the pod reports `1/1 Running` and the `SingleInstanceDatabase` resource reports a healthy status.

## Verify the Restored Database

Connect to the database container:

```sh
kubectl exec -it "$POD" -n "$NS" -- bash
```

Connect as SYSDBA:

```sh
sqlplus "/ as sysdba"
```

Verify the database name, open mode, and CDB type:

```sql
SELECT name, open_mode, cdb FROM v$database;
```

For a CDB restore, verify the PDB state:

```sql
SHOW PDBS;
```

Expected results:

- The database is open in `READ WRITE` mode.
- A non-CDB restore reports `CDB` as `NO`.
- A CDB restore reports `CDB` as `YES`, and the target PDB is open in `READ WRITE` mode.

## Troubleshooting

- If the pod remains `Pending`, verify the storage class, PVC, access mode, node placement, and FSS mount configuration.
- If the pod remains `0/1 Running`, review the database container logs with `kubectl logs`.
- If the Object Storage installer ConfigMap fails with a size error, verify the size of `opc_installer.zip`; ConfigMaps are limited to 1 MiB.
- If RMAN cannot identify the backup, verify the source DBID, source database name, bucket or backup path, and backup accessibility.
- If wallet-related operations fail, verify the wallet archive, password Secret, Secret keys, and `restore.target.walletRoot`.
