# Scale-Out an existing deployment of Oracle PrivateAI Container

**IMPORTANT:** This example assumes that you have an existing Oracle PrivateAI Container Deployment with `replicas=1` using the example [PrivateAI Container using OCI Public LoadBalancer](./deploy_privateai_publiclb.md) which uses the file [pai_sample_publiclb.yaml](./provisioning/pai_sample_publiclb.yaml).

In this example, we will Scale Out an existing deployment with `replicas=1` to `replicas=3`.

Use the file: [pai_sample_scale_out.yaml](./provisioning/pai_sample_scale_out.yaml) for this use case as below:

1. Check the status of the deployment:

    ```sh
    # Check the status of the Kubernetes Pods:
    kubectl get all -n pai
    ```

2. Deploy the `pai_sample_scale_out.yaml` file to scale out:

    ```sh
    kubectl apply -f pai_sample_scale_out.yaml
    ```

3. Check the status of the deployment:

    ```sh
    # Check the status of the Kubernetes Pods:
    kubectl get all -n pai
    ```

    You should see additional Kubernetes Pods being deployed automatically after the Scale Out is done.
  