# Accessing the PrivateAI Container Pod in Kubernetes

**IMPORTANT:** This example assumes that you have an existing Oracle PrivateAI Container Deployment in the `pai` namespace and you have the Reserved Public IP of the LoadBalancer.

## Accessing the PrivateAI Container in Kubernetes using REST Calls
Please follow the below steps to access:

1. Retrieve API Key

When you have used the file [pai_secret.sh](./pai_secret.sh) during the deployment, you will have the file named `api-key` generated in the same location. Copy this file `api-key` to the machine where you want to run the API Call to the Model Endpoint.

2. Keep the LoadBalancer Reserved Public IP ready. You can use this IP in the API Endpoint call in the next step.

3. Assume the Loadbalancer Reserved Public IP from the last step is `129.xxx.xxx.xxx`, you can use the below command to make an API Endpoint Call:
    ```sh
    curl -k --noproxy '*' -v -X POST --header "Content-Type: application/json"  --header "Authorization: Bearer `cat /home/opc/api-key`" -d '{"input": {"textList":["The quick brown fox jumped over the fence.","Another test sentence"]}}' https://129.xxx.xxx.xxx:443/omlmodels/all_minilm_v6/score
    ```

## Accessing the PrivateAI Container using REST Calls with SSL certificate

In case you want to use SSL authentication while accessing the PrivateAI Container in Kubernetes using SSL certificate, then you will need to follow below additional steps:

1. Copy the `cert.pem` generated when you had run `pai_secret.sh` script, to the machine where you want to run the API Call to the Model Endpoint.
2. Use this key file while running the below modified command to make an API Endpoint Call:
    ```sh
    curl --cacert cert.pem --noproxy '*' -v -X POST --header "Content-Type: application/json"  --header "Authorization: Bearer `cat /home/opc/api-key`" -d '{"input": {"textList":["The quick brown fox jumped over the fence.","Another test sentence"]}}' https://129.xxx.xxx.xxx:443/omlmodels/all_minilm_v6/score
    ```

## Accessing the PrivateAI Container using PLSQL Commands from an Oracle Database

- The access to the PrivateAI Container deployed in Kubernetes Cluster is provided via the utl_to_embedding(s) apis which are part of the dbms_vector_chain package.
- When using the database as a client via the utl_to_embedding or utl_to_embeddings functions, a user will first need to create a credential containing the key using the CREATE_CREDENTIAL procedure. This credential is then referenced when registering the AI Container as a provider for generating embeddings.

We will follow the below steps to access the PrivateAI Container using PLSQL Commands from an Oracle Database:

- Copy the `cert.pem` file, which was generated during the SSL certificate creation at the time of the deploying PrivateAI Container in the Kubernetes Cluster, to the Oracle Database host.
- Create a wallet location, create the wallet and add `cert.pem` to this wallet:
    ```sh
    mkdir -p /home/oracle/wallet/
    orapki wallet create -wallet /home/oracle/wallet/ -pwd <walletpassword>
    orapki wallet add -wallet /home/oracle/wallet/ -trusted_cert -cert cert.pem -pwd <walletpassword>
    ```

- Add permission to connect as well as to use client certificates. You need to change principal_name to your user/schema("vectordb" in this case).
- Also, use the same `api-key` which was generated when you using the script `pai_secret.sh`
    ```sh
    connect / as sysdba
 
    BEGIN
        DBMS_NETWORK_ACL_ADMIN.APPEND_HOST_ACE(
            host => '*',
            ace => xs$ace_type(privilege_list => xs$name_list('connect'),
                               principal_name => 'vectordb',
                               principal_type => xs_acl.ptype_db));
    END;
    /

    BEGIN
        DBMS_NETWORK_ACL_ADMIN.APPEND_WALLET_ACE(
            wallet_path => 'file:/home/oracle/wallet',
            ace => xs$ace_type(
                privilege_list => xs$name_list('use_client_certificates', 'use_passwords'),
                principal_name => 'vectordb',
                principal_type => xs_acl.ptype_db));
    END;
    /
    ```

- Login as the user and create credential:
    ```sh
    connect vectordb/<vectord password>@<pdbname>
    exec dbms_vector.drop_credential('PRIVATEAI_CRED');

    declare
        jo json_object_t;
    begin
        jo := json_object_t();
        jo.put('access_token', '<api key>');
        dbms_vector.create_credential(
            credential_name   => 'PRIVATEAI_CRED',
            params            => json(jo.to_string));
    end;
    /
    ```
- If required, set proxy and no proxy uising "utl_http.set_proxy()"

- Set the wallet
    ```sh
    exec utl_http.set_wallet('file:/home/oracle/wallet/', '<walletpassword>');
    ```

- Declare embedding parameters:
    ```sh
    var params clob;
 
    begin
    :params := '
    {
        "provider": "oracle_ai_instance",
        "credential_name": "PRIVATEAI_CRED",
        "url": "https://HOSTNAME_OR_IP_ADDRESS_AI_CONTAINER:port_number/omlmodels/all_minilm_v6/score",
        "model": "all_minilm_v6",
    }';
    end;
    /
    ```

- Get the embeddings
    ```sh
    select dbms_vector.utl_to_embedding('Hello world', json(:params)) from dual;
    ```

- To list the models
    ```sh
    declare
    preferences clob;
    input clob;
    output clob;
    begin
    preferences := '{
    "provider": "oracle_ai_instance",
    "url": "https://HOSTNAME_OR_IP_ADDRESS_AI_CONTAINER:port_number/omlmodels",
    "credential_name": "PRIVATEAI_CRED",
    }';
    
    output := dbms_vector_chain.list_models(json(preferences));
    if length(output) > 5000 then
        dbms_output.put_line(dbms_lob.substr(output, 5000));
    else
        dbms_output.put_line(json_query(output, '$' returning clob pretty));
    end if;
    
    if output is not null then
        dbms_lob.freetemporary(output);
    end if;
    end;
    /
    ```