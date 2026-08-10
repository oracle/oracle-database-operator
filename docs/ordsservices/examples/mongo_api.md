# OrdsSrvs Controller: Oracle API for MongoDB Support

This example walks through using the **OrdsSrvs controller** with a Containerized Oracle Database to enable MongoDB API Support.

Before testing this example, please verify the prerequisites : [OrdsSrvs prerequisites](../README.md#prerequisites)

### Database Access

This example assumes you have a running, accessible Oracle Database.  For demonstration purposes,
the [Containerized Single Instance Database using the OraOperator](sidb_container.md) will be used.

### REST-enable a Schema

In the database, create an ORDS-enabled user.  As this example uses the [Containerized Single Instance Database using the OraOperator](sidb_container.md), the following was performed:


1. Connect to the database:

    ```bash
    DBPWD=$(kubectl get secrets ordssrvs-auth -n ordsnamespace --template='{{.data.adminAuth | base64decode}}')
    POD_NAME=$(kubectl get pod -l "app=oraoper-sidb" -n ordsnamespace -o custom-columns=NAME:.metadata.name --no-headers)
    kubectl exec -it ${POD_NAME} -n ordsnamespace -- sqlplus SYSTEM/${DBPWD}@FREEPDB1
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

### Create ordssrvs Resource

1. Reuse the `ordssrvs-auth` Secret from the [Containerized Single Instance Database using the OraOperator](sidb_container.md) example and retrieve the Connection String from the containerized SIDB.

    ```bash
    CONN_STRING=$(kubectl get singleinstancedatabase oraoper-sidb \
      -n ordsnamespace \
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
    kind: OrdsSrvs
    ```

    Example output:

    ```text
    apiVersion: database.oracle.com/v4
    metadata:
      name: ords-sidb
      namespace: ordsnamespace
    spec:
      image: container-registry.oracle.com/database/ords:<ords-version>
      forceRestart: true
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
            secretName:  ordssrvs-auth
            passwordKey: dbAuth
          db.adminUser: SYS
          db.adminUser.secret:
            secretName:  ordssrvs-auth
            passwordKey: adminAuth" | kubectl apply -f -
    ```
1. Watch the OrdsSrvs resource until the status is **Healthy**:
    ```bash
    kubectl get ordssrvs ords-sidb -n ordsnamespace -w
    ```

    **NOTE**: If this is the first time pulling the ORDS image, it may take up to 5 minutes.  If APEX
    is being installed for the first time by the Operator, it may remain in the **Preparing**
    status for an additional 5 minutes.

    You can watch the APEX/ORDS Installation progress by running:

    ```bash
    POD_NAME=$(kubectl get pod -l "app.kubernetes.io/instance=ords-sidb" -o custom-columns=NAME:.metadata.name -n ordsnamespace --no-headers)

    kubectl logs ${POD_NAME} -c ordssrvs-init -n ordsnamespace -f
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
* The `passwordKey` fields identify the credential keys used by `db.secret` and `db.adminUser.secret`
* The MongoAPI service has been enabled: `mongo.enabled: true`
* The MongoAPI service will default to port: `27017` as the property: `mongo.port` has been left undefined
* A number of JDBC parameters were set at the pool level for achieving high performance:
    * `jdbc.MaxConnectionReuseCount: 5000`
    * `jdbc.MaxConnectionReuseTime: 900`
    * `jdbc.SecondsToTrustIdleConnection: 1`
    * `jdbc.InitialLimit: 100`
    * `jdbc.MaxLimit: 100`
