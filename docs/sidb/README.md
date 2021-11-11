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

  Provision a new database instance by specifying appropriate values for the attributes in the the example `.yaml` file, and running the following command:

  ```sh
  $ kubectl create -f singleinstancedatabase.yaml
  
    singleinstancedatabase.database.oracle.com/sidb-sample created
  ```

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
  
## Kind OracleRestDataService

The Oracle Database Operator creates the OracleRestDataService (ORDS) kind as a custom resource that enables RESTful API access to the Oracle Database in K8s

* ### OracleRestDataService Sample YAML
  
    For the use cases detailed below a sample .yaml file is available at
    [config/samples/sidb/oraclerestdataservice.yaml](config/samples/sidb/oraclerestdataservice.yaml)

    **Note:** The `adminPassword` , `ordsPassword` fields of the above `oraclerestdataservice.yaml` yaml contains secrets for authenticating Single Instance Database and for ORDS user with roles `SQL Administrator , System Administrator , SQL Developer , oracle.dbtools.autorest.any.schema` respectively . These secrets gets delete after the first deployed ORDS pod REST enables the database successfully for security reasons.  

* ### List OracleRestDataServices

    ```sh
    $ kubectl get oraclerestdataservice -o name

      oraclerestdataservice.database.oracle.com/ords-sample 

    ```

* ### Quick Status

    ```sh
    $ kubectl get oraclerestdataservice ords-sample

    NAME          STATUS    DATABASE      DATABASE API URL                     DATABASE ACTIONS URL
    ords-sample   Healthy   sidb-sample   https://144.25.121.118:8443/ords/     https://144.25.121.118:8443/ords/sql-developer
    ```

* ### Detailed Status

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
        Cluster Db API URL:    https://ords21c-1.default:8443/ords/
        Database Actions URL:  https://144.25.121.118:8443/ords/sql-developer
        Database API URL:      https://144.25.121.118:8443/ords/
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
        Service IP:       144.25.121.118
        Status:           Healthy

    ```

## REST Enable Database

  Provision a new ORDS instance by specifying appropriate values for the attributes in the the sample .yaml file and executing the following command . ORDS is installed in the root container(CDB) of SingleInstanceDatabase to enable PDB Lifecycle Management .

  ```sh
  $ kubectl create -f oraclerestdataservice.yaml
  
    oraclerestdataservice.database.oracle.com/ords-sample created
  ```

* ### Creation Status
  
    Creating a new ORDS instance takes a while. ORDS is open for connections when the 'status' status returns a "Healthy"

     ```sh
    $ kubectl get oraclerestdataservice/ords-sample --template={{.status.status}}

      Healthy
    ```

* ### REST Endpoints

    External and internal (running in Kubernetes pods) clients can access the REST Endpoints using .status.databaseApiUrl and .status.clusterDbApiUrl respectively in the following command .

    ```sh
    $ kubectl get oraclerestdataservice/ords-sample --template={{.status.databaseApiUrl}}

      https://144.25.121.118:8443/ords/
    ```

    All the REST Endpoints can be found at <https://docs.oracle.com/en/database/oracle/oracle-database/21/dbrst/rest-endpoints.html>

    There are two basic approaches for authentication to the REST Endpoints. Certain APIs are specific about which authentication method they will accept.

* #### Default Administrator

  ORDS User with role "SQL Administrator" , `.spec.ordsUser` (defaults to ORDS_PUBLIC_USER if not mentioned in yaml) credentials are required to call certain REST Endpoints .

  This user has also given the additional roles `System Administrator , SQL Developer , oracle.dbtools.autorest.any.schema` .

  This user can now be used to authenticate
  * PDB Lifecycle Management APIs
  * Any Protected AutoRest Enabled Object APIs
  * Database Actions of any REST Enabled Schema

* #### ORDS Enabled Schema

  Alternatively one can use an ORDS enabled schema. Access to the certain APIs will use the credentials of the ORDS enabled schema , which are defined in the `.spec.restEnableSchemas` atrribute in sample yaml .

  This schema authentication can be used to authorise database actions of this schema

  Note :  Browser may not prompt for credentials while accessing certain REST Endpoints and in such case one can use clients like curl and pass credentials while calling REST Endpoints .

* #### Some Usecases

* ##### PDB Lifecycle Management

    The Oracle REST Data Services (ORDS) database API allows us to manage the lifecycle of PDBs via REST web service calls.
    Few APIs :
  * List PDB's

      ```sh
        `$ curl -s -k -X GET -u 'ORDS_PUBLIC_USER:<.spec.ordsPassword>' https://144.25.121.118:8443/ords/_/db-api/stable/database/pdbs/ | python -m json.tool`
      ```

  * Create PDB

      Create a file called "pdbsample.json" with the following contents.

      ```sh
        { "method": "CREATE",
        "adminName": "pdbsample_admin",
        "adminPwd": "<Admin Password for PDB>",
        "pdb_name": "pdbsample",
        "fileNameConversions": "('/opt/oracle/oradata/<.spec.databaseRef.sid>/pdbseed/', '/opt/oracle/oradata/<.spec.databaseRef.sid>/pdbsample/')",
        "reuseTempFile": true,
        "totalSize": "10G",
        "tempSize": "100M",
        "getScript": false
        }
      ```

      Execute the follwing API to run the above json script to create pdb.

      ```sh
        $ curl -k --request POST --url https://144.25.121.118:8443/ords/_/db-api/latest/database/pdbs/ \
        --user 'ORDS_PUBLIC_USER:<.spec.ordsPassword>' \
        --header 'content-type: application/json' \
        --data @pdbsample.json
      ```

  * Open/Close PDB

      ```sh
        $ curl -k --request POST --url https://144.25.121.118:8443/ords/_/db-api/latest/database/pdbs/pdbsample/status \
        --user 'ORDS_PUBLIC_USER:<.spec.ordsPassword>'\
        --header 'content-type: application/json' \
        --data '  {
          "state": "OPEN/CLOSE",
          "modifyOption": "NORMAL"
        } '
      ```

  * Clone PDB

      Create a file called "pdbclone.json" with the following contents.

      ```sh
        {
          "method": "CLONE",
          "clonePDBName": "pdbclone",
          "fileNameConversions": "('/opt/oracle/oradata/<.spec.databaseRef.sid>/pdbsample/', '/opt/oracle/oradata/<.spec.databaseRef.sid>/pdbclone/')",
          "unlimitedStorage": true,
          "reuseTempFile": true,
          "totalSize": "UNLIMITED",
          "tempSize": "UNLIMITED"
        } 
      ```

      Execute the follwing API to run the above json script to clone pdb from pdbsample.

      ```sh
        $ curl -k --request POST --url https://144.25.121.118:8443/ords/_/db-api/latest/database/pdbs/pdbsample/ \
        --user 'ORDS_PUBLIC_USER:<.spec.ordsPassword>' \
        --header 'content-type: application/json' \
        --data @pdbclone.json
      ```

      More REST APIs for PDB Lifecycle Management can be found at [https://docs.oracle.com/en/database/oracle/oracle-database/21/dbrst/api-pluggable-database-lifecycle-management.html](https://docs.oracle.com/en/database/oracle/oracle-database/21/dbrst/api-pluggable-database-lifecycle-management.html)

* #### REST Enabled SQL

    The REST Enabled SQL functionality allows REST calls to send DML, DDL and scripts to any REST enabled schema by exposing the same SQL engine used in SQL Developer and SQLcl.

  * Run a Script

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

      Execute the follwing API to run the above script.

      ```sh
        curl -s -k -X "POST" "https://144.25.121.118:8443/ords/<.spec.restEnableSchemas[].pdb>/<.spec.restEnableSchemas[].urlMapping>/_/sql" \
        -H "Content-Type: application/sql" \
        -u '<.spec.restEnableSchemas[].schema>:<.spec.ordsPassword>' \
        -d @/tmp/table.sql
      ```

  * Basic Call

      Fetch all entries from 'DEPT' table by calling the following API

      ```sh
        curl -s -k -X "POST" "https://144.25.121.118:8443/ords/<.spec.restEnableSchemas[].pdb>/<.spec.restEnableSchemas[].urlMapping>/_/sql" \
        -H "Content-Type: application/sql" \
        -u '<.spec.restEnableSchemas[].schema>:<.spec.ordsPassword>' \
        -d $'select * from dept;' | python -m json.tool
      ```

    **NOTE:** `.spec.restEnableSchema[].urlMapping` is optional and is defaulted to `.spec.restEnableSchema[].schema`

* #### Data Pump

    The Oracle REST Data Services (ORDS) database API allows user to create Data Pump export and import jobs via REST web service calls.

    REST APIs for Data Pump Jobs can be found at [https://docs.oracle.com/en/database/oracle/oracle-database/21/dbrst/op-database-datapump-jobs-post.html](https://docs.oracle.com/en/database/oracle/oracle-database/21/dbrst/op-database-datapump-jobs-post.html)

* ### Database Actions

    Database Actions is a web-based interface that uses Oracle REST Data Services to provide development, data tools, administration and monitoring features for Oracle Database .

  * To use Database Actions, one must sign in as a database user whose schema has been REST-enabled.
  * This can be done by specifying appropriate values for the `.spec.restEnableSchemas` attributes details in the sample yaml [config/samples/sidb/oraclerestdataservice.yaml](config/samples/sidb/oraclerestdataservice.yaml) which are needed for authorising Database Actions.
  * Schema will be created (if not exists) with username as `.spec.restEnableSchema[].schema` and password as `.spec.ordsPassword.`.
  * UrlMapping `.spec.restEnableSchema[].urlMapping` is optional and is defaulted to `.spec.restEnableSchema[].schema`.

    Database Actions can be accessed via browser using `.status.databaseActionsUrl` in the following command

    ```sh
    $ kubectl get oraclerestdataservice/ords-sample --template={{.status.databaseActionsUrl}}

      https://144.25.121.118:8443/ords/sql-developer
    ```

    Sign in to Database Actions using
  * First Page: \
    PDB Name: `.spec.restEnableSchema[].pdb` \
    Username: `.spec.restEnableSchema[].urlMapping`

  * Second Page: \
    Username: `.spec.restEnableSchema[].schema` \
    Password: `.spec.ordsPassword`

    ![database-actions-home](/images/sidb/database-actions-home.png)

    More info on Database Actions can be found at <https://docs.oracle.com/en/database/oracle/sql-developer-web/21.2/index.html>

* ### Application Express

    Oracle Application Express (APEX) is a low-code development platform that enables developers to build scalable, secure enterprise apps, with world-class features, that can be deployed anywhere.

    Using APEX, developers can quickly develop and deploy compelling apps that solve real problems and provide immediate value. Developers won't need to be an expert in a vast array of technologies to deliver sophisticated solutions. Focus on solving the problem and let APEX take care of the rest.

  **Download APEX:**
  * Download latest version of apex using <https://download.oracle.com/otn_software/apex/apex-latest.zip>
  * Copy apex-latest.zip file to the location '/opt/oracle/oradata' of SingleInstanceDatabase pod .
  * Use `kubectl cp </full/path/to/local/apex-latest.zip> <anyone-of-running-sidbpodname>:/opt/oracle/oradata` to copy apex-latest.zip

  **Install APEX:**
  * Set `.spec.installApex` to true in [config/samples/sidb/singleinstancedatabase.yaml](config/samples/sidb/singleinstancedatabase.yaml)
  * Status of SIDB turns to 'Updating' during apex installation and turns 'Healthy' after successful installation. You can also check status using below cmd

    ```sh
    $ kubectl get singleinstancedatabase sidb-sample -o "jsonpath=[{.status.apexInstalled}]"

      [true]
    ```

  * To access APEX , You need to configure APEX with ORDS

  **Configure APEX with ORDS:**
  * Set `.spec.apexPassword.secretName` to a non-null string in [config/samples/sidb/oraclerestdataservice.yaml](config/samples/sidb/oraclerestdataservice.yaml)
  * This is used as a common password for APEX_PUBLIC_USER, APEX_REST_PUBLIC_USER, APEX_LISTENER and Apex administrator (username: ADMIN) mapped to secretKey
  * Status of ORDS turns to 'Updating' during apex configuration and turns 'Healthy' after successful configuration. You can also check status using below cmd

    ```sh
    $ kubectl get oraclerestdataservice ords-sample -o "jsonpath=[{.status.apexConfigured}]"

      [true]
    ```

  * If you configure APEX after ORDS is installed, ORDS pods will be deleted and recreated

    Application Express can be accessed via browser using `.status.databaseApiUrl` in the following command .\

    ```sh
    $ kubectl get oraclerestdataservice/ords-sample --template={{.status.databaseApiUrl}}

      https://144.25.121.118:8888/ords/
    ```

    Sign in to Administration servies using \
    workspace: `INTERNAL` \
    username: `ADMIN` \
    password: `.spec.apexPassword`

    ![application-express-admin-home](/images/sidb/application-express-admin-home.png)

    **NOTE**
    * Apex Administrator for pdbs other than `.spec.databaseRef.pdbName` has to be created Manually

    More Info on creating Apex Administrator can be found at [APEX_UTIL.CREATE_USER]<https://docs.oracle.com/en/database/oracle/application-express/21.1/aeapi/CREATE_USER-Procedure.html#GUID-95721E36-4DAB-4BCA-A6F3-AC2BACC52A66>
  
  **Uninstall APEX:**
  * Set `.spec.installApex` to false in [config/samples/sidb/singleinstancedatabase.yaml](config/samples/sidb/singleinstancedatabase.yaml)
  * If you install APEX again, re-configure APEX with ORDS again.

    More info on Application Express can be found at <https://apex.oracle.com/en/>

* ### Multiple Replicas
  
    Currently only single replica mode is supported

* ### Patch Attributes

    The following attributes cannot be patched post SingleInstanceDatabase instance Creation : databaseRef, loadBalancer, image, ordsPassword, adminPassword, apexPassword.

  * A schema can be rest enabled or disabled by setting the `.spec.restEnableSchemas[].enable` to true or false respectively in ords sample .yaml file and apply using the kubectl apply command or edit/patch commands. This requires `.spec.ordsPassword` secret.
