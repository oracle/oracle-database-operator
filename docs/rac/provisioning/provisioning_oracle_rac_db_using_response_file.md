# Provisioning Oracle RAC Database using a response file

#### Use Case
* In this use case, the Oracle Grid Infrastructure and Oracle RAC Database are deployed automatically using Oracle RAC Controller. The responsefile is 
provided to the controller using a configmap created in the same namespace. 
* This example uses `racdb_prov_using_rspfile.yaml` to provision an Oracle RAC Database using Oracle RAC Controller. The provisioning includes:
  * 2 Kubernetes Pods as the RAC Nodes
  * Headless services for RAC
    * VIP Service
    * Scan Service
    * RAC Node hostname
  * Shared Persistent volumes created automatically based on specified shared disks for RAC shared storage(ASM)
  * Software Persistent Volumes and Staged Software Persistent Volumes using the specified location on the corresponding worker nodes
  * Namespace: `rac` 
  * Staged Software location on the worker nodes is specified by `hostSwStageLocation`. Grid Infrastructure and RDBMS Binaries are copied to this location on the worker nodes. 
  * Software location on the worker nodes is specified by `racHostSwLocation`. The GI HOME and the RDBMS HOME in the Oracle RAC Pods will be mounted using this location on the corresponding worker node. 


### In this example, 
  * A pre-built Oracle RAC Database slim image available on Oracle OCIR i.e. `phx.ocir.io/intsanjaysingh/db-repo/oracle/database-rac:19.3.0-slim` is used. 
  * If you plan to build the image yourself, you can build using the files from this [GitHub location](https://github.com/oracle/docker-images/tree/main/OracleDatabase/RAC/OracleRealApplicationClusters#building-oracle-rac-database-container-slim-image). In this case, you will need to change value of `image` with the image you have built in your enviornment in file `racdb_prov_using_rspfile.yaml`. 
  * The ASM diskgroup is configured using the shared disks on the worker nodes i.e. `/dev/disk/by-partlabel/qck-ocne19-asmdisk1` and `/dev/disk/by-partlabel/qck-ocne19-asmdisk2`. These disks are specified using parameter `asmDiskGroupDetails` in the YAML file. 
  * The response files named [gi.rsp](./responsefile/gi.rsp) and [dbca.rsp](./responsefile/dbca.rsp) are used in this case. You can change the parameter values in the response file as per your requirement. 

### Steps: Deploy 2 Node Oracle RAC Database using response file  
Use the file: [racdb_prov_using_rspfile.yaml](./racdb_prov_using_rspfile.yaml) for this use case as below:

1. Create the configmap named `girsp` using the response file [gi.rsp](./responsefile/gi.rsp):
    ```sh
    kubectl create configmap girsp --from-file=gi.rsp -n rac
    ```
2. Create the configmap named `dbcarsp` using the response file [dbca.rsp](./responsefile/dbca.rsp):
    ```sh
    kubectl create configmap dbcarsp --from-file=dbca.rsp -n rac
    ```
3. Deploy the `racdb_prov_using_rspfile.yaml` file:
    ```sh
    kubectl apply -f racdb_prov_using_rspfile.yaml
    ```
4. Check the status of the deployment:
    ```sh
    # Check the status of the Kubernetes Pods:
    kubectl get all -n rac

    
    # Check the logs of a particular pod. For example, to check status of pod "racnode1-0":    
    kubectl exec -it pod/racnode1-0 -n rac -- bash -c "tail -f /tmp/orod/oracle_db_setup.log"
    ```
5. Samples logs in [Logs](./logs/racdb_prov_using_rspfile/racdbprov-sample_details.txt) and the corresponding [DB Operator Logs](./logs/racdb_prov_using_rspfile/operator_logs.txt) when the above YAML file is applied. 
