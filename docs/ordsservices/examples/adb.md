# OrdsSrvs Example: Autonomous Database without the OraOperator

This example walks through using the **OrdsSrvs controller** with an Oracle Autonomous Database.

This assumes that an ADB has already been provisioned and is configured as "Secure Access from Anywhere".
Note that if behind a Proxy, this example will not work as the Wallet will need to be modified to support the proxy configuration.

Before testing this example, please verify the prerequisites : [OrdsSrvs prerequisites](../README.md#prerequisites)

### ADB Wallet Secret

Download the ADB Wallet and create a Secret:

```bash
kubectl create secret generic adb-wallet \
  --from-file=<full_path_to_wallet.zip> -n ordsnamespace
```

Replace `<full_path_to_wallet.zip>` with the path to the wallet zip file.

### Database Credential Secrets

This example uses native Kubernetes Secrets for credentials.

```bash
kubectl create secret generic ordssrvs-auth \
  --from-literal=dbAuth='<ords-db-credential>' \
  --from-literal=adminAuth='<adb-admin-credential>' \
  -n ordsnamespace
```

### Create OrdsSrvs Resource

1. Create a manifest for ORDS.

    As an ADB already maintains ORDS and APEX, `autoUpgradeORDS` and `autoUpgradeAPEX` will be ignored if set. A new DB User for ORDS will be created or configured to avoid conflict with the pre-provisioned one. Explicitly set `db.username` to a user other than `ORDS_PUBLIC_USER`, such as `ORDS_PUBLIC_USER_OPER`; no username is substituted automatically.

    ```yaml
    apiVersion: database.oracle.com/v4
    kind:  OrdsSrvs
    metadata:
      name: ords-adb
      namespace: ordsnamespace
    spec:
      image: container-registry.oracle.com/database/ords:<ords-version>
      globalSettings:
        database.api.enabled: true
      poolSettings:
        - poolName: adb
          restEnabledSql.active: true
          plsql.gateway.mode: direct
          db.wallet.zip.service: <ADB_NAME>_TP
          dbWalletSecret:
            secretName: adb-wallet
            walletName: Wallet_<ADB_NAME>.zip
          db.username: ORDS_PUBLIC_USER_OPER
          db.secret:
            secretName:  ordssrvs-auth
            passwordKey: dbAuth
          db.adminUser: ADMIN
          db.adminUser.secret:
            secretName:  ordssrvs-auth
            passwordKey: adminAuth
    ```

    Replace `<ADB_NAME>` with the ADB name and `<ords-version>` with your image
    version. Ensure `db.wallet.zip.service` matches a service in your wallet
    (for example, `_TP` or `_HIGH`).

1. Watch the OrdsSrvs resource until the status is **Healthy**:
    ```bash
    kubectl get -n ordsnamespace ordssrvs ords-adb -w
    ```

    **NOTE**: If this is the first time pulling the ORDS image, it may take up to 5 minutes.

### Test

Open a port-forward to the ORDS service, for example:

```bash
kubectl port-forward service/ords-adb -n ordsnamespace 8443:8443
```

Direct your browser to: `https://localhost:8443/ords/adb`

> If `standalone.https.host` is configured, use that hostname instead of `localhost`.

## Conclusion

This example has a single database pool, named `adb`.  It is set to:

* Not automatically restart when the configuration changes: `forceRestart` is not set.
  The pod must be manually restarted for new configurations to be picked up.
* Leave ORDS and APEX schema management to ADB; the auto-upgrade settings are ignored.
* Use the ADB admin user to create or configure the ORDS runtime user.
* Use the ADB Wallet file to connect to the database: `db.wallet.zip.service: <ADB_NAME>_TP` and `dbWalletSecret`
