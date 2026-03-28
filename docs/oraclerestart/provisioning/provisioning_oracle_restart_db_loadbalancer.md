# Provisioning an Oracle Restart Database with Load Balancer

### In this Use case:
* In this use case, the Oracle Grid Infrastructure and Oracle Database are deployed automatically using Oracle Restart Controller.
* When Oracle Restart is deployed using the Oracle Restart Controller with a Load Balancer Service, the database is exposed externally through the cloud provider’s (for example, Oracle Cloud's) load balancer. This setup allows you to connect remotely to the database using the load balancer’s external IP and the target port, which is commonly 1521 (the Oracle default).
* This example uses the file `oraclerestart_prov_loadbalancer.yaml` to provision an Oracle Restart Database using Oracle Restart Controller with:
  * Oracle Restart Pod
  * Headless services for Oracle Restart
    * Oracle Restart Node hostname
  * Load Balancer Service with target port 1521
  * Persistent volumes created automatically based on specified disks for Oracle ASM storage
  * Software Persistent Volume and Staged Software Persistent Volume using the specified location on the corresponding worker node.
  * Namespace: `orestart`
  * Staged Software location on the worker nodes is specified by `hostSwStageLocation`. The Grid Infrastructure and RDBMS Binaries are copied to this location on the worker node.
  * Software location on the worker nodes is specified by `hostSwLocation`. The GI HOME and the RDBMS HOME in the Oracle Restart Pod will be mounted using this location on the worker node.

### In this Example:
  * Oracle Restart Database Slim Image `dbocir/oracle/database-orestart:19.3.0-slim` is used and it is built using files from [GitHub location](https://github.com/oracle/docker-images/tree/main/OracleDatabase/RAC/OracleRealApplicationClusters#building-oracle-rac-database-container-slim-image). Default image created using files from this project is `localhost/oracle/database-rac:19.3.0-slim`. You need to tag it with name you want. You can also push the image to your container repository. 
  * When you are building the image yourself, update the image value in the `oraclerestart_prov_nodeports.yaml` file to point to the container image you have built. 
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

### Steps: Provision Oracle Restart Database 
* Use the file [oraclerestart_prov_loadbalancer.yaml](./oraclerestart_prov_loadbalancer.yaml) for this procedure. 
* Deploy the `oraclerestart_prov_loadbalancer.yaml` file:
    ```sh
    kubectl apply -f oraclerestart_prov_loadbalancer.yaml
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
* Check Details of Kubernetes CRD Object as in this [example](./orestart_loadbalancer_object.txt)
* For details about how to connect to the Oracle Restart Database deployed using this procedure, see: [Database Connection](./database_connection.md) .
