# Deploy PrivateAI in OKE cluster using Multiple AI Models on File System and an Internal LoadBalancer

Deploy Oracle PrivateAI Container on your Cloud-based Kubernetes cluster. The PrivateAI Container is deployed using multiple AI Models. The model files are accessible to the PrivateAI Pod using Mounted Persistent Volume (PV). In this example, the deployment uses the YAML file based on the `OCI OKE` cluster. 

See [Create OCI FSS based PVC](./create_oci_fss_based_pvc.md) for the steps to create an `oci-fss` based PVC to use in this example.

**IMPORTANT:** Confirm you have completed the [Prerequisites for running Oracle PrivartAI Controller](./README.md#prerequisites-for-running-oracle-privartai-controller) before using Oracle PrivateAI Controller.

**NOTE:** The option to reserve a Private IP and use that with an OCI Internal LoadBalancer is not available at this time. For more information, see the [documentation](https://docs.oracle.com/en-us/iaas/Content/ContEng/Tasks/contengconfiguringloadbalancersnetworkloadbalancers-subtopic.htm).

If you want to use an OCI internal load balancer, follow these steps:

1. Download the files and upload them to the OCI File System that you created earlier. For example, you can mount the same file system to an OCI Compute VM in the same VCN by adding the following entry in the `/etc/fstab` file:
    ```sh
    XX.XX.XX.XX:/privateai_models   /mnt nfs defaults 0  0
    ```

    **Note:** Replace "XX.XX.XX.XX" with the IP of the OCI Internal LoadBalancer.

2. Confirm you have created the [configmap](./configmap_multi_model_filesystem.md)

3. Deploy the [pai_sample_multi_model_filesystem_internallb.yaml](./provisioning/pai_sample_multi_model_filesystem_internallb.yaml) file:
    ```sh
    kubectl apply -f pai_sample_multi_model_filesystem_internallb.yaml
    ```
    This will provision the PrivateAI Container in the OKE cluster using Internal LoadBalancer with Ephemeral Private IP.

    The PrivateAI Pod will mount the PVC `/privateai_models` which is created earlier using `oci-fss` storage class.

4. Check the deployment status and note the IP under field `EXTERNAL-IP` for `service/pai-sample-svc`.
    ```sh
    # Check the status of the Kubernetes Pods:
    kubectl get all -n pai

    # Check the logs of a particular pod:
    kubectl logs -f -n pai $(kubectl get pod -n pai -l app.kubernetes.io/name=pai-sample -o jsonpath='{.items[0].metadata.name}')
    ```

In this case, the internal LoadBalancer is created as an OCI load balancer with a private IP address, hosted on the subnet specified for load balancers when the OKE cluster was created.

By default, the internal load balancer uses the subnet specified during OKE cluster creation. If you want the internal LoadBalancer to be created as an OCI load balancer with a private IP address, hosted on the alternative subnet to the one specified for load balancers when the OKE cluster was created, then add the following annotation to your YAML file:

```sh
  pailbAnnotation:
   # Specify the OCID of the alternate Subnet
   service.beta.kubernetes.io/oci-load-balancer-subnet1: "ocid1.subnet.oc1..aaaaaa....vdfw"
```

**NOTE:** At this stage, the SSL certificate used in the deployment has the `common name` as empty. In order to avoid a hostname mismatch error while using the `cert.pem` file to make a authenicated connection, we will need to replace this SSL certificate with a new certificate which has the `common name` set to the IP of the Internal LoadBalancer.

5. Use the file [pai_secret_update_files.sh](./provisioning/pai_secret_update_files.sh) to do the following:

- Generate a new set of required keys and an SSL certificate, specifyint the inernal IP load balancer noted in Step 2 for `common name` while generating the SSL certificate.
- Encode these files using `base64` 
- Write the encoded values to a file named `secretupdate.yaml` using the following format:

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

6. Patch the secret `paisecret`. It will replace the Internal LoadBalancer Certificate:
    ```sh
    kubectl patch secret paisecret --patch-file secretupdate.yaml -n pai
    ```
**NOTE:** This action results in termination of the existing PrivateAI Container Pod and creation of new Pod. The Internal LoadBalancer IP will not change.

7. Verify deployment and AI models. After updating the secret, you can access the PrivateAI Container using the Internal LoadBalancer IP and and the `cert.pem` file from the new SSL certificate.

**NOTE:** The file `/privateai/config/config.json` inside the running Kubernetes Pod lists all AI Models currently deployed. You can use the following steps to confirm:
```sh
kubectl exec -it -n pai $(kubectl get pod -n pai -l app.kubernetes.io/name=pai-sample -o jsonpath='{.items[0].metadata.name}') -- /bin/bash
cat /privateai/config/config.json
```