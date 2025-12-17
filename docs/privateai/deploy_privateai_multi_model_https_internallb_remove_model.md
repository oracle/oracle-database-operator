# Remove an existing Model from PrivateAI deployed in OKE cluster

The existing PrivateAI Container is deployed in case [PrivateAI Container using Multiple AI Models with HTTPS URL and an Internal LoadBalancer](./deploy_privateai_multi_model_https_internallb.md) using multiple AI Models and HTTPS URLs for those model files are provided using a configmap. 

or 

Additional AI Models have been added to the existing PrivateAI Deployment using [Add New Model](./deploy_privateai_multi_model_https_internallb_add_model.md).

In this example, an existing AI Model is removed from the existing PrivateAI deployment.

1. Use the below command to edit the configmap used by the existing PrivateAI Deployment to removew an existing AI Model:
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