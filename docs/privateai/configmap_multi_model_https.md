# Configure Multiple AI Models with HTTPS URLs

To configure multiple models whose files are hosted at HTTPS URLs, define the models in `config.json` and specify each model’s URL in the appropriate model property.

Use the example file [`multi_model_https_config.json`](./provisioning/multi_model_https_config.json). The ConfigMap must contain the file under the key `config.json`, as expected by the Private AI configuration.

**Note:** For a list of supported model-specific configuration options, including runtime, context_size, kv_cache, and runtime_arguments, see [Inference Service Model Property](https://docs.oracle.com/en/database/oracle/oracle-database/26/prvai/inference-service-model-property.html).

Create or update the ConfigMap in the `pai` namespace. Run this command from the repository root:

```sh
kubectl create configmap multiconfigjson \
  --from-file=config.json=docs/privateai/provisioning/provisioning/multi_model_https_config.json \
  --namespace pai \
  --dry-run=client -o yaml | kubectl apply -f -
```

Verify the ConfigMap and its `config.json` entry:

```sh
kubectl get configmap multiconfigjson -n pai -o yaml
```

Ensure the HTTPS URLs are reachable from the Private AI pod.
