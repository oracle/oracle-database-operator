# Provisioning Oracle Globally Distributed Database with User-Defined Sharding without Database Gold Image

**IMPORTANT:** Make sure you have completed the steps for [Prerequsites for Running Oracle Sharding Database Controller](../../README.md#prerequsites-for-running-oracle-sharding-database-controller) before using Oracle Sharding Controller.

In this use case, the database is created automatically using DBCA during the provisioning of the shard databases and the catalog database when the Oracle Sharding topology with User-Defined Sharding is deployed using Oracle Sharding controller.

**NOTE** In this use case, because DBCA creates the database automatically during the deployment, the time required to create the database is greater than the time it takes when the database is created by cloning from a Database Gold Image.

This example uses `udsharding_shard_prov.yaml` to provision an Oracle Globally Distributed Database topology using Oracle Sharding controller with:

* Primary GSM Pods `gsm1` and standby GSM Pod `gsm2`
* Three Shard Database Pods: `shard1`, `shard2` and `shard3`
* One Catalog Pod: `catalog`
* Namespace: `shns`
* User-Defined Sharding is specified using `shardingType: USER`


In this example, we are using pre-built Oracle Database and Global Data Services container images available on [Oracle Container Registry](https://container-registry.oracle.com/)
  * To pull the above images from Oracle Container Registry, create a Kubernetes secret named `ocr-reg-cred` using your credentials with type set to `kubernetes.io/dockerconfigjson` in the namespace `shns`.
  * If you plan to use images built by you, you need to change `dbImage` and `gsmImage` tag with the images you have built in your enviornment in file `udsharding_shard_prov.yaml`.
  * To understand the Pre-requisite of Database and Global Data Services docker images, refer [Oracle Database and Global Data Services Docker Images](../../README.md#3-oracle-database-and-global-data-services-docker-images)
  * In case you want to use the [Oracle Database 23ai Free](https://www.oracle.com/database/free/get-started/) Image for Database and GSM, then you will need to add the additional parameter `dbEdition: "free"` to the below .yaml file.

Use the file: [udsharding_shard_prov.yaml](./udsharding_shard_prov.yaml) for this use case as below:

1. Deploy the `udsharding_shard_prov.yaml` file:
    ```sh
    kubectl apply -f udsharding_shard_prov.yaml
    ```
1. Check the status of the deployment:
    ```sh
    # Check the status of the Kubernetes Pods:
    kubectl get all -n shns

    # Check the logs of a particular pod. For example, to check status of pod "shard1-0":
    kubectl logs -f pod/shard1-0 -n shns
    ```
