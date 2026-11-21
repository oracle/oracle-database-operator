# Configure a Single AI Model with an HTTPS URL

To configure a single model whose file is hosted at an HTTPS URL, define the model in `config.json` and specify its URL in the appropriate model property.

Use the example file [`single_model_https_config.json`](./provisioning/single_model_https_config.json). The ConfigMap must contain the file under the key `config.json`, as expected by the Private AI configuration.

**Note:** For a list of supported model-specific configuration options, including `runtime`, `context_size`, `kv_cache`, and `runtime_arguments`, see [Inference Service Model Property](https://docs.oracle.com/en/database/oracle/oracle-database/26/prvai/inference-service-model-property.html).

Create or update the ConfigMap in the `pai` namespace. Run this command from the repository root:

```sh
kubectl create configmap privateaiconfigjson \
  --from-file=config.json=docs/privateai/provisioning/provisioning/single_model_https_config.json \
  --namespace pai \
  --dry-run=client -o yaml | kubectl apply -f -
```

Verify the ConfigMap and its `config.json` entry:

```sh
kubectl get configmap privateaiconfigjson -n pai -o yaml
```

Ensure the HTTPS URL is reachable from the Private AI pod.
