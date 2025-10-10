# Patching Existing Database of DB System in OCI Base DBCS Service

In this use case, see how an existing OCI OBDS system deployed earlier can be patched using the OCI Oracle Base Database System (OBDS). This is a two-step operation. 

To patch OBDS to an existing OBDS system, obtain the OCID of the database system ID that you want to patch.

**NOTE:** We are assuming that before this step, you have followed the [prerequisite](./../README.md#prerequisites-to-deploy-a-dbcs-system-using-oracle-db-operator-dbcs-controller) steps to create the configmap and the secrets required during the deployment.
As step 1, first bind the existing DBCS System to DBCS Controller following [documentation](./../provisioning/bind_to_existing_dbcs_system.md). After successful binding, it will appear as follows:
```bash
kubectl get dbcssystems
NAME                  AGE
dbcssystem-existing   3m33s
```

Step 2 uses `patch_dbcs_system.yaml` to patch a Single Instance OBDS VMDB using Oracle DB Operator OBDS Controller with the following:

- OCID of the existing VMDB as `id` to be patched.
- OCI Configmap as `oci-cred`  
- OCI Secret as `oci-privatekey`
- `isPatch` as true
- Specification of the database system that you are patching as `dbPatchOcid`. The OCIDs must be unique, and you must provide new details for new patched DB system that are to be created.
**NOTE:** For the details of the parameters to be used in the `.yaml` file, see: [here](./dbcs_controller_parameters.md).

Use the file: [patch_dbcs_system.yaml](./patch_dbcs_system.yaml) for this use case as described in the following steps:

1. Deploy the `.yaml` file:  
```sh
[root@docker-test-server DBCS]# kubectl apply -f patch_dbcs_system.yaml
dbcssystem.database.oracle.com/dbcssystem-existing configured
```

2. Monitor the Oracle DB Operator Pod `pod/oracle-database-operator-controller-manager-665874bd57-g2cgw` to follow the progress of the DBCS VMDB creation of PDBs. 

NOTE: Check the DB Operator Pod name in your environment.

```
[root@docker-test-server DBCS]# kubectl logs -f pod/oracle-database-operator-controller-manager-665874bd57-g2cgw -n  oracle-database-operator-system
```
3. Check details of the Kubernetes object post patching to ensure that it is complete, and verify that the patched version is as expected:
```bash
kubectl describe dbcssystems.database.oracle.com dbcssystem-existing

kubectl get dbcssystems.database.oracle.com dbcssystem-existing -o jsonpath='{.status.dbVersion}'
19.28.0.0.0
```