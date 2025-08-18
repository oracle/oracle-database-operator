# Configmap using Single AI Model with HTTPS URL

Create a `config.json` file to create a configmap. This file has the HTTPS link for the AI Model File. 

You can use the example file [single_model_https_config.json](./provisioning/single_model_https_config.json).

Rename the file `single_model_https_config.json` to `config.json`.

Create a configmap using the above file as below:
```sh
kubectl create configmap omlconfigjson --from-file=config.json -n pai
```

You can check the details of the configmap as below:
```sh
kubectl get configmap -n pai
```