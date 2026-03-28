## Adding ASM Disks - Add ASM Disks to an existing Oracle Restart Database

### In this use case:

* You have previously deployed an Oracle Restart Database in Kubernetes (for example, on OKE or OpenShift) using the Oracle Restart Database Controller. Now, you need to expand ASM storage by adding new ASM disks.
* The existing Oracle Restart Database is deployed with Node Port Service using the file `oraclerestart_prov_nodeports.yaml` from Case [Provisioning an Oracle Restart Database with NodePort Service](./provisioning_oracle_restart_db_nodeport.md) using Oracle Restart Controller with:
  * Oracle Restart Pod
  * Headless services for Oracle Restart
    * Oracle Restart Node hostname
  * Node Port 30007 mapped to port 1521 for Database Listener. If you are using Loadbalancer then you will see LB service  
  * Persistent volumes created automatically based on specified disks for Oracle ASM storage 
  * Software Persistent Volume and Staged Software Persistent Volume using the specified location on the corresponding worker node 
  * Namespace: `orestart` 
  * Staged Software location on the worker nodes is specified by `hostSwStageLocation`. The Grid Infrastructure and RDBMS Binaries are copied to this location on the worker node 
  * Software location on the worker nodes is specified by `hostSwLocation`. The GI HOME and the RDBMS HOME in the Oracle Restart Pod will be mounted using this location on the worker node 

### General Steps 
  * If you are using storage class to dynamically provision the ASM disks, then you do not need to allocate block devices. If you are not using storage class, then you must allocate block devices to worker nodes where the Oracle Restart database pod is running. You must clean up the new ASM disks by using the `dd` command.         
  * Update the Oracle Restart Custom Resource. Edit the custom resource YAML (`oraclerestarts.database.oracle.com`) to reference the new PVCs/disks under the appropriate ASM configuration.

### In this Example: 
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
  * The default value in YAML file is `autoUpdate: "true"`, which will delete and recreate the pod with updated ASM disks in the Oracle Restart Deployment. In this case, the new disks will be automatically added to the existing Diskgroup.
  * If the value in the YAML file is set to `autoUpdate: "false"`, then the Oracle Restart Database Pod is recreated, but the additional disks are _not_ added to the ASM Disk Group automatically.


## When autoUpdate is set to true
* For this use case, use the file [orestart_prov_asm_disk_addition.yaml](./orestart_prov_asm_disk_addition.yaml):
* Deploy the `orestart_prov_asm_disk_addition.yaml` file:
    ```sh
    kubectl apply -f orestart_prov_asm_disk_addition.yaml
    ```
In this case, the new disks will be added to the existing Diskgroup in the Oracle Restart Database.
* Check the status of the deployment:
    ```sh
    # Check the status of the Kubernetes Pods:
    kubectl get all -n orestart

    # Check the logs of a particular pod. For example, to check status of pod "dbmc1-0":
    kubectl exec -it pod/dbmc1-0 -n orestart -- bash -c "tail -f /tmp/orod/oracle_db_setup.log"
    ```
 * Example logs are in [ASM addition autoupdate_true logs](./asm_addition_autoupdate_true.txt) for disk addition with the option `autoUpdate: true`.


## When autoUpdate is set to false
* For this use case, use the file: [orestart_prov_asm_disk_addition_autoupdate_false.yaml](./orestart_prov_asm_disk_addition_autoupdate_false.yaml):
* Deploy the `orestart_prov_asm_disk_addition_autoupdate_false.yaml` file:
    ```sh
    kubectl apply -f orestart_prov_asm_disk_addition_autoupdate_false.yaml
    ```
In this scenario, new disks are added to Oracle Restart Database Object Statefulset and Pods are recreated, but this disk is not added to the ASM Disk Group.
* Check the status of the deployment:
    ```sh
    # Check the status of the Kubernetes Pods:
    kubectl get all -n orestart

    # Check the logs of a particular pod. For example, to check status of pod "dbmc1-0":
    kubectl exec -it pod/dbmc1-0 -n orestart -- bash -c "tail -f /tmp/orod/oracle_db_setup.log"
    ```
 * Example logs are in [ASM addition autoupdate_false logs](./asm_addition_autoupdate_false.txt) for disk addition with option `autoUpdate: false`.
