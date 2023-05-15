<span style="font-family:Liberation mono; font-size:0.9em; line-height: 1.1em">


# STEP BY STEP USE CASE 

- [INTRODUCTION](#introduction)
- [OPERATION STEPS ](#operation-steps)
- [Download latest version from github ](#download-latest-version-from-orahub-a-namedownloada)
- [Upload webhook certificates](#upload-webhook-certificates-a-namewebhooka)
- [Create the dboperator](#create-the-dboperator-a-namedboperatora)
- [Create Secret for container registry](#create-secret-for-container-registry)
- [Build ords immage ](#build-ords-immage-a-nameordsimagea)
- [Database Configuration](#database-configuration)
- [Create CDB secret ](#create-cdb-secret)
- [Create Certificates](#create-certificates)
- [Apply cdb.yaml](#apply-cdbyaml)
- [Logs and throuble shutting](#cdb---logs-and-throuble-shutting)
- [Create PDB secret](#create-pdb-secret)
- [Other action ](#other-actions)



##### INTRODUCTION

This readme is a step by step guide used to implement database multi tenant operator. It assumes that a kubernets cluster and a database server are already available (no matter if single instance or RAC). kubectl must be configured in order to reach k8s cluster.

The following table reports the parameters required to configure and use oracle multi tenant controller for pluggable database lifecycle management.

| yaml file parameters            	| value  	| description /ords parameter                     	              |
|--------------	|---------------------------	|-------------------------------------------------|
| dbserver     	| <db_host\> or <scan_name>   | [--db-hostname][1]                              |
| dbTnsurl      | <tns connect descriptor\>   | [--db-custom-url/db.customURL][dbtnsurl]                                |
| port         	| <oracle_port\>        	    | [--db-port][2]                                	|
| cdbName       | <dbname\>                   | Container Name                                  |
| name          | <cdb-dev\>                  | Ords podname prefix in cdb.yaml                 |
| name          | <pdb\>                      | pdb resource in pdb.yaml                        | 
| ordsImage     | <public_container_registry\>/ords-dboper:latest|My public container registry  |
| pdbName       | <pdbname\>                  | Pluggable database name                         |
| servicename  	| <service_name\>           	| [--db-servicename][3]                           |
| sysadmin_user | <SYS_SYSDBA\>               | [--admin-user][adminuser]                       |
| sysadmin_pwd  | <sys_password\>             | [--password-stdin][pwdstdin]                    |
| cdbadmin_user | <CDB_ADMIN_USER\>           | [db.cdb.adminUser][1]                           |
| cdbadmin_pwd  | <CDB_ADMIN_PASS\>           | [db.cdb.adminUser.password][cdbadminpwd]        |
| webserver_user| <web_user\>                 | [https user][http] <span style="color:red"> NOT A DB USER </span> |
| webserver_pwd | <web_user_passwoed\>        | [http user password][http]                      |
| ords_pwd      | <ords_password\>            | [ORDS_PUBLIC_USER password][public_user]        |
| pdbTlsKey     | <keyfile\>                  | [standalone.https.cert.key][key]                |
| pdbTlsCrt     | <certfile\>                 | [standalone.https.cert][cr]                     |
| pdbTlsCat     | <certauth\>                 | certificate authority                           |

> A [makfile](./makefile) is available to sped up the command execution for the multitenant setup and test. See the comments in the header of file  

### OPERATIONAL STEPS 
----


#### Download latest version from github <a name="Download"></a>


```bash
git clone https://github.com/oracle/oracle-database-operator.git
```

If golang compiler is installed on your environment and you've got a public container registry then you can compile the operator, upload to the registry and use it

```bash

cd oracle-database-operator 
make generate
make manifests
make install
make docker-build IMG=<public_container_registry>/operator:latest

make operator-yaml IMG=<public_container_registry>operator:latest
```

> **NOTE:** The last make executions recreates the **oracle-database-operator.yaml** with the  **image:** parameter pointing to your public container registry. If you don't have a golang compilation environment you can use the **oracle-database-operator.yaml** provided in the github distribution. Check [operator installation documentation](../installation/OPERATOR_INSTALLATION_README.md ) for more details.

> <span style="color:red"> **NOTE:** If you are using oracle-container-registry make sure to accept the license agreement otherwise the operator image pull fails. </span>
----
#### Upload webhook certificates <a name="webhook"></a>

```bash
kubectl apply -f https://github.com/jetstack/cert-manager/releases/latest/download/cert-manager.yaml
```

#### Create the dboperator <a name="dboperator"></a>

```bash
cd oracle-database-operator
/usr/bin/kubectl apply -f oracle-database-operator.yaml
```
+ Check the status of the operator

```bash
/usr/bin/kubectl get pods -n oracle-database-operator-system
NAME                                                           READY   STATUS    RESTARTS   AGE
oracle-database-operator-controller-manager-557ff6c659-g7t66   1/1     Running   0          10s
oracle-database-operator-controller-manager-557ff6c659-rssmj   1/1     Running   0          10s
oracle-database-operator-controller-manager-557ff6c659-xpswv   1/1     Running   0          10s

```
----
#### Create secret for container registry

+ Make sure to login to your container registry and then create the secret for you container registry.  

```bash
docker login **<public-container_registry>**
/usr/bin/kubectl create secret generic container-registry-secret --from-file=.dockerconfigjson=/home/oracle/.docker/config.json --type=kubernetes.io/dockerconfigjson -n oracle-database-operator-system
```

+ Check secret 

```bash
kubectl get secret -n oracle-database-operator-system
NAME                        TYPE                             DATA   AGE
container-registry-secret   kubernetes.io/dockerconfigjson   1      19s
webhook-server-cert         kubernetes.io/tls 
```
----
#### Build ords immage <a name="ordsimage"></a>

+ Build the ords image, downloading ords software is no longer needed; just build the image and push it to your repository

```bash
cd oracle-database-operator/ords
docker build -t oracle/ords-dboper:latest .
```

[example of execution](./BuildImage.log)
+ Login to your container registry and push the ords image. 

```bash 
docker tag <public-container-registry>/ords-dboper:latest
docker push <public-container-registry>/ords-dboper:latest
```
[example of execution](./ImagePush.log)

----
#### Database Configuration

+ Configure Database

Connect as sysdba and execute the following script in order to create the required ords accounts.

```sql
ALTER SESSION SET "_oracle_script"=true;
DROP USER <CDB_ADMIN_USER> cascade;
CREATE USER <CDB_ADMIN_USER> IDENTIFIED BY <CDB_ADMIN_PASS> CONTAINER=ALL ACCOUNT UNLOCK;
GRANT SYSOPER TO <CDB_ADMIN_USER> CONTAINER = ALL;
GRANT SYSDBA TO <CDB_ADMIN_USER> CONTAINER = ALL;
GRANT CREATE SESSION TO <CDB_ADMIN_USER> CONTAINER = ALL;
```
----
#### Create CDB secret 

+ Create secret for CDB connection 

```bash
kubectl apply -f cdb_secret.yaml -n oracle-database-operator-system

```
Exmaple: **cdb_secret.yaml**

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: cdb1-secret
  namespace: oracle-database-operator-system
type: Opaque
data:
  ords_pwd: "encoded value"
  sysadmin_pwd: "encoded value"
  cdbadmin_user: "encoded value"
  cdbadmin_pwd: "encoded value"
  webserver_user: "encoded value"
  webserver_pwd: "encoded value"

```
Use **base64** command to encode/decode username and password in the secret file as shown in the following example 

- encode
```bash
echo  "ThisIsMyPassword" |base64 -i
VGhpc0lzTXlQYXNzd29yZAo=
```
- decode 
```bash
 echo "VGhpc0lzTXlQYXNzd29yZAo=" | base64 --decode
ThisIsMyPassword

```


>Note that we do not have to create webuser on the database. 

+ Check secret:

```bash
kubectl get secret -n oracle-database-operator-system
NAME                        TYPE                             DATA   AGE
cdb1-secret                 Opaque                           6      7s <---
container-registry-secret   kubernetes.io/dockerconfigjson   1      2m17s
webhook-server-cert         kubernetes.io/tls                3      4m55s
```

>**TIPS:** Use the following commands to analyze contents of an existing secret  ```bash kubectl   get secret <secret name> -o yaml -n <namespace_name>```
----
#### Create Certificates

+ Create certificates: At this stage we need to create certificates on our local machine and upload into kubernetes cluster by creating new secrets.



```text

       +-----------+
       |  openssl  |
       +-----------+
            |
            |
       +-----------+
       | tls.key   |
       | tls.crt   +------------+
       | ca.crt    |            |
       +-----------+            |
                                |
       +------------------------|---------------------------+
       |KUBERNETES       +------+--------+                  |
       |CLUSTER      +---|kubernet secret|---+              |
       |             |   +---------------+   |              |
       |             |                       |              |
       |  +----------+---+     https      +--+----------+   |
       |  |ORDS CONTAINER|<-------------->|  PDB/POD    |   |
       |  +----------+---+                +-------------+   |
       |  cdb.yaml   |                     pdb.yaml         |
       +-------------|--------------------------------------+
                     |
                     |
               +-----------+
               | DB SERVER |
               +-----------+

```

```bash

genrsa -out <certauth> 2048
openssl req -new -x509 -days 365 -key <certauth> -subj "/C=CN/ST=GD/L=SZ/O=oracle, Inc./CN=oracle Root CA" -out <certfile>
openssl req -newkey rsa:2048 -nodes -keyout <keyfile> -subj "/C=CN/ST=GD/L=SZ/O=oracle, Inc./CN=<cdb-dev>-ords" -out server.csr
/usr/bin/echo "subjectAltName=DNS:<cdb-dev>-ords,DNS:www.example.com" > extfile.txt
openssl x509 -req -extfile extfile.txt -days 365 -in server.csr -CA <certfile> -CAkey <certauth> -CAcreateserial -out <certfile>

kubectl create secret tls db-tls --key="<keyfile>" --cert="<certfile>"  -n oracle-database-operator-system
kubectl create secret generic db-ca --from-file=<certfile> -n oracle-database-operator-system

```

[example of execution:](./openssl_execution.log)


----
#### Apply cdb.yaml

+ Create ords container 

```bash
/usr/bin/kubectl apply -f cdb.yaml  -n oracle-database-operator-system
```
Example: **cdb.yaml**

```yaml
apiVersion: database.oracle.com/v1alpha1
kind: CDB
metadata:
  name: <cdb-dev>
  namespace: oracle-database-operator-system
spec:
  cdbName: "<cdbname>"
  dbServer: "<host_name>" or <scan_name> 
  dbPort: <oracle_port>
  ordsImage: "<public_container_registry>/ords-dboper:.latest"
  ordsImagePullPolicy: "Always"
  serviceName: <service_name>
  replicas: 1
  sysAdminPwd:
    secret:
      secretName: "cdb1-secret"
      key: "sysadmin_pwd"
  ordsPwd:
    secret:
      secretName: "cdb1-secret"
      key: "ords_pwd"
  cdbAdminUser:
    secret:
      secretName: "cdb1-secret"
      key: "cdbadmin_user"
  cdbAdminPwd:
    secret:
      secretName: "cdb1-secret"
      key: "cdbadmin_pwd"
  webServerUser:
    secret:
      secretName: "cdb1-secret"
      key: "webserver_user"
  webServerPwd:
    secret:
      secretName: "cdb1-secret"
      key: "webserver_pwd"
  cdbTlsKey:
    secret:
      secretName: "db-tls"
      key: "<keyfile>"
  cdbTlsCrt:
    secret:
      secretName: "db-tls"
      key: "<certfile>:"

```
> **Note** if you are working in dataguard environment with multiple sites (AC/DR) specifying the host name (dbServer/dbPort/serviceName) may not be the suitable solution for this kind of configuration, use **dbTnsurl** instead. Specify the whole tns string which includes the hosts/scan list. 

```                         
                        +----------+
                    ____| standbyB |
                    |   | scanB    |   (DESCRIPTION=
 +----------+       |   +----------+      (CONNECT_TIMEOUT=90)
 | primary  |_______|                     (RETRY_COUNT=30)(RETRY_DELAY=10)(TRANSPORT_CONNECT_TIMEOUT=70)
 | scanA    |       |   +----------+      (TRANSPORT_CONNECT_TIMEOUT=10)(LOAD_BALLANCE=ON)
 +----------+       |___| stanbyC  |      (ADDRESS=(PROTOCOL=TCP)(HOST=scanA.testrac.com)(PORT=1521)(IP=V4_ONLY))
                        | scanC    |      (ADDRESS=(PROTOCOL=TCP)(HOST=scanB.testrac.com)(PORT=1521)(IP=V4_ONLY))
                        +----------+      (ADDRESS=(PROTOCOL=TCP)(HOST=scanC.testrac.com)(PORT=1521)(IP=V4_ONLY))
                                             (CONNECT_DATA=(SERVER=DEDICATED)(SERVICE_NAME=TESTORDS)))


   dbtnsurl:((DESCRIPTION=(CONNECT_TIMEOUT=90)(RETRY_COUNT=30)(RETRY_DELAY=10)(TRANSPORT_CONNECT_TIMEOUT=70)(TRANS......
```
     
[example of cdb.yaml](./cdb.yaml)

----
#### CDB - Logs and throuble shutting 

+ Check the status of ords container 

```bash
/usr/bin/kubectl get pods -n oracle-database-operator-system
NAME                                                           READY   STATUS              RESTARTS   AGE
cdb-dev-ords-rs-m9ggp                                          0/1     ContainerCreating   0          67s <-----
oracle-database-operator-controller-manager-557ff6c659-g7t66   1/1     Running             0          11m
oracle-database-operator-controller-manager-557ff6c659-rssmj   1/1     Running             0          11m
oracle-database-operator-controller-manager-557ff6c659-xpswv   1/1     Running             0          11m
```
+ Make sure that the cdb container is running

```bash
/usr/bin/kubectl get pods -n oracle-database-operator-system
NAME                                                           READY   STATUS    RESTARTS   AGE
cdb-dev-ords-rs-dnshz                                          1/1     Running   0          31s
oracle-database-operator-controller-manager-557ff6c659-9bjfl   1/1     Running   0          2m42s
oracle-database-operator-controller-manager-557ff6c659-cx8hd   1/1     Running   0          2m42s
oracle-database-operator-controller-manager-557ff6c659-rq9xs   1/1     Running   0          2m42s
```
+ Check the status of the services

```bash 
kubectl get cdb -n oracle-database-operator-system
NAME      CDB NAME   DB SERVER              DB PORT   REPLICAS   STATUS   MESSAGE
[.....................................................]          Ready
```
+ Use log file to trouble shutting 

```bash
/usr/bin/kubectl logs `/usr/bin/kubectl get pods -n oracle-database-operator-system|grep ords|cut -d ' ' -f 1` -n oracle-database-operator-system
```
[example of execution](./cdb.log)

+ Test REST API from the pod. By querying the metadata catalog you can verify the status of https setting 

```bash
 /usr/bin/kubectl exec -it  `/usr/bin/kubectl get pods -n oracle-database-operator-system|grep ords|cut -d ' ' -f 1` -n oracle-database-operator-system -i -t --  /usr/bin/curl -sSkv -k -X GET https://localhost:8888/ords/_/db-api/stable/metadata-catalog/
```
[example of execution](./testapi.log)

+ Verify the pod environment varaibles
 ```bash 
 kubectl set env pods --all --list -n oracle-database-operator-system 
 ```

+ Connect to cdb pod

```bash
 kubectl exec -it  `kubectl get pods -n oracle-database-operator-system|grep ords|cut -d ' ' -f 1` -n oracle-database-operator-system bash
```
+ Dump ords server configuration 

```bash
/usr/bin/kubectl exec -it  `/usr/bin/kubectl get pods -n oracle-database-operator-system|grep ords|cut -d ' ' -f 1` -n oracle-database-operator-system -i -t --  /usr/local/bin/ords --config /etc/ords/config config list
```
[Example of executions](./ordsconfig.log)

-----
#### Create PDB secret


```bash
/usr/bin/kubectl apply -f pdb.yaml  -n oracle-database-operator-system
```
Exmaple: **pdb_secret.yaml**

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: pdb1-secret
  namespace: oracle-database-operator-system
type: Opaque
data:
  sysadmin_user: "encoded value"
  sysadmin_pwd: "encoded value"
```

+ Check secret creation 

```bash
kubectl get secret -n oracle-database-operator-system
NAME                        TYPE                             DATA   AGE
cdb1-secret                 Opaque                           6      79m
container-registry-secret   kubernetes.io/dockerconfigjson   1      79m
db-ca                       Opaque                           1      78m
db-tls                      kubernetes.io/tls                2      78m
pdb1-secret                 Opaque                           2      79m <---
webhook-server-cert         kubernetes.io/tls                3      79m
```
---
#### Apply pdb yaml file to create pdb 

```bash
/usr/bin/kubectl apply -f pdb.yaml  -n oracle-database-operator-system
```

Example: **pdb.yaml**

```yaml
apiVersion: database.oracle.com/v1alpha1
kind: PDB
metadata:
  name: <pdb>
  namespace: oracle-database-operator-system
  labels:
    cdb: <cdb-dev>
spec:
  cdbResName: "<cdb-dev>"
  cdbName: "<cdb>"
  pdbName: <pdbname>
  adminName:
    secret:
      secretName: "pdb1-secret"
      key: "sysadmin_user"
  adminPwd:
    secret:
      secretName: "pdb1-secret"
      key: "sysadmin_pwd"
  pdbTlsKey:
    secret:
      secretName: "db-tls"
      key: "<keyfile>"
  pdbTlsCrt:
    secret:
      secretName: "db-tls"
      key: "<certfile>"
  pdbTlsCat:
    secret:
      secretName: "db-ca"
      key: "<certauth>"
  fileNameConversions: "NONE"
  totalSize: "1G"
  tempSize: "100M"
  action: "Create"
```

+ Monitor the pdb creation status until message is success

```bash
kubectl get pdbs --all-namespaces=true

  +-----------------------------------------+        +-----------------------------------------+
  | STATUS   MESSAGE                        |______\ |              STATUS  MESSAGE            |
  | Creating Waiting for PDB to be created  |      / |              Ready   Success            |
  +-----------------------------------------+        +-----------------------------------------+

NAMESPACE                         NAME   DBSERVER   CDB NAME   PDB NAME   PDB STATE   PDB SIZE   STATUS   MESSAGE
oracle-database-operator-system   <pdb>  <db_host>  <dbname>   <pdbname>                    1G Creating   Waiting for PDB to be created

[wait sometimes]

kubectl get pdbs --all-namespaces=true
NAMESPACE                         NAME   DBSERVER   CDB NAME   PDB NAME   PDB STATE   PDB SIZE   STATUS   MESSAGE
oracle-database-operator-system   pdb1   <dbhost>   <dbname>   <pdbname>  READ WRITE        1G    Ready   Success
```

Connect to the hosts and verify the PDB creation.

```text
[oracle@racnode1 ~]$ sqlplus '/as sysdba'
[...]
Oracle Database 19c Enterprise Edition Release 19.0.0.0.0 - Production
Version 19.15.0.0.0


SQL> show pdbs

    CON_ID CON_NAME                       OPEN MODE  RESTRICTED
---------- ------------------------------ ---------- ----------
         2 PDB$SEED                       READ ONLY  NO
         3 PDBDEV                         READ WRITE NO

```
Check controller log to debug pluggable database life cycle actions in case of problem

```bash
kubectl logs -f $(kubectl get pods -n oracle-database-operator-system|grep oracle-database-operator-controller|head -1|cut -d ' ' -f 1) -n oracle-database-operator-system
```

---
#### Other actions

Configure and use other yaml files to perform pluggable database life cycle managment action **modify_pdb_open.yaml**  **modify_pdb_close.yaml**

> **Note** sql command *"alter pluggable database <pdbname> open instances=all;"* acts  only on closed databases, so you want get any oracle error in case of execution against an pluggable database already opened




[1]:https://docs.oracle.com/en/database/oracle/oracle-rest-data-services/22.2/ordig/installing-and-configuring-oracle-rest-data-services.html#GUID-E9625FAB-9BC8-468B-9FF9-443C88D76FA1:~:text=Table%202%2D2%20Command%20Options%20for%20Command%2DLine%20Interface%20Installation

[2]:https://docs.oracle.com/en/database/oracle/oracle-rest-data-services/22.2/ordig/installing-and-configuring-oracle-rest-data-services.html#GUID-E9625FAB-9BC8-468B-9FF9-443C88D76FA1:~:text=Table%202%2D2%20Command%20Options%20for%20Command%2DLine%20Interface%20Installation

[3]:https://docs.oracle.com/en/database/oracle/oracle-rest-data-services/22.2/ordig/installing-and-configuring-oracle-rest-data-services.html#GUID-DAA027FA-A4A6-43E1-B8DD-C92B330C2341:~:text=%2D%2Ddb%2Dservicename%20%3Cstring%3E

[adminuser]:https://docs.oracle.com/en/database/oracle/oracle-rest-data-services/22.2/ordig/installing-and-configuring-oracle-rest-data-services.html#GUID-A9AED253-4EEC-4E13-A0C4-B7CE82EC1C22:~:text=Table%202%2D6%20Command%20Options%20for%20Uninstall%20CLI

[public_user]:https://docs.oracle.com/en/database/oracle/oracle-rest-data-services/22.2/ordig/using-multitenant-architecture-oracle-rest-data-services.html#GUID-E64A141A-A71F-4979-8D33-C5F8496D3C19:~:text=Preinstallation%20Tasks%20for%20Oracle%20REST%20Data%20Services%20CDB%20Installation

[key]:https://docs.oracle.com/en/database/oracle/oracle-rest-data-services/22.2/ordig/about-REST-configuration-files.html#GUID-006F916B-8594-4A78-B500-BB85F35C12A0:~:text=standalone.https.cert.key

[cr]:https://docs.oracle.com/en/database/oracle/oracle-rest-data-services/22.2/ordig/about-REST-configuration-files.html#GUID-006F916B-8594-4A78-B500-BB85F35C12A0

[cdbadminpwd]:https://docs.oracle.com/en/database/oracle/oracle-rest-data-services/22.2/ordig/about-REST-configuration-files.html#GUID-006F916B-8594-4A78-B500-BB85F35C12A0:~:text=Table%20C%2D1%20Oracle%20REST%20Data%20Services%20Configuration%20Settings

[pwdstdin]:https://docs.oracle.com/en/database/oracle/oracle-rest-data-services/22.2/ordig/installing-and-configuring-oracle-rest-data-services.html#GUID-88479C84-CAC1-4133-A33E-7995A645EC05:~:text=default%20database%20pool.-,2.1.4.1%20Understanding%20Command%20Options%20for%20Command%2DLine%20Interface%20Installation,-Table%202%2D2

[http]:https://docs.oracle.com/en/database/oracle/oracle-rest-data-services/22.2/ordig/installing-and-configuring-oracle-rest-data-services.html#GUID-BEECC057-A8F5-4EAB-B88E-9828C2809CD8:~:text=Example%3A%20delete%20%5B%2D%2Dglobal%5D-,user%20add,-Add%20a%20user

[dbtnsurl]:https://docs.oracle.com/en/database/oracle/oracle-rest-data-services/22.2/ordig/installing-and-configuring-oracle-rest-data-services.html#GUID-A9AED253-4EEC-4E13-A0C4-B7CE82EC1C22