# Scale In - Delete an existing Shard from a working Oracle Sharded Database provisioned earlier with User Defined Sharding

**IMPORTANT:** Make sure you have completed the steps for [Prerequsites for Running Oracle Sharding Database Controller](../../README.md#prerequsites-for-running-oracle-sharding-database-controller) before using Oracle Sharding Controller.

This use case demonstrates how to delete an existing Shard from an existing Oracle Database sharding topology with User Defined Sharding provisioned using Oracle Database Sharding controller.

In this use case, the existing database Sharding is having:

* Primary GSM Pods `gsm1` and standby GSM Pod `gsm2`
* Five sharding Pods: `shard1`,`shard2`,`shard3`,`shard4` and `shard5`
* One Catalog Pod: `catalog`
* Namespace: `shns`
* User Defined Sharding is specified using `shardingType: USER`

In this example, we are using pre-built Oracle Database and Global Data Services container images available on [Oracle Container Registry](https://container-registry.oracle.com/)
  * To pull the above images from Oracle Container Registry, create a Kubernetes secret named `ocr-reg-cred` using your credentials with type set to `kubernetes.io/dockerconfigjson` in the namespace `shns`.
  * If you plan to use images built by you, you need to change `dbImage` and `gsmImage` tag with the images you have built in your enviornment in file `udsharding_shard_prov_delshard.yaml`.
  * To understand the Pre-requisite of Database and Global Data Services docker images, refer [Oracle Database and Global Data Services Docker Images](../../README.md#3-oracle-database-and-global-data-services-docker-images)
  * In case you want to use the [Oracle Database 23ai Free](https://www.oracle.com/database/free/get-started/) Image for Database and GSM, then you will need to add the additional parameter `dbEdition: "free"` to the below .yaml file. 
  * Make sure the version of `openssl` in the Oracle Database and Oracle GSM images is compatible with the `openssl` version on the machine where you will run the openssl commands to generated the encrypted password file during the deployment.

**NOTE:** Use tag `isDelete: enable` to delete the shard you want.

This use case deletes the shard `shard4` from the above Sharding Topology.

Use the file: [udsharding_shard_prov_delshard.yaml](./udsharding_shard_prov_delshard.yaml) for this use case as below:

1. Move out the chunks from the shard to be deleted to another shard. For example, in the current case, before deleting the `shard4`, if you want to move the chunks from `shard4` to `shard2`, then you can run the below `kubectl` command where `/u01/app/oracle/product/23ai/gsmhome_1` is the GSM HOME:
    ```sh
    kubectl exec -it pod/gsm1-0 -n shns -- /u01/app/oracle/product/23ai/gsmhome_1/bin/gdsctl "move chunk -chunk all -source shard4_shard4pdb -target shard4_shard4pdb"
    ```
2. Confirm the shard to be deleted (`shard4` in this case) is not having any chunk using below command:
    ```sh
    kubectl exec -it pod/gsm1-0 -n shns -- /u01/app/oracle/product/23ai/gsmhome_1/bin/gdsctl "config chunks"
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
- After you apply `udsharding_shard_prov_delshard.yaml`, the change may not be visible immediately and it may take some time for the delete operation to complete.
- If the shard, that you are trying to delete, is still having chunks, then the you will see message like below in the logs of the Oracle Database Operator Pod.
    ```sh
    INFO    controllers.database.ShardingDatabase   manual intervention required
    ```
  In this case, you will need to first move out the chunks from the shard to be deleted using Step 2 above and then apply the file in Step 3 to delete that shard.

To check the status, use the following command:
  ```sh
  # Switch to the primary GSM Container:
  kubectl exec -i -t gsm1-0 -n shns /bin/bash

  # Check the status shards:
  gdsctl config shard

  # Check the status of the chunks:
  gdsctl config chunks
  ```
