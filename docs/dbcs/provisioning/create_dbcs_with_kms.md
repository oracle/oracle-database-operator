# Deploy a DBCS DB System alongwith KMS Vault Encryption in OCI

In this use case, an OCI DBCS system is deployed using Oracle DB Operator DBCS controller along with KMS Vault configuration

**NOTE** It is assumed that before this step, you have followed the [prerequisite](./../README.md#prerequsites-to-deploy-a-dbcs-system-using-oracle-db-operator-dbcs-controller) steps to create the configmap and the secrets required during the deployment.


This example uses `dbcs_service_with_kms.yaml` to deploy a Single Instance DBCS VMDB using Oracle DB Operator DBCS Controller with:

- OCI Configmap as `oci-cred`  
- OCI Secret as `oci-privatekey`  
- Availability Domain for the DBCS VMDB as `OLou:US-ASHBURN-AD-1`
- Compartment OCID as `ocid1.compartment.oc1..aaaaaaaa63yqilqhgxv3dszur3a2fgwc64ohpfy43vpqjm7q5zq4q4yaw72a`
- Database Admin Credential as `admin-password`  
- Database Name as `dbsystem0130`  
- Oracle Database Software Image Version as `21c`  
- Database Workload Type as Transaction Processing i.e. `OLTP`  
- Database Hostname Prefix as `host1205`
- Oracle VMDB Shape as `VM.Standard2.2`  
- SSH Public key for the DBCS system being deployed as `oci-publickey`  
- domain `subd215df3e6.k8stest.oraclevcn.com`
- OCID of the Subnet as `ocid1.subnet.oc1.iad.aaaaaaaa3lmmxwsykn2jc2vphzpq6eoyoqtte3dpwg6s5fzfkti22ibol2ua`
- KMS Vault Name as `basdbvault`
- KMS Compartment Id as `ocid1.compartment.oc1..aaaaaaaa63yqilqhgxv3dszur3a2fgwc64ohpfy43vpqjm7q5zq4q4yaw72a`
- KMS Key Name as `dbvaultkey`

**NOTE:** For the details of the parameters to be used in the .yaml file, please refer [here](./dbcs_controller_parameters.md). While giving KMS Vault make sure not to pass TDE wallet password in DB creation as either of them can be only used for encryption.

Use the file: [dbcs_service_with_kms.yaml](./dbcs_service_with_kms.yaml) for this use case as below:

1. Deploy the .yaml file:  
```bash
[root@docker-test-server DBCS]# kubectl apply -f dbcs_service_with_kms.yaml
dbcssystem.database.oracle.com/dbcssystem-create configured
```

2. Monitor the Oracle DB Operator Pod `pod/oracle-database-operator-controller-manager-665874bd57-g2cgw` for the progress of the DBCS VMDB deployment. 

NOTE: Check the DB Operator Pod name in your environment.

```bash
[root@docker-test-server DBCS]# kubectl logs -f pod/oracle-database-operator-controller-manager-665874bd57-g2cgw -n  oracle-database-operator-system
```

## Sample Output

[Here](./dbcs_service_with_kms_sample_output.log) is the sample output for a DBCS System deployed in OCI using Oracle DB Operator DBCS Controller with KMS configurations.
