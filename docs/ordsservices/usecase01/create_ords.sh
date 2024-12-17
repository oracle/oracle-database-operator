#!/bin/bash

set -x

TNS_ALIAS=`kubectl get singleinstancedatabase oraoper-sidb  -n oracle-database-operator-system -o jsonpath='{.status.pdbConnectString}'`

echo "
apiVersion: database.oracle.com/v1alpha1
kind: RestDataServices
metadata:
  name: ords-sidb
  namespace: oracle-database-operator-system
spec:
  image: container-registry.oracle.com/database/ords:24.1.0
  forceRestart: true
  globalSettings:
    database.api.enabled: true
  poolSettings:
    - poolName: default
      autoUpgradeORDS: true
      autoUpgradeAPEX: true
      restEnabledSql.active: true
      plsql.gateway.mode: direct
      db.connectionType: customurl
      db.customURL: jdbc:oracle:thin:@//${TNS_ALIAS}
      db.username: ORDS_PUBLIC_USER
      db.secret:
        secretName:  sidb-db-auth
      db.adminUser: SYS
      db.adminUser.secret:
        secretName:  sidb-db-auth" | kubectl apply -f -
