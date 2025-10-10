# Upgrade Existing Database of DB System in OCI Base DBCS Service

In this use case, an existing OCI OBDS system deployed earlier is going to be upgraded in OCI Oracle Base Database System (OBDS). This is a two-step operation.

To upgrade OBDS to an existing OBDS system, obtain the OCID of DB System ID that you want to upgrade.

**NOTE:** We are assuming that before this step, you have followed the [prerequisite steps](./../README.md#prerequisites-to-deploy-a-dbcs-system-using-oracle-db-operator-dbcs-controller) to create the configmap and the secrets required during the deployment.

For the first step, bind the existing DBCS System to DBCS Controller following the [Bind ot Existing DBCS System documentation](./../provisioning/bind_to_existing_dbcs_system.md). After successful binding, it will show as follows: 
```bash
kubectl get dbcssystems
NAME                  AGE
dbcssystem-existing   3m33s
```

Step 2 uses `upgrade_dbcs_system.yaml` to upgrade a Single Instance OBDS VMDB using Oracle DB Operator OBDS Controller with the following:

- OCID of existing VMDB as `id` to be upgraded.
- OCI Configmap as `oci-cred`  
- OCI Secret as `oci-privatekey`
- `isupgrade` as true
- Specification of the DB System being upgraded as `dbUpgradeVersion`. The specification details must be unique and new for the new upgraded DB system that is created.
**NOTE:** For the details of the parameters to be used in the `.yaml` file, see: [DBCS Controller Parameters](./dbcs_controller_parameters.md).

Use the file: [upgrade_dbcs_system.yaml](./upgrade_dbcs_system.yaml) for this use case as described in the following steps:

1. Deploy the `.yaml` file:  
```sh
[root@docker-test-server DBCS]# kubectl apply -f upgrade_dbcs_system.yaml
dbcssystem.database.oracle.com/dbcssystem-existing configured
```

2. Monitor the Oracle DB Operator Pod `pod/oracle-database-operator-controller-manager-665874bd57-g2cgw` for the progress of the DBCS VMDB creation of PDBs. 

NOTE: Check the DB Operator Pod name in your environment.

```
[root@docker-test-server DBCS]# kubectl logs -f pod/oracle-database-operator-controller-manager-665874bd57-g2cgw -n  oracle-database-operator-system
```

3. After the upgrade, describe the Kubernetes object to ensure that the correct database version is completed: 
```bash
kubectl get dbcssystems.database.oracle.com dbcssystem-existing
kubectl get dbcssystems.database.oracle.com dbcssystem-existing -o jsonpath='{.status.dbVersion}'
```