# Terminate an existing Oracle Base Database System (OBDS)

In this use case, an existing OCI OBDS system deployed earlier is terminated using Oracle DB Operator OBDS controller. Its a 2 Step operation.

In order to terminate an existing OBDS system, the steps will be:

1. Bind the existing OBDS System to OBDS Controller.
2. Apply the change to terminate this OBDS System.

**NOTE** We are assuming that before this step, you have followed the [prerequisite](./../README.md#prerequsites-to-deploy-a-dbcs-system-using-oracle-db-operator-dbcs-controller) steps to create the configmap and the secrets required during the deployment.

This example uses `terminate_dbcs_system.yaml` to terminated a Single Instance OBDS VMDB using Oracle DB Operator OBDS Controller with:

- OCID of existing VMDB as `ocid1.dbsystem.oc1.phx.anyhqljrabf7htyanr3lnp6wtu5ld7qwszohiteodvwahonr2yymrftarkqa`
- OCI Configmap as `oci-cred`  
- OCI Secret as `oci-privatekey`  

**NOTE:** For the details of the parameters to be used in the .yaml file, please refer [here](./dbcs_controller_parameters.md).

Use the file: [terminate_dbcs_system.yaml](./terminate_dbcs_system.yaml) for this use case as below:

1. Deploy the .yaml file:  
```sh
[root@test-server OBDS]# kubectl apply -f terminate_dbcs_system.yaml
dbcssystem.database.oracle.com/dbcssystem-terminate created
 

[root@test-server OBDS]# kubectl delete -f terminate_dbcs_system.yaml
dbcssystem.database.oracle.com "dbcssystem-terminate" deleted
```

2. Check the logs of Oracle DB Operator Pod `pod/oracle-database-operator-controller-manager-665874bd57-g2cgw` for an update on the terminate operation been accepted. 

```
[root@test-server OBDS]# kubectl logs -f pod/oracle-database-operator-controller-manager-665874bd57-g2cgw -n  oracle-database-operator-system
```

3. Check and confirm if the existing OCI OBDS system is NO longer available after sometime because of termination:

```
[root@test-server OBDS]# kubectl describe dbcssystems.database.oracle.com dbcssystem-terminate
```

## Sample Output

[Here](./terminate_dbcs_system_sample_output.log) is the sample output for terminating an existing OBDS System deployed in OCI using Oracle DB Operator OBDS Controller with minimal parameters.
