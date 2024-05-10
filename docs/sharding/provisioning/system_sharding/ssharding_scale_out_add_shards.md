# Scale Out - Add Shards to an existing Oracle Sharded Database provisioned earlier with System Sharding

**IMPORTANT:** Make sure you have completed the steps for [Prerequsites for Running Oracle Sharding Database Controller](../../README.md#prerequsites-for-running-oracle-sharding-database-controller) before using Oracle Sharding Controller.

This use case demonstrates adding a new shard to an existing Oracle Database sharding topology with System Sharding provisioned earlier using Oracle Database Sharding controller.

In this use case, the existing Oracle Database sharding topology is having:

* Primary GSM Pods `gsm1` and standby GSM Pod `gsm2`
* Three sharding Pods: `shard1`, `shard2` and `shard3`
* One Catalog Pod: `catalog`
* Namespace: `shns`

In this example, we are using pre-built Oracle Database and Global Data Services container images available on [Oracle Container Registry](https://container-registry.oracle.com/)
  * To pull the above images from Oracle Container Registry, create a Kubernetes secret named `ocr-reg-cred` using your credentials with type set to `kubernetes.io/dockerconfigjson` in the namespace `shns`.
  * If you plan to use images built by you, you need to change `dbImage` and `gsmImage` tag with the images you have built in your enviornment in file `ssharding_shard_prov_extshard.yaml`.
  * To understand the Pre-requisite of Database and Global Data Services docker images, refer [Oracle Database and Global Data Services Docker Images](../../README.md#3-oracle-database-and-global-data-services-docker-images)

This use case adds two new shards `shard4`,`shard5` to above Sharding Topology.

Use the file: [ssharding_shard_prov_extshard.yaml](./ssharding_shard_prov_extshard.yaml) for this use case as below:

1. Deploy the `ssharding_shard_prov_extshard.yaml` file:
    ```sh
    kubectl apply -f ssharding_shard_prov_extshard.yaml
    ```
2. Check the status of the deployment:
    ```sh
    # Check the status of the Kubernetes Pods:
    kubectl get all -n shns

    # Check the logs of a particular pod. For example, to check status of pod "shard4-0":
    kubectl logs -f pod/shard4-0 -n shns
