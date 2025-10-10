# Example: Autonomous Database using the OraOperator

This example walks through using the **ORDS Controller** with a Containerised Oracle Database created by the **ADB Controller** in the same Kubernetes Cluster.

When connecting to a mTLS enabled ADB while using the OraOperator to retreive the Wallet as is done in the example, it is currently not supported to have multiple, different databases supported by the single Ordssrvs resource.  This is due to a requirement to set the `TNS_ADMIN` parameter at the Pod level ([#97](https://github.com/oracle/oracle-database-operator/issues/97)).

Before testing this example, please verify the prerequisites : [ORDSSRVS prerequisites](../README.md#prerequisites)

### Setup Oracle Cloud Authorisation

In order for the OraOperator to access the ADB, some additional pre-requisites are required, as detailed [here](https://github.com/oracle/oracle-database-operator/blob/main/docs/adb/ADB_PREREQUISITES.md).  
Either establish Instance Principles or create the required ConfigMap/Secret.  This example uses the later, using the helper script [set_ocicredentials.sh](https://github.com/oracle/oracle-database-operator/blob/main/set_ocicredentials.sh) :

```bash
./set_ocicredentials.sh run -n ordsnamespace
```

### ADB ADMIN Password Secret

Create a Secret for the ADB Admin password:

```bash
DB_PWD=$(echo "ORDSpoc_$(date +%H%S%M)")

kubectl create secret generic adb-oraoper-db-auth \
  -n ordsnamespace \
  --from-literal=adb-oraoper-db-auth=${DB_PWD}
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

1. Update the ADMIN Password:

    ```bash
    kubectl patch adb adb-oraoper --type=merge \
      -n ordsnamespace \
      -p '{"spec":{"details":{"adminPassword":{"k8sSecret":{"name":"adb-oraoper-db-auth"}}}}}'
    ```

1. Watch the `adb` resource until the STATE is **AVAILABLE**:

    ```bash
    kubectl get -n ordsnamespace adb/adb-oraoper -w
    ```

### Create encrypted password 

```bash
echo ${DB_PWD} > db-auth
openssl  genpkey -algorithm RSA  -pkeyopt rsa_keygen_bits:2048 -pkeyopt rsa_keygen_pubexp:65537 > ca.key
openssl rsa -in ca.key -outform PEM  -pubout -out public.pem
kubectl create secret generic prvkey --from-file=privateKey=ca.key  -n ordsnamespace
openssl pkeyutl -encrypt -pubin -inkey public.pem -in db-auth -pkeyopt rsa_padding_mode:oaep -pkeyopt rsa_oaep_md:sha256 |base64 > e_db-auth
kubectl create secret generic adb-oraoper-db-auth-enc  --from-file=password=e_db-auth -n  ordsnamespace
rm db-auth e_db-auth
```

### Create OrdsSrvs Resource

1. Obtain the Service Name from the OraOperator

    ```bash
    SERVICE_NAME=$(kubectl get -n ordsnamespace adb adb-oraoper -o=jsonpath='{.spec.details.dbName}'_TP)
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
      image: container-registry.oracle.com/database/ords:25.1.0
      forceRestart: true
      encPrivKey:
        secretName: prvkey
        passwordKey: privateKey
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
            secretName:  adb-oraoper-db-auth-enc
          db.adminUser: ADMIN
          db.adminUser.secret:
            secretName:  adb-oraoper-db-auth-enc
    ```
    <sup>latest container-registry.oracle.com/database/ords version, **25.1.0**, valid as of **26-May-2025**</sup>

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
* Automatically install/update ORDS on startup, if required.  This occurs due to the database being detected as an ADB.
* Automatically install/update APEX on startup, if required: This occurs due to the database being detected as an ADB.
* The ADB `ADMIN` user will be used to connect the ADB to install APEX/ORDS
* Use a TNS connection string to connect to the database: `db.customURL: jdbc:oracle:thin:@//${CONN_STRING}`
  The `tnsAdminSecret` Secret `adb-oraoper-tns-admin` was created by the OraOperator
* The `passwordKey` has been specified for both `db.secret` and `db.adminUser.secret` as `adb-oraoper-password` to match the OraOperator specification.
* The ADB `ADMIN` user will be used to connect the ADB to install APEX/ORDS