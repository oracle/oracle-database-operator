# Clone DB System from Existing Database of DB System in OCI Oracle Base Database System (OBDS)

In this use case, an existing OCI OBDS system deployed earlier with existing Database is going to be cloned in OCI Base OBDS Service using existing Database ID. 

As an prerequisite, obtain the details of OCID of database of an existing OBDS System that you want to clone.  

**NOTE:** We are assuming that before this step, you have followed the [prerequisite steps](./../README.md#prerequisites-to-deploy-a-dbcs-system-using-oracle-db-operator-dbcs-controller) to create the configmap and the secrets required during the deployment.

This example uses `clone_dbcs_system_from_database.yaml` to clone a Single Instance OBDS VMDB using Oracle DB Operator OBDS Controller with the following:
- OCID of existing as `databaseId`
- OCI Configmap as `oci-cred`  
- OCI Secret as `oci-privatekey`
- setupDBCloning: as `true` 
- Specification of dbClone as - Details of new DB system for cloning `dbAdminPasswordSecret`,`tdeWalletPasswordSecret`, `dbName`,`hostName`,`displayName`,`licenseModel`,`domain`,`sshPublicKeys`,`subnetId`, `initialDataStorageSizeInGB`
**NOTE:** For the details of the parameters to be used in the .yaml file, please refer [here](./dbcs_controller_parameters.md).

Use the file: [clone_dbcs_system_from_database.yaml](./clone_dbcs_system_from_database.yaml) for this use case as described in the following steps:

1. Deploy the .yaml file:  
```sh
[root@docker-test-server OBDS]# kubectl apply -f clone_dbcs_system_from_database.yaml
dbcssystem.database.oracle.com/dbcssystem-clone created
```

2. Monitor the Oracle DB Operator Pod `pod/oracle-database-operator-controller-manager-665874bd57-g2cgw` to follow the progress of the OBDS VMDB creation of PDBs. 

NOTE: Check the DB Operator Pod name in your environment.

```
[root@docker-test-server OBDS]# kubectl logs -f pod/oracle-database-operator-controller-manager-665874bd57-g2cgw -n  oracle-database-operator-system
```

## Sample Output

[This log file](./clone_dbcs_system_from_database_sample_output.log) is an example output log file for cloning an existing OBDS System deployed in OCI using Oracle DB Operator OBDS Controller.
