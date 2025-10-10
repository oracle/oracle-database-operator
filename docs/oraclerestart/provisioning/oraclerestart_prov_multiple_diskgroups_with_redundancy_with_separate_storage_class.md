# Provisioning an Oracle Restart Database with multiple diskgroups with different redundancy and option to specify separate storage class

### In this Usecase:
The Oracle Grid Infrastructure and Oracle Restart Database are deployed automatically using Oracle Restart Controller. In this example, multiple diskgroups are created for CRS Files, Database Files, Recovery Area Files and Redo Log Files. Different Disk Groups have different redundancy levels.

This example uses `oraclerestart_prov_multiple_diskgroups_with_redundancy_with_separate_storage_class.yaml` to provision an Oracle Database configured with Oracle Restart using Oracle Restart Controller. The provisioning includes:
  * Oracle Restart Pod
  * Headless services for Oracle Restart.
    * Oracle Database Node hostname.
  * Persistent volumes created automatically based on specified disks for Oracle ASM Storage.
  * Software Persistent Volume and Staged Software Persistent Volume using the specified location on the corresponding worker node.
  * Namespace: `orestart`
  * Staged Software location on the worker nodes is specified by `hostSwStageLocation`. The Grid Infrastructure and RDBMS Binaries are copied to this location on the worker node.
  * The GI HOME and the RDBMS HOME in the Oracle Restart Pod are mounted using a Persistent Volume created using the Storage Class (specified using `swDgStorageClass`). This Persistent Volume is mounted to `/u01` inside the Pod. Size of this Persistent Volume is specified using `swLocStorageSizeInGb`.
  * Name of Custom Storage Class for Diskgroup having CRS files is specified by `crsDgStorageClass`.  
  * Name of Custom Storage Class for Diskgroup having Database files is specified by `dataDgStorageClass`.  
  * Name of Custom Storage Class for Diskgroup having Recovery Area files is specified by `recoDgStorageClass`.  
  * Name of Custom Storage Class for Diskgroup having Redo Log files is specified by `redoDgStorageClass`.  

### In this example, 
  * Oracle Restart Database Slim Image `dbocir/oracle/database-orestart:19.3.0-slim` is used and it is built using files from [GitHub location](https://github.com/oracle/docker-images/tree/main/OracleDatabase/RAC/OracleRealApplicationClusters#building-oracle-rac-database-container-slim-image). Default image created using files from this project is `localhost/oracle/database-rac:19.3.0-slim`. You need to tag it with name `localhost/oracle/database-orestart:19.3.0-slim`. 
  * When you are building the image yourself, update the image value in the `oraclerestart_prov_multiple_diskgroups_with_redundancy_with_separate_storage_class.yaml` file to point to the container image you have built. 
  * The disks provisioned using custom storage classes are mounted inside the Oracle Restart Pod as `/dev/asm-disk1` to `/dev/asm-disk10`. 
  * Specify the size of disk devices along with names using the parameter `storageSizeInGb`. Size is by-default in GBs. 
  * The Diskgroup for CRS files is specified by `crsAsmDiskDg` and the disks on the worker nodes for this diskgroup are specified by `crsAsmDeviceList`. 
  * The Diskgroup for Database files is specified by `dbDataFileDestDg` and the disks on the worker nodes for this diskgroup are specified by `dbAsmDeviceList`. 
  * The Diskgroup for Recovery Area files is specified by `dbRecoveryFileDest` and the disks on the worker nodes for this diskgroup are specified by `recoAsmDeviceList`. 
  * The Diskgroup for Redo Log files is specified by `redoAsmDiskDg` and the disks on the worker nodes for this diskgroup are specified by `redoAsmDeviceList`. 
  * Redundancy level for the diskgroup with CRS files is mentioned by `crsAsmDiskDgRedundancy`.
  * Redundancy level for the diskgroup with Database files is mentioned by `dbAsmDiskDgRedundancy`.
  * Redundancy level for the diskgroup with Recovery files is mentioned by `recoAsmDiskDgRedudancy`. 
  * Redundancy level for the diskgroup with Redo Log files is mentioned by `redoAsmDiskDgRedundancy`. 

### Steps: Deploy Oracle Restart Database
* Use the file: [oraclerestart_prov_multiple_diskgroups_with_redundancy_with_separate_storage_class.yaml](./oraclerestart_prov_multiple_diskgroups_with_redundancy_with_separate_storage_class.yaml) for this use case as below:
* Deploy the `oraclerestart_prov_multiple_diskgroups_with_redundancy_with_separate_storage_class.yaml` file:
    ```sh
    kubectl apply -f oraclerestart_prov_multiple_diskgroups_with_redundancy_with_separate_storage_class.yaml
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
* Check Details of Kubernetes CRD Object as in this [example](./oraclerestart_prov_multiple_diskgroups_with_redundancy_with_separate_storage_class.txt)
* Refer to the page [Database Connection](./database_connection.md) for the details to connect to Oracle Restart Database deployed using above example.
