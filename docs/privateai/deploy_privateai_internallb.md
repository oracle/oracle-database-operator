# Deploy PrivateAI in OKE cluster using Internal LB

Deploy Oracle PrivateAI Container on your Cloud based Kubernetes cluster.  In this example, the deployment uses the YAML file based on `OCI OKE` cluster. 

**IMPORTANT:** Ensure that you have completed the steps for [Prerequisites for running Oracle PrivartAI Controller](./README.md#prerequisites-for-running-oracle-privartai-controller) before using Oracle PrivateAI Controller.

**NOTE:** The option to reserve a Private IP and use that IP with an OCI Internal LoadBalancer is not available at this time. For more information, see [the OCI documentation about configuring load balancers](https://docs.oracle.com/en-us/iaas/Content/ContEng/Tasks/contengconfiguringloadbalancersnetworkloadbalancers-subtopic.htm).

If you want to use the OCI Internal LoadBalancer, then you must complete the following steps:

1. Deploy the [pai_sample_internallb.yaml](./provisioning/pai_sample_internallb.yaml) file:
    ```sh
    kubectl apply -f pai_sample_internallb.yaml
    ```
    This will provision the PrivateAI Container in the OKE cluster using Internal LoadBalancer with Ephemeral Private IP.

2. Check the status of the deployment and note the IP under field `EXTERNAL-IP` for `service/pai-sample-svc`.
    ```sh
    # Check the status of the Kubernetes Pods:
    kubectl get all -n pai

    # Check the logs of a particular pod. For example, to check status of pod "pai-sample-b669d7897-nkkhz":
    kubectl logs pod/pai-sample-b669d7897-nkkhz -n pai
    ```

In this case, the internal LoadBalancer is created as an OCI load balancer with a private IP address, hosted on the subnet specified for load balancers when the OKE cluster was created.

With our example, we want to create the internal LoadBalancer as an OCI load balancer with a private IP address, hosted on the alternative subnet to the one specified for load balancers when the OKE cluster was created. To do this, you must add the following annotations in the `.yaml` file:

```sh
  pailbAnnotation:
   # Specify the OCID of the alternate Subnet
   service.beta.kubernetes.io/oci-load-balancer-subnet1: "ocid1.subnet.oc1..aaaaaa....vdfw"
```

**NOTE:** At this stage, the SSL certificate used in the deployment has the `common name` as empty. To avoid a hostname mismatch error while using the `cert.pem` file to make a authenticated connection, we must replace this SSL certificate with a new certificate that has the `common name` set to the IP of the Internal LoadBalancer.

3. Use the file [pai_secret_new.sh](./pai_secret_new.sh) to generate a new Kubernetes secret `paisecretnew`. While using this script, use the IP from Step 2 for `common name` while generating the SSL certificate.

4. Apply the modified file [pai_sample_internallb_replace_cert.yaml](./provisioning/pai_sample_internallb_replace_cert.yaml) to replace the Internal LoadBalancer Certificate:
   ```sh
    kubectl apply -f pai_sample_internallb_replace_cert.yaml
    ```
**NOTE:** This step will result in termination of the existing PrivateAI Container Pod and creation of new Pod. The Internal LoadBalancer IP will not change.

5. After this change, you are now able to access the PrivateAI Container using the Internal LoadBalancer IP using an authenticated connection, which uses the `cert.pem` file from the new SSL certificate.