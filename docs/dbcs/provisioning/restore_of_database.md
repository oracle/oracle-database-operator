# Restore from Backup of Existing Database of DB System in OCI OBDS Service

In this use case, an existing OCI OBDS system deployed earlier with existing backup of Database is configured to use restore in OCI Base OBDS Service with an existing database system ID. 

As an prerequisite, obtain the OCID details for the database of an existing OBDS System that you want to back up.  

**NOTE:** We are assuming that before this step, you have followed the [prerequisite](./../README.md#prerequisites-to-deploy-a-dbcs-system-using-oracle-db-operator-dbcs-controller) steps to create the configmap and the secrets required during the deployment.

This example uses `restore_of_database.yaml` to clone a Single Instance OBDS VMDB using Oracle DB Operator OBDS Controller with the following:
- OCID of existing as DB System as `id`
- OCI Configmap as `oci-cred`  
- OCI Secret as `oci-privatekey`
- Restore Configuration with the restore taken from a backup. To perform the restoration, the backup is provided with one of `latest` , `scn` and `timestamp` under `restoreConfig`. For restorations, do not provide more than one option.
**NOTE:** For the details of the parameters to be used in the `.yaml` file, see [dBCS Controller Parameters](./dbcs_controller_parameters.md).

Use the file: [restore_of_database.yaml](./restore_of_database.yaml) for this use case, as described in the following steps:

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

[Here](./restore_database_sample_output.log) is an example output log of a restore from an existing OBDS System deployed in OCI using Oracle DB Operator OBDS Controller.