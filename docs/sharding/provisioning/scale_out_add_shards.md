# Scale Out - Add Shards to an existing Oracle Database Sharding Topology

This use case demonstrates adding a new shard to an existing Oracle Database sharding topology provisioned earlier using Oracle Database Sharding controller.

In this use case, the existing Oracle Database sharding topology is having:

* Primary GSM Pods `gsm1` and standby GSM Pod `gsm2`
* Two sharding Pods: `shard1` and `shard2`
* One Catalog Pod: `catalog`
* Namespace: `shns`

In this example, we are using pre-built Oracle Database and Global Data Services container images available on [Oracle Container Registry](https://container-registry.oracle.com/)
  * To pull the above images from Oracle Container Registry, create a Kubernetes secret named `ocr-reg-cred` using your credentials with type set to `kubernetes.io/dockerconfigjson` in the namespace `shns`.
  * If you plan to use images built by you, you need to change `dbImage` and `gsmImage` tag with the images you have built in your enviornment in file `shard_prov.yaml`.
  * To understand the Pre-requisite of Database and Global Data Services docker images, refer [Oracle Database and Global Data Services Docker Images](../README.md#3-oracle-database-and-global-data-services-docker-images)

This use case adds three new shards `shard3`,`shard4`,`shard4` to above Sharding Topology.

Use the file: [shard_prov_extshard.yaml](./shard_prov_extshard.yaml) for this use case as below:

1. Deploy the `shard_prov_extshard.yaml` file:
    ```sh
    kubectl apply -f shard_prov_extshard.yaml
    ```
2. Check the status of the deployment:
    ```sh
    # Check the status of the Kubernetes Pods:
    kubectl get all -n shns

    # Check the logs of a particular pod. For example, to check status of pod "shard3-0":
    kubectl logs -f pod/shard3-0 -n shns
