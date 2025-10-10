# Terminate an existing Oracle Base Database System (OBDS)

In this use case, an existing OCI OBDS system deployed earlier is terminated using Oracle DB Operator OBDS controller. This is a two-step operation.

In order to terminate an existing OBDS system, the two steps are as follows:

1. Bind the existing OBDS System to OBDS Controller.
2. Apply the change to terminate this OBDS System.

**NOTE** We are assuming that before this step, you have followed the [prerequisite steps](./../README.md#prerequsites-to-deploy-a-dbcs-system-using-oracle-db-operator-dbcs-controller) to create the configmap and the secrets required during the deployment.

This example uses `terminate_dbcs_system.yaml` to terminated a Single Instance OBDS VMDB using Oracle DB Operator OBDS Controller with the following:

- OCID of existing VMDB as `ocid1.dbsystem.oc1.phx.anyhqljrabf7htyanr3lnp6wtu5ld7qwszohiteodvwahonr2yymrftarkqa`
- OCI Configmap as `oci-cred`  
- OCI Secret as `oci-privatekey`  

**NOTE:** For the details of the parameters to be used in the `.yaml` file, see [DBCS Controller Parameters](./dbcs_controller_parameters.md).

Use the file: [terminate_dbcs_system.yaml](./terminate_dbcs_system.yaml) for this use case as described in the following steps:

1. Deploy the `.yaml` file:  
```sh
[root@test-server OBDS]# kubectl apply -f terminate_dbcs_system.yaml
dbcssystem.database.oracle.com/dbcssystem-terminate created
 

[root@test-server OBDS]# kubectl delete -f terminate_dbcs_system.yaml
dbcssystem.database.oracle.com "dbcssystem-terminate" deleted
```

2. Check the logs of Oracle DB Operator Pod `pod/oracle-database-operator-controller-manager-665874bd57-g2cgw` for an update on the terminate operation to confirm it has been accepted. 

```
[root@test-server OBDS]# kubectl logs -f pod/oracle-database-operator-controller-manager-665874bd57-g2cgw -n  oracle-database-operator-system
```

3. Give some time for the termination operation to be completed, and then check and confirm if the existing OCI OBDS system is no longer available:

```
[root@test-server OBDS]# kubectl describe dbcssystems.database.oracle.com dbcssystem-terminate
```

## Sample Output

[This example log](./terminate_dbcs_system_sample_output.log) is an example output log for terminating an existing OBDS System deployed in OCI using Oracle DB Operator OBDS Controller with minimal parameters.
