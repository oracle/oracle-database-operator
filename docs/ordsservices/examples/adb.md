# OrdsSrvs Example: Autonomous Database without the OraOperator

This example walks through using the **ORDSSRVS controller** with an Oracle Autonomous Database.  

This assumes that an ADB has already been provisioned and is configured as "Secure Access from Anywhere".  
Note that if behind a Proxy, this example will not work as the Wallet will need to be modified to support the proxy configuration.

Before testing this example, please verify the prerequisites : [ORDSSRVS prerequisites](../README.md#prerequisites)

### ADB Wallet Secret

Download the ADB Wallet and create a Secret, replacing `<full_path_to_wallet.zip>` with the path to the wallet zip file:

```bash
kubectl create secret generic adb-wallet \
  --from-file=<full_path_to_wallet.zip> -n ordsnamespace
```

### ADB ADMIN Password Secret

Create a Secret for the ADB ADMIN password, replacing <ADMIN_PASSWORD> with the real password:

```bash
echo -n "Enter ADB ADMIN password: " && read -s ADMIN_PASSWORD
openssl genpkey -algorithm RSA  -pkeyopt rsa_keygen_bits:2048 -pkeyopt rsa_keygen_pubexp:65537 > ca.key
openssl rsa -in ca.key -outform PEM  -pubout -out public.pem
kubectl create secret generic prvkey --from-file=secret=ca.key  -n ordsnamespace
echo -n "${ADMIN_PASSWORD}" |openssl pkeyutl -encrypt -pubin -inkey public.pem -pkeyopt rsa_padding_mode:oaep -pkeyopt rsa_oaep_md:sha256 |base64 > e_db-auth
kubectl create secret generic adb-oraoper-db-auth-enc  --from-file=password=e_db-auth -n  ordsnamespace
rm e_db-auth ca.key public.pem
unset ADMIN_PASSWORD
```

### Create RestDataServices Resource

1. Create a manifest for ORDS.

    As an ADB already maintains ORDS and APEX, `autoUpgradeORDS` and `autoUpgradeAPEX` will be ignored if set.  A new DB User for ORDS will be created to avoid conflict with the pre-provisioned one.  This user will be
    named, `ORDS_PUBLIC_USER_OPER` if `db.username` is either not specified or set to `ORDS_PUBLIC_USER`.

    Replace <ADB_NAME> with the ADB Name and ensure that the `db.wallet.zip.service` is valid for your ADB Workload (e.g. _TP or _HIGH, etc.):

    ```yaml
    apiVersion: database.oracle.com/v4
    kind:  OrdsSrvs
    metadata:
      name: ords-adb
      namespace: ordsnamespace
    spec:
      image: container-registry.oracle.com/database/ords:25.1.0
      encPrivKey:
        secretName: prvkey
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
            secretName:  adb-oraoper-db-auth-enc
          db.adminUser: ADMIN
          db.adminUser.secret:
            secretName:  adb-oraoper-db-auth-enc
    ```
    <sup>latest container-registry.oracle.com/database/ords version, **25.1.0**, valid as of **26-May-2025**</sup>
    
1. Watch the restdataservices resource until the status is **Healthy**:
    ```bash
    kubectl get -n ordsnamespace ordssrvs ords-adb -w
    ```

    **NOTE**: If this is the first time pulling the ORDS image, it may take up to 5 minutes.  If APEX
    is being installed for the first time by the Operator, it may remain in the **Preparing** 
    status for an additional 5 minutes.

### Test

Open a port-forward to the ORDS service, for example:

```bash
kubectl port-forward service/ords-adb -n ordsnamespace 8443:8443
```

Direct your browser to: `https://localhost:8443/ords/adb`

## Conclusion

This example has a single database pool, named `adb`.  It is set to:

* Not automatically restart when the configuration changes: `forceRestart` is not set.  
  The pod must be manually resarted for new configurations to be picked-up.
* Automatically install/update APEX on startup, if required: This occurs due to the database being detected as an ADB.
* The ADB `ADMIN` user will be used to connect the ADB to install APEX/ORDS
* Use the ADB Wallet file to connect to the database: `db.wallet.zip.service: adbpoc_tp` and `dbWalletSecret`