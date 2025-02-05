# AutoUpgrade

Each pool can be configured to automatically install and upgrade the ORDS and/or APEX schemas in the database.
The ORDS and APEX version is based on the ORDS image used for the RestDataServices resource.

For example, in the below manifest:
* `Pool: pdb1` is configured to automatically install/ugrade both ORDS and APEX to version 24.1.0  
* `Pool: pdb2` will not install or upgrade ORDS/APEX

As an additional requirement for `Pool: pdb1`, the `spec.poolSettings.db.adminUser` and `spec.poolSettings.db.adminUser.secret`
must be provided.  If they are not, the `autoUpgrade` specification is ignored.

```yaml
apiVersion: database.oracle.com/v1
kind: OrdsSrvs
metadata:
    name: ordspoc-server
spec:
    image: container-registry.oracle.com/database/ords:24.1.0
    forceRestart: true
    globalSettings:
        database.api.enabled: true
    encPrivKey:
        secretName: prvkey
        passwordKey: privateKey
    poolSettings:
      - poolName: pdb1
        autoUpgradeORDS: true
        autoUpgradeAPEX: true
        db.connectionType: customurl
        db.customURL: jdbc:oracle:thin:@//localhost:1521/PDB1
        db.secret:
            secretName:  pdb1-ords-auth
        db.adminUser: SYS
        db.adminUser.secret:
            secretName:  pdb1-sys-auth-enc
      - poolName: pdb2
        db.connectionType: customurl
        db.customURL: jdbc:oracle:thin:@//localhost:1521/PDB2
        db.secret:
            secretName:  pdb2-ords-auth-enc
```

## Minimum Privileges for Admin User

The `db.adminUser` must have privileges to create users and objects in the database.  For Oracle Autonomous Database (ADB), this could be `ADMIN` while for
non-ADBs this could be `SYS AS SYSDBA`.  When you do not want to use `ADMIN` or `SYS AS SYSDBA` to install, upgrade, validate and uninstall ORDS a script is provided
to create a new user to be used.

1. Download the equivalent version of ORDS to the image you will be using.
1. Extract the software and locate: `scripts/installer/ords_installer_privileges.sql`
1. Using SQLcl or SQL*Plus, connect to the Oracle PDB with SYSDBA privileges.
1. Execute the following script providing the database user:
    ```sql
    @/path/to/installer/ords_installer_privileges.sql privuser
    exit
    ```
