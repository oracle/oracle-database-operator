# Adding ASM Disks - Add ASM Disks to an existing Oracle Restart Database

This use case demonstrates adding a new ASM Disks to an existing Oracle Restart Database provisioned earlier using Oracle Restart Controller.

In this use case, the existing Oracle Restart Database is deployed with Node Port Service using the file `oraclerestart_prov_nodeports.yaml` from Case [Provisioning an Oracle Restart Database with NodePort Service](./provisioning/provisioning_oracle_restart_db_nodeport.md) using Oracle Restart Controller with:

* 1 Node Oracle Restart
* Headless services for Oracle Restart
  * Oracle Restart Node hostname
* Node Port 30007 mapped to port 1521 for Database Listener
* Persistent volumes created automatically based on specified disks for Oracle ASM storage
* Software Persistent Volume and Staged Software Persistent Volume using the specified location on the corresponding worker node.
* Namespace: `orestart`
* Staged Software location on the worker nodes is specified by `hostSwStageLocation`. The Grid Infrastructure and RDBMS Binaries are copied to this location on the worker node.
* Software location on the worker nodes is specified by `hostSwLocation`. The GI HOME and the RDBMS HOME in the Oracle Restart Pod will be mounted using this location on the worker node.


In this example, 
  * Oracle Restart Database Slim Image `localhost/oracle/database-orestart:19.3.0-slim` is used and it is built using files from [GitHub location](https://github.com/oracle/docker-images/tree/main/OracleDatabase/RAC/OracleRealApplicationClusters#building-oracle-rac-database-container-slim-image). Default image created using files from this project is `localhost/oracle/database-rac:19.3.0-slim`. You need to tag it with name `localhost/oracle/database-orestart:19.3.0-slim`.
  * When you are building the image yourself, update the image value in the `oraclerestart_prov_nodeports.yaml` file to point to the container image you have built. 
  * The disks on the worker nodes for the Oracle Restart storage are `/dev/disk/by-partlabel/asm-disk1` and `/dev/disk/by-partlabel/asm-disk2`. 
  * Specify the size of these devices along with names using the parameter `disksBySize`. Size is by-default in GBs.
  * In this example, two new disks will be added to the existing Oracle Restart Database Deployment. For this purpose, the disks on the worker nodes which will be used are `/dev/disk/by-partlabel/asm-disk3` and `/dev/disk/by-partlabel/asm-disk4`.
  * Default value in yaml file is `autoUpdate: "true"`, which will delete and recreate the pod with updated ASM disks in the Oracle Restart Deployment. In this case, the new disks will be automatically added to the existing Diskgroup.
  * If the value in yaml file is set to `autoUpdate: "false"`, the Oracle Restart Database Pod is recreated, but the additional disks are `NOT` added to the ASM Disk Group automatically.


## When autoUpdate is set to true

Use the file: [orestart_prov_asm_disk_addition.yaml](./orestart_prov_asm_disk_addition.yaml) for this use case as below:

1. Deploy the `orestart_prov_asm_disk_addition.yaml` file:
    ```sh
    kubectl apply -f orestart_prov_asm_disk_addition.yaml
    ```
In this case, the new disks will be added to the existing Diskgroup in the Oracle Restart Database.

2. Check the status of the deployment:
    ```sh
    # Check the status of the Kubernetes Pods:
    kubectl get all -n orestart

    # Check the logs of a particular pod. For example, to check status of pod "dbmc1-0":
    kubectl exec -it pod/dbmc1-0 -n orestart -- bash -c "tail -f /tmp/orod/oracle_db_setup.log"
    ```
 3. Samples logs in [logs](./logs/asm_addition_autoupdate_true.txt) for disk addition with option `autoUpdate: true`.


## When autoUpdate is set to false

Use the file: [orestart_prov_asm_disk_addition_autoupdate_false.yaml](./orestart_prov_asm_disk_addition_autoupdate_false.yaml) for this use case as below:

1. Deploy the `orestart_prov_asm_disk_addition_autoupdate_false.yaml` file:
    ```sh
    kubectl apply -f orestart_prov_asm_disk_addition_autoupdate_false.yaml
    ```
In this case, new disks are added to Oracle Restart Database Object Statefulset and Pods are recreated, but this disk is not added to the ASM Disk Group.

2. Check the status of the deployment:
    ```sh
    # Check the status of the Kubernetes Pods:
    kubectl get all -n orestart

    # Check the logs of a particular pod. For example, to check status of pod "dbmc1-0":
    kubectl exec -it pod/dbmc1-0 -n orestart -- bash -c "tail -f /tmp/orod/oracle_db_setup.log"
    ```
 3. Samples logs in [logs](./logs/asm_addition_autoupdate_false.txt) for disk addition with option `autoUpdate: false`.