# Example: Containerised Single Instance Database using the OraOperator

This example walks through using the **ORDS Operator** with a Containerised Oracle Database created by the **OraOperator** in the same Kubernetes Cluster.

### Install Cert-Manager

The OraOperator uses webhooks for validating user input before persisting it in etcd. 
Webhooks require TLS certificates that are generated and managed by a certificate manager.

Install [cert-manager](https://cert-manager.io/) with the following command:

```bash
kubectl apply -f https://github.com/cert-manager/cert-manager/releases/download/v1.14.5/cert-manager.yaml
```
<sup>latest cert-manager version, **v1.14.5**, valid as of **2-May-2024**</sup>

Check that all pods have a STATUS of **Running** before proceeding to the next step:
```bash
kubectl -n cert-manager get pods
```

Review [cert-managers installation documentation](https://cert-manager.io/docs/installation/kubectl/) for more information.

### Install OraOperator

Install the [Oracle Operator for Kubernetes](https://github.com/oracle/oracle-database-operator):

```bash
kubectl apply -f https://raw.githubusercontent.com/oracle/oracle-database-operator/main/oracle-database-operator.yaml
```

### Install ORDS Operator

Install the Oracle ORDS Operator:

```bash
kubectl apply -f https://github.com/gotsysdba/oracle-ords-operator/releases/latest/download/oracle-ords-operator.yaml
```

### Deploy a Containerised Oracle Database

1. Create a Secret for the Database password:

    ```bash
    DB_PWD=$(echo "ORDSpoc_$(date +%H%S%M)")

    kubectl create secret generic sidb-db-auth \
      --from-literal=password=${DB_PWD}
    ```
1. Create a manifest for the containerised Oracle Database.

    The POC uses an Oracle Free Image, but other versions may be subsituted; review the OraOperator Documentation for details on the manifests.

    ```bash
    echo "
    apiVersion: database.oracle.com/v1alpha1
    kind: SingleInstanceDatabase
    metadata:
      name: oraoper-sidb
    spec:
      replicas: 1
      image:
        pullFrom: container-registry.oracle.com/database/free:23.4.0.0
        prebuiltDB: true
      sid: FREE
      edition: free
      adminPassword:
        secretName: sidb-db-auth
        secretKey: password
      pdbName: FREEPDB1" | kubectl apply -f -
    ```
    <sup>latest container-registry.oracle.com/database/free version, **23.4.0.0**, valid as of **2-May-2024**</sup>

1. Watch the `singleinstancedatabases` resource until the database status is **Healthy**:

    ```bash
    kubectl get singleinstancedatabases/oraoper-sidb -w
    ```

    **NOTE**: If this is the first time pulling the free database image, it may take up to 15 minutes for the database to become available.

### Create RestDataServices Resource

1. Retrieve the Connection String from the containerised SIDB.

    ```bash
    CONN_STRING=$(kubectl get singleinstancedatabase oraoper-sidb \
      -o jsonpath='{.status.pdbConnectString}')

    echo $CONN_STRING
    ```

1. Create a manifest for ORDS.

    As the DB in the Free image does not contain ORDS (or APEX), the following additional keys are specified for the pool:
    * `autoUpgradeORDS` - Boolean; when true the ORDS will be installed/upgraded in the database
    * `autoUpgradeAPEX` - Boolean; when true the APEX will be installed/upgraded in the database
    * `db.adminUser` - User with privileges to install, upgrade or uninstall ORDS in the database (SYS).
    * `db.adminUser.secret` - Secret containing the password for `db.adminUser` (created in the first step)

    The `db.username` will be used as the ORDS schema in the database during the install/upgrade process (ORDS_PUBLIC_USER).

    ```bash
    echo "
    apiVersion: database.oracle.com/v1
    kind: RestDataServices
    metadata:
      name: ords-sidb
    spec:
      image: container-registry.oracle.com/database/ords:24.1.1
      forceRestart: true
      globalSettings:
        database.api.enabled: true
      poolSettings:
        - poolName: default
          autoUpgradeORDS: true
          autoUpgradeAPEX: true
          restEnabledSql.active: true
          plsql.gateway.mode: direct
          db.connectionType: customurl
          db.customURL: jdbc:oracle:thin:@//${CONN_STRING}
          db.username: ORDS_PUBLIC_USER
          db.secret:
            secretName:  sidb-db-auth
          db.adminUser: SYS
          db.adminUser.secret:
            secretName:  sidb-db-auth" | kubectl apply -f -
    ```
    <sup>latest container-registry.oracle.com/database/ords version, **24.1.1**, valid as of **30-May-2024**</sup>

1. Watch the restdataservices resource until the status is **Healthy**:
    ```bash
    kubectl get restdataservices ords-sidb -w
    ```

    **NOTE**: If this is the first time pulling the ORDS image, it may take up to 5 minutes.  If APEX
    is being installed for the first time by the Operator, it may remain in the **Preparing** 
    status for an additional 5 minutes.

    You can watch the APEX/ORDS Installation progress by running:

    ```bash
    POD_NAME=$(kubectl get pod -l "app.kubernetes.io/instance=ords-sidb" -o custom-columns=NAME:.metadata.name --no-headers)

    kubectl logs ${POD_NAME} -c ords-sidb-init -f
    ```

### Test

Open a port-forward to the ORDS service, for example:

```bash
kubectl port-forward service/ords-sidb 8443:8443
```

Direct your browser to: `https://localhost:8443/ords`

## Conclusion

This example has a single database pool, named `default`.  It is set to:

* Automatically restart when the configuration changes: `forceRestart: true`
* Automatically install/update ORDS on startup, if required: `autoUpgradeORDS: true`
* Automatically install/update APEX on startup, if required: `autoUpgradeAPEX: true`
* Use a basic connection string to connect to the database: `db.customURL: jdbc:oracle:thin:@//${CONN_STRING}`
* The `passwordKey` has been ommitted from both `db.secret` and `db.adminUser.secret` as the password was stored in the default key (`password`)