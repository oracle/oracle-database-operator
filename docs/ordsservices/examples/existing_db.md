# Example: Pre-existing Database

This example walks through configuring the ORDS Controller to use either a database deployed within Kubernetes, or an existing database external to your cluster.

Before testing this example, please verify the prerequisites : [ORDSSRVS prerequisites](../README.md#prerequisites)

### Database Access

This example assumes you have a running, accessible Oracle Database.  

```bash
export CONN_STRING=<database host ip or scan>:<port>/<service_name>
```

### Create encrypted secrets 

```bash
DB_PWD=<specify password here>
openssl  genpkey -algorithm RSA  -pkeyopt rsa_keygen_bits:2048 -pkeyopt rsa_keygen_pubexp:65537 > ca.key
openssl rsa -in ca.key -outform PEM -pubout -out public.pem
kubectl create secret generic prvkey --from-file=privateKey=ca.key -n ordsnamespace

echo "${DB_PWD}" > db-auth
openssl pkeyutl -encrypt -pubin -inkey public.pem -in db-auth -pkeyopt rsa_padding_mode:oaep -pkeyopt rsa_oaep_md:sha256 |base64 > e_db-auth
kubectl create secret generic db-auth-enc --from-file=password=e_db-auth -n ordsnamespace
rm db-auth e_db-auth
```

### Create ordssrvs Resource

1. Create a manifest for ORDS.

    This example assumes APEX is already installed in the database.

    The following additional keys are specified for the pool:
    * `autoUpgradeORDS` - Boolean; when true the ORDS will be installed/upgraded in the database
    * `db.adminUser` - User with privileges to install, upgrade or uninstall ORDS in the database (SYS).
    * `db.adminUser.secret` - Secret containing the password for `db.adminUser` (created in the first step)
    * `db.username` will be used as the ORDS schema in the database during the install/upgrade process (ORDS_PUBLIC_USER).

    ```bash
    echo "
    apiVersion: database.oracle.com/v4
    kind: OrdsSrvs
    metadata:
      name: ords-db
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
            secretName:  db-auth-enc
          db.adminUser: SYS
          db.adminUser.secret:
            secretName:  db-auth-enc
    " > ords-db.yaml

    kubectl apply -f ords-db.yaml
    ```

    <sup>latest container-registry.oracle.com/database/ords version, **25.1.0**, valid as of **26-May-2025**</sup>
    
1. Watch the restdataservices resource until the status is **Healthy**:
    ```bash
    kubectl get ordssrvs ords-sidb -w
    ```

    **NOTE**: If this is the first time pulling the ORDS image, it may take up to 5 minutes.

    You can watch the APEX/ORDS Installation progress by running:

    ```bash
    POD_NAME=$(kubectl get pod -l "app.kubernetes.io/instance=ords-sidb" -o custom-columns=NAME:.metadata.name -n ordsnamespace --no-headers)

    kubectl logs ${POD_NAME} -c ords-sidb-init -n ordsnamespace -f
    ```

### Test

Open a port-forward to the ORDS service, for example:

```bash
kubectl port-forward service/ords-db -n ordsnamespace 8443:8443
```

Direct your browser to: `https://localhost:8443/ords`


## Conclusion

This example has a single database pool, named `default`.  It is set to:

* Automatically restart when the configuration changes: `forceRestart: true`
* Automatically install/update ORDS on startup, if required: `autoUpgradeORDS: true`
* Use a basic connection string to connect to the database: `db.customURL: jdbc:oracle:thin:@//${CONN_STRING}`
* The `passwordKey` has been ommitted from both `db.secret` and `db.adminUser.secret` as the password was stored in the default key (`password`)
