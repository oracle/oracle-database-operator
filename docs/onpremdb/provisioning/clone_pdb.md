# Clone a PDB using Oracle DB Operator On-Prem Controller in a target CDB

In this use case, a PDB is cloned using Oracle DB Operator On-Prem controller.

**NOTE:** It is assumed that before this step, you have followed the [prerequisite](./../README.md#prerequsites-to-manage-pdb-life-cycle-using-oracle-db-operator-on-prem-database-controller) steps.

This example uses `clone_pdb.yaml` to clone a PDB using Oracle DB Operator On-Prem Controller with:

- PDB CRD resource Name as `pdb1-clone`
- Pluggable Database (PDB) Name as `pdbnewclone`
- Total Size of the PDB as `UNLIMITED`
- Total size for temporary tablespace as `UNLIMITED`
- Target CDB CRD Resource Name as `cdb-dev`
- Target CDB name as `goldcdb`
- Source PDB Name as `pdbnew`

**NOTE:** For the details of the parameters to be used in the .yaml file, please refer [config/crd/bases/database.oracle.com_pdbs.yaml](../../../config/crd/bases/database.oracle.com_pdbs.yaml)

Use the file: [clone_pdb.yaml](./clone_pdb.yaml) for this use case as below:

1. Deploy the .yaml file:
```sh
[root@test-server oracle-database-operator]# kubectl apply -f clone_pdb.yaml
```

2. Monitor the Oracle DB Operator Pod for the progress of the PDB creation.

NOTE: Check the DB Operator Pod name in your environment.

```
[root@test-server oracle-database-operator]# kubectl logs -f pod/oracle-database-operator-controller-manager-76cb674c5c-f9wsd -n oracle-database-operator-system
```

## Sample Output

[Here](./clone_pdb.log) is the sample output for a PDB created using Oracle DB Operator On-Prem Controller using file [clone_pdb.yaml](./clone_pdb.yaml)
