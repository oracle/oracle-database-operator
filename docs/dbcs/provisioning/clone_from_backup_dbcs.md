# Clone DB System from Backup of Existing DB System in OCI Oracle Base Database System (OBDS)

In this use case, you can see how to clone an existing OCI OBDS system deployed earlier with the Backup.

To clone OBDS to an existing OBDS system using Backup, you must obtain the OCID details for the backup in OCI OBDS. 

**NOTE:** We are assuming that before this step, you have followed the [prerequisite](./../README.md#prerequisites-to-deploy-a-dbcs-system-using-oracle-db-operator-dbcs-controller) steps to create the configmap and the secrets required during the deployment.

This example uses `clone_dbcs_system_from_backup.yaml` to clone a Single Instance OBDS VMDB using Oracle DB Operator OBDS Controller with:

- OCI Configmap as `oci-cred`  
- OCI Secret as `oci-privatekey`
- setupDBCloning: as `true` 
- OCID of Backup DB as `dbBackupId` of existing OBDS system.
- Specification for DB Cloning as `dbClone`-> `dbAdminPasswordSecret`,`tdeWalletPasswordSecret`, `dbName`,`hostName`,`displayName`,`licenseModel`,`domain`,`sshPublicKeys`,`subnetId`, `initialDataStorageSizeInGB`
**NOTE:** For the details of the parameters to be used in the .yaml file, please refer [here](./dbcs_controller_parameters.md).

Use the file: [clone_dbcs_system_from_backup.yaml](./clone_dbcs_system_from_backup.yaml) for this use case, as described in the following steps:

1. Deploy the `.yaml` file:  
```sh
[root@docker-test-server OBDS]# kubectl apply -f clone_dbcs_system_from_backup.yaml
dbcssystem.database.oracle.com/dbcssystem-clone created
```

2. Monitor the Oracle DB Operator Pod `pod/oracle-database-operator-controller-manager-665874bd57-g2cgw` to track the progress of the OBDS VMDB creation of PDBs. 

NOTE: Check the DB Operator Pod name in your environment.

```
[root@docker-test-server OBDS]# kubectl logs -f pod/oracle-database-operator-controller-manager-665874bd57-g2cgw -n  oracle-database-operator-system
```

## Sample Output

[Here](./clone_dbcs_system_from_backup_sample_output.log) is an example output log for cloning an existing OBDS System deployed in OCI using Oracle DB Operator OBDS Controller.
