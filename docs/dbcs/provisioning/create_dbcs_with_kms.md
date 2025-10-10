# Deploy a OBDS DB System alongwith KMS Vault Encryption in OCI

In this use case, an OCI OBDS system is deployed using Oracle DB Operator OBDS controller along with KMS Vault configuration

**NOTE** We assume that before this procedure, you have followed the [prerequisite steps](./../README.md#prerequsites-to-deploy-a-dbcs-system-using-oracle-db-operator-dbcs-controller) to create the configmap and the secrets required during the deployment.

## Pre-requisites for KMS Vaults related to OBDS System
You also must have completed the prerequisites for KMS Vaults related to dynamic group and policies. 

1. Create Dynamic group with rule `ALL {resource.compartment.id =<ocid>` and give it a name.
2. Create a policy in your compartment for this dynamic group to grant it access to key/vaults by database.

```txt
Allow dynamic-group <> to manage secret-family in compartment <>	
Allow dynamic-group <> to manage instance-family in compartment <>	
Allow dynamic-group <> to manage database-family in compartment <>	
Allow dynamic-group <> to manage keys in compartment <>
Allow dynamic-group <> to manage vaults in compartment <>
```

For example: 

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
3. Create KMS Vault and KMS Keys so that you can use it during OBDS provisioning. We refer to those variables (`vaultName`, `keyName`) in the yaml file.

This example uses `dbcs_service_with_kms.yaml` to deploy a Single Instance OBDS VMDB using Oracle DB Operator OBDS Controller with the following:

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

**NOTE:** For the details of the parameters used in the `.yaml` file, see: [DBCS Controller Parameters](./dbcs_controller_parameters.md). When providing the KMS Vault, ensure that you do not pass the TDE wallet password in database creation, because either of them can be used only for encryption.

For the steps that follow, use this file: [dbcs_service_with_kms.yaml](./dbcs_service_with_kms.yaml). Complete the following steps: 

1. Deploy the `.yaml` file:  
```bash
[root@docker-test-server OBDS]# kubectl apply -f dbcs_service_with_kms.yaml
dbcssystem.database.oracle.com/dbcssystem-create created
```

2. Monitor the Oracle DB Operator Pod `pod/oracle-database-operator-controller-manager-665874bd57-g2cgw` to monitor the progress of the OBDS VMDB deployment. 

NOTE: Check the DB Operator Pod name in your environment.

```bash
[root@docker-test-server OBDS]# kubectl logs -f pod/oracle-database-operator-controller-manager-665874bd57-g2cgw -n  oracle-database-operator-system
```

## Sample Output

[This log file](./dbcs_service_with_kms_sample_output.log) is an example output log file for a OBDS System deployed in OCI using Oracle DB Operator OBDS Controller with KMS configurations.
