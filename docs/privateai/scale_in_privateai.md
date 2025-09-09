# Scale-In an existing deployment of Oracle PrivateAI Container

**IMPORTANT:** This example assumes that you have an existing Oracle PrivateAI Container Deployment with `replicas=3` using the file [pai_sample_scale_up.yaml](./provisioning/pai_sample_scale_up.yaml)

In this example, we will Reduce the number of allocated resources by using the `scale in` command an existing deployment. We will update `replicas=3` to `replicas=2`.

For this use case, we will update the following file: [pai_sample_scale_in.yaml](./provisioning/pai_sample_scale_in.yaml). Example:

1. Check the status of the deployment:
    ```sh
    # Check the status of the Kubernetes Pods:
    kubectl get all -n pai
    ```
2. Apply the `pai_sample_scale_in.yaml` file to scale in:
    ```sh
    kubectl apply -f pai_sample_scale_in.yaml
    ```
3. Check the status of the deployment:
    ```sh
    # Check the status of the Kubernetes Pods:
    kubectl get all -n pai
    ```

 As a result of this procedure, the Kubernetes Pods are reduced in number after the scale in is completed automatically.
  