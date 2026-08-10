# Scale in an existing Oracle GDD deployment with Composite Sharding and Native (Raft) replication

**IMPORTANT:** Make sure you have completed the steps for [Prerequisites for running Oracle Sharding Database Controller](../../README.md#prerequisites-for-running-oracle-sharding-database-controller) before using Oracle Sharding Controller.

This use case demonstrates how to delete a shard from an existing Oracle GDD deployment with Composite Sharding and Native (Raft) replication that was provisioned using the Oracle Sharding Controller.

**NOTE:** A shard is deleted only after all chunks have been moved out of it.

This example assumes the existing Oracle GDD deployment includes:

* Primary GSM pod: `gsm1`
* Standby GSM pod: `gsm2`
* Four shard database pods (`shardNum: 4`)
* One catalog database pod: `catalog`
* `shardingType: COMPOSITE` (Composite Sharding)
* Replication type: Native (Raft) (`replicationType: NATIVE`)
* Namespace: `shns`

This example uses pre-built Oracle Database and Global Data Services container images available from [Oracle Container Registry](https://container-registry.oracle.com/).

* To pull the images from Oracle Container Registry, create a Kubernetes secret named `ocr-reg-cred` in the `shns` namespace. For details, see [Creating an image pull secret](../container_reg_secret.md).
* If you plan to build and use the images, update the `dbImage` and `gsmImage` values to reference the images built in your environment.
* For prerequisites for Oracle Database and Global Data Services container images, see [Oracle Database and Global Data Services Docker Images](../../README.md#3-oracle-database-and-global-data-services-container-images).
* If you want to use the [Oracle AI Database 26ai Free](https://www.oracle.com/database/free/get-started/) image for the database and GSM, add the additional parameter `dbEdition: "free"` to the YAML manifest.

Scale in the deployment by changing `shardNum` from `4` to `3` in the manifest and then applying the updated configuration.

Use the following updated manifest:

[composite_shard_prov_delshard_native.yaml](./composite_shard_prov_delshard_native.yaml)

1. Deploy the `composite_shard_prov_delshard_native.yaml` manifest:

    ```sh
    kubectl apply -f composite_shard_prov_delshard_native.yaml
    ```

2. Check the status of the deployment:

    ```sh
    # Check the status of the Kubernetes pods:
    kubectl get all -n shns
    ```

    **NOTE:** After you apply `composite_shard_prov_delshard_native.yaml`, the change may not be visible immediately. The shard is removed only after all its chunks have been relocated and the required Replication Unit (RU) role changes have completed.

    To monitor the chunk movement, use the following command:

    ```sh
    # Switch to the primary GSM container:
    kubectl exec -i -t gsm1-0 -n shns /bin/bash

    # Check the chunk distribution. Repeat this command to monitor chunk relocation:
    gdsctl config chunks

    # Check the status of the Replication Units (RUs):
    gdsctl status ru
    ```

3. Verify using the following commands:

    ```sh
    # Switch to the primary GSM container:
    kubectl exec -i -t gsm1-0 -n shns /bin/bash

    # Check the status of the shards:
    gdsctl config shard

    # Check the status of the chunks:
    gdsctl config chunks

    # Check the status of the Replication Units (RUs):
    gdsctl status ru
    ```

    When the scale-in operation completes successfully, the removed shard no longer appears in the output of `gdsctl config shard`, and the corresponding Kubernetes pod is no longer listed by `kubectl get all -n shns`.
