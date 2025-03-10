# Delete PDB of an existing DBCS System

In this use case, an existing OCI DBCS system deployed earlier is going to have PDB/PDBs deleted. Its a 2 Step operation.

In order to create PDBs to an existing DBCS system, the steps will be:

1. Bind the existing DBCS System to DBCS Controller.
2. Apply the change to delete PDBs.

**NOTE:** We are assuming that before this step, you have followed the [prerequisite](./../README.md#prerequsites-to-deploy-a-dbcs-system-using-oracle-db-operator-dbcs-controller) steps to create the configmap and the secrets required during the deployment.

As step 1, first bind the existing DBCS System to DBCS Controller following [documentation](./../provisioning/bind_to_existing_dbcs_system.md). After successful binding, it will show as below-
```bash
kubectl get dbcssystems
NAME                  AGE
dbcssystem-existing   3m33s
```

This example uses `deletepdb_in_existing_dbcs_system_list.yaml` to delete PDBs of a Single Instance DBCS VMDB using Oracle DB Operator DBCS Controller with:

- OCID of existing VMDB as `ocid1.dbsystem.oc1.iad.anuwcljsabf7htyag4akvoakzw4qk7cae55qyp7hlffbouozvyl5ngoputza`
- OCI Configmap as `oci-cred`  
- OCI Secret as `oci-privatekey`
- PDB Name to be deleted e.g `pdb_sauahuja_11` and `pdb_sauahuja_12`
**NOTE:** For the details of the parameters to be used in the .yaml file, please refer [here](./dbcs_controller_parameters.md).

Use the file: [deletepdb_in_existing_dbcs_system_list.yaml](./deletepdb_in_existing_dbcs_system_list.yaml) for this use case as below:

1. Deploy the .yaml file:  
```sh
[root@docker-test-server DBCS]# kubectl apply -f deletepdb_in_existing_dbcs_system_list.yaml
dbcssystem.database.oracle.com/dbcssystem-existing configured
```

2. Monitor the Oracle DB Operator Pod `pod/oracle-database-operator-controller-manager-665874bd57-g2cgw` for the progress of the DBCS VMDB deletion of PDBs. 

NOTE: Check the DB Operator Pod name in your environment.

```bash
[root@docker-test-server DBCS]# kubectl logs -f pod/oracle-database-operator-controller-manager-665874bd57-g2cgw -n  oracle-database-operator-system
```

3. Remove DBCS Systems resource-
```bash
kubectl delete -f deletepdb_in_existing_dbcs_system_list.yaml
```

## Sample Output

[Here](./deletepdb_in_existing_dbcs_system_list_sample_output.log) is the sample output for deletion of PDBs from an existing DBCS System deployed in OCI using Oracle DB Operator DBCS Controller.