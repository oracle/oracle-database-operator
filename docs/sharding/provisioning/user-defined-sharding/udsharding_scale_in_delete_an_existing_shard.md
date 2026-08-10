# Scale in an existing Oracle GDD deployment with User-Defined Sharding

**IMPORTANT:** Make sure you have completed the steps for [Prerequisites for running Oracle Sharding Database Controller](../../README.md#prerequisites-for-running-oracle-sharding-database-controller) before using Oracle Sharding Controller.

This use case demonstrates how to delete a shard from an existing Oracle GDD deployment with User-Defined Sharding that was provisioned using the Oracle Sharding Controller.

This example assumes that the existing Oracle GDD deployment includes:

* Primary GSM pod: `gsm1`
* Standby GSM pod: `gsm2`
* Three Shard Database Pods: `pshard1`, `pshard2` and `pshard3`
* `shardingType: USER` (User-Defined Sharding)
* One catalog database pod: `catalog`
* Namespace: `shns`

This example uses pre-built Oracle Database and Global Data Services container images available from [Oracle Container Registry](https://container-registry.oracle.com/).

* To pull the images from Oracle Container Registry, create a Kubernetes secret named `ocr-reg-cred` in the `shns` namespace. For details, see [Creating an image pull secret](../container_reg_secret.md).
* If you plan to build and use the images, update the `dbImage` and `gsmImage` values to reference the images built in your environment.
* For prerequisites for Oracle Database and Global Data Services container images, see [Oracle Database and Global Data Services Docker Images](../../README.md#3-oracle-database-and-global-data-services-container-images).
* If you want to use the [Oracle AI Database 26ai Free](https://www.oracle.com/database/free/get-started/) image for the database and GSM, add the additional parameter `dbEdition: "free"` to the YAML manifest.

**NOTE:** Set `isDelete: enable` for the shard you want to remove.

This use case deletes the shard `pshard2` from this sharding topology.

Update your existing manifest or use the following updated manifest: [udsharding_shard_prov_delshard.yaml](./udsharding_shard_prov_delshard.yaml)

1. Move out the chunks from the shard to be deleted to another shard. For example, in the current case, before deleting `pshard2`, if you want to move the chunks from `pshard2` to `pshard3`, then run the following `kubectl` command, where `/u01/app/oracle/product/26ai/gsmhome_1` is the GSM HOME:

    ```sh
    kubectl exec -it pod/gsm1-0 -n shns -- /u01/app/oracle/product/26ai/gsmhome_1/bin/gdsctl "move chunk -chunk all -source pshard2_pshard2pdb -target pshard3_pshard3pdb"
    ```

2. To confirm that the shard that you want to be deleted (`pshard2` in this case) does not have any chunks, use the following command:

    ```sh
    kubectl exec -it pod/gsm1-0 -n shns -- /u01/app/oracle/product/26ai/gsmhome_1/bin/gdsctl "config chunks"
    ```

    If there are no chunks in the shard to be deleted, proceed to the next step.

3. Apply the `udsharding_shard_prov_delshard.yaml` file:

    ```sh
    kubectl apply -f udsharding_shard_prov_delshard.yaml
    ```

4. Check the status of the deployment:

    ```sh
    # Check the status of the Kubernetes Pods:
    kubectl get all -n shns
    ```

   **NOTE:**

    * After you apply `udsharding_shard_prov_delshard.yaml`, the change may not be visible immediately. It can take some time for the delete operation to complete.
    * If the shard still contains chunks, the Oracle Database Operator logs will show the following message:

      ```sh
      INFO    controllers.database.ShardingDatabase   manual intervention required
      ```

      In that case, move the chunks out of the shard first, then reapply the manifest to delete the shard.

5. Verify using the following commands:

   ```sh
   # Switch to the primary GSM container:
   kubectl exec -i -t gsm1-0 -n shns /bin/bash

   # Check the status of the shards:
   gdsctl config shard

   # Check the status of the chunks:
   gdsctl config chunks
   ```
