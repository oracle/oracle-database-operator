# LREST Pod Creation Phases

Starting with Oracle Database Operator 2.3, LREST pod creation provides more verbose status information. The `PHASE` column has been replaced by the `OP` column, which reports the operation currently running during creation. At the code level, a bitmask tracks the creation phases; it is not displayed by `kubectl get`. The information shown during pod creation is intended for debugging. When pod creation completes successfully, the message is **Healthy**.

The following output shows a usable running pod:

```
kubectl get pod -n cdbnamespace
NAME                     READY   STATUS    RESTARTS   AGE
cdb-dev-lrest-rs-lmwzp   1/1     Running   0          38m
kubectl get lrest -n cdbnamespace
NAME      CDB NAME   OP                             MESSAGE   AUTODISCOVER   PDB:CRD   TNS STRING
cdb-dev   DB12       POD/LREST creation completed   Healthy   true           0:0       (DESCRIPTION=(ADDRESS=(PROTOCOL=TCP)(HOST=89.168.27.73)(PORT=1521))(CONNECT_DATA=(SERVER=DEDICATED)(SERVICE_NAME=DBOKE_tst.testoke)))
```

Use `kubectl get --watch` to monitor LREST creation progress.

```text
kubectl get lrest cdb-dev -n cdbnamespace --no-headers  -o jsonpath='{.status.phase}{"\t"}{.status.lreststrstate}{"\t"}{.status.msg}{"\n"}'  --watch

OP                             PHASE bitmask                              MSG: function/debug info
-----------------------------  -----------------------------------------  -------------------
Init start                                                                secrets:init
Init waiting                   [8]|LRSINE|                                Init:Waiting[3]
Init completed                 [4]|LRSINI|                                -----------------
pod creation start             [4]|LRSINI|                                pod:createLRESTInstances
pod creation completed         [20]|LRSINI|LRSPOD|                        pod:createLRESTInstances
svc creation start             [20]|LRSINI|LRSPOD|                        svc:createLRESTSVC
svc creation completed         [20]|LRSINI|LRSPOD|                        svc:createLRESTSVC
pod validation start           [276]|LRSINI|LRSPOD|LRSSVC|                pod:validateLRESTPods2
pod validation waiting         [404]|LRSINI|LRSPOD|LRSPVE|LRSSVC|         pod:validateLRESTPods2
pod validation completed       [340]|LRSINI|LRSPOD|LRSPVD|LRSSVC|         -----------------
POD/LREST creation completed   [342]|LRSCMP|LRSINI|LRSPOD|LRSPVD|LRSSVC|  -----------------
POD/LREST creation completed   [342]|LRSCMP|LRSINI|LRSPOD|LRSPVD|LRSSVC|  Healthy
POD/LREST creation completed   [342]|LRSCMP|LRSINI|LRSPOD|LRSPVD|LRSSVC|  Healthy
```

### Bitask values

| NAME  |    VALUE    | DESCRIPTION  
|-------|-------------|-----------------------
|LRSRBT | 0x00000001  | Rebotting rest server
|LRSCMP | 0x00000002  | Lrest setup complete  
|LRSINI | 0x00000004  | Init phase
|LRSINE | 0x00000008  | Init phase wait
|LRSPOD | 0x00000010  | Pod creation
|LRSPDE | 0x00000020  | Pod creation wait
|LRSPVD | 0x00000040  | Pod validation
|LRSPVE | 0x00000080  | Pod validation wait
|LRSSVC | 0x00000100  | Svc creation
|LRSSVE | 0x00000200  | Svc creation wait

## Liveness and Readiness Probes

Pod readiness and liveness probes use [corev1.HTTPGetAction](https://pkg.go.dev/k8s.io/api/core/v1#HTTPGetAction).

### Behavior

- **Readiness** checks whether the HTTPS listener is active. `HTTPGetAction` does not require HTTPS authentication.
- **Liveness** checks the database connection status. It uses the most recent return code from [OCIPing](https://docs.oracle.com/en/database/oracle/oracle-database/19/lnoci/miscellaneous-functions.html#GUID-033BF96D-D88D-4F18-909A-3AB7C2F6C70F), which the REST server exposes indirectly. If the ping fails, the REST endpoint returns HTTP 500 and Kubernetes marks the liveness probe as failed.

### Non-default parameters

To modify the default probe parameters, add the following section to the LREST YAML manifest. A large initial delay is used instead of a startup probe; avoid using a readiness initial delay of less than 30 seconds.

```yaml
spec:
  livenessProbe:
    initialDelaySeconds: <value>
    periodSeconds: <value>
    timeoutSeconds: <value>
    failureThreshold: <value>
  readinessProbe:
    initialDelaySeconds: <value>
    periodSeconds: <value>
    timeoutSeconds: <value>
    failureThreshold: <value>
```

## Container Database Outage

Each multitenant controller monitors the database connection with **OCIPing**. The REST call exports the returned value to the controllers. A problematic or broken connection can produce different return values; on a connection failure, the resource message shows the ping result. Regardless of that value, the controller sets the **PDBCNE** bit in the bitmask. The following examples show the status patterns for a container database shutdown using `shutdown abort`.

- The following output shows resource status while the container database is running.

```bash
kubectl get pod -n cdbnamespace
NAME                     READY   STATUS    RESTARTS   AGE
cdb-dev-lrest-rs-ttf9w   1/1     Running   0          9m3s
```

```bash
kubectl get lrest -n cdbnamespace
NAME      CDB NAME   OP                             MESSAGE   AUTODISCOVER   PDB:CRD   TNS STRING
cdb-dev   DB12       POD/LREST creation completed   Healthy   true           5:5       (DESCRIPTION=(ADDRES.......
```

```bash
kubectl get lrpdb pdb1  -n pdbnamespace --no-headers  -o jsonpath='{.metadata.name}{" - "}{.status.bitstatstr}{" - "}{.status.msg}{"\n"}'
pdb1 - [579]|PDBCRT|PDBOPN|FNALAZ|APPUSR|  - OCIPing: db connection [OK]:0
```

- **Database shutdown abort**

  The following output shows:

  - Pod readiness is **0/1**.
  - The LREST resource message contains `ORA-3114` or a generic **OCIPing(error)** message.
  - The LRPDB resource has the **PDBCNE** bit and a failure message.

```bash
kubectl get pods -n cdbnamespace
NAME                     READY   STATUS    RESTARTS      AGE
cdb-dev-lrest-rs-ttf9w   0/1     Running   1 (29s ago)   17m
```

```bash
kubectl get lrest -n cdbnamespace --watch
NAME      CDB NAME   OP                             MESSAGE              AUTODISCOVER   PDB:CRD   TNS STRING
cdb-dev   DB12       POD/LREST creation completed   Unhealthy:ORA-3114   true           5:5       (DESCRIPTION=(ADDR.....
```

```bash
kubectl get lrpdb pdb1  -n pdbnamespace --no-headers  -o jsonpath='{.metadata.name}{" - "}{.status.pdbBitMaskStr}{" - "}{.status.msg}{"\n"}'
pdb1 - [2097731]|PDBCRT|PDBOPN|FNALAZ|APPUSR|PDBCNE| - getCDBstate failure
```

- **Database startup**

  The following output shows the resources after they return to normal.

```bash
kubectl get pod -n cdbnamespace
NAME                     READY   STATUS    RESTARTS      AGE
cdb-dev-lrest-rs-ttf9w   1/1     Running   3 (16m ago)   38m
```

```bash
kubectl get lrest -n cdbnamespace
NAME      CDB NAME   OP                             MESSAGE   AUTODISCOVER   PDB:CRD   TNS STRING
cdb-dev   DB12       POD/LREST creation completed   Healthy   true           5:5       (DESCRIPTION=(ADDRESS=(PROTOCOL=TCP)(HOST=89.168.27.73)(PORT=1521))(CONNECT_DATA=(SERVER=DEDICATED)(SERVICE_NAME=DBOKE_tst.testoke)))

```

```bash
kubectl get lrpdb pdb1  -n pdbnamespace --no-headers  -o jsonpath='{.metadata.name}{" - "}{.status.pdbBitMaskStr}{" - "}{.status.msg}{"\n"}'
pdb1 - [579]|PDBCRT|PDBOPN|FNALAZ|APPUSR| - open:[op. completed]
```
