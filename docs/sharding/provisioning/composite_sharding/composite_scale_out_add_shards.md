# Scale out an existing Oracle GDD deployment with Composite Sharding and Data Guard replication

**IMPORTANT:** Make sure you have completed the steps for [Prerequisites for running Oracle Sharding Database Controller](../../README.md#prerequisites-for-running-oracle-sharding-database-controller) before using Oracle Sharding Controller.

This use case demonstrates how to add a new shard to an existing Oracle GDD deployment with Composite Sharding and Data Guard replication that was provisioned using the Oracle Sharding Controller.

This example assumes the existing Oracle GDD deployment includes:

* Primary GSM pod: `gsm1`
* Standby GSM pod: `gsm2`
* Two shard database pods (`shardNum: 2`)
* One catalog database pod: `catalog`
* `shardingType: COMPOSITE` (Composite Sharding)
* Replication type: Data Guard (replicationType: DG)
* Namespace: `shns`

This example uses pre-built Oracle Database and Global Data Services container images available from [Oracle Container Registry](https://container-registry.oracle.com/).

* To pull the images from Oracle Container Registry, create a Kubernetes secret named `ocr-reg-cred` in the `shns` namespace. For details, see [Creating an image pull secret](../container_reg_secret.md).
* If you plan to build and use the images, update the `dbImage` and `gsmImage` values to reference the images built in your environment.
* For prerequisites for Oracle Database and Global Data Services container images, see [Oracle Database and Global Data Services Docker Images](../../README.md#3-oracle-database-and-global-data-services-container-images).
* If you want to use the [Oracle AI Database 26ai Free](https://www.oracle.com/database/free/get-started/) image for the database and GSM, add the additional parameter `dbEdition: "free"` to the YAML manifest.

Scale out the deployment by changing `shardNum` from `2` to `3` in the manifest and applying the updated configuration.

Use the following updated manifest:

[composite_shard_prov_extshard.yaml](./composite_shard_prov_extshard.yaml)

1. Deploy the updated `composite_shard_prov_extshard.yaml` manifest:

    ```sh
    kubectl apply -f composite_shard_prov_extshard.yaml
    ```

    **Note:** Applying the updated manifest triggers the Oracle Sharding Controller to reconcile the deployment and provision the additional shard automatically.

2. Check the status of the deployment:

    ```sh
    # Check the status of the Kubernetes pods:
    kubectl get all -n shns

    # View the logs for a specific pod (for example, "cpsp1-0"):
    kubectl logs -f pod/cpsp1-0 -n shns
    ```

3. Verify using the following commands:

    ```sh
    # Switch to the primary GSM container:
    kubectl exec -i -t gsm1-0 -n shns /bin/bash

    # Check the status of the shards:
    gdsctl config shard

    # Check the status of the chunks:
    gdsctl config chunks

    # Check the details of the Oracle GDD database:
    gdsctl config sdb
    ```

    When the scale-out operation completes successfully, the newly added shard appears in the output of `gdsctl config shard`, and the corresponding Kubernetes pod (`cpsp3-0`) is in the `Running` state.
