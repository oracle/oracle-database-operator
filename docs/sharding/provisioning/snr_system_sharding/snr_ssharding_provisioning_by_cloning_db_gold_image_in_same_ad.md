# Provisioning System managed Sharding Topology with Raft replication enabled by cloning database from your own Database Gold Image in the same Availability Domain(AD)

**NOTE: RAFT Replication Feature is available only for Oracle 23c RDBMS and Oracle 23c GSM version.**

**IMPORTANT:** Make sure you have completed the steps for [Prerequsites for Running Oracle Sharding Database Controller](../../README.md#prerequsites-for-running-oracle-sharding-database-controller) before using Oracle Sharding Controller.

In this test case, you provision the System managed Sharding Topology with Raft replication enabled while provisioning the Catalog and Shard Databases by cloning from an existing Oracle Database Gold Image created earlier.

This use case applies when you are cloning from a Block Volume, and you can clone _only_ in the same availability domain (AD). The result is that the cloned shard database PODs can be created _only_ in the same AD where the Gold Image Block Volume is present.

Choosing this option takes substantially less time during the Oracle Database Sharding Topology setup.

**NOTE** For this step, the Persistent Volume that has the Oracle Database Gold Image is identified using its OCID.

1. Check the OCID of the Persistent Volume provisioned earlier using below command:

    ```sh
    kubectl get pv -n shns
    ```

2. This example uses `snr_ssharding_shard_prov_clone.yaml` to provision an Oracle Database sharding topology using Oracle Sharding controller with:

* Primary GSM Pods `gsm1` and standby GSM Pod `gsm2`
* Three sharding Pods: `shard1`, `shard2` and `shard3`
* One Catalog Pod: `catalog`
* Namespace: `shns`
* Database Cloning from the Database Gold Image present in Persistent Volume having OCID: `ocid1.volume.oc1.phx.abyhqljr3z3w72t6ay5eud7d5w3kdfhktfp6gwb6euy5tzwfaxgmbvwqlvsq`
* `RAFT Replication` enabled

NOTE: In this case, the Persistent Volume with DB Gold Image was provisioned in the Availability Domain `PHX-AD-1`. The Shards and Catalog will be provisioned in the same Availability Domain `PHX-AD-1` by cloning the database.

In this example, we are using pre-built Oracle Database and Global Data Services container images available on [Oracle Container Registry](https://container-registry.oracle.com/)
  * To pull the above images from Oracle Container Registry, create a Kubernetes secret named `ocr-reg-cred` using your credentials with type set to `kubernetes.io/dockerconfigjson` in the namespace `shns`.
  * If you plan to use images built by you, you need to change `dbImage` and `gsmImage` tag with the images you have built in your enviornment in file `snr_ssharding_shard_prov_clone.yaml`. 
  * The `dbImage` used during provisioning the Persistent Volume with Database Gold Image and the `dbImage` used for deploying the Shard or Catalog Database by cloning should be same.
  * To understand the Pre-requisite of Database and Global Data Services docker images, refer [Oracle Database and Global Data Services Docker Images](../../README.md#3-oracle-database-and-global-data-services-docker-images)

**NOTE:** Provisioning the Sharded Database using Cloning from Database Gold Image is `NOT` supported with Oracle Database 23ai Free.

Use the file: [snr_ssharding_shard_prov_clone.yaml](./snr_ssharding_shard_prov_clone.yaml) for this use case as below:

1. Deploy the `snr_ssharding_shard_prov_clone.yaml` file:
    ```sh
    kubectl apply -f snr_ssharding_shard_prov_clone.yaml
    ```
2. Check the status of the deployment:
    ```sh
    # Check the status of the Kubernetes Pods:
    kubectl get all -n shns

    # Check the logs of a particular pod. For example, to check status of pod "shard1-0":
    kubectl logs -f pod/shard1-0 -n shns
    ```
