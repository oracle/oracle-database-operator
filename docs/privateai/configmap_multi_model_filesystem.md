# Configure Multiple AI Models on a File System

To configure multiple models whose files are available on a file system, define the models in `config.json` and specify each model’s file path in the appropriate model property.

Use the example file [`multi_model_filesystem_config.json`](./provisioning/multi_model_filesystem_config.json). The ConfigMap must contain the file under the key `config.json`, as expected by the Private AI configuration.

**Note:** For a list of supported model-specific configuration options, including `runtime`, `context_size`, `kv_cache`, and `runtime_arguments`, see [Inference Service Model Property](https://docs.oracle.com/en/database/oracle/oracle-database/26/prvai/inference-service-model-property.html).

Create or update the ConfigMap in the `pai` namespace. Run this command from the repository root:

```sh
kubectl create configmap multiconfigjson \
  --from-file=config.json=docs/privateai/provisioning/provisioning/multi_model_filesystem_config.json \
  --namespace pai \
  --dry-run=client -o yaml | kubectl apply -f -
```

Verify the ConfigMap and its `config.json` entry:

```sh
kubectl get configmap multiconfigjson -n pai -o yaml
```

Ensure the configured model paths are available and readable from within the Private AI pod.
