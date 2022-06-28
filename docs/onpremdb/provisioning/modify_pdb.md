# Modify a PDB using Oracle DB Operator On-Prem Controller in a target CDB

In this use case, the state of an existing PDB is modified using Oracle DB Operator On-Prem controller.

**NOTE:** It is assumed that before this step, you have followed the [prerequisite](./../README.md#prerequsites-to-manage-pdb-life-cycle-using-oracle-db-operator-on-prem-database-controller) steps.

Subcase 1: This example uses `modify_pdb_close.yaml` to close a PDB using Oracle DB Operator On-Prem Controller with:

- PDB CRD resource Name as `pdb1`
- Pluggable Database (PDB) Name as `pdbnew`
- Target CDB CRD Resource Name as `cdb-dev`
- Target CDB name as `goldcdb`
- Action to be taken on the PDB as `MODIFY`
- Target state of the PDB as `CLOSE`
- Option to close the state (i.e. modify) as `IMMEDIATE`


**NOTE:** For the details of the parameters to be used in the .yaml file, please refer [config/crd/bases/database.oracle.com_pdbs.yaml](../../../config/crd/bases/database.oracle.com_pdbs.yaml)

Use the file: [modify_pdb_close.yaml](./modify_pdb_close.yaml) for this use case as below:

1. Deploy the .yaml file:
```sh
% kubectl apply -f modify_pdb_close.yaml
pdb.database.oracle.com/pdb1 configured
```

2. Monitor the Oracle DB Operator Pod for the progress of the PDB creation.

NOTE: Check the DB Operator Pod name in your environment.

```
[root@test-server oracle-database-operator]# kubectl logs -f pod/oracle-database-operator-controller-manager-76cb674c5c-f9wsd -n oracle-database-operator-system
```

Subcase 2: This example uses `modify_pdb_open.yaml` to open a PDB using Oracle DB Operator On-Prem Controller with:

- PDB CRD resource Name as `pdb1`
- Pluggable Database (PDB) Name as `pdbnew`
- Target CDB CRD Resource Name as `cdb-dev`
- Target CDB name as `goldcdb`
- Action to be taken on the PDB as `MODIFY`
- Target state of the PDB as `OPEN`
- Option to close the state (i.e. modify) as `READ WRITE`


**NOTE:** For the details of the parameters to be used in the .yaml file, please refer [config/crd/bases/database.oracle.com_pdbs.yaml](../../config/crd/bases/database.oracle.com_pdbs.yaml)

Use the file: [modify_pdb_open.yaml](./modify_pdb_open.yaml) for this use case as below:

1. Deploy the .yaml file:
```sh
% kubectl apply -f modify_pdb_open.yaml
pdb.database.oracle.com/pdb1 configured
```

2. Monitor the Oracle DB Operator Pod for the progress of the PDB creation.

NOTE: Check the DB Operator Pod name in your environment.

```
[root@test-server oracle-database-operator]# kubectl logs -f pod/oracle-database-operator-controller-manager-76cb674c5c-f9wsd -n oracle-database-operator-system
```

## Sample Output

[Here](./modify_pdb.log) is the sample output for a PDB created using Oracle DB Operator On-Prem Controller using file [modify_pdb_close.yaml](./modify_pdb_close.yaml) and [modify_pdb_open.yaml](./modify_pdb_open.yaml)
