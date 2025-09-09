# Deploy PrivateAI in OKE cluster using Multiple AI Models with HTTPS URL and an Internal LoadBalancer

Deploy Oracle PrivateAI Container on your Cloud-based Kubernetes cluster. The PrivateAI Container is deployed using multiple AI Models and HTTPS URLs for those model files are provided using a configmap. In this example, the deployment uses the YAML file based on `OCI OKE` cluster. 

**IMPORTANT:** Make sure you have completed the steps for [Prerequisites for running Oracle PrivartAI Controller](./README.md#prerequisites-for-running-oracle-privartai-controller) before using Oracle PrivateAI Controller.

**NOTE:** The option to reserve a Private IP and use that with an OCI Internal LoadBalancer is not available at this time. For more information, see [Configuring Load Balancers](https://docs.oracle.com/en-us/iaas/Content/ContEng/Tasks/contengconfiguringloadbalancersnetworkloadbalancers-subtopic.htm).

To use the OCI Internal LoadBalancer, complete the following steps:

1. Ensure you have created the [configmap](./configmap_multi_model_https.md)
2. Deploy the [pai_sample_multi_model_https_internallb.yaml](./provisioning/pai_sample_multi_model_https_internallb.yaml) file:
    ```sh
    kubectl apply -f pai_sample_multi_model_https_internallb.yaml
    ```
    This will provision the PrivateAI Container in the OKE cluster using Internal LoadBalancer with Ephemeral Private IP.

3. Check the status of the deployment and note the IP under field `EXTERNAL-IP` for `service/pai-sample-svc`.
    ```sh
    # Check the status of the Kubernetes Pods:
    kubectl get all -n pai

    # Check the logs of a particular pod:
    kubectl logs -f -n pai $(kubectl get pod -n pai -l app.kubernetes.io/name=pai-sample -o jsonpath='{.items[0].metadata.name}')
    ```

In this case, the internal LoadBalancer is created as an OCI load balancer with a private IP address, hosted on the subnet specified for load balancers when the OKE cluster was created.

If you want the internal LoadBalancer to be created as an OCI load balancer with a private IP address, hosted on the alternative subnet to the one specified for load balancers when the OKE cluster was created, then you must add following annotations to the .yaml file:

```sh
  pailbAnnotation:
   # Specify the OCID of the alternate Subnet
   service.beta.kubernetes.io/oci-load-balancer-subnet1: "ocid1.subnet.oc1..aaaaaa....vdfw"
```

**NOTE:** At this stage, the SSL certificate used in the deployment has the `common name` as empty. In order to avoid a hostname mismatch error while using the `cert.pem` file to make a authenicated connection, you must replace this SSL certificate with a new certificate that has the `common name` set to the IP of the Internal LoadBalancer.

4. Use the file [pai_secret_new.sh](./provisioning/pai_secret_new.sh) to generate a new Kubernetes secret `paisecretnew`. While configuring this script, use the IP noted in Step 2 for the `common name` when generating the SSL certificate.

```sh
cd provisioning
./pai_secret_new.sh
```

5. Apply the modified file [pai_sample_multi_model_https_internallb_replace_cert.yaml](./provisioning/pai_sample_multi_model_https_internallb_replace_cert.yaml) to replace the Internal LoadBalancer Certificate:
   ```sh
    kubectl apply -f pai_sample_multi_model_https_internallb_replace_cert.yaml
    ```
**NOTE:** This step will result in termination of the existing PrivateAI Container Pod and creation of new Pod while the Internal LoadBalancer IP will not change.

6. After this change, you should be able to access the PrivateAI Container using the Internal LoadBalancer IP where you obtain an authenticated connection using the `cert.pem` file from the new SSL certificate.

**NOTE:** The file `/oml/config/config.json` inside the running Kubernetes Pod will have the details of the AI Models currently Deployed. You can use the below steps to confirm:
```sh
kubectl exec -it -n pai $(kubectl get pod -n pai -l app.kubernetes.io/name=pai-sample -o jsonpath='{.items[0].metadata.name}') -- /bin/bash
cat /oml/config/config.json
```