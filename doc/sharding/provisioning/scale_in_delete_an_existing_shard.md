# Scale In - Delete an existing Shard From a Working Oracle Database Sharding Topology

This use case demonstrates how to delete an existing Shard from an existing Oracle Database sharding topology provisioned using Oracle Database Sharding controller.

**NOTE** The deletion of a shard is done after verifying the Chunks have been moved out of that shard.

In this use case, the existing database Sharding is having:

* Primary GSM Pods `gsm1` and standby GSM Pod `gsm2`
* Five sharding Pods: `shard1`,`shard2`,`shard3`,`shard4` and `shard5`
* One Catalog Pod: `catalog`
* Namespace: `shns`

In this example, we are using pre-built Oracle Database and Global Data Services container images available on [Oracle Container Registry](https://container-registry.oracle.com/)
  * To pull the above images from Oracle Container Registry, create a Kubernetes secret named `ocr-reg-cred` using your credentials with type set to `kubernetes.io/dockerconfigjson` in the namespace `shns`.
  * If you plan to use images built by you, you need to change `dbImage` and `gsmImage` tag with the images you have built in your enviornment in file `shard_prov.yaml`.
  * To understand the Pre-requisite of Database and Global Data Services docker images, refer [Oracle Database and Global Data Services Docker Images](../ORACLE_SHARDING_CONTROLLER_README.md#3-oracle-database-and-global-data-services-docker-images)

NOTE: Use tag `isDelete: true` to delete the shard you want.

This use case deletes the shard `shard2` from the above Sharding Topology.

Use the file: [shard_prov_delshard.yaml](./doc/sharding/provisioning/shard_prov_delshard.yaml) for this use case as below:

1. Deploy the `shard_prov_delshard.yaml` file:
    ```sh
    kubectl apply -f shard_prov_delshard.yaml
    ```
2. Check the status of the deployment:
    ```sh
    # Check the status of the Kubernetes Pods:
    kubectl get all -n shns

**NOTE:** After you apply `shard_prov_delshard.yaml`, the change may not be visible immediately. When the shard is removed, first the chunks will be moved out of that shard that is going to be deleted.

To monitor the chunk movement, use the following command:

```sh
# Switch to the primary GSM Container:
kubectl exec -i -t gsm1-0 -n shns /bin/bash

# Check the status of the chunks and repeat to observe the chunk movement:
gdsctl config chunks
```
