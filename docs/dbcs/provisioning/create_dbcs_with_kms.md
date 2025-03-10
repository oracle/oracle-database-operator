# Deploy a OBDS DB System alongwith KMS Vault Encryption in OCI

In this use case, an OCI OBDS system is deployed using Oracle DB Operator OBDS controller along with KMS Vault configuration

**NOTE** It is assumed that before this step, you have followed the [prerequisite](./../README.md#prerequsites-to-deploy-a-dbcs-system-using-oracle-db-operator-dbcs-controller) steps to create the configmap and the secrets required during the deployment.

## Pre-requisites for KMS Vaults related to OBDS System
There is also other set of pre-requisites for KMS Vaults related to dynamic group and policies. Please follow instructions for same. 
1. Create Dynamic group with rule `ALL {resource.compartment.id =<ocid>` and give it some name.
2. Create policy in your compartment for this dynamic group to access to key/vaults by database.

```txt
Allow dynamic-group <> to manage secret-family in compartment <>	
Allow dynamic-group <> to manage instance-family in compartment <>	
Allow dynamic-group <> to manage database-family in compartment <>	
Allow dynamic-group <> to manage keys in compartment <>
Allow dynamic-group <> to manage vaults in compartment <>
```

E.g

```txt
ALL {resource.compartment.id = 'ocid1.compartment.oc1..aaaaaaaa63yqilqhgxv3dszur3a2fgwc64ohpfy43vpqjm7q5zq4q4yaw72a'}
```
```txt
Allow dynamic-group db_dynamic_group to manage secret-family in compartment sauahuja	
Allow dynamic-group db_dynamic_group to manage instance-family in compartment sauahuja	
Allow dynamic-group db_dynamic_group to manage database-family in compartment sauahuja	
Allow dynamic-group db_dynamic_group to manage keys in compartment sauahuja
Allow dynamic-group db_dynamic_group to manage vaults in compartment sauahuja
```
3. Do also create KMS Vault and KMS Key in order to use it during OBDS provisioning. We are going to refer those variables (`vaultName`, `keyName`) in the yaml file.

This example uses `dbcs_service_with_kms.yaml` to deploy a Single Instance OBDS VMDB using Oracle DB Operator OBDS Controller with:

- OCI Configmap as `oci-cred`  
- OCI Secret as `oci-privatekey`  
- Availability Domain for the OBDS VMDB as `OLou:AP-MUMBAI-1-AD-1`
- Compartment OCID as `ocid1.compartment.oc1..aaaaaaaa63yqilqhgxv3dszur3a2fgwc64ohpfy43vpqjm7q5zq4q4yaw72a`
- Database Admin Credential as `admin-password`  
- Database Name as `kmsdb`  
- Oracle Database Software Image Version as `19c`  
- Database Workload Type as Transaction Processing i.e. `OLTP`  
- Database Hostname Prefix as `kmshost`
- Oracle VMDB Shape as `VM.Standard2.2`  
- SSH Public key for the OBDS system being deployed as `oci-publickey`  
- domain `subdda0b5eaa.cluster1.oraclevcn.com`
- OCID of the Subnet as `ocid1.subnet.oc1.ap-mumbai-1.aaaaaaaa5zpzfax66omtbmjwlv4thruyru7focnu7fjcjksujmgwmr6vpbvq`
- KMS Vault Name as `dbvault`
- KMS Compartment Id as `ocid1.compartment.oc1..aaaaaaaa63yqilqhgxv3dszur3a2fgwc64ohpfy43vpqjm7q5zq4q4yaw72a`
- KMS Key Name as `dbkey`

**NOTE:** For the details of the parameters to be used in the .yaml file, please refer [here](./dbcs_controller_parameters.md). While giving KMS Vault make sure not to pass TDE wallet password in DB creation as either of them can be only used for encryption.

Use the file: [dbcs_service_with_kms.yaml](./dbcs_service_with_kms.yaml) for this use case as below:

1. Deploy the .yaml file:  
```bash
[root@docker-test-server OBDS]# kubectl apply -f dbcs_service_with_kms.yaml
dbcssystem.database.oracle.com/dbcssystem-create created
```

2. Monitor the Oracle DB Operator Pod `pod/oracle-database-operator-controller-manager-665874bd57-g2cgw` for the progress of the OBDS VMDB deployment. 

NOTE: Check the DB Operator Pod name in your environment.

```bash
[root@docker-test-server OBDS]# kubectl logs -f pod/oracle-database-operator-controller-manager-665874bd57-g2cgw -n  oracle-database-operator-system
```

## Sample Output

[Here](./dbcs_service_with_kms_sample_output.log) is the sample output for a OBDS System deployed in OCI using Oracle DB Operator OBDS Controller with KMS configurations.
