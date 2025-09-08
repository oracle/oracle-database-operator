# Configmap using Multiple AI Models on File System

Create a `config.json` file to create a configmap. This file has the HTTPS link for the AI Model File. 

You can use the example file [multi_model_filesystem_config.json](./provisioning/multi_model_filesystem_config.json).

Rename the file `multi_model_filesystem_config.json` to `config.json`.

Create a configmap using that renamed file. Example:
```sh
kubectl create configmap multiconfigjson --from-file=config.json -n pai
```

You can check the details of the configmap. Example:
```sh
kubectl get configmap -n pai
```