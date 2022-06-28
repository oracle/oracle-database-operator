# Unplug a PDB using Oracle DB Operator On-Prem Controller in a target CDB

In this use case, a PDB is unplugged using Oracle DB Operator On-Prem controller.

**NOTE:** It is assumed that before this step, you have followed the [prerequisite](./../README.md#prerequsites-to-manage-pdb-life-cycle-using-oracle-db-operator-on-prem-database-controller) steps.

This example uses `unplug_pdb.yaml` to unplug a PDB from a target CDB using Oracle DB Operator On-Prem Controller with:

- Pluggable Database CRD Resource Name as `pdb1`
- Pluggable Database (PDB) Name as `pdbnew`
- Target CDB CRD Resource Name as `cdb-dev`
- CDB Name as `goldcdb`
- Action to be taken on the PDB as `Unplug`
- XML metadata filename as `/tmp/pdbnewclone.xml`

**NOTE:** For the details of the parameters to be used in the .yaml file, please refer [config/crd/bases/database.oracle.com_pdbs.yaml](../../../config/crd/bases/database.oracle.com_pdbs.yaml)

**NOTE:** Before performing the unplug operation on the PDB CRD Resource, you will first need to perform the Modify Operation on that PDB CRD resource to Close the the PDB. After that you will be able to perform the Unplug operation. Please refer to the use case to modify the PDB state to Close.

Use the file: [unplug_pdb.yaml](./unplug_pdb.yaml) for this use case as below:

1. Deploy the .yaml file:
```sh
% kubectl apply -f unplug_pdb.yaml
```

2. Monitor the Oracle DB Operator Pod for the progress of the PDB Unplug operation:

NOTE: Check the DB Operator Pod name in your environment.

```sh
% kubectl logs -f pod/oracle-database-operator-controller-manager-76cb674c5c-f9wsd -n oracle-database-operator-system
```

## Sample Output

[Here](./unplug_pdb.log) is the sample output for a PDB created using Oracle DB Operator On-Prem Controller using file [unplug_pdb.yaml](./unplug_pdb.yaml)
