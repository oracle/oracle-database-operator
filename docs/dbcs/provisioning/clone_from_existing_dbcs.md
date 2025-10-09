# Clone DB System from Existing DB System in OCI Oracle Base Database System (OBDS)

In this use case, an existing OCI OBDS system deployed earlier is going to be cloned in OCI Oracle Base Database System (OBDS). It is a two-step operation. 

To clone OBDS to an existing OBDS system, obtain the OCID of the database system ID that you want to clone.

**NOTE:** We are assuming that before this step, you have followed the [prerequisite steps](./../README.md#prerequisites-to-deploy-a-dbcs-system-using-oracle-db-operator-dbcs-controller) to create the configmap and the secrets required during the deployment.

This example uses `clone_dbcs_system.yaml` to clone a Single Instance OBDS VMDB using Oracle DB Operator OBDS Controller with the following:

- OCID of existing VMDB as `id` to be cloned.
- OCI Configmap as `oci-cred`  
- OCI Secret as `oci-privatekey`
- setupDBCloning: as `true` 
- Specification of DB System been cloned as `dbClone` -> `dbAdminPaswordSecret`, `dbName`,`hostName`,`displayName`,`licenseModel`,`domain`,`sshPublicKeys`,`subnetId`. These must be unique and new details for new cloned DB system to be created.
**NOTE:** For the details of the parameters to be used in the .yaml file, please refer [here](./dbcs_controller_parameters.md).

Use the file: [clone_dbcs_system.yaml](./clone_dbcs_system.yaml) for this use case as described in the following steps:

1. Deploy the `.yaml` file:  
```sh
[root@docker-test-server DBCS]# kubectl apply -f clone_dbcs_system.yaml
dbcssystem.database.oracle.com/dbcssystem-clone created
```

2. Monitor the Oracle DB Operator Pod `pod/oracle-database-operator-controller-manager-665874bd57-g2cgw` to follow the progress of the DBCS VMDB creation of PDBs. 

NOTE: Check the DB Operator Pod name in your environment.

```
[root@docker-test-server DBCS]# kubectl logs -f pod/oracle-database-operator-controller-manager-665874bd57-g2cgw -n  oracle-database-operator-system
```

## Sample Output

[This log file](./clone_dbcs_system_sample_output.log) is an example output log file for cloning an existing DBCS System deployed in OCI using Oracle DB Operator DBCS Controller.
