# Deploy a DBCS DB System using OCI DBCS Service with minimal parameters

In this use case, an OCI DBCS system is deployed using Oracle DB Operator DBCS controller using minimal required parameters in the .yaml file being used during the deployment.

**NOTE** It is assumed that before this step, you have followed the [prerequisite](./../README.md#prerequsites-to-deploy-a-dbcs-system-using-oracle-db-operator-dbcs-controller) steps to create the configmap and the secrets required during the deployment.

This example uses `dbcs_service_with_minimal_parameters.yaml` to deploy a Single Instance DBCS VMDB using Oracle DB Operator DBCS Controller with:

- OCI Configmap as `oci-cred`  
- OCI Secret as `oci-privatekey`  
- Availability Domain for the DBCS VMDB as `OLou:EU-MILAN-1-AD-1`
- Compartment OCID as `ocid1.compartment.oc1..aaaaaaaaks5baeqlvv4kyj2jiwnrbxgzm3gsumcfy4c6ntj2ro5i3a5gzhhq`
- Database Admin Credential as `admin-password`  
- Database Name as `dbsystem0130`  
- Oracle Database Software Image Version as `19c`  
- Database Workload Type as Transaction Processing i.e. `OLTP`  
- Database Hostname Prefix as `host1205`  
- Oracle VMDB Shape as `VM.Standard2.1`  
- SSH Public key for the DBCS system being deployed as `oci-publickey`  
- domain `vcndns.oraclevcn.com`
- OCID of the Subnet as `ocid1.subnet.oc1.eu-milan-1.aaaaaaaaeiy3tvcsnyg6upfp3ydtu7jmfnmoyifq2ax6y45b5qpdbpide5xa`


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
[root@docker-test-server DBCS]# kubectl logs -f pod/oracle-database-operator-controller-manager-665874bd57-g2cgw -n  oracle-database-operator-system
```

## Sample Output

[Here](./dbcs_service_with_minimal_parameters_sample_output.log) is the sample output for a DBCS System deployed in OCI using Oracle DB Operator DBCS Controller with minimal parameters.
