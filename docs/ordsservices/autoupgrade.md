# ORDS and APEX AutoUpgrade

Each pool can be configured to automatically install and upgrade the ORDS and/or APEX schemas in the database.

## ORDS autoUpgrade

The ORDS version is determined by the ORDS image used for the OrdsSrvs resource.
ORDS schema installation and upgrade can be activated at the pool level:

Set `db.adminUser` and `db.adminUser.secret` on the same pool as `autoUpgradeORDS: true`. If either admin setting is missing, the ORDS auto-upgrade setting is ignored.

This is a partial manifest; merge these settings into your resource:

```yaml
apiVersion: database.oracle.com/v4
kind: OrdsSrvs
metadata:
    name: ordspoc-server
    namespace: NAMESPACE
spec:
    ...
    poolSettings:
      - poolName: pdb1
        autoUpgradeORDS: true
        db.adminUser: SYS
        db.adminUser.secret:
          secretName: <secret-name>
          passwordKey: adminAuth
```

Replace `<secret-name>` with an existing Secret in the resource namespace
containing the SYS password under `adminAuth`.

## APEX autoUpgrade

ORDS image does **not** contain APEX installation files.
You can provide APEX installation files in a dedicated `PersistentVolume` containing a single `apex.zip` file.

You can download `apex.zip` from: [Oracle APEX Downloads](https://www.oracle.com/tools/downloads/apex-downloads/)

Set `db.adminUser` and `db.adminUser.secret` on the same pool as `autoUpgradeAPEX: true`. If either admin setting is missing, the APEX auto-upgrade setting is ignored. The admin account runs the APEX installation or upgrade and must have the required privileges. This example uses `SYS`.

```yaml
apiVersion: database.oracle.com/v4
kind: OrdsSrvs
metadata:
  name: ordssrvs-apexpv
  namespace: NAMESPACE
spec:
  image: ORDSIMG
  globalSettings:
    apex.installation.persistence:
      volumeName: APEXPVNAME
      storageClass: APEXPVSTORAGECLASS
      size: APEXPVSIZE
      accessMode: APEXPVACCESSMODE
  poolSettings:
    - poolName: default
      autoUpgradeAPEX: true
      db.connectionType: customurl
      db.customURL: jdbc:oracle:thin:@//CONNECTSTRING
      db.username: ORDS_PUBLIC_USER
      db.secret:
        secretName: <secret-name>
        passwordKey: dbAuth
      db.adminUser: SYS
      db.adminUser.secret:
        secretName: <secret-name>
        passwordKey: adminAuth
```

Replace `<secret-name>` with an existing Secret in the resource namespace
containing the ORDS runtime user password under `dbAuth` and the SYS password
under `adminAuth`.

The OrdsSrvs controller will create a `PersistentVolumeClaim` (PVC) for the PV and mount it in the pod’s container at `/opt/oracle/apex`.

The volume can be static or dynamic. If the volume is empty, then the init container will wait until it finds `apex.zip` at the mount point.
The init container logs the following message:

``` bash
Missing /opt/oracle/apex/apex.zip, manually copy apex.zip in /opt/oracle/apex on the init container of the pod
```

Copy `apex.zip` into the `ordssrvs-init` container while the init script is waiting:

```bash
kubectl cp /tmp/apex.zip ORDS_POD:/tmp -c ordssrvs-init -n NAMESPACE
kubectl exec -c ordssrvs-init -n NAMESPACE ORDS_POD -- mv /tmp/apex.zip /opt/oracle/apex
```
