# Delete a PDB using Oracle DB Operator On-Prem Controller in a target CDB

In this use case, a PDB is deleted using Oracle DB Operator On-Prem controller.

To delete a PDB CRD Resource, a sample .yaml file is available here: [config/samples/onpremdb/pdb_delete.yaml](../../../config/samples/onpremdb/pdb_delete.yaml)

**NOTE:** It is assumed that before this step, you have followed the [prerequisite](./../README.md#prerequsites-to-manage-pdb-life-cycle-using-oracle-db-operator-on-prem-database-controller) steps.

This example uses `delete_pdb.yaml` to delete a PDB using Oracle DB Operator On-Prem Controller with:

- Pluggable Database (PDB) Name as `pdbnewclone`
- Target CDB CRD Resource Name as `cdb-dev`
- Action to be taken on the PDB as `Delete`
- Option to specify if datafiles should be removed as `INCLUDING`

**NOTE:** You need to *modify* the PDB status to MOUNTED, as described earlier, on the target CDB before you want to delete that PDB.

**NOTE:** For the details of the parameters to be used in the .yaml file, please refer [config/crd/bases/database.oracle.com_pdbs.yaml](../../../config/crd/bases/database.oracle.com_pdbs.yaml)

Use the file: [delete_pdb.yaml](./delete_pdb.yaml) for this use case as below:

1. Deploy the .yaml file:
```sh
% kubectl apply -f delete_pdb.yaml
```

2. Monitor the Oracle DB Operator Pod `pod/oracle-database-operator-controller-manager-665874bd57-g2cgw` for the progress of the PDB deletion.

NOTE: Check the DB Operator Pod name in your environment.

```sh
% kubectl logs -f pod/oracle-database-operator-controller-manager-76cb674c5c-f9wsd -n oracle-database-operator-system
```

## Sample Output

[Here](./delete_pdb.log) is the sample output for a PDB created using Oracle DB Operator On-Prem Controller using file [delete_pdb.yaml](./delete_pdb.yaml)
