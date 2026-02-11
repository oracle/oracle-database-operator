# Accessing the PrivateAI Container Pod in Kubernetes

**IMPORTANT:** This example assumes that you have an existing Oracle PrivateAI Container Deployment in the `pai` namespace and you have:
- The Reserved Public IP in case of the Public LoadBalancer
- The Private IP in case of case of the Internal LoadBalancer
- The AI Model details to be used in the URL to access

Please follow the below steps to access:

1. Retrieve API Key

When you have used the file [pai_secret.sh](./provisioning/pai_secret.sh) during the deployment, you will have the file named `api-key` generated in the same location. Copy this file `api-key` to the machine where you want to run the API Call to the Model Endpoint.

2. Keep the LoadBalancer Reserved Public IP or the Private IP ready. You can use this IP in the API Endpoint call in the next step.

## Accessing the PrivateAI Container in Kubernetes using REST Calls
When HTTPS and authentication are enabled, you can add the Bearer token to the header. Note that if your certificate is self-signed, you need to add "-k" flag to the curl request, which tells curl to skip SSL certificate verification.

To get the list of Models, you can use below command:

```sh
curl -k --noproxy '*' --header "Authorization: Bearer `cat  <PATH of the api-key file>/api-key`" https://xxx.xxx.xxx.xxx:443/v1/models
```

Assume the Oracle PrivateAI Container Deployment is using Public Loadbalancer and the Loadbalancer Reserved Public IP is `xxx.xxx.xxx.xxx`, you can use the below example command to make an API Endpoint Call:
```sh
curl -k --noproxy '*' -X POST --header 'Content-Type: application/json' --header 'Accept: application/json' --header "Authorization: Bearer `cat <PATH of the api-key file>/api-key`" -d '{"model": "<<AI Model>>","input": ["The quick brown fox jumped over the fence.","Another test sentence"]}' https://xxx.xxx.xxx.xxx:443/v1/embeddings
```


**NOTE:** In case of the Private LoadBalancer, use the Internal IP in place of the IP `xxx.xxx.xxx.xxx` in above example.

## Accessing the PrivateAI Container using REST Calls with SSL certificate

In case you want to use SSL authentication while accessing the PrivateAI Container in Kubernetes using SSL certificate, then you will need to follow below additional steps:

1. Copy the `cert.pem` filee generated when you had run `pai_secret.sh` script, to the machine where you want to run the API Call to the Model Endpoint.
2. Use this key file while running the below modified example command to make an API Endpoint Call

To get the list of Models, you can use below command:

```sh
curl -k --noproxy '*' --header "Authorization: Bearer `cat  <PATH of the api-key file>/api-key`" https://xxx.xxx.xxx.xxx:443/v1/models
```

Assume the Oracle PrivateAI Container Deployment is using Public Loadbalancer and the Loadbalancer Reserved Public IP is `xxx.xxx.xxx.xxx`, you can use the below example command to make an API Endpoint Call:
```sh
curl --cacert cert.pem --noproxy '*' -v -X POST --header "Content-Type: application/json"  --header "Authorization: Bearer `cat <PATH of the api-key file>/api-key`" -d '{"model": "<<AI Model>>","input": ["The quick brown fox jumped over the fence.","Another test sentence"]}' https://xxx.xxx.xxx.xxx:443/v1/embeddings
```

**NOTE:** 
- In case of the Private LoadBalancer, use the Internal IP in place of the IP `xxx.xxx.xxx.xxx` in above example.
- Replace the details of the AI Model in the above URL with the Model deployed.