# ADD New AI Model to PrivateAI deployed in OKE cluster

The existing PrivateAI Container is deployed in the task example [PrivateAI Container using Multiple AI Models with HTTPS URL and an Internal LoadBalancer](./deploy_privateai_multi_model_https_internallb.md), using multiple AI Models. The HTTPS URLs for those model files are provided using a configmap. 

In this example, an additional AI Model is deployed to the existing deployment.

1. Use the following command to edit the configmap used by the existing PrivateAI Deployment to add a new AI Model:
    ```sh
    kubectl edit configmap multiconfigjson -n pai
    ```

2. Wait for few minutes. You should see the `configmap` updated inside the Pod. 

**NOTE:** The Pod will not be restarted in this case. The Application inside the Pod is periodically checking for Configmap changes.