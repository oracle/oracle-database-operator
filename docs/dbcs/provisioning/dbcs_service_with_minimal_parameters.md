# Deploy a DB System using OCI Oracle Base Database System (OBDS) with minimal parameters

In this use case, an OCI Oracle Base Database System (OBDS) system is deployed using Oracle DB Operator OBDS controller using minimal required parameters in the .yaml file being used during the deployment.

**NOTE** It is assumed that before this step, you have followed the [prerequisite](./../README.md#prerequsites-to-deploy-a-dbcs-system-using-oracle-db-operator-dbcs-controller) steps to create the configmap and the secrets required during the deployment.

This example uses `dbcs_service_with_minimal_parameters.yaml` to deploy a Single Instance OBDS VMDB using Oracle DB Operator OBDS Controller with:

- OCI Configmap as `oci-cred`  
- OCI Secret as `oci-privatekey`  
- Availability Domain for the OBDS VMDB as `OLou:AP-MUMBAI-1-AD-1`
- Compartment OCID as `cid1.compartment.oc1..aaaaaaaa63yqilqhgxv3dszur3a2fgwc64ohpfy43vpqjm7q5zq4q4yaw72a`
- Database Admin Credential as `admin-password`  
- Database Name as `dbsystem1234`  
- Oracle Database Software Image Version as `19c`  
- Database Workload Type as Transaction Processing i.e. `OLTP`  
- Database Hostname Prefix as `host1234`  
- Oracle VMDB Shape as `VM.Standard2.1`  
- SSH Public key for the OBDS system being deployed as `oci-publickey`  
- domain `vcndns.oraclevcn.com`
- OCID of the Subnet as `ocid1.subnet.oc1.ap-mumbai-1.aaaaaaaa5zpzfax66omtbmjwlv4thruyru7focnu7fjcjksujmgwmr6vpbvq`


**NOTE:** For the details of the parameters to be used in the .yaml file, please refer [here](./dbcs_controller_parameters.md). 

Use the file: [dbcs_service_with_minimal_parameters.yaml](./dbcs_service_with_minimal_parameters.yaml) for this use case as below:

1. Deploy the .yaml file:  
```sh
[root@docker-test-server DBCS]# kubectl apply -f create_required.yaml
dbcssystem.database.oracle.com/dbcssystem-create created
```

2. Monitor the Oracle DB Operator Pod `pod/oracle-database-operator-controller-manager-665874bd57-g2cgw` for the progress of the DBCS VMDB deployment. 

NOTE: Check the DB Operator Pod name in your environment.

```
[root@docker-test-server OBDS]# kubectl logs -f pod/oracle-database-operator-controller-manager-665874bd57-g2cgw -n  oracle-database-operator-system
```

## Sample Output

[Here](./dbcs_service_with_minimal_parameters_sample_output.log) is the sample output for a OBDS System deployed in OCI using Oracle DB Operator OBDS Controller with minimal parameters.
