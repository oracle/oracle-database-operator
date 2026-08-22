# Deploy Oracle GDD with System-Managed Sharding with Custom CPU and Memory Resources

**IMPORTANT:** Make sure you have completed the steps for [Prerequisites for running Oracle Sharding Database Controller](../../README.md#prerequisites-for-running-oracle-sharding-database-controller) before using Oracle Sharding Controller.

In this use case, additional parameters are used to configure the CPU and memory resources for the catalog, shard, and gsm pods.

This example uses `ssharding_shard_prov_memory_cpu.yaml` to provision an Oracle GDD deployment with the Oracle Sharding Controller using:

* Primary GSM pod: `gsm1`
* Standby GSM pod: `gsm2`
* Two shard database pods (`shardNum: 2`)
* One catalog database pod: `catalog`
* Custom CPU and memory settings for the catalog, shard, and gsm pods
* `INIT_SGA_SIZE` and `INIT_PGA_SIZE` environment variables to configure the database SGA and PGA sizes
* Namespace: `shns`

This example uses pre-built Oracle Database and Global Data Services container images available from [Oracle Container Registry](https://container-registry.oracle.com/).

* To pull the images from Oracle Container Registry, create a Kubernetes secret named `ocr-reg-cred` in the `shns` namespace. For details, see [Creating an image pull secret](../container_reg_secret.md).
* If you plan to build and use the images, update the `dbImage` and `gsmImage` values to reference the images built in your environment.
* For prerequisites for Oracle Database and Global Data Services container images, see [Oracle Database and Global Data Services Docker Images](../../README.md#3-oracle-database-and-global-data-services-container-images).
* If you want to use the [Oracle AI Database 26ai Free](https://www.oracle.com/database/free/get-started/) image for the database and GSM, add the additional parameter `dbEdition: "free"` to the YAML manifest.

**NOTE:** When using Oracle AI Database 26ai Free, you can configure pod CPU and memory resources using the `cpu` and `memory` parameters. However, the `INIT_SGA_SIZE` and `INIT_PGA_SIZE` environment variables are not supported.

Use the following manifest:

[`ssharding_shard_prov_memory_cpu.yaml`](./ssharding_shard_prov_memory_cpu.yaml)

1. Deploy the `ssharding_shard_prov_memory_cpu.yaml` manifest:

    ```sh
    kubectl apply -f ssharding_shard_prov_memory_cpu.yaml
    ```

2. Check the status of the deployment:

    ```sh
    kubectl get all -n shns
    ```

3. Monitor the database creation logs for the catalog and shard database pods. For example:

    ```sh
    # Catalog database pod
    kubectl logs -f pod/catalog-0 -n shns

    # Shard database pod
    kubectl logs -f pod/pshard1-0 -n shns
    ```

    Database creation can take approximately 20 minutes. For the catalog database pod and each shard database pod, wait for the following message, which indicates that the database is ready to use:

    ```text
    #########################
    DATABASE IS READY TO USE!
    #########################
    ```

    Repeat this check for each shard database pod in the deployment.

4. After the databases are ready, monitor the sharding setup log for the catalog database pod:

    ```sh
    kubectl exec pod/catalog-0 -n shns -- \
      /bin/bash -c "tail -f /var/tmp/gdd/oracle_sharding_setup.log"
    ```

    Wait for the following message, which indicates that the catalog setup is complete:

    ```text
    ==============================================
         GSM Catalog Setup Completed
    ==============================================
    ```

    Monitor the sharding setup log for each shard database pod. For example, for `pshard1-0`:

    ```sh
    kubectl exec pod/pshard1-0 -n shns -- \
      /bin/bash -c "tail -f /var/tmp/gdd/oracle_sharding_setup.log"
    ```

    Wait for the following message, which indicates that the shard setup is complete:

    ```text
    ==============================================
         GSM Shard Setup Completed
    ==============================================
    ```

    Repeat this check for each shard database pod in the deployment.

5. Monitor the sharding setup log for each GSM pod. For example, for the primary GSM pod `gsm1-0`:

    ```sh
    kubectl exec pod/gsm1-0 -n shns -- \
      /bin/bash -c "tail -f /var/tmp/gdd/oracle_sharding_setup.log"
    ```

    For each GSM pod, wait for the following message, which indicates that the GSM setup is complete:

    ```text
    ==============================================
         GSM Setup Completed
    ==============================================
    ```

6. Check the details of a pod. For example, to check the details of pod `pshard1-0`:

    ```sh
    kubectl describe pod/pshard1-0 -n shns
    ```
