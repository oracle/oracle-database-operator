# Clone DB System from Backup of Existing DB System in OCI Base DBCS Service

In this use case, an existing OCI DBCS system deployed earlier with the Backup is going to be cloned.

In order to clone DBCS to an existing DBCS system using Backup, get the details of OCID of backup in OCI DBCS. 

**NOTE:** We are assuming that before this step, you have followed the [prerequisite](./../README.md#prerequisites-to-deploy-a-dbcs-system-using-oracle-db-operator-dbcs-controller) steps to create the configmap and the secrets required during the deployment.

This example uses `clone_dbcs_system_from_backup.yaml` to clone a Single Instance DBCS VMDB using Oracle DB Operator DBCS Controller with:

- OCI Configmap as `oci-cred`  
- OCI Secret as `oci-privatekey`
- setupDBCloning: as `true` 
- OCID of Backup DB as `dbBackupId` of existing DBCS system.
- Specification for DB Cloning as `dbClone`-> `dbAdminPaswordSecret`,`tdeWalletPasswordSecret`, `dbName`,`hostName`,`displayName`,`licenseModel`,`domain`,`sshPublicKeys`,`subnetId`, `initialDataStorageSizeInGB`
**NOTE:** For the details of the parameters to be used in the .yaml file, please refer [here](./dbcs_controller_parameters.md).

Use the file: [clone_dbcs_system_from_backup.yaml](./clone_dbcs_system_from_backup.yaml) for this use case as below:

1. Deploy the .yaml file:  
```sh
[root@docker-test-server DBCS]# kubectl apply -f clone_dbcs_system_from_backup.yaml
dbcssystem.database.oracle.com/dbcssystem-clone created
```

2. Monitor the Oracle DB Operator Pod `pod/oracle-database-operator-controller-manager-665874bd57-g2cgw` for the progress of the DBCS VMDB creation of PDBs. 

NOTE: Check the DB Operator Pod name in your environment.

```
[root@docker-test-server DBCS]# kubectl logs -f pod/oracle-database-operator-controller-manager-665874bd57-g2cgw -n  oracle-database-operator-system
```

## Sample Output

[Here](./clone_dbcs_system_from_backup_sample_output.log) is the sample output for cloning an existing DBCS System deployed in OCI using Oracle DB Operator DBCS Controller.
