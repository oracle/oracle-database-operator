# Scale In - Delete an existing Shard from a working Oracle Globally Distributed Database provisioned earlier with System-Managed Sharding

**IMPORTANT:** Make sure you have completed the steps for [Prerequisites for running Oracle Sharding Database Controller](../../README.md#prerequisites-for-running-oracle-sharding-database-controller) before using Oracle Sharding Controller. 

This use case demonstrates how to delete an existing Shard from an existing Oracle Database sharding topology with System-Managed Sharding provisioned using Oracle Database Sharding controller.

**NOTE** The deletion of a shard is done after verifying the Chunks have been moved out of that shard.

In this use case, the existing database Sharding is having:

* Primary GSM Pods `gsm1` and standby GSM Pod `gsm2`
* Five Shard Database Pods: `shard1`, `shard2`, `shard3`, `shard4` and `shard5`
* One Catalog Database Pod: `catalog`
* Namespace: `shns`

In this example, we are using pre-built Oracle Database and Global Data Services container images available on [Oracle Container Registry](https://container-registry.oracle.com/)
  * To pull the above images from Oracle Container Registry, create a Kubernetes secret named `ocr-reg-cred` in the namespace `shns`. Please refer to [this page](./../container_reg_secret.md) for the details. 
  * If you plan to build and use the images, you need to change `dbImage` and `gsmImage` tag with the images you have built in your enviornment in file `ssharding_shard_prov_delshard.yaml`. 
  * To understand Database and Global Data Services Docker images prerequsites, see [Oracle Database and Global Data Services Docker Images](../../README.md#3-oracle-database-and-global-data-services-container-images) 
  * The version of `openssl` in the Oracle Database and Oracle GSM images must be compatible with the `openssl` version on the machine where you will run the openssl commands to generate the encrypted password file during the deployment. 

NOTE: Use tag `isDelete: enable` to delete the shard you want.

This use case deletes the shard `shard4` from the above Sharding Topology.

Use the file: [ssharding_shard_prov_delshard.yaml](./ssharding_shard_prov_delshard.yaml) for this use case as below:

1. Deploy the `ssharding_shard_prov_delshard.yaml` file:
    ```sh
    kubectl apply -f ssharding_shard_prov_delshard.yaml
    ```
2. Check the status of the deployment:
    ```sh
    # Check the status of the Kubernetes Pods:
    kubectl get all -n shns

**NOTE:** After you apply `ssharding_shard_prov_delshard.yaml`, the change may not be visible immediately. First the chunks will be moved out of that shard that is going to be deleted and then the shard will be removed from the topology.

To monitor the chunk movement, use the following command:

```sh
# Switch to the primary GSM Container:
kubectl exec -i -t gsm1-0 -n shns /bin/bash

# Check the status of the chunks and repeat to observe the chunk movement:
gdsctl config chunks
```
