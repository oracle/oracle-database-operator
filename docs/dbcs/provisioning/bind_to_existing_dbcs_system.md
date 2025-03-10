# Binding to an existing OBDS System already deployed in OCI Oracle Base Database Service

In this use case, we bind the Oracle DB Operator OBDS Controller to an existing OCI OBDS System which has already been deployed earlier. This will help to manage the life cycle of that OBDS System using the Oracle DB Operator OBDS Controller.

**NOTE** We are assuming that before this step, you have followed the [prerequisite](./../README.md#prerequsites-to-deploy-a-dbcs-system-using-oracle-db-operator-dbcs-controller) steps to create the configmap and the secrets required during the deployment.

This example uses `bind_to_existing_dbcs_system.yaml` to bind to an existing OBDS VMDB using Oracle DB Operator OBDS Controller with:

- OCI Configmap as `oci-cred-mumbai`  
- OCI Secret as `oci-privatekey`  
- OCID of the existing OBDS System as `ocid1.dbsystem.oc1.ap-mumbai-1.anrg6ljrabf7htyadgsso7aessztysrwaj5gcl3tp7ce6asijm2japyvmroa`


Use the file: [bind_to_existing_dbcs_system.yaml](./bind_to_existing_dbcs_system.yaml) for this use case as below:

1. Deploy the .yaml file:  
```bash
kubectl apply -f bind_to_existing_dbcs_system.yaml
dbcssystem.database.oracle.com/dbcssystem-existing created
```

2. Monitor the Oracle DB Leader Operator Pod `pod/oracle-database-operator-controller-manager-665874bd57-g2cgw` for the progress of the OBDS VMDB deployment. 

NOTE: Check the DB Operator Pod name in your environment.

```bash
[root@docker-test-server OBDS]# kubectl logs -f pod/oracle-database-operator-controller-manager-665874bd57-g2cgw -n  oracle-database-operator-system
```

## Sample Output

[Here](./bind_to_existing_dbcs_system_sample_output.log) is the sample output for binding to an existing OBDS System already deployed in OCI using Oracle DB Operator OBDS Controller.
