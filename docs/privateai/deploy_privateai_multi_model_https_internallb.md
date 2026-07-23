# Deploy PrivateAI in OKE cluster using Multiple AI Models with HTTPS URL and an Internal LoadBalancer

Deploy Oracle PrivateAI Container on your Cloud-based Kubernetes cluster. The PrivateAI Container is deployed using multiple AI Models. HTTPS URLs for those model files are provided using a configmap. In this example, the deployment uses the YAML file based on an `OCI OKE` cluster.

**NOTE:** The option to reserve a Private IP and use that with an OCI Internal LoadBalancer is available from `OCI OKE` cluster with Kubernetes version 1.32 onwards. Please check the [documentation](https://docs.oracle.com/en-us/iaas/Content/ContEng/Tasks/contengconfiguringloadbalancersnetworkloadbalancers-subtopic.htm).

**IMPORTANT:** Complete [Before You Begin](./README.md#before-you-begin) before using Oracle PrivateAI Controller. Make sure you use the Reserved Private IP Address for the Internal LoadBalancer to the parameter `IP_ADDRESS` while creating the certificate for this `PrivateAi` Deployment. Please refer to the [documentation](https://docs.oracle.com/en-us/iaas/Content/Network/Tasks/reserved-ipv4-adding.htm) for the steps to reserve an IP in the subnet you want.

**NOTE:** Modify the file `pai_sample_multi_model_https_internallb.yaml` with the actual Reserved Private IP before deployment.

Use the file: [pai_sample_multi_model_https_internallb.yaml](./provisioning/pai_sample_multi_model_https_internallb.yaml) for this use case as below:

1. Confirm you have created the [configmap](./configmap_multi_model_https.md).
2. Deploy the `pai_sample_multi_model_https_internallb.yaml` file:

    ```sh
    kubectl apply -f pai_sample_multi_model_https_internallb.yaml
    ```

3. Check the status of the deployment:

    ```sh
    # Check the status of the Kubernetes Pods:
    kubectl get all -n pai

    # Check the logs of a particular pod. For example, to check status of pod "pai-sample-b669d7897-nkkhz":
    kubectl logs pod/pai-sample-b669d7897-nkkhz -n pai
    ```

**NOTE:** The file `/privateai/config/config.json` inside the running Kubernetes Pod will have the details of the AI Models currently deployed. To confirm, use the following steps:

```sh
kubectl exec -it -n pai $(kubectl get pod -n pai -l app.kubernetes.io/name=pai-sample -o jsonpath='{.items[0].metadata.name}') -- /bin/bash
cat /privateai/config/config.json
```
