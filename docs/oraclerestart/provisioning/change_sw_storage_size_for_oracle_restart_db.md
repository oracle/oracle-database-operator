# Change the size of Software Storage Location for an existing Oracle Restart Database

### In this Usecase:
* You have previously deployed an Oracle Restart Database in Kubernetes (for example, on OKE or OpenShift) using the Oracle Restart Database Controller.
* Change the software volume size where Oracle binaries are installed for an existing Oracle Restart Database Pod, you can do so by updating in the Custom Resource YAML associated with your Oracle Restart instance.
* This example uses `oraclerestart_prov_nodeports.yaml` to provision the initial Oracle Restart Database using Oracle Restart Controller with:

  * Oracle Restart Pod
  * Headless services for Oracle Restart
    * Oracle Restart Node hostname
  * Node Port 30007 mapped to port 1521 for Database Listener. If you are using Loadbalancer service then you will see lbservice.
  * Persistent volumes created automatically based on specified disks for Oracle ASM storage
  * Software Persistent Volume and Staged Software Persistent Volume using the specified location on the corresponding worker node. If you are using Storageclass then the software volume is dynamically provisioned.
  * Namespace: `orestart`
  * Staged Software location on the worker nodes is specified by `hostSwStageLocation`. The Grid Infrastructure and RDBMS Binaries are copied to this location on the worker node. If you are using, exisitng NFS based PVC for the staged software, the pramater is `swStagePvcMountLocation` under `configParams`.
  * You will be using storageclass to dynamically allcate the storage. using the storage class **oci-bv**.

### In this Example: 
  * Oracle Restart Database Slim Image `dbocir/oracle/database-orestart:19.3.0-slim` is used and it is built using files from [GitHub location](https://github.com/oracle/docker-images/tree/main/OracleDatabase/RAC/OracleRealApplicationClusters#building-oracle-rac-database-container-slim-image). 
  * The disks provisioned using storageclass are mounted inside the Oracle Restart Pod as `/dev/asm-disk1` and `/dev/asm-disk2`. 
  * Specify the size of these devices along with names using the parameter `swLocStorageSizeInGb`. Size is by-default in GBs.

**NOTE:** When no separate diskgroup names are specified for CRS Files, Database Files and Recovery Area Files, then the default diskgroup named `+DATA` is created from the disks specified by the parameter `crsAsmDeviceList`.
  
### Steps - Deploy the Oracle Restart Database
* Skip this step if you have already deployed the Oracle Restart database using storage class.
* Use the file: [oraclerestart_prov_storage_class_before_sw_home_resize.yaml](./oraclerestart_prov_storage_class_before_sw_home_resize.yaml) for this use case as below:
* Update the Oracle Restart container image. In this example, we have `dbocir/oracle/database-orestart:19.3.0-slim` in [oraclerestart_prov_storage_class_before_sw_home_resize.yaml](./oraclerestart_prov_storage_class_before_sw_home_resize.yaml) file to point to the container image you have built. 
* Deploy the `oraclerestart_prov_storage_class_before_sw_home_resize.yaml` file:
    ```sh
    kubectl apply -f oraclerestart_prov_storage_class_before_sw_home_resize.yaml
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
* Check Details of Kubernetes CRD Object as in this [example](./orestart_storage_class_object_before_sw_home_resize.txt)

### Steps - Update the Software Home Location in Oracle Restart Database
*  In order to `increase` the size of Software Home Location, you can use the updated file [oraclerestart_prov_storage_class_after_sw_home_resize.yaml](./oraclerestart_prov_storage_class_after_sw_home_resize.yaml). 
* Update the Oracle Restart container image. In this example, we have `dbocir/oracle/database-orestart:19.3.0-slim` in [oraclerestart_prov_storage_class_before_sw_home_resize.yaml](./oraclerestart_prov_storage_class_before_sw_home_resize.yaml) file to point to the container image you have built.
*  Deploy the `oraclerestart_prov_storage_class_after_sw_home_resize.yaml` file:
    ```sh
    $ kubectl apply -f oraclerestart_prov_storage_class_after_sw_home_resize.yaml
    oraclerestart.database.oracle.com/oraclerestart-sample configured
    ```
   You will notice Persistent Volume for the Software Location has been resized. You can check Details of updated Kubernetes CRD Object as in this [example](./orestart_storage_class_object_after_sw_home_resize.txt)
