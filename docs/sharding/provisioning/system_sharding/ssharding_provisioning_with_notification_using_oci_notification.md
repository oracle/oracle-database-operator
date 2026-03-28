# Provisioning Oracle Globally Distributed Database with System-Managed Sharding and send Notification using OCI Notification Service

**IMPORTANT:** Make sure you have completed the steps for [Prerequisites for running Oracle Sharding Database Controller](../../README.md#prerequisites-for-running-oracle-sharding-database-controller) before using Oracle Sharding Controller.

This use case demonstrates how to use a notification service like OCI Notification service to send an email notification when a particular operation is completed.

This example uses `ssharding_shard_prov_send_notification.yaml` to provision an Oracle Database sharding topology using Oracle Sharding controller with:

* Primary GSM Pods `gsm1` and standby GSM Pod `gsm2`
* Three Shard Database Pods: `shard1`, `shard2` and `shard3`
* One Catalog Database Pod: `catalog`
* Namespace: `shns`
* Database Cloning from the `BLOCK VOLUME FULL BACKUP` of the Persistent Volume that has the Database Gold Image created earlier.
* OCID of the Block Volume Backup: `ocid1.volumebackup.oc1.phx.abyhqljrxtv7tu5swqb3lzc7vpzwbwzdktd2y4k2vjjy2srmgu2w7bqdftjq`
* Configmap to send notification email when a particular operation is completed. For example: When a shard is added.

**NOTE:**

* The notification will be sent using a configmap created with the credentials of the OCI user account used with this procedure.

To do this:

1. Create a topic in Notification Service of the OCI Console and note its OCID. 
2. Create a `configmap_data.txt` file, such as the following, which has the OCI User details that will be used to send notification:

    ```sh
    user=ocid1.user.oc1........fx7omxfq
    fingerprint=fa:18:98:...............:8a
    tenancy=ocid1.tenancy.oc1..aaaa.......orpn7inq
    region=us-phoenix-1
    topicid=ocid1.onstopic.oc1.phx.aaa............6xrq
    ```
3. Using the file created in step 1, create a configmap with the following command:
    ```sh
    kubectl create configmap onsconfigmap --from-file=./configmap_data.txt -n shns
    ```

4. Create a key file called `privatekey` that has the PEM key of the OCI user being used to send notification:
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
5. Use the key file `privatekey` to create a Kubernetes secret in namespace `shns`:

    ```sh
    kubectl create secret generic my-secret --from-file=./privatekey -n shns
    ```

6. Use this command to check details of the secret that you created:

    ```sh
    kubectl describe secret my-secret -n shns
    ```

In this example, we are using pre-built Oracle Database and Global Data Services container images available on the [Oracle Container Registry](https://container-registry.oracle.com/)
  * To pull the above images from Oracle Container Registry, create a Kubernetes secret named `ocr-reg-cred` in the namespace `shns`. Please refer to [this page](./../container_reg_secret.md) for the details. 
  * If you plan to build and use the images, then you must exchange the `dbImage` and `gsmImage` tags for the images that you have built in your enviornment in file `ssharding_shard_prov_send_notification.yaml`. 
  * To understand Database and Global Data Services Docker images prerequsites, see [Oracle Database and Global Data Services Docker Images](../../README.md#3-oracle-database-and-global-data-services-container-images) 
  * The version of `openssl` in the Oracle Database and Oracle GSM images must be compatible with the `openssl` version on the machine where you will run the openssl commands to generate the encrypted password file during the deployment. 
  
**NOTE:** Provisioning the Sharded Database using Cloning from Database Gold Image is _not_ supported with Oracle AI Database 26ai Free.

Use the file: [ssharding_shard_prov_send_notification.yaml](./ssharding_shard_prov_send_notification.yaml) for this use case: 

1. Deploy the `ssharding_shard_prov_send_notification.yaml` file:
    ```sh
    kubectl apply -f ssharding_shard_prov_send_notification.yaml
    ```
2. Check the status of the deployment:
    ```sh
    # Check the status of the Kubernetes Pods:
    kubectl get all -n shns

    # Check the logs of a particular pod. For example, to check status of pod "shard1-0":
    kubectl logs -f pod/shard1-0 -n shns
