# Example: Containerised Single Instance Database using the OraOperator

This example walks through using the **ORDSSRVS Controller** with a Containerised Oracle Database created by the **SIDB Controller** in the same Kubernetes Cluster.

Before testing this example, please verify the prerequisites : [ORDSSRVS prerequisites](../README.md#prerequisites)

### Deploy a Containerised Oracle Database

Refer to Single Instance Database (SIDB) [README](https://github.com/oracle/oracle-database-operator/blob/main/docs/sidb/README.md) for details.

1. Create a Secret for the Database password:

    ```bash
    DB_PWD=<specify password here>
    kubectl create secret generic sidb-db-auth --from-literal=oracle_pwd=${DB_PWD} --namespace ordsnamespace
    ```
1. Create a manifest for the containerised Oracle Database.

    The POC uses an Oracle Free Image, but other versions may be subsituted; review the OraOperator Documentation for details on the manifests.

    ```yaml
    apiVersion: database.oracle.com/v4
    kind: SingleInstanceDatabase
    metadata:
      name: oraoper-sidb
      namespace: ordsnamespace
    spec:
      edition: free
      adminPassword:
        secretName: sidb-db-auth
      image:
        pullFrom: container-registry.oracle.com/database/free:23.7.0.0
        prebuiltDB: true
      replicas: 1
    ```
    <sup>latest container-registry.oracle.com/database/free version, **23.7.0.0-lite**, valid as of **2-May-2025**</sup>


1. Watch the `singleinstancedatabases` resource until the database status is **Healthy**:

    ```bash
    kubectl get singleinstancedatabases/oraoper-sidb -w -n ordsnamespace
    ```
    **NOTE**: If this is the first time pulling the free database image, it may take up to 15 minutes for the database to become available.

### Create encryped secret 

```bash
openssl  genpkey -algorithm RSA  -pkeyopt rsa_keygen_bits:2048 -pkeyopt rsa_keygen_pubexp:65537 > ca.key
openssl rsa -in ca.key -outform PEM  -pubout -out public.pem
kubectl create secret generic prvkey --from-file=privateKey=ca.key  -n ordsnamespace

echo "${DB_PWD}"     > db-auth
openssl pkeyutl -encrypt -pubin -inkey public.pem -in db-auth -pkeyopt rsa_padding_mode:oaep -pkeyopt rsa_oaep_md:sha256 |base64 > e_db-auth
kubectl create secret generic sidb-db-auth-enc --from-file=password=e_db-auth -n  ordsnamespace
rm db-auth e_db-auth
```

### Create RestDataServices Resource

1. Retrieve the Connection String from the containerised SIDB.

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
        passwordKey: privateKey
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
            secretName:  sidb-db-auth-enc
          db.adminUser: SYS
          db.adminUser.secret:
            secretName:  sidb-db-auth-enc
    " > ords-sidb.yaml

    kubectl apply -f ords-sidb.yaml
    ```
    <sup>latest container-registry.oracle.com/database/ords version, **25.1.0**, valid as of **26-May-2025**</sup>

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

    kubectl logs ${POD_NAME} -c ords-sidb-init -n ordsnamespace -f
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
* The `passwordKey` has been ommitted from both `db.secret` and `db.adminUser.secret` as the password was stored in the default key (`password`)
