# Update License type of an existing DBCS System

In this use case, the license type of an existing OCI DBCS system deployed earlier is changed from `License Included` to `Bring your own license` using Oracle DB Operator DBCS controller. Its a 2 Step operation.

In order to update the license type an existing DBCS system, the steps will be:

1. Bind the existing DBCS System to DBCS Controller.
2. Apply the change to change its license type.

**NOTE** We are assuming that before this step, you have followed the [prerequisite](./../README.md#prerequsites-to-deploy-a-dbcs-system-using-oracle-db-operator-dbcs-controller) steps to create the configmap and the secrets required during the deployment.

This example uses `update_license.yaml` to change the license type of a Single Instance DBCS VMDB using Oracle DB Operator DBCS Controller with:

- OCID of existing VMDB as `ocid1.dbsystem.oc1.phx.anyhqljrabf7htyanr3lnp6wtu5ld7qwszohiteodvwahonr2yymrftarkqa`
- OCI Configmap as `oci-cred`  
- OCI Secret as `oci-privatekey`  
- Availability Domain for the DBCS VMDB as `OLou:PHX-AD-1`  
- Compartment OCID as `ocid1.compartment.oc1..aaaaaaaa4hecw2shffuuc4fcatpin4x3rdkesmmf4he67osupo7g6f7i6eya`  
- Database Admin Credential as `admin-password`  
- Database Hostname Prefix as `host0130`
- Target license model as `BRING_YOUR_OWN_LICENSE`
- Oracle VMDB Shape as `VM.Standard2.1`  
- SSH Public key for the DBCS system being deployed as `oci-publickey`  
- OCID of the Subnet as `ocid1.subnet.oc1.phx.aaaaaaaauso243tymnzeh6zbz5vkejgyu4ugujul5okpa5xbaq3275izbc7a`  

**NOTE:** For the details of the parameters to be used in the .yaml file, please refer [here](./dbcs_controller_parameters.md).

Use the file: [update_license.yaml](./update_license.yaml) for this use case as below:

1. Deploy the .yaml file:  
```sh
[root@test-server DBCS]# kubectl apply -f update_license.yaml
dbcssystem.database.oracle.com/dbcssystem-existing configured
```

2. Monitor the Oracle DB Operator Pod `pod/oracle-database-operator-controller-manager-665874bd57-g2cgw` for the progress of the DBCS VMDB Scale up. 

NOTE: Check the DB Operator Pod name in your environment.

```
[root@docker-test-server DBCS]# kubectl logs -f pod/oracle-database-operator-controller-manager-665874bd57-g2cgw -n  oracle-database-operator-system
```

## Sample Output

[Here](./update_license_sample_output.log) is the sample output for updating the license type an existing DBCS System deployed in OCI using Oracle DB Operator DBCS Controller.
