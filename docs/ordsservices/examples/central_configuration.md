# OrdsSrvs Controller: Central Configuration via central.config.url

This feature introduces support for configuring ORDS instances managed by the OrdsSrvs controller using a central configuration manager. By setting the `central.config.url` attribute, OrdsSrvs retrieves global and pool-specific settings from a central endpoint that implements the ORDS Central Config Manager OpenAPI.

This document shows:
- A minimal OrdsSrvs example that uses `central.config.url`
- A demo central config manager implemented with Apache HTTPD in Kubernetes
- How to validate the setup and make REST calls against multiple pools

See ORDS docs: [Configuring additional databases][ords-config-addl-dbs] (tested with 25.3):  
https://docs.oracle.com/en/database/oracle/oracle-rest-data-services/25.3/ordig/configuring-additional-databases.html#GUID-EEDA7256-7EDE-467B-B71D-6C7C184D982E

>Note: This example is for demo/testing only. Do not use plaintext passwords or HTTP in production.

## Overview

- Central configuration endpoint:
  - Global config: `GET /central/v1/config`
  - Pool config: `GET /central/v1/config/pool/{poolName}`
- In this example, the pool is resolved from the URL path using `security.externalMappingPathPrefix = true`.
- Example pools: `pool-a` and `pool-b`
- ORDS base path: `/ords`
- Example REST access: `/ords/{poolName}/...`

## Prerequisites

See ORDSSRVS prerequisites: [ORDSSRVS prerequisites](../README.md#prerequisites)

In addition to the above, you’ll need:
- A reachable Oracle database for the pools
- Target Kubernetes namespace (replace NAMESPACE)

Security reminder:
>- Use HTTPS/TLS for the central config service and for ORDS in production.
>- Store credentials in Kubernetes Secrets (not ConfigMaps) and avoid plaintext passwords.
>- Restrict network access via NetworkPolicies.

## Central Config Manager files

Create these four files locally. We will package them into a ConfigMap.

- **central-config-httpd.conf** httpd config
- **central-config-global.json** ORDS global configuration
- **central-config-pool-a.json** pool-a configuration
- **central-config-pool-b.json** pool-b configuration 

central-config-httpd.conf:
```
ServerName localhost
ServerRoot "/usr/local/apache2"
Listen 80
LoadModule mpm_event_module modules/mod_mpm_event.so
LoadModule authz_core_module modules/mod_authz_core.so
LoadModule dir_module modules/mod_dir.so
LoadModule mime_module modules/mod_mime.so
LoadModule log_config_module modules/mod_log_config.so
LoadModule rewrite_module modules/mod_rewrite.so
LoadModule unixd_module modules/mod_unixd.so
LoadModule alias_module modules/mod_alias.so
User www-data
Group www-data
PidFile /usr/local/apache2/logs/httpd.pid
ErrorLog /proc/self/fd/2
LogLevel warn
CustomLog /proc/self/fd/1 common
DocumentRoot "/usr/local/apache2/htdocs"
<Directory "/usr/local/apache2/htdocs">
  AllowOverride None
  Require all granted
</Directory>
AddType application/json .json
FileETag MTime Size
RewriteEngine On
RewriteRule ^/health$ /ok.txt [PT,L]
#RewriteCond %{REQUEST_URI} ^/central/
#RewriteCond %{HTTP:Request-Id} =""
#RewriteRule ^ - [R=400,L]
RewriteRule ^/central/v1/config$ /global.json [PT,L]
#RewriteRule ^/central/v1/config/pool/localhost /pools/default.json [PT,L]
RewriteRule ^/central/v1/config/pool/([^/]+)$ /pools/$1.json [PT,L]
DirectoryIndex disabled
```

central-config-global.json:
```
{
  "settings": {
    "security.externalMappingPathPrefix": true,
    "restEnabledSql.active": true,
    "feature.sdw": true,
    "debug.debugger": "debug",
    "standalone.https.port": "8443",
    "standalone.http.port": "8080",
    "security.forceHTTPS": false,
    "standalone.context.path": "/ords"
  },
  "links": [
    {
      "rel": "collection",
      "href": "http://central-config-svc/central/v1/config/"
    },
    {
      "rel": "self",
      "href": "http://central-config-svc/central/v1/config/"
    },
    {
      "rel": "search",
      "href": "http://central-config-svc/central/v1/config/pool/{host}",
      "templated": true
    }
  ]
}
```

central-config-pool-a.json:
```
{
  "database": {
    "pool": {
      "name": "pool-a",
      "settings": {
        "db.connectionType": "customurl",
        "db.customURL": "jdbc:oracle:thin:@sidb:1521/FREEPDB1",
        "db.username": "ORDS_PUBLIC_USER",
        "db.password": "<password>"
      }
    }
  }
}
```

central-config-pool-b.json:
```
{
  "database": {
    "pool": {
      "name": "pool-b",
      "settings": {
        "db.connectionType": "customurl",
        "db.customURL": "jdbc:oracle:thin:@sidb:1521/FREEPDB1",
        "db.username": "ORDS_PUBLIC_USER",
        "db.password": "<password>"
      }
    }
  }
}
```

>Important: Passwords here are for testing only. 

## Create the ConfigMap with the central config content

```
kubectl -n NAMESPACE create configmap central-config \
  --from-file=central-config-httpd.conf \
  --from-file=central-config-global.json \
  --from-file=central-config-pool-a.json \
  --from-file=central-config-pool-b.json
```

## Deploy the demo central config server (Apache HTTPD) and Service

central-config-server.template:
```
apiVersion: apps/v1
kind: Deployment
metadata:
  name: central-config-server
  namespace: NAMESPACE
spec:
  replicas: 2
  selector:
    matchLabels:
      app: central-config-server
  template:
    metadata:
      labels:
        app: central-config-server
    spec:
      containers:
      - name: httpd
        image: docker.io/library/httpd:latest
        ports:
        - containerPort: 80
        volumeMounts:
        - name: httpd-conf
          mountPath: /usr/local/apache2/conf/httpd.conf
          subPath: httpd.conf
          readOnly: true
        - name: config
          mountPath: /usr/local/apache2/htdocs
          readOnly: true
      volumes:
      - name: httpd-conf
        configMap:
          name: central-config
          items:
          - key: central-config-httpd.conf
            path: httpd.conf
      - name: config
        configMap:
          name: central-config
          items:
          - key: central-config-global.json
            path: global.json
          - key: central-config-pool-a.json
            path: pools/pool-a.json
          - key: central-config-pool-b.json
            path: pools/pool-b.json
---
apiVersion: v1
kind: Service
metadata:
  name: central-config-svc
  namespace: NAMESPACE
spec:
  type: ClusterIP
  selector:
    app: central-config-server
  ports:
  - port: 80
    targetPort: 80
```

Apply:
```
kubectl apply -f central-config-server.template
```

## Validate the central config endpoints

from the cluster network:
```
  curl http://central-config-svc/central/v1/config
  curl http://central-config-svc/central/v1/config/pool/pool-a
  curl http://central-config-svc/central/v1/config/pool/pool-b
```

You should receive the JSON documents defined above.

## Configure OrdsSrvs to use central.config.url

OrdsSrvs manifest:
```
apiVersion: database.oracle.com/v4
kind: OrdsSrvs
metadata:
  name: ordssrvs-cc
  namespace: NAMESPACE
spec:
  image: container-registry.oracle.com/database/ords:latest
  central.config.url: http://central-config-svc/central/v1/config
  serviceAccountName: ordssrvs-sa
```

Apply:
```
kubectl apply -f ordssrvs-central-config.yaml
```

The controller will:
- Fetch the global config from `/central/v1/config`
- Resolve the pool by URL path (because `security.externalMappingPathPrefix` is true)
- Fetch pool configs from `/central/v1/config/pool/{poolName}` as requests arrive

## Prepare a test schema and object in the database

Run the following in your database (adjust schema if needed). Example uses schema `ORDSSRVSTESTCASE`.

```
CREATE TABLE TESTCASE ( STATUS VARCHAR2 (100));
TRUNCATE TABLE TESTCASE;
INSERT INTO TESTCASE VALUES ('ORDSSRVS_TESTCASE_CHECK');

BEGIN
  ORDS.enable_schema(
    p_enabled             => TRUE,
    p_schema              => 'ORDSSRVSTESTCASE',
    p_url_mapping_type    => 'BASE_PATH',
    p_url_mapping_pattern => 'ordssrvs_testcase',
    p_auto_rest_auth      => FALSE
  );
END;
/

BEGIN
  ORDS.ENABLE_OBJECT(
    p_enabled      => TRUE,
    p_schema       => 'ORDSSRVSTESTCASE',
    p_object       => 'TESTCASE',
    p_object_type  => 'TABLE',
    p_object_alias => 'testcase_table'
  );
END;
/
```

## Test REST calls through ORDS with path-based pool resolution

Assuming ORDS is listening on 8443 and using the `/ords` context path:

```
curl -ik https://ordssrvs-cc:8443/ords/pool-a/ordssrvs_testcase/testcase_table/ -H "Host: localhost"
curl -ik https://ordssrvs-cc:8443/ords/pool-b/ordssrvs_testcase/testcase_table/ -H "Host: localhost"
```

- The pool is resolved from the path segment after `/ords/` (pool-a or pool-b).
- There is no server-name-to-pool mapping in this example.

## Troubleshooting

- OrdsSrvs not picking up central config:
  - Check logs of the OrdsSrvs pods for fetch errors
  - Confirm `central.config.url` is reachable via in-cluster DNS
  - Validate central config endpoints return HTTP 200 and JSON

- Pool resolution issues:
  - Ensure `security.externalMappingPathPrefix = true` in the global config
  - Confirm the path is `/ords/{poolName}/...`

- Database connectivity:
  - Verify `db.customURL`, `db.username`, and credentials for each pool
  - Use Secrets for credentials in production
  - Check network access from ORDS pods to the database

- Security:
  - Use TLS for the central config service (`https`) and for ORDS
  - Align with Oracle internal security/compliance guidelines when deploying third-party images (e.g., httpd)

## Notes and Best Practices

- Replace `NAMESPACE`, and `<password>` with your values.
- For production:
  - Replace HTTP with HTTPS and configure trusted certificates
  - Move credentials to Secrets or wallet-based authentication
  - Add health and readiness probes to both central-config and ORDS
  - Apply NetworkPolicies to limit ingress/egress
  - Implement caching/failover strategies for central config availability

This example demonstrates how OrdsSrvs can centrally source both global and pool configurations, enabling consistent, scalable ORDS deployments with path-based pool selection.