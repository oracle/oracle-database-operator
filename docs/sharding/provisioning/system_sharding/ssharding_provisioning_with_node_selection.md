# Deploy Oracle GDD with Node Selection for Pod Placement

**IMPORTANT:** Make sure you have completed the steps for [Prerequisites for running Oracle Sharding Database Controller](../../README.md#prerequisites-for-running-oracle-sharding-database-controller) before using Oracle Sharding Controller.

This use case demonstrates how to use `nodeSelector` to place GSM, shard, and catalog pods on specific worker node pools.

In this example, certain worker nodes are labeled `gsm_pool`, `catalog_pool` and `shard_pool`. Using these labels, GSM, catalog and shard pods are scheduled only on nodes that match their assigned label.

This example uses `ssharding_shard_prov_node_selection.yaml` to provision an Oracle GDD deployment with the Oracle Sharding Controller using:

* Primary GSM pod: `gsm1`
* Standby GSM pod: `gsm2`
* Two shard database pods (`shardNum: 2`)
* One catalog database pod: `catalog`
* `shardingType: SYSTEM` (System-Managed Sharding)
* Replication type: Data Guard (replicationType: DG)
* Worker nodes with labels: `gsm_pool`, `catalog_pool` and `shard_pool`
* Namespace: `shns`

This example uses pre-built Oracle Database and Global Data Services container images available from [Oracle Container Registry](https://container-registry.oracle.com/).

* To pull the images from Oracle Container Registry, create a Kubernetes secret named `ocr-reg-cred` in the `shns` namespace. For details, see [Creating an image pull secret](../container_reg_secret.md).
* If you plan to build and use the images, update the `dbImage` and `gsmImage` values to reference the images built in your environment.
* For prerequisites for Oracle Database and Global Data Services container images, see [Oracle Database and Global Data Services Docker Images](../../README.md#3-oracle-database-and-global-data-services-container-images).
* If you want to use the [Oracle AI Database 26ai Free](https://www.oracle.com/database/free/get-started/) image for the database and GSM, add the additional parameter `dbEdition: "free"` to the YAML manifest.

Use this manifest: [`ssharding_shard_prov_node_selection.yaml`](./ssharding_shard_prov_node_selection.yaml)

1. Deploy the `ssharding_shard_prov_node_selection.yaml` manifest:

    ```sh
    kubectl apply -f ssharding_shard_prov_node_selection.yaml
    ```

2. Check the status of the deployment:

    ```sh
    # Check the status of the Kubernetes pods:
    kubectl get all -n shns

    # View the logs for a specific pod (for example, "pshard1-0"):
    kubectl logs -f pod/pshard1-0 -n shns

    # Check the node assigned to the Kubernetes pods:
    kubectl get all -n shns -o wide    
    ```
