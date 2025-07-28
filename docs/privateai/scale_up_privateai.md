# Scale-Up an existing deployment of Oracle PrivateAI Container

**IMPORTANT:** This example assumes that you have an existing Oracle PrivateAI Container Deployment with `replicas=1`

In this example, we will Scale Up an existing deployment with `replicas=1` to `replicas=3`.

Use the file: [pai-sample-scale-up.yaml](./pai-sample-scale-up.yaml) for this use case as below:

1. Check the status of the deployment:
    ```sh
    # Check the status of the Kubernetes Pods:
    kubectl get all -n pai
    ```
2. Deploy the `pai-sample-scale-up.yaml` file to Scale Up:
    ```sh
    kubectl apply -f pai-sample-scale-up.yaml
    ```
3. Check the status of the deployment:
    ```sh
    # Check the status of the Kubernetes Pods:
    kubectl get all -n pai
    ```

    You will see, additional Kubernetes Pods getting deployed once the scale up is done automatically.
  