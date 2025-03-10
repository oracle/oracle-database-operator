# Deploy a 2 Node RAC DB System using OCI OBDS Service

In this use case, a 2 Node RAC OCI OBDS system is deployed using Oracle DB Operator OBDS controller using all the available parameters in the .yaml file being used during the deployment. The type of the Storage Management in this case is ASM.

**NOTE** We are assuming that before this step, you have followed the [prerequisite](./../README.md#prerequsites-to-deploy-a-dbcs-system-using-oracle-db-operator-dbcs-controller) steps to create the configmap and the secrets required during the deployment.

This example uses `dbcs_service_with_2_node_rac.yaml` to deploy a 2 Node RAC VMDB using Oracle DB Operator OBDS Controller with:

- OCI Configmap as `oci-cred`  
- OCI Secret as `oci-privatekey`  
- Cluster Name as `maa-cluster`
- Availability Domain for the OBDS VMDB as `OLou:AP-MUMBAI-1-AD-1`  
- Compartment OCID as `ocid1.compartment.oc1..aaaaaaaa63yqilqhgxv3dszur3a2fgwc64ohpfy43vpqjm7q5zq4q4yaw72a`  
- Database Admin Credential as `admin-password`  
- Enable flag for Automatic Backup for OBDS Database as `True`
- Auto Backup Window for OBDS Database as `SLOT_FOUR`
- Recovery Windows for Backup retention in days as `15`
- Oracle Database Edition as `STANDARD_EDITION`
- Database Name as `db0130`  
- Oracle Database Software Image Version as `19c`  
- Database Workload Type as Transaction Processing i.e. `OLTP`  
- Redundancy of the ASM Disks as `EXTERNAL`
- Display Name for the OBDS System as `dbsys123`
- Database Hostname Prefix as `host01234`  
- Initial Size of the DATA Storage in GB as `256`
- License Model as `BRING_YOUR_OWN_LICENSE`
- Name of the PDB to be created as `PDB0123`
- Private IP explicitly assigned to be `10.0.1.99`
- Node count as `2`
- Oracle VMDB Shape as `VM.Standard2.1` 
- SSH Public key for the OBDS system being deployed as `oci-publickey`  
- Storage Management type as `ASM`
- OCID of the Subnet as `ocid1.subnet.oc1.ap-mumbai-1.aaaaaaaa5zpzfax66omtbmjwlv4thruyru7focnu7fjcjksujmgwmr6vpbv`  
- Tag the OBDS system with two key value pairs as `"TEST": "test_case_provision"` and `"CreatedBy": "MAA_TEAM"`
- TDE Wallet Secret as `tde-password`
- Time Zone for the OBDS System as `Europe/Berlin`


**NOTE:** For the details of the parameters to be used in the .yaml file, please refer [here](./dbcs_controller_parameters.md). 

Use the file: [dbcs_service_with_all_parameters_asm.yaml](./dbcs_service_with_2_node_rac.yaml) for this use case as below:

1. Deploy the .yaml file:  
```sh
[root@docker-test-server test]# kubectl apply -f dbcs_service_with_2_node_rac.yaml
dbcssystem.database.oracle.com/dbcssystem-create configured
```

2. Monitor the Oracle DB Operator Pod `pod/oracle-database-operator-controller-manager-665874bd57-g2cgw` for the progress of the OBDS VMDB deployment. 

NOTE: Check the DB Operator Pod name in your environment.

```
[root@docker-test-server OBDS]# kubectl logs -f pod/oracle-database-operator-controller-manager-665874bd57-g2cgw -n  oracle-database-operator-system
```

## Sample Output

[Here](./dbcs_service_with_2_node_rac_sample_output.log) is the sample output for a 2 Node RAC OBDS System deployed in OCI using Oracle DB Operator OBDS Controller with all parameters and with Storage Management as ASM.
