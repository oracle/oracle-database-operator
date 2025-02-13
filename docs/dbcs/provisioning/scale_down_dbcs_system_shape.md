# Scale Down the shape of an existing OBDS System

In this use case, an existing OCI OBDS system deployed earlier is scaled down for its shape using Oracle DB Operator OBDS controller. Its a 2 Step operation.

In order to scale down an existing OBDS system, the steps will be:

1. Bind the existing OBDS System to OBDS Controller.
2. Apply the change to scale down its shape.

**NOTE** We are assuming that before this step, you have followed the [prerequisite](./../README.md#prerequsites-to-deploy-a-dbcs-system-using-oracle-db-operator-dbcs-controller) steps to create the configmap and the secrets required during the deployment.

This example uses `scale_down_dbcs_system_shape.yaml` to scale down a Single Instance OBDS VMDB using Oracle DB Operator OBDS Controller with:

- OCID of existing VMDB as `ocid1.dbsystem.oc1.ap-mumbai-1.anrg6ljrabf7htyadgsso7aessztysrwaj5gcl3tp7ce6asijm2japyvmroa`
- OCI Configmap as `oci-cred`  
- OCI Secret as `oci-privatekey`  
- Availability Domain for the OBDS VMDB as `OLou:AP-MUMBAI-1-AD-1`  
- Compartment OCID as `ocid1.compartment.oc1..aaaaaaaa63yqilqhgxv3dszur3a2fgwc64ohpfy43vpqjm7q5zq4q4yaw72`  
- Database Admin Credential as `admin-password`  
- Database Hostname Prefix as `host1234`  
- Oracle VMDB target Shape as `VM.Standard2.1`  
- SSH Public key for the OBDS system being deployed as `oci-publickey`  
- OCID of the Subnet as `ocid1.subnet.oc1.ap-mumbai-1.aaaaaaaa5zpzfax66omtbmjwlv4thruyru7focnu7fjcjksujmgwmr6vpbvq`  

**NOTE:** For the details of the parameters to be used in the .yaml file, please refer [here](./dbcs_controller_parameters.md).

Use the file: [scale_down_dbcs_system_shape.yaml](./scale_down_dbcs_system_shape.yaml) for this use case as below:

1. Deploy the .yaml file:  
```sh
[root@docker-test-server OBDS]# kubectl apply -f scale_down_dbcs_system_shape.yaml
dbcssystem.database.oracle.com/dbcssystem-existing configured
```

2. Monitor the Oracle DB Operator Pod `pod/oracle-database-operator-controller-manager-665874bd57-g2cgw` for the progress of the OBDS VMDB Scale down. 

NOTE: Check the DB Operator Pod name in your environment.

```
[root@docker-test-server OBDS]# kubectl logs -f pod/oracle-database-operator-controller-manager-665874bd57-g2cgw -n  oracle-database-operator-system
```

## Sample Output

[Here](./scale_down_dbcs_system_shape_sample_output.log) is the sample output for scaling down the shape of an existing OBDS System deployed in OCI using Oracle DB Operator OBDS Controller.
