# Change the PrivateAI container image for an existing PrivateAI Container Deployment on Kubernetes

In this example, the initial deployment was done on an `OCI OKE` cluster following the example [PrivateAI Container using OCI Public LoadBalancer](./deploy_privateai_publiclb.md) but using the file [pai_sample_publiclb_old_container_image.yaml](./provisioning/pai_sample_publiclb_old_container_image.yaml)

Using an updated YAML file with a new PrivateAI Container image tag, you can change the image used for the Pods in the earlier deployment.

**NOTE:** Replace `<previous-privateai-tag>` and `<new-privateai-tag>` in the sample YAML files with the actual image tags you want to roll from and to.

**NOTE:** In this case, newer pods will be created with the new container image and the exising Pod(s) will be recycled.

**IMPORTANT:** Complete [Before You Begin](./README.md#before-you-begin) before using Oracle PrivateAI Controller.

Use the file: [pai_sample_publiclb_new_container_image.yaml](./provisioning/pai_sample_publiclb_new_container_image.yaml) for this use case as below:

1. Deploy the `pai_sample_publiclb_new_container_image.yaml` file:

    ```sh
    kubectl apply -f pai_sample_publiclb_new_container_image.yaml
    ```

2. Check the status of the deployment:

    ```sh
    # Check the status of the Kubernetes Pods:
    kubectl get all -n pai

    # Check the logs of a particular pod. For example, to check status of pod "pai-sample-b669d7897-nkkhz":
    kubectl logs pod/pai-sample-b669d7897-nkkhz -n pai
    ```
