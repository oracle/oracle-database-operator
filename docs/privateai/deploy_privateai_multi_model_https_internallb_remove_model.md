# Remove a Model from an Existing Private AI Deployment

The existing Private AI deployment uses multiple models configured through the `multiconfigjson` ConfigMap. See [Private AI with Multiple Models from HTTPS URLs and an Internal Load Balancer](./deploy_privateai_multi_model_https_internallb.md).

If you added models after the initial deployment, see [Add an AI Model to an Existing Private AI Deployment](./deploy_privateai_multi_model_https_internallb_add_model.md).

In this example, you remove a model from the existing deployment. This removes the model from Private AI’s configuration; it does not delete the model file at its HTTPS URL.

1. Edit the ConfigMap:

   ```sh
   kubectl edit configmap multiconfigjson -n pai
   ```

   In `data.config.json`, remove the complete model object from the `models` array. Preserve the other model entries and ensure the JSON syntax remains valid.

2. Verify that the model entry is absent from the ConfigMap:

   ```sh
   kubectl get configmap multiconfigjson -n pai -o yaml
   ```

3. Wait for the Private AI Pod to be replaced, then confirm that the new Pod is ready:

   ```sh
   kubectl get pods -n pai -l app.kubernetes.io/name=pai-sample -w
   ```

4. Verify the configuration in the new Pod:

   ```sh
   POD_NAME=$(kubectl get pods -n pai \
     -l app.kubernetes.io/name=pai-sample \
     -o jsonpath='{.items[0].metadata.name}')

   kubectl exec -n pai "$POD_NAME" -- cat /privateai/config/config.json
   ```

**Note:** Updating the ConfigMap causes the existing Pod to be terminated and replaced so the deployment can load the updated model configuration. Expect a brief service interruption while the replacement Pod starts. The Internal LoadBalancer IP address remains unchanged.