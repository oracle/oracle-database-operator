# Setup Dataguard Association to Existing Database of DB System in OCI Base DBCS Service

In this use case, an existing OCI DBCS system previously deployed with an existing Database is provided with an Oracle Data Guard association in OCI Base DBCS Service using the existing Database ID. 

As a prerequisite, obtain the OCID details for the database of the existing DBCS System that you want to clone.  

**NOTE:** We are assuming that before this step, you have followed the [prerequisite](./../README.md#prerequisites-to-deploy-a-dbcs-system-using-oracle-db-operator-dbcs-controller) steps to create the configmap and the secrets required during the deployment.

This example uses `dataguard_in_database.yaml` to clone a Single Instance DBCS VMDB using Oracle DB Operator DBCS Controller with:
- OCI Configmap as `oci-cred`  
- OCI Secret as `oci-privatekey`
- Specification of dataGuard as - Details for dataguard setup `primaryDatabaseId`,`dbAdminPasswordSecret`, `protectionMode`,`transportType`,`displayName`,`availabilityDomain`,`shape`,`subnetId`,`hostName`.
**NOTE:** For the details of the parameters to be used in the .yaml file, please refer [here](./dbcs_controller_parameters.md).

Use the file: [dataguard_in_database.yaml](./dataguard_in_database.yaml) for this use case as below:

1. Deploy the `.yaml` file:  
```sh
[root@docker-test-server DBCS]# kubectl apply -f dataguard_in_database.yaml
dbcssystem.database.oracle.com/dbcssystem-existing configured
```

2. Monitor the Oracle DB Operator Pod `pod/oracle-database-operator-controller-manager-665874bd57-g2cgw` to follow the progress of the DBCS VMDB creation of PDBs. 

NOTE: Check the DB Operator Pod name in your environment.

```
[root@docker-test-server DBCS]# kubectl logs -f pod/oracle-database-operator-controller-manager-665874bd57-g2cgw -n  oracle-database-operator-system
```

## Sample Output

[Here](./dataguard_in_database_sample_output.log) is the example output log for cloning an existing DBCS System deployed in OCI using Oracle DB Operator DBCS Controller.
