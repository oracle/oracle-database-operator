# Create a PDB using Oracle DB Operator On-Prem Controller in a target CDB

The Oracle Database Operator On-Prem Controller creates the PDB kind as a custom resource that models a PDB as a native Kubernetes object. There is a one-to-one mapping between the actual PDB and the Kubernetes PDB Custom Resource. Each PDB resource follows the PDB CRD as defined here: [config/crd/bases/database.oracle.com_pdbs.yaml](../../../config/crd/bases/database.oracle.com_pdbs.yaml)

To create a PDB CRD Resource, a sample .yaml file is available here: [config/samples/onpremdb/pdb_create.yaml](../../../config/samples/onpremdb/pdb_create.yaml)

**NOTE:** It is assumed that before this step, you have followed the [prerequisite](./../README.md#prerequsites-to-manage-pdb-life-cycle-using-oracle-db-operator-on-prem-database-controller) steps.

This example uses `create_pdb.yaml` to create a PDB using Oracle DB Operator On-Prem Controller with:

- PDB CRD resource Name as `pdb1`
- Pluggable Database (PDB) Name as `pdbnew`
- Total Size of the PDB as `1GB`
- Total size for temporary tablespace as `100M`
- Target CDB CRD Resource Name as `cdb-dev`
- Target CDB name as `goldcdb`

**NOTE:** For the details of the parameters to be used in the .yaml file, please refer [config/crd/bases/database.oracle.com_pdbs.yaml](../../../config/crd/bases/database.oracle.com_pdbs.yaml)

Use the file: [create_pdb.yaml](./create_pdb.yaml) for this use case as below:

1. Deploy the .yaml file:
```sh
[root@test-server oracle-database-operator]# kubectl apply -f create_pdb.yaml
```

2. Monitor the Oracle DB Operator Pod for the progress of the PDB creation.

NOTE: Check the DB Operator Pod name in your environment.

```
[root@test-server oracle-database-operator]# kubectl logs -f pod/oracle-database-operator-controller-manager-76cb674c5c-f9wsd -n oracle-database-operator-system
```

## Sample Output

[Here](./create_pdb.log) is the sample output for a PDB created using Oracle DB Operator On-Prem Controller using file [create_pdb.yaml](./create_pdb.yaml)
