# Provisioning Oracle Database Sharding Topology by Cloning the Database from Your Own Database Gold Image in the same Availability Domain (AD)

In this case, the database is created automatically by cloning from an existing Oracle Database Gold Image during the provisioning of the shard databases and the catalog database when the Oracle Sharding topology is deployed using Oracle Sharding controller.

This use case applies when you are cloning from a Block Volume, and you can clone _only_ in the same availability domain (AD). The result is that the cloned shard database PODs can be created _only_ in the same AD where the Gold Image Block Volume is present.

Choosing this option takes substantially less time during the Oracle Database Sharding Topology setup.

**NOTE** For this step, the Persistent Volume that has the Oracle Database Gold Image is identified using its OCID.

1. Check the OCID of the Persistent Volume provisioned by above step using below command:

    ```sh
    kubectl get pv -n shns
    ```

2. This example uses `shard_prov_clone.yaml` to provision an Oracle Database sharding topology using Oracle Sharding controller with:

* Primary GSM Pods `gsm1` and standby GSM Pod `gsm2`
* Two sharding Pods: `shard1` and `shard2`
* One Catalog Pod: `catalog`
* Namespace: `shns`
* Database Cloning from the Database Gold Image present in Persistent Volume having OCID: `ocid1.volume.oc1.eu-frankfurt-1.abtheljtmwcwf7liuhaibzgdcoxqcwwfpsqiqlsumrjlzkin7y4zx3x2idua`

In this example, we are using pre-built Oracle Database and Global Data Services container images available on [Oracle Container Registry](https://container-registry.oracle.com/)
  * To pull the above images from Oracle Container Registry, create a Kubernetes secret named `ocr-reg-cred` using your credentials with type set to `kubernetes.io/dockerconfigjson` in the namespace `shns`.
  * If you plan to use images built by you, you need to change `dbImage` and `gsmImage` tag with the images you have built in your enviornment in file `shard_prov.yaml`.
  * To understand the Pre-requisite of Database and Global Data Services docker images, refer [Oracle Database and Global Data Services Docker Images](../ORACLE_SHARDING_CONTROLLER_README.md#3-oracle-database-and-global-data-services-docker-images)

Use the file: [shard_prov_clone.yaml](./doc/sharding/provisioning/shard_prov_clone.yaml) for this use case as below:

1. Deploy the `shard_prov_clone.yaml` file:
    ```sh
    kubectl apply -f shard_prov_clone.yaml
    ```
2. Check the status of the deployment:
    ```sh
    # Check the status of the Kubernetes Pods:
    kubectl get all -n shns

    # Check the logs of a particular pod. For example, to check status of pod "shard1-0":
    kubectl logs -f pod/shard1-0 -n shns
    ```
