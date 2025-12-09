# Provisioning an Oracle Restart Database with multiple diskgroups with different redundancy
# In this Usecase:
The Oracle Grid Infrastructure and Oracle Restart Database are deployed automatically using Oracle Restart Controller. In this example, multiple diskgroups are created for CRS Files, Database Files, Recovery Area Files and Redo Log Files. Different Disk Groups have different redundancy levels.

This example uses `oraclerestart_prov_multiple_diskgroups_with_redundancy.yaml` to provision an Oracle Database configured with Oracle Restart using Oracle Restart Controller. The provisioning includes:
  * Oracle Restart Pod
  * Headless services for Oracle Restart.
    * Oracle Database Node hostname.
  * Persistent volumes created automatically based on specified disks for Oracle ASM Storage.
  * Software Persistent Volume and Staged Software Persistent Volume using the specified location on the corresponding worker node.
  * Namespace: `orestart`
  * Staged Software location on the worker nodes is specified by `hostSwStageLocation`. The Grid Infrastructure and RDBMS Binaries are copied to this location on the worker node.
  * Software location on the worker nodes is specified by `hostSwLocation`. The GI HOME and the RDBMS HOME in the Oracle Restart Pod will be mounted using this location on the worker node.

### In this example, 
  * Oracle Restart Database Slim Image `dbocir/oracle/database-orestart:19.3.0-slim` is used and it is built using files from [GitHub location](https://github.com/oracle/docker-images/tree/main/OracleDatabase/RAC/OracleRealApplicationClusters#building-oracle-rac-database-container-slim-image). Default image created using files from this project is `localhost/oracle/database-rac:19.3.0-slim`. You need to tag it with name `localhost/oracle/database-orestart:19.3.0-slim`. 
  * When you are building the image yourself, update the image value in the `oraclerestart_prov_multiple_diskgroups_with_redundancy.yaml` file to point to the container image you have built. 
The ASM diskgroup is configured using `asmDiskGroupDetails` in the YAML file. The disks specified in `asmDiskGroupDetails` are used for Oracle ASM Storage-    
```text
For example:
asmDiskGroupDetails:
  - name: CRSDATA
    redundancy: NORMAL       # Recommended: ASM-provided two-way mirroring for OCR/voting files
    type: CRSDG
    disks:
      - /dev/disk/by-partlabel/asm-disk1
      - /dev/disk/by-partlabel/asm-disk2

  - name: DBDATA
    redundancy: HIGH         # ASM three-way mirroring for critical DB data (optional, use NORMAL for balance)
    type: DBDATAFILESDG
    disks:
      - /dev/disk/by-partlabel/asm-disk3
      - /dev/disk/by-partlabel/asm-disk4

  - name: RECO
    redundancy: EXTERNAL    # Depend on storage-level RAID; change to NORMAL/EXTERNAL per risk/compliance needs
    type: DBRECOVERY
    disks:
      - /dev/disk/by-partlabel/asm-disk5
      - /dev/disk/by-partlabel/asm-disk6

  - name: REDO
    redundancy: HIGH         # Maximum protection for redo logs; use NORMAL if storage already provides redundancy
    type: DBREDO
    disks:
      - /dev/disk/by-partlabel/asm-disk7
      - /dev/disk/by-partlabel/asm-disk8           
```

### Steps: Deploy Oracle Restart Database
* Use the file: [oraclerestart_prov_multiple_diskgroups_with_redundancy.yaml](./oraclerestart_prov_multiple_diskgroups_with_redundancy.yaml) for this use case as below:
* Deploy the `oraclerestart_prov_multiple_diskgroups_with_redundancy.yaml` file:
    ```sh
    kubectl apply -f oraclerestart_prov_multiple_diskgroups_with_redundancy.yaml
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
* Check Details of Kubernetes CRD Object as in this [example](./oraclerestart_prov_multiple_diskgroups_with_redundancy.txt)
* Refer to the page [Database Connection](./database_connection.md) for the details to connect to Oracle Restart Database deployed using above example.
