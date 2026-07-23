# Adding ASM Disk - Add an ASM Disk to an existing Oracle RAC Database
Use this guide to add new ASM disks to the DATA disk group of an Oracle RAC Database previously deployed with the Oracle RAC Database Controller on Kubernetes.


#### Use Case
This use case demonstrates adding new ASM Disks to an existing Oracle RAC Database provisioned earlier using Oracle RAC Database Controller. The existing 2-node Oracle RAC Database was deployed using the file [racdb_prov_np.yaml](./racdb_prov_np.yaml) from the case [Provisioning an Oracle RAC Database with Node Port Service](./provisioning/provisioning_oracle_rac_db_with_node_port.md). The provisioning includes:

  * 2 Kubernetes Pods as the RAC Nodes
  * Headless services for RAC
    * VIP Service
    * Scan Service
    * RAC Node hostname
  * Shared Persistent volumes created automatically based on specified shared disks for RAC shared storage(ASM)
  * Software Persistent Volumes and Staged Software Persistent Volumes using the specified location on the corresponding worker nodes
  * Namespace: `rac` 
  * Staged Software location on the worker nodes is specified by `hostSwStageLocation`. Grid Infrastructure and RDBMS Binaries are copied to this location on the worker nodes. 
  * Software location on the worker nodes, specified by `racHostSwLocation`. The GI HOME and the RDBMS HOME in the Oracle RAC Pods are mounted from this location on the corresponding worker node. 

This use case covers adding new ASM Disks to `DATA` diskgroup of an existing Oracle RAC Database. 

### Example Setup 
  * The example uses the pre-built Oracle RAC Database slim image from Oracle OCIR: `phx.ocir.io/intsanjaysingh/db-repo/oracle/database-rac:19.3.0-slim`. To use a custom image, update the `image` field with the image that you have built in your enviornment in file `rac_prov_asm_disk_addition.yaml`
  * The new ASM shared disks on the worker nodes for the RAC shared storage is `/dev/disk/by-partlabel/ocne_asm_disk_03`
  * Similar settings for `hostSwStageLocation`  and `hostSwLocation` also apply to the worker node where the Pod for the New RAC Database Instance will be running when we scale out the existing RAC Database

  * The existing RAC Database was deployed using the pre-built Oracle RAC Database slim image available on Oracle OCIR: `phx.ocir.io/intsanjaysingh/db-repo/oracle/database-rac:19.3.0-slim`. 
  * If you created a custom image using the files from this [GitHub location for building an Oracle RAC Database container slim image](https://github.com/oracle/docker-images/tree/main/OracleDatabase/RAC/OracleRealApplicationClusters#building-oracle-rac-database-container-slim-image), then update the value of `image` with your image you built in your enviornment in the file `rac_prov_asm_disk_addition_autoupdate_true.yaml`. 
  * The ASM diskgroup in the existing Oracle RAC Database was configured using the shared disks on the worker nodes: `/dev/disk/by-partlabel/qck-ocne19-asmdisk1` and `/dev/disk/by-partlabel/qck-ocne19-asmdisk2`. These disks are specified using the parameter `asmDiskGroupDetails` in the YAML file.   

**Note:** 
* The Default value during disk addition is `autoUpdate: "true"`. When `autoUpdate` is set to `"true"`,  RAC node pods are recreated in a `rolling` manner as part of the disk addition. The new disks become available within the recreated pods, and the disks are automatically added to the ASM disk group.
* When `autoUpdate` is set to `"false"`, the pods are still recreated in a 'rolling' manner and the disks are available in the pods. However, the disks are not automatically added to the ASM disk group. You must add them manually after deployment. 


### Steps: Disk Addition with `autoUpdate: "true"`
For this case, use this file: [rac_prov_asm_disk_addition_autoupdate_true.yaml](./rac_prov_asm_disk_addition_autoupdate_true.yaml). 
Follow these steps: 

1. Deploy the `rac_prov_asm_disk_addition_autoupdate_true.yaml` file:
    ```sh
    kubectl apply -f rac_prov_asm_disk_addition_autoupdate_true.yaml
    ```

2. Check the status of the deployment:
    ```sh
    # Check the status of the Kubernetes Pods:
    kubectl get all -n rac

    # Check the logs of a particular pod. For example, to check status of pod "racnode1-0":
    kubectl exec -it pod/racnode1-0 -n rac -- bash -c "tail -f /tmp/orod/oracle_db_setup.log"
    ```
 3. Review the sample logs in [logs](./logs/rac_prov_asm_disk_addition/asm_disk_addition_autoupdate_true.txt) and the corresponding [DB Operator Logs](./logs/rac_prov_asm_disk_addition/operator_logs_autoupdate_true.txt) generated during the application of the YAML file. 



### Steps: Disk Addition with `autoUpdate: "false"`
Use the file: [rac_prov_asm_disk_addition_autoupdate_false.yaml](./rac_prov_asm_disk_addition_autoupdate_false.yaml). 
Follow these steps: 

1. Deploy the `rac_prov_asm_disk_addition_autoupdate_false.yaml` file:
    ```sh
    kubectl apply -f rac_prov_asm_disk_addition_autoupdate_false.yaml
    ```

2. Check the status of the deployment:
    ```sh
    # Check the status of the Kubernetes Pods:
    kubectl get all -n rac

    # Check the logs of a particular pod. For example, to check status of pod "racnode1-0":
    kubectl exec -it pod/racnode1-0 -n rac -- bash -c "tail -f /tmp/orod/oracle_db_setup.log"
    ```
 3. Samples logs in [logs](./logs/rac_prov_asm_disk_addition/asm_disk_addition_autoupdate_false.txt) and the corresponding [DB Operator Logs](./logs/rac_prov_asm_disk_addition/operator_logs_autoupdate_false.txt) when the above YAML file is applied. 