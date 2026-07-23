# Use only Specific Pre-approved model

If only specific included models need to be used, the admin can choose to explicitly specify them in the models section of config.json.

Create a `config.json` file to create a configmap.

You can use the example file [specific_preapproved_model.json](./provisioning/specific_preapproved_model.json).

Rename the file `specific_preapproved_model.json` to `config.json`.

Create a configmap using the above file as below:

```sh
kubectl create configmap privateaiconfigjson --from-file=config.json -n pai
```

You can check the details of the configmap as below:

```sh
kubectl get configmap -n pai
```