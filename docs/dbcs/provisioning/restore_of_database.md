# Restore from Backup of Existing Database of DB System in OCI OBDS Service

In this use case, an existing OCI OBDS system deployed earlier with existing backup of Database is going to have restore in OCI Base OBDS Service using existing DB System Id. 

As an pre-requisite, get the details of OCID of database of an existing OBDS System which you want to backup.  

**NOTE:** We are assuming that before this step, you have followed the [prerequisite](./../README.md#prerequisites-to-deploy-a-dbcs-system-using-oracle-db-operator-dbcs-controller) steps to create the configmap and the secrets required during the deployment.

This example uses `restore_of_database.yaml` to clone a Single Instance OBDS VMDB using Oracle DB Operator OBDS Controller with:
- OCID of existing as DB System as `id`
- OCI Configmap as `oci-cred`  
- OCI Secret as `oci-privatekey`
- Restore Configuration of taking restore from a backup. Provided one of `latest` , `scn` and `timestamp` under `restoreConfig` to restore to. Do not provided more than one option to restore from.
**NOTE:** For the details of the parameters to be used in the .yaml file, please refer [here](./dbcs_controller_parameters.md).

Use the file: [restore_of_database.yaml](./restore_of_database.yaml) for this use case as below:

1. Deploy the .yaml file:  
```sh
[root@docker-test-server OBDS]# kubectl apply -f restore_of_database.yaml
dbcssystem.database.oracle.com/dbcssystem-restore configured
```

2. Monitor the Oracle DB Operator Pod `pod/oracle-database-operator-controller-manager-665874bd57-g2cgw` for the progress of the OBDS VMDB creation of PDBs. 

NOTE: Check the DB Operator Pod name in your environment.

```
[root@docker-test-server OBDS]# kubectl logs -f pod/oracle-database-operator-controller-manager-665874bd57-g2cgw -n  oracle-database-operator-system
```

## Sample Output

[Here](./restore_database_sample_output.log) is the sample output of restore from an existing OBDS System deployed in OCI using Oracle DB Operator OBDS Controller.