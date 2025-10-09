# Scale UP the storage of an existing OBDS System

In this use case, an existing OCI OBDS system deployed earlier is scaled up for its storage using Oracle DB Operator OBDS controller. This is a two-step operation.

To scale up storage of an existing OBDS system, the two steps are as follows:

1. Bind the existing OBDS System to OBDS Controller.
2. Apply the change to scale up its storage.

**NOTE** We are assuming that before this step, you have followed the [prerequisite steps](./../README.md#prerequsites-to-deploy-a-dbcs-system-using-oracle-db-operator-dbcs-controller) to create the configmap and the secrets required during the deployment.

This example uses `scale_up_storage.yaml` to scale up storage of an existing Single Instance OBDS VMDB using Oracle DB Operator OBDS Controller with the following:

- OCID of existing VMDB as `ocid1.dbsystem.oc1.ap-mumbai-1.anrg6ljrabf7htyadgsso7aessztysrwaj5gcl3tp7ce6asijm2japyvmroa`
- OCI Configmap as `oci-cred`  
- OCI Secret as `oci-privatekey`  
- Availability Domain for the OBDS VMDB as `OLou:AP-MUMBAI-1-AD-1`  
- Compartment OCID as `ocid1.compartment.oc1..aaaaaaaa63yqilqhgxv3dszur3a2fgwc64ohpfy43vpqjm7q5zq4q4yaw72a`  
- Database Admin Credential as `admin-password`  
- Database Hostname Prefix as `host1234`  
- Target Data Storage Size in GBs as `512`
- Oracle VMDB Shape as `VM.Standard2.1`  
- SSH Public key for the OBDS system being deployed as `oci-publickey`  
- OCID of the Subnet as `ocid1.subnet.oc1.ap-mumbai-1.aaaaaaaa5zpzfax66omtbmjwlv4thruyru7focnu7fjcjksujmgwmr6vpbvq`  


Use the file: [scale_up_storage.yaml](./scale_up_storage.yaml) for this use case as described in the following steps:

1. Deploy the `.yaml` file:  
```sh
[root@test-server OBDS]# kubectl apply -f scale_storage.yaml
dbcssystem.database.oracle.com/dbcssystem-existing configured
```

2. Monitor the Oracle DB Operator Pod `pod/oracle-database-operator-controller-manager-665874bd57-g2cgw` for the progress of the OBDS VMDB Scale up. 

NOTE: Check the DB Operator Pod name in your environment.

```
[root@docker-test-server OBDS]# kubectl logs -f pod/oracle-database-operator-controller-manager-665874bd57-g2cgw -n  oracle-database-operator-system
```

## Sample Output

[this log file](./scale_up_storage_sample_output.log) is an example of an output log file for scaling up the storage of an existing OBDS System deployed in OCI using Oracle DB Operator OBDS Controller with minimal parameters.
