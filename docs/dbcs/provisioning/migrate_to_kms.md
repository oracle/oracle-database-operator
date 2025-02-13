# Create and update KMS vault to an existing OBDS System already deployed in OCI Oracle Base Database System (OBDS)

In this use case, an existing OCI OBDS system deployed earlier having encryption with TDE Wallet Password, will be migrated to have KMS Vault created and update OBDS System in OCI. Its a 2 Step operation.

In order to create KMS Vaults to an existing OBDS system, the steps will be:

1. Bind the existing OBDS System (having encryption enabled with TDE Wallet password) to the OBDS Controller.
2. Apply the change to create KMS Vaults.

**NOTE:** We are assuming that before this step, you have followed the [prerequisite](./../README.md#prerequsites-to-deploy-a-dbcs-system-using-oracle-db-operator-dbcs-controller) steps to create the configmap and the secrets required during the deployment. It is also assumed that OBDS System you created earlier is using TDE Wallet password.

As step 1, first bind the existing OBDS System to OBDS Controller following [documentation](./../provisioning/bind_to_existing_dbcs_system.md). After successful binding, it will show as below-
```bash
kubectl get dbcssystems
NAME                  AGE
dbcssystem-create   3m33s
```
Below proceeding further create PDB Admin Password which is going to used as name suggests. 

This example uses `dbcs_service_migrate_to_kms.yaml` to create KMS Vault to existing OBDS VMDB having encryption already enabled earlier with TDE Wallet using Oracle DB Operator OBDS Controller with:

- OCID of existing VMDB as `ocid1.dbsystem.oc1.iad.anuwcljsabf7htyaoja4v2kx5rcfe5w2onndjfpqjhjoakxgwxo2sbgei5iq`
- OCI Configmap as `oci-cred`  
- OCI Secret as `oci-privatekey`
- Existing `dbSystem` details (`compartmentId`,`dbAdminPasswordSecret`,`tdeWalletPasswordSecret`)used before to create OBDS system.
- kmsConfig - vaultName as `dbvault` as an example.
- kmsConfig - keyName as `dbkey` as an example.
- kmsConfig - compartmentId as `ocid1.compartment.oc1..aaaaaaaa63yqilqhgxv3dszur3a2fgwc64ohpfy43vpqjm7q5zq4q4yaw72a` as an example.
**NOTE:** For the details of the parameters to be used in the .yaml file, please refer [here](./dbcs_controller_parameters.md).

Use the file: [dbcs_service_migrate_to_kms.yaml](./dbcs_service_migrate_to_kms.yaml) for this use case as below:

1. Deploy the .yaml file:  
```sh
[root@docker-test-server OBDS]# kubectl apply -f dbcs_service_migrate_to_kms.yaml
dbcssystem.database.oracle.com/dbcssystem-existing configured
```

2. Monitor the Oracle DB Operator Pod `pod/oracle-database-operator-controller-manager-665874bd57-g2cgw` for the progress of the OBDS VMDB creation of KMS Vaults. 

NOTE: Check the DB Operator Pod name in your environment.

```
[root@docker-test-server OBDS]# kubectl logs -f pod/oracle-database-operator-controller-manager-665874bd57-g2cgw -n  oracle-database-operator-system
```

## Sample Output

[Here](./dbcs_service_migrate_to_kms.log) is the sample output for creation of KMS Vaults on an existing OBDS System deployed in OCI using Oracle DB Operator OBDS Controller.
