# Binding to an existing DBCS System already deployed in OCI DBCS Service

In this use case, we bind the Oracle DB Operator DBCS Controller to an existing OCI DBCS System which has already been deployed earlier. This will help to manage the life cycle of that DBCS System using the Oracle DB Operator DBCS Controller.

**NOTE** We are assuming that before this step, you have followed the [prerequisite](./../README.md#prerequsites-to-deploy-a-dbcs-system-using-oracle-db-operator-dbcs-controller) steps to create the configmap and the secrets required during the deployment.

This example uses `bind_to_existing_dbcs_system.yaml` to bind to an existing DBCS VMDB using Oracle DB Operator DBCS Controller with:

- OCI Configmap as `oci-cred`  
- OCI Secret as `oci-privatekey`  
- OCID of the existing DBCS System as `ocid1.dbsystem.oc1.phx.anyhqljrabf7htyanr3lnp6wtu5ld7qwszohiteodvwahonr2yymrftarkqa`


Use the file: [bind_to_existing_dbcs_system.yaml](./bind_to_existing_dbcs_system.yaml) for this use case as below:

1. Deploy the .yaml file:  
```sh
[root@docker-test-server DBCS]# kubectl apply -f bind_dbcs.yaml
dbcssystem.database.oracle.com/dbcssystem-existing created
```

2. Monitor the Oracle DB Operator Pod `pod/oracle-database-operator-controller-manager-665874bd57-g2cgw` for the progress of the DBCS VMDB deployment. 

NOTE: Check the DB Operator Pod name in your environment.

```
[root@docker-test-server DBCS]# kubectl logs -f pod/oracle-database-operator-controller-manager-665874bd57-g2cgw -n  oracle-database-operator-system
```

## Sample Output

[Here](./bind_to_existing_dbcs_system_sample_output.log) is the sample output for binding to an existing DBCS System already deployed in OCI using Oracle DB Operator DBCS Controller.
