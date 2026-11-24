# Add an AI Model to an Existing Private AI Deployment

The existing Private AI deployment uses multiple models configured through the `multiconfigjson` ConfigMap. For the deployment setup, see [Private AI with Multiple Models from HTTPS URLs and an Internal Load Balancer](./deploy_privateai_multi_model_https_internallb.md).

This procedure adds a model whose file is hosted at an HTTPS URL to the existing model configuration.

1. Edit the ConfigMap:

   ```sh
   kubectl edit configmap multiconfigjson -n pai
   ```

   In the ConfigMap YAML, find the `data` entry for `config.json`. Add the new model object to the JSON `models` array, preserving the existing entries and valid JSON syntax.

2. Verify that the updated `config.json` is valid JSON and includes the new model:

   ```sh
   kubectl get configmap multiconfigjson -n pai \
     -o jsonpath='{.data.config\.json}' | jq .
   ```

3. Wait for the Private AI operator to replace the Pod.

   ```sh
   kubectl get pods -n pai \
     -l app.kubernetes.io/name=pai-sample \
     -w
   ```

   Stop watching after the replacement Pod is ready.

4. Verify the configuration in the replacement Pod:

   ```sh
   POD_NAME=$(kubectl get pods -n pai \
     -l app.kubernetes.io/name=pai-sample \
     -o jsonpath='{.items[0].metadata.name}')

   kubectl exec -n pai "$POD_NAME" -- cat /privateai/config/config.json
   ```

**Note:** Replacing the Pod can briefly interrupt service when the deployment has one replica. The internal load balancer IP remains unchanged as long as its Service is not deleted or recreated. If the HTTPS URLs are pre-authenticated, treat the ConfigMap contents as sensitive because those URLs grant access to the model files.
