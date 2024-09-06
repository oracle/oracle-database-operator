# Example of provisioning Oracle Globally Distributed Database along with DB Events set at Database Level

**IMPORTANT:** Make sure you have completed the steps for [Prerequsites for Running Oracle Sharding Database Controller](../../README.md#prerequsites-for-running-oracle-sharding-database-controller) before using Oracle Sharding Controller.

This example sets a Database Event at the Database Level for Catalog and Shard Databases. 

The Oracle Globally Distributed Database in this example is deployed with System-Managed Sharding type. In this use case, the database is created automatically using DBCA during the provisioning of the shard databases and the catalog database when the Oracle Globally Distributed Database topology with System-Managed Sharding is deployed using Oracle Sharding controller. 

**NOTE:** In this use case, because DBCA creates the database automatically during the deployment, the time required to create the database is greater than the time it takes when the database is created by cloning from a Database Gold Image.

This example uses `sharding_provisioning_with_db_events.yaml` to provision an Oracle Globally Distributed Database topology using Oracle Sharding controller with:

* Primary GSM Pods `gsm1` and standby GSM Pod `gsm2`
* Three Shard Database  Pods: `shard1`, `shard2` and `shard3`
* One Catalog Pod: `catalog`
* Namespace: `shns`
* Database Event: `10798 trace name context forever, level 7` set along with `GWM_TRACE level 263`


In this example, we are using pre-built Oracle Database and Global Data Services container images available on [Oracle Container Registry](https://container-registry.oracle.com/)
  * To pull the above images from Oracle Container Registry, create a Kubernetes secret named `ocr-reg-cred` using your credentials with type set to `kubernetes.io/dockerconfigjson` in the namespace `shns`.
  * If you plan to use images built by you, you need to change `dbImage` and `gsmImage` tag with the images you have built in your enviornment in file `sharding_provisioning_with_db_events.yaml`.
  * To understand the Pre-requisite of Database and Global Data Services docker images, refer [Oracle Database and Global Data Services Docker Images](../../README.md#3-oracle-database-and-global-data-services-docker-images)
  

Use the file: [sharding_provisioning_with_db_events.yaml](./sharding_provisioning_with_db_events.yaml) for this use case as below:

1. Deploy the `sharding_provisioning_with_db_events.yaml` file:
    ```sh
    kubectl apply -f sharding_provisioning_with_db_events.yaml
    ```
2. Check the status of the deployment:
    ```sh
    # Check the status of the Kubernetes Pods:
    kubectl get all -n shns

    # Check the logs of a particular pod. For example, to check status of pod "shard1-0":
    kubectl logs -f pod/shard1-0 -n shns
    ```
3. You can confirm the Database event and the tracing enabled in the RDBMS alert log file of the Database.