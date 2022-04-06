# Using the DB Operator DBCS Controller 

The Oracle Cloud Infrastructure's Database Service furnishes [co-managed Oracle Database cloud solutions](https://docs.oracle.com/en-us/iaas/Content/Database/Concepts/overview.htm). A single-node DB systems on either bare metal or virtual machines, and 2-node RAC DB systems on virtual machines. To manage the life cycle of an OCI DBCS system, we can use the OCI Console, the REST API or the Oracle Cloud Infrastructure CLI and at the granular level, the Database CLI (DBCLI), Enterprise Manager, or SQL Developer.

The Oracle DB Operator DBCS Controller is a feature of the Oracle DB Operator for Kubernetes (a.k.a. OraOperator) which supports the life cycle management of the Database Systems deployed using OCI's DBCS Service.

# Supported Database Editions and Versions

All single-node OCI Oracle RAC DB systems support the following Oracle Database editions:

- Standard Edition
- Enterprise Edition
- Enterprise Edition - High Performance
- Enterprise Edition - Extreme Performance


Two-node Oracle RAC DB systems require Oracle Enterprise Edition - Extreme Performance.

For standard provisioning of DB systems (using Oracle Automatic Storage Management (ASM) as your storage management software), the supported database versions are:

-   Oracle Database 21c
-   Oracle Database 19c
-   Oracle Database 18c (18.0)
-   Oracle Database 12c Release 2 (12.2)
-   Oracle Database 12c Release 1 (12.1)
-   Oracle Database 11g Release 2 (11.2)


For fast provisioning of single-node virtual machine database systems (using Logical Volume Manager as your storage management software), the supported database versions are:

- Oracle Database 21c
- Oracle Database 19c
- Oracle Database 18c
- Oracle Database 12c Release 2 (12.2)


# Oracle DB Operator DBCS Controller Deployment

The step by step procedure to deploy the OraOperator is documented [here](https://github.com/oracle/oracle-database-operator/blob/main/README.md).

Once the Oracle DB Operator has been deployed, we can see the DB operator pods running in the Kubernetes Cluster. The DBCS Controller will deployed as part of the Oracle DB Operator Deployment as a CRD (Custom Resource Definition). Below is an example of such a deployment:
```
[root@test-server oracle-database-operator]# kubectl get ns
NAME                              STATUS   AGE
cert-manager                      Active   2m5s
default                           Active   125d
kube-node-lease                   Active   125d
kube-public                       Active   125d
kube-system                       Active   125d
oracle-database-operator-system   Active   17s    <<<< namespace to deploy the Oracle Database Operator
 
 
[root@test-server oracle-database-operator]# kubectl get all -n  oracle-database-operator-system
NAME                                                               READY   STATUS    RESTARTS   AGE
pod/oracle-database-operator-controller-manager-665874bd57-dlhls   1/1     Running   0          28s
pod/oracle-database-operator-controller-manager-665874bd57-g2cgw   1/1     Running   0          28s
pod/oracle-database-operator-controller-manager-665874bd57-q42f8   1/1     Running   0          28s
 
NAME                                                                  TYPE        CLUSTER-IP      EXTERNAL-IP   PORT(S)    AGE
service/oracle-database-operator-controller-manager-metrics-service   ClusterIP   10.96.130.124   <none>        8443/TCP   29s
service/oracle-database-operator-webhook-service                      ClusterIP   10.96.4.104     <none>        443/TCP    29s
 
NAME                                                          READY   UP-TO-DATE   AVAILABLE   AGE
deployment.apps/oracle-database-operator-controller-manager   3/3     3            3           29s
 
NAME                                                                     DESIRED   CURRENT   READY   AGE
replicaset.apps/oracle-database-operator-controller-manager-665874bd57   3         3         3       29s
[root@docker-test-server oracle-database-operator]#
 
 
[root@test-server oracle-database-operator]# kubectl get crd
NAME                                             CREATED AT
autonomousdatabasebackups.database.oracle.com    2022-02-08T18:28:55Z
autonomousdatabaserestores.database.oracle.com   2022-02-08T18:28:55Z
autonomousdatabases.database.oracle.com          2022-02-22T23:23:25Z
certificaterequests.cert-manager.io              2022-02-22T23:21:35Z
certificates.cert-manager.io                     2022-02-22T23:21:36Z
challenges.acme.cert-manager.io                  2022-02-22T23:21:36Z
clusterissuers.cert-manager.io                   2022-02-22T23:21:36Z
dbcssystems.database.oracle.com                  2022-02-22T23:23:25Z  <<<< CRD for DBCS Controller
issuers.cert-manager.io                          2022-02-22T23:21:36Z
orders.acme.cert-manager.io                      2022-02-22T23:21:37Z
shardingdatabases.database.oracle.com            2022-02-22T23:23:25Z
singleinstancedatabases.database.oracle.com      2022-02-22T23:23:25Z
```


# Prerequsites to deploy a DBCS system using Oracle DB Operator DBCS Controller

Complete the following steps before deploying a DBCS system in OCI using the Oracle DB Operator DBCS Controller:

**IMPORTANT :** You must make the changes specified in this section before you proceed to the next section.

## 1. Create a Kubernetes Configmap. For example: We are creating a Kubernetes Configmap named "oci-cred" using the OCI account we are using as below: 

```
kubectl create configmap oci-cred \
--from-literal=tenancy=ocid1.tenancy.oc1..................67iypsmea \
--from-literal=user=ocid1.user.oc1..aaaaaaaaxw3i...............ce6qzdrnmq \
--from-literal=fingerprint=b2:7c:a8:d5:44:f5.....................:9a:55 \
--from-literal=region=us-phoenix-1
```


## 2. Create a Kubernetes secret "oci-privatekey" using the OCI Pem key taken from OCI console for the account you are using:

```
-- assuming the OCI Pem key to be "/root/.oci/oci_api_key.pem"

kubectl create secret generic oci-privatekey --from-file=privatekey=/root/.oci/oci_api_key.pem
```


## 3. Create a Kubernetes secret named "admin-password". This passward needs to satisfy the minimum passward requirements for the OCI DBCS Service. For example:

```
-- assuming the passward has been added to a text file named "admin-password":

kubectl create secret generic admin-password --from-file=./admin-password -n default
```


## 4. Create a Kubernetes secret named "tde-password". This passward needs to satisfy the minimum passward requirements for the OCI DBCS Service. For example:

```
-- assuming the passward has been added to a text file named "tde-password":

kubectl create secret generic tde-password --from-file=./tde-password -n default
```


## 5. Create an ssh key pair and use its public key to create a Kubernetes secret named "oci-publickey". The private key for this public key can be used later to access the DBCS system's host machine using ssh:

```
[root@test-server DBCS]# ssh-keygen -N "" -C "DBCS_System"-`date +%Y%m` -P ""
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
 

[root@test-server DBCS]# kubectl create secret generic oci-publickey --from-file=publickey=/root/DBCS/id_rsa.pub
```




# Use Cases to manage life cycle of a OCI DBCS System using Oracle DB Operator DBCS Controller

There are multiple use cases to deploy and manage the OCI DBCS Service based database using the Oracle DB Operator DBCS Controller.

[1. Deploy a DB System using OCI DBCS Service with minimal parameters](./provisioning/dbcs_service_with_minimal_parameters.md)  
[2. Binding to an existing DBCS System already deployed in OCI DBCS Service](./provisioning/bind_to_existing_dbcs_system.md)  
[3. Scale UP the shape of an existing DBCS System](./provisioning/scale_up_dbcs_system_shape.md)  
[4. Scale DOWN the shape of an existing DBCS System](./provisioning/scale_down_dbcs_system_shape.md)  
[5. Scale UP the storage of an existing DBCS System](./provisioning/scale_up_storage.md)  
[6. Update License type of an existing DBCS System](./provisioning/update_license.md)  
[7. Terminate an existing DBCS System](./provisioning/terminate_dbcs_system.md)  
[8. Create DBCS with All Parameters with Storage Management as LVM](./provisioning/dbcs_service_with_all_parameters_lvm.md)  
[9. Create DBCS with All Parameters with Storage Management as ASM](./provisioning/dbcs_service_with_all_parameters_asm.md)  
[10. Deploy a 2 Node RAC DB System using OCI DBCS Service](./provisioning/dbcs_service_with_2_node_rac.md)

## Connecting to OCI DBCS database deployed using Oracle DB Operator DBCS Controller

After the OCI DBCS database has been deployed using Oracle DB Operator DBCS Controller, you can follow the steps in this document to connect to this Database: [Database Connectivity](./provisioning/database_connection.md)

## Known Issues

Please refer to the list of [Known Issues](./provisioning/known_issues.md) for an OCI DBCS System deployed using Oracle DB Operator DBCS Controller.
