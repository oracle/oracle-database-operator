<span style="font-family:Liberation mono; font-size:0.9em; line-height: 1.1em">


# UNPLUG - PLUG - CLONE 

- [UNPLUG - PLUG - CLONE](#unplug---plug---clone)
    - [INTRODUCTION](#introduction)
    - [UNPLUG DATABASE](#unplug-database)
    - [PLUG DATABASE](#plug-database)
    - [CLONE PDB](#clone-pdb)

### INTRODUCTION

> &#9758; The examples of this folder are based on single namespace   **oracle-database-operator-system**

This page explains how to plug and unplug database a pdb; it assumes that you have already configured a pluggable database (see [usecase01](../usecase01/README.md)) 
The following table reports the parameters required to configure and use oracle multi tenant controller for pluggable database lifecycle management.

| yaml file parameters            	| value  	| description /ords parameter                     |
|--------------	|---------------------------	|-------------------------------------------------|
| dbserver     	| <db_host\> or <scan_name>   | [--db-hostname][1]                              |
| dbTnsurl      | <tns connect descriptor\>   | [--db-custom-url/db.customURL][dbtnsurl]        |
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
| xmlFileName   | <xml file path\>            | path for the unplug and plug operation          |
| srcPdbName    | <source db\>                | name of the database to be cloned               |
| fileNameConversions | <file name conversion\> | used for database cloning                     |
| tdeKeystorePath     | <TDE keystore path is required if the tdeExport flag is set to true\>   |  [tdeKeystorePath][tdeKeystorePath] |
| tdeExport           | <BOOLEAN\>              | [tdeExport] |
| tdeSecret           | <TDE secret is required if the tdeExport flag is set to true\>  | [tdeSecret][tdeSecret] |
| tdePassword         | <TDE password for unplug operations only\>  | [tdeSecret][tdeSecret] | 

```text


                                                        +--------------------------------+
     UNPLUG PDB                         PLUG PDB        |  CLONE PDB                     |
                                                        |                                |
     +-----------+                      +-----------+   | +-----------+  +----------+    |
     | PDB       |                      | PDB       |   | | PDB       |  |CLONED PDB|    |
     +----+------+                      +----+------+   | +----+------+  +----------+    |
          |                                  |          |      |            |            |
+---->  UNPLUG  -----+                +--> PLUG         |    CLONE ---------+            |
|         |          |                |      |          |      |                         |
|    +----+------+   |                | +----+------+   | +----+------+                  |
|    | Container |   |                | | Container |   | | Container |                  |
|    |           |   |                | |           |   | |           |                  |
|    +-----------+   |                | +-----------+   | +-----------+                  |
|                    |                |                 |                                |
|             +------+----+           |                 | kubectk apply -f pdb_clone.yaml|
|             |           |           |                 |                                |
|      +------|-----------|--------+  |                 +--------------------------------+
|      | +----+----+   +--+------+ |  |
|      | |xml file |   |DB FILES | |--+
|      | +---------+   +---------+ |  |
|      +---------------------------+  |
|                                     |
|                                     |
+- kubectl apply -f pdb_unplug.yaml   |
                                      |
   kubectl apply -f pdb_plug.yaml-----+
```

### UNPLUG DATABASE 

Use the following command to check kubernets pdb resources. Note that the output of the commands can be tailored to meet your needs. Just check the structure of pdb resource  **kubectl get pdbs -n oracle-database-operator-system -o=json** and modify the script accordingly. For the sake of simplicity put this command in a single script **checkpdbs.sh**.

```bash
kubectl get pdbs -n oracle-database-operator-system -o=jsonpath='{range .items[*]}
{"\n==================================================================\n"}
{"CDB="}{.metadata.labels.cdb}
{"K8SNAME="}{.metadata.name}
{"PDBNAME="}{.spec.pdbName}
{"OPENMODE="}{.status.openMode}
{"ACTION="}{.status.action}
{"MSG="}{.status.msg}
{"\n"}{end}'
```

We assume that the pluggable database pdbdev is already configured and opened in read write mode  

```bash
./checkpdbs.sh
==================================================================
CDB=cdb-dev
K8SNAME=pdb1
PDBNAME=pdbdev
OPENMODE=READ WRITE
ACTION=CREATE
MSG=Success

```

Prepare a new yaml file **pdb_unplug.yaml**  to unplug the pdbdev database. Make sure that the  path of the xml file is correct and check the existence of all the required secrets. Do not reuse an existing xml files.

```yaml
# Copyright (c) 2022, Oracle and/or its affiliates. All rights reserved.
# Licensed under the Universal Permissive License v 1.0 as shown at http://oss.oracle.com/licenses/upl.
#
#pdb_unplug.yaml
apiVersion: database.oracle.com/v1alpha1
kind: PDB
metadata:
  name: pdb1
  namespace: oracle-database-operator-system
  labels:
    cdb: cdb-dev
spec:
  cdbResName: "cdb-dev"
  cdbNamespace: "oracle-database-operator-system"
  cdbName: "DB12"
  pdbName: "pdbdev"
  xmlFileName: "/tmp/pdbunplug.xml"
  action: "Unplug"
  pdbTlsKey:
    secret:
      secretName: "db-tls"
      key: "tls.key"
  pdbTlsCrt:
    secret:
      secretName: "db-tls"
      key: "tls.crt"
  pdbTlsCat:
    secret:
      secretName: "db-ca"
      key: "ca.crt"
  webServerUser:
    secret:
      secretName: "pdb1-secret"
      key: "webserver_user"
  webServerPwd:
    secret:
      secretName: "pdb1-secret"
      key: "webserver_pwd"

```

Close the pluggable database by applying the following yaml file **pdb_close.yaml** 

```yaml
# Copyright (c) 2022, Oracle and/or its affiliates. All rights reserved.
# Licensed under the Universal Permissive License v 1.0 as shown at http://oss.oracle.com/licenses/upl.
#
#pdb_close.yaml
apiVersion: database.oracle.com/v1alpha1
kind: PDB
metadata:
  name: pdb1
  namespace: oracle-database-operator-system
  labels:
    cdb: cdb-dev
spec:
  cdbResName: "cdb-dev"
  cdbNamespace: "oracle-database-operator-system"
  cdbName: "DB12"
  pdbName: "pdbdev"
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
      key: "tls.key"
  pdbTlsCrt:
    secret:
      secretName: "db-tls"
      key: "tls.crt"
  pdbTlsCat:
    secret:
      secretName: "db-ca"
      key: "ca.crt"
  pdbState: "CLOSE"
  modifyOption: "IMMEDIATE"
  action: "Modify"
```

```bash
kubectl apply -f pdb_close.yaml
pdb.database.oracle.com/pdb1 configured

sh checkpdbs.sh
==================================================================
CDB=cdb-dev
K8SNAME=pdb1
PDBNAME=pdbdev
OPENMODE=MOUNTED
ACTION=MODIFY
MSG=Success
```
After that apply the unplug file **pdb_unplug.yaml** ; The resource is no longer available once the unplug operation is completed.  

```bash
kubectl apply -f pdb_unplug.yaml
pdb.database.oracle.com/pdb1 configured

sh checkpdbs.sh
==================================================================
CDB=cdb-dev
K8SNAME=pdb1
PDBNAME=pdbdev
OPENMODE=MOUNTED
ACTION=MODIFY
MSG=Waiting for PDB to be unplugged
```

Check kubernets log files and the database alert log 

```text
/usr/bin/kubectl logs -f  pod/`/usr/bin/kubectl get pods -n oracle-database-operator-system|grep oracle-database-operator-controller|head -1|cut -d ' ' -f 1` -n oracle-database-operator-system
[...]
base-oracle-com-v1alpha1-pdb", "UID": "6f469423-85e5-4287-94d5-3d91a04b621e", "kind": "database.oracle.com/v1alpha1, Kind=PDB", "resource": {"group":"database.oracle.com","version":"v1alpha1","resource":"pdbs"}}
2023-01-03T14:04:05Z    INFO    pdb-webhook     ValidateUpdate-Validating PDB spec for : pdb1
2023-01-03T14:04:05Z    INFO    pdb-webhook     validateCommon  {"name": "pdb1"}
2023-01-03T14:04:05Z    INFO    pdb-webhook     Valdiating PDB Resource Action : UNPLUG
2023-01-03T14:04:05Z    DEBUG   controller-runtime.webhook.webhooks     wrote response  {"webhook": "/validate-database-oracle-com-v1alpha1-pdb", "code": 200, "reason": "", "UID": "6f469423-85e5-4287-94d5-3d91a04b621e", "allowed": true}


[database alert log]
Domain Action Reconfiguration complete (total time 0.0 secs)
Completed: ALTER PLUGGABLE DATABASE "pdbdev" UNPLUG INTO '/tmp/pdbunplug.xml'
DROP PLUGGABLE DATABASE "pdbdev" KEEP DATAFILES
2023-01-03T14:04:05.518845+00:00
Deleted Oracle managed file +DATA/DB12/F146D9482AA0260FE0531514000AB1BC/TEMPFILE/temp.266.1125061101
2023-01-03T14:04:05.547820+00:00
Stopped service pdbdev
Completed: DROP PLUGGABLE DATABASE "pdbdev" KEEP DATAFILES

```


login to the server and check xml file existence. Verify the datafile path on the ASM filesystem.

```bash
ls -ltr /tmp/pdbunplug.xml
-rw-r--r--. 1 oracle asmadmin 8007 Jan  3 14:04 /tmp/pdbunplug.xml
[..]
cat /tmp/pdbunplug.xml |grep path
      <path>+DATA/DB12/F146D9482AA0260FE0531514000AB1BC/DATAFILE/system.353.1125061021</path>
      <path>+DATA/DB12/F146D9482AA0260FE0531514000AB1BC/DATAFILE/sysaux.328.1125061021</path>
      <path>+DATA/DB12/F146D9482AA0260FE0531514000AB1BC/DATAFILE/undotbs1.347.1125061021</path>
      <path>+DATA/DB12/F146D9482AA0260FE0531514000AB1BC/TEMPFILE/temp.266.1125061101</path>
      <path>+DATA/DB12/F146D9482AA0260FE0531514000AB1BC/DATAFILE/undo_2.318.1125061021</path>
[..]
asmcmd ls -l +DATA/DB12/F146D9482AA0260FE0531514000AB1BC/DATAFILE/system.353.1125061021
Type      Redund  Striped  Time             Sys  Name
DATAFILE  UNPROT  COARSE   JAN 03 14:00:00  Y    system.353.1125061021
```

### PLUG DATABASE

Prepare a new yaml file **pdb_plug.yaml** to plug the database back into the container. 

```yaml 
# Copyright (c) 2022, Oracle and/or its affiliates. All rights reserved.
# Licensed under the Universal Permissive License v 1.0 as shown at http://oss.oracle.com/licenses/upl.
#
# pdb_plug.yaml
apiVersion: database.oracle.com/v1alpha1
kind: PDB
metadata:
  name: pdb1
  namespace: oracle-database-operator-system
  labels:
    cdb: cdb-dev
spec:
  cdbResName: "cdb-dev"
  cdbNamespace: "oracle-database-operator-system"
  cdbName: "DB12"
  pdbName: "pdbdev"
  xmlFileName: "/tmp/pdbunplug.xml"
  fileNameConversions: "NONE"
  sourceFileNameConversions: "NONE"
  copyAction: "MOVE"
  totalSize: "1G"
  tempSize: "100M"
  action: "Plug"
  pdbTlsKey:
    secret:
      secretName: "db-tls"
      key: "tls.key"
  pdbTlsCrt:
    secret:
      secretName: "db-tls"
      key: "tls.crt"
  pdbTlsCat:
    secret:
      secretName: "db-ca"
      key: "ca.crt"

```
Apply **pdb_plug.yaml** 

```bash
kubectl apply -f pdb_plug.yaml
[...]
sh checkpdbs.sh
==================================================================
CDB=cdb-dev
K8SNAME=pdb1
PDBNAME=pdbdev
OPENMODE=
ACTION=
MSG=Waiting for PDB to be plugged
[...]
sh checkpdbs.sh
==================================================================
CDB=cdb-dev
K8SNAME=pdb1
PDBNAME=pdbdev
OPENMODE=READ WRITE
ACTION=PLUG
MSG=Success
```

Check kubernets log files and the database alert log

```text
/usr/bin/kubectl logs -f  pod/`/usr/bin/kubectl get pods -n oracle-database-operator-system|grep oracle-database-operator-controller|head -1|cut -d ' ' -f 1` -n oracle-database-operator-system

2023-01-03T14:33:51Z    INFO    pdb-webhook     ValidateCreate-Validating PDB spec for : pdb1
2023-01-03T14:33:51Z    INFO    pdb-webhook     validateCommon  {"name": "pdb1"}
2023-01-03T14:33:51Z    INFO    pdb-webhook     Valdiating PDB Resource Action : PLUG
2023-01-03T14:33:51Z    INFO    pdb-webhook     PDB Resource : pdb1 successfully validated for Action : PLUG
2023-01-03T14:33:51Z    DEBUG   controller-runtime.webhook.webhooks     wrote response  {"webhook": "/validate-database-oracle-com-v1alpha1-pdb", "code": 200, "reason": "", "UID": "fccac7ba-7540-42ff-93b2-46675506a098", "allowed": true}
2023-01-03T14:34:16Z    DEBUG   controller-runtime.webhook.webhooks     received request        {"webhook": "/mutate-database-oracle-com-v1alpha1-pdb", "UID": "766dadcc-aeea-4a80-bc17-e957b4a44d3c", "kind": "database.oracle.com/v1alpha1, Kind=PDB", "resource": {"group":"database.oracle.com","version":"v1alpha1","resource":"pdbs"}}
2023-01-03T14:34:16Z    INFO    pdb-webhook     Setting default values in PDB spec for : pdb1
2023-01-03T14:34:16Z    DEBUG   controller-runtime.webhook.webhooks     wrote response  {"webhook": "/mutate-database-oracle-com-v1alpha1-pdb", "code": 200, "reason": "", "UID": "766dadcc-aeea-4a80-bc17-e957b4a44d3c", "allowed": true}

[database alert log]
...
All grantable enqueues granted
freeing rdom 3
freeing the fusion rht of pdb 3
freeing the pdb enqueue rht
Domain Action Reconfiguration complete (total time 0.0 secs)
Completed: CREATE PLUGGABLE DATABASE "pdbdev"
                USING '/tmp/pdbunplug.xml'
                SOURCE_FILE_NAME_CONVERT=NONE
                MOVE
                FILE_NAME_CONVERT=NONE
                STORAGE UNLIMITED       TEMPFILE REUSE

2023-01-03T14:35:41.500186+00:00
ALTER PLUGGABLE DATABASE "pdbdev" OPEN READ WRITE INSTANCES=ALL
2023-01-03T14:35:41.503482+00:00
PDBDEV(3):Pluggable database PDBDEV opening in read write
PDBDEV(3):SUPLOG: Initialize PDB SUPLOG SGA, old value 0x0, new value 0x18
PDBDEV(3):Autotune of undo retention is turned on
...
```
### CLONE PDB

Prepare and apply a new yaml file **pdb_clone.yaml** to clone the existing pluggable database.

```yaml
#pdb_clone.yaml
apiVersion: database.oracle.com/v1alpha1
kind: PDB
metadata:
  name: pdb2
  namespace: oracle-database-operator-system
  labels:
    cdb: cdb-dev
spec:
  cdbResName: "cdb-dev"
  cdbNamespace: "oracle-database-operator-system"
  cdbName: "DB12"
  pdbName: "pdb2-clone"
  srcPdbName: "pdbdev"
  fileNameConversions: "NONE"
  totalSize: "UNLIMITED"
  tempSize: "UNLIMITED"
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
      key: "tls.key"
  pdbTlsCrt:
    secret:
      secretName: "db-tls"
      key: "tls.crt"
  pdbTlsCat:
    secret:
      secretName: "db-ca"
      key: "ca.crt"
  action: "Clone"

```
```bash
kubectl apply -f pdb_clone.yaml
pdb.database.oracle.com/pdb2 created
[oracle@mitk01 https.ords.22]$ sh checkpdbs.sh
==================================================================
CDB=cdb-dev
K8SNAME=pdb1
PDBNAME=pdbdev
OPENMODE=READ WRITE
ACTION=PLUG
MSG=Success
==================================================================
CDB=cdb-dev
K8SNAME=pdb2
PDBNAME=pdb2-clone
OPENMODE=
ACTION=
MSG=Waiting for PDB to be cloned
[...]
[.wait sometimes..]
 sh checkpdbs.sh
==================================================================
CDB=cdb-dev
K8SNAME=pdb1
PDBNAME=pdbdev
OPENMODE=READ WRITE
ACTION=PLUG
MSG=Success
==================================================================
CDB=cdb-dev
K8SNAME=pdb2
PDBNAME=pdb2-clone
OPENMODE=READ WRITE
ACTION=CLONE
MSG=Success
```
log info 

```text
[kubernets log]
2023-01-03T15:13:31Z    INFO    pdb-webhook      - asClone : false
2023-01-03T15:13:31Z    INFO    pdb-webhook      - getScript : false
2023-01-03T15:13:31Z    DEBUG   controller-runtime.webhook.webhooks     wrote response  {"webhook": "/mutate-database-oracle-com-v1alpha1-pdb", "code": 200, "reason": "", "UID": "7c17a715-7e4e-47d4-ad42-dcb37526bb3e", "allowed": true}
2023-01-03T15:13:31Z    DEBUG   controller-runtime.webhook.webhooks     received request        {"webhook": "/validate-database-oracle-com-v1alpha1-pdb", "UID": "11e0d49c-afaa-47ac-a301-f1fdd1e70173", "kind": "database.oracle.com/v1alpha1, Kind=PDB", "resource": {"group":"database.oracle.com","version":"v1alpha1","resource":"pdbs"}}
2023-01-03T15:13:31Z    INFO    pdb-webhook     ValidateCreate-Validating PDB spec for : pdb2
2023-01-03T15:13:31Z    INFO    pdb-webhook     validateCommon  {"name": "pdb2"}
2023-01-03T15:13:31Z    INFO    pdb-webhook     Valdiating PDB Resource Action : CLONE
2023-01-03T15:13:31Z    INFO    pdb-webhook     PDB Resource : pdb2 successfully validated for Action : CLONE
2023-01-03T15:13:31Z    DEBUG   controller-runtime.webhook.webhooks     wrote response  {"webhook": "/validate-database-oracle-com-v1alpha1-pdb", "code": 200, "reason": "", "UID": "11e0d49c-afaa-47ac-a301-f1fdd1e70173", "allowed": true}

[database alert log]
Domain Action Reconfiguration complete (total time 0.0 secs)
2023-01-03T15:15:00.670436+00:00
Completed: CREATE PLUGGABLE DATABASE "pdb2-clone" FROM "pdbdev"
                STORAGE UNLIMITED
                TEMPFILE REUSE
                FILE_NAME_CONVERT=NONE
ALTER PLUGGABLE DATABASE "pdbdev" CLOSE IMMEDIATE INSTANCES=ALL
2023-01-03T15:15:00.684271+00:00
PDBDEV(3):Pluggable database PDBDEV closing
PDBDEV(3):JIT: pid 8235 requesting stop
PDBDEV(3):Buffer Cache flush started: 3
PDBDEV(3):Buffer Cache flush finished: 3

```
### UNPLUG AND PLUG WITH TDE


<span style="color:red">

> &#9888; __WARNING FOR THE TDE USERS__ &#9888; According to the [ords documentation](https://docs.oracle.com/en/database/oracle/oracle-database/21/dbrst/op-database-pdbs-pdb_name-post.html) the plug and unplug operation with tde is supported only if ords runs on the same host of the database which is not the case of operator where ords runs on an isolated pods. Do not use pdb controller for unplug and plug operation with tde in production environments.   

</span>

You can use unplug and plug database with TDE; in order to do that you have to specify a key store path and create new kubernets secret for TDE using the following yaml file. **tde_secrete.yaml**. The procedure to unplug and plug database does not change apply the same file. 

```yaml
#tde_secret
apiVersion: v1
kind: Secret
metadata:
  name: tde1-secret
  namespace: oracle-database-operator-system
type: Opaque
data:
  tdepassword: "d2VsY29tZTEK"
  tdesecret:   "bW1hbHZlenoK"
```

```bash 
kubectl apply -f tde_secret.yaml
```

The file to unplug and plug database with TDE are the following 


```yaml
#pdb_unplugtde.yaml
apiVersion: database.oracle.com/v1alpha1
kind: PDB
metadata:
  name: pdb1
  namespace: oracle-database-operator-system
  labels:
    cdb: cdb-dev
spec:
  cdbResName: "cdb-dev"
  cdbNamespace: "oracle-database-operator-system"
  cdbName: "DB12"
  pdbName: "pdbdev"
  adminName:
    secret:
      secretName: pdb1-secret
      key: "sysadmin_user"
  adminPwd:
    secret:
      secretName: pdb1-secret
      key: "sysadmin_pwd"
  pdbTlsKey:
    secret:
      secretName: "db-tls"
      key: "tls.key"
  pdbTlsCrt:
    secret:
      secretName: "db-tls"
      key: "tls.crt"
  pdbTlsCat:
    secret:
      secretName: "db-ca"
      key: "ca.crt"
  tdePassword:
    secret:
      secretName: "tde1-secret"
      key: "tdepassword"
  tdeSecret:
    secret:
      secretName: "tde1-secret"
      key: "tdesecret"
  totalSize: 1G
  tempSize: 1G
  unlimitedStorage: true
  reuseTempFile: true
  fileNameConversions: NONE
  action: "Unplug"
  xmlFileName: "/home/oracle/unplugpdb.xml"
  tdeExport: true
```

```yaml
#pdb_plugtde.ymal
kind: PDB
metadata:
  name: pdb1
  namespace: oracle-database-operator-system
  labels:
    cdb: cdb-dev
spec:
  cdbResName: "cdb-dev"
  cdbNamespace: "oracle-database-operator-system"
  cdbName: "DB12"
  pdbName: "pdbdev"
  adminName:
    secret:
      secretName: pdb1-secret
      key: "sysadmin_user"
  adminPwd:
    secret:
      secretName: pdb1-secret
      key: "sysadmin_pwd"
  pdbTlsKey:
    secret:
      secretName: "db-tls"
      key: "tls.key"
  pdbTlsCrt:
    secret:
      secretName: "db-tls"
      key: "tls.crt"
  pdbTlsCat:
    secret:
      secretName: "db-ca"
      key: "ca.crt"
  tdePassword:
    secret:
      secretName: "tde1-secret"
      key: "tdepassword"
  tdeSecret:
    secret:
      secretName: "tde1-secret"
      key: "tdesecret"
  totalSize: 1G
  tempSize: "100M"
  unlimitedStorage: true
  reuseTempFile: true
  fileNameConversions: NONE
  sourceFileNameConversions: "NONE"
  copyAction: "MOVE"
  action: "Plug"
  xmlFileName: /home/oracle/unplugpdb.xml
  tdeImport: true
  tdeKeystorePath: /home/oracle/keystore

```


</span>



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
                                                   
[tdeKeystorePath]:https://docs.oracle.com/en/database/oracle/oracle-rest-data-services/21.4/orrst/op-database-pdbs-pdb_name-post.html

[tdeSecret]:https://docs.oracle.com/en/database/oracle/oracle-database/19/sqlrf/ADMINISTER-KEY-MANAGEMENT.html#GUID-E5B2746F-19DC-4E94-83EC-A6A5C84A3EA9
