# Accessing the PrivateAI Container Pod in Kubernetes

**IMPORTANT:** To use this example, you must have an existing Oracle PrivateAI Container Deployment in the `pai` namespace, and you must have the following:
- The Reserved Public IP for the Public LoadBalancer
- The Private IP for the Internal LoadBalancer
- The AI Model details that you want to be used in the URL to access

## Accessing the PrivateAI Container in Kubernetes using REST Calls
Complete the following steps:

1. Retrieve the API Key

When you have used the file [pai_secret.sh](./provisioning/pai_secret.sh) during the deployment, you have a file named `api-key` generated in the same location. Copy this `api-key` file to the machine where you want to run the API Call to the Model Endpoint.

2. Keep the LoadBalancer Reserved Public IP or the Private IP ready. You can use this IP in the API Endpoint call in the next step.

3. If we assume the Loadbalancer Reserved Public IP from the previous step is `129.xxx.xxx.xxx`, you can use the following example command to see how to make an API Endpoint Call:
    ```sh
    curl -k --noproxy '*' -v -X POST --header "Content-Type: application/json"  --header "Authorization: Bearer `cat <PATH of the api-key file>/api-key`" -d '{"input": {"textList":["The quick brown fox jumped over the fence.","Another test sentence"]}}' https://129.xxx.xxx.xxx:443/omlmodels/all_minilm_l6_txt/score
    ```

**NOTE:** If you have a Private LoadBalancer, then use the Internal IP in place of the IP `129.xxx.xxx.xxx` in the preceding example.

## Accessing the PrivateAI Container using REST Calls with SSL certificate

To use SSL authentication while accessing the PrivateAI Container in Kubernetes using SSL certificate, complete the following additional steps:

1. Copy the `cert.pem` generated when you ran the `pai_secret.sh` script to the machine where you want to run the API Call to the Model Endpoint.
2. Use this key file to run the following modified example command to make an API Endpoint Call:
    ```sh
    curl --cacert cert.pem --noproxy '*' -v -X POST --header "Content-Type: application/json"  --header "Authorization: Bearer `cat <PATH of the api-key file>/api-key`" -d '{"input": {"textList":["The quick brown fox jumped over the fence.","Another test sentence"]}}' https://129.xxx.xxx.xxx:443/omlmodels/all_minilm_l6_txt/score
    ```
**NOTE:** 
- If you have a Private LoadBalancer, then use the Internal IP in place of the IP `129.xxx.xxx.xxx` in the preceding example.
- Replace the details of the AI Model in the URL used in the exmaple with the Model deployed.