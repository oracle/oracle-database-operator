# Patching Existing Database of DB System in OCI Base DBCS Service

In this use case, an existing OCI OBDS system deployed earlier is going to be patched in OCI Oracle Base Database System (OBDS). Its a 2 Step operation.

In order to patch OBDS to an existing OBDS system, get the OCID of DB System ID  you want to patch.

**NOTE:** We are assuming that before this step, you have followed the [prerequisite](./../README.md#prerequisites-to-deploy-a-dbcs-system-using-oracle-db-operator-dbcs-controller) steps to create the configmap and the secrets required during the deployment.
As step 1, first bind the existing DBCS System to DBCS Controller following [documentation](./../provisioning/bind_to_existing_dbcs_system.md). After successful binding, it will show as below-
```bash
kubectl get dbcssystems
NAME                  AGE
dbcssystem-existing   3m33s
```

Step 2 uses `patch_dbcs_system.yaml` to patch a Single Instance OBDS VMDB using Oracle DB Operator OBDS Controller with:

- OCID of existing VMDB as `id` to be patched.
- OCI Configmap as `oci-cred`  
- OCI Secret as `oci-privatekey`
- `isPatch` as true
- Specification of DB System been patched as `dbPatchOcid`. These must be unique and new details for new patched DB system to be created.
**NOTE:** For the details of the parameters to be used in the .yaml file, please refer [here](./dbcs_controller_parameters.md).

Use the file: [patch_dbcs_system.yaml](./patch_dbcs_system.yaml) for this use case as below:

1. Deploy the .yaml file:  
```sh
[root@docker-test-server DBCS]# kubectl apply -f patch_dbcs_system.yaml
dbcssystem.database.oracle.com/dbcssystem-patch configured
```

2. Monitor the Oracle DB Operator Pod `pod/oracle-database-operator-controller-manager-665874bd57-g2cgw` for the progress of the DBCS VMDB creation of PDBs. 

NOTE: Check the DB Operator Pod name in your environment.

```
[root@docker-test-server DBCS]# kubectl logs -f pod/oracle-database-operator-controller-manager-665874bd57-g2cgw -n  oracle-database-operator-system
```

## Sample Output

[Here](./patch_dbcs_system_sample_output.log) is the sample output for cloning an existing DBCS System deployed in OCI using Oracle DB Operator DBCS Controller.
