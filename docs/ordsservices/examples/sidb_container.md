# OrdsSrvs Controller: Containerized Single Instance Database using the OraOperator

This example walks through using the **OrdsSrvs Controller** with a Containerized Oracle Database created by the **SIDB Controller** in the same Kubernetes Cluster.

Before testing this example, please verify the prerequisites : [OrdsSrvs prerequisites](../README.md#prerequisites)

### Deploy a Containerized Oracle Database

Refer to Single Instance Database (SIDB) [README](https://github.com/oracle/oracle-database-operator/blob/main/docs/sidb/README.md) for details.

1. Create a Secret for the database admin credential and the ORDS database user credential:

    ```bash
    kubectl create secret generic ordssrvs-auth \
      --from-literal=dbAuth='<ords-db-credential>' \
      --from-literal=adminAuth='<database-admin-credential>' \
      --namespace ordsnamespace
    ```
1. Create a manifest for the containerized Oracle Database.

    The POC uses an Oracle Free Image, but other versions may be substituted; review the OraOperator documentation for details on the manifests.

    ```yaml
    apiVersion: database.oracle.com/v4
    kind: SingleInstanceDatabase
    metadata:
      name: oraoper-sidb
      namespace: ordsnamespace
    spec:
      edition: free
      adminPassword:
        secretName: ordssrvs-auth
        secretKey: adminAuth
      image:
        pullFrom: container-registry.oracle.com/database/free:<database-version>
        prebuiltDB: true
      replicas: 1
    ```

1. Watch the `singleinstancedatabases` resource until the database status is **Healthy**:

    ```bash
    kubectl get singleinstancedatabases/oraoper-sidb -w -n ordsnamespace
    ```
    **NOTE**: If this is the first time pulling the free database image, it may take up to 15 minutes for the database to become available.

### Create OrdsSrvs Resource

1. Retrieve the Connection String from the containerized SIDB.

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
    kubectl apply -f ords-sidb.yaml
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
      poolSettings:
        - poolName: default
          autoUpgradeORDS: true
          restEnabledSql.active: true
          plsql.gateway.mode: direct
          db.connectionType: customurl
          db.customURL: jdbc:oracle:thin:@//${CONN_STRING}
          db.username: ORDS_PUBLIC_USER
          db.secret:
            secretName:  ordssrvs-auth
            passwordKey: dbAuth
          db.adminUser: SYS
          db.adminUser.secret:
            secretName:  ordssrvs-auth
            passwordKey: adminAuth
    " > ords-sidb.yaml
    ```
1. Watch the ordssrvs resource until the status is **Healthy**:
    ```bash
    kubectl get ordssrvs ords-sidb -n ordsnamespace -w
    ```

    **NOTE**: If this is the first time pulling the ORDS image, it may take up to 5 minutes.  If APEX
    is being installed for the first time by the Operator, it may remain in the **Preparing**
    status for an additional 5 minutes.

    You can watch the APEX/ORDS Installation progress by running:

    ```bash
    POD_NAME=$(kubectl get pod -l "app.kubernetes.io/instance=ords-sidb" -n ordsnamespace -o custom-columns=NAME:.metadata.name --no-headers)

    kubectl logs ${POD_NAME} -c ordssrvs-init -n ordsnamespace -f
    ```

### Test

Open a port-forward to the ORDS service, for example:

```bash
kubectl port-forward service/ords-sidb -n ordsnamespace 8443:8443
```

Direct your browser to: `https://localhost:8443/ords`

## Conclusion

This example has a single database pool, named `default`.  It is set to:

* Automatically restart when the configuration changes: `forceRestart: true`
* Automatically install/update ORDS on startup, if required: `autoUpgradeORDS: true`
* Use a basic connection string to connect to the database: `db.customURL: jdbc:oracle:thin:@//${CONN_STRING}`
* The `passwordKey` fields identify the credential keys used by `db.secret` and `db.adminUser.secret`
