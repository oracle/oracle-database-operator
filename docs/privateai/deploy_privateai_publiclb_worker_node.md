# Deploying Oracle PrivateAI Container using Public LoadBalancer and with worker node selection

Deploy Oracle PrivateAI Container on your Cloud based Kubernetes cluster.  In this example, the deployment uses the YAML file based on `OCI OKE` cluster and worker nodes are specified to provision the Pods only on those worker nodes.

**IMPORTANT:** Make sure you have completed the steps for [Prerequisites for running Oracle PrivartAI Controller](./README.md#prerequisites-for-running-oracle-privartai-controller) before using Oracle PrivateAI Controller.

**NOTE:** Modify the file `pai_sample_publiclb_select_worker_node.yaml` with the actual Reserved Public IP before deployment.

Use the file: [pai_sample_publiclb_select_worker_node.yaml](./provisioning/pai_sample_publiclb_select_worker_node.yaml) for this use case as below:

1. Deploy the `pai_sample_publiclb_select_worker_node.yaml` file:
    ```sh
    kubectl apply -f pai_sample_publiclb_select_worker_node.yaml
    ```
2. Check the status of the deployment. You will see Pods are created only on the worker nodes specified in the above YAML file:
    ```sh
    # Check the status of the Kubernetes Pods:
    kubectl get all -n pai

    # Check the logs of a particular pod. For example, to check status of pod "pai-sample-b669d7897-nkkhz":
    kubectl logs pod/pai-sample-b669d7897-nkkhz -n pai
    ```
  