# Scale In - Delete an existing Shard from a working Oracle Globally Distributed Database provisioned earlier with User-Defined Sharding

**IMPORTANT:** Make sure you have completed the steps for [Prerequisites for running Oracle Sharding Database Controller](../../README.md#prerequisites-for-running-oracle-sharding-database-controller) before using Oracle Sharding Controller.

This use case demonstrates how to delete an existing Shard from an existing Oracle Database sharding topology with User Defined Sharding provisioned using Oracle Database Sharding controller.

In this use case, the existing database Sharding has the following:

* Primary GSM Pods `gsm1` and standby GSM Pod `gsm2` 
* Five Shard Database Pods: `shard1`, `shard2`, `shard3`, `shard4` and `shard5` 
* One Catalog Database Pod: `catalog` 
* Namespace: `shns`
* User Defined Sharding is specified using `shardingType: USER`

In this example, we are using pre-built Oracle Database and Global Data Services container images available on the [Oracle Container Registry](https://container-registry.oracle.com/)
  * To pull the above images from Oracle Container Registry, create a Kubernetes secret named `ocr-reg-cred` in the namespace `shns`. Please refer to [this page](./../container_reg_secret.md) for the details.  
  * If you plan to build and use the images, then you need to change the `dbImage` and `gsmImage` tags with the images you have built in your enviornment in the file `udsharding_shard_prov_delshard.yaml`. 
  * To understand Database and Global Data Services Docker images prerequsites, see [Oracle Database and Global Data Services Docker Images](../../README.md#3-oracle-database-and-global-data-services-container-images) 
  * The version of `openssl` in the Oracle Database and Oracle GSM images must be compatible with the `openssl` version on the machine where you will run the openssl commands to generate the encrypted password file during the deployment. 

**NOTE:** Use the tag `isDelete: enable` to delete the shard that you want to remove.

This use case deletes the shard `shard4` from this Sharding Topology.

Use the file: [udsharding_shard_prov_delshard.yaml](./udsharding_shard_prov_delshard.yaml) for this use case, as described in the following steps:

1. Move out the chunks from the shard to be deleted to another shard. For example, in the current case, before deleting `shard4`, if you want to move the chunks from `shard4` to `shard2`, then you can run the `kubectl` command where `/u01/app/oracle/product/26ai/gsmhome_1` is the GSM HOME:
    ```sh
    kubectl exec -it pod/gsm1-0 -n shns -- /u01/app/oracle/product/26ai/gsmhome_1/bin/gdsctl "move chunk -chunk all -source shard4_shard4pdb -target shard4_shard4pdb"
    ```
2. To confirm that the shard that you want to be deleted (`shard4` in this case) does not have any chunks, use the following command:
    ```sh
    kubectl exec -it pod/gsm1-0 -n shns -- /u01/app/oracle/product/26ai/gsmhome_1/bin/gdsctl "config chunks"
    ```
    If there is no chunk present in the shard to be deleted, you can move to the next step.

3. Apply the `udsharding_shard_prov_delshard.yaml` file:
    ```sh
    kubectl apply -f udsharding_shard_prov_delshard.yaml
    ```
4. Check the status of the deployment:
    ```sh
    # Check the status of the Kubernetes Pods:
    kubectl get all -n shns
    ```

**NOTE:** 
- After you apply `udsharding_shard_prov_delshard.yaml`, the change may not be visible immediately. It can take some time for the delete operation to complete.
- If the shard that you are trying to delete still contains chunks, then a message such as the following is displayed in the logs of the Oracle Database Operator Pod:
    ```sh
    INFO    controllers.database.ShardingDatabase   manual intervention required
    ```
  When you see that message, you are required to first move the chunks out of the shard that you want to delete using Step 2 as described above, and then apply the file in Step 3 to delete that shard.

To check the status, use the following command:
  ```sh
  # Switch to the primary GSM Container:
  kubectl exec -i -t gsm1-0 -n shns /bin/bash

  # Check the status shards:
  gdsctl config shard

  # Check the status of the chunks:
  gdsctl config chunks
  ```
