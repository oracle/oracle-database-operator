# Example of provisioning Oracle Sharded Database with Oracle 23ai FREE Database and GSM Images

**IMPORTANT:** Make sure you have completed the steps for [Prerequsites for Running Oracle Sharding Database Controller](../../README.md#prerequsites-for-running-oracle-sharding-database-controller) before using Oracle Sharding Controller.

This example uses the Oracle 23ai FREE Database and GSM Images.

The sharded database in this example is deployed with System-Managed Sharding type. In this use case, the database is created automatically using DBCA during the provisioning of the shard databases and the catalog database when the Oracle Sharding topology with System-Managed Sharding is deployed using Oracle Sharding controller. 

**NOTE:** In this use case, because DBCA creates the database automatically during the deployment, the time required to create the database is greater than the time it takes when the database is created by cloning from a Database Gold Image.

This example uses `sharding_provisioning_with_free_images.yaml` to provision an Oracle Database sharding topology using Oracle Sharding controller with:

* Primary GSM Pods `gsm1` and standby GSM Pod `gsm2`
* Three sharding Pods: `shard1`, `shard2` and `shard3`
* One Catalog Pod: `catalog`
* Namespace: `shns`


To get the Oracle 23ai FREE Database and GSM Images:
  * The Oracle 23ai FREE RDBMS Image used is `container-registry.oracle.com/database/free:latest`. Check [Oracle Database Free Get Started](https://www.oracle.com/database/free/get-started/?source=v0-DBFree-ChatCTA-j2032-20240709) for details.
  * To pull the above image from Oracle Container Registry, create a Kubernetes secret named `ocr-reg-cred` using your credentials with type set to `kubernetes.io/dockerconfigjson` in the namespace `shns`.
  * The the Oracle 23ai FREE GSM Image used is `container-registry.oracle.com/database/gsm:latest`.
  * To pull the above image from Oracle Container Registry, create a Kubernetes secret named `ocr-reg-cred` using your credentials with type set to `kubernetes.io/dockerconfigjson` in the namespace `shns`.
  * You need to change `dbImage` and `gsmImage` tag with the images you want to use in your enviornment in file `sharding_provisioning_with_free_images.yaml`.

**IMPORTANT:** Make sure the version of `openssl` in the Oracle Database and Oracle GSM images is compatible with the `openssl` version on the machine where you will run the openssl commands to generated the encrypted password file during the deployment.
  
  

Use the file: [sharding_provisioning_with_free_images.yaml](./sharding_provisioning_with_free_images.yaml) for this use case as below:

1. Deploy the `sharding_provisioning_with_free_images.yaml` file:
    ```sh
    kubectl apply -f sharding_provisioning_with_free_images.yaml
    ```
2. Check the status of the deployment:
    ```sh
    # Check the status of the Kubernetes Pods:
    kubectl get all -n shns

    # Check the logs of a particular pod. For example, to check status of pod "shard1-0":
    kubectl logs -f pod/shard1-0 -n shns
    ```