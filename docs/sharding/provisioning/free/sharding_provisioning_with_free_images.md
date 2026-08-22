# Deploy Oracle GDD with System-Managed Sharding using Oracle AI Database 26ai Free Database and GSM Images

**IMPORTANT:** Make sure you have completed the steps for [Prerequisites for running Oracle Sharding Database Controller](../../README.md#prerequisites-for-running-oracle-sharding-database-controller) before using Oracle Sharding Controller.

This example deploys an Oracle GDD topology with System-Managed Sharding using Oracle AI Database 26ai Free Database and GSM images.

This example uses the `sharding_provisioning_with_free_images.yaml` manifest to provision an Oracle GDD deployment with the Oracle Sharding Controller using:

* Primary GSM pod: `gsm1`
* Standby GSM pod: `gsm2`
* Two shard database pods (`shardNum: 2`)
* One catalog database pod: `catalog`
* Namespace: `shns`

To get the Oracle AI Database 26ai Free Database and GSM Images:

* The Oracle AI Database 26ai Free image used is `container-registry.oracle.com/database/free:latest`. Check [Oracle AI Database Free Get Started](https://www.oracle.com/database/free/get-started/?source=v0-DBFree-ChatCTA-j2032-20240709) for details.
* The Oracle AI Database 26ai GSM image used is `container-registry.oracle.com/database/gsm:latest`.
* To pull the above images from Oracle Container Registry, create a Kubernetes secret named `ocr-reg-cred` in the namespace `shns`. Please refer to [this page](./../container_reg_secret.md) for the details.
* You need to update the `dbImage` and `gsmImage` tag with the images you want to use in your environment in file `sharding_provisioning_with_free_images.yaml`.  

Use the following manifest:

[sharding_provisioning_with_free_images.yaml](./sharding_provisioning_with_free_images.yaml)

1. Deploy the `sharding_provisioning_with_free_images.yaml` manifest:

    ```sh
    kubectl apply -f sharding_provisioning_with_free_images.yaml
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

**NOTE:** Native (Raft) replication requires at least three shards. Oracle AI Database 26ai Free supports a maximum of three shards. For licensing details and feature limitations, see the [Oracle AI Database Licensing Information](https://docs.oracle.com/en/database/oracle/oracle-database/26/dblic/Licensing-Information.html).