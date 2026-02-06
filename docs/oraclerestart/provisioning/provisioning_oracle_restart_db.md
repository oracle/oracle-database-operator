# Provisioning an Oracle Restart Database
### In this Usecase:
* In this use case, the Oracle Grid Infrastructure and Oracle Restart Database are deployed automatically using Oracle Restart Controller.
* This example uses `oraclerestart_prov.yaml` to provision an Oracle Database configured with Oracle Restart using Oracle Restart Controller. The provisioning includes:
  * Oracle Restart Pod
  * Headless services for Oracle Restart.
    * Oracle Database Node hostname.
  * Persistent volumes created automatically based on specified disks for Oracle ASM Storage.
  * Software Persistent Volume and Staged Software Persistent Volume using the specified location on the corresponding worker node.
  * Namespace: `orestart`
  * Staged Software location on the worker nodes is specified by `hostSwStageLocation`. The Grid Infrastructure and RDBMS Binaries are copied to this location on the worker node.
  * Software location on the worker nodes is specified by `hostSwLocation`. The GI HOME and the RDBMS HOME in the Oracle Restart Pod will be mounted using this location on the worker node.

### In this Example:
  * Oracle Restart Database Slim Image `dbocir/oracle/database-orestart:19.3.0-slim` is used. It is built using files from thsi [GitHub location](https://github.com/oracle/docker-images/tree/main/OracleDatabase/RAC/OracleRealApplicationClusters#building-oracle-rac-database-container-slim-image). The default image created using files from this project is `localhost/oracle/database-rac:19.3.0-slim`. You must tag it with the name `dbocir/oracle/database-orestart:19.3.0-slim`.
  * When you are building the image yourself, update the image value in the `oraclerestart_prov.yaml` file to point to the container image that you have built. 
The ASM diskgroup is configured using `asmDiskGroupDetails` in the YAML file. The disks specified in `asmDiskGroupDetails` are used for Oracle ASM Storage-    
```text
For example:
  - name: DATA
    redundancy: EXTERNAL
    type: CRSDG
    disks:
      - /dev/oracleoci/oraclevdd
      - /dev/oracleoci/oraclevde
```

### Steps: Deploy Oracle Restart Database
* Use the file [oraclerestart_prov.yaml](./oraclerestart_prov.yaml) for this procedure:
* Deploy the `oraclerestart_prov.yaml` file:
    ```sh
    kubectl apply -f oraclerestart_prov.yaml
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
* Check Details of Kubernetes CRD Object as shown in this [example](./orestart_object.txt)
* For details about how ot connect to the Oracle Restart Database deployed in this procedure, refer to [Database Connection](./database_connection.md).
