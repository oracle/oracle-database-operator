# OrdsSrvs Controller: Oracle API for MongoDB Support

This example walks through using the **ORDSSRVS Controller** with a Containerised Oracle Database to enable MongoDB API Support.

Before testing this example, please verify the prerequisites : [ORDSSRVS prerequisites](../README.md#prerequisites)

### Database Access

This example assumes you have a running, accessible Oracle Database.  For demonstration purposes,
the [Containerised Single Instance Database using the OraOperator](sidb_container.md) will be used.

### Rest Enable a Schema

In the database, create an ORDS-enabled user.  As this example uses the [Containerised Single Instance Database using the OraOperator](sidb_container.md), the following was performed:


1. Connect to the database: 

    ```bash
    DB_PWD=$(kubectl get secrets sidb-db-auth --template='{{.data.password | base64decode}}')
    POD_NAME=$(kubectl get pod -l "app=oraoper-sidb" -o custom-columns=NAME:.metadata.name --no-headers)
    kubectl exec -it ${POD_NAME} -- sqlplus SYSTEM/${DB_PWD}@FREEPDB1
    ```
    
1. Create the User:
    ```sql
    create user MONGO identified by "<password>";
    grant soda_app, create session, create table, create view, create sequence, create procedure, create job, 
    unlimited tablespace to MONGO;
    -- Connect as new user
    conn MONGO/<password>@FREEPDB1;
    exec ords.enable_schema;
    ```

### Create encrypted secrets 

```bash

openssl  genpkey -algorithm RSA  -pkeyopt rsa_keygen_bits:2048 -pkeyopt rsa_keygen_pubexp:65537 > ca.key
openssl rsa -in ca.key -outform PEM  -pubout -out public.pem
kubectl create secret generic prvkey --from-file=privateKey=ca.key  -n ordsnamespace

echo -n "Enter password: " && read -s DBPWD
echo -n "${DBPWD}" | openssl pkeyutl -encrypt -pubin -inkey public.pem -pkeyopt rsa_padding_mode:oaep -pkeyopt rsa_oaep_md:sha256 |base64 > e_db-auth
kubectl create secret generic sidb-db-auth-enc --from-file=password=e_db-auth -n  ordsnamespace
rm e_db-auth
```

### Create ordssrvs Resource

1. Retrieve the Connection String from the containerised SIDB.

    ```bash
    CONN_STRING=$(kubectl get singleinstancedatabase oraoper-sidb \
      -o jsonpath='{.status.pdbConnectString}')

    echo $CONN_STRING
    ```

1. Create a manifest for ORDS.

    As the DB in the Free image does not contain ORDS (or APEX), the following additional keys are specified for the pool:
    * `autoUpgradeORDS` - Boolean; when true the ORDS will be installed/upgraded in the database
    * `db.adminUser` - User with privileges to install, upgrade or uninstall ORDS in the database (SYS).
    * `db.adminUser.secret` - Secret containing the password for `db.adminUser` (created in the first step)

    The `db.username` will be used as the ORDS schema in the database during the install/upgrade process (ORDS_PUBLIC_USER).

    ```bash
    echo "
    apiVersion: database.oracle.com/v4
    kind: OrdsSrvs
    metadata:
      name: ords-sidb
      namespace: ordsnamespace
    spec:
      image: container-registry.oracle.com/database/ords:25.1.0
      forceRestart: true
      encPrivKey:
        secretName: prvkey
      globalSettings:
        database.api.enabled: true
        mongo.enabled: true
      poolSettings:
        - poolName: default
          autoUpgradeORDS: true
          restEnabledSql.active: true
          plsql.gateway.mode: direct
          jdbc.MaxConnectionReuseCount: 5000
          jdbc.MaxConnectionReuseTime: 900
          jdbc.SecondsToTrustIdleConnection: 1
          jdbc.InitialLimit: 100
          jdbc.MaxLimit: 100
          db.connectionType: customurl
          db.customURL: jdbc:oracle:thin:@//${CONN_STRING}
          db.username: ORDS_PUBLIC_USER
          db.secret:
            secretName:  sidb-db-auth-enc
          db.adminUser: SYS
          db.adminUser.secret:
            secretName:  sidb-db-auth-enc" | kubectl apply -f -
    ```
    <sup>latest container-registry.oracle.com/database/ords version, **25.1.0**, valid as of **26-May-2025**</sup>

    
1. Watch the restdataservices resource until the status is **Healthy**:
    ```bash
    kubectl get ordssrvs ords-sidb -w
    ```

    **NOTE**: If this is the first time pulling the ORDS image, it may take up to 5 minutes.  If APEX
    is being installed for the first time by the Operator, it may remain in the **Preparing** 
    status for an additional 5 minutes.

    You can watch the APEX/ORDS Installation progress by running:

    ```bash
    POD_NAME=$(kubectl get pod -l "app.kubernetes.io/instance=ords-sidb" -o custom-columns=NAME:.metadata.name -n ordsnamespace --no-headers)

    kubectl logs ${POD_NAME} -c ords-sidb-init -n ordsnamespace -f
    ```

### Test

1. Open a port-forward to the MongoAPI service, for example:
    ```bash
    kubectl port-forward service/ords-sidb 27017:27017 -n ordsnamespace
    ```

1. Connect to ORDS using the MongoDB shell:
    ```bash
    mongosh  --tlsAllowInvalidCertificates 'mongodb://MONGO:<password>!@localhost:27017/MONGO?authMechanism=PLAIN&authSource=$external&tls=true&retryWrites=false&loadBalanced=true' 
    ```

1. Insert some data:
    ```txt
    db.createCollection('emp');
    db.emp.insertOne({"name":"Blake","job": "Intern","salary":30000});
    db.emp.insertOne({"name":"Miller","job": "Programmer","salary": 70000});
    db.emp.find({"name":"Miller"});
    ```

## Conclusion

This example has a single database pool, named `default`.  It is set to:

* Automatically restart when the configuration changes: `forceRestart: true`
* Automatically install/update ORDS on startup, if required: `autoUpgradeORDS: true`
* Use a basic connection string to connect to the database: `db.customURL: jdbc:oracle:thin:@//${CONN_STRING}`
* The `passwordKey` has been ommitted from both `db.secret` and `db.adminUser.secret` as the password was stored in the default key (`password`)
* The MongoAPI service has been enabled: `mongo.enabled: true`
* The MongoAPI service will default to port: `27017` as the property: `mongo.port` has been left undefined
* A number of JDBC parameters were set at the pool level for achieving high performance:
    * `jdbc.MaxConnectionReuseCount: 5000`
    * `jdbc.MaxConnectionReuseTime: 900`
    * `jdbc.SecondsToTrustIdleConnection: 1`
    * `jdbc.InitialLimit: 100`
    * `jdbc.MaxLimit: 100`
