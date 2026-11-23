# Update an Existing SIDB Configuration

Use these examples to update an existing SingleInstanceDatabase (SIDB) and
verify the result in the resource, Pod, and database. The memory examples use
explicit SGA and PGA targets. For memory-management choices and HugePages
prerequisites, see [Database Pod Resources and Init Parameters](../README.md#database-pod-resources-and-init-parameters).

1. [Update the SIDB CPU Count](#update-the-sidb-cpu-count)
2. [Update the SIDB Processes Parameter](#update-the-sidb-processes-parameter)
3. [Update Pod Memory, SGA, and PGA Without HugePages](#update-pod-memory-sga-and-pga-without-hugepages)
4. [Configure Automatic Memory Management](#configure-automatic-memory-management)
5. [Retain and Reuse Database Storage](#retain-and-reuse-database-storage)
6. [Expand Existing ORADATA and FRA PVCs](#expand-existing-oradata-and-fra-pvcs)
7. [Add an Additional PVC](#add-an-additional-pvc)
8. [Add and Remove TNS Aliases](#add-and-remove-tns-aliases)

## Update the SIDB CPU Count

### 1. Set the deployment details

Replace the namespace placeholder and confirm that the database Pod is Ready.
These commands assume one database Pod and a container named after the SIDB.

```sh
NS='<namespace-for-sidb-deployment>'
SIDB=sidb-sample

kubectl get pods -n "$NS" -l "app=$SIDB"
```

### 2. Check the configured CPU count

```sh
kubectl get sidb "$SIDB" -n "$NS" \
  -o jsonpath='cpuCount={.spec.initParams.cpuCount}{"\n"}'
```

A blank value means the field is not explicitly set.

### 3. Check the database CPU count

```sh
POD=$(kubectl get pods -n "$NS" -l "app=$SIDB" \
  -o jsonpath='{.items[0].metadata.name}')

kubectl exec -i "$POD" -n "$NS" -c "$SIDB" -- \
  bash -lc 'sqlplus -s "/ as sysdba"' <<'SQL'
WHENEVER SQLERROR EXIT SQL.SQLCODE
SET LINESIZE 160
COLUMN name FORMAT A24
COLUMN value FORMAT A30
SELECT name, display_value AS value
FROM   v$parameter
WHERE  name = 'cpu_count';
EXIT
SQL
```

### 4. Patch the CPU count to a new value (For example: 8 in this case)

```sh
kubectl patch sidb "$SIDB" -n "$NS" \
  --type=merge \
  --patch '{"spec":{"initParams":{"cpuCount":8}}}'
```

### 5. Verify the change

Allow the operator to reconcile, then check SIDB health and the configured value:

```sh
kubectl wait -n "$NS" \
  --for=jsonpath='{.status.status}'=Healthy \
  "singleinstancedatabase/$SIDB" --timeout=10m

kubectl get sidb "$SIDB" -n "$NS" \
  -o jsonpath='cpuCount={.spec.initParams.cpuCount}{"\n"}'

kubectl get pods -n "$NS" -l "app=$SIDB"
```

Confirm `cpuCount=8`. Once the Pod is Ready, repeat **step 3**, including the
Pod lookup, and confirm that the database reports `cpu_count = 8`.

Set `spec.initParams.cpuCount: 8` in your maintained SIDB manifest as well.

## Update the SIDB Processes Parameter

This example changes `spec.initParams.processes` to a new value (In this example: `1000`).
The change requires a database restart, so perform it during a
maintenance window.

### 1. Set the deployment details

Replace the namespace placeholder and confirm that the database Pod is Ready.
These commands assume one database Pod and a container named after the SIDB.

```sh
NS='<namespace-for-sidb-deployment>'
SIDB=sidb-sample

kubectl get pods -n "$NS" -l "app=$SIDB"
```

### 2. Check the configured processes value

```sh
kubectl get sidb "$SIDB" -n "$NS" \
  -o jsonpath='processes={.spec.initParams.processes}{"\n"}'
```

A blank value means the field is not explicitly set in the SIDB resource.

### 3. Check the database processes value

```sh
POD=$(kubectl get pods -n "$NS" -l "app=$SIDB" \
  -o jsonpath='{.items[0].metadata.name}')

kubectl exec -i "$POD" -n "$NS" -c "$SIDB" -- \
  bash -lc 'sqlplus -s "/ as sysdba"' <<'SQL'
WHENEVER SQLERROR EXIT SQL.SQLCODE
SET LINESIZE 160
COLUMN name FORMAT A24
COLUMN value FORMAT A30
SELECT name, display_value AS value
FROM   v$parameter
WHERE  name = 'processes';
EXIT
SQL
```

### 4. Patch processes to 1000

```sh
kubectl patch sidb "$SIDB" -n "$NS" \
  --type=merge \
  --patch '{"spec":{"initParams":{"processes":1000}}}'
```

### 5. Verify the change

Allow the operator to reconcile, then check SIDB health and the configured value:

```sh
kubectl wait -n "$NS" \
  --for=jsonpath='{.status.status}'=Healthy \
  "singleinstancedatabase/$SIDB" --timeout=10m

kubectl get sidb "$SIDB" -n "$NS" \
  -o jsonpath='processes={.spec.initParams.processes}{"\n"}'

kubectl get pods -n "$NS" -l "app=$SIDB"
```

Expected configured value:

```text
processes=1000
```

Once the Pod is Ready, repeat **step 3**, including the Pod lookup,
and confirm that the database reports:

```text
NAME                     VALUE
------------------------ ------------------------------
processes                1000
```

The SIDB may still report `Healthy` before reconciliation completes.
The update is complete only when the database query returns `1000`.
If it still returns the previous value, inspect SIDB events and operator
logs to determine whether reconciliation or the required database restart
is pending.

Set `spec.initParams.processes: 1000` in your maintained SIDB manifest as well.

## Update Pod Memory, SGA, and PGA Without HugePages

This example sets the database container's memory request and limit to
`32Gi`, the SGA target to 24GiB, and the PGA aggregate target to 6GiB.

Use this procedure for an SIDB that does not configure HugePages or
`spec.initParams.memoryTarget`. Perform the change during a maintenance
window because reconciliation can replace the Pod or restart the database.

### 1. Set the deployment details

Replace the namespace placeholder and confirm that the database Pod is Ready.
These commands assume one database Pod and a container named after the SIDB.

```sh
NS='<namespace-for-sidb-deployment>'
SIDB=sidb-sample

kubectl get pods -n "$NS" -l "app=$SIDB"
```

### 2. Check the current memory configuration

Inspect the SIDB resource:

```sh
kubectl get sidb "$SIDB" -n "$NS" \
  -o jsonpath='resources={.spec.resources}{"\n"}initParams={.spec.initParams}{"\n"}'
```

Confirm that no `hugepages-*` resource is configured under requests or limits
and that `spec.initParams.memoryTarget` is unset. This procedure does not
convert an existing HugePages or AMM deployment.

Check the current Pod's memory requests and limits:

```sh
POD=$(kubectl get pods -n "$NS" -l "app=$SIDB" \
  -o jsonpath='{.items[0].metadata.name}')

kubectl get pod "$POD" -n "$NS" \
  -o jsonpath='{range .spec.containers[*]}{.name}{" request="}{.resources.requests.memory}{" limit="}{.resources.limits.memory}{"\n"}{end}'
```

For a container without these fields set, the output is:

```text
sidb-sample request= limit=
```

### 3. Check the database memory parameters

```sh
POD=$(kubectl get pods -n "$NS" -l "app=$SIDB" \
  -o jsonpath='{.items[0].metadata.name}')

kubectl exec -i "$POD" -n "$NS" -c "$SIDB" -- \
  bash -lc 'sqlplus -s "/ as sysdba"' <<'SQL'
WHENEVER SQLERROR EXIT SQL.SQLCODE
SET LINESIZE 160
COLUMN name FORMAT A24
COLUMN value FORMAT A30
SELECT name, display_value AS value
FROM   v$parameter
WHERE  name IN ('sga_target', 'pga_aggregate_target', 'memory_target')
ORDER BY name;
EXIT
SQL
```

Confirm that `memory_target` is `0` before proceeding.

### 4. Patch the memory request, limit, and SGA/PGA targets

The SGA and PGA fields use integer values in MiB:

- `sgaTarget: 24576` represents 24GiB.
- `pgaAggregateTarget: 6144` represents 6GiB.

Without HugePages, the operator requires regular memory of at least
`24GiB + 6GiB + 1GiB = 31GiB`. The proposed `32Gi` limit satisfies this
requirement and the 4GiB minimum. These are validation thresholds;
production sizing must also account for the workload.

Apply all values in one patch:

```sh
kubectl patch sidb "$SIDB" -n "$NS" \
  --type=merge \
  --patch '{
    "spec": {
      "resources": {
        "requests": {
          "memory": "32Gi"
        },
        "limits": {
          "memory": "32Gi"
        }
      },
      "initParams": {
        "sgaTarget": 24576,
        "pgaAggregateTarget": 6144
      }
    }
  }'
```

### 5. Verify the change

Check the values stored in the SIDB resource:

```sh
kubectl get sidb "$SIDB" -n "$NS" \
  -o jsonpath='memoryRequest={.spec.resources.requests.memory}{"\n"}memoryLimit={.spec.resources.limits.memory}{"\n"}sgaTarget={.spec.initParams.sgaTarget}{"\n"}pgaAggregateTarget={.spec.initParams.pgaAggregateTarget}{"\n"}'
```

Expected output:

```text
memoryRequest=32Gi
memoryLimit=32Gi
sgaTarget=24576
pgaAggregateTarget=6144
```

Watch the Pod while the operator reconciles:

```sh
kubectl get pods -n "$NS" -l "app=$SIDB" -w
```

After the updated Pod is Ready, press Ctrl+C and check SIDB health:

```sh
kubectl wait -n "$NS" \
  --for=jsonpath='{.status.status}'=Healthy \
  "singleinstancedatabase/$SIDB" --timeout=30m
```

Retrieve the current Pod name again because reconciliation may have replaced it:

```sh
POD=$(kubectl get pods -n "$NS" -l "app=$SIDB" \
  -o jsonpath='{.items[0].metadata.name}')

kubectl get pod "$POD" -n "$NS" \
  -o jsonpath='{range .spec.containers[*]}{.name}{" request="}{.resources.requests.memory}{" limit="}{.resources.limits.memory}{"\n"}{end}'
```

Expected output for the database container:

```text
sidb-sample request=32Gi limit=32Gi
```

Repeat **step 3**, including the Pod lookup, and confirm these database values:

```text
NAME                     VALUE
------------------------ ------------------------------
memory_target            0
pga_aggregate_target     6G
sga_target               24G
```

The SIDB may still report `Healthy` before reconciliation completes.
Confirm the updated resource values, Pod memory settings, and database
parameters before considering the change complete.

Update your maintained SIDB manifest with the same memory request, memory
limit, SGA target, and PGA aggregate target.

## Configure Automatic Memory Management

This example sets `spec.initParams.memoryTarget` to `"24576Mi"` (24GiB),
configures `/dev/shm` to `24Gi`, and sets the regular container memory
request and limit to `32Gi`.

The AMM target covers both SGA and PGA. SIDB does not allow `memoryTarget`
together with explicit `sgaTarget`, `pgaAggregateTarget`, or HugePages.

Perform this change during a maintenance window because the Pod may be
recreated and the database restarted. For guidance on choosing AMM or
ASMM, see [Database Pod Resources and Init Parameters](../README.md#database-pod-resources-and-init-parameters).

### 1. Set the deployment details and check the current configuration

Replace the namespace placeholder. These commands assume one database
Pod and a container named after the SIDB.

```sh
NS='<namespace-for-sidb-deployment>'
SIDB=sidb-sample

kubectl get pods -n "$NS" -l "app=$SIDB"

kubectl get sidb "$SIDB" -n "$NS" \
  -o jsonpath='initParams={.spec.initParams}{"\n"}shmSize={.spec.shmSize}{"\n"}requests={.spec.resources.requests}{"\n"}limits={.spec.resources.limits}{"\n"}'
```

This procedure assumes that HugePages are not configured. If requests
or limits contain a nonzero `hugepages-2Mi` allocation, do not apply this
AMM example to that configuration.

### 2. Check shared memory and the database parameters

```sh
POD=$(kubectl get pods -n "$NS" -l "app=$SIDB" \
  -o jsonpath='{.items[0].metadata.name}')

kubectl exec "$POD" -n "$NS" -c "$SIDB" -- df -h /dev/shm

kubectl exec -i "$POD" -n "$NS" -c "$SIDB" -- \
  bash -lc 'sqlplus -s "/ as sysdba"' <<'SQL'
WHENEVER SQLERROR EXIT SQL.SQLCODE
SET LINESIZE 160
COLUMN name FORMAT A24
COLUMN value FORMAT A30
SELECT name, display_value AS value
FROM   v$parameter
WHERE  name IN (
  'memory_target',
  'memory_max_target',
  'sga_target',
  'pga_aggregate_target'
)
ORDER BY name;
EXIT
SQL
```

A default `/dev/shm` mount of `64M` is insufficient for this example.
The following patch increases its size along with the memory settings.

### 3. Remove explicit SGA/PGA targets and enable AMM

Check and if set, remove the explicit SGA/PGA targets:

```sh
kubectl get sidb "$SIDB" -n "$NS" \
  -o jsonpath='memoryRequest={.spec.resources.requests.memory}{"\n"}memoryLimit={.spec.resources.limits.memory}{"\n"}sgaTarget={.spec.initParams.sgaTarget}{"\n"}pgaAggregateTarget={.spec.initParams.pgaAggregateTarget}{"\n"}'

kubectl patch sidb "$SIDB" -n "$NS" \
  --type=merge \
  --patch '{"spec":{"initParams":{"sgaTarget":null,"pgaAggregateTarget":null}}}'
```

Set the regular memory request and limit along with setting`/dev/shm` and AMM target to 24GiB in the same patch:

```sh
kubectl patch sidb "$SIDB" -n "$NS" \
  --type=merge \
  --patch '{
    "spec": {
      "resources": {
        "requests": {"memory": "32Gi"},
        "limits": {"memory": "32Gi"}
      },
      "shmSize": "24Gi",
      "initParams": {
        "memoryTarget": "24576Mi"
      }
    }
  }'
```

`memoryTarget` must be a string; "24576Mi" represents 24GiB. HugePages must not be configured for this case.

### 4. Verify the resource and Pod configuration

```sh
kubectl get sidb "$SIDB" -n "$NS" \
  -o jsonpath='memoryTarget={.spec.initParams.memoryTarget}{"\n"}sgaTarget={.spec.initParams.sgaTarget}{"\n"}pgaAggregateTarget={.spec.initParams.pgaAggregateTarget}{"\n"}shmSize={.spec.shmSize}{"\n"}memoryRequest={.spec.resources.requests.memory}{"\n"}memoryLimit={.spec.resources.limits.memory}{"\n"}'
```

Expected output:

```text
memoryTarget=24576Mi
sgaTarget=
pgaAggregateTarget=
shmSize=24Gi
memoryRequest=32Gi
memoryLimit=32Gi
```

Watch the Pod while the operator reconciles:

```sh
kubectl get pods -n "$NS" -l "app=$SIDB" -w
```

Once the replacement Pod is Running and Ready, press Ctrl+C. Check SIDB
health and refresh the Pod name:

```sh
kubectl wait -n "$NS" \
  --for=jsonpath='{.status.status}'=Healthy \
  "singleinstancedatabase/$SIDB" --timeout=30m

POD=$(kubectl get pods -n "$NS" -l "app=$SIDB" \
  -o jsonpath='{.items[0].metadata.name}')

kubectl get pod "$POD" -n "$NS" \
  -o jsonpath='{range .spec.containers[*]}{.name}{" request="}{.resources.requests.memory}{" limit="}{.resources.limits.memory}{"\n"}{end}'

kubectl exec "$POD" -n "$NS" -c "$SIDB" -- df -h /dev/shm
```

Confirm that the database container has a `32Gi` memory request and limit,
and that `/dev/shm` reports a size of at least `24G`.

### 5. Verify the database parameters

Repeat **step 2**, including the Pod lookup. Expected database values
for this example are:

```text
NAME                     VALUE
------------------------ ------------------------------
memory_max_target        24G
memory_target            24G
pga_aggregate_target     0
sga_target               0
```

The zero SGA/PGA targets indicate that explicit targets have been cleared;
they do not mean that Oracle has stopped allocating SGA or PGA memory.

A successful patch or `Healthy` status alone does not confirm that the
new memory configuration is active. Verify the Pod settings and database
values before considering the change complete.

Update the maintained SIDB manifest with the same configuration, omitting
`sgaTarget` and `pgaAggregateTarget` from `spec.initParams`.

## Retain and Reuse Database Storage

This example verifies that `spec.persistence.retainOnDelete: true`
preserves the operator-created datafiles and Fast Recovery Area (FRA)
PVCs when the SIDB resource is deleted. It then recreates the SIDB using
those existing claims and verifies database access.

Perform this test during a maintenance window. The database is unavailable
between deletion and successful recreation. Keep the namespace, PVCs,
and referenced Secrets in place.

### 1. Check the existing deployment

Replace the namespace placeholder. These commands assume one database
Pod and a container named after the SIDB.

```sh
NS='<namespace-for-sidb-deployment>'
SIDB=sidb-sample
ORADATA_PVC=sidb-sample
FRA_PVC=sidb-sample-fra

kubectl get sidb "$SIDB" -n "$NS"
kubectl get pods -n "$NS" -l "app=$SIDB"
kubectl get pvc "$ORADATA_PVC" "$FRA_PVC" -n "$NS"
```

The SIDB must be Healthy, its database Pod Ready, and both PVCs Bound.

The initial deployment uses dynamically provisioned storage:

```yaml
spec:
  persistence:
    oradata:
      size: 100Gi
      storageClass: oci-bv
      accessMode: ReadWriteOnce
    fra:
      size: 100Gi
      storageClass: oci-bv
      accessMode: ReadWriteOnce
      mountPath: /opt/oracle/fra
      recoveryAreaSize: 80Gi
```

Confirm the retention setting:

```sh
kubectl get sidb "$SIDB" -n "$NS" \
  -o jsonpath='retainOnDelete={.spec.persistence.retainOnDelete}{"\n"}'
```

Patch `retainOnDelete` to `true` and check the retension setting again:

```sh
kubectl patch sidb "$SIDB" -n "$NS" \
  --type=merge \
  --patch '{"spec":{"persistence":{"retainOnDelete":true}}}'

kubectl get sidb "$SIDB" -n "$NS" \
  -o jsonpath='retainOnDelete={.spec.persistence.retainOnDelete}{"\n"}'
```

Expected output:

```text
retainOnDelete=true
```

Proceed only after this setting has been applied and reconciled.

### 2. Record the storage and database information

Capture the PVC names, UIDs, bound PVs, and status:

```sh
kubectl get pvc "$ORADATA_PVC" "$FRA_PVC" -n "$NS" \
  -o custom-columns='NAME:.metadata.name,UID:.metadata.uid,PV:.spec.volumeName,STATUS:.status.phase' \
  > sidb-pvc-before.txt

cat sidb-pvc-before.txt
```

Get the current Pod name and record the database identity, datafile paths,
and FRA configuration:

```sh
POD=$(kubectl get pods -n "$NS" -l "app=$SIDB" \
  -o jsonpath='{.items[0].metadata.name}')

kubectl exec -i "$POD" -n "$NS" -c "$SIDB" -- \
  bash -lc 'sqlplus -s "/ as sysdba"' <<'SQL'
WHENEVER SQLERROR EXIT SQL.SQLCODE
SET LINESIZE 200
COLUMN name FORMAT A100
SELECT dbid, name, open_mode FROM v$database;
SELECT file#, name FROM v$datafile ORDER BY file#;
SHOW PDBS
SHOW PARAMETER db_recovery_file_dest
EXIT
SQL
```

Record the DBID for comparison after recreation. For this example, the
FRA destination should be `/opt/oracle/fra` and its size should be `80G`.

Capture the existing FRA file paths:

```sh
kubectl exec "$POD" -n "$NS" -c "$SIDB" -- \
  find /opt/oracle/fra -type f -print |
  LC_ALL=C sort > sidb-fra-before.txt
```

If the inventory is empty, there are no existing FRA files to test for
preservation. Use a database with existing recovery files for that part
of the test. Avoid concurrent FRA cleanup during the comparison.

Also record the result of a known application-data query so that it can
be repeated after recreation.

### 3. Prepare a manifest that reuses the existing PVCs

Copy your maintained deployment manifest before deleting the SIDB:

```sh
cp singleinstancedatabase_persistence_pvc.yaml \
  singleinstancedatabase_reuse_persistence_pvc.yaml
```

In the copied manifest, replace the `oradata` and `fra` entries with:

```yaml
spec:
  persistence:
    oradata:
      pvcName: sidb-sample
    fra:
      pvcName: sidb-sample-fra
      mountPath: /opt/oracle/fra
      recoveryAreaSize: 80Gi
    retainOnDelete: true
```

This is a fragment of the complete manifest. Preserve any other required
persistence settings.

When `pvcName` is set, remove `size`, `storageClass`, and `accessMode`
from that volume entry. Both claims must exist in the SIDB namespace.

Keep the same resource name, namespace, SID, PDB name, and database image
version. Preserve the administrator credentials and any TDE wallet,
keys, and credential Secrets required to open the existing database.
Retain the current resource and initialization settings as well.

### 4. Delete the SIDB and verify PVC retention

Delete only the SIDB resource:

```sh
kubectl delete sidb "$SIDB" -n "$NS"

kubectl wait -n "$NS" --for=delete \
  "pod/$POD" --timeout=10m
```

Confirm that the SIDB no longer exists:

```sh
kubectl get sidb "$SIDB" -n "$NS"
```

Expected result: `NotFound`.

Verify that both PVCs remain Bound with their original UIDs and PVs:

```sh
kubectl get pvc "$ORADATA_PVC" "$FRA_PVC" -n "$NS" \
  -o custom-columns='NAME:.metadata.name,UID:.metadata.uid,PV:.spec.volumeName,STATUS:.status.phase' \
  > sidb-pvc-after-delete.txt

diff -u sidb-pvc-before.txt sidb-pvc-after-delete.txt
```

No differences are expected. Do not proceed with recreation until the
old database Pod has terminated.

### 5. Recreate the SIDB using the retained claims

Apply the complete manifest prepared in step 3:

```sh
kubectl apply -n "$NS" \
  -f singleinstancedatabase_reuse_persistence_pvc.yaml

kubectl wait -n "$NS" \
  --for=jsonpath='{.status.status}'=Healthy \
  "singleinstancedatabase/$SIDB" --timeout=30m

kubectl get pods -n "$NS" -l "app=$SIDB"
```

Confirm that the recreated database Pod is Ready.

### 6. Verify storage reuse and database access

Refresh the Pod name:

```sh
POD=$(kubectl get pods -n "$NS" -l "app=$SIDB" \
  -o jsonpath='{.items[0].metadata.name}')
```

Confirm which PVCs the new Pod references:

```sh
kubectl get pod "$POD" -n "$NS" \
  -o jsonpath='{range .spec.volumes[?(@.persistentVolumeClaim)]}{.name}{" claim="}{.persistentVolumeClaim.claimName}{"\n"}{end}'
```

The output must include `sidb-sample` and `sidb-sample-fra`.

Compare the PVC identities again:

```sh
kubectl get pvc "$ORADATA_PVC" "$FRA_PVC" -n "$NS" \
  -o custom-columns='NAME:.metadata.name,UID:.metadata.uid,PV:.spec.volumeName,STATUS:.status.phase' \
  > sidb-pvc-after-recreate.txt

diff -u sidb-pvc-before.txt sidb-pvc-after-recreate.txt
```

No differences are expected.

Verify both mount paths:

```sh
kubectl exec "$POD" -n "$NS" -c "$SIDB" -- \
  df -h /opt/oracle/oradata /opt/oracle/fra
```

Repeat the SQL checks from step 2 and confirm:

- The database has the same DBID and datafile paths.
- The primary database and application PDB are open as expected.
- The FRA destination remains `/opt/oracle/fra`.
- `db_recovery_file_dest_size` remains `80G`.
- The application-data query returns the expected retained data.

Check for missing FRA files:

```sh
kubectl exec "$POD" -n "$NS" -c "$SIDB" -- \
  find /opt/oracle/fra -type f -print |
  LC_ALL=C sort > sidb-fra-after.txt

LC_ALL=C comm -23 sidb-fra-before.txt sidb-fra-after.txt
```

No output means every previously recorded FRA file path is still present.
New files created during startup are permitted. This inventory check
verifies file presence, not backup-file integrity.

The test passes when the original PVCs remain Bound, the recreated Pod
mounts those same claims, the original database opens successfully, and
the recorded data and FRA files remain accessible.

## Expand Existing ORADATA and FRA PVCs

This example expands the ORADATA and FRA PVCs from `100Gi` to `150Gi`.
It applies to SIDB deployments that reference existing claims through
`spec.persistence.oradata.pvcName` and `spec.persistence.fra.pvcName`.

With `pvcName`, resize the PVC directly. For dynamically provisioned
storage, retain `size`, `storageClass`, and `accessMode`. When referencing
an existing PVC through `pvcName`, omit those provisioning fields from the
SIDB volume configuration and manage capacity directly on the PVC.

PVC expansion supports increasing capacity only.

### 1. Set the deployment details and check current capacity

These commands assume one database Pod and a container named after the SIDB.

```sh
NS='<namespace-for-sidb-deployment>'
SIDB=sidb-sample
ORADATA_PVC=sidb-sample
FRA_PVC=sidb-sample-fra

kubectl get sidb "$SIDB" -n "$NS" \
  -o jsonpath='oradataPVC={.spec.persistence.oradata.pvcName}{"\n"}fraPVC={.spec.persistence.fra.pvcName}{"\n"}'

kubectl get pvc "$ORADATA_PVC" "$FRA_PVC" -n "$NS"
```

Confirm that the SIDB references the expected claims and that both PVCs
are `Bound`. Record their current PV names and capacities.

This example uses the `oci-bv` StorageClass. Confirm that expansion is enabled:

```sh
kubectl get storageclass oci-bv \
  -o jsonpath='allowVolumeExpansion={.allowVolumeExpansion}{"\n"}'
```

Expected result:

```text
allowVolumeExpansion=true
```

The CSI driver must also support volume expansion.

### 2. Increase the PVC requests to 150Gi

Expand the ORADATA claim:

```sh
kubectl patch pvc "$ORADATA_PVC" -n "$NS" \
  --type=merge \
  --patch '{"spec":{"resources":{"requests":{"storage":"150Gi"}}}}'
```

Expand the FRA claim:

```sh
kubectl patch pvc "$FRA_PVC" -n "$NS" \
  --type=merge \
  --patch '{"spec":{"resources":{"requests":{"storage":"150Gi"}}}}'
```

Patch the PVC storage request, not the PV capacity.

### 3. Wait for the expanded capacity

A successful patch confirms that the request was accepted. Wait for the
reported capacity to reach the requested size:

```sh
kubectl wait -n "$NS" \
  --for=jsonpath='{.status.capacity.storage}'=150Gi \
  "pvc/$ORADATA_PVC" "pvc/$FRA_PVC" \
  --timeout=15m

kubectl get pvc "$ORADATA_PVC" "$FRA_PVC" -n "$NS"
```

Both PVCs should remain `Bound`, report `150Gi`, and retain their original
PV names. `Bound` alone does not confirm that expansion has completed.

Inspect the resize events, especially if the wait times out:

```sh
kubectl describe pvc "$ORADATA_PVC" "$FRA_PVC" -n "$NS"
```

Successful filesystem expansion may report:

```text
FileSystemResizeSuccessful
MountVolume.NodeExpandVolume succeeded
```

### 4. Verify filesystem capacity inside the database Pod

```sh
POD=$(kubectl get pods -n "$NS" -l "app=$SIDB" \
  -o jsonpath='{.items[0].metadata.name}')

kubectl exec "$POD" -n "$NS" -c "$SIDB" -- \
  df -h /opt/oracle/oradata /opt/oracle/fra
```

Verify that both filesystems have grown. In the tested configuration,
each `150Gi` PVC appeared as approximately `148G` in `df`; usable
filesystem capacity can be smaller than the provisioned volume capacity.

The tested OCI CSI configuration completed expansion while the Pod
remained running. A Pod restart is unnecessary when online expansion
succeeds. If filesystem expansion remains pending, inspect the PVC
events and follow the CSI driver's requirements for completing it.

### 5. Optionally increase the FRA database quota

Expanding the FRA PVC does not automatically increase
`DB_RECOVERY_FILE_DEST_SIZE`. For example, the database can retain an
`80G` FRA quota after its PVC grows to `150Gi`.

To increase the quota to `120G`, first align the SIDB configuration:

```sh
kubectl patch sidb "$SIDB" -n "$NS" \
  --type=merge \
  --patch '{"spec":{"persistence":{"fra":{"recoveryAreaSize":"120Gi"}}}}'
```

Apply and verify the database setting in the CDB root:

```sh
POD=$(kubectl get pods -n "$NS" -l "app=$SIDB" \
  -o jsonpath='{.items[0].metadata.name}')

kubectl exec -i "$POD" -n "$NS" -c "$SIDB" -- \
  bash -lc 'sqlplus -s "/ as sysdba"' <<'SQL'
WHENEVER SQLERROR EXIT SQL.SQLCODE
ALTER SESSION SET CONTAINER = CDB$ROOT;
SHOW PARAMETER db_recovery_file_dest_size
ALTER SYSTEM SET db_recovery_file_dest_size=120G SCOPE=BOTH;
SHOW PARAMETER db_recovery_file_dest_size
EXIT
SQL
```

Expected final value: `120G`. This leaves filesystem capacity outside
the FRA quota for overhead.

Update `spec.persistence.fra.recoveryAreaSize` to `120Gi` in your maintained
SIDB manifest as well. If you maintain separate PVC manifests, update
their storage requests to `150Gi`.

ORADATA expansion provides additional filesystem space. Existing
datafile sizes and autoextend limits remain database configuration
settings and may require separate changes.

## Add an Additional PVC

This example adds a dynamically provisioned PVC mounted at
`/opt/oracle/testdata`. It provides supplemental storage without moving
existing database datafiles or configuring the FRA.

The example assumes that no additional PVCs are currently configured.
Plan for a database interruption while the Pod is replaced to include
the new volume.

### 1. Set the deployment details

Replace the namespace placeholder. These commands assume one database
Pod and a container named after the SIDB. The configuration checks
require `jq`.

```sh
NS='<namespace-for-sidb-deployment>'
SIDB=sidb-sample
MOUNT=/opt/oracle/testdata
ADDITIONAL_PVC="${SIDB}-additional-0"

kubectl get sidb "$SIDB" -n "$NS"
kubectl get pods -n "$NS" -l "app=$SIDB"
kubectl get pvc -n "$NS"
kubectl get storageclass oci-bv
```

Confirm that the SIDB is Healthy and its database Pod is Ready.

Record the current Pod name:

```sh
OLD_POD=$(kubectl get pods -n "$NS" -l "app=$SIDB" \
  -o jsonpath='{.items[0].metadata.name}')

printf 'Current Pod: %s\n' "$OLD_POD"
```

### 2. Check the existing additional PVC configuration

```sh
kubectl get sidb "$SIDB" -n "$NS" -o json |
  jq '.spec.persistence.additionalPVCs // []'
```

For this example, the expected output is:

```json
[]
```

The merge patch in the next step replaces the entire `additionalPVCs`
list. If entries already exist, preserve them by including them alongside
the new entry in the patch. The generated PVC name will also use the new
entry's index rather than `additional-0`.

Confirm that the expected PVC does not already exist:

```sh
kubectl get pvc "$ADDITIONAL_PVC" -n "$NS"
```

Expected result: `NotFound`. If the claim exists, inspect it before
proceeding.

### 3. Patch the SIDB to add the PVC

Add a volume using `oci-bv`, with `storageSizeInGb` set to `50`:

```sh
kubectl patch sidb "$SIDB" -n "$NS" \
  --type=merge \
  --patch '{
    "spec": {
      "persistence": {
        "additionalPVCs": [
          {
            "mountPath": "/opt/oracle/testdata",
            "storageSizeInGb": 50,
            "storageClass": "oci-bv"
          }
        ]
      }
    }
  }'
```

Verify the updated configuration:

```sh
kubectl get sidb "$SIDB" -n "$NS" -o json |
  jq '.spec.persistence.additionalPVCs'
```

Expected output:

```json
[
  {
    "mountPath": "/opt/oracle/testdata",
    "storageClass": "oci-bv",
    "storageSizeInGb": 50
  }
]
```

### 4. Monitor PVC creation and Pod replacement

Allow the operator to reconcile the updated configuration.

```sh
kubectl get pvc,pods -n "$NS"
```

To monitor continuously, run the following commands in separate terminals:

```sh
kubectl get pvc -n "$NS" -w
```

```sh
kubectl get pods -n "$NS" -l "app=$SIDB" -w
```

Wait until the additional PVC is Bound and the replacement database Pod
is Ready. The replacement Pod should have a different name from
`$OLD_POD`. Press Ctrl+C to stop each watch.

Watch one resource type per command; do not use
`kubectl get pvc,pods -w`.

If the StorageClass uses `WaitForFirstConsumer`, a Pending PVC can be
expected until a Pod referencing it is scheduled.

If the PVC repeatedly disappears or the replacement Pod does not become
Ready, inspect the resource and events before continuing:

```sh
kubectl describe sidb "$SIDB" -n "$NS"
kubectl describe pvc "$ADDITIONAL_PVC" -n "$NS"
kubectl get events -n "$NS" --sort-by=.metadata.creationTimestamp
```

### 5. Verify the PVC, mount, and SIDB health

After Pod replacement completes, retrieve the new Pod name:

```sh
POD=$(kubectl get pods -n "$NS" -l "app=$SIDB" \
  -o jsonpath='{.items[0].metadata.name}')

printf 'Previous Pod: %s\nReplacement Pod: %s\n' "$OLD_POD" "$POD"

kubectl wait -n "$NS" \
  --for=condition=Ready "pod/$POD" --timeout=10m
```

Check the additional PVC's status and capacity:

```sh
kubectl get pvc "$ADDITIONAL_PVC" -n "$NS" \
  -o jsonpath='name={.metadata.name}{"\n"}phase={.status.phase}{"\n"}requested={.spec.resources.requests.storage}{"\n"}capacity={.status.capacity.storage}{"\n"}'
```

Confirm that `phase=Bound`.

List the Pod's PVC references:

```sh
kubectl get pod "$POD" -n "$NS" \
  -o jsonpath='{range .spec.volumes[?(@.persistentVolumeClaim)]}{.name}{" claim="}{.persistentVolumeClaim.claimName}{"\n"}{end}'
```

Confirm that the output includes `sidb-sample-additional-0`.

Verify the mount inside the database container:

```sh
kubectl exec -n "$NS" "$POD" -c "$SIDB" -- \
  mountpoint "$MOUNT"

kubectl exec -n "$NS" "$POD" -c "$SIDB" -- \
  df -h "$MOUNT"
```

The first command should report that `/opt/oracle/testdata` is a mount
point. The second displays the mounted filesystem's capacity and usage.

Finally, wait for the SIDB to report Healthy:

```sh
kubectl wait -n "$NS" \
  --for=jsonpath='{.status.status}'=Healthy \
  "singleinstancedatabase/$SIDB" --timeout=10m

kubectl get sidb "$SIDB" -n "$NS"
```

The test passes when the additional PVC remains Bound, the replacement
database Pod is Ready and mounts the volume at `/opt/oracle/testdata`,
and the SIDB reports Healthy.

### 6. Update the maintained manifest

Add the same configuration under `spec.persistence`, preserving the
existing `oradata`, `fra`, and other persistence settings:

```yaml
spec:
  persistence:
    additionalPVCs:
      - mountPath: /opt/oracle/testdata
        storageSizeInGb: 50
        storageClass: oci-bv
```

## Add and Remove TNS Aliases

This example adds `TEST_SIDB_ALIAS` through `spec.tnsAliases`, verifies
its generated entry in `tnsnames.ora`, and then removes it.

Use an existing database service as the alias destination. Adding a TNS
alias does not create a database service.

### 1. Set the deployment details

Replace the namespace placeholder and adjust the destination host,
database service, and TNS file path for your deployment.

These commands assume one database Pod and a container named after the
SIDB. The patch commands require `jq`.

```sh
NS='<namespace-for-sidb-deployment>'
SIDB=sidb-sample
ALIAS=TEST_SIDB_ALIAS
TNS_HOST="${SIDB}.${NS}.svc.cluster.local"
TNS_SERVICE=ORCLPDB1
TNS_FILE=/opt/oracle/product/26ai/dbhome_1/network/admin/tnsnames.ora

kubectl get sidb "$SIDB" -n "$NS"
kubectl get pods -n "$NS" -l "app=$SIDB"
```

Confirm that the SIDB is Healthy and its database Pod is Ready.

### 2. Check the existing aliases

Display the configured aliases:

```sh
kubectl get sidb "$SIDB" -n "$NS" -o json |
  jq '.spec.tnsAliases // []'
```

An empty list means no aliases are explicitly configured through
`spec.tnsAliases`. The TNS file can still contain other database-generated
entries.

Define a helper to check whether the test alias exists in the TNS file.
It refreshes the Pod name before each check and reports file or command
errors separately from an absent alias.

```sh
check_tns_alias() {
  POD=$(kubectl get pods -n "$NS" -l "app=$SIDB" \
    -o jsonpath='{.items[0].metadata.name}') || return

  kubectl exec -n "$NS" "$POD" -c "$SIDB" -- \
    bash -c '
      grep -Ei -- "^[[:space:]]*${2}[[:space:]]*=" "$1" >/dev/null
      result=$?

      case "$result" in
        0) echo present ;;
        1) echo absent ;;
        *) exit "$result" ;;
      esac
    ' bash "$TNS_FILE" "$ALIAS"
}

check_tns_alias
```

Expected output before adding the test alias:

```text
absent
```

Proceed only if `TEST_SIDB_ALIAS` is absent from both the configured
alias list and the TNS file. If it already exists, select another unused
test alias.

### 3. Patch the SIDB to add the alias

The following command preserves existing entries and adds the test alias.
It also handles a missing or empty `tnsAliases` list without first clearing
the list.

```sh
PATCH=$(kubectl get sidb "$SIDB" -n "$NS" -o json |
  jq -ce \
    --arg alias "$ALIAS" \
    --arg host "$TNS_HOST" \
    --arg service "$TNS_SERVICE" '
      (.spec.tnsAliases // []) as $aliases
      | if any($aliases[];
          (.name | ascii_upcase) == ($alias | ascii_upcase))
        then error("The test alias is already configured")
        else {
          metadata: {
            resourceVersion: .metadata.resourceVersion
          },
          spec: {
            tnsAliases: ($aliases + [{
              name: $alias,
              host: $host,
              port: 1521,
              protocol: "TCP",
              serviceName: $service
            }])
          }
        }
        end
    ') &&
kubectl patch sidb "$SIDB" -n "$NS" \
  --type=merge \
  --patch "$PATCH"
```

The `resourceVersion` check prevents overwriting a concurrent update.
If the server reports a resource-version conflict, rerun this step to
build the patch from the latest resource.

### 4. Verify the added alias

Check the alias in the SIDB resource:

```sh
kubectl get sidb "$SIDB" -n "$NS" -o json |
  jq --arg alias "$ALIAS" \
    '.spec.tnsAliases[]? | select(.name == $alias)'
```

Expected output for namespace `sidb`:

```json
{
  "host": "sidb-sample.sidb.svc.cluster.local",
  "name": "TEST_SIDB_ALIAS",
  "port": 1521,
  "protocol": "TCP",
  "serviceName": "ORCLPDB1"
}
```

Allow the operator to reconcile, then check the TNS file:

```sh
check_tns_alias
```

Expected output:

```text
present
```

If the alias is still absent, allow reconciliation to complete and repeat
the check.

Display the complete operator-managed block:

```sh
kubectl exec -n "$NS" "$POD" -c "$SIDB" -- \
  awk -v alias="$ALIAS" '
    $0 == "# BEGIN AUTO_TNS_" alias {printing=1}
    printing {print}
    $0 == "# END AUTO_TNS_" alias {printing=0}
  ' "$TNS_FILE"
```

Confirm that the block contains the requested alias, host, port,
protocol, and service name.

### 5. Remove the test alias

Locate the alias by name and remove only that entry. Do not assume that
the test alias is at array index `0`.

```sh
PATCH=$(kubectl get sidb "$SIDB" -n "$NS" -o json |
  jq -ce --arg alias "$ALIAS" '
    (.spec.tnsAliases // [] | map(.name) | index($alias)) as $index
    | if $index == null
      then error("The test alias is not configured")
      else
        ("/spec/tnsAliases/" + ($index | tostring)) as $path
        | [
            {
              op: "test",
              path: ($path + "/name"),
              value: $alias
            },
            {
              op: "remove",
              path: $path
            }
          ]
      end
    ') &&
kubectl patch sidb "$SIDB" -n "$NS" \
  --type=json \
  --patch "$PATCH"
```

The `test` operation checks that the selected entry still has the expected
name before removing it.

### 6. Verify removal

Display the remaining configured aliases:

```sh
kubectl get sidb "$SIDB" -n "$NS" -o json |
  jq '.spec.tnsAliases // []'
```

If the test alias was the only configured entry, the expected output is:

```json
[]
```

Otherwise, the existing aliases should remain and `TEST_SIDB_ALIAS`
should be absent.

Allow the operator to reconcile, then check the TNS file again:

```sh
check_tns_alias
```

Expected output:

```text
absent
```

Confirm that the generated block has also been removed:

```sh
kubectl exec -n "$NS" "$POD" -c "$SIDB" -- \
  awk -v alias="$ALIAS" '
    $0 == "# BEGIN AUTO_TNS_" alias {printing=1}
    printing {print}
    $0 == "# END AUTO_TNS_" alias {printing=0}
  ' "$TNS_FILE"
```

Expected result: no output.

Check SIDB health:

```sh
kubectl get sidb "$SIDB" -n "$NS"
```

The test passes when:

- Adding the alias creates the expected entry in both `spec.tnsAliases`
  and `tnsnames.ora`.
- Removing the alias removes its resource entry and generated TNS block.
- Other configured aliases and existing database-generated entries remain.
- The SIDB remains Healthy.

Keep the maintained SIDB manifest aligned with the final configuration.
Remove the test alias from the manifest if it was added there.
