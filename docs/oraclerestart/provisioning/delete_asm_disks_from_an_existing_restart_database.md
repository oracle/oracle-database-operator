# Deleting ASM Disks - Delete ASM Disks from an existing Oracle Restart Database

### In this use case:

* You have previously deployed an Oracle Restart Database in Kubernetes (for example, on OKE or OpenShift) using the Oracle Restart Database Controller. Now, you need to remove ASM disks from the ASM storage.
* The existing Oracle Restart Database is deployed with Node Port Service using the file `oraclerestart_prov_nodeports.yaml` described in [Provisioning an Oracle Restart Database with NodePort Service](./provisioning/provisioning_oracle_restart_db_nodeport.md) This deployment uses Oracle Restart Controller with:
  * Oracle Restart Pod
  * Headless services for Oracle Restart
    * Oracle Restart Node hostname
  * Node Port 30007 mapped to port 1521 for Database Listener. If you are using Load Balancer,  then you will see the LB service 
  * Persistent volumes created automatically based on specified disks for Oracle ASM storage
  * Software Persistent Volume and Staged Software Persistent Volume using the specified location on the corresponding worker node 
  * Namespace: `orestart`
  * Staged Software location on the worker nodes is specified by `hostSwStageLocation`. The Grid Infrastructure and RDBMS Binaries are copied to this location on the worker node 
  * Software location on the worker nodes is specified by `hostSwLocation`. The GI HOME and the RDBMS HOME in the Oracle Restart Pod will be mounted using this location on the worker node 

### In this exapmple:
  * Oracle Restart Database Slim Image `dbocir/oracle/database-orestart:19.3.0-slim` is used and it is built using files from [GitHub location](https://github.com/oracle/docker-images/tree/main/OracleDatabase/RAC/OracleRealApplicationClusters#building-oracle-rac-database-container-slim-image). Default image created using files from this project is `localhost/oracle/database-rac:19.3.0-slim`. You need to tag it with name you want. You can also push the image to your container repository. 
 The ASM diskgroup is configured using `asmDiskGroupDetails` in the YAML file. The disks specified in `asmDiskGroupDetails` are used for Oracle ASM Storage-    
```text
For example:
  asmDiskGroupDetails:
    - name: DATA
      redundancy: EXTERNAL
      type: CRSDG
      disks:
        - /dev/disk/by-partlabel/asm-disk1  # ASM disk device path 1
        - /dev/disk/by-partlabel/asm-disk2  # ASM disk device path 2
```
  * Before deleting the disk, **you will need to remove the disk at the ASM Level** using `ALTER DISKGROUP DROP DISK` command. 
  * If you delete the disk’s YAML file before first removing the disk at the ASM level, thne the Operator will not delete the disk from the StatefulSet, and the Oracle Restart Database Pod will not be recreated. Instead, to prevent data loss, the operator will wait until you remove the disk at the ASM level before proceeding.
  * In this example, out of the two disks mentioned above, the disk `/dev/disk/by-partlabel/asm-disk2` will be deleted from the existing Oracle Restart Database Deployment. 

### Steps: Delete the ASM Disk
Use the file: [orestart_prov_asm_disk_deletion.yaml](./orestart_prov_asm_disk_deletion.yaml) for this procedure:

* Deploy the `orestart_prov_asm_disk_deletion.yaml` file:
    ```sh
    kubectl apply -f orestart_prov_asm_disk_deletion.yaml
    ```

In this case, the disk will be deleted and Oracle Restart Database Pod will be recreated. 

* Check the status of the deployment:
    ```sh
    # Check the status of the Kubernetes Pods:
    kubectl get all -n orestart

    # Check the logs of a particular pod. For example, to check status of pod "dbmc1-0":
    kubectl exec -it pod/dbmc1-0 -n orestart -- bash -c "tail -f /tmp/orod/oracle_db_setup.log"
    ```
* You can review example logs in [logs](./asm_disk_deletion.txt) when the disk `/dev/disk/by-partlabel/asm-disk2` is removed at the ASM Level first and then the file [orestart_prov_asm_disk_deletion.yaml](./orestart_prov_asm_disk_deletion.yaml) is applied to remove it from the Stateful set.

* Example logs in [logs](./asm_disk_deletion1.txt) show what happens when the attempt  is made to remove the disk `/dev/disk/by-partlabel/asm-disk2` from the Stateful Set by using the file [orestart_prov_asm_disk_deletion.yaml](./orestart_prov_asm_disk_deletion.yaml) but this disk is NOT yet removed at the ASM Level. 

In this case, the Operator will be waiting for the disk to be deleted at the ASM Level. After it detects the Disk has been deleted at the ASM Level, and it is safe to proceed, it will remove the disk from the Stateful Set.