# Provisioning Oracle Sharded Database with User Defined Sharding and send Notification using OCI Notification Service

**IMPORTANT:** Make sure you have completed the steps for [Prerequsites for Running Oracle Sharding Database Controller](../../README.md#prerequsites-for-running-oracle-sharding-database-controller) before using Oracle Sharding Controller.

This use case demonstrates how to use a notification service like OCI Notification service to send an email notification when a particular operation is completed on an Oracle Database sharding topology provisioned using the Oracle Database sharding controller. 

This example uses `udsharding_shard_prov_send_notification.yaml` to provision an Oracle Database sharding topology using Oracle Sharding controller with:

* Primary GSM Pods `gsm1` and standby GSM Pod `gsm2`
* Three sharding Pods: `shard1`, `shard2` and `shard3`
* One Catalog Pod: `catalog`
* Namespace: `shns`
* Database Cloning from the `BLOCK VOLUME FULL BACKUP` of the Persistent Volume that has the Database Gold Image created earlier.
* OCID of the Block Volume Backup: `ocid1.volumebackup.oc1.phx.abyhqljrxtv7tu5swqb3lzc7vpzwbwzdktd2y4k2vjjy2srmgu2w7bqdftjq`
* Configmap to send notification email when a particular operation is completed. For example: When a shard is added.
* User Defined Sharding is specified using `shardingType: USER`

**NOTE:**

* The notification will be sent using a configmap created with the credentials of the OCI user account in this use case.

We will create a topic in Notification Service of the OCI Console and use its OCID. 

To do this:

1. Create a `configmap_data.txt` file, such as the following, which has the OCI User details that will be used to send notfication:

    ```sh
    user=ocid1.user.oc1........fx7omxfq
    fingerprint=fa:18:98:...............:8a
    tenancy=ocid1.tenancy.oc1..aaaa.......orpn7inq
    region=us-phoenix-1
    topicid=ocid1.onstopic.oc1.phx.aaa............6xrq
    ```
2. Create a configmap using the below command using the file created above:
    ```sh
    kubectl create configmap onsconfigmap --from-file=./configmap_data.txt -n shns
    ```

3. Create a key file `priavatekey` having the PEM key of the OCI user being used to send notification:
    ```sh
    -----BEGIN PRIVATE KEY-G----
    MIIEvQIBADANBgkqhkiG9w0BAQEFAASCBKcwggSjAgEAAoIBAQCXYxA0DJvEwtVR
    +o4OxrunL3L2NZJRADTFR+TDHqrNF1JwbaFBizSdL+EXbxQW1faZs5lXZ/sVmQF9
    .
    .
    .
    zn/xWC0FzXGRzfvYHhq8XT3omf6L47KqIzqo3jDKdgvVq4u+lb+fXJlhj6Rwi99y
    QEp36HnZiUxAQnR331DacN+YSTE+vpzSwZ38OP49khAB1xQsbiv1adG7CbNpkxpI
    nS7CkDLg4Hcs4b9bGLHYJVY=
    -----END PRIVATE KEY-----
    ```
4. Use the key file `privatekey` to create a Kubernetes secret in namespace `shns`:

    ```sh
    kubectl create secret generic my-secret --from-file=./privatekey -n shns
    ```

5. Use this command to check details of the secret that you created:

    ```sh
    kubectl describe secret my-secret -n shns
    ```

In this example, we are using pre-built Oracle Database and Global Data Services container images available on [Oracle Container Registry](https://container-registry.oracle.com/)
  * To pull the above images from Oracle Container Registry, create a Kubernetes secret named `ocr-reg-cred` using your credentials with type set to `kubernetes.io/dockerconfigjson` in the namespace `shns`.
  * If you plan to use images built by you, you need to change `dbImage` and `gsmImage` tag with the images you have built in your enviornment in file `udsharding_shard_prov_send_notification.yaml`.
  * To understand the Pre-requisite of Database and Global Data Services docker images, refer [Oracle Database and Global Data Services Docker Images](../../README.md#3-oracle-database-and-global-data-services-docker-images)
  * In case you want to use the [Oracle Database 23ai Free](https://www.oracle.com/database/free/get-started/) Image for Database and GSM, then you will need to add the additional parameter `dbEdition: "free"` to the below .yaml file.

**NOTE:** Provisioning the Sharded Database using Cloning from Database Gold Image is `NOT` supported with Oracle Database 23ai Free.

Use the file: [udsharding_shard_prov_send_notification.yaml](./udsharding_shard_prov_send_notification.yaml) for this use case as below:

1. Deploy the `udsharding_shard_prov_send_notification.yaml` file:
    ```sh
    kubectl apply -f udsharding_shard_prov_send_notification.yaml
    ```
2. Check the status of the deployment:
    ```sh
    # Check the status of the Kubernetes Pods:
    kubectl get all -n shns

    # Check the logs of a particular pod. For example, to check status of pod "shard1-0":
    kubectl logs -f pod/shard1-0 -n shns
