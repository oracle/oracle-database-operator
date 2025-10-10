# Disable Dataguard Association to Existing Database of DB System and Terminate Peer DB System in OCI Base DBCS Service

In this use case, we are going to disable Dataguard and Terminate Peer DB System of an existing OCI OCS system deployed earlier with existing Database and dataguard association. Note: Both disable Dataguard and Terminate Peer DB System will happen together. 

As an pre-requisite, get the details of OCID of database of an existing DBCS System which you want to clone.  

**NOTE:** We are assuming that before this step, you have followed the [prerequisite](./../README.md#prerequisites-to-deploy-a-dbcs-system-using-oracle-db-operator-dbcs-controller) steps to create the configmap and the secrets required during the deployment.

This example uses `dataguard_in_database.yaml` to clone a Single Instance DBCS VMDB using Oracle DB Operator DBCS Controller with:
- OCI Configmap as `oci-cred`  
- OCI Secret as `oci-privatekey`
- Specification of dataGuard as - Details for dataguard setup `peerDbSystemId`, `enabled: false` and `isDelete: true`.
**NOTE:** For the details of the parameters to be used in the .yaml file, please refer [here](./dbcs_controller_parameters.md).

Use the file: [disable_dataguard_in_database.yaml](./disable_dataguard_in_database.yaml) for this use case as below:

1. Deploy the .yaml file:  
```sh
[root@docker-test-server DBCS]# kubectl apply -f disable_dataguard_in_database.yaml
dbcssystem.database.oracle.com/dbcssystem-existing configured
```

2. Monitor the Oracle DB Operator Pod `pod/oracle-database-operator-controller-manager-665874bd57-g2cgw` for the progress of the DBCS VMDB creation of PDBs. 

NOTE: Check the DB Operator Pod name in your environment.

```
[root@docker-test-server DBCS]# kubectl logs -f pod/oracle-database-operator-controller-manager-665874bd57-g2cgw -n  oracle-database-operator-system
```
3. Describe the Kubernetes object to see more details
```bash
kubectl describe dbcssystems.database.oracle.com dbcssystem-existing
```
More precisely
```bash
kubectl get dbcssystems.database.oracle.com dbcssystem-existing  -o jsonpath='{.status.dataGuardStatus.lifecycleState}'
TERMINATED
```


