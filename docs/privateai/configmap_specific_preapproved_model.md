# Use Only Selected Pre-approved Models

To make only selected pre-approved models available, explicitly specify those models in the `models` section of `config.json`.

The selected pre-approved model is included in the Private AI container image. You do not need to provide an HTTPS URL or download the model file separately.

Use the example file [`specific_preapproved_model.json`](./provisioning/specific_preapproved_model.json). The ConfigMap must contain the file under the key `config.json`, as expected by the Private AI configuration.

**Note:** For a list of supported model-specific configuration options, including `runtime`, `context_size`, `kv_cache`, and `runtime_arguments`, see [Inference Service Model Property](https://docs.oracle.com/en/database/oracle/oracle-database/26/prvai/inference-service-model-property.html).

Create or update the ConfigMap in the `pai` namespace. Run this command from the repository root:

```sh
kubectl create configmap privateaiconfigjson \
  --from-file=config.json=docs/privateai/provisioning/specific_preapproved_model.json \
  --namespace pai \
  --dry-run=client -o yaml | kubectl apply -f -
```

Verify the ConfigMap and its `config.json` entry:

```sh
kubectl get configmap privateaiconfigjson -n pai -o yaml
```
