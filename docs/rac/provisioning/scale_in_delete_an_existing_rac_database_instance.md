# Scale In - Delete a node from an existing Oracle RAC Database Cluster

#### Use Case
* This use case demonstrates deletion of a RAC Node from an existing Oracle RAC Database Cluster provisioned earlier using Oracle RAC Database Controller.
* In this This example, the exising 3 Node Oracle RAC Database has been deployed earlier using the file [racdb_prov_3_node.yaml](./racdb_prov_3_node.yaml) which includes:
  * 3 Kubernetes Pods as the RAC Nodes
  * Headless services for RAC
    * VIP Service
    * Scan Service
    * RAC Node hostname
  * Shared Persistent volumes created automatically based on specified shared disks for RAC shared storage(ASM)
  * Software Persistent Volumes and Staged Software Persistent Volumes using the specified location on the corresponding worker nodes
  * Namespace: `rac` 
  * Staged Software location on the worker nodes is specified by `hostSwStageLocation`. Grid Infrastructure and RDBMS Binaries are copied to this location on the worker nodes. 
  * Software location on the worker nodes is specified by `racHostSwLocation`. The GI HOME and the RDBMS HOME in the Oracle RAC Pods will be mounted using this location on the corresponding worker node. 
* This use case will delete an existing RAC Node `racnode3` from the existing Oracle RAC Cluster completing all the required steps and then the correspoding Pod will be removed from the Kubernetes Cluster. 

### In this example, 
  * The existing RAC Database was deployed using pre-built Oracle RAC Database slim image available on Oracle OCIR i.e. `phx.ocir.io/intsanjaysingh/db-repo/oracle/database-rac:19.3.0-slim`. 
  * If you had built the image yourself using the files from this [GitHub location](https://github.com/oracle/docker-images/tree/main/OracleDatabase/RAC/OracleRealApplicationClusters#building-oracle-rac-database-container-slim-image), you will need to change value of `image` with the image you had built in your enviornment in file `racdb_prov_scale_in.yaml`. 
  * The ASM diskgroup in the existing RAC Database was configured using the shared disks on the worker nodes i.e. `/dev/disk/by-partlabel/qck-ocne19-asmdisk1` and `/dev/disk/by-partlabel/qck-ocne19-asmdisk2`. These disks are specified using parameter `asmDiskGroupDetails` in the YAML file. 


### Steps: Scale In to delete a Node
Use the file: [racdb_prov_scale_in.yaml](./racdb_prov_scale_in.yaml) for this use case as below:

1. Deploy the `racdb_prov_scale_in.yaml` file:
    ```sh
    kubectl apply -f racdb_prov_scale_in.yaml
    ```

2. Check the status of the deployment:
    ```sh
    # Check the status of the Kubernetes Pods:
    kubectl get all -n rac

    # Check the logs during the node deletion for the pod "racnode3-0":
    kubectl exec -it pod/racnode3-0 -n rac -- bash -c "tail -f /tmp/orod/oracle_db_setup.log"
    ```

3. Samples logs in [Logs](./logs/racdb_prov_scale_in/racdbprov-sample_details_scale_in.txt) and the corresponding [DB Operator Logs](./logs/racdb_prov_scale_in/scale_in_operator_logs.txt) when the above YAML file is applied. 
