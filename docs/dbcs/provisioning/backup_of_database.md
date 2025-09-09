# Create Backup of Existing Database of DB System in OCI OBDS Service

In this example, an existing OCI OBDS system previously deployed with an existing Database is updated to have a full manual backup in the OCI Base OBDS Service using the existing Compartment ID and database system ID. 

To use this example on your system, before you begin, obtain the details of the database OCID for the existing OBDS System which you want to backup.  

**NOTE:** We are assuming that before this step, you have followed the [prerequisite](./../README.md#prerequisites-to-deploy-a-dbcs-system-using-oracle-db-operator-dbcs-controller) steps to create the configmap and the secrets required during the deployment.

This example uses `backup_of_database.yaml` to clone a Single Instance OBDS VMDB using Oracle DB Operator OBDS Controller with the following configuration:
- OCID of existing as DB System as `id`
- OCI Configmap as `oci-cred`  
- OCI Secret as `oci-privatekey`
- enableBackup: as `true` 
- Specification of backup prefix as - Details of `backupDisplayName`. This field is optional. If it is not provided, then the controller uses the keyword `backup` as the prefix. In both cases of displayName, whether you provide a backup display name or not, the suffix of the backup name is the timestamp. The timestamp ensure that the backup name created for the database is unique.
**NOTE:** For the details of the parameters to be used in the .yaml file, see [DBCS controller parameters](./dbcs_controller_parameters.md).

Use the file: [backup_of_database.yaml](./backup_of_database.yaml) for this use case. Example:

1. Deploy the .yaml file:  
```sh
[root@docker-test-server OBDS]# kubectl apply -f backup_of_database.yaml
dbcssystem.database.oracle.com/dbcssystem-backup configured
```

2. Monitor the Oracle Database Operator Pod `pod/oracle-database-operator-controller-manager-665874bd57-g2cgw` for the progress of the OBDS VMDB creation of PDBs. 

NOTE: Check the DB Operator Pod name in your environment.

```
[root@docker-test-server OBDS]# kubectl logs -f pod/oracle-database-operator-controller-manager-665874bd57-g2cgw -n  oracle-database-operator-system
```

## Sample Output

[Backup Database Sample Output Log](./backup_database_sample_output.log) is an example output for a backup an existing OBDS System deployed in OCI using the Oracle Database Operator OBDS Controller.