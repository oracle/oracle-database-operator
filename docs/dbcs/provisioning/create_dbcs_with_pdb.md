# Deploy a DBCS DB System using OCI DBCS Service alongwith PDB

In this use case, an OCI DBCS system is deployed using Oracle DB Operator DBCS controller along with PDB configuration

**NOTE** It is assumed that before this step, you have followed the [prerequisite](./../README.md#prerequisites-to-deploy-a-dbcs-system-using-oracle-db-operator-dbcs-controller) steps to create the configmap and the secrets required during the deployment.

Also, create a Kubernetes secret `pdb-password` using the file:

```bash
#---assuming the PDB password is in ./pdb-password file"

kubectl create secret generic pdb-password --from-file=./pdb-password -n default
```

This example uses `dbcs_service_with_pdb.yaml` to deploy a Single Instance DBCS VMDB using Oracle DB Operator DBCS Controller with:

- OCI Configmap as `oci-cred`  
- OCI Secret as `oci-privatekey`  
- Availability Domain for the DBCS VMDB as `OLou:US-ASHBURN-AD-1`
- Compartment OCID as `ocid1.compartment.oc1..aaaaaaaa63yqilqhgxv3dszur3a2fgwc64ohpfy43vpqjm7q5zq4q4yaw72a`
- Database Admin Credential as `admin-password`  
- Database Name as `dbsystem24`  
- Oracle Database Software Image Version as `21c`  
- Database Workload Type as Transaction Processing i.e. `OLTP`  
- Database Hostname Prefix as `host24`
- Cpu Core Count as `1`
- Oracle VMDB Shape as `VM.Standard2.1`  
- SSH Public key for the DBCS system being deployed as `oci-publickey`  
- domain `subd215df3e6.k8stest.oraclevcn.com`
- OCID of the Subnet as `ocid1.subnet.oc1.iad.aaaaaaaa3lmmxwsykn2jc2vphzpq6eoyoqtte3dpwg6s5fzfkti22ibol2ua`
- PDB Name as `pdb_sauahuja_11`
- TDE Wallet Password as `tde-password`
- PDB Admin Password as `pdb-password`

**NOTE:** For the details of the parameters to be used in the .yaml file, please refer [here](./dbcs_controller_parameters.md). 

Use the file: [dbcs_service_with_pdb.yaml](./dbcs_service_with_pdb.yaml) for this use case as below:

1. Deploy the .yaml file:  
```bash
[root@docker-test-server DBCS]# kubectl apply -f dbcs_service_with_pdb.yaml
dbcssystem.database.oracle.com/dbcssystem-create-with-pdb created
```

2. Monitor the Oracle DB Operator Pod `pod/oracle-database-operator-controller-manager-665874bd57-g2cgw` for the progress of the DBCS VMDB deployment. 

NOTE: Check the DB Operator Pod name in your environment.

```bash
[root@docker-test-server DBCS]# kubectl logs -f pod/oracle-database-operator-controller-manager-665874bd57-g2cgw -n  oracle-database-operator-system
```

## Sample Output

[Here](./dbcs_service_with_pdb_sample_output.log) is the sample output for a DBCS System deployed in OCI using Oracle DB Operator DBCS Controller with PDB configurations.
