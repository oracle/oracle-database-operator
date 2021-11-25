# Oracle On-Premise Database Controller

The On-Premise Database Controller enables provisioning of Oracle Databases (PDBs) both on a Kubernetes cluster or outside of a Kubernetes cluster. The following sections explain the setup and functionality of this controller:

* [Prerequisites for On-Premise Database Controller](ORACLE_ONPREMDB_CONTROLLER_README.md#prerequisites-and-setup)
* [Kubernetes Secrets](ORACLE_ONPREMDB_CONTROLLER_README.md#kubernetes-secrets) 
* [Kubernetes CRD for CDB](ORACLE_ONPREMDB_CONTROLLER_README.md#kubernetes-crd-for-cdb) 
* [Kubernetes CRD for PDB](ORACLE_ONPREMDB_CONTROLLER_README.md#kubernetes-crd-for-pdb)
* [PDB Lifecycle Management Operations](ORACLE_ONPREMDB_CONTROLLER_README.md#pdb-lifecycl-management-operations)
* [Validation and Errors](ORACLE_ONPREMDB_CONTROLLER_README.md#validation-and-errors)

## Prerequisites for On-Premise Database Controller

+ ### Prepare CDB for PDB Lifecycme Management (PDB-LM)

  Pluggable Database management is performed in the Container Database (CDB) and includes create, clone, plug, unplug and delete operations.
  You cannot have an ORDS enabled schema in the container database. To perform the PDB lifecycle management operations, the default CDB administrator credentials must be defined. 

  To define the default CDB administrator credentials, perform the following steps on the target CDB(s) where PDB-LM operations are to be performed:

  Create the CDB administrator user and grant the SYSDBA privilege. In this example, the user is called C##DBAPI_CDB_ADMIN. However, any suitable common user name can be used.

  ```sh
  CREATE USER C##DBAPI_CDB_ADMIN IDENTIFIED BY <PASSWORD>;
  GRANT SYSOPER TO C##DBAPI_CDB_ADMIN CONTAINER = ALL;
  ```
+ ### Building the Oracle REST Data Service (ORDS) Image

  Oracle On-Premise Database controller enhances the Oracle REST Data Services (ORDS) image to enable it for PDB Lifecycle Management. You can build this image by following the instructions below: 
    * After cloning the repository, go to the "ords" folder, and run: 
        ```sh
        docker build -t oracle/ords-dboper:latest .
        ```
    * Once the image is ready, you need to push it to your Docker Images Repository to pull it during CDB Resource creation.

+ ### Install cert-manager

  Validating webhook is an endpoint Kubernetes can invoke prior to persisting resources in ETCD. This endpoint returns a structured response indicating whether the resource should be rejected or accepted and persisted to the datastore.

  Webhook requires a TLS certificate that the apiserver is configured to trust . Install the cert-manager with the following command:
  
  ```sh
  kubectl apply -f https://github.com/jetstack/cert-manager/releases/latest/download/cert-manager.yaml
  ```

## Kubernetes Secrets

  Both CDBs and PDBs make use of Kubernetes Secrets to store usernames and passwords. 

  For CDB, create a secret file as shown here: [config/samples/onpremdb/cdb_secret.yaml](../../config/samples/onpremdb/cdb_secret.yaml)

  ```sh
  $ kubectl apply -f cdb_secret.yaml
  secret/cdb1-secret created
  ```
  On successful creation of the CDB Resource, the CDB secrets would be deleted from the Kubernetes system. 

  For PDB, create a secret file as shown here: [config/samples/onpremdb/pdb_secret.yaml](../../config/samples/onpremdb/pdb_secret.yaml)

  ```sh
  $ kubectl apply -f pdb_secret.yaml
  secret/pdb1-secret created
  ```
  **Note:** Don't leave plaintext files containing sensitive data on disk. After loading the Secret, remove the plaintext file or move it to secure storage.

  Another option is to use "kubectl create secret" command as shown below for the PDB:
  ```sh
  $ kubectl create secret generic pdb1-secret --from-literal sysadmin_user=pdbadmin --from-literal sysdamin_pwd=WE2112#232#
  secret/pdb1-secret created
  ```
  
## Kubernetes CRD for CDB

  The Oracle Database Operator creates the CDB kind as a custom resource that models a target CDB as a native Kubernetes object. This is only used to create Pods to connect to the target CDB to perform PDB-LM operations. Each CDB resource follows the CDB CRD as defined here: [config/crd/bases/database.oracle.com_cdbs.yaml](../../config/crd/bases/database.oracle.com_cdbs.yaml)

 + ### CDB Sample YAML   

  A sample .yaml file is available here: [config/samples/onpremdb/cdb.yaml](../../config/samples/onpremdb/cdb.yaml)

  **Note:** The password and username fields in the above `cdb.yaml` yaml are Kubernetes Secrets. Please see the section [Kubernetes Secrets](ORACLE_ONPREMDB_CONTROLLER_README.md#kubernetes-secrets) for more information. 

 + ### Check the status of the all CDBs
  ```sh
  $ kubectl get cdbs -A

  NAMESPACE                         NAME      CDB NAME   DB SERVER    DB PORT   SCAN NAME   STATUS   MESSAGE
  oracle-database-operator-system   cdb-dev   devdb      172.17.0.4   1521      devdb       Ready    Success
  ```

## Kubernetes CRD for PDB  

  The Oracle Database Operator creates the PDB kind as a custom resource that models a PDB as a native Kubernetes object. This PDB resource can be used to perform PDB-LM operations by specifying the action attribute in the PDB specs. Each PDB resource follows the PDB CRD as defined here: [config/crd/bases/database.oracle.com_pdbs.yaml](../../config/crd/bases/database.oracle.com_pdbs.yaml)


 + ### PDB Sample YAML   

  A sample .yaml file is available here: [config/samples/onpremdb/pdb.yaml](../../config/samples/onpremdb/pdb.yaml)

  **Note:** The password and username fields in the above `pdb.yaml` yaml are Kubernetes Secrets. Please see the section [Kubernetes Secrets](ORACLE_ONPREMDB_CONTROLLER_README.md#kubernetes-secrets) for more information.

 + ### Check the status of the all PDBs
  ```sh
  $ kubectl get pdbs -A

  NAMESPACE                         NAME   CONNECT STRING       CDB NAME   PDB NAME   PDB SIZE   STATUS   MESSAGE
  oracle-database-operator-system   pdb1   devdb:1521/pdbdev    cdb-dev     pdbdev      2G       Ready    Success
  oracle-database-operator-system   pdb2   testdb:1521/pdbtets  cdb-test    pdbtes      1G       Ready    Success
  ```

## PDB Lifecycle Management Operations

  Using ORDS, you can perform the following PDB-LM operations: CREATE, CLONE, PLUG, UNPLUG and DELETE

+ ### Create PDB

  A sample .yaml file is available here: [config/samples/onpremdb/pdb.yaml](../../config/samples/onpremdb/pdb.yaml)

+ ### Clone PDB

  A sample .yaml file is available here: [config/samples/onpremdb/pdb_clone.yaml](../../config/samples/onpremdb/pdb_clone.yaml)

+ ### Plug PDB

  A sample .yaml file is available here: [config/samples/onpremdb/pdb_plug.yaml](../../config/samples/onpremdb/pdb_plug.yaml)

+ ### Unplug PDB

  A sample .yaml file is available here: [config/samples/onpremdb/pdb_unplug.yaml](../../config/samples/onpremdb/pdb_unplug.yaml)

+ ### Delete PDB

  A sample .yaml file is available here: [config/samples/onpremdb/pdb_delete.yaml](../../config/samples/onpremdb/pdb_delete.yaml)

## Validation and Errors

You can check Kubernetes events for any errors or status updates as shown below:
```sh
$ kubectl get events -A
NAMESPACE                         LAST SEEN   TYPE      REASON               OBJECT                                                             MESSAGE
oracle-database-operator-system   58m         Warning   Failed               pod/cdb-dev-ords-qiigr                                             Error: secret "cdb1-secret" not found
oracle-database-operator-system   56m         Normal    DeletedORDSPod       cdb/cdb-dev                                                        Deleted ORDS Pod(s) for cdb-dev
oracle-database-operator-system   56m         Normal    DeletedORDSService   cdb/cdb-dev                                                        Deleted ORDS Service for cdb-dev
...
oracle-database-operator-system   26m         Warning   OraError             pdb/pdb1                                                            ORA-65016: FILE_NAME_CONVERT must be specified...
oracle-database-operator-system   24m         Warning   OraError             pdb/pdb2                                                            ORA-65011: Pluggable database DEMOTEST does not exist.
...
oracle-database-operator-system   20m         Normal    Created              pdb/pdb1                                                            PDB 'demotest' created successfully
...
oracle-database-operator-system   17m         Warning   OraError             pdb/pdb3                                                            ORA-65012: Pluggable database DEMOTEST already exists...
```

+ ### CDB Validation and Errors

Validation is done at the time of CDB resource creation as shown below:
```sh
$ kubectl apply -f cdb1.yaml
The PDB "cdb-dev" is invalid:
* spec.dbServer: Required value: Please specify Database Server Name or IP Address
* spec.dbPort: Required value: Please specify DB Server Port
* spec.ordsImage: Required value: Please specify name of ORDS Image to be used
```

Apart from events, listing of CDBs will also show the possible reasons why a particular CDB CR could not be created as shown below:
```sh
  $ kubectl get cdbs -A

  NAMESPACE                         NAME      CDB NAME   DB SERVER    DB PORT   SCAN NAME   STATUS   MESSAGE
  oracle-database-operator-system   cdb-dev   devdb      172.17.0.4   1521      devdb       Failed   Secret not found:cdb1-secret
```

+ ### PDB Validation and Errors

Validation is done at the time of PDB resource creation as shown below:
```sh
$ kubectl apply -f pdb1.yaml
The PDB "pdb1" is invalid:
* spec.cdbResName: Required value: Please specify the name of the CDB Kubernetes resource to use for PDB operations
* spec.pdbName: Required value: Please specify name of the PDB to be created
* spec.adminPwd: Required value: Please specify PDB System Administrator Password
* spec.fileNameConversions: Required value: Please specify a value for fileNameConversions. Values can be a filename convert pattern or NONE
```

Similarly, for PDBs, listing of PDBs will also show the possible reasons why a particular PDB CR could not be created as shown below:
```sh
$ kubectl get pdbs -A
NAMESPACE                         NAME   CONNECT STRING   CDB NAME   PDB NAME   PDB SIZE   STATUS   MESSAGE
oracle-database-operator-system   pdb1                    democdb    demotest1             Failed   Secret not found:pdb12-secret
oracle-database-operator-system   pdb2                    democdb    demotest2             Failed   ORA-65016: FILE_NAME_CONVERT must be specified...
```