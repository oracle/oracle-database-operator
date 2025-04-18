# Provisioning System-Managed Sharding Topology with Raft replication enabled with additional control on resources like Memory and CPU allocated to Pods

**NOTE: RAFT Replication Feature is available only for Oracle 23ai RDBMS and Oracle 23ai GSM version.**

**IMPORTANT:** Make sure you have completed the steps for [Prerequsites for Running Oracle Sharding Database Controller](../../README.md#prerequsites-for-running-oracle-sharding-database-controller) before using Oracle Sharding Controller.

In this use case, there are additional tags used to control resources such as CPU and Memory used by the different Pods when the Oracle Sharding topology with System-Managed with RAFT Replication is deployed using Oracle Sharding controller.

This example uses `snr_ssharding_shard_prov_memory_cpu.yaml` to provision an Oracle Database sharding topology using Oracle Sharding controller with:

* Primary GSM Pods `gsm1` and standby GSM Pod `gsm2`
* Three sharding Pods: `shard1`, `shard2` and `shard3`
* One Catalog Pod: `catalog`
* Namespace: `shns`
* Tags `memory` and `cpu`  to control the Memory and CPU of the PODs
* Additional tags `INIT_SGA_SIZE` and `INIT_PGA_SIZE` to control the SGA and PGA allocation at the database level
* `RAFT Replication` enabled

In this example, we are using pre-built Oracle Database and Global Data Services container images available on [Oracle Container Registry](https://container-registry.oracle.com/)
  * To pull the above images from Oracle Container Registry, create a Kubernetes secret named `ocr-reg-cred` using your credentials with type set to `kubernetes.io/dockerconfigjson` in the namespace `shns`.
  * If you plan to use images built by you, you need to change `dbImage` and `gsmImage` tag with the images you have built in your enviornment in file `snr_ssharding_shard_prov_memory_cpu.yaml`.
  * To understand the Pre-requisite of Database and Global Data Services docker images, refer [Oracle Database and Global Data Services Docker Images](../../README.md#3-oracle-database-and-global-data-services-docker-images)
  * In case you want to use the [Oracle Database 23ai Free](https://www.oracle.com/database/free/get-started/) Image for Database and GSM, then you will need to add the additional parameter `dbEdition: "free"` to the below .yaml file.
  * Make sure the version of `openssl` in the Oracle Database and Oracle GSM images is compatible with the `openssl` version on the machine where you will run the openssl commands to generated the encrypted password file during the deployment.

**NOTE:** For Oracle Database 23ai Free, you can control the `CPU` and `Memory` allocation of the PODs using tags `cpu` and `memory` respectively but tags `INIT_SGA_SIZE` and `INIT_PGA_SIZE` to control the SGA and PGA allocation at the database level are `not` supported.

Use the YAML file [snr_ssharding_shard_prov_memory_cpu.yaml](./snr_ssharding_shard_prov_memory_cpu.yaml).

1. Deploy the `snr_ssharding_shard_prov_memory_cpu.yaml` file:

    ```sh
    kubectl apply -f snr_ssharding_shard_prov_memory_cpu.yaml
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
