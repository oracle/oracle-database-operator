# Configmap using Single AI Model with HTTPS URL

If only one model is available and the model file is accessible through an HTTPS URL, the administrator can specify the URL in the models section of `config.json`.

Create a `config.json` file to create a configmap. This file has the HTTPS link for the AI Model File.

You can use the example file [single_model_https_config.json](./provisioning/single_model_https_config.json).

Rename the file `single_model_https_config.json` to `config.json`.

Create a configmap using this file as follows:

```sh
kubectl create configmap privateaiconfigjson --from-file=config.json -n pai
```

Check the details of the configmap:

```sh
kubectl get configmap -n pai
```
