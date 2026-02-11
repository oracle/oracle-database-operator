# Scale Out - Add Shards to an existing Oracle Globally Distributed Database provisioned earlier with User-Defined Sharding

**IMPORTANT:** Make sure you have completed the steps for [Prerequisites for running Oracle Sharding Database Controller](../../README.md#prerequisites-for-running-oracle-sharding-database-controller) before using Oracle Sharding Controller. 

This use case demonstrates adding a new shard to an existing Oracle Database sharding topology with User Defined Sharding provisioned earlier using Oracle Database Sharding controller.

In this use case, the existing Oracle Database sharding topology is as follows:

* Primary GSM Pods `gsm1` and standby GSM Pod `gsm2` 
* Three Shard Database Pods: `shard1`, `shard2` and `shard3` 
* One Catalog Database Pod: `catalog` 
* Namespace: `shns` 
* User Defined Sharding is specified using `shardingType: USER` 

In this example, we are using pre-built Oracle Database and Global Data Services container images available on the [Oracle Container Registry](https://container-registry.oracle.com/)
  * To pull the above images from Oracle Container Registry, create a Kubernetes secret named `ocr-reg-cred` in the namespace `shns`. Please refer to [this page](./../container_reg_secret.md) for the details. 
  * If you plan to build and use the images, then you must exchange the `dbImage` and `gsmImage` tags for the images that you have built in your enviornment in file `udsharding_shard_prov_extshard.yaml`.
  * To understand Database and Global Data Services Docker images prerequsites, see [Oracle Database and Global Data Services Docker Images](../../README.md#3-oracle-database-and-global-data-services-container-images) 
  * The version of `openssl` in the Oracle Database and Oracle GSM images must be compatible with the `openssl` version on the machine where you will run the openssl commands to generate the encrypted password file during the deployment. 

This use case adds two new shards `shard4`,`shard5` to the Sharding Topology.

Use the file: [udsharding_shard_prov_extshard.yaml](./udsharding_shard_prov_extshard.yaml) for this use case:

1. Deploy the `udsharding_shard_prov_extshard.yaml` file:
    ```sh
    kubectl apply -f udsharding_shard_prov_extshard.yaml
    ```
2. Check the status of the deployment:
    ```sh
    # Check the status of the Kubernetes Pods:
    kubectl get all -n shns

    # Check the logs of a particular pod. For example, to check status of pod "shard4-0":
    kubectl logs -f pod/shard4-0 -n shns
