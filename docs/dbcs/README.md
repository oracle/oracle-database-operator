# Using the DB Operator Oracle Base Database Service (OBDS) Controller 

Oracle Cloud Infastructure (OCI) Oracle Base Database Service (OBDS) provides single-node Database (DB) systems, deployed on virtual machines, and provides two-node Oracle Real Application Clusters (Oracle RAC) database systems on virtual machines.

The single-node DB systems and Oracle RAC systems on virtual machines are [co-managed Oracle Database cloud solutions](https://docs.oracle.com/en-us/iaas/Content/Database/Concepts/overview.htm). To manage the lifecycle of an OCI OBDS system, you can use the OCI Console, the REST API, or the Oracle Cloud Infrastructure command-line interface (CLI). At the granular level, you can use the Oracle Database CLI (DBCLI), Oracle Enterprise Manager, or Oracle SQL Developer.

The Oracle DB Operator Oracle Base Database Service (OBDS) Controller is a feature of the Oracle DB Operator for Kubernetes (OraOperator) which uses OCI's Oracle Base Database Service OBDS service to support lifecycle management of the database systems.

Note: Oracle Base Database Cloud Service (OBDS) was previously known as Database Cloud Service (DBCS).

# Supported Database Editions and Versions

All single-node OCI Oracle RAC DB systems support the following Oracle Database editions:

- Standard Edition
- Enterprise Edition
- Enterprise Edition - High Performance
- Enterprise Edition - Extreme Performance


Two-node Oracle RAC DB systems require Oracle Enterprise Edition - Extreme Performance.

For standard provisioning of DB systems (using Oracle Automatic Storage Management (ASM) as your storage management software), the following database releases are supported:

-   Oracle Database 23ai 
-   Oracle Database 19c

For fast provisioning of single-node virtual machine database systems (using Logical Volume Manager as your storage management software), the following database releases are supported:

- Oracle Database 23ai
- Oracle Database 19c


# Oracle DB Operator Oracle Base Database Service (OBDS) Controller Deployment

To deploy Oracle Database Operator (`OraOperator`), use the [Oracle Database Operator for Kubernetes](https://github.com/oracle/oracle-database-operator/blob/main/README.md) step-by-step procedure.

After the Oracle Database Operator is deployed, you can see the DB operator pods running in the Kubernetes Cluster. As part of the `OraOperator` deployment, the OBDS Controller is deployed as a CRD (Custom Resource Definition). The following screen output is an example of such a deployment:
```bash
[root@test-server oracle-database-operator]# kubectl get ns
NAME                              STATUS   AGE
cert-manager                      Active   33d
default                           Active   118d
kube-node-lease                   Active   118d
kube-public                       Active   118d
kube-system                       Active   118d
oracle-database-operator-system   Active   10m   # <<<< NAMESPACE TO DEPLOY ORACLE DB OPERATOR 

 
 
[root@test-server oracle-database-operator]# kubectl get all -n  oracle-database-operator-system
NAME                                                               READY   STATUS    RESTARTS   AGE
pod/oracle-database-operator-controller-manager-678f96f5f4-f4rhq   1/1     Running   0          10m
pod/oracle-database-operator-controller-manager-678f96f5f4-plxcp   1/1     Running   0          10m
pod/oracle-database-operator-controller-manager-678f96f5f4-qgcg8   1/1     Running   0          10m

NAME                                                                  TYPE        CLUSTER-IP      EXTERNAL-IP   PORT(S)    AGE
service/oracle-database-operator-controller-manager-metrics-service   ClusterIP   10.96.197.164   <none>        8443/TCP   11m
service/oracle-database-operator-webhook-service                      ClusterIP   10.96.35.62     <none>        443/TCP    11m

NAME                                                          READY   UP-TO-DATE   AVAILABLE   AGE
deployment.apps/oracle-database-operator-controller-manager   3/3     3            3           11m

NAME                                                                     DESIRED   CURRENT   READY   AGE
replicaset.apps/oracle-database-operator-controller-manager-6657bfc664   0         0         0       11m
replicaset.apps/oracle-database-operator-controller-manager-678f96f5f4   3         3         3       10m 
 
[root@test-server oracle-database-operator]# kubectl get crd
NAME                                             CREATED AT
autonomousdatabasebackups.database.oracle.com    2022-02-08T18:28:55Z
autonomousdatabaserestores.database.oracle.com   2022-02-08T18:28:55Z
autonomousdatabases.database.oracle.com          2022-02-22T23:23:25Z
certificaterequests.cert-manager.io              2022-02-22T23:21:35Z
certificates.cert-manager.io                     2022-02-22T23:21:36Z
challenges.acme.cert-manager.io                  2022-02-22T23:21:36Z
clusterissuers.cert-manager.io                   2022-02-22T23:21:36Z
dbcssystems.database.oracle.com                  2022-02-22T23:23:25Z  # <<<< CRD for OBDS Controller
issuers.cert-manager.io                          2022-02-22T23:21:36Z
orders.acme.cert-manager.io                      2022-02-22T23:21:37Z
shardingdatabases.database.oracle.com            2022-02-22T23:23:25Z
singleinstancedatabases.database.oracle.com      2022-02-22T23:23:25Z
```


# Prerequisites to deploy a OBDS system using Oracle DB Operator OBDS Controller

Before you deploy a OBDS system in OCI using the Oracle DB Operator OBDS Controller, complete the following procedure.

**CAUTION :** You must make the changes specified in this section before you proceed to the next section.

## 1. Create a Kubernetes Configmap. In this example. we create a Kubernetes Configmap named `oci-cred` with the OCI account we are using: 

```bash
kubectl create configmap oci-cred \
--from-literal=tenancy=<tenancy-ocid> \
--from-literal=user=<user-ocid> \
--from-literal=fingerprint=<fingerprint in xx:xx format> \
--from-literal=region=us-phoenix-1
```


## 2. Create a Kubernetes secret `oci-privatekey` using the OCI Pem key taken from OCI console for the account you are using:

```bash
#---assuming the OCI Pem key to be "/root/.oci/oci_api_key.pem"

kubectl create secret generic oci-privatekey --from-file=privatekey=/root/.oci/oci_api_key.pem
```


## 3. Create a Kubernetes secret named `admin-password`; This passward must meet the minimum passward requirements for the OCI OBDS Service.
For example:

```bash
#-- assuming the passward has been added to a text file named "admin-password":

kubectl create secret generic admin-password --from-file=./admin-password -n default
```


## 4. Create a Kubernetes secret named `tde-password`; this passward must meet the minimum passward requirements for the OCI OBDS Service.
For example:

```bash
# -- assuming the passward has been added to a text file named "tde-password":

kubectl create secret generic tde-password --from-file=./tde-password -n default
```


## 5. Create an SSH key pair, and use its public key to create a Kubernetes secret named `oci-publickey`; the private key for this public key can be used later to access the OBDS system's host machine using SSH:

```bash
[root@test-server OBDS]# ssh-keygen -N "" -C "DBCS_System"-`date +%Y%m` -P ""
Generating public/private rsa key pair.
Enter file in which to save the key (/root/.ssh/id_rsa):
Your identification has been saved in /root/.ssh/id_rsa.
Your public key has been saved in /root/.ssh/id_rsa.pub.
The key fingerprint is:
SHA256:+SuiES/3m9+iuIVyG/QBQL1x7CfRsxtvswBsaBuW5iE DBCS_System-202203
The key's randomart image is:
+---[RSA 2048]----+
|   .o. . .       |
|     .o + o      |
|      .O . o     |
|    E X.*.+      |
|    .*.=S+ +     |
|     +oo oo +    |
|    + * o .o o   |
|     *.*...o.    |
|    ..+o==o..    |
+----[SHA256]-----+
 

[root@test-server OBDS]# kubectl create secret generic oci-publickey --from-file=publickey=/root/DBCS/id_rsa.pub
```

# Use Cases to manage the lifecycle of an OCI OBDS System with Oracle DB Operator OBDS Controller

For more informatoin about the multiple use cases available to you to deploy and manage the OCI OBDS Service-based database using the Oracle DB Operator OBDS Controller, review this list:

[1. Deploy a DB System using OCI OBDS Service with minimal parameters](./provisioning/dbcs_service_with_minimal_parameters.md)  
[2. Binding to an existing OBDS System already deployed in OCI Oracle Base Database Service](./provisioning/bind_to_existing_dbcs_system.md)  
[3. Scale UP the shape of an existing OBDS System](./provisioning/scale_up_dbcs_system_shape.md)  
[4. Scale DOWN the shape of an existing OBDS System](./provisioning/scale_down_dbcs_system_shape.md)  
[5. Scale UP the storage of an existing OBDS System](./provisioning/scale_up_storage.md)  
[6. Update License type of an existing OBDS System](./provisioning/update_license.md)  
[7. Terminate an existing OBDS System](./provisioning/terminate_dbcs_system.md)  
[8. Create OBDS with All Parameters with Storage Management as LVM](./provisioning/dbcs_service_with_all_parameters_lvm.md)  
[9. Create OBDS with All Parameters with Storage Management as ASM](./provisioning/dbcs_service_with_all_parameters_asm.md)  
[10. Deploy a 2 Node RAC DB System using OCI OBDS Service](./provisioning/dbcs_service_with_2_node_rac.md)  
[11. Create PDB to an existing OBDS System already deployed in OCI OBDS Service](./provisioning/create_pdb_to_existing_dbcs_system.md)   
[12. Create OBDS with PDB in OCI](./provisioning/create_dbcs_with_pdb.md)  
[13. Create OBDS with KMS Vault Encryption in OCI](./provisioning/create_dbcs_with_kms.md)    
[14. Migrate to KMS vault from TDE Wallet password encryption of an existing OBDS System already deployed in OCI Base OBDS Service](./provisioning/migrate_to_kms.md)  
[15. Clone DB System from Existing DB System in OCI OBDS Service](./provisioning/clone_from_existing_dbcs.md)  
[16. Clone DB System from Backup of Existing DB System in OCI OBDS Service](./provisioning/clone_from_backup_dbcs.md)  
[17. Clone DB System from Existing Database of DB System in OCI OBDS Service](./provisioning/clone_from_database.md)
[18. Create Backup of Existing Database of DB System in OCI OBDS Service](./provisioning/backup_of_database.md)
[19. Restore from Backup of Existing Database of DB System in OCI OBDS Service](./provisioning/restore_of_database.md)
[20. Setup Dataguard Association to Existing Database of DB System in OCI Base DBCS Service](./provisioning/dataguard_to_database.md)
[21. Disable Dataguard Association to Existing Database of DB System and Terminate Peer DB System in OCI Base DBCS Service](./provisioning/disable_dataguard_to_database.md)
[22. Patching Existing Database of DB System in OCI Base DBCS Service](./provisioning/patching_database.md) 
[23. Upgrading Existing Database of DB System in OCI Base DBCS Service](./provisioning/upgrading_database.md) 
[17. Clone DB System from Existing Database of DB System in OCI OBDS Service](./provisioning/clone_from_database.md)  

## Connecting to OCI OBDS database deployed using Oracle DB Operator OBDS Controller

After you have deployed the OCI OBDS database with the Oracle DB Operator OBDS Controller, you can connect to the database. To see how to connect and use the database, refer to the steps in [Database Connectivity](./provisioning/database_connection.md).

## Known Issues

If you encounter any issues with deployment, refer to the list of [Known Issues](./provisioning/known_issues.md) for an OCI OBDS System deployed using Oracle DB Operator OBDS Controller.
