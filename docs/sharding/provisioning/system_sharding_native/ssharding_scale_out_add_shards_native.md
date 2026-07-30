# Scale out an existing Oracle GDD deployment with System-Managed Sharding and Native (Raft) replication

**IMPORTANT:** Make sure you have completed the steps for [Prerequisites for running Oracle Sharding Database Controller](../../README.md#prerequisites-for-running-oracle-sharding-database-controller) before using Oracle Sharding Controller.

This use case demonstrates how to add a new shard to an existing Oracle GDD deployment with System-Managed Sharding and Native (Raft) replication that was provisioned using the Oracle Sharding Controller.

This example assumes the existing Oracle GDD deployment includes:

* Primary GSM pod: `gsm1`
* Standby GSM pod: `gsm2`
* Three shard database pods (`shardNum: 3`)
* One catalog database pod: `catalog`
* `shardingType: SYSTEM` (System-Managed Sharding)
* Replication type: Native (Raft) (`replicationType: NATIVE`)
* Namespace: `shns`

This example uses pre-built Oracle Database and Global Data Services container images available from [Oracle Container Registry](https://container-registry.oracle.com/).

* To pull the images from Oracle Container Registry, create a Kubernetes secret named `ocr-reg-cred` in the `shns` namespace. For details, see [Creating an image pull secret](../container_reg_secret.md).
* If you plan to build and use the images, update the `dbImage` and `gsmImage` values to reference the images built in your environment.
* For prerequisites for Oracle Database and Global Data Services container images, see [Oracle Database and Global Data Services Docker Images](../../README.md#3-oracle-database-and-global-data-services-container-images).
* If you want to use the [Oracle AI Database 26ai Free](https://www.oracle.com/database/free/get-started/) image for the database and GSM, add the additional parameter `dbEdition: "free"` to the YAML manifest.

Scale out the deployment by changing `shardNum` from `3` to `5` in the manifest and applying the updated configuration. The Oracle Sharding Controller provisions two additional shard database pods and adds them to the existing Oracle GDD deployment.

Use the following updated manifest for this deployment:

[ssharding_shard_prov_extshard_native.yaml](./ssharding_shard_prov_extshard_native.yaml)

1. Deploy the updated `ssharding_shard_prov_extshard_native.yaml` manifest:

    ```sh
    kubectl apply -f ssharding_shard_prov_extshard_native.yaml
    ```

   **Note:** Applying the updated manifest triggers the Oracle Sharding Controller to reconcile the deployment and provision the additional shards automatically.

2. Check the status of the deployment:

    ```sh
    # Check the status of the Kubernetes pods:
    kubectl get all -n shns

    # View the logs for a specific pod (for example, "pshard4-0"):
    kubectl logs -f pod/pshard4-0 -n shns
    ```

    When provisioning completes successfully, the newly added shard pods (`pshard4-0` and `pshard5-0`) should be in the `Running` state.
