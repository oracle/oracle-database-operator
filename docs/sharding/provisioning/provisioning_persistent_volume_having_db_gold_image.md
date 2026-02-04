# Provisioning a Persistent Volume Containing a Database Gold Image

 In this use case, a Persistent Volume with a Oracle Database Gold Image is created. 
 
 This is required when you do not already have a Persistent Volume with a Database Gold Image from which you can clone database to save time while deploying Oracle Globally Distributed Database topology using Oracle Sharding controller.

This example uses file `oraclesi.yaml` to provision a single instance Oracle Database with:

* A Persistent Volume Claim
* Repository location for Database Container Image: `image: container-registry.oracle.com/database/enterprise:23.26.1.0`
* Namespace: `shns`
* Tag `nodeSelector` to deploy the Single Oracle Database in AD `PHX-AD-1`

In this example, we are using pre-built Oracle Database image available on [Oracle Container Registry](https://container-registry.oracle.com/)
  * To pull the above images from Oracle Container Registry, create a Kubernetes secret named `ocr-reg-cred` in the namespace `shns`. Please refer to [this page](./container_reg_secret.md) for the details. 
  * If you plan to build and use your own images, you need to change `image` tag with the image you have built in your enviornment in file `oraclesi.yaml`. 

1. Use this YAML file: [oraclesi.yaml](./oraclesi.yaml) for this use case.

    Use the following command to deploy the `oraclesi.yaml` file:

    ```sh
    kubectl apply -f oraclesi.yaml
    ```

2. After the Database Deployment is completed, switch to the Single Instancd Oracle Database Pod and confirm that the Database Instance is UP and running in READ WRITE Mode.
3. Shutdown the database instance cleanly, using `shutdown immediate` from the SQLPLUS Prompt.
4. For this use case, use the modified file [oraclesi_pvc_commented.yaml](./oraclesi_pvc_commented.yaml). This file is a copy of the file `oraclesi.yaml` but to keep the Persistent Volume claim and delete the database deployment, it has the lines of the Persistent Volume Claim commended out.
5. Delete all the components provisioned for Single Instance Oracle Database Deployment `except` the Persistent Volume Claim by using this file:

    ```sh
    kubectl delete -f oraclesi_pvc_commented.yaml
    ```

6. Check the OCID of the Persistent Volume provisioned by the preceding step using this command:

    ```sh
    kubectl get pv -n shns
    ```
