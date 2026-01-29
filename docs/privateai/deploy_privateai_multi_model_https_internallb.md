# Deploy PrivateAI in OKE cluster using Multiple AI Models with HTTPS URL and an Internal LoadBalancer

Deploy Oracle PrivateAI Container on your Cloud-based Kubernetes cluster. The PrivateAI Container is deployed using multiple AI Models. HTTPS URLs for those model files are provided using a configmap. In this example, the deployment uses the YAML file based on `OCI OKE` cluster. 

**IMPORTANT:** Confirm you have completed the [Prerequisites for running Oracle PrivateAI Controller](./README.md#prerequisites-for-running-oracle-privartai-controller) before using Oracle PrivateAI Controller.

**NOTE:** Currently, reserving a private IP for use with an OCI internal load balancer is not supported. For more information, check the [documentation](https://docs.oracle.com/en-us/iaas/Content/ContEng/Tasks/contengconfiguringloadbalancersnetworkloadbalancers-subtopic.htm).

To use an OCI internal load balancer, complete the following steps:

1. Confirm that you have created the [configmap](./configmap_multi_model_https.md)
2. Deploy the [pai_sample_multi_model_https_internallb.yaml](./provisioning/pai_sample_multi_model_https_internallb.yaml) file:
    ```sh
    kubectl apply -f pai_sample_multi_model_https_internallb.yaml
    ```
    This will provision the PrivateAI Container in the OKE cluster using Internal LoadBalancer with Ephemeral Private IP.

3. Check the deployment status and note the IP under field `EXTERNAL-IP` for `service/pai-sample-svc`.
    ```sh
    # Check the status of the Kubernetes Pods:
    kubectl get all -n pai

    # Check the logs of a particular pod:
    kubectl logs -f -n pai $(kubectl get pod -n pai -l app.kubernetes.io/name=pai-sample -o jsonpath='{.items[0].metadata.name}')
    ```

In this case, the internal LoadBalancer is created as an OCI load balancer with a private IP address, hosted on the subnet specified for load balancers when the OKE cluster was created.

If you want the internal LoadBalancer to be created as an OCI load balancer with a private IP address, hosted on the alternative subnet to the one specified for load balancers when the OKE cluster was created, then add the following annotations to your YAML file:

```sh
  pailbAnnotation:
   # Specify the OCID of the alternate Subnet
   service.beta.kubernetes.io/oci-load-balancer-subnet1: "ocid1.subnet.oc1..aaaaaa....vdfw"
```

**NOTE:** At this stage, the SSL certificate used in the deployment has the `common name` as empty. In order to avoid a hostname mismatch error while using the `cert.pem` file to make a authenicated connection, we will need to replace this SSL certificate with a new certificate that has the `common name` set to the IP of the Internal LoadBalancer.

4. Use the file [pai_secret_update_files.sh](./provisioning/pai_secret_update_files.sh) to do the following:

- Generate a new set of required keys and an SSL certificate. When generating the SSL certificate, specify the load balancer’s IP (from Step 2) as the `common name` while generating the SSL certificate.
- Encode these files using `base64` and write them to a file named `secretupdate.yaml` in the following format:

```sh
data:
  # Base64-encoded API key
  api-key: your-base64-encoded-api-key-here
  # Base64-encoded certificate file (cert.pem)
  cert.pem: your-base64-encoded-cert-content-here
  # Base64-encoded private key file (e.g., key.pem)
  key.pem: your-base64-encoded-key-content-here
  # Base64-encoded keystore file
  keystore: your-base64-encoded-keystore-content-here
  # Base64-encoded password file
  privateai-ssl-pwd: your-base64-encoded-password-file-content-here
```

4. Use the following command to patch the secret `paisecret`. This secret will replace the Internal LoadBalancer Certificate:
    ```sh
    kubectl patch secret paisecret --patch-file secretupdate.yaml -n pai
    ```
**NOTE:** This step will result in the termination of the existing PrivateAI Container Pod and the creation of a new Pod. The Internal LoadBalancer IP will remain unchanged.

6. After updating the secret, you can access the PrivateAI Container through the Internal LoadBalancer IP using an authenticated connection with the new SSL certificate `cert.pem` file.

**NOTE:** The file `/privateai/config/config.json` inside the running Kubernetes Pod will have the details of the AI Models currently deployed. To confirm, use the following steps:
```sh
kubectl exec -it -n pai $(kubectl get pod -n pai -l app.kubernetes.io/name=pai-sample -o jsonpath='{.items[0].metadata.name}') -- /bin/bash
cat /privateai/config/config.json
```