# Deploy Oracle GDD with System-Managed Sharding and Data Guard replication using a custom number of chunks

**IMPORTANT:** Make sure you have completed the steps for [Prerequisites for running Oracle Sharding Database Controller](../../README.md#prerequisites-for-running-oracle-sharding-database-controller) before using Oracle Sharding Controller.

In this use case, DBCA automatically creates the shard and catalog databases during provisioning.

**NOTE:** Because DBCA creates the databases during deployment, provisioning takes longer than when the databases are cloned from a Database Gold Image.

By default, System-Managed Sharding creates 120 chunks per shard database. For example, if the deployment contains two shard databases, a total of 240 chunks are created.

This example sets the total number of chunks by using the `CATALOG_CHUNKS` environment variable.

This example uses `ssharding_shard_prov_chunks.yaml` to provision an Oracle GDD system with the Oracle Sharding Controller using:

* Primary GSM pod: `gsm1`
* Standby GSM pod: `gsm2`
* Two shard database pods (`shardNum: 2`)
* One catalog database pod: `catalog`
* Total chunks: `120` (configured using `CATALOG_CHUNKS`, resulting in 60 chunks per shard)
* `shardingType: SYSTEM` (System-Managed Sharding)
* Replication type: Data Guard (replicationType: DG)
* Namespace: `shns`

This example uses pre-built Oracle Database and Global Data Services container images available from [Oracle Container Registry](https://container-registry.oracle.com/).

* To pull the images from Oracle Container Registry, create a Kubernetes secret named `ocr-reg-cred` in the `shns` namespace. For details, see [Creating an image pull secret](../container_reg_secret.md).
* If you plan to build and use the images, update the `dbImage` and `gsmImage` values to reference the images built in your environment.
* For prerequisites for Oracle Database and Global Data Services container images, see [Oracle Database and Global Data Services Docker Images](../../README.md#3-oracle-database-and-global-data-services-container-images).

Use this manifest: [`ssharding_shard_prov_chunks.yaml`](./ssharding_shard_prov_chunks.yaml)

1. Deploy the `ssharding_shard_prov_chunks.yaml` manifest:

    ```sh
    kubectl apply -f ssharding_shard_prov_chunks.yaml
    ```

2. Check the status of the deployment:

    ```sh
    # Check the status of the Kubernetes pods:
    kubectl get all -n shns

    # View the logs for a specific pod (for example, "pshard1-0"):
    kubectl logs -f pod/pshard1-0 -n shns
    ```

3. Verify using the following commands:

    ```sh
    # Switch to the primary GSM container:
    kubectl exec -i -t gsm1-0 -n shns /bin/bash

    # Check the status of the shards:
    gdsctl config shard

    # Check the status of the chunks:
    gdsctl config chunks
    ```
