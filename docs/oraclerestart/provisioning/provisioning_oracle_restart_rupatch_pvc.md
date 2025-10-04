# Provisioning an Oracle Restart Database with RU Patch on Existing PVC

### In this Usecase:
* The Oracle Grid Infrastructure and Oracle Restart Database are deployed automatically using Oracle Restart Controller. In this case, no storage location from the worker node is used for ASM Disks or GI HOME or RDBMS HOME or Software Staging etc. 
* This example uses `oraclerestart_prov_rupatch_pvc.yaml` to provision an Oracle Restart Database using Oracle Restart Controller with:
  * Oracle Restart Database pod
  * Headless services for Oracle Restart
    * Oracle Restart Node hostname
  * Node Port 30007 mapped to port 1521 for Database Listener
  * Persistent volumes for ASM Disks created automatically using the Storage Class (specified using `storageClass`) for Oracle ASM storage.
  * Software location and Staged Software location using a mount location which is using a pre created Persistent Volume from a network file system(NFS)
  * Namespace: `orestart`
  * Mount point for the Software Stage Location inside the Pod is specified by `swStagePvcMountLocation`.
  * Staged Software location inside the Pod is specified by `hostSwStageLocation`. It is assumed that the Grid Infrastructure and RDBMS Binaries are already available in this path on the Persistent Volume used.
  * The GI HOME and the RDBMS HOME in the Oracle Restart Pod are mounted using a Persistent Volume created using the Storage Class (specified using `storageClass`). This Persistent Volume is mounted to `/u01` inside the Pod. Size of this Persistent Volume is specified using `swLocStorageSizeInGb`.
  * Location where the `RU Patch` has been unzipped on the mounted PV is specified by `ruPatchLocation`.
  * Path to the Opatch Software compatible with the RU Patch is specified using `oPatchLocation`.

### In this Example: 
  * Oracle Restart Database Slim Image `dbocir/oracle/database-orestart:19.3.0-slim` is used and it is built using files from [GitHub location](https://github.com/oracle/docker-images/tree/main/OracleDatabase/RAC/OracleRealApplicationClusters#building-oracle-rac-database-container-slim-image). Default image created using files from this project is `localhost/oracle/database-rac:19.3.0-slim`. You need to tag it with name `dbocir/oracle/database-orestart:19.3.0-slim`. 
  * When you are building the image yourself, update the image value in the `oraclerestart_prov_rupatch_pvc.yaml` file to point to the container image you have built. 
  * Use the file [nfs_pv_stage_vol.yaml](./nfs_pv_stage_vol.yaml) to mount the Network File System as a Persistent Volume named `pv-stage-vol1`. It is assumed this NFS has the required GI and RDBMS Base Software, unzipped RU Patch binaries and Opatch binaries in the specified location. In current case, an OCI File System is used with its export path as `/stage` and Mount Target IP as `10.0.10.212`. 
  * The disk names for Oracle ASM storage are specified as `/dev/asm-disk1` and `/dev/asm-disk2`. These will be mounted using the Persistent Volumes. 
  * Specify the size of these devices using the parameter `storageSizeInGb`. Size is by-default in GBs.

**NOTE:** When no separate diskgroup names are specified for CRS Files, Database Files and Recovery Area Files, then the default diskgroup named `+DATA` is created from the disks specified by the parameter `crsAsmDeviceList`.

### Steps: Deploy Oracle Restart Database
* Use the file: [oraclerestart_prov_rupatch_pvc.yaml](./oraclerestart_prov_rupatch_pvc.yaml) for this use case as below:
* Deploy the `oraclerestart_prov_rupatch_pvc.yaml` file:
    ```sh
    kubectl apply -f oraclerestart_prov_rupatch_pvc.yaml
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
* Check Details of Kubernetes CRD Object as in this [example](./orestart_rupatch_pvc_object.txt)
* Refer to the page [Database Connection](./database_connection.md) for the details to connect to Oracle Restart Database deployed using above example.
