# Example: Multipool, Multidatabase using a TNS Names file

This example walks through using the **ORDS Operator** with multiple databases using a TNS Names file.  
Keep in mind that all pools are running in the same Pod, therefore, changing the configuration of one pool will require
a recycle of all pools.

### Install ORDS Operator

Install the Oracle ORDS Operator:

```bash
kubectl apply -f https://github.com/gotsysdba/oracle-ords-operator/releases/latest/download/oracle-ords-operator.yaml
```

### TNS_ADMIN Secret

Create a Secret with the contents of the TNS_ADMIN directory.  This can be a single `tnsnames.ora` file or additional files such as `sqlnet.ora` or `ldap.ora`.
The example shows using a `$TNS_ADMIN` enviroment variable which points to a directory with valid TNS_ADMIN files.

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

### ORDS_PUBLIC_USER Secret

Create a Secret for each of the databases `ORDS_PUBLIC_USER` user.  
If multiple databases use the same password, the same secret can be re-used.

The following secret will be used for PDB1:
```bash
kubectl create secret generic pdb1-ords-auth \
  --from-literal=password=pdb1-battery-staple
```

The following secret will be used for PDB2:
```bash
kubectl create secret generic pdb2-ords-auth \
  --from-literal=password=pdb2-battery-staple
```

The following secret will be used for PDB3 and PDB4:
```bash
kubectl create secret generic multi-ords-auth \
  --from-literal=password=multiple-battery-staple
```

### Privileged Secret (*Optional)

If taking advantage of the [AutoUpgrade](../autoupgrade.md) functionality, create a secret for a user with the privileges to modify the ORDS and/or APEX schemas.

In this example, only PDB1 will be set for [AutoUpgrade](../autoupgrade.md), the other PDBs already have APEX and ORDS installed.

```bash
kubectl create secret generic pdb1-priv-auth \
  --from-literal=password=pdb1-battery-staple
```

### Create RestDataServices Resource

1. Create a manifest for ORDS.

    ```bash
    echo "
    apiVersion: database.oracle.com/v1
    kind: RestDataServices
    metadata:
      name: ords-multi-pool
    spec:
      image: container-registry.oracle.com/database/ords:24.1.1
      forceRestart: true
      globalSettings:
        database.api.enabled: true
      poolSettings:
        - poolName: pdb1
          autoUpgradeORDS: true
          autoUpgradeAPEX: true
          db.connectionType: tns
          db.tnsAliasName: PDB1
          tnsAdminSecret:
            secretName: multi-tns-admin
          restEnabledSql.active: true
          feature.sdw: true
          plsql.gateway.mode: proxied
          db.username: ORDS_PUBLIC_USER
          db.secret:
            secretName: pdb1-ords-auth
          db.adminUser: SYS
          db.adminUser.secret:
            secretName: pdb1-priv-auth
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
            secretName: pdb2-ords-auth
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
            secretName: multi-ords-auth
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
            secretName: multi-ords-auth" | kubectl apply -f -
    ```
    <sup>latest container-registry.oracle.com/database/ords version, **24.1.1**, valid as of **30-May-2024**</sup>
    
1. Watch the restdataservices resource until the status is **Healthy**:
    ```bash
    kubectl get restdataservices ords-multi-pool -w
    ```

    **NOTE**: If this is the first time pulling the ORDS image, it may take up to 5 minutes.  As APEX
    is being installed for the first time by the Operator into PDB1, it will remain in the **Preparing** 
    status for an additional 5-10 minutes.

### Test

Open a port-forward to the ORDS service, for example:

```bash
kubectl port-forward service/ords-multi-pool 8443:8443
```

1. For PDB1, direct your browser to: `https://localhost:8443/ords/pdb1`
1. For PDB2, direct your browser to: `https://localhost:8443/ords/pdb2`
1. For PDB3, direct your browser to: `https://localhost:8443/ords/pdb3`
1. For PDB4, direct your browser to: `https://localhost:8443/ords/pdb4`

## Conclusion

This example has multiple pools, named `pdb1`, `pdb2`, `pdb3`, and `pdb4`.

* They all share the same `tnsAdminSecret` to connect using thier individual `db.tnsAliasName`
* They will all automatically restart when the configuration changes: `forceRestart: true`
* Only the `pdb1` pool will automatically install/update ORDS on startup, if required: `autoUpgradeORDS: true`
* Only the `pdb1` pool will automatically install/update APEX on startup, if required: `autoUpgradeAPEX: true`
* The `passwordKey` has been ommitted from both `db.secret` and `db.adminUser.secret` as the password was stored in the default key (`password`)
