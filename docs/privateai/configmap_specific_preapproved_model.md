# Use only Specific Pre-approved model

If only specific included models need to be used, the admin can choose to explicitly specify them in the models section of `config.json`.

Create a `config.json` file to create a configmap.

You can use the example file [specific_preapproved_model.json](./provisioning/specific_preapproved_model.json).

Rename the file `specific_preapproved_model.json` to `config.json`.

Create a configmap using this file as follows:

```sh
kubectl create configmap privateaiconfigjson --from-file=config.json -n pai
```

Check the details of the configmap:

```sh
kubectl get configmap -n pai
```