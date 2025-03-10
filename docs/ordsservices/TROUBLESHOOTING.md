
<span style="font-family:Liberation mono; font-size:0.8em; line-height: 1.2em">

## TROUBLESHOOTING 

### Init container error 

Check the pod status and verify the init outcome 

----
*Command:*
```bash
kubectl get pods -n <namespace>
```

*Example:*
```bash
kubectl get pods -n ordsnamespace
NAME                               READY   STATUS                  RESTARTS      AGE
ords-multi-pool-55db776994-7rrff   0/1     Init:CrashLoopBackOff   6 (61s ago)   12m
```
In case of error identify the *initContainer* name 

----
*Command:*
```bash
kubectl get pod -n <namespace> -o="custom-columns=NAME:.metadata.name,INIT-CONTAINERS:.spec.initContainers[*].name,CONTAINERS:.spec.containers[*].name"
```

Use the initContainers info to dump log information 
**Command:**
```bash 
kubectl logs -f --since=0 <podname> -n <namespace> -c <podinit>
```

*Example:*

In this particular case we are providing wrong credential: "SYT" user does not exist

```text
kubectl logs -f --since=0 ords-multi-pool-55db776994-m7782 -n ordsnamespace -c ords-multi-pool-init

[..omissis...]
Running SQL...
Picked up JAVA_TOOL_OPTIONS: -Doracle.ml.version_check=false
BACKTRACE [24:09:17 08:59:03]

filename:line   function
-------------   --------
/opt/oracle/sa/bin/init_script.sh:115   run_sql
/opt/oracle/sa/bin/init_script.sh:143   check_adb
/opt/oracle/sa/bin/init_script.sh:401   main
SQLERROR:
  USER          = SYT
  URL           = jdbc:oracle:thin:@PDB2
  Error Message = 🔥ORA-01017: invalid username/password;🔥 logon denied
Pool: pdb2, Exit Code: 1
Pool: pdb1, Exit Code: 1
```

---
*Diag shell* Use the following script to dump the container init log

```bash
#!/bin/bash
NAMESPACE=${1:-"ordsnamespace"}
KUBECTL=/usr/bin/kubectl
for _pod in `${KUBECTL} get pods  --no-headers -o custom-columns=":metadata.name" --no-headers -n ${NAMESPACE}`
do
        for _podinit in   `${KUBECTL} get pod ${_pod} -n ${NAMESPACE} -o="custom-columns=INIT-CONTAINERS:.spec.initContainers[*].name" --no-headers`
        do
        echo "DUMPINIT ${_pod}:${_podinit}"
        ${KUBECTL} logs -f --since=0 ${_pod} -n ${NAMESPACE} -c ${_podinit}
        done
done
```

## Ords init error 

Get pod name

*Command:*
```bash
kubectl get pods -n <namespace>
```

*Example:*
```
kubectl get pods -n ordsnamespace
NAME                               READY   STATUS    RESTARTS   AGE
ords-multi-pool-55db776994-m7782   1/1     Running   0          2m51s
```
----
Dump ords log

*Commands:*
```bash
kubectl logs --since=0 <podname> -n <namespace>
```
*Example:*
```text
kubectl logs --since=0 ords-multi-pool-55db776994-m7782  -n ordsnamespace
[..omissis..]
2024-09-17T09:47:39.227Z WARNING     The pool named: |pdb2|lo| is invalid and will be ignored: ORDS was unable to make a connection to the database. The database user specified by db.username configuration setting is locked. The connection pool named: |pdb2|lo| had the following error(s): 🔥ORA-28000: The account is locked.🔥

2024-09-17T09:47:39.370Z WARNING     The pool named: |pdb1|lo| is invalid and will be ignored: ORDS was unable to make a connection to the database. The database user specified by db.username configuration setting is locked. The connection pool named: |pdb1|lo| had the following error(s): 🔥ORA-28000: The account is locked.🔥

2024-09-17T09:47:39.375Z INFO

Mapped local pools from /opt/oracle/sa/config/databases:
  /ords/pdb1/                         => pdb1                           => INVALID
  /ords/pdb2/                         => pdb2                           => INVALID


2024-09-17T09:47:39.420Z INFO        Oracle REST Data Services initialized
Oracle REST Data Services version : 24.1.1.r1201228
Oracle REST Data Services server info: jetty/10.0.20
Oracle REST Data Services java info: Java HotSpot(TM) 64-Bit Server VM 11.0.15+8-LTS-149
```

*Solution:* Connect to the container db to unlock the account

```sql
alter user ORDS_PUBLIC_USER account unlock;
```


<span/>

