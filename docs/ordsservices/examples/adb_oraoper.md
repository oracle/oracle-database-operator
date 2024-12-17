# Example: Autonomous Database using the OraOperator

This example walks through using the **ORDS Operator** with a Containerised Oracle Database created by the **OraOperator** in the same Kubernetes Cluster.

When connecting to a mTLS enabled ADB while using the OraOperator to retreive the Wallet as is done in the example, it is currently not supported to have multiple, different databases supported by the single RestDataServices resource.  This is due to a requirement to set the `TNS_ADMIN` parameter at the Pod level ([#97](https://github.com/oracle/oracle-database-operator/issues/97)).

### Install Cert-Manager

The OraOperator uses webhooks for validating user input before persisting it in etcd. 
Webhooks require TLS certificates that are generated and managed by a certificate manager.

Install [cert-manager](https://cert-manager.io/) with the following command:

```bash
kubectl apply -f https://github.com/cert-manager/cert-manager/releases/download/v1.14.5/cert-manager.yaml
```
<sup>latest cert-manager version, **v1.14.5**, valid as of **30-May-2024**</sup>

Check that all pods have a STATUS of **Running** before proceeding to the next step:
```bash
kubectl -n cert-manager get pods
```

Review [cert-managers installation documentation](https://cert-manager.io/docs/installation/kubectl/) for more information.

### Install OraOperator

Install the [Oracle Operator for Kubernetes](https://github.com/oracle/oracle-database-operator/tree/main):

```bash
kubectl apply -f https://raw.githubusercontent.com/oracle/oracle-database-operator/main/oracle-database-operator.yaml
```

### Setup Oracle Cloud Authorisation

In order for the OraOperator to access the ADB, some pre-requisites are required, as detailed [here](https://github.com/oracle/oracle-database-operator/blob/main/docs/adb/ADB_PREREQUISITES.md).  Either establish Instance Principles or create the required ConfigMap/Secret.  This example uses the later:

```
kubectl create configmap oci-cred \
--from-literal=tenancy=<TENANCY_OCID> \
--from-literal=user=<USER_OCID> \
--from-literal=fingerprint=<FINGERPRINT> \
--from-literal=region=<REGION>

kubectl create secret generic oci-privatekey \
--from-file=privatekey=<full path to private key>
```

### Install ORDS Operator

Install the Oracle ORDS Operator:

```bash
kubectl apply -f https://github.com/gotsysdba/oracle-ords-operator/releases/latest/download/oracle-ords-operator.yaml
```

### ADB ADMIN Password Secret

Create a Secret for the ADB Admin password:

```bash
DB_PWD=$(echo "ORDSpoc_$(date +%H%S%M)")

kubectl create secret generic adb-oraoper-db-auth \
  --from-literal=adb-oraoper-db-auth=${DB_PWD}
```

**NOTE**: When binding to the ADB in a later step, the OraOperator will change the ADB password to what is specified in the Secret.

### Bind the OraOperator to the ADB

1. Obtain the OCID of the ADB and set to an environment variable:

  ```
  export ADB_OCID=<insert OCID here>
  ```

1. Create a manifest to bind to the ADB.

    ```bash
    echo "
    apiVersion: database.oracle.com/v1alpha1
    kind: AutonomousDatabase
    metadata:
      name: adb-oraoper
    spec:
      hardLink: false
      ociConfig:
        configMapName: oci-cred
        secretName: oci-privatekey
      details:
        autonomousDatabaseOCID: $ADB_OCID
        wallet:
          name: adb-oraoper-tns-admin
          password:
            k8sSecret:
              name: adb-oraoper-db-auth" | kubectl apply -f -
    ```

1. Update the ADMIN Password:

```bash
  kubectl patch adb adb-oraoper --type=merge \
    -p '{"spec":{"details":{"adminPassword":{"k8sSecret":{"name":"adb-oraoper-db-auth"}}}}}'
```

1. Watch the `adb` resource until the STATE is **AVAILABLE**:

    ```bash
    kubectl get adb/adb-oraoper -w
    ```

### Create RestDataServices Resource

1. Obtain the Service Name from the OraOperator

  ```bash
  SERVICE_NAME=$(kubectl get adb adb-oraoper -o=jsonpath='{.spec.details.dbName}'_TP)
  ```

1. Create a manifest for ORDS.

    As an ADB already maintains ORDS and APEX, `autoUpgradeORDS` and `autoUpgradeAPEX` will be ignored if set.  A new DB User for ORDS will be created to avoid conflict with the pre-provisioned one.  This user will be
    named, `ORDS_PUBLIC_USER_OPER` if `db.username` is either not specified or set to `ORDS_PUBLIC_USER`.

    ```bash
    echo "
    apiVersion: database.oracle.com/v1
    kind: RestDataServices
    metadata:
      name: ords-adb-oraoper
    spec:
      image: container-registry.oracle.com/database/ords:24.1.1
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
            secretName:  adb-oraoper-db-auth
            passwordKey: adb-oraoper-db-auth
          db.adminUser: ADMIN
          db.adminUser.secret:
            secretName:  adb-oraoper-db-auth
            passwordKey: adb-oraoper-db-auth" | kubectl apply -f -
    ```
    <sup>latest container-registry.oracle.com/database/ords version, **24.1.1**, valid as of **30-May-2024**</sup>

1. Watch the restdataservices resource until the status is **Healthy**:
    ```bash
    kubectl get restdataservices ords-adb-oraoper -w
    ```

    **NOTE**: If this is the first time pulling the ORDS image, it may take up to 5 minutes.  If APEX
    is being installed for the first time by the Operator, it may remain in the **Preparing** 
    status for an additional 5 minutes.


### Test

Open a port-forward to the ORDS service, for example:

```bash
kubectl port-forward service/ords-adb-oraoper 8443:8443
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