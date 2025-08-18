# Remove an existing Model from PrivateAI deployed in OKE cluster

The existing PrivateAI Container is deployed in case [PrivateAI Container using Multiple AI Models with HTTPS URL and an Internal LoadBalancer](./deploy_privateai_multi_model_https_internallb.md) using multiple AI Models and HTTPS URLs for those model files are provided using a configmap. 

or 

Additional AI Models have been added to the existing PrivateAI Deployment using [Add New Model](./deploy_privateai_multi_model_https_internallb_add_model.md).

In this example, an existing AI Model is removed from the existing PrivateAI deployment.

1. Use the below command to edit the configmap used by the existing PrivateAI Deployment to removew an existing AI Model:
    ```sh
    kubectl edit configmap multiconfigjson -n pai
    ```

2. Wait for few minutes and you should see configmap updated inside the Pod. 

**NOTE:** The Pod will not be restarted in this case. The Application inside the Pod is periodically checking for Configmap changes.