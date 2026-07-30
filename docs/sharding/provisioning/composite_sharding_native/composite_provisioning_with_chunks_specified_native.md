# Deploy Oracle GDD with Composite Sharding and Native (Raft) replication using a custom number of chunks

**IMPORTANT:** Make sure you have completed the steps for [Prerequisites for running Oracle Sharding Database Controller](../../README.md#prerequisites-for-running-oracle-sharding-database-controller) before using Oracle Sharding Controller.

In this use case, DBCA automatically creates the shard and catalog databases during provisioning.

**NOTE:** Because DBCA creates the databases during deployment, provisioning takes longer than when the databases are cloned from a Database Gold Image.

By default, Composite Sharding with Native (Raft) replication creates 360 unique logical chunks for the Oracle GDD deployment. Because the default replication factor is 3, each logical chunk is replicated three times, resulting in 1,080 physical chunk instances (360 × 3) across the deployment.

This example customizes the number of unique logical chunks by setting the `CATALOG_CHUNKS` environment variable.

This example uses the `composite_shard_prov_chunks_native.yaml` manifest to provision an Oracle GDD deployment with the Oracle Sharding Controller using:

* Primary GSM pod: `gsm1`
* Standby GSM pod: `gsm2`
* Three shard database pods (`shardNum: 3`)
* One catalog database pod: `catalog`
* Total unique logical chunks: `120` (configured using `CATALOG_CHUNKS`)
* `shardingType: COMPOSITE` (Composite Sharding)
* Replication type: Native (Raft) (`replicationType: NATIVE`)
* Namespace: `shns`

This example uses pre-built Oracle Database and Global Data Services container images available from [Oracle Container Registry](https://container-registry.oracle.com/).

* To pull the images from Oracle Container Registry, create a Kubernetes secret named `ocr-reg-cred` in the `shns` namespace. For details, see [Creating an image pull secret](../container_reg_secret.md).
* If you plan to build and use the images, update the `dbImage` and `gsmImage` values to reference the images built in your environment.
* For prerequisites for Oracle Database and Global Data Services container images, see [Oracle Database and Global Data Services Docker Images](../../README.md#3-oracle-database-and-global-data-services-container-images).

Use the following manifest:

[`composite_shard_prov_chunks_native.yaml`](./composite_shard_prov_chunks_native.yaml)

1. Deploy the `composite_shard_prov_chunks_native.yaml` manifest:

    ```sh
    kubectl apply -f composite_shard_prov_chunks_native.yaml
    ```

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

    # Check the status of the Replication Units (RUs):
    gdsctl status ru

    # Check the details of the Oracle GDD database:
    gdsctl config sdb
    ```
