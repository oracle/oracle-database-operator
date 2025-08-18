# Deploy PrivateAI in OKE cluster using Multiple AI Models on File System and an Internal LoadBalancer

Deploy Oracle PrivateAI Container on your Cloud based Kubernetes cluster. The PrivateAI Container is deployed using multiple AI Models and the model files are accessible to the PrivateAI Pod using Mounted Persistent Volume(PV). In this example, the deployment uses the YAML file based on `OCI OKE` cluster. 

Please refer to [Create OCI FSS based PVC](./create_oci_fss_based_pvc.md) for the steps to create an `oci-fss` based PVC to use in the current case.

**IMPORTANT:** Make sure you have completed the steps for [Prerequisites for running Oracle PrivartAI Controller](./README.md#prerequisites-for-running-oracle-privartai-controller) before using Oracle PrivateAI Controller.

**NOTE:** The option to reserve a Private IP and use that with an OCI Internal LoadBalancer is not available as of now. Please check the [documentation](https://docs.oracle.com/en-us/iaas/Content/ContEng/Tasks/contengconfiguringloadbalancersnetworkloadbalancers-subtopic.htm).

If you want to use the OCI Internal LoadBalancer, then you will need to follow the below steps:

1. Refer to the file [Oracle Machine Learning AI models](./Oracle_Machine_Learning_AI_models.htm) to get details of the AI Model files. Download the files and upload them to the OCI File System you have created in above step. For Example: You can mount the same File System to an OCI Compute VM in the same VCN using below entry in `/etc/fstab` file
    ```sh
    10.0.XX.XX:/oml_models   /mnt nfs defaults 0  0
    ```

2. Make sure you have created the [configmap](./configmap_multi_model_filesystem.md)

3. Deploy the [pai_sample_multi_model_filesystem_internallb.yaml](./provisioning/pai_sample_multi_model_filesystem_internallb.yaml) file:
    ```sh
    kubectl apply -f pai_sample_multi_model_filesystem_internallb.yaml
    ```
    This will provision the PrivateAI Container in the OKE cluster using Internal LoadBalancer with Ephemeral Private IP.

    The PrivateAI Pod will mount the PVC `/oml/models` which is created earlier using `oci-fss` storage class.

4. Check the status of the deployment and note the IP under field `EXTERNAL-IP` for `service/pai-sample-svc`.
    ```sh
    # Check the status of the Kubernetes Pods:
    kubectl get all -n pai

    # Check the logs of a particular pod:
    kubectl logs -f -n pai $(kubectl get pod -n pai -l app.kubernetes.io/name=pai-sample -o jsonpath='{.items[0].metadata.name}')
    ```

In this case, the internal LoadBalancer is created as an OCI load balancer with a private IP address, hosted on the subnet specified for load balancers when the OKE cluster was created.

In case, you want the internal LoadBalancer to be created as an OCI load balancer with a private IP address, hosted on the alternative subnet to the one specified for load balancers when the OKE cluster was created, you need to add following annotations in the above .yaml file:

```sh
  pailbAnnotation:
   # Specify the OCID of the alternate Subnet
   service.beta.kubernetes.io/oci-load-balancer-subnet1: "ocid1.subnet.oc1..aaaaaa....vdfw"
```

**NOTE:** At this stage, the SSL certificate used in the deployment has the `common name` as empty. In order to avoid a hostname mismatch error while using the `cert.pem` file to make a authenicated connection, we will need to replace this SSL certificate with a new certificate which has the `common name` set to the IP of the Internal LoadBalancer.

5. Use the file [pai_secret_new.sh](./provisioning/pai_secret_new.sh) to generate a new Kubernetes secret `paisecretnew`. While using this script, use the IP noted in Step 2 for `common name` while generating the SSL certificate.

```sh
cd provisioning
./pai_secret_new.sh
```

6. Apply the modified file [pai_sample_multi_model_https_internallb_replace_cert.yaml](./provisioning/pai_sample_multi_model_https_internallb_replace_cert.yaml) to replace the Internal LoadBalancer Certificate:
```sh
kubectl apply -f pai_sample_multi_model_https_internallb_replace_cert.yaml
```
**NOTE:** This step will result in termination of the existing PrivateAI Container Pod and creation of new Pod while the Internal LoadBalancer IP will not change.

7. After this change, you will be able to access the PrivateAI Container using the Internal LoadBalancer IP using an authenticated connection using the `cert.pem` file from the new SSL certificate.

**NOTE:** The file `/oml/config/config.json` inside the running Kubernetes Pod will have the details of the AI Models currently Deployed. You can use the below steps to confirm:
```sh
kubectl exec -it -n pai $(kubectl get pod -n pai -l app.kubernetes.io/name=pai-sample -o jsonpath='{.items[0].metadata.name}') -- /bin/bash
cat /oml/config/config.json
```