# Test Oracle PrivateAI Container Deployment using API Endpoint

**IMPORTANT:** This example assumes that you have an existing Oracle PrivateAI Container Deployment in the `pai` namespace.

Complete the following steps: 

1. Retrieve API Key

When you have used the file [pai_secret.sh](./pai_secret.sh) during the deployment, the file `api-key` is generated in the same location. Copy this `api-key` file to the machine where you want to run the API Call to the Model Endpoint.

2. Obtain the Loadbalancer External IP from the existing deployment of Oracle PrivateAI Container for the service `service/pai-sample-svc`. You can use this IP in the API Endpoint call in the next step.

3. If we assume the Loadbalancer Extenral IP from the last step is `141.xxx.xxx.xxx`, then the following commmand is an example of how to make an API Endpoint Call:
    ```sh
    curl -k --noproxy '*' -v -X POST --header "Content-Type: application/json"  --header "Authorization: Bearer `cat /home/opc/api-key`" -d '{"input": {"textList":["The quick brown fox jumped over the fence.","Another test sentence"]}}' https://141.xxx.xxx.xxx:443/omlmodels/all_minilm_v6/score
    ```