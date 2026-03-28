# Change Memory and CPU allocation for an earlier provisioned Oracle Restart Database

### In this Use case:
* You have previously deployed an Oracle Restart Database in Kubernetes (for example, on OKE or OpenShift) using the Oracle Restart Database Controller.
* You can change the memory and CPU allocation for an existing Oracle Restart Database Pod by updating resource requests and limits in the Custom Resource YAML associated with your Oracle Restart instance.
* This example uses `oraclerestart_prov_nodeports.yaml` to provision the initial Oracle Restart Database using Oracle Restart Controller with:

  * Oracle Restart Pod
  * Headless services for Oracle Restart
    * Oracle Restart Node hostname
  * Node Port 30007 mapped to port 1521 for Database Listener. If you are using Loadbalancer service then you will see `lbservice`.
  * Persistent volumes that are created automatically based on specified disks for Oracle ASM storage
  * Software Persistent Volume and Staged Software Persistent Volume using the specified location on the corresponding worker node. If you are using Storageclass, then the software volume is dynamically provisioned.
  * Namespace: `orestart`
  * Staged Software location on the worker nodes is specified by `hostSwStageLocation`. The Grid Infrastructure and RDBMS Binaries are copied to this location on the worker node. If you are using, existing NFS-based PVC for the staged software, then the parameter is `swStagePvcMountLocation` under `configParams`.
  * Software location on the worker nodes is specified by `hostSwLocation`. The GI HOME and the RDBMS HOME in the Oracle Restart Pod will be mounted using this location on the worker node.


### In this Example: 
  * Oracle Restart Database Slim Image `dbocir/oracle/database-orestart:19.3.0-slim` is used. It is built using files from this [GitHub location](https://github.com/oracle/docker-images/tree/main/OracleDatabase/RAC/OracleRealApplicationClusters#building-oracle-rac-database-container-slim-image). Default image created using files from this project is `localhost/oracle/database-rac:19.3.0-slim`. You must tag it with the name `localhost/oracle/database-orestart:19.3.0-slim`. 
  * When you are building the image yourself, update the image value in the `oraclerestart_prov_nodeports.yaml` file to point to the container image that you have built. 
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

### Steps - Deploy initial Oracle Restart Database 
* Use the file [oraclerestart_prov_nodeports.yaml](./oraclerestart_prov_nodeports.yaml) for the initial deployment before any CPU or memory change.
* Deploy the `oraclerestart_prov_nodeports.yaml` file:
    ```sh
    kubectl apply -f oraclerestart_prov_nodeports.yaml
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

### Steps - Modify the Memory and CPU in the Oracle Restart Database Pod
* Use the file [oraclerestart_prov_nodeports_mcpu_change.yaml](./oraclerestart_prov_nodeports_mcpu_change.yaml) to change the Memory and CPU allocation for the existing Oracle Restart Database Pod:
    ```sh
    kubectl apply -f oraclerestart_prov_nodeports_mcpu_change.yaml
    oraclerestart.database.oracle.com/oraclerestart-sample configured
    ```
  
**NOTE:**  You will notice that the exiting Oracle Restart Database Pod will be recreated with updated Memory and CPU allocation.

* Check Details of Kubernetes CRD Object before and after the change as in this [example](./oraclerestart_prov_nodeports_mcpu_change.txt). It also has the details of the memory and cpu inside the pod before and after the change.