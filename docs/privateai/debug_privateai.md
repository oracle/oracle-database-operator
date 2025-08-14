# Debug and Triubleshoot the PrivateAI Container Pod in Kubernetes

You can use the below commands to debug and troubleshoot the issues listed in this document:

## To check the logs of the PrivateAI Container Pod

Use the below command to get the logs of the PrivateAI Container Pod deployed in the Kubernetes Cluster using PrivateAI Controller:
   ```sh
    - Get the name of the PrivateAI Container Pod deployed in the namespace "pai"
    kubectl get pod -n pai

    - Get the logs of the PrivateAI Container Pod deployed in the namespace "pai"
    kubectl logs -f pod/<name of the pod> -n pai
   ```