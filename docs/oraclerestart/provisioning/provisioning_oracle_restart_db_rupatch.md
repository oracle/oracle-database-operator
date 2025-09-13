# Provisioning an Oracle Restart Database with RU Patch on FileSystem
### In this Usecase:
* In this use case, the Oracle Grid Infrastructure and Oracle Restart Database are deployed automatically using Oracle Restart Controller. 
* In this use case, before the installation of Oracle Restart, the GRID HOME and RDBMS HOME are patched with the provided `RU Patch`.
* This example uses `oraclerestart_prov_rupatch.yaml` to provision an Oracle Restart Database using Oracle Restart Controller with:
  * Oracle Restart Pod
  * Headless services for Oracle Restart
    * Oracle Restart Node hostname
  * Node Port 30007 mapped to port 1521 for Database Listener
  * Persistent volumes created automatically based on specified disks for Oracle ASM storage
  * Software Persistent Volume and Staged Software Persistent Volume using the specified location on the corresponding worker node.
  * Namespace: `orestart`
  * Staged Software location on the worker nodes is specified by `hostSwStageLocation`. The Grid Infrastructure and RDBMS Binaries are copied to this location on the worker node.
  * Software location on the worker nodes is specified by `hostSwLocation`. The GI HOME and the RDBMS HOME in the Oracle Restart Pod will be mounted using this location on the worker node.
  * Directory where the **Release Update (RU) patch** has been unzipped is specified by `ruPatchLocation`. 
    * For Example: To apply the 19.28 RU Patch `37957391`, if you have unzipped the RU Patch .zip file `p37957391_190000_Linux-x86-64.zip` to location `/scratch/software/19c/19.28/` on the worker node, then set this parameter `ruPatchLocation` to value `/scratch/software/ru_patch/37957391`.
    * Set the permission on the unzipped RU software directory to be `755` recursively.

### In this Example:
  * Oracle Restart Database Slim Image `dbocir/oracle/database-orestart:19.3.0-slim` is used and it is built using files from [GitHub location](https://github.com/oracle/docker-images/tree/main/OracleDatabase/RAC/OracleRealApplicationClusters#building-oracle-rac-database-container-slim-image). Default image created using files from this project is `localhost/oracle/database-rac:19.3.0-slim`. You need to tag it with name `dbocir/oracle/database-orestart:19.3.0-slim`. 
  * When you are building the image yourself, update the image value in the `oraclerestart_prov_rupatch.yaml` file to point to the container image you have built. 
  * The disks on the worker nodes for the Oracle Restart storage are `/dev/disk/by-partlabel/asm-disk1` and `/dev/disk/by-partlabel/asm-disk2`. 
  * Specify the size of these devices along with names using the parameter `storageSizeInGb`. Size is by-default in GBs.

**NOTE:** When no separate diskgroup names are specified for CRS Files, Database Files and Recovery Area Files, then the default diskgroup named `+DATA` is created from the disks specified by the parameter `crsAsmDeviceList`.

### Steps: Deploy Oracle Restart Database
* Use the file: [oraclerestart_prov_rupatch.yaml](./oraclerestart_prov_rupatch.yaml) for this use case as below:
* Deploy the `oraclerestart_prov_rupatch.yaml` file:
    ```sh
    kubectl apply -f oraclerestart_prov_rupatch.yaml
    oraclerestart.database.oracle.com/oraclerestart-sample created
    ```
* Check the status of the deployment:
    ```sh
    # Check the status of the Kubernetes Pods:    
    kubectl get all -n orestart

    # Check the logs of a particular pod. For example, to check status of pod "dbmc1-0":    
    kubectl exec -it pod/dbmc1-0 -n orestart -- bash -c "tail -f /tmp/orod/oracle_db_setup.log"
    ===============================
    ORACLE DATABASE IS READY TO USE
    ===============================
    ```
* Check Details of Kubernetes CRD Object as in this [example](./orestart_rupatch_object.txt)
* Refer to the page [Database Connection](./database_connection.md) for the details to connect to Oracle Restart Database deployed using above example.
