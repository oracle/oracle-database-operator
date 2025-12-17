# ADD New AI Model to PrivateAI deployed in OKE cluster

The existing PrivateAI Container is deployed in case [PrivateAI Container using Multiple AI Models with HTTPS URL and an Internal LoadBalancer](./deploy_privateai_multi_model_https_internallb.md) using multiple AI Models and HTTPS URLs for those model files are provided using a configmap. 

In this example, an additional AI Model is deployed to the existing deployment.

1. Use the below command to edit the configmap used by the existing PrivateAI Deployment to add a new AI Model:
    ```sh
    kubectl edit configmap multiconfigjson -n pai
    ```

2. Wait for few minutes and you should see configmap updated inside the Pod. You can verify that using file `/privateai/config/config.json` inside the Pod.
    ```sh
    kubectl exec -i -t pod/pai-sample-699b88cdb-5h9bq -n pai /bin/bash

    # Once you are inside the pod, check the contents of below file:
    cat /privateai/config/config.json
    ```

**NOTE:** This step will result in termination of the existing PrivateAI Container Pod and creation of new Pod while the Internal LoadBalancer IP will not change.