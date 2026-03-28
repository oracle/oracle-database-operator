# Delete ASM Disk - Delete an ASM Disk from an existing Oracle RAC Database

#### Use Case
* This use case demonstrates deleting ASM Disk from an existing Oracle RAC Database provisioned earlier using Oracle RAC Database Controller.
* In this This example, the exising 2 Node Oracle RAC Database has been deployed using the file [racdb_prov_np.yaml](./racdb_prov_np.yaml) from the case [Provisioning an Oracle RAC Database with Node Port Service](./provisioning/provisioning_oracle_rac_db_with_node_port.md) which includes:
  * 2 Kubernetes Pods as the RAC Nodes
  * Headless services for RAC
    * VIP Service
    * Scan Service
    * RAC Node hostname
  * Shared Persistent volumes created automatically based on specified shared disks for RAC shared storage(ASM)
  * Software Persistent Volumes and Staged Software Persistent Volumes using the specified location on the corresponding worker nodes
  * Namespace: `rac` 
  * Staged Software location on the worker nodes is specified by `hostSwStageLocation`. Grid Infrastructure and RDBMS Binaries are copied to this location on the worker nodes. 
  * Software location on the worker nodes is specified by `racHostSwLocation`. The GI HOME and the RDBMS HOME in the Oracle RAC Pods will be mounted using this location on the corresponding worker node. 
* This use case will be deleting a Disk from `DATA` diskgroup of an existing Oracle RAC Database. 

### In this example, 
  * The existing RAC Database was deployed using pre-built Oracle RAC Database slim image available on Oracle OCIR i.e. `phx.ocir.io/intsanjaysingh/db-repo/oracle/database-rac:19.3.0-slim`. 
  * If you had built the image yourself using the files from this [GitHub location](https://github.com/oracle/docker-images/tree/main/OracleDatabase/RAC/OracleRealApplicationClusters#building-oracle-rac-database-container-slim-image), you will need to change value of `image` with the image you had built in your enviornment in file `rac_prov_asm_disk_deletion.yaml`. 
  * The ASM diskgroup in the existing RAC Database was configured using the shared disks on the worker nodes i.e. `/dev/disk/by-partlabel/qck-ocne19-asmdisk1` and `/dev/disk/by-partlabel/qck-ocne19-asmdisk2`. These disks are specified using parameter `asmDiskGroupDetails` in the YAML file. 


**Note:** 
* In case, the disk being deleted from the diskgroup is currently in use, the RAC Controller will wait for the disk to be manually dropped from the ASM Diskgroupneed and REBALANCE to complete before dropping the asm disk froms Kubernetes Pods. Corresponding, Kubernetes PV and PVC will also be deleted. The Kubernetes Pods will be recreated in a Rolling Manner. 
* In case, the disk being deleted from the diskgroup is currently not in use, the RAC Controller will go ahead to drop the asm disk froms Kubernetes Pods. Corresponding, Kubernetes PV and PVC will also be deleted. The Kubernetes Pods will be recreated in a Rolling Manner. 

Use the file: [rac_prov_asm_disk_deletion.yaml](./rac_prov_asm_disk_deletion.yaml) for this use case as below:

1. Deploy the `rac_prov_asm_disk_deletion.yaml` file:
    ```sh
    kubectl apply -f rac_prov_asm_disk_deletion.yaml
    ```

2. Check the status of the deployment:
    ```sh
    # Check the status of the Kubernetes Pods:
    kubectl get all -n rac

    # Check the logs of a particular pod. For example, to check status of pod "racnode1-0":
    kubectl exec -it pod/racnode1-0 -n rac -- bash -c "tail -f /tmp/orod/oracle_db_setup.log"
    ```
3. Samples logs in [Logs](./logs/rac_prov_asm_disk_deletion/asm_disk_deletion_disk_in_use.txt) and the corresponding [DB Operator Logs](./logs/rac_prov_asm_disk_deletion/operator_logs_disk_in_use.txt) when the above YAML file is applied and the ASM disk to be deleted is currently in use. 
4. Samples logs in [Logs](./logs/rac_prov_asm_disk_deletion/asm_disk_deletion_disk_not_in_use.txt) and the corresponding [DB Operator Logs](./logs/rac_prov_asm_disk_deletion/operator_logs_disk_not_in_use.txt) when the above YAML file is applied and the ASM disk to be deleted is currently not in use. 