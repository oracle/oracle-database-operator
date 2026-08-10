# OrdsSrvs Controller: Multipool, Multidatabase using a TNS Names file

This example walks through using the **OrdsSrvs controller** with multiple databases using a TNS Names file.
Keep in mind that all pools are running in the same Pod, therefore, changing the configuration of one pool will require
a recycle of all pools.

Before testing this example, please verify the prerequisites : [OrdsSrvs prerequisites](../README.md#prerequisites)


### TNS_ADMIN Secret

Create a Secret with the contents of the TNS_ADMIN directory.  This can be a single `tnsnames.ora` file or additional files such as `sqlnet.ora` or `ldap.ora`.
The example shows using a `$TNS_ADMIN` environment variable which points to a directory with valid TNS_ADMIN files.

To create a secret with all files in the TNS_ADMIN directory:
```bash
kubectl create secret generic multi-tns-admin \
    --from-file=$TNS_ADMIN
```

To create a secret with just the tnsnames.ora file:
```bash
kubectl create secret generic multi-tns-admin \
    --from-file=$TNS_ADMIN/tnsnames.ora
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

### Create OrdsSrvs Resource

1. Create a manifest for ORDS, ords-multi-pool.yaml:

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

    **NOTE**: If this is the first time pulling the ORDS image, it may take up to 5 minutes.  As APEX
    is being installed for the first time by the Operator into PDB1, it will remain in the **Preparing**
    status for an additional 5-10 minutes.

### Test

Open a port-forward to the ORDS service, for example:

```bash
kubectl port-forward service/ords-multi-pool -n ordsnamespace 8443:8443
```

1. For PDB1, direct your browser to: `https://localhost:8443/ords/pdb1`
1. For PDB2, direct your browser to: `https://localhost:8443/ords/pdb2`
1. For PDB3, direct your browser to: `https://localhost:8443/ords/pdb3`
1. For PDB4, direct your browser to: `https://localhost:8443/ords/pdb4`

## Conclusion

This example has multiple pools, named `pdb1`, `pdb2`, `pdb3`, and `pdb4`.

* They all share the same `tnsAdminSecret` to connect using their individual `db.tnsAliasName`
* They will all automatically restart when the configuration changes: `forceRestart: true`
* Only the `pdb1` pool will automatically install/update ORDS on startup, if required: `autoUpgradeORDS: true`
* The `passwordKey` fields identify the credential keys used by `db.secret` and `db.adminUser.secret`
