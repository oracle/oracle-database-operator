# Deploying Oracle PrivateAI Container using Public LoadBalancer

Deploy Oracle PrivateAI Container on your Cloud based Kubernetes cluster.  In this example, the deployment uses the YAML file based on `OCI OKE` cluster.

**IMPORTANT:** Complete [Before You Begin](./README.md#before-you-begin) before using Oracle PrivateAI Controller. Make sure you use the Public IP Address of the Public LoadBalancer to the parameter `IP_ADDRESS` while creating the certificate for this `PrivateAi` Deployment.

**NOTE:** Modify the file `pai_sample_publiclb.yaml` with the actual Reserved Public IP before deployment.

Use the file: [pai_sample_publiclb.yaml](./provisioning/pai_sample_publiclb.yaml) for this use case as below:

1. Deploy the `pai_sample_publiclb.yaml` file:

    ```sh
    kubectl apply -f pai_sample_publiclb.yaml
    ```

2. Check the status of the deployment:

    ```sh
    # Check the status of the Kubernetes Pods:
    kubectl get all -n pai

    # Check the logs of a particular pod. For example, to check status of pod "pai-sample-b669d7897-nkkhz":
    kubectl logs pod/pai-sample-b669d7897-nkkhz -n pai
    ```
