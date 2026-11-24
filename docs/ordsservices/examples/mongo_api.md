# OrdsSrvs Controller: Oracle API for MongoDB Support

This example walks through using the **OrdsSrvs controller** with a Containerized Oracle Database to enable MongoDB API Support.

## Prerequisites

See [OrdsSrvs prerequisites](../README.md#prerequisites).

### Database Access

Use the database and credential Secret from the
[Containerized Single Instance Database example](./sidb_container.md).

This example assumes a running `SingleInstanceDatabase` resource named
`oraoper-sidb` in `ordsnamespace`, with an accessible `FREEPDB1` PDB and ORDS
already installed in that PDB. The database Pod must have the label
`app=oraoper-sidb`. The example uses the same admin password for SYS and SYSTEM.

### REST-enable a Schema

Create the database user used by the MongoDB client and enable its schema for
ORDS:

1. Connect to the database:

    ```bash
    DBPWD=$(kubectl get secrets '<secret-name>' -n ordsnamespace --template='{{.data.adminAuth | base64decode}}')
    POD_NAME=$(kubectl get pod -l "app=oraoper-sidb" -n ordsnamespace -o custom-columns=NAME:.metadata.name --no-headers)
    kubectl exec -it ${POD_NAME} -n ordsnamespace -- sqlplus SYSTEM/${DBPWD}@FREEPDB1
    ```

    Replace `<secret-name>` with the existing credential Secret from the SIDB
    example (`ordssrvs-auth` if unchanged). It must contain `adminAuth` and `dbAuth`.

1. Create the User:
    ```sql
    create user MONGO identified by "<password>";
    grant soda_app, create session, create table, create view, create sequence, create procedure, create job,
    unlimited tablespace to MONGO;
    -- Connect as new user
    conn MONGO/<password>@FREEPDB1;
    exec ords.enable_schema;
    ```

Use the same password in the MongoDB connection string below.

### Create ordssrvs Resource

1. Retrieve the connection string from the existing SIDB resource:

    ```bash
    CONN_STRING=$(kubectl get singleinstancedatabase oraoper-sidb \
      -n ordsnamespace \
      -o jsonpath='{.status.pdbConnectString}')

    echo $CONN_STRING
    ```

1. Create an `OrdsSrvs` custom resource.

    The following additional keys are specified for the pool:

    * `autoUpgradeORDS` - Boolean; when true, ORDS will be installed/upgraded in the database. Both `db.adminUser` and `db.adminUser.secret` must be set on the same pool.
    * `db.adminUser` - User with privileges to install, upgrade or uninstall ORDS in the database (SYS).
    * `db.adminUser.secret` - Secret containing the password for `db.adminUser` (`adminAuth` in the existing credential Secret).
    * `db.username` - ORDS runtime database user (`ORDS_PUBLIC_USER`).
    * APEX installation is not enabled in this example.

    ```bash
    cat > ords-sidb.yaml <<EOF
    apiVersion: database.oracle.com/v4
    kind: OrdsSrvs
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
          jdbc.MaxConnectionReuseTime: "900"
          jdbc.SecondsToTrustIdleConnection: 1
          jdbc.InitialLimit: 100
          jdbc.MaxLimit: 100
          db.connectionType: customurl
          db.customURL: jdbc:oracle:thin:@//${CONN_STRING}
          db.username: ORDS_PUBLIC_USER
          db.secret:
            secretName:  <secret-name>
            passwordKey: dbAuth
          db.adminUser: SYS
          db.adminUser.secret:
            secretName:  <secret-name>
            passwordKey: adminAuth
    EOF
    kubectl apply -f ords-sidb.yaml
    ```

    Replace `<secret-name>` with the same credential Secret used above.

1. Watch the OrdsSrvs resource until the status is **Healthy**:
    ```bash
    kubectl get ordssrvs ords-sidb -n ordsnamespace -w
    ```

    Image downloads and ORDS initialization can take several minutes. Inspect
    the `ordssrvs-init` logs to check progress or diagnose startup errors.

    You can watch ORDS initialization by running:

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
    mongosh  --tlsAllowInvalidCertificates 'mongodb://MONGO:<password>@localhost:27017/MONGO?authMechanism=PLAIN&authSource=$external&tls=true&retryWrites=false&loadBalanced=true'
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
* A number of JDBC parameters were set at the pool level for performance tuning:
    * `jdbc.MaxConnectionReuseCount: 5000`
    * `jdbc.MaxConnectionReuseTime: "900"`
    * `jdbc.SecondsToTrustIdleConnection: 1`
    * `jdbc.InitialLimit: 100`
    * `jdbc.MaxLimit: 100`
