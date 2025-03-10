<span style="font-family:Liberation mono; font-size:0.9em; line-height: 1.1em">


# UNPLUG - PLUG - CLONE 

- [UNPLUG - PLUG - CLONE](#unplug---plug---clone)
    - [INTRODUCTION](#introduction)
    - [UNPLUG DATABASE](#unplug-database)
    - [PLUG DATABASE](#plug-database)
    - [CLONE PDB](#clone-pdb)

### INTRODUCTION

> &#9758; The examples of this folder are based on single namespace   **oracle-database-operator-system**

This page explains how to plug and unplug database a pdb; it assumes that you have already configured a pluggable database (see [usecase01](../usecase01/README.md)).  Check yaml parameters in the CRD tables in the main [README](../README.md) file.

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
  [ secret sections ]
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
  pdbState: "CLOSE"
  modifyOption: "IMMEDIATE"
  action: "Modify"
  [secret section]
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
  [secrets section]
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
  action: "Clone"
  [secret section]

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

You can use unplug and plug database with TDE; in order to do that you have to specify a key store path and create new kubernets secret for TDE using the following yaml file. **tde_secrete.yaml**. 

```yaml
#tde_secret
apiVersion: v1
kind: Secret
metadata:
  name: tde1-secret
  namespace: oracle-database-operator-system
type: Opaque
data:
  tdepassword: "...."
  tdesecret:   "...."
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
      secretName: 
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



