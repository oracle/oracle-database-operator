# Delete ASM Disk - Delete an ASM Disk to an existing Oracle Restart Database

This use case demonstrates deleting a ASM Disk from an existing Oracle Restart Database provisioned earlier using Oracle Restart Database Controller.

In this use case, the existing Oracle Restart Database Deployed on a Kubernetes Cluster is having:

This example uses `oraclerestart_prov.yaml` to provision an Oracle Restart Database using Oracle Restart Controller with:

* 1 Node Oracle Restart
* Headless services for Oracle Restart
  * Oracle Restart node hostname
* Persistent volumes created automatically based on specified disks for Oracle Restart storage
* Software Persistent Volumes and Staged Software Persistent Volumes using the specified location on the corresponding worker nodes
* Namespace: `orestart`
* Staged Software location on the worker nodes is specified by `hostSwStageLocation` and we have copied the Grid Infrastructure and RDBMS Binaries to this location on the worker nodes
* Software location on the worker nodes is specified by `hostSwLocation`. The GI HOME and the RDBMS HOME in the Oracle Restart Pod will be mounted using this location on the worker node.

This use case will be deleting a ASM Disk which will deleted to the existing Oracle Restart Database. The existing Oracle Restart Database has been deployed using the file [./oraclerestart_prov_rupatch.yaml](././oraclerestart_prov_rupatch.yaml) from Case 4 [Provisioning an Oracle Restart Database with RU Patch on FileSystem](./provisioning_oracle_restart_db_rupatch.md)  

In this example, 
  * We are using Oracle Restart Database slim image by building it from Git location(./https://orahub.oci.oraclecorp.com/rac-docker-dev/rac-docker-images/-/blob/master/OracleRealApplicationClusters/README.md#building-oracle-rac-database-container-slim-image) i.e. `localhost/oracle/database-rac:19.3.0-slim`. To use this in your in own environment, update the image value in the `oraclerestart_prov_rupatch.yaml` file to point to your own container registry base container image.
  * The disks on the worker nodes for the Oracle Restart storage are `/dev/oracleoci/oraclevdd`. Disk `/dev/oracleoci/oraclevde` is removed manually and not in use.
  * Specify the size of these devices along with names using the parameter `disksBySize`. Size is by-default in GBs.
  * Similar settings for `hostSwStageLocation`  and `hostSwLocation` also apply to the worker node.

Use the file: [orestart_prov_asm_disk_deletion.yaml](./orestart_prov_asm_disk_deletion.yaml) for this use case as below:

1. Deploy the `orestart_prov_asm_disk_deletion.yaml` file:
    ```sh
    kubectl apply -f orestart_prov_asm_disk_deletion.yaml
    ```
Note:  
 - Default value in yaml file is `autoUpdate: "true"`, which will delete and recreate pods with updated ASM disks in the Oracle Restart Database. In this case, the new disks will be added to the existing Diskgroup in the Oracle Restart Database.

2. Check the status of the deployment:
    ```sh
    # Check the status of the Kubernetes Pods:
    kubectl get all -n orestart

    # Check the logs of a particular pod. For example, to check status of pod "dbmc1-0":
    kubectl exec -it pod/dbmc1-0 -n orestart -- bash -c "tail -f /tmp/orod/oracle_db_setup.log"
    ```
 3. Samples logs in [logs](./logs/asm_addition_logs.txt) when the [orestart_prov_asm_disk_deletion.yaml](./orestart_prov_asm_disk_deletion.yaml) file is applied where `/dev/oracleoci/oraclevde` is removed and not in use. If its in use, operator will show error in logs and it wont be able to remove it.
    
    Describe the Restart Object to see new ASM Disks in Status:

    ```bash
    $kubectl get oraclerestarts.database.oracle.com/oraclerestart-sample -n orestart -o json | jq '.status.asmDetails.diskgroup'
    [
      {
        "disks": [
          "/dev/oracleoci/oraclevdd"
        ],
        "name": "DATA",
        "redundancy": "EXTERN"
      }
    ]

    [grid@dbmc1-0 ~]$ export ORACLE_HOME=/u01/app/19c/grid
    [grid@dbmc1-0 ~]$ export ORACLE_SID=+ASM
    [grid@dbmc1-0 ~]$ export PATH=$ORACLE_HOME/bin:$PATH
    [grid@dbmc1-0 ~]$ 
    [grid@dbmc1-0 ~]$ /u01/app/19c/grid/bin/asmcmd lsdsk
    Path
    /dev/oracleoci/oraclevdd
    [grid@dbmc1-0 ~]$ 
    ```
