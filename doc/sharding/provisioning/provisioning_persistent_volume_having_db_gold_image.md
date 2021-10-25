# Provisioning a Persistent Volume Containing a Database Gold Image

 In this use case, a Persistent Volume with a Oracle Database Gold Image is created. 
 
 This is required when you do not already have a Persistent Volume with a Database Gold Image from which you can clone database to save time while deploying Oracle Sharding topology using Oracle Sharding controller.

This example uses file `oraclesi.yaml` to provision a single instance Oracle Database:

* A Persistent Volume Claim
* Repository location for Database Docker Image: `image: container-registry.oracle.com/database/enterprise:latest`
* Namespace: `shns`
* Tag `nodeSelector` to deploy the Single Oracle Database in AD `EU-FRANKFURT-1-AD-1`

In this example, we are using pre-built Oracle Database image available on [Oracle Container Registry](https://container-registry.oracle.com/)
  * To pull the above image from Oracle Container Registry, create a Kubernetes secret named `ocr-reg-cred` using your credentials with type set to `kubernetes.io/dockerconfigjson` in the namespace `shns`.
  * If you plan to use image built by you, you need to change `image` tag with the image you have built in your enviornment in file `oraclesi.yaml`.
  * To understand the Pre-requisite of Database and Global Data Services docker images, refer [Oracle Database and Global Data Services Docker Images](../ORACLE_SHARDING_CONTROLLER_README.md#3-oracle-database-and-global-data-services-docker-images)

1. Use this YAML file: [oraclesi.yaml](./doc/sharding/provisioning/oraclesi.yaml) for this use case.

    Use the following command to deploy the `oraclesi.yaml` file:

    ```sh
    kubectl apply -f oraclesi.yaml
    ```

2. After the Database Deployment is completed, switch to the Single Oracle Database Pod and confirm that the Database Instance is UP and running in RW Mode.
3. Shut down the database instance cleanly, using `shutdown immediate` from the SQLPLUS Prompt.
4. For this use case, use the modified file [oraclesi_pvc_commented.yaml](./doc/sharding/provisioning/oraclesi_pvc_commented.yaml). This file is a copy of the file `oraclesi.yaml` but to keep the Persistent Volume claim and delete the database deployment, it has the lines of the Persistent Volume Claim commended out.
5. Delete all the components provisioned for Single Oracle Database Deployment EXCEPT the Persistent Volume Claim by applying this file:

    ```sh
    kubectl delete -f oraclesi_pvc_commented.yaml
    ```

6. Check the OCID of the Persistent Volume provisioned by the preceding step using this command:

    ```sh
    kubectl get pv -n shns
    ```
