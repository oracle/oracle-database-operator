# OrdsSrvs Controller: Multipool, Multidatabase using a TNS Names file

This example walks through using the **OrdsSrvs controller** with multiple databases using a TNS Names file.
Keep in mind that all pools are running in the same Pod, therefore, changing the configuration of one pool will require
a recycle of all pools.

## Prerequisites

See [OrdsSrvs prerequisites](../README.md#prerequisites).

### Database Requirements

ORDS must already be installed in PDB2, PDB3, and PDB4; this example installs or
upgrades it only in PDB1.

### TNS_ADMIN Secret

Create a Secret with the contents of the TNS_ADMIN directory.  This can be a single `tnsnames.ora` file or additional files such as `sqlnet.ora` or `ldap.ora`.
The example shows using a `$TNS_ADMIN` environment variable which points to a directory with valid TNS_ADMIN files.

To create a secret with all files in the TNS_ADMIN directory:
```bash
kubectl create secret generic multi-tns-admin \
    --from-file="$TNS_ADMIN" \
    -n ordsnamespace
```

To create a secret with just the tnsnames.ora file:
```bash
kubectl create secret generic multi-tns-admin \
    --from-file="$TNS_ADMIN/tnsnames.ora" \
    -n ordsnamespace
```

In this example, 4 PDBs will be connected to and the example `tnsnames.ora` file contents are as below:
```text
PDB1=(DESCRIPTION=(ADDRESS_LIST=(LOAD_BALANCE=on)(ADDRESS=(PROTOCOL=TCP)(HOST=10.10.0.1)(PORT=1521)))(CONNECT_DATA=(SERVICE_NAME=PDB1)))

PDB2=(DESCRIPTION=(ADDRESS_LIST=(LOAD_BALANCE=on)(ADDRESS=(PROTOCOL=TCP)(HOST=10.10.0.2)(PORT=1521)))(CONNECT_DATA=(SERVICE_NAME=PDB2)))

PDB3=(DESCRIPTION=(ADDRESS_LIST=(LOAD_BALANCE=on)(ADDRESS=(PROTOCOL=TCP)(HOST=10.10.0.3)(PORT=1521)))(CONNECT_DATA=(SERVICE_NAME=PDB3)))

PDB4=(DESCRIPTION=(ADDRESS_LIST=(LOAD_BALANCE=on)(ADDRESS=(PROTOCOL=TCP)(HOST=10.10.0.4)(PORT=1521)))(CONNECT_DATA=(SERVICE_NAME=PDB4)))
```

### Credential Secret

Create a Secret for the `ORDS_PUBLIC_USER` and admin user credentials:

```bash
kubectl create secret generic ordssrvs-auth \
  --from-literal=dbAuth='<ords-db-credential>' \
  --from-literal=adminAuth='<database-admin-credential>' \
  -n ordsnamespace
```

All four pools use the same runtime user password from `ordssrvs-auth.dbAuth`.
The `adminAuth` key supplies the SYS password for PDB1.

### Create OrdsSrvs Resource

1. Create an `OrdsSrvs` custom resource, ords-multi-pool.yaml:

    ```yaml
    apiVersion: database.oracle.com/v4
    kind: OrdsSrvs
    metadata:
      name: ords-multi-pool
      namespace: ordsnamespace
    spec:
      image: container-registry.oracle.com/database/ords:<ords-version>
      forceRestart: true
      globalSettings:
        database.api.enabled: true
      poolSettings:
        - poolName: pdb1
          autoUpgradeORDS: true
          db.connectionType: tns
          db.tnsAliasName: PDB1
          tnsAdminSecret:
            secretName: multi-tns-admin
          restEnabledSql.active: true
          feature.sdw: true
          plsql.gateway.mode: proxied
          db.username: ORDS_PUBLIC_USER
          db.secret:
            secretName: ordssrvs-auth
            passwordKey: dbAuth
          db.adminUser: SYS
          db.adminUser.secret:
            secretName: ordssrvs-auth
            passwordKey: adminAuth
        - poolName: pdb2
          db.connectionType: tns
          db.tnsAliasName: PDB2
          tnsAdminSecret:
            secretName:  multi-tns-admin
          restEnabledSql.active: true
          feature.sdw: true
          plsql.gateway.mode: proxied
          db.username: ORDS_PUBLIC_USER
          db.secret:
            secretName: ordssrvs-auth
            passwordKey: dbAuth
        - poolName: pdb3
          db.connectionType: tns
          db.tnsAliasName: PDB3
          tnsAdminSecret:
            secretName: multi-tns-admin
          restEnabledSql.active: true
          feature.sdw: true
          plsql.gateway.mode: proxied
          db.username: ORDS_PUBLIC_USER
          db.secret:
            secretName: ordssrvs-auth
            passwordKey: dbAuth
        - poolName: pdb4
          db.connectionType: tns
          db.tnsAliasName: PDB4
          tnsAdminSecret:
            secretName: multi-tns-admin
          restEnabledSql.active: true
          feature.sdw: true
          plsql.gateway.mode: proxied
          db.username: ORDS_PUBLIC_USER
          db.secret:
            secretName: ordssrvs-auth
            passwordKey: dbAuth
    ```
1. Apply the yaml file:
    ```bash
    kubectl apply -f ords-multi-pool.yaml
    ```

1. Watch the ordssrvs resource until the status is **Healthy**:
    ```bash
    kubectl get OrdsSrvs ords-multi-pool -n ordsnamespace -w
    ```

    APEX installation is not enabled in this example. Image downloads and ORDS
    initialization can take several minutes; inspect the `ordssrvs-init` logs
    if the resource remains in `Preparing`.

### Test

Open a port-forward to the OrdsSrvs Service, for example:

```bash
kubectl port-forward service/ords-multi-pool -n ordsnamespace 8443:8443
```

1. For PDB1, direct your browser to: `https://localhost:8443/ords/pdb1`
1. For PDB2, direct your browser to: `https://localhost:8443/ords/pdb2`
1. For PDB3, direct your browser to: `https://localhost:8443/ords/pdb3`
1. For PDB4, direct your browser to: `https://localhost:8443/ords/pdb4`

> If `standalone.https.host` is configured, use that hostname instead of `localhost`.

## Conclusion

This example has multiple pools, named `pdb1`, `pdb2`, `pdb3`, and `pdb4`.

* They all share the same `tnsAdminSecret` to connect using their individual `db.tnsAliasName`
* They will all automatically restart when the configuration changes: `forceRestart: true`
* Only `pdb1` enables `autoUpgradeORDS`. Both `db.adminUser` and `db.adminUser.secret` must be set on that pool; otherwise auto-upgrade is ignored.
* The `passwordKey` fields identify the credential keys used by `db.secret` and `db.adminUser.secret`
