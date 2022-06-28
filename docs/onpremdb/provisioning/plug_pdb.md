# Plug in a PDB using Oracle DB Operator On-Prem Controller in a target CDB

In this use case, a PDB is plugged in using Oracle DB Operator On-Prem controller using an existing .xml file which was generated when the PDB was unplugged from this target CDB or another CDB.

To plug in a PDB CRD Resource, a sample .yaml file is available here: [config/samples/onpremdb/pdb_plug.yaml](../../../config/samples/onpremdb/pdb_plug.yaml)

**NOTE:** It is assumed that before this step, you have followed the [prerequisite](./../README.md#prerequsites-to-manage-pdb-life-cycle-using-oracle-db-operator-on-prem-database-controller) steps.

This example uses `plug_pdb.yaml` to plug in a PDB to a target CDB using Oracle DB Operator On-Prem Controller with:

- Pluggable Database CRD Resource Name as `pdb1`
- Pluggable Database (PDB) Name as `pdbnew`
- Target CDB CRD Resource Name as `cdb-dev`
- CDB Name as `goldcdb`
- Action to be taken on the PDB as `Plug`
- XML metadata filename as `/tmp/pdbnewclone.xml`
- Source File Name Conversion as `NONE`
- File Name Conversion as `NONE`
- Copy Action as `MOVE`
- PDB Size as `1G`
- Temporary tablespace Size as `100M`

**NOTE:** For the details of the parameters to be used in the .yaml file, please refer [config/crd/bases/database.oracle.com_pdbs.yaml](../../../config/crd/bases/database.oracle.com_pdbs.yaml)

**NOTE:** Before performing the plug inoperation, you will first need to confirm the availability of the .xml file and the PDB datafiles.

Use the file: [plug_pdb.yaml](./plug_pdb.yaml) for this use case as below:

1. Deploy the .yaml file:
```sh
% kubectl apply -f plug_pdb.yaml
```

2. Monitor the Oracle DB Operator Pod for the progress of the PDB Unplug operation:

NOTE: Check the DB Operator Pod name in your environment.

```sh
% kubectl logs -f pod/oracle-database-operator-controller-manager-76cb674c5c-f9wsd -n oracle-database-operator-system
```

## Sample Output

[Here](./plug_pdb.log) is the sample output for a PDB created using Oracle DB Operator On-Prem Controller using file [plug_pdb.yaml](./plug_pdb.yaml)
