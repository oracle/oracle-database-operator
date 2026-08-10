# Scale out an existing Oracle GDD deployment with User-Defined Sharding

**IMPORTANT:** Make sure you have completed the steps for [Prerequisites for running Oracle Sharding Database Controller](../../README.md#prerequisites-for-running-oracle-sharding-database-controller) before using Oracle Sharding Controller.

This use case demonstrates how to scale out an existing Oracle GDD deployment by adding a new shard to a User-Defined Sharding topology that was provisioned using the Oracle Sharding Controller.

This example assumes that the existing Oracle GDD deployment includes:

* Primary GSM pod: `gsm1`
* Standby GSM pod: `gsm2`
* Two Shard Database Pods: `pshard1` and `pshard2`
* `shardingType: USER` (User-Defined Sharding)
* One catalog database pod: `catalog`
* Namespace: `shns`

This example uses pre-built Oracle Database and Global Data Services container images available from [Oracle Container Registry](https://container-registry.oracle.com/).

* To pull the images from Oracle Container Registry, create a Kubernetes secret named `ocr-reg-cred` in the `shns` namespace. For details, see [Creating an image pull secret](../container_reg_secret.md).
* If you plan to build and use the images, update the `dbImage` and `gsmImage` values to reference the images built in your environment.
* For prerequisites for Oracle Database and Global Data Services container images, see [Oracle Database and Global Data Services Docker Images](../../README.md#3-oracle-database-and-global-data-services-container-images).
* If you want to use the [Oracle AI Database 26ai Free](https://www.oracle.com/database/free/get-started/) image for the database and GSM, add the additional parameter `dbEdition: "free"` to the YAML manifest.

Scale out the deployment by adding `pshard3` to the Oracle GDD deployment.

To add `pshard3`, update the manifest to include the definition for pshard3, and then apply the updated manifest.

Update your existing manifest or use the following updated manifest: [udsharding_shard_prov_extshard.yaml](./udsharding_shard_prov_extshard.yaml)

1. Deploy the `udsharding_shard_prov_extshard.yaml` file:

    ```sh
    kubectl apply -f udsharding_shard_prov_extshard.yaml
    ```

    **Note:** Applying the updated manifest triggers the Oracle Sharding Controller to reconcile the deployment and provision the new shard automatically.

2. Check the status of the deployment:

    ```sh
    # Check the status of the Kubernetes Pods:
    kubectl get all -n shns

    # View the logs for a newly added pod (for example, "pshard3-0"):
    kubectl logs -f pod/pshard3-0 -n shns
    ```

    When provisioning completes successfully, the `pshard3-0` pod should be in the `Running` state.

**Note:** For User-Defined Sharding, the newly added shard does not automatically receive existing chunks or data. Existing chunks remain on their current shards until they are manually reassigned or migrated using Oracle Sharding administration tools.
