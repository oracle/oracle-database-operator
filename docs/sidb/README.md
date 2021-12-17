# Managing Oracle Single Instance Databases with Oracle Database Operator for Kubernetes

Oracle Database Operator for Kubernetes (the operator) includes the Single Instance Database Controller that enables provisioning, cloning, and patching of Oracle Single Instance Databases on Kubernetes. The following sections explain the setup and functionality of the operator

* [Prerequisites](#prerequisites)
* [Kind SingleInstanceDatabase Resource](#kind-singleinstancedatabase-resource)
* [Provision New Database](#provision-new-database)
* [Clone Existing Database](#clone-existing-database)
* [Patch/Rollback Database](#patchrollback-database)

## Prerequisites

Oracle strongly recommends that you follow the [Prerequisites](./SIDB_PREREQUISITES.md).

## Kind SingleInstanceDatabase Resource

  The Oracle Database Operator creates the SingleInstanceDatabase kind as a custom resource that enables Oracle Database to be managed as a native Kubernetes object

* ### SingleInstanceDatabase Sample YAML
  
  For the use cases detailed below a sample .yaml file is available at
  * Enterprise, Standard Editions
  [config/samples/sidb/singleinstancedatabase.yaml](./../../config/samples/sidb/singleinstancedatabase.yaml)

  **Note:** The `adminPassword` field of the above `singleinstancedatabase.yaml` yaml contains a secret for Single Instance Database creation (Provisioning a new database or cloning an existing database). This secret gets deleted after the database pod becomes ready for security reasons.  

  More info on creating Kubernetes Secret available at [https://kubernetes.io/docs/tasks/configmap-secret/managing-secret-using-kubectl/](https://kubernetes.io/docs/tasks/configmap-secret/managing-secret-using-kubectl/)

* ### List Databases

  ```sh
  $ kubectl get singleinstancedatabases -o name

    singleinstancedatabase.database.oracle.com/sidb-sample  
    singleinstancedatabase.database.oracle.com/sidb-sample-clone

  ```

* ### Quick Status
  
  ```sh
  $ kubectl get singleinstancedatabase sidb-sample

  NAME           EDITION        STATUS      ROLE         VERSION                  CLUSTER CONNECT STR              CONNECT STR               OEM EXPRESS URL
  sidb-sample    Enterprise     Healthy     PRIMARY      19.3.0.0.0 (29517242)    sidb-sample.default:1521/ORCL1   144.25.10.119:1521/ORCL   https://144.25.10.119:5500/em
  ```

* ### Detailed Status

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
          Last Transition Time:   2021-06-29T15:45:33Z
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
          Last Transition Time:   2021-06-30T11:16:58Z
          Message:                no reconcile errors
          Observed Generation:    3
          Reason:                 LastReconcileCycleCompleted
          Status:                 True
          Type:                   ReconcileComplete
        Connect String:          144.25.10.119:1521/ORCL1C
        Datafiles Created:       true
        Datafiles Patched:       true
        Edition:                 Enterprise
        Flash Back:              true
        Force Log:               false
        Oem Express URL:         https://144.25.10.119:5500/em
        Pdb Name:                orclpdb1
        Release Update:          19.11.0.0.0 (32545013)
        Replicas:                2
        Role:                    PRIMARY
        Sid:                     ORCL1C
        Status:                  Healthy
    Events:
        Type     Reason                 Age                    From                    Message
        ----     ------                 ----                   ----                    -------
        Normal   Database Pending       35m (x2 over 35m)      SingleInstanceDatabase  Waiting for database pod to be ready
        Normal   Database Creating      27m (x24 over 34m)     SingleInstanceDatabase  Waiting for database to be ready
        Normal   Database Ready         22m                    SingleInstanceDatabase  database open on pod sidb-sample-clone-133ol scheduled on node 10.0.10.6
        Normal   Datapatch Pending      21m                    SingleInstanceDatabase  datapatch execution pending
        Normal   Datapatch Executing    20m                    SingleInstanceDatabase  datapatch begin execution
        Normal   Datapatch Done         8s                     SingleInstanceDatabase  Datapatch from 19.3.0.0.0 to 19.11.0.0.0 : SUCCESS

  ```

## Provision New Database

  - Quickly Provision a new database instance on **minikube** using the [singleinstancedatabase_minikube.yaml](../../config/samples/sidb/singleinstancedatabase_minikube.yaml) using following one command.

  ```sh
  $ kubectl create -f singleinstancedatabase_minikube.yaml
  
    singleinstancedatabase.database.oracle.com/sidb-sample created
  ```

  - Provision a new database instance on any k8s cluster by specifying appropriate values for the attributes in the the example `.yaml` file, and running the following command:

  ```sh
  $ kubectl create -f singleinstancedatabase.yaml
  
    singleinstancedatabase.database.oracle.com/sidb-sample created
  ```

  **NOTE:** Make sure you have created the required `.spec.adminPassword` [secret](https://kubernetes.io/docs/tasks/configmap-secret/managing-secret-using-kubectl/)) and `.spec.persistence` [persistent volume](https://kubernetes.io/docs/concepts/storage/persistent-volumes/)


  
* ### Creation Status
  
 Creating a new database instance takes a while. When the 'status' status returns the response "Healthy", the Database is open for connections. 
 
  ```sh
$ kubectl get singleinstancedatabase sidb-sample -o "jsonpath={.status.status}"
   
  Healthy
```
  


* ### Connection Information

  External and internal (running in Kubernetes pods) clients can connect to the database using .status.connectString and .status.clusterConnectString
  respectively in the following command

  ```sh
  $ kubectl get singleinstancedatabase sidb-sample -o "jsonpath={.status.connectString}"

    144.25.10.119:1521/ORCL
  ```

  The Oracle Database inside the container also has Oracle Enterprise Manager Express configured. To access OEM Express, start the browser and follow the URL:

  ```sh
  $ kubectl get singleinstancedatabase sidb-sample -o "jsonpath={.status.oemExpressUrl}"

    https://144.25.10.119:5500/em
  ```

* ### Update Database Config
  
  The following database parameters can be updated post database creation: flashBack, archiveLog, forceLog. Change their attribute values and apply using
  kubectl apply or edit/patch commands . Enable archiveLog before turning ON flashBack . Turn OFF flashBack before disabling the archiveLog

  ```sh
  $ kubectl patch singleinstancedatabase sidb-sample --type merge -p '{"spec":{"forceLog": true}}' 

    singleinstancedatabase.database.oracle.com/sidb-sample patched
  ```

* #### Database Config Status

  Check the Database Config Status using the following command

  ```sh
  $ kubectl get singleinstancedatabase sidb-sample -o "jsonpath=[{.status.archiveLog}, {.status.flashBack}, {.status.forceLog}]"

    [true, true, true]
  ```

* ### Update Initialization Parameters

  The following database initialization parameters can be updated post database creation: `sgaTarget, pgaAggregateTarget, cpuCount, processes`. Change their attribute values and apply using kubectl apply or edit/patch commands.

  **NOTE**
  * `sgaTarget` should be in range [sga_min_size, sga_max_size], else initialization parameter `sga_target` would not be updated to specified `sgaTarget`.

* ### Multiple Replicas
  
  Multiple database pod replicas can be provisioned when the persistent volume access mode is ReadWriteMany. Database is open and mounted by one of the replicas. Other replicas will have instance started but not mounted and serve to provide quick cold fail-over in case the active pod dies. Update the replica attribute in the .yaml and apply using the kubectl apply command or edit/patch commands

  Note: This functionality requires the [K8s extension](https://github.com/oracle/docker-images/tree/main/OracleDatabase/SingleInstance/extensions/k8s)
        Pre-built images from container-registry.oracle.com include the K8s extension

* ### Patch Attributes

  The following attributes cannot be patched post SingleInstanceDatabase instance Creation : sid, edition, charset, pdbName, cloneFrom.

  ```sh
  $ kubectl --type=merge -p '{"spec":{"sid":"ORCL1"}}' patch singleinstancedatabase sidb-sample 

    The SingleInstanceDatabase "sidb-sample" is invalid: spec.sid: Forbidden: cannot be changed
  ```

* #### Patch Persistence Volume Claim

  Persistence Volume Claim (PVC) can be patched post SingleInstanceDatabase instance Creation . This will **delete all the database pods, PVC** and new database pods are created using the new PVC .

  ```sh
  $ kubectl --type=merge -p '{"spec":{"persistence":{"accessMode":"ReadWriteMany","size":"110Gi","storageClass":""}}}' patch singleinstancedatabase sidb-sample 

    singleinstancedatabase.database.oracle.com/sidb-sample patched
  ```

* #### Patch Service

  Service can be patched post SingleInstanceDatabase instance Creation . This will **replace the Service with a new type** .
  * NodePort     - '{"spec":{"loadBalancer": false}}'
  * LoadBalancer - '{"spec":{"loadBalancer": true }}'

  ```sh
  $ kubectl --type=merge -p '{"spec":{"loadBalancer": false}}' patch singleinstancedatabase sidb-sample 

    singleinstancedatabase.database.oracle.com/sidb-sample patched
  ```

## Clone Existing Database

  Quickly create copies of your existing database using this cloning functionality. A cloned database is an exact, block-for-block copy of the source database.
  This is much faster than creating a fresh new database and copying over the data.
  
  To clone, specify the source database reference as value for the cloneFrom attribute in the sample .yaml.  
  The source database must have archiveLog mode set to true.

  ```sh
  $ grep 'cloneFrom:' singleinstancedatabase.yaml
  
    cloneFrom: "sidb-sample"
    
  $ kubectl create -f singleinstancedatabase.yaml

    singleinstancedatabase.database.oracle.com/sidb-sample-clone created
  ```

  Note: The clone database can specify a database image different from the source database. In such cases, cloning is supported only between databases of the same major release.
  
## Patch/Rollback Database

  Databases running in your cluster and managed by this operator can be patched or rolled back between release updates of the same major release. To patch databases, specify an image of the higher release update, and to roll back, specify an image of the lower release update.
  
  Patched Oracle Docker images can be built using this [patching extension](https://github.com/oracle/docker-images/tree/main/OracleDatabase/SingleInstance/extensions/patching)

  ```sh
  kubectl --type=merge -p '{"spec":{"image":{"pullFrom":"patched-image:tag","pullSecrets":"pull-secret"}}}' patch singleinstancedatabase sidb-sample

  singleinstancedatabase.database.oracle.com/sidb-sample patched

  ```

* ### Patch existing Database

  Edit and apply the `singleinstancedatabase.yaml` file of the database resource/object by specifying a new release update for image attributes. The database pods will be restarted with the new release update image. For minimum downtime, ensure that you have mutiple replicas of the database pods running.
  
* ### Clone and Patch Database
  
  Clone your source database using the method of [cloning existing database](README.md#clone-existing-database) and specify a new release image for the cloned database. Use this method to enusure there are no patching related issues impacting your database performance/functionality
  
* ### Datapatch status

  Patching/Rollback operations are complete when the datapatch tool completes patching or rollback of the data files. Check the data files patching status
  and current release update version using the following commands

  ```sh
  $ kubectl get singleinstancedatabase sidb-sample -o "jsonpath={.status.datafilesPatched}"

    true
    
  $ kubectl get singleinstancedatabase sidb-sample -o "jsonpath={.status.releaseUpdate}"

    19.3.0.0.0 (29517242)
  ```
  
