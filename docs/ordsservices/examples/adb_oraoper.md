# OrdsSrvs Example: Autonomous Database using the OraOperator

This example walks through using the **OrdsSrvs controller** with an Autonomous Database managed through the OraOperator.

When connecting to a mTLS enabled ADB while using the OraOperator to retrieve the Wallet as is done in the example, it is currently not supported to have multiple, different databases supported by the single OrdsSrvs resource.  This is due to a requirement to set the `TNS_ADMIN` parameter at the Pod level ([#97](https://github.com/oracle/oracle-database-operator/issues/97)).

Before testing this example, please verify the prerequisites : [OrdsSrvs prerequisites](../README.md#prerequisites)

### Setup Oracle Cloud Authorisation

In order for the OraOperator to access the ADB, some additional prerequisites are required, as detailed [here](https://github.com/oracle/oracle-database-operator/blob/main/docs/adb/ADB_PREREQUISITES.md).
Either establish Instance Principles or create the required ConfigMap/Secret.  This example uses the latter, using the helper script [set_ocicredentials.sh](https://github.com/oracle/oracle-database-operator/blob/main/set_ocicredentials.sh) :

```bash
./set_ocicredentials.sh run -n ordsnamespace
```

### ADB Admin Credential Secret

Create a Secret for the ADB admin credential:

```bash
read -rsp "Enter ADB admin credential: " DBPWD
echo

printf '%s' "${DBPWD}" | kubectl create secret generic adb-oraoper-db-auth \
  --from-file=password=/dev/stdin \
  -n ordsnamespace
```

Example output:

```text
unset DBPWD
```

**NOTE**: When binding to the ADB in a later step, the OraOperator will change the ADB password to what is specified in the Secret.

### Bind the OraOperator to the ADB

1. Obtain the OCID of the ADB and set to an environment variable:

    ```bash
    export ADB_OCID=<insert OCID here>
    ```

1. Create and apply a manifest to bind to the ADB.
    "adb-oraoper-tns-admin" secret will be created by the controller.

    ```yaml
    apiVersion: database.oracle.com/v4
    kind: AutonomousDatabase
    metadata:
      name: adb-oraoper
      namespace: ordsnamespace
    spec:
      action: Sync
      wallet:
          name: adb-oraoper-tns-admin
          password:
            k8sSecret:
              name: adb-oraoper-db-auth
      details:
        id: $ADB_OCID
    ```

1. Update the ADB admin credential:

    ```bash
    kubectl patch adb adb-oraoper --type=merge \
      -n ordsnamespace \
      -p '{"spec":{"details":{"adminPassword":{"k8sSecret":{"name":"adb-oraoper-db-auth"}}}}}'
    ```

1. Watch the `adb` resource until the STATE is **AVAILABLE**:

    ```bash
    kubectl get -n ordsnamespace adb/adb-oraoper -w
    ```

### OrdsSrvs Credential Secret

```bash
kubectl create secret generic ordssrvs-auth \
  --from-literal=dbAuth='<ords-db-credential>' \
  --from-literal=adminAuth='<adb-admin-credential>' \
  -n ordsnamespace
```

### Create OrdsSrvs Resource

1. Obtain the Service Name from the OraOperator

    ```bash
    SERVICE_NAME=$(kubectl get -n ordsnamespace adb adb-oraoper -o=jsonpath='{.spec.details.dbName}')_TP
    ```

1. Create a manifest for ORDS.

    As an ADB already maintains ORDS and APEX, `autoUpgradeORDS` and `autoUpgradeAPEX` will be ignored if set.  A new DB User for ORDS will be created to avoid conflict with the pre-provisioned one.  This user will be
    named, `ORDS_PUBLIC_USER_OPER` if `db.username` is either not specified or set to `ORDS_PUBLIC_USER`.

    ```yaml
    apiVersion: database.oracle.com/v4
    kind:  OrdsSrvs
    metadata:
      name: ords-adb-oraoper
      namespace: ordsnamespace
    spec:
      image: container-registry.oracle.com/database/ords:<ords-version>
      forceRestart: true
      globalSettings:
        database.api.enabled: true
      poolSettings:
        - poolName: adb-oraoper
          db.connectionType: tns
          db.tnsAliasName: $SERVICE_NAME
          tnsAdminSecret:
            secretName: adb-oraoper-tns-admin
          restEnabledSql.active: true
          feature.sdw: true
          plsql.gateway.mode: proxied
          db.username: ORDS_PUBLIC_USER_OPER
          db.secret:
            secretName:  ordssrvs-auth
            passwordKey: dbAuth
          db.adminUser: ADMIN
          db.adminUser.secret:
            secretName:  ordssrvs-auth
            passwordKey: adminAuth
    ```
1. Watch the ordssrvs resource until the status is **Healthy**:
    ```bash
    kubectl get ordssrvs ords-adb-oraoper -n ordsnamespace -w
    ```

    **NOTE**: If this is the first time pulling the ORDS image, it may take up to 5 minutes.  If APEX
    is being installed for the first time by the Operator, it may remain in the **Preparing**
    status for an additional 5 minutes.


### Test

Open a port-forward to the ORDS service, for example:

```bash
kubectl port-forward service/ords-adb-oraoper -n ordsnamespace 8443:8443
```

Direct your browser to: `https://localhost:8443/ords/adb-oraoper`

## Conclusion

This example has a single database pool, named `adb-oraoper`.  It is set to:

* Automatically restart when the configuration changes: `forceRestart: true`
