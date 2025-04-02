# Provisioning System managed Sharding Topology with Raft replication enabled by cloning database from your own Database Gold Image across Availability Domains(ADs)

**NOTE: RAFT Replication Feature is available only for Oracle 23ai RDBMS and Oracle 23ai GSM version.**

**IMPORTANT:** Make sure you have completed the steps for [Prerequsites for Running Oracle Sharding Database Controller](../../README.md#prerequsites-for-running-oracle-sharding-database-controller) before using Oracle Sharding Controller.

In this test case, you provision the System managed Sharding Topology with Raft replication enabled while provisioning the Catalog and Shard Databases by cloning from an existing Oracle Database Gold Image created earlier.

This use case applies when you want to provision the database Pods on a Kubernetes Node in any availability domain (AD), which can also be different from the availability domain (AD) of the Block Volume that has the Oracle Database Gold Image provisioned earlier.

Choosing this option takes substantially less time during the Oracle Database Sharding Topology setup across ADs.

NOTE:

* Cloning from Block Volume Backup in OCI enables the new Persistent Volumes to be created in other ADs.
* To specify the AD where you want to provision the database Pod, use the tag `nodeSelector` and the POD will be provisioned in a node running in that AD.
* To specify GSM containers, you can also use the tag `nodeSelector` to specify the AD.
* Before you can provision with the Gold Image, you need the OCID of the Persistent Volume that has the Oracle Database Gold Image.

1. Check the OCID of the Persistent Volume provisioned for the Oracle Database Gold Image:
    ```sh
    kubectl get pv -n shns
    ```
2. Create a Block Volume Backup for this Block Volume, and use the OCID of the Block Volume Backup in the next step. This example uses `snr_ssharding_shard_prov_clone_across_ads.yaml` to provision an Oracle Database sharding topology using Oracle Sharding controller with:

* Primary GSM Pods `gsm1` and standby GSM Pod `gsm2`
* Three sharding Pods: `shard1`, `shard2` and `shard3`
* One Catalog Pod: `catalog`
* Namespace: `shns`
* Database Cloning from the `BLOCK VOLUME FULL BACKUP` of the Persistent Volume which had the Gold Image.
* OCID of the Block Volume Backup: `ocid1.volumebackup.oc1.phx.abyhqljrxtv7tu5swqb3lzc7vpzwbwzdktd2y4k2vjjy2srmgu2w7bqdftjq`
* `RAFT Replication` enabled

NOTE: In this case, the Persistent Volume with DB Gold Image was provisioned in the Availability Domain `PHX-AD-1`. The Shards and Catalog will be provisioned across multiple Availability Domains by cloning the database.

**NOTE:** Provisioning the Sharded Database using Cloning from Database Gold Image is `NOT` supported with Oracle Database 23ai Free.

In this example, we are using pre-built Oracle Database and Global Data Services container images available on [Oracle Container Registry](https://container-registry.oracle.com/)
  * To pull the above images from Oracle Container Registry, create a Kubernetes secret named `ocr-reg-cred` using your credentials with type set to `kubernetes.io/dockerconfigjson` in the namespace `shns`.
  * If you plan to use images built by you, you need to change `dbImage` and `gsmImage` tag with the images you have built in your enviornment in file `snr_ssharding_shard_prov_clone_across_ads.yaml`.
  * The `dbImage` used during provisioning the Persistent Volume with Database Gold Image and the `dbImage` used for deploying the Shard or Catalog Database by cloning should be same.
  * To understand the Pre-requisite of Database and Global Data Services docker images, refer [Oracle Database and Global Data Services Docker Images](../../README.md#3-oracle-database-and-global-data-services-docker-images)
  * Make sure the version of `openssl` in the Oracle Database and Oracle GSM images is compatible with the `openssl` version on the machine where you will run the openssl commands to generated the encrypted password file during the deployment.

Use the file: [snr_ssharding_shard_prov_clone_across_ads.yaml](./snr_ssharding_shard_prov_clone_across_ads.yaml) for this use case as below:

1. Deploy the `snr_ssharding_shard_prov_clone_across_ads.yaml` file:
    ```sh
    kubectl apply -f snr_ssharding_shard_prov_clone_across_ads.yaml
    ```
2. Check the status of the deployment:
    ```sh
    # Check the status of the Kubernetes Pods:
    kubectl get all -n shns

    # Check the logs of a particular pod. For example, to check status of pod "shard1-0":
    kubectl logs -f pod/shard1-0 -n shns
  
