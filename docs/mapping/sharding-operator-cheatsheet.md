# Sharding Operator Cheat Sheet

Quick, validated patterns for `ShardingDatabase` using mapped runtime parameters.

## 1) System Sharding + DG

Notes:
- `spec.shardingType: SYSTEM`
- `spec.replicationType: DG`
- shard uses `shardGroup` (not `shardSpace`)
- in system add-shard flow, avoid setting `deployAs` on `spec.shard[]` with `shardGroup`

```yaml
apiVersion: database.oracle.com/v4
kind: ShardingDatabase
metadata:
  name: sdb-system-dg
  namespace: shns
spec:
  storageClass: oci
  dbImage: <db-image>
  gsmImage: <gsm-image>
  shardingType: SYSTEM
  replicationType: DG
  dbSecret:
    name: db-user-pass-rsa
    pwdFileName: pwdfile.enc
    tdePwdFileName: tdepwdfile.enc
    keyFileName: key.pem
  catalog:
    - name: catalog
      storageSizeInGb: 50
      autoVncr: "ON"
  gsm:
    - name: gsm1
      storageSizeInGb: 50
      region: primary
  shardGroup:
    - name: pshg1
      region: primary
      deployAs: PRIMARY
    - name: sshg1
      region: standby
      deployAs: STANDBY
  shard:
    - name: pshard1
      storageSizeInGb: 50
      shardGroup: pshg1
      shardRegion: primary
    - name: pshard2
      storageSizeInGb: 50
      shardGroup: sshg1
      shardRegion: standby
```

## 2) User Sharding + DG

Notes:
- `spec.shardingType: USER`
- shard requires `shardSpace`
- no `shardGroup` in user flow
- `deployAs` allowed in user add-shard DG flow

```yaml
apiVersion: database.oracle.com/v4
kind: ShardingDatabase
metadata:
  name: sdb-user-dg
  namespace: shns
spec:
  storageClass: oci
  dbImage: <db-image>
  gsmImage: <gsm-image>
  shardingType: USER
  replicationType: DG
  dbSecret:
    name: db-user-pass-rsa
    pwdFileName: pwdfile.enc
    tdePwdFileName: tdepwdfile.enc
    keyFileName: key.pem
  catalog:
    - name: catalog
      storageSizeInGb: 50
  gsm:
    - name: gsm1
      storageSizeInGb: 50
      region: primary
  shardSpace:
    - name: usp1
      chunks: 12
      protectMode: MAXAVAILABILITY
  shard:
    - name: ushard1
      storageSizeInGb: 50
      shardSpace: usp1
      shardRegion: primary
      deployAs: PRIMARY
```

## 3) Composite Sharding + DG

Notes:
- `spec.shardingType: COMPOSITE`
- shard requires both `shardGroup` and `shardSpace`
- do not set `deployAs` in `spec.shard[]` when using `shardGroup`

```yaml
apiVersion: database.oracle.com/v4
kind: ShardingDatabase
metadata:
  name: sdb-composite-dg
  namespace: shns
spec:
  storageClass: oci
  dbImage: <db-image>
  gsmImage: <gsm-image>
  shardingType: COMPOSITE
  replicationType: DG
  dbSecret:
    name: db-user-pass-rsa
    pwdFileName: pwdfile.enc
    tdePwdFileName: tdepwdfile.enc
    keyFileName: key.pem
  catalog:
    - name: catalog
      storageSizeInGb: 50
  gsm:
    - name: gsm1
      storageSizeInGb: 50
      region: primary
  shardSpace:
    - name: csp1
      chunks: 24
      protectMode: MAXPERFORMANCE
  shardGroup:
    - name: cgrp1
      region: primary
      shardSpace: csp1
      deployAs: PRIMARY
  shard:
    - name: cshard1
      storageSizeInGb: 50
      shardGroup: cgrp1
      shardSpace: csp1
      shardRegion: primary
```

## 4) User Sharding + Native (Raft aliases accepted)

Notes:
- `replicationType` can be `NATIVE`, `RAFT`, `RAFTREPLICATION`, `RAFTREPLICATIN` (normalized)
- `deployAs` is not supported for native
- native-specific shard params are allowed (`cloneSchemas`, `ggService`, `replace`)

```yaml
apiVersion: database.oracle.com/v4
kind: ShardingDatabase
metadata:
  name: sdb-user-native
  namespace: shns
spec:
  storageClass: oci
  dbImage: <db-image>
  gsmImage: <gsm-image>
  shardingType: USER
  replicationType: RAFTREPLICATION
  dbSecret:
    name: db-user-pass-rsa
    pwdFileName: pwdfile.enc
    tdePwdFileName: tdepwdfile.enc
    keyFileName: key.pem
  catalog:
    - name: catalog
      storageSizeInGb: 50
      repl: NATIVE
      repFactor: 3
      repUnits: 3
      multiwriter: true
  gsm:
    - name: gsm1
      storageSizeInGb: 50
      region: primary
  shardSpace:
    - name: nsp1
      chunks: 16
      repFactor: 3
      repUnits: 3
  shard:
    - name: nshard1
      storageSizeInGb: 50
      shardSpace: nsp1
      shardRegion: primary
      cloneSchemas: true
      ggService: "https://ggadmin.example.com"
      replace: oldnshard1
```

## 5) Existing Catalog Reuse

Notes:
- if `useExistingCatalog: true`, webhook requires `catalogDatabaseRef`

```yaml
apiVersion: database.oracle.com/v4
kind: ShardingDatabase
metadata:
  name: sdb-existing-catalog
  namespace: shns
spec:
  storageClass: oci
  dbImage: <db-image>
  gsmImage: <gsm-image>
  shardingType: SYSTEM
  replicationType: DG
  dbSecret:
    name: db-user-pass-rsa
    pwdFileName: pwdfile.enc
    tdePwdFileName: tdepwdfile.enc
    keyFileName: key.pem
  catalog:
    - name: catalog
      useExistingCatalog: true
      catalogDatabaseRef:
        host: catalog-db.example.com
        port: 1521
        cdbName: CATALOG
        pdbName: CATALOGPDB
  gsm:
    - name: gsm1
      storageSizeInGb: 50
      region: primary
```

## Apply

```bash
kubectl apply -f <your-file>.yaml
```

## Fast Validation Checklist

- `SYSTEM`: shard has `shardGroup`, no `shardSpace`.
- `USER`: shard has `shardSpace`, no `shardGroup`.
- `COMPOSITE`: shard has both `shardGroup` and `shardSpace`.
- Native replication: do not use `deployAs`.
- `useExistingCatalog: true` requires `catalogDatabaseRef`.
