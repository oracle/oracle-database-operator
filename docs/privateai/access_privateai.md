# Accessing the PrivateAI Container Pod in Kubernetes

This page has the details to access the PrivateAI Container deployed in Kubernetes:
- [Accessing the PrivateAI Container in Kubernetes using REST Calls](#accessing-the-privateai-container-in-kubernetes-using-rest-calls) 
- [Accessing the PrivateAI Container using REST Calls with SSL certificate](#accessing-the-privateai-container-using-rest-calls-with-ssl-certificate) 
- [Accessing the PrivateAI Container from Oracle Database](#accessing-the-privateai-container-from-oracle-database)
  - [Create wallet](#create-wallet) 
  - [Create Database user to access the PrivateAI Container](#create-database-user-to-access-the-privateai-container) 
  - [Access the PrivateAI Container from within the Oracle Database](#access-the-privateai-container-from-within-the-oracle-database) 
    - [Access the PrivateAI Container using "dbms_vector.utl_to_embedding()"](#access-the-privateai-container-using-dbms_vectorutl_to_embedding) 
    - [Access the PrivateAI Container using "dbms_vector_chain.utl_to_embedding()"](#access-the-privateai-container-using-dbms_vector_chainutl_to_embedding)

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


## Accessing the PrivateAI Container from Oracle Database

You can access the PrivateAI Container deployed in Kubernetes from an Oracle Database using PL/SQL Commands. You can use `dbms_vector.utl_to_embedding()` or `dbms_vector_chain.utl_to_embedding()` for this purpose.

**Note:** Your Oracle Database version must be 23.26.0.0.0 (26ai) to access the PrivateAI Container. 

You need to follow the below steps:

### Create wallet
- Create a wallet on the Database Host using `orapki` 
- Copy the `cert.pem` filee generated when you had run `pai_secret.sh` script, to the Database Host and add to the wallet using `orapki` 
- If you have more than 1 node then you need to execute the orapki commands on all Database Nodes 

  ```sh
  rm -rf /home/oracle/wallet/*
  mkdir -p /home/oracle/wallet/
  orapki wallet create -wallet /home/oracle/wallet/ -pwd <<wallet password>>
  orapki wallet add -wallet /home/oracle/wallet/ -trusted_cert -cert cert.pem -pwd Oracle_26ai
  ```

### Create Database user to access the PrivateAI Container
- Create a user on the Database. In this example, the user is created at the PDB `ORCLPDB` Level. 
  ```sh
  sqlplus "/ as sysdba"
  alter session set container=ORCLPDB;
  create user vectordb identified by <<Password>>;
  ```
- Add permission to connect and use client certificates to this user. 
- Change principal_name to this user to grant the "connect" network privilege for the specified host:
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

  **Note:** In above command, replace `xxx.xxx.xxx.xxx` with the Public IP or the Private IP of the LoadBalancer used in the deployment of PrivateAI Container on Kubernetes. 

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

### Access the PrivateAI Container from within the Oracle Database
- Connect to the PDB as the user you have created earlier:
  ```sh
  connect vectordb/"<<Password>>"@orclpdb
  ```
- Create credential named `ORACLEAI_CRED` using "api-key" generated when you used `pai_secret.sh` script during the PrivateAI Container Deployment: 
  ```sh
  -- Drop the credential in case existing earlier with this name:
  exec dbms_vector.drop_credential('ORACLEAI_CRED');

  declare
    jo json_object_t;
  begin
    jo := json_object_t();
    jo.put('access_token', 'b5aa613a41e6767c4980e09f6f9cbb549aaa6d3dbaf2991641417e31b59fe2ad');
    dbms_vector.create_credential(
      credential_name   => 'ORACLEAI_CRED',
      params            => json(jo.to_string));
  end;
  /
  ```
- Set proxy and add LoadBalancer IP address to no proxy list. For Example:
  ```sh
  exec utl_http.set_proxy('<proxy-hostname>:<port>', 'localhost,127.0.0.1,IP_ADDRESS_AI_CONTAINER,<additional-bypass-hosts-or-domains>');
  ```
- Set wallet
  ```sh
  exec utl_http.set_wallet('file:/home/oracle/wallet/', '<<wallet password>>');
  ```

#### Access the PrivateAI Container using "dbms_vector.utl_to_embedding()"
- Declare embedding parameters: 
  ```sh
  var embed_params clob;
 
  begin
   :embed_params := '{
    "provider": "privateai",
    "url": "https://xxx.xxx.xxx.xxx:443/v1/embeddings",
    "credential_name": "ORACLEAI_CRED",  
    "model": "clip-vit-base-patch32-txt",
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
 


- Get the embeddings
SET LONG 50000;
select dbms_vector.utl_to_embedding('Hello world', json(:embed_params)) from dual;

#### Access the PrivateAI Container using "dbms_vector_chain.utl_to_embedding()"
- Get the list of available models from the PrivateAI Container deployed in Kubernetes 
- In this case, `xxx.xxx.xxx.xxx` is the LoadBalancer IP and `ORACLEAI_CRED` is the credential name created earlier 
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

- Select the AI Model from the list of Models from above list. In this example, the AI Model `clip-vit-base-patch32-txt` is be used in this step to get the Text embeddings:

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
