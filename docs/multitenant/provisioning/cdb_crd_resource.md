# Create a CDB CRD Resource using Oracle DB Operator On-Prem Controller

In this use case, using the Oracle Database Operator On-Prem Controller, you will create the CDB kind as a custom resource that will model a CDB as a native Kubernetes object. 

**NOTE:** It is assumed that before this step, you have followed the [prerequisite](./../README.md#prerequsites-to-manage-pdb-life-cycle-using-oracle-db-operator-on-prem-database-controller) steps.

This example uses `create_cdb.yaml` with:

- CDB CRD resource Name as `cdb-dev`
- Container Database (CDB) Name as `goldcdb`
- Scan Name as `goldhost-scan.lbsub52b3b1cae.okecluster.oraclevcn.com`
- Database Server Name as `goldhost1.lbsub52b3b1cae.okecluster.oraclevcn.com`
- ORDS Docker Image as `phx.ocir.io/<repo_name>/oracle/ords:21.4.3`
- Image Pull Secret as `container-registry-secret`
- Database Listener Port as `1521`
- Database Service Name as `goldcdb_phx1pw.lbsub52b3b1cae.okecluster.oraclevcn.com`
- Number of replicas for CDB CRD Resource as 1

**NOTE:** For the details of the parameters to be used in the .yaml file, please refer [config/crd/bases/database.oracle.com_cdbs.yaml](../../../config/crd/bases/database.oracle.com_cdbs.yaml)

Use the file: [cdb.yaml](./cdb.yaml) for this use case as below:

1. Deploy the .yaml file:
```sh
[root@test-server oracle-database-operator]# kubectl apply -f cdb.yaml
```

2. Monitor the Oracle DB Operator Pod for the progress of the CDB CRD Resource creation.

NOTE: Check the DB Operator Pod name in your environment.

```
[root@test-server oracle-database-operator]# kubectl logs -f pod/oracle-database-operator-controller-manager-76cb674c5c-f9wsd -n oracle-database-operator-system
```

## Sample Output

[Here](./cdb.log) is the sample output for a PDB created using Oracle DB Operator On-Prem Controller using file [cdb.yaml](./cdb.yaml)
