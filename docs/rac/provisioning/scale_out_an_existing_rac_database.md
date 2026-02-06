# Scale Out - Add Node to an existing Oracle RAC Database Cluster

#### Use Case
* This use case demonstrates addition of a RAC Node to an existing Oracle RAC Database Cluster provisioned earlier using Oracle RAC Database Controller.
* In this This example, the exising 2 Node Oracle RAC Database has been deployed using the file [racdb_prov_np.yaml](./racdb_prov_np.yaml) from the case [Provisioning an Oracle RAC Database with Node Port Service](./provisioning/provisioning_oracle_rac_db_with_node_port.md) which includes:
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
* This use case will add a new Kubernetes Pod which will work as the new RAC Node after getting added to the existing Oracle RAC Database.


### In this example, 
  * The existing RAC Database was deployed using pre-built Oracle RAC Database slim image available on Oracle OCIR i.e. `phx.ocir.io/intsanjaysingh/db-repo/oracle/database-rac:19.3.0-slim`. 
  * If you had built the image yourself using the files from this [GitHub location](https://github.com/oracle/docker-images/tree/main/OracleDatabase/RAC/OracleRealApplicationClusters#building-oracle-rac-database-container-slim-image), you will need to change value of `image` with the image you had built in your enviornment in file `racdb_prov_scale_out.yaml`. 
  * The ASM diskgroup in the existing RAC Database was configured using the shared disks on the worker nodes i.e. `/dev/disk/by-partlabel/qck-ocne19-asmdisk1` and `/dev/disk/by-partlabel/qck-ocne19-asmdisk2`. These disks are specified using parameter `asmDiskGroupDetails` in the YAML file. 

**Note:** In the current case, a new worker node was labelled as below to be used for the new Kubernetes Pod `racnode3-0`(the new RAC Node):
  ```sh
  $ kubectl label node qck-ocne19-w3 raccluster=raccluster01
  node/qck-ocne19-w3 labeled
  ```

### Steps: Scale Out to add third Node
Use the file: [racdb_prov_scale_out.yaml](./racdb_prov_scale_out.yaml) for this use case as below:

1. Deploy the `racdb_prov_scale_out.yaml` file:
    ```sh
    kubectl apply -f racdb_prov_scale_out.yaml
    ```

2. Check the status of the deployment:
    ```sh
    # Check the status of the Kubernetes Pods:
    kubectl get all -n rac

    # Check the logs during the node addition for the new pod "racnode3-0":
    kubectl exec -it pod/racnode3-0 -n rac -- bash -c "tail -f /tmp/orod/oracle_db_setup.log"
    ```

3. Samples logs in [Logs](./logs/racdb_prov_scale_out/racdbprov-sample_details_scale_out.txt) and the corresponding [DB Operator Logs](./logs/racdb_prov_scale_out/operator_logs_scale_out.txt) when the above YAML file is applied. 
