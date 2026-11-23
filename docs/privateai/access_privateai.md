# Accessing the Private AI Container Pod in Kubernetes

This page has the details to access the Private AI Container deployed in Kubernetes using Oracle Database Operator Private AI Controller:

- [Accessing the Private AI Container in Kubernetes using REST Calls](#accessing-the-private-ai-container-in-kubernetes-using-rest-calls)
- [Accessing the Private AI Container using REST Calls with SSL certificate](#accessing-the-private-ai-container-using-rest-calls-with-ssl-certificate)
- [Accessing the Private AI Container from Oracle Database](#accessing-the-private-ai-container-from-oracle-database)
  - [Create wallet](#create-wallet)
  - [Create Database user to access the Private AI Container](#create-database-user-to-access-the-private-ai-container)
  - [Access the Private AI Container from within the Oracle Database](#access-the-private-ai-container-from-within-the-oracle-database)
    - [Access the Private AI Container using "dbms_vector.utl_to_embedding()"](#access-the-private-ai-container-using-dbms_vectorutl_to_embedding)
    - [Access the Private AI Container using "dbms_vector_chain.utl_to_embedding()"](#access-the-private-ai-container-using-dbms_vector_chainutl_to_embedding)
    - [Access the Private AI Container Using "dbms_vector.utl_to_generate_text()"](#access-the-private-ai-container-using-dbms_vectorutl_to_generate_text)

**IMPORTANT:** This example assumes that you have an existing Oracle Private AI Container Deployment in the `pai` namespace and you have:

- The Reserved Public IP in case of the Public LoadBalancer
- The Private IP in case of case of the Internal LoadBalancer
- The AI Model details to be used in the URL to access

Please follow the below steps to access:

1. Retrieve API Key

    - When you have used the file [create_auth_secret.sh](./auth-secret/create_auth_secret.sh) during the deployment, you will have the file named `api-key` generated in the same location under directory `privateai-auth-secret`. Copy this file `api-key` to the machine where you want to run the API Call to the Model Endpoint.
    - When you have used the legacy file [pai_secret.sh](./provisioning/pai_secret.sh) during the deployment, you will have the file named `api-key` generated in the same location. Copy this file `api-key` to the machine where you want to run the API Call to the Model Endpoint.

2. Extract the CA Certificate

    - When you have used the file [tr_cert_manager.sh](./tls-cert-manager/tr_cert_manager.sh) to create certificate during the deployment, you can use below command to extract the `CA` certificate and save it to a local file:

      ```sh
      kubectl -n pai get secret privateai-tls -o json | jq -r '.data["ca.crt"]' | base64 -d > ca.crt
      ```

      Copy this file `ca.crt` to the machine where you want to run the API Call to the Model Endpoint.
    - When you have used the legacy file [pai_secret.sh](./provisioning/pai_secret.sh) during the deployment, you will have the file named `cert.pem` generated in the same location. Copy this file as `ca.crt` to the machine where you want to run the API Call to the Model Endpoint.

3. LoadBalancer IP

    Keep the LoadBalancer Reserved Public IP or the Private IP ready. You can use this IP in the API Endpoint call in the next step.

## Accessing the Private AI Container in Kubernetes using REST Calls

When HTTPS and authentication are enabled, you can add the Bearer token to the header. Note that if your certificate is self-signed, you need to add "-k" flag to the curl request, which tells curl to skip SSL certificate verification.

Assume the Oracle Private AI Container Deployment is using Public Loadbalancer and the Loadbalancer Reserved Public IP is `xxx.xxx.xxx.xxx`.

To get the list of Models, you can use below command:

```sh
export API_KEY_FILE=/secure/path/api-key
curl -k --noproxy '*' \
  --header "Authorization: Bearer $(<"$API_KEY_FILE")" \
  "https://xxx.xxx.xxx.xxx:443/v1/models"
```

To make an API Endpoint Call:

```sh
export API_KEY_FILE=/secure/path/api-key
curl -k --noproxy '*' -X POST \
  --header 'Content-Type: application/json' --header 'Accept: application/json' \
  --header "Authorization: Bearer $(<"$API_KEY_FILE")" \
  -d '{"model": "<<AI Model>>","input": ["The quick brown fox jumped over the fence.","Another test sentence"]}' https://xxx.xxx.xxx.xxx:443/v1/embeddings
```

In case of an LLM Model, use the below syntax:

```sh
export API_KEY_FILE=/secure/path/api-key
curl -k --noproxy '*' -X POST --header 'Content-Type: application/json' --header "Authorization: Bearer $(<"$API_KEY_FILE")" --data '{
   "model": "<< AI LLM Model Name>>",
   "messages":[
      {"role": "user", "content": "What does the SELECT statement in SQL do?"}
   ],                             
   "max_tokens": 256,
   "temperature": 0.7
}' https://xxx.xxx.xxx.xxx:443/v1/chat/completions
```

**NOTE:** In case of the Private LoadBalancer, use the Internal IP in place of the IP `xxx.xxx.xxx.xxx` in above examples.

## Accessing the Private AI Container using REST Calls with SSL certificate

In case you want to use SSL authentication while accessing the Private AI Container in Kubernetes using SSL certificate, then you will need to follow below additional steps:

1. Copy the `ca.crt` file to the machine where you want to run the API Call to the Model Endpoint.
2. Use this key file while running the below modified example command to make an API Endpoint Call

To get the list of Models, you can use below command:

```sh
export API_KEY_FILE=/secure/path/api-key
export CA_FILE=/secure/path/ca.crt
curl --cacert "$CA_FILE" --noproxy '*' \
  --header "Authorization: Bearer $(<"$API_KEY_FILE")" \
  "https://xxx.xxx.xxx.xxx:443/v1/models"
```

Assume the Oracle Private AI Container Deployment is using Public Loadbalancer and the Loadbalancer Reserved Public IP is `xxx.xxx.xxx.xxx`, you can use the below example command to make an API Endpoint Call:

```sh
export API_KEY_FILE=/secure/path/api-key
export CA_FILE=/secure/path/ca.crt
curl --cacert "$CA_FILE" --noproxy '*' -v -X POST \
  --header "Content-Type: application/json"  \
  --header "Authorization: Bearer $(<"$API_KEY_FILE")" \
  -d '{"model": "<<AI Model>>","input": ["The quick brown fox jumped over the fence.","Another test sentence"]}' https://xxx.xxx.xxx.xxx:443/v1/embeddings
```

In case of an LLM Model, use the below syntax:

```sh
export API_KEY_FILE=/secure/path/api-key
export CA_FILE=/secure/path/ca.crt
curl --cacert "$CA_FILE" --noproxy '*' -X POST --header 'Content-Type: application/json' --header "Authorization: Bearer $(<"$API_KEY_FILE")" --data '{
   "model": "<<AI Model>>",
   "messages":[
      {"role": "user", "content": "What does the SELECT statement in SQL do?"}
   ],                             
   "max_tokens": 256,
   "temperature": 0.7
}' https://xxx.xxx.xxx.xxx:443/v1/chat/completions
```

**NOTE:**

- In case of the Private LoadBalancer, use the Internal IP in place of the IP `xxx.xxx.xxx.xxx` in above examples.
- Replace the details of the AI Model in the above URL with the available model name.

## Accessing the Private AI Container from Oracle Database

You can access the Private AI Container deployed in Kubernetes from an Oracle Database using PL/SQL Commands. You can use `dbms_vector.utl_to_embedding()` or `dbms_vector_chain.utl_to_embedding()` for this purpose.

**Note:** Your Oracle database version must be Oracle AI Database 26ai (23.26.0.0.0) to access the Private AI Container.

You must complete all of the following steps:

### Create wallet

- Create a wallet on the Database Host using `orapki`
- Copy the CA Certificate file from earlier step or the `cert.pem` file generated when you had run `pai_secret.sh` script, to the Database Host as `ca.crt` and add to the wallet using `orapki`
- If you have more than one node, then you must run the `orapki` commands on all database nodes.

  ```sh
  rm -rf /home/oracle/wallet/*
  mkdir -p /home/oracle/wallet/
  orapki wallet create -wallet /home/oracle/wallet/ -pwd <<wallet password>>
  orapki wallet add -wallet /home/oracle/wallet/ -trusted_cert -cert ca.crt -pwd Oracle_26ai
  ```

### Create Database User to Access the Private AI Container

- Create a user on the database. In this example, the user is created at the PDB `ORCLPDB` Level.

  ```sh
  sqlplus "/ as sysdba"
  alter session set container=ORCLPDB;
  create user vectordb identified by <<Password>>;
  ```

- Add permission to connect and use client certificates to this user. 
- Change `principal_name` to this user to grant the "connect" network privilege for the specified host:

  ```sh
  grant connect, resource, dba to vectordb;
  grant create any credential to vectordb;
  
  BEGIN
      DBMS_NETWORK_ACL_ADMIN.APPEND_HOST_ACE(
          host => 'xxx.xxx.xxx.xxx',
          ace => xs$ace_type(privilege_list => xs$name_list('connect'),
                             principal_name => 'vectordb',
                             principal_type => xs_acl.ptype_db));
  END;
  /
  ```

  **Note:** In the preceding command, replace `xxx.xxx.xxx.xxx` with the Public IP or the Private IP of the LoadBalancer used in the deployment of Private AI Container on Kubernetes.

  ```sh
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

### Access the Private AI Container from within the Oracle Database

- Connect to the PDB as the user that you previously created:

  ```sh
  connect vectordb/"<<Password>>"@orclpdb
  ```

- Create credential named `ORACLEAI_CRED` using "api-key" generated when you used `pai_secret.sh` script during the Private AI Container Deployment:

  ```sh
  -- Drop the credential in case existing earlier with this name:
  exec dbms_vector.drop_credential('ORACLEAI_CRED');

  declare
    jo json_object_t;
  begin
    jo := json_object_t();
    jo.put('access_token', 'b5aa..................................59fe2ad');
    dbms_vector.create_credential(
      credential_name   => 'ORACLEAI_CRED',
      params            => json(jo.to_string));
  end;
  /
  ```

- Set proxy and add LoadBalancer IP address to the no proxy list. For example:

  ```sh
  exec utl_http.set_proxy('<proxy-hostname>:<port>', 'localhost,127.0.0.1,IP_ADDRESS_AI_CONTAINER,<additional-bypass-hosts-or-domains>');
  ```

- Set wallet

  ```sh
  exec utl_http.set_wallet('file:/home/oracle/wallet/', '<<wallet password>>');
  ```

#### Access the Private AI Container using "dbms_vector.utl_to_embedding()"

- Declare embedding parameters:

  ```sh
  var embed_params clob;
 
  begin
   :embed_params := '{
    "provider": "privateai",
    "url": "https://xxx.xxx.xxx.xxx:443/v1/embeddings",
    "credential_name": "ORACLEAI_CRED",  
    "model": "clip-vit-base-patch32-txt"
  }';
  end;
  /
  ```

  **Note:** `xxx.xxx.xxx.xxx` is the LoadBalancer IP and `clip-vit-base-patch32-txt` is the AI Model.

- Get the embeddings:

  ```sh
  SET LONG 50000;
  select dbms_vector.utl_to_embedding('Hello world', json(:embed_params)) from dual;
  ```

#### Access the Private AI Container using "dbms_vector_chain.utl_to_embedding()"

- Obtain the list of available models from the Private AI Container deployed in Kubernetes
- In this example, `xxx.xxx.xxx.xxx` is the LoadBalancer IP and `ORACLEAI_CRED` is the credential name created earlier

  ```sh
  set serveroutput on;
  declare
    preferences clob;
    input clob;
    output clob;
  begin
    preferences := '{
    "provider": "privateai",
    "url": "https://xxx.xxx.xxx.xxx:443/v1/models",
    "credential_name": "ORACLEAI_CRED",
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

- Select the AI Model from the list of models from the above list. In this example, the AI Model `clip-vit-base-patch32-txt` is used in this step to obtain the Text embeddings:

  ```sh
  set serveroutput on;
  declare
    preferences clob;
    input clob;
    output clob;
    v vector;
    ja json_array_t;
  begin
    preferences := '
  {
    "provider": "privateai",
    "url": "https://xxx.xxx.xxx.xxx:443/v1/embeddings",
    "credential_name": "ORACLEAI_CRED",
    "model": "clip-vit-base-patch32-txt"
  }';
    input := 'hello world';
    v := dbms_vector_chain.utl_to_embedding(input, json(preferences));
    output := to_clob(v);
    ja := json_array_t(output);
    dbms_output.put_line('vector size=' || ja.get_size);
    dbms_output.put_line('vector length=' || length(output));
    dbms_output.put_line('vector data=' || dbms_lob.substr(output, 10000) || '...');
  end;
  /
  ```

#### Access the Private AI Container Using "dbms_vector.utl_to_generate_text()"

- This example sends a prompt to a text-generation model through the Private AI chat completions endpoint.
- In this example, `xxx.xxx.xxx.xxx` is the Load Balancer IP, and `ORACLEAI_CRED` is the credential created earlier. Replace `your-text-generation-model` with the model ID returned by the Private AI `/v1/models` endpoint. Confirm that the model supports text generation.
- For vLLM’s OpenAI-compatible endpoint, use `"provider": "openai"` and the `/v1/chat/completions` URL. For details, see [Oracle Database 26ai `UTL_TO_GENERATE_TEXT` documentation](https://docs.oracle.com/en/database/oracle/oracle-database/26/vecse/utl_to_generate_text-dbms_vector.html).

```sql
SET SERVEROUTPUT ON;

DECLARE
  preferences CLOB;
  input       CLOB;
  output      CLOB;
BEGIN
  preferences := '{
    "provider": "openai",
    "url": "https://xxx.xxx.xxx.xxx:443/v1/chat/completions",
    "credential_name": "ORACLEAI_CRED",
    "model": "your-text-generation-model",
    "max_tokens": 128,
    "temperature": 0
  }';

  input := 'In one sentence, explain what an embedding is.';

  output := DBMS_VECTOR.UTL_TO_GENERATE_TEXT(
    TO_CLOB(input),
    JSON(preferences)
  );

  IF output IS NOT NULL THEN
    DBMS_OUTPUT.PUT_LINE(DBMS_LOB.SUBSTR(output, 5000, 1));
  END IF;
END;
/
```
