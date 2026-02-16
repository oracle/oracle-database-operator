# Provisioning an Oracle Restart Database with multiple diskgroups
### In this Usecase:
* In this use case, the Oracle Grid Infrastructure and Oracle Restart Database are deployed automatically using Oracle Restart Controller. In this example, multiple diskgroups are created for CRS Files, Database Files and Recovery Area Files 
* This example uses `oraclerestart_prov_multiple_diskgroups.yaml` to provision an Oracle Database configured with Oracle Restart using Oracle Restart Controller. The provisioning includes:
  * Oracle Restart Database Pod 
  * Headless services for Oracle Restart 
    * Oracle Database Node hostname 
  * Persistent volumes created automatically based on specified disks for Oracle ASM Storage 
  * Software Persistent Volume and Staged Software Persistent Volume using the specified location on the corresponding worker node 
  * Namespace: `orestart`
  * Staged Software location on the worker nodes is specified by `hostSwStageLocation`. The Grid Infrastructure and RDBMS Binaries are copied to this location on the worker node 
  * Software location on the worker nodes is specified by `hostSwLocation`. The GI HOME and the RDBMS HOME in the Oracle Restart Pod will be mounted using this location on the worker node 
### In this Example:
  * Oracle Restart Database Slim Image `dbocir/oracle/database-orestart:19.3.0-slim` is used and it is built using files from [GitHub location](https://github.com/oracle/docker-images/tree/main/OracleDatabase/RAC/OracleRealApplicationClusters#building-oracle-rac-database-container-slim-image). Default image created using files from this project is `localhost/oracle/database-rac:19.3.0-slim`. You need to tag it with name you want. You can also push the image to your container repository 
  * When you are building the image yourself, update the image value in the `oraclerestart_prov_multiple_diskgroups.yaml` file to point to the container image you have built. 
The ASM diskgroup is configured using `asmDiskGroupDetails` in the YAML file. The disks specified in `asmDiskGroupDetails` are used for Oracle ASM Storage-    
```text
For example:
  asmDiskGroupDetails:
    - name: CRSDATA                                                                # Name of the ASM Disk Group
      redundancy: EXTERNAL                                                         # ASM Diskgroup Redundancy Level for CRSDATA Diskgroup
      type: CRSDG                                                                  # Type of disk group
      disks:
        - /dev/disk/by-partlabel/asm-disk1
        - /dev/disk/by-partlabel/asm-disk2
    - name: DATA                                                                   # Name of the ASM Disk Group
      redundancy: EXTERNAL                                                         # ASM Diskgroup Redundancy Level for DATA Diskgroup
      type: DBDATAFILESDG                                                          # Type of disk group
      disks:
        - /dev/disk/by-partlabel/asm-disk3
        - /dev/disk/by-partlabel/asm-disk4
    - name: RECO                                                                   # Name of the ASM Disk Group
      redundancy: EXTERNAL                                                         # ASM Diskgroup Redundancy Level for RECO Diskgroup
      type: DBRECOVERY                                                             # Type of disk group
      disks:
        - /dev/disk/by-partlabel/asm-disk5
        - /dev/disk/by-partlabel/asm-disk6
    - name: REDO                                                                   # Name of the ASM Disk Group
      redundancy: EXTERNAL                                                         # ASM Diskgroup Redundancy Level for REDO Diskgroup
      type: DBREDO                                                                 # Type of disk group
      disks:
        - /dev/disk/by-partlabel/asm-disk7
        - /dev/disk/by-partlabel/asm-disk8                       
```

### Steps: Deploy Oracle Restart Database
* Use the file: [oraclerestart_prov_multiple_diskgroups.yaml](./oraclerestart_prov_multiple_diskgroups.yaml) for this use case as below:
* Deploy the `oraclerestart_prov_multiple_diskgroups.yaml` file:
    ```sh
    kubectl apply -f oraclerestart_prov_multiple_diskgroups.yaml
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
* Check Details of Kubernetes CRD Object as in this [example](./oraclerestart_prov_multiple_diskgroups.txt)
* Refer to the page [Database Connection](./database_connection.md) for the details to connect to Oracle Restart Database deployed using above example.
