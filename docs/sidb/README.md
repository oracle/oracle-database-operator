# Managing Oracle Single Instance Databases with Oracle Database Operator for Kubernetes

Oracle Database Operator for Kubernetes (`OraOperator`) includes the Single Instance Database Controller, which enables provisioning, cloning, and patching of Oracle Single Instance Databases on Kubernetes. It also enables configuring the database for Oracle REST Data Services with Oracle APEX development platform. The following sections explain the setup and functionality of the operator

  * [Prerequisites](#prerequisites)
  * [SingleInstanceDatabase Resource](#singleinstancedatabase-resource)
    * [Create a Database](#create-a-database)
      * [New Database](#new-database)
      * [Pre-built Database](#pre-built-database)
      * [XE Database](#xe-database)
    * [Connecting to Database](#connecting-to-database)
    * [Database Persistence (Storage) Configuration Options](#database-persistence-storage-configuration-options)
      * [Dynamic Persistence](#dynamic-persistence)
      * [Static Persistence](#static-persistence)
    * [Configuring a Database](#configuring-a-database)
      * [Configure ArchiveLog, Flashback and ForceLog](#configure-archivelog-flashback-and-forcelog)
      * [Change Init Parameters](#change-init-parameters)
    * [Clone a Database](#clone-a-database)
    * [Patch a Database](#patch-a-database)
    * [Delete a Database](#delete-a-database)
    * [Advanced Database Configurations](#advanced-database-configurations)
      * [Run Database with Multiple Replicas](#run-database-with-multiple-replicas)
      * [Setup Database with LoadBalancer](#setup-database-with-loadbalancer)
  * [OracleRestDataService Resource](#oraclerestdataservice-resource)
    * [REST Enable a Database](#rest-enable-a-database)
      * [Provision ORDS](#provision-ords)
      * [Database API](#database-api)
      * [Advanced Usages](#advanced-usages)
        * [Oracle Data Pump](#oracle-data-pump)
        * [REST Enabled SQL](#rest-enabled-sql)
        * [Database Actions](#database-actions)
    * [APEX Installation](#apex-installation)
    * [Delete ORDS](#delete-ords)
  * [Maintenance Operations](#maintenance-operations)

## Prerequisites

Oracle strongly recommends that you follow the [prerequisites](./PREREQUISITES.md).

## SingleInstanceDatabase Resource

The Oracle Database Operator creates the `SingleInstanceDatabase` as a custom resource. Doing this enables Oracle Database to be managed as a native Kubernetes object. We will refer `SingleInstanceDatabase` resource as Database from now onwards.

### Resource Details

#### Database List
To list databases, use the following command as an example, where the database names are `sidb-sample` and `sidb-sample-clone`, which are the names we will use as database names in command examples:

```sh
$ kubectl get singleinstancedatabases -o name

  singleinstancedatabase.database.oracle.com/sidb-sample  
  singleinstancedatabase.database.oracle.com/sidb-sample-clone

```

#### Quick Status
To obtain a quick database status, use the following command as an example:

```sh
$ kubectl get singleinstancedatabase sidb-sample

NAME           EDITION      STATUS    VERSION      CONNECT STR                  OEM EXPRESS URL
sidb-sample    Enterprise   Healthy   19.3.0.0.0   10.0.25.54:1521/ORCLCDB      https://10.0.25.54:5500/em
```

#### Detailed Status
To obtain a detailed database status, use the following command as an example:

```sh
$ kubectl describe singleinstancedatabase sidb-sample-clone

  Name:         sidb-sample-clone
  Namespace:    default
  Labels:       <none>
  Annotations:  <none>
  API Version:  database.oracle.com/v1alpha1
  Kind:         SingleInstanceDatabase
  Metadata: ....
  Spec: ....
  Status:
      Cluster Connect String:  sidb-sample-clone.default:1521/ORCL1C
      Conditions:
        Last Transition Time:   (YYYY-MM-DD)T(HH:MM:SS)Z
        Message:                Waiting for database to be ready
        Observed Generation:    2
        Reason:                 LastReconcileCycleQueued
        Status:                 True
        Type:                   ReconcileQueued
        Last Transition Time:   2021-06-30T11:07:56Z
        Message:                processing datapatch execution
        Observed Generation:    3
        Reason:                 LastReconcileCycleBlocked
        Status:                 True
        Type:                   ReconcileBlocked
        Last Transition Time:   (YYYY-MM-DD)T(HH:MM:SS)Z
        Message:                no reconcile errors
        Observed Generation:    3
        Reason:                 LastReconcileCycleCompleted
        Status:                 True
        Type:                   ReconcileComplete
      Connect String:          10.0.25.58:1521/ORCL1C
      Datafiles Created:       true
      Datafiles Patched:       true
      Edition:                 Enterprise
      Flash Back:              true
      Force Log:               false
      Oem Express URL:         https://10.0.25.58:5500/em
      Pdb Name:                orclpdb1
      Release Update:          19.11.0.0.0
      Replicas:                2
      Role:                    PRIMARY
      Sid:                     ORCL1C
      Status:                  Healthy
  Events:
      Type     Reason                 Age                    From                    Message
      ----     ------                 ----                   ----                    -------
      Normal   Database Pending       35m (x2 over 35m)      SingleInstanceDatabase  waiting for database pod to be ready
      Normal   Database Creating      27m (x24 over 34m)     SingleInstanceDatabase  waiting for database to be ready
      Normal   Database Ready         22m                    SingleInstanceDatabase  database open on pod sidb-sample-clone-133ol scheduled on node 10.0.10.6
      Normal   Datapatch Pending      21m                    SingleInstanceDatabase  datapatch execution pending
      Normal   Datapatch Executing    20m                    SingleInstanceDatabase  datapatch begin execution
      Normal   Datapatch Done         8s                     SingleInstanceDatabase  datafiles patched from 19.3.0.0.0 to 19.11.0.0.0 : SUCCESS

```

### Template YAML
  
The template `.yaml` file for Single Instance Database (Enterprise and Standard Editions), including all the configurable options, is available at:
**[config/samples/sidb/singleinstancedatabase.yaml](./../../config/samples/sidb/singleinstancedatabase.yaml)**

**Note:** 
The `adminPassword` field in the above `singleinstancedatabase.yaml` file refers to a secret for the SYS, SYSTEM and PDBADMIN users of the Single Instance Database. This secret is required when you provision a new database, or when you clone an existing database.

Create this secret using the following command as an example:

    kubectl create secret generic admin-secret --from-literal=oracle_pwd=<specify password here>

This command creates a secret named `admin-secret`, with the key `oracle_pwd` mapped to the actual password specified in the command.

### Create a Database

#### New Database

To provision a new database instance on the Kubernetes cluster, use the example **[config/samples/sidb/singleinstancedatabase_create.yaml](../../config/samples/sidb/singleinstancedatabase_create.yaml)**.

1. Log into [Oracle Container Registry](https://container-registry.oracle.com/) and accept the license agreement for the Database image; ignore if you have accepted the license agreement already.

2. If you have not already done so, create an image pull secret for the Oracle Container Registry:

    ```sh
    $ kubectl create secret docker-registry oracle-container-registry-secret --docker-server=container-registry.oracle.com --docker-username='<oracle-sso-email-address>' --docker-password='<oracle-sso-password>' --docker-email='<oracle-sso-email-address>'
      
      secret/oracle-container-registry-secret created
    ```
    This secret can also be created using the docker config file after a successful docker login  
    ```sh
    $ docker login container-registry.oracle.com
    $ kubectl create secret generic oracle-container-registry-secret  --from-file=.dockerconfigjson=.docker/config.json --type=kubernetes.io/dockerconfigjson
    ```
3. Provision a new database instance on the cluster by using the following command:

    ```sh
    $ kubectl apply -f singleinstancedatabase_create.yaml

    singleinstancedatabase.database.oracle.com/sidb-sample created
    ```

**NOTE:** 
- For ease of use, the storage class **oci-bv** is specified in the **[singleinstancedatabase_create.yaml](../../config/samples/sidb/singleinstancedatabase_create.yaml)**. This storage class facilitates dynamic provisioning of the OCI block volumes on the Oracle OKE for persistent storage of the database. The supported access mode for this class is `ReadWriteOnce`. For other cloud providers, you can similarly use their dynamic provisioning storage classes.
- It is beneficial to have the database replica pods more than or equal to the number of available nodes if `ReadWriteMany` access mode is used with the OCI NFS volume. By doing so, the pods get distributed on different nodes and the database image is downloaded on all those nodes. This helps in reducing time for the database fail-over if the active database pod dies.
- Supports Oracle Database Enterprise Edition (19.3.0), and later releases.
- To pull the database image faster from the container registry, so that you can bring up the SIDB instance quickly, you can use the container-registry mirror of the corresponding cluster's region. For example, if the cluster exists in Mumbai region, then you can use the `container-registry-bom.oracle.com` mirror. For more information on container-registry mirrors, follow the link [https://blogs.oracle.com/wim/post/oracle-container-registry-mirrors-in-oracle-cloud-infrastructure](https://blogs.oracle.com/wim/post/oracle-container-registry-mirrors-in-oracle-cloud-infrastructure).
- To update the init parameters like `sgaTarget` and `pgaAggregateTarget`, refer the `initParams` section of the [singleinstancedatabase.yaml](../../config/samples/sidb/singleinstancedatabase.yaml) file.

#### Pre-built Database

To provision a new pre-built database instance, use the sample **[config/samples/sidb/singleinstancedatabase_prebuiltdb.yaml](../../config/samples/sidb/singleinstancedatabase_prebuiltdb.yaml)** file. For example:
```sh
$ kubectl apply -f singleinstancedatabase_prebuiltdb.yaml

  singleinstancedatabase.database.oracle.com/prebuiltdb-sample created
```

This pre-built image includes the data files of the database inside the image itself. As a result, the database startup time of the container is reduced, down to a couple of seconds. The pre-built database image can be very useful in continuous integration/continuous delivery (CI/CD) scenarios, in which databases are used for conducting tests or experiments, and the workflow is simple. 

To build the pre-built database image for the Enterprise/Standard edition, follow these instructions: [Pre-built Database (prebuiltdb) Extension](https://github.com/oracle/docker-images/blob/main/OracleDatabase/SingleInstance/extensions/prebuiltdb/README.md).

#### XE Database
To provision new Oracle Database Express Edition (XE) database, use the sample **[config/samples/sidb/singleinstancedatabase_express.yaml](../../config/samples/sidb/singleinstancedatabase_express.yaml)** file. For example:

      kubectl apply -f singleinstancedatabase_express.yaml

This command pulls the XE image uploaded on the [Oracle Container Registry](https://container-registry.oracle.com/).

**NOTE:**
- Provisioning Oracle Database express edition is supported for release 21c (21.3.0) and later releases.
- For XE database, only single replica mode (i.e. `replicas: 1`) is supported.
- For XE database, you **cannot change** the init parameters i.e. `cpuCount, processes, sgaTarget or pgaAggregateTarget`.

### Connecting to Database

Creating a new database instance takes a while. When the `status` column returns the response `Healthy`, the Database is open for connections.

```sh
$ kubectl get singleinstancedatabase sidb-sample -o "jsonpath={.status.status}"
  
  Healthy
```

Clients can get the connect-string to the CDB from `.status.connectString` and PDB from `.status.pdbConnectString`. For example:

```sh
$ kubectl get singleinstancedatabase sidb-sample -o "jsonpath={.status.connectString}"

  10.0.25.54:1521/ORCL
```
```sh
$ kubectl get singleinstancedatabase sidb-sample -o "jsonpath={.status.pdbConnectString}"

  10.0.25.54:1521/ORCLPDB
```

Use any supported client or SQLPlus to connect to the database using the above connect strings as follows
```sh
$ sqlplus sys/<.spec.adminPassword>@10.0.25.54:1521/ORCL as sysdba

SQL*Plus: Release 19.0.0.0.0 - Production on Wed May 4 16:00:49 2022
Version 19.14.0.0.0

Copyright (c) 1982, 2021, Oracle.  All rights reserved.


Connected to:
Oracle Database 21c Express Edition Release 21.0.0.0.0 - Production
Version 21.3.0.0.0

SQL>
```
**Note:** The `<.spec.adminPassword>` above refers to the database password for SYS, SYSTEM and PDBADMIN users, which in turn represented by `spec` section's `adminPassword` field of the **[config/samples/sidb/singleinstancedatabase.yaml](../config/samples/sidb/../../../../config/samples/sidb/singleinstancedatabase.yaml)** file.

The Oracle Database inside the container also has Oracle Enterprise Manager Express (OEM Express) configured. To access OEM Express, start the browser, and paste in a URL similar to the following example:

```sh
$ kubectl get singleinstancedatabase sidb-sample -o "jsonpath={.status.oemExpressUrl}"

  https://10.0.25.54:5500/em
```

### Database Persistence (Storage) Configuration Options
The database persistence can be achieved in the following two ways:
- Dynamic Persistence Provisioning
- Static Persistence Provisioning

#### Dynamic Persistence
In **Dynamic Persistence Provisioning**, a persistent volume is provisioned by mentioning a storage class. For example, **oci-bv** storage class is specified in the **[singleinstancedatabase_create.yaml](../../config/samples/sidb/singleinstancedatabase_create.yaml)** file. This storage class facilitates dynamic provisioning of the OCI block volumes. The supported access mode for this class is `ReadWriteOnce`. For other cloud providers, you can similarly use their dynamic provisioning storage classes.
                     
**Note:** 
- Generally, the `Reclaim Policy` of such dynamically provisioned volumes is `Delete`. These volumes are deleted when their corresponding database deployment is deleted. To retain volumes, use static provisioning, as explained in the Block Volume Static Provisioning section.
- In **Minikube**, the dynamic persistence provisioning class is **standard**.

#### Static Persistence
In **Static Persistence Provisioning**, you have to create a volume manually, and then use the name of this volume with the `<.spec.persistence.volumeName>` field which corresponds to the `volumeName` field of the persistence section in the **[singleinstancedatabase.yaml](../../config/samples/sidb/singleinstancedatabase.yaml)**. The `Reclaim Policy` of such volume can be set to `Retain`. So, this volume does not get deleted with the deletion of its corresponding deployment. 
For example in **Minikube**, a persistent volume can be provisioned using the sample yaml file below:
```yaml
apiVersion: v1
kind: PersistentVolume
metadata:
  name: db-vol
spec:
  capacity:
    storage: 10Gi
  accessModes:
    - ReadWriteMany
  persistentVolumeReclaimPolicy: Retain
  hostPath:
    path: /data/oradata
```
The persistent volume name (i.e. db-vol) can be mentioned in the `volumeName` field of the **[singleinstancedatabase.yaml](../../config/samples/sidb/singleinstancedatabase.yaml)**. `storageClass` field is not required in this case, and can be left empty.

Static Persistence Provisioning in Oracle Cloud Infrastructure (OCI) is explained in the following subsections:

##### OCI Block Volume Static Provisioning
With block volume static provisioning, you must manually create a block volume resource from the OCI console, and fetch its `OCID`. To create the persistent volume, you can use the following YAML file:
```yaml
apiVersion: v1
kind: PersistentVolume
metadata:
  name: block-vol
spec:
  capacity:
    storage: 1024Gi
  accessModes:
    - ReadWriteOnce
  persistentVolumeReclaimPolicy: Retain
  csi:
    driver: blockvolume.csi.oraclecloud.com
    volumeHandle: <OCID of the block volume>
```

**Note:** OCI block volumes are AD (Availability Domain) specific. Ensure that the database is deployed in the same AD as that of its statically provisioned block volume. In dynamic provisioning, this is done automatically.
To provision the database in a specific AD, uncomment the following line from the **[singleinstancedatabase.yaml](../../config/samples/sidb/singleinstancedatabase.yaml)** file:

```yaml
nodeSelector:
   topology.kubernetes.io/zone: PHX-AD-1
```

##### OCI NFS Volume Static Provisioning
Similar to the block volume static provisioning, you have to manually create a file system resource from the OCI console, and fetch its `OCID, Mount Target and Export Path`. Mention these values in the following YAML file to create the persistent volume:

```yaml
apiVersion: v1
kind: PersistentVolume
metadata:
  name: nfs-vol
spec:
  capacity:
    storage: 1024Gi
  volumeMode: Filesystem
  accessModes:
    - ReadWriteMany
  persistentVolumeReclaimPolicy: Retain
  csi:
    driver: fss.csi.oraclecloud.com
    volumeHandle: "<OCID of the file system>:<Mount Target>/<Export Path>"
```

**Note:** Whenever a mount target is provisioned in OCI, its `Reported Size (GiB)` values are very large. This is visible on the mount target page when logged in to the OCI console. Some applications will fail to install if the results of a space requirements check show too much available disk space. So specify, in gibibytes (GiB), the maximum capacity reported by file systems exported through this mount target. This setting does not limit the actual amount of data you can store.

### Configuring a Database
The `OraOperator` facilitates you to configure the database. Various database configuration options are explained in the following subsections:

#### Configure ArchiveLog, Flashback, and ForceLog
The following database parameters can be updated after the database is created: 

- `flashBack`
- `archiveLog`
- `forceLog`

To change these parameters, change their attribute values, and apply the change by using the 
`kubectl apply` or `kubectl edit/patch` commands. 

**Caution**: Enable `archiveLog` mode before setting `flashback` to `ON`, and set `flashback` to `OFF` before disabling `archiveLog` mode.

For example:

```sh
$ kubectl patch singleinstancedatabase sidb-sample --type merge -p '{"spec":{"forceLog": true}}' 

  singleinstancedatabase.database.oracle.com/sidb-sample patched
```
Check the Database Config Status by using the following command:

```sh
$ kubectl get singleinstancedatabase sidb-sample -o "jsonpath=[{.status.archiveLog}, {.status.flashBack}, {.status.forceLog}]"

  [true, true, true]
```

#### Change Init Parameters

The following database initialization parameters can be updated after the database is created:

- sgaTarget
- pgaAggregateTarget
- cpuCount
- processes. 

Change their attribute values and apply using `kubectl apply` or `kubectl edit/patch` commands.

**NOTE:**
The value for the initialization parameter `sgaTarget` that you provide should be within the range set by [sga_min_size, sga_max_size]. If the value you provide is not in that range, then `sga_target` is not updated to the value you specify for `sgaTarget`.

#### Immutable YAML Attributes

The following attributes cannot be modified after creating the Single Instance Database instance: 

- `sid`
- `edition`
- `charset`
- `pdbName`
- `cloneFrom`

If you attempt to changing one of these attributes, then you receive an error similar to the following:

```sh
$ kubectl --type=merge -p '{"spec":{"sid":"ORCL1"}}' patch singleinstancedatabase sidb-sample 

  The SingleInstanceDatabase "sidb-sample" is invalid: spec.sid: Forbidden: cannot be changed
```

### Clone a Database

To create copies of your existing database quickly, you can use the cloning functionality. A cloned database is an exact, block-for-block copy of the source database. Cloning is much faster than creating a fresh database and copying over the data.

To quickly clone the existing database sidb-sample created above, use the sample **[config/samples/sidb/singleinstancedatabase_clone.yaml](../../config/samples/sidb/singleinstancedatabase_clone.yaml)** file.

**Note**: To clone a database, the source database must have archiveLog mode set to true.

For example:

```sh
  
$ kubectl apply -f singleinstancedatabase_clone.yaml

  singleinstancedatabase.database.oracle.com/sidb-sample-clone created
```

**Note:** The clone database can specify a database image that is different from the source database. In such cases, cloning is supported only between databases of the same major release.
  
### Patch a Database

Databases running in your cluster and managed by the Oracle Database operator can be patched or rolled back between release updates of the same major release. To patch databases, specify an image of the higher release update. To roll back databases, specify an image of the lower release update.

Patched Oracle Docker images can be built by using this [patching extension](https://github.com/oracle/docker-images/tree/main/OracleDatabase/SingleInstance/extensions/patching).

#### Patch

To patch an existing database, edit and apply the **[config/samples/sidb/singleinstancedatabase_patch.yaml](../../config/samples/sidb/singleinstancedatabase_patch.yaml)** file of the database resource/object either by specifying a new release update for image attributes, or by running the following command:

```sh
kubectl --type=merge -p '{"spec":{"image":{"pullFrom":"patched-image:tag","pullSecrets":"pull-secret"}}}' patch singleinstancedatabase sidb-sample

singleinstancedatabase.database.oracle.com/sidb-sample patched

```

After patching is complete, the database pods are restarted with the new release update image. For minimum downtime, ensure that you have multiple replicas of the database pods running before you start the patch operation.

#### Patch after Cloning
    
To clone and patch the database at the same time, clone your source database by using the [cloning existing database](#clone-existing-database) method, and specify a new release image for the cloned database. Use this method to ensure there are no patching related issues impacting your database performance or functionality.
    
#### Datapatch Status

Patching/Rollback operations are complete when the datapatch tool completes patching or rollback of the data files. Check the data files patching status
and current release update version using the following commands

```sh
$ kubectl get singleinstancedatabase sidb-sample -o "jsonpath={.status.datafilesPatched}"

  true
  
$ kubectl get singleinstancedatabase sidb-sample -o "jsonpath={.status.releaseUpdate}"

  19.3.0.0.0
```

#### Rollback
You can roll back to a prior database version by specifying the old image in the `image` field of the **[config/samples/sidb/singleinstancedatabase_patch.yaml](../../config/samples/sidb/singleinstancedatabase_patch.yaml)** file, and applying it by the following command:

```bash
kubectl apply -f singleinstancedatabase_patch.yaml
```

This can also be done using the following command:

```sh
kubectl --type=merge -p '{"spec":{"image":{"pullFrom":"old-image:tag","pullSecrets":"pull-secret"}}}' patch singleinstancedatabase sidb-sample

singleinstancedatabase.database.oracle.com/sidb-sample patched

```

### Delete a Database
Please run the following command to delete the database:

```bash
kubectl delete singleinstancedatabase.database.oracle.com sidb-sample
```
The command above will delete the database pods and associated service.

### Advanced Database Configurations
Some advanced database configuration scenarios are as follows:

#### Run Database with Multiple Replicas
In multiple replicas mode, more than one pod is created for the database. The database is open and mounted by one of the replica pods. Other replica pods have instances started but not mounted, and serve to provide a quick cold fail-over in case the active pod goes down. Multiple replicas are also helpful in [patching](#patch-existing-database) operation. Ensure that you have multiple replicas of the database pods running before you start the patching operation for minimum downtime.

To enable multiple replicas, update the replica attribute in the `.yaml`, and apply by using the `kubectl apply` or `kubectl scale` commands.

**Note:** 
- This functionality requires the [k8s extension](https://github.com/oracle/docker-images/tree/main/OracleDatabase/SingleInstance/extensions/k8s) extended images. The database image from the container registry `container-registry.oracle.com` includes the K8s extension.
- Because Oracle Database Express Edition (XE) does not support [k8s extension](https://github.com/oracle/docker-images/tree/main/OracleDatabase/SingleInstance/extensions/k8s), it does not support multiple replicas.
- If the `ReadWriteOnce` access mode is used, all the replicas will be scheduled on the same node where the persistent volume would be mounted.
- If the `ReadWriteMany` access mode is used, all the replicas will be distributed on different nodes. So, it is recommended to have replicas more than or equal to the number of the nodes as the database image is downloaded on all those nodes. This is beneficial in quick cold fail-over scenario (when the active pod dies) as the image would already be available on that node.

#### Setup Database with LoadBalancer
For the Single Instance Database, the default service is the `NodePort` service. You can enable the `LoadBalancer` service by using `kubectl patch` command.

For example:

```sh
$ kubectl --type=merge -p '{"spec":{"loadBalancer": true}}' patch singleinstancedatabase sidb-sample 

  singleinstancedatabase.database.oracle.com/sidb-sample patched
```

## OracleRestDataService Resource

The Oracle Database Operator creates the `OracleRestDataService` as a custom resource. We will refer `OracleRestDataService` as ORDS from now onwards. Creating ORDS as a custom resource enables the RESTful API access to the Oracle Database in K8s and enables it to be managed as a native Kubernetes object.

### Resource Details

#### ORDS List
To list ORDS services, use the following command: 

```sh
$ kubectl get oraclerestdataservice -o name

  oraclerestdataservice.database.oracle.com/ords-sample 

```

#### Quick Status
To obtain a quick status check of the ORDS service, use the following command:

```sh
$ kubectl get oraclerestdataservice ords-sample

NAME          STATUS      DATABASE         DATABASE API URL                                           DATABASE ACTIONS URL                           APEX URL
ords-sample   Healthy     sidb-sample      https://10.0.25.54:8443/ords/ORCLPDB1/_/db-api/stable/     https://10.0.25.54:8443/ords/sql-developer     https://10.0.25.54:8443/ords/ORCLPDB1/apex

```

#### Detailed Status
To obtain a detailed status check of the ORDS service, use the following command:

```sh
$ kubectl describe oraclerestdataservice ords-sample

  Name:         ords-sample
  Namespace:    default
  Labels:       <none>
  Annotations:  <none>
  API Version:  database.oracle.com/v1alpha1
  Kind:         OracleRestDataService
  Metadata: ...
  Spec: ...
  Status:
    Cluster Db API URL:    https://ords21c-1.default:8443/ords/ORCLPDB1/_/db-api/stable/
    Database Actions URL:  https://10.0.25.54:8443/ords/sql-developer
    Database API URL:      https://10.0.25.54:8443/ords/ORCLPDB1/_/db-api/stable/
    Apex URL:              https://10.0.25.54:8443/ords/ORCLPDB1/apex
    Database Ref:          sidb21c-1
    Image:
      Pull From:     ...
      Pull Secrets:  ...
    Load Balancer:   true
    Ords Installed:  true
    Persistence:
      Access Mode:    ReadWriteMany
      Size:           100Gi
      Storage Class:  
    Service IP:       10.0.25.54
    Status:           Healthy

```

### Template YAML
  
The template `.yaml` file for Oracle Rest Data Services (`OracleRestDataService` kind), including all the configurable options, is available at **[config/samples/sidb/oraclerestdataservice.yaml](config/samples/sidb/oraclerestdataservice.yaml)**.

**Note:**  
- The `adminPassword` and `ordsPassword` fields in the `oraclerestdataservice.yaml` file contains secrets for authenticating the Single Instance Database and the ORDS user with the following roles: `SQL Administrator, System Administrator, SQL Developer, oracle.dbtools.autorest.any.schema`.  
- To build the ORDS image, use the following instructions: [Building Oracle REST Data Services Install Images](https://github.com/oracle/docker-images/tree/main/OracleRestDataServices#building-oracle-rest-data-services-install-images).
- By default, ORDS uses self-signed certificates. To use certificates from the Certificate Authority, the ORDS image needs to be rebuilt after specifying the values of `ssl.cert` and `ssl.cert.key` in the [standalone.properties](https://github.com/oracle/docker-images/blob/main/OracleRestDataServices/dockerfiles/standalone.properties.tmpl) file. After you rebuild the ORDS image, use the rebuilt image in the **[config/samples/sidb/oraclerestdataservice.yaml](config/samples/sidb/oraclerestdataservice.yaml)** file.
- If you want to install ORDS in a [prebuilt database](#provision-a-pre-built-database), make sure to attach the **database persistence** by uncommenting the `persistence` section in the **[config/samples/sidb/singleinstancedatabase_prebuiltdb.yaml](../../config/samples/sidb/singleinstancedatabase_prebuiltdb.yaml)** file, while provisioning the prebuilt database.

### REST Enable a Database

#### Provision ORDS

To quickly provision a new ORDS instance, use the sample **[config/samples/sidb/oraclerestdataservice_create.yaml](../../config/samples/sidb/oraclerestdataservice_create.yaml)** file. For example: 

```sh
$ kubectl apply -f oraclerestdataservice_create.yaml

  oraclerestdataservice.database.oracle.com/ords-sample created
```
After this command completes, ORDS is installed in the container database (CDB) of the Single Instance Database.

#### Creation Status
  
Creating a new ORDS instance takes a while. To check the status of the ORDS instance, use the following command:

```sh
$ kubectl get oraclerestdataservice/ords-sample -o "jsonpath={.status.status}"

  Healthy
```
ORDS is open for connections when the `status` column returns `Healthy`.

#### REST Endpoints

Clients can access the REST Endpoints using `.status.databaseApiUrl` as shown in the following command.

```sh
$ kubectl get oraclerestdataservice/ords-sample -o "jsonpath={.status.databaseApiUrl}"

  https://10.0.25.54:8443/ords/ORCLPDB1/_/db-api/stable/
```

All the REST Endpoints can be found in [_REST APIs for Oracle Database_](https://docs.oracle.com/en/database/oracle/oracle-database/21/dbrst/rest-endpoints.html).

There are two basic approaches for authentication to the REST Endpoints. Certain APIs are specific about which authentication method they will accept.

#### Database API

To call certain REST endpoints, you must use the ORDS_PUBLIC_USER with role `SQL Administrator`, and `.spec.ordsPassword` credentials.

The ORDS user also has the following additional roles: `System Administrator, SQL Developer, oracle.dbtools.autorest.any.schema`.

Use this ORDS user to authenticate the following: 
* Database APIs
* Any Protected AutoRest Enabled Object APIs
* Database Actions of any REST Enabled Schema
  
##### Examples
Some examples for the Database API usage are as follows:
- **Get all Database Components**
    ```sh
    curl -s -k -X GET -u 'ORDS_PUBLIC_USER:<.spec.ordsPassword>' https://10.0.25.54:8443/ords/ORCLPDB1/_/db-api/stable/database/components/ | python -m json.tool
    ```
- **Get all Database Users** 
    ```sh
    curl -s -k -X GET -u 'ORDS_PUBLIC_USER:<.spec.ordsPassword>' https://10.0.25.54:8443/ords/ORCLPDB1/_/db-api/stable/database/security/users/ | python -m json.tool
    ```
- **Get all Tablespaces**
    ```sh
    curl -s -k -X GET -u 'ORDS_PUBLIC_USER:<.spec.ordsPassword>' https://10.0.25.54:8443/ords/ORCLPDB1/_/db-api/stable/database/storage/tablespaces/ | python -m json.tool
    ```
- **Get all Database Parameters**
    ```sh
    curl -s -k -X GET -u 'ORDS_PUBLIC_USER:<.spec.ordsPassword>' https://10.0.25.54:8443/ords/ORCLPDB1/_/db-api/stable/database/parameters/ | python -m json.tool
    ```
- **Get all Feature Usage Statistics**
    ```sh
    curl -s -k -X GET -u 'ORDS_PUBLIC_USER:<.spec.ordsPassword>' https://10.0.25.54:8443/ords/ORCLPDB1/_/db-api/stable/database/feature_usage/ | python -m json.tool
    ```
#### Advanced Usages

##### Oracle Data Pump
The Oracle REST Data Services (ORDS) database API enables you to create Oracle Data Pump export and import jobs by using REST web service calls.

REST APIs for Oracle Data Pump Jobs can be found at [https://docs.oracle.com/en/database/oracle/oracle-database/21/dbrst/op-database-datapump-jobs-post.html](https://docs.oracle.com/en/database/oracle/oracle-database/21/dbrst/op-database-datapump-jobs-post.html).
##### REST Enabled SQL

The REST Enable SQL functionality is available to all the schemas specified in the `.spec.restEnableSchemas` attribute of the sample yaml.
Only these schemas will have access SQL Developer Web Console specified by the Database Actions URL. 

The REST Enabled SQL functionality enables REST calls to send DML, DDL and scripts to any REST enabled schema by exposing the same SQL engine used in SQL Developer and Oracle SQLcl (SQL Developer Command Line).

For example:

**Run a Script:**

Create a file called "/tmp/table.sql" with the following contents.

```sh
  CREATE TABLE DEPT (
    DEPTNO NUMBER(2) CONSTRAINT PK_DEPT PRIMARY KEY,
    DNAME VARCHAR2(14),
    LOC VARCHAR2(13)
  ) ;

  INSERT INTO DEPT VALUES (10,'ACCOUNTING','NEW YORK');
  INSERT INTO DEPT VALUES (20,'RESEARCH','DALLAS');
  INSERT INTO DEPT VALUES (30,'SALES','CHICAGO');
  INSERT INTO DEPT VALUES (40,'OPERATIONS','BOSTON');
  COMMIT;
```

Run the following API to run the script created in the previous example:

```sh
  curl -s -k -X "POST" "https://10.0.25.54:8443/ords/<.spec.restEnableSchemas[].pdbName>/<.spec.restEnableSchemas[].urlMapping>/_/sql" \
  -H "Content-Type: application/sql" \
  -u '<.spec.restEnableSchemas[].schemaName>:<.spec.ordsPassword>' \
  -d @/tmp/table.sql
```

**Basic Call:**

Fetch all entries from 'DEPT' table by calling the following API

```sh
  curl -s -k -X "POST" "https://10.0.25.54:8443/ords/<.spec.restEnableSchemas[].pdbName>/<.spec.restEnableSchemas[].urlMapping>/_/sql" \
  -H "Content-Type: application/sql" \
  -u '<.spec.restEnableSchemas[].schemaName>:<.spec.ordsPassword>' \
  -d $'select * from dept;' | python -m json.tool
```

**NOTE:** `.spec.restEnableSchema[].urlMapping` is optional and is defaulted to `.spec.restEnableSchemas[].schemaName`

##### Database Actions

Database Actions is a web-based interface that uses Oracle REST Data Services to provide development, data tools, administration and monitoring features for Oracle Database.

* To use Database Actions, you must sign in as a database user whose schema has been REST-enabled.
* To enable a schema for REST, you can specify appropriate values for the `.spec.restEnableSchemas` attributes details in the sample `yaml` **[config/samples/sidb/oraclerestdataservice.yaml](config/samples/sidb/oraclerestdataservice.yaml)**, which are needed for authorizing Database Actions.
* Schema are created (if they exist) with the username as `.spec.restEnableSchema[].schema` and password as `.spec.ordsPassword.`.
* UrlMapping `.spec.restEnableSchema[].urlMapping` is optional and is defaulted to `.spec.restEnableSchema[].schema`.

Database Actions can be accessed with a browser by using `.status.databaseActionsUrl`. For example:

```sh
$ kubectl get oraclerestdataservice/ords-sample -o "jsonpath={.status.databaseActionsUrl}"

  https://10.0.25.54:8443/ords/sql-developer
```

To access Database Actions, sign in by using the following code as a database user whose schema has been REST-enabled: 

* First Page: \
PDB Name: `.spec.restEnableSchemas[].pdbName` \
Username: `.spec.restEnableSchemas[].urlMapping`

* Second Page: \
Username: `.spec.restEnableSchemas[].schemaName` \
Password: `.spec.ordsPassword`

![database-actions-home](/images/sidb/database-actions-home.png)

For more information about Database Actions, see: [Oracle Database Actions](https://docs.oracle.com/en/database/oracle/sql-developer-web/21.2/index.html).

### APEX Installation

Oracle APEX is a low-code development platform that enables developers to build scalable, secure enterprise apps, with world-class features that can be deployed anywhere.

Using APEX, developers can quickly develop and deploy compelling apps that solve real problems and provide immediate value. Developers won't need to be an expert in a vast array of technologies to deliver sophisticated solutions. Focus on solving the problem and let APEX take care of the rest.

The `OraOperator` facilitates installation of APEX in the database and also configures ORDS for it. The following section will explain installing APEX with configured ORDS:

* For quick provisioning, use the sample **[config/samples/sidb/oraclerestdataservice_apex.yaml](../../confi/samples/sidb/oraclerestdataservice_apex.yaml)** file. For example:

      kubectl apply -f oraclerestdataservice_apex.yaml

* The APEX Password is used as a common password for `APEX_PUBLIC_USER, APEX_REST_PUBLIC_USER, APEX_LISTENER` and Apex administrator (username: `ADMIN`) mapped to secretKey.
* The status of ORDS turns to `Updating` during APEX configuration, and changes to `Healthy` after successful configuration. You can also check status by using the following command:


  ```sh
  $ kubectl get oraclerestdataservice ords-sample -o "jsonpath={.status.apexConfigured}"

    [true]
  ```

* If you configure APEX after ORDS is installed, then ORDS pods will be deleted and recreated.

Application Express can be accessed via browser using `.status.apexUrl` in the following command.

```sh
$ kubectl get oraclerestdataservice/ords-sample -o "jsonpath={.status.apexUrl}"

  https://10.0.25.54:8443/ords/ORCLPDB1/apex
```

Sign in to Administration services using
workspace: `INTERNAL`
username: `ADMIN`
password: `.spec.apexPassword`

![application-express-admin-home](/images/sidb/application-express-admin-home.png)

**NOTE:**
- By default, the full development environment is initialized in APEX. After deployment, you can change it manually to the runtime environment. To change environments, run the script `apxdevrm.sql` after connecting to the primary database from the ORDS pod as the `SYS` user with `SYSDBA` privilege. For detailed instructions, see: [Converting a Full Development Environment to a Runtime Environment](https://docs.oracle.com/en/database/oracle/application-express/21.2/htmig/converting-between-runtime-and-full-development-environments.html#GUID-B0621B40-3441-44ED-9D86-29B058E26BE9).

### Delete ORDS
- To delete ORDS run the following command:
      
      kubectl delete oraclerestdataservice ords-sample

- You cannot delete the referred Database before deleting its ORDS resource.
- APEX, if installed, also gets uninstalled from the database when ORDS gets deleted.


## Maintenance Operations
If you need to perform some maintenance operations (Database/ORDS) manually, then the procedure is as follows:
1. Use `kubectl exec` to access the pod where you want to perform the manual operation, a command similar to the following:

       kubectl exec -it <pod-name> /bin/bash

2. The important locations, such as ORACLE_HOME, ORDS_HOME, and so on, can be found in the environment, by using the `env` command.

3. Log In to `sqlplus` to perform manual operations by using the following command:       
      
        sqlplus / as sysdba
