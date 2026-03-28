# Change the memory and cpu limits for pods for an existing PrivateAI Container Deployment on Kubernetes

You can change the CPU and memory limits assigned to PrivateAI container pods for an existing deployment that uses the PrivateAI Controller. 

This example assumes the initial deployment occurred on an `OCI OKE` cluster using the PrivateAI container, configured with the example [PrivateAI Container using OCI Public LoadBalancer with memory and cpu limits for pods](./deploy_privateai_publiclb_mem_cpu_limit.md).

By updating the YAML file with new memory and CPU limits, you can assign different resource allocations to pods as needed. 

**NOTE:** When you update the limits in this example, Kubernetes creates new pods with the updated limits and recycles the existing pods.

**IMPORTANT:** Complete all [Prerequisites for running Oracle PrivartAI Controller](./README.md#prerequisites-for-running-oracle-privartai-controller) before using the Oracle PrivateAI Controller.

**NOTE:** Update the file `pai_sample_publiclb_mem_cpu_limit_changed.yaml` with your actual Reserved Public IP before deployment.

Use the file [pai_sample_publiclb_mem_cpu_limit_changed.yaml](./provisioning/pai_sample_publiclb_mem_cpu_limit_changed.yaml) for this procedure:

1. Deploy the `pai_sample_publiclb_mem_cpu_limit_changed.yaml` file:
    ```sh
    kubectl apply -f pai_sample_publiclb_mem_cpu_limit_changed.yaml
    ```
2. Check the status of the deployment:
    ```sh
    # Check the status of the Kubernetes Pods:
    kubectl get all -n pai

    # Check the logs of a particular pod. For example, to check status of pod "pai-sample-b669d7897-nkkhz":
    kubectl logs pod/pai-sample-b669d7897-nkkhz -n pai

    # Check the memory and cpu limits on the Pods by describing them:
    kubectl describe pod/pai-sample-b669d7897-nkkhz -n pai
    ```
  