# Deleting ASM Disks - Delete ASM Disks from an existing Oracle Restart Database

This use case demonstrates deleting a ASM Disk from an existing Oracle Restart Database provisioned earlier using Oracle Restart Controller.

In this use case, the existing Oracle Restart Database Deployed on a Kubernetes Cluster is having:

In this use case, the existing Oracle Restart Database is deployed with Node Port Service using the file `oraclerestart_prov_nodeports.yaml` from Case [Provisioning an Oracle Restart Database with NodePort Service](./provisioning/provisioning_oracle_restart_db_nodeport.md) using Oracle Restart Controller with:

* 1 Node Oracle Restart
* Headless services for Oracle Restart
  * Oracle Restart Node hostname
* Node Port 30007 mapped to port 1521 for Database Listener
* Persistent volumes created automatically based on specified disks for Oracle ASM storage
* Software Persistent Volume and Staged Software Persistent Volume using the specified location on the corresponding worker node.
* Namespace: `orestart`
* Staged Software location on the worker nodes is specified by `hostSwStageLocation`. The Grid Infrastructure and RDBMS Binaries are copied to this location on the worker node.
* Software location on the worker nodes is specified by `hostSwLocation`. The GI HOME and the RDBMS HOME in the Oracle Restart Pod will be mounted using this location on the worker node.


In this example, 
  * Oracle Restart Database Slim Image `localhost/oracle/database-orestart:19.3.0-slim` is used and it is built using files from [GitHub location](https://github.com/oracle/docker-images/tree/main/OracleDatabase/RAC/OracleRealApplicationClusters#building-oracle-rac-database-container-slim-image). Default image created using files from this project is `localhost/oracle/database-rac:19.3.0-slim`. You need to tag it with name `localhost/oracle/database-orestart:19.3.0-slim`. 
  * When you are building the image yourself, update the image value in the `oraclerestart_prov_nodeports.yaml` file to point to the container image you have built. 
  * The disks on the worker nodes for the Oracle Restart storage are `/dev/disk/by-partlabel/asm-disk1` and `/dev/disk/by-partlabel/asm-disk2`. 
  * Specify the size of these devices along with names using the parameter `storageSizeInGb`. Size is by-default in GBs.
  * Before deleting the disk, you will need to remove the disk at the ASM Level using "ALTER DISKGROUP DROP DISK" command. 
  * IF the disk is first NOT removed at the ASM Level and the .yaml file to delete the disk is applied, to avoid any data loss, the operator will NOT delete the disk from the Stateful Set and the Oracle Restart Database Pod will not be recreated. Operator will be in wait state until the disk is deleted at the ASM Level.
  * In this example, out of the two disks mentioned above, the disk `/dev/disk/by-partlabel/asm-disk2` will be deleted from the existing Oracle Restart Database Deployment. 

Use the file: [orestart_prov_asm_disk_deletion.yaml](./orestart_prov_asm_disk_deletion.yaml) for this use case as below:

1. Deploy the `orestart_prov_asm_disk_deletion.yaml` file:
    ```sh
    kubectl apply -f orestart_prov_asm_disk_deletion.yaml
    ```

In this case, the disk will be deleted and Oracle Restart Database Pod will be recreated. 

2. Check the status of the deployment:
    ```sh
    # Check the status of the Kubernetes Pods:
    kubectl get all -n orestart

    # Check the logs of a particular pod. For example, to check status of pod "dbmc1-0":
    kubectl exec -it pod/dbmc1-0 -n orestart -- bash -c "tail -f /tmp/orod/oracle_db_setup.log"
    ```
3. Samples logs in [logs](./asm_disk_deletion.txt) when the disk `/dev/disk/by-partlabel/asm-disk2` is removed at the ASM Level first and then[orestart_prov_asm_disk_deletion.yaml](./orestart_prov_asm_disk_deletion.yaml) file is applied to remove it from the Stateful set.

4. Samples logs in [logs](./asm_disk_deletion1.txt) when the disk `/dev/disk/by-partlabel/asm-disk2` is tried to be removed from Stateful Set using the file [orestart_prov_asm_disk_deletion.yaml](./orestart_prov_asm_disk_deletion.yaml) but this disk is NOT yet removed at the ASM Level. 

In this case, the Operator will be waiting for the disk to be deleted at the ASM Level. Once it detects the Disk has been deleted at the ASM Level and it is safe to proceed, it will remove the disk from the Stateful Set.