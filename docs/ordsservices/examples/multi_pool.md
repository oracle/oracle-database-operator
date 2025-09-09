# Example: Multipool, Multidatabase using a TNS Names file

This example walks through using the **ORDSSRVS Operator** with multiple databases using a TNS Names file.  
Keep in mind that all pools are running in the same Pod, therefore, changing the configuration of one pool will require
a recycle of all pools.

Before testing this example, please verify the prerequisites : [ORDSSRVS prerequisites](../README.md#prerequisites)


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

### PRIVATE KEY SECRET

Secrets are encrypted using openssl rsa algorithm. Create public and private key. 
Use private key to create a secret.

```bash
openssl  genpkey -algorithm RSA  -pkeyopt rsa_keygen_bits:2048 -pkeyopt rsa_keygen_pubexp:65537 > ca.key
openssl rsa -in ca.key -outform PEM  -pubout -out public.pem
kubectl create secret generic prvkey --from-file=privateKey=ca.key  -n ordsnamespace 
```

### ORDS_PUBLIC_USER Secret

Create a Secret for each of the databases `ORDS_PUBLIC_USER` user.  
If multiple databases use the same password, the same secret can be re-used.

The following secret will be used for PDB1:

```bash
echo "THIS_IS_A_PASSWORD"     > ordspwdfile
openssl pkeyutl -encrypt -pubin -inkey public.pem -in ordspwdfile -pkeyopt rsa_padding_mode:oaep -pkeyopt rsa_oaep_md:sha256 |base64 > e_ordspwdfile
kubectl create secret generic pdb1-ords-auth-enc --from-file=password=e_ordspwdfile -n  ordsnamespace 
rm ordspwdfile e_ordspwdfile
```

The following secret will be used for PDB2:

```bash
echo "THIS_IS_A_PASSWORD"     > ordspwdfile
openssl pkeyutl -encrypt -pubin -inkey public.pem -in ordspwdfile -pkeyopt rsa_padding_mode:oaep -pkeyopt rsa_oaep_md:sha256 |base64 > e_ordspwdfile
kubectl create secret generic pdb2-ords-auth-enc --from-file=password=e_ordspwdfile -n  ordsnamespace 
rm ordspwdfile e_ordspwdfile
```

The following secret will be used for PDB3 and PDB4:

```bash
echo "THIS_IS_A_PASSWORD"     > ordspwdfile
openssl pkeyutl -encrypt -pubin -inkey public.pem -in ordspwdfile -pkeyopt rsa_padding_mode:oaep -pkeyopt rsa_oaep_md:sha256 |base64 > e_ordspwdfile
kubectl create secret generic multi-ords-auth-enc --from-file=password=e_ordspwdfile -n  ordsnamespace 
rm ordspwdfile e_ordspwdfile
```

### Privileged Secret (*Optional)

If taking advantage of the [AutoUpgrade](../autoupgrade.md) functionality, create a secret for a user with the privileges to modify the ORDS and/or APEX schemas.

In this example, only PDB1 will be set for [AutoUpgrade](../autoupgrade.md), the other PDBs already have APEX and ORDS installed.

```bash
echo "THIS_IS_A_PASSWORD"     > syspwdfile
openssl pkeyutl -encrypt -pubin -inkey public.pem -in syspwdfile -pkeyopt rsa_padding_mode:oaep -pkeyopt rsa_oaep_md:sha256 |base64 > e_syspwdfile
kubectl create secret generic pdb1-priv-auth-enc --from-file=password=e_syspwdfile -n  ordsnamespace
rm syspwdfile e_syspwdfile
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
      image: container-registry.oracle.com/database/ords:25.1.0
      forceRestart: true
      encPrivKey:
        secretName: prvkey
        passwordKey: privateKey
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
            secretName: pdb1-ords-auth-enc
          db.adminUser: SYS
          db.adminUser.secret:
            secretName: pdb1-priv-auth-enc
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
            secretName: pdb2-ords-auth-enc
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
            secretName: multi-ords-auth-enc
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
            secretName: multi-ords-auth-enc
    ```
    <sup>latest container-registry.oracle.com/database/ords version, **25.1.0**, valid as of **26-May-2025**</sup>


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

* They all share the same `tnsAdminSecret` to connect using thier individual `db.tnsAliasName`
* They will all automatically restart when the configuration changes: `forceRestart: true`
* Only the `pdb1` pool will automatically install/update ORDS on startup, if required: `autoUpgradeORDS: true`
* The `passwordKey` has been ommitted from both `db.secret` and `db.adminUser.secret` as the password was stored in the default key (`password`)
