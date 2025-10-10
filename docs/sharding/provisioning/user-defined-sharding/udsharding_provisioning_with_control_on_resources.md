# Provisioning Oracle Sharded Database with User Defined Sharding with additional control on resources like Memory and CPU allocated to Pods

**IMPORTANT:** Make sure you have completed the steps for [Prerequsites for Running Oracle Sharding Database Controller](../../README.md#prerequsites-for-running-oracle-sharding-database-controller) before using Oracle Sharding Controller.

In this use case, there are additional tags used to control resources such as CPU and Memory used by the different Pods when the Oracle Sharding topology with User Defined Sharding is deployed using Oracle Sharding controller.

This example uses `udsharding_shard_prov_memory_cpu.yaml` to provision an Oracle Database sharding topology using Oracle Sharding controller with:

* Primary GSM Pods `gsm1` and standby GSM Pod `gsm2`
* Three sharding Pods: `shard1`, `shard2` and `shard3`
* One Catalog Pod: `catalog`
* Namespace: `shns`
* Tags `memory` and `cpu`  to control the Memory and CPU of the PODs
* Additional tags `INIT_SGA_SIZE` and `INIT_PGA_SIZE` to control the SGA and PGA allocation at the database level
* User Defined Sharding is specified using `shardingType: USER`

In this example, we are using pre-built Oracle Database and Global Data Services container images available on the [Oracle Container Registry](https://container-registry.oracle.com/)
  * To pull the above images from Oracle Container Registry, create a Kubernetes secret named `ocr-reg-cred` using your credentials with type set to `kubernetes.io/dockerconfigjson` in the namespace `shns`.
  * If you plan to use images built by you, then you must change the `dbImage` and `gsmImage` tags with the images that you have built in your enviornment in file `udsharding_shard_prov_memory_cpu.yaml`.
  * To understand the Database and Global Data Services Docker images prerequisites, see: [Oracle Database and Global Data Services Docker Images](../../README.md#3-oracle-database-and-global-data-services-docker-images)
  * If you want to use the [Oracle Database 23ai Free image](https://www.oracle.com/database/free/get-started/) for Database and GSM, then you must add the additional parameter `dbEdition: "free"` to the `.yaml` file used with this procedure. 
  * Ensure that the version of `openssl` in the Oracle Database and Oracle GSM images is compatible with the `openssl` version on the machine where you will run the openssl commands to generated the encrypted password file during the deployment.

**NOTE:** For Oracle Database 23ai Free, you can control the `CPU` and `Memory` allocation of the PODs by using tags `cpu` and `memory` respectively, but tags `INIT_SGA_SIZE` and `INIT_PGA_SIZE` to control the SGA and PGA allocation at the database level are _not_ supported.

Use the YAML file [udsharding_shard_prov_memory_cpu.yaml](./udsharding_shard_prov_memory_cpu.yaml).

1. Deploy the `udsharding_shard_prov_memory_cpu.yaml` file:

    ```sh
    kubectl apply -f udsharding_shard_prov_memory_cpu.yaml
    ```

1. Check the details of a POD. For example: To check the details of Pod `shard1-0`:

    ```sh
    kubectl describe pod/shard1-0 -n shns
    ```
3. Check the status of the deployment:
    ```sh
    # Check the status of the Kubernetes Pods:
    kubectl get all -n shns

    # Check the logs of a particular pod. For example, to check status of pod "shard1-0":
    kubectl logs -f pod/shard1-0 -n shns
    ```
