# ORDS and APEX AutoUpgrade

Each pool can be configured to automatically install and upgrade the ORDS and/or APEX schemas in the database.  

## ORDS autoUpgrade

The ORDS version is determined by the ORDS image used for the RestDataServices resource.
ORDS schema installation and upgrade can be activated at the pool level:  

```yaml
apiVersion: database.oracle.com/v1
kind: OrdsSrvs
metadata:
    name: ordspoc-server
spec:
    ...
    poolSettings:
      - poolName: pdb1
        autoUpgradeORDS: true
```

## APEX autoUpgrade

ORDS image does **not** contain APEX installation files.  
APEX installation files can be provided to the pod in two ways:  

 - automatic download
 - external storage (PersistenceVolume)


### APEX installation automatic download

The ORDS container can download the latest APEX version either from "Oracle APEX Downloads" or a specified custom URL.  
To download APEX installation files, the Kubernetes worker node must have internet access.  
The APEX download is defined globally, and upgrades can be enabled or disabled for each pool individually.

```yaml
apiVersion: database.oracle.com/v1
kind: OrdsSrvs
metadata:
    name: ordspoc-server
spec:
    ...
    globalSettings:
        downloadAPEX : true
        downloadUrlAPEX : https://download.oracle.com/otn_software/apex/apex_24.2.zip
    encPrivKey:
      ...
    poolSettings:
      - poolName: pdb1
        autoUpgradeAPEX: true
        ...
      - poolName: pdb2
        autoUpgradeAPEX: false
        ...
```

If you do not specify a download URL (downloadUrlAPEX), the default value is used:
https://download.oracle.com/otn_software/apex/apex-latest.zip


### APEX installation files on external storage

Alternatively, you can provide APEX installation files in a dedicated PersistentVolume containing a single apex.zip file.  

You can download apex.zip from:  
https://www.oracle.com/tools/downloads/apex-downloads/

```yaml
apiVersion: database.oracle.com/v4
kind: OrdsSrvs
metadata:
  name: ordssrvs
  namespace: testcase
spec:
  ...
  globalSettings:
    apex.download : false
    apex.installation.persistence:
      volumeName : apexpv
      storageClass :
      size : 20Gi
      accessMode : ReadWriteMany
    ...  
  poolSettings:
    - poolName: default
      autoUpgradeAPEX: true
```

The OrdsSrvs controller will create a PersistentVolumeClaim (PVC) for the PV and mount it in the pod’s container at /opt/oracle/apex.

The volume can be static or dynamic. If the volume is empty, the init container will wait until it finds apex.zip at the mount point.   
The init container logs the following message:  

``` bash
Missing /opt/oracle/apex/apex.zip, manually copy apex.zip in /opt/oracle/apex on the init container of the pod
```

You can copy the apex.zip file into the container while the init script is waiting:  

``` bash
kubectl cp /tmp/apex.zip <ordspod>:/tmp -c ordssrvs-init -n ordsnamespace
kubectl exec -c ordssrvs-init -n ordsnamespace <ordspod> -- mv /tmp/apex.zip /opt/oracle/apex
```



## Example: ORDS autoUpgrade and APEX download/autoUpgrade

In the following manifest example:  

* APEX installation files will be downloaded from latest version.
* `Pool: pdb1` is configured to automatically install/ugrade both ORDS and APEX to version 25.1.0  
* `Pool: pdb2` will install or upgrade ORDS
* `Pool: pdb2` will not install or upgrade ORDS/APEX

As an additional requirement for `Pool: pdb1`, the `spec.poolSettings.db.adminUser` and `spec.poolSettings.db.adminUser.secret`
must be provided.  If they are not, the `autoUpgrade` specification is ignored.

```yaml
apiVersion: database.oracle.com/v1
kind: OrdsSrvs
metadata:
    name: ordspoc-server
spec:
    image: container-registry.oracle.com/database/ords:25.1.0
    forceRestart: true
    globalSettings:
        database.api.enabled: true
        downloadAPEX : true
        downloadUrlAPEX : https://download.oracle.com/otn_software/apex/apex_24.2.zip
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
        autoUpgradeORDS: true
        db.connectionType: customurl
        db.customURL: jdbc:oracle:thin:@//localhost:1521/PDB2
        db.secret:
            secretName:  pdb2-ords-auth-enc
      - poolName: pdb3
        db.connectionType: customurl
        db.customURL: jdbc:oracle:thin:@//localhost:1521/PDB3
        db.secret:
            secretName:  pdb3-ords-auth-enc
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
