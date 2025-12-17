# Change the memory and cpu limits for pods for an existing PrivateAI Container Deployment on Kubernetes

You can change the CPU and Memory Limits assigned to the PrivateAI Container Pods for an existing deployment which was done using PrivateAI Controller.  

In this example, the initial deployment was done on an `OCI OKE` cluster in the example [PrivateAI Container using OCI Public LoadBalancer with memory and cpu limits for pods](./deploy_privateai_publiclb_mem_cpu_limit.md).

Using an updated YAML file with change in the memory and cpu limits, you can assign different memory and cpu to Pods according to the requirement. 

**NOTE:** In this case, newer pods will be created with updated limits and the exising set of Pods will be recycled.

**IMPORTANT:** Make sure you have completed the steps for [Prerequisites for running Oracle PrivartAI Controller](./README.md#prerequisites-for-running-oracle-privartai-controller) before using Oracle PrivateAI Controller.

**NOTE:** Modify the file `pai_sample_publiclb_mem_cpu_limit_changed.yaml` with the actual Reserved Public IP before deployment.

Use the file: [pai_sample_publiclb_mem_cpu_limit_changed.yaml](./provisioning/pai_sample_publiclb_mem_cpu_limit_changed.yaml) for this use case as below:

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
  