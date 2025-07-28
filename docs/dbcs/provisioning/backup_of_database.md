# Create Backup of Existing Database of DB System in OCI OBDS Service

In this use case, an existing OCI OBDS system deployed earlier with existing Database is going to have full manual backup in OCI Base OBDS Service using existing Compartment ID and  DB System Id. 

As an pre-requisite, get the details of OCID of database of an existing OBDS System which you want to backup.  

**NOTE:** We are assuming that before this step, you have followed the [prerequisite](./../README.md#prerequisites-to-deploy-a-dbcs-system-using-oracle-db-operator-dbcs-controller) steps to create the configmap and the secrets required during the deployment.

This example uses `backup_of_database.yaml` to clone a Single Instance OBDS VMDB using Oracle DB Operator OBDS Controller with:
- OCID of existing as DB System as `id`
- OCI Configmap as `oci-cred`  
- OCI Secret as `oci-privatekey`
- enableBackup: as `true` 
- Specification of backup prefix as - Details of `backupDisplayName`. This is optional field. If it is not provided, controller will use keyword `backup` as prefix. In both cases of displayName whether its provided or not, suffix of backup name is timestamp to have uniqueness of backup name of database created.
**NOTE:** For the details of the parameters to be used in the .yaml file, please refer [here](./dbcs_controller_parameters.md).

Use the file: [backup_of_database.yaml](./backup_of_database.yaml) for this use case as below:

1. Deploy the .yaml file:  
```sh
[root@docker-test-server OBDS]# kubectl apply -f backup_of_database.yaml
dbcssystem.database.oracle.com/dbcssystem-backup configured
```

2. Monitor the Oracle DB Operator Pod `pod/oracle-database-operator-controller-manager-665874bd57-g2cgw` for the progress of the OBDS VMDB creation of PDBs. 

NOTE: Check the DB Operator Pod name in your environment.

```
[root@docker-test-server OBDS]# kubectl logs -f pod/oracle-database-operator-controller-manager-665874bd57-g2cgw -n  oracle-database-operator-system
```

## Sample Output

[Here](./backup_database_sample_output.log) is the sample output for backup an existing OBDS System deployed in OCI using Oracle DB Operator OBDS Controller.