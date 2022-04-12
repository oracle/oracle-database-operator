# Scale Down the shape of an existing DBCS System

In this use case, an existing OCI DBCS system deployed earlier is scaled down for its shape using Oracle DB Operator DBCS controller. Its a 2 Step operation.

In order to scale down an existing DBCS system, the steps will be:

1. Bind the existing DBCS System to DBCS Controller.
2. Apply the change to scale down its shape.

**NOTE** We are assuming that before this step, you have followed the [prerequisite](./../README.md#prerequsites-to-deploy-a-dbcs-system-using-oracle-db-operator-dbcs-controller) steps to create the configmap and the secrets required during the deployment.

This example uses `scale_down_dbcs_system_shape.yaml` to scale down a Single Instance DBCS VMDB using Oracle DB Operator DBCS Controller with:

- OCID of existing VMDB as `ocid1.dbsystem.oc1.phx.anyhqljrabf7htyanr3lnp6wtu5ld7qwszohiteodvwahonr2yymrftarkqa`
- OCI Configmap as `oci-cred`  
- OCI Secret as `oci-privatekey`  
- Availability Domain for the DBCS VMDB as `OLou:PHX-AD-1`  
- Compartment OCID as `ocid1.compartment.oc1..aaaaaaaa4hecw2shffuuc4fcatpin4x3rdkesmmf4he67osupo7g6f7i6eya`  
- Database Admin Credential as `admin-password`  
- Database Hostname Prefix as `host0130`  
- Oracle VMDB target Shape as `VM.Standard2.1`  
- SSH Public key for the DBCS system being deployed as `oci-publickey`  
- OCID of the Subnet as `ocid1.subnet.oc1.phx.aaaaaaaauso243tymnzeh6zbz5vkejgyu4ugujul5okpa5xbaq3275izbc7a`  

**NOTE:** For the details of the parameters to be used in the .yaml file, please refer [here](./dbcs_controller_parameters.md).

Use the file: [scale_down_dbcs_system_shape.yaml](./scale_down_dbcs_system_shape.yaml) for this use case as below:

1. Deploy the .yaml file:  
```sh
[root@docker-test-server DBCS]# kubectl apply -f scale_down_dbcs_system_shape.yaml
dbcssystem.database.oracle.com/dbcssystem-existing configured
```

2. Monitor the Oracle DB Operator Pod `pod/oracle-database-operator-controller-manager-665874bd57-g2cgw` for the progress of the DBCS VMDB Scale down. 

NOTE: Check the DB Operator Pod name in your environment.

```
[root@docker-test-server DBCS]# kubectl logs -f pod/oracle-database-operator-controller-manager-665874bd57-g2cgw -n  oracle-database-operator-system
```

## Sample Output

[Here](./scale_down_dbcs_system_shape_sample_output.log) is the sample output for scaling down the shape of an existing DBCS System deployed in OCI using Oracle DB Operator DBCS Controller.
