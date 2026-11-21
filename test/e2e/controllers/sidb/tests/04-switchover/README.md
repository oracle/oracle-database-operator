# SIDB test04: Switchover and switchback

Switch to the current physical standby, verify both roles, then switch back to the primary discovered at the start.

## Prerequisites

Run `sidb/test03` first, or use the resources retained by its passing run.
The operator must already be installed and watching `sidb-ns`. Both databases
and the broker must be Healthy, with exactly one PRIMARY and one PHYSICAL_STANDBY.
FSFO must be disabled in spec, broker status and both databases' SQL views.
Active broker operations cause this case to fail before issuing a new request.

Defaults are `sidb-e2e-primary-test03`, `sidb-e2e-standby-test03`, and
`sidb-e2e-dg-test03`. Resource names remain the same after a role reversal;
the helper discovers the current roles instead of assuming names imply roles.
Operation targets come from the broker member's `dbUniqueName`.

This case creates no SIDB, PVC, password or admin Secret. No new database
manifest is required. It uses the existing common libraries and the additional
`../common/dataguard-operations-lib.sh`.

## Direct run against the retained databases

From the repository root, after establishing the existing <target-env> Bastion tunnel:

```bash
export KUBECONFIG=$KUBECONFIG_DIR
export KUBE_CONTEXT_NAME=context-c6vqu4eb34q
bash test/e2e/controllers/sidb/tests/04-switchover/run.sh
```

The database container must contain `bash` and `sqlplus` and permit local
`sqlplus "/ as sysdba"` authentication. The helper explicitly sets ORACLE_SID
from the SIDB spec and selects the named database container. Kubernetes access
must allow get/list, broker patch, Pod exec and Pod logs.

## Fresh pipeline

Select preset `sidb/test03,sidb/test04`, or use custom:

```text
RUN_OPERATOR_E2E_ON_DEMAND=true
E2E_TARGET_ENV=<target-env>
E2E_TEST_PRESET=custom
E2E_TESTS=sidb/test03,sidb/test04
E2E_CLEANUP_POLICY=pre
```

Use the full suite `sidb/test03,sidb/test04,sidb/test05,sidb/test06` to run
all cases. Do not put `sidb/cleanup` between these entries. With pre-cleanup
enabled, selecting only `sidb/test04` will remove the test03 prerequisite
resources before this case starts. Direct execution reuses them without the
pipeline's operator setup and pre-cleanup.

## Verification and operation receipts

Every switchover/conversion request gets a unique ID containing the case,
pipeline/job, time, process and random component. The API patch response must
contain the new requested value; a CRD that drops unsupported fields fails.

When a broker publishes `.status.operations.<operation>` (or the equivalent
operation-status/last-operation record), the helper requires the matching
request ID and a Succeeded/Success/Completed phase. Failed, Error, Rejected
or Cancelled results fail immediately. If no operation receipt is published,
completion still requires the persisted new request, the Ready condition's
observed generation, broker resolved-member roles, Healthy SIDB roles and
live SQL evidence. A cached Healthy status alone cannot pass.

SQL assertions verify PRIMARY/READ WRITE, SNAPSHOT STANDBY/READ WRITE, or
PHYSICAL STANDBY with mounted/read-only-with-apply mode and an MRP process.
FSFO enable requires a ready observer Pod and SYNCHRONIZED or TARGET UNDER
LAG LIMIT with observer presence YES on both databases. Disable requires
DISABLED; other FS_FAILOVER columns are not checked in that state.

This is an enable/disable test, not a forced crash or automatic-failover test.
No database workload or committed application data is created in snapshot mode.

## Configuration

| Variable | Default / meaning |
| --- | --- |
| SIDB_CASE_NAMESPACE / SIDB_E2E_NAMESPACE | sidb-ns |
| SIDB_TEST04_PRIMARY_NAME | Original test03 primary resource name |
| SIDB_TEST04_STANDBY_NAME | Original test03 standby resource name |
| SIDB_TEST04_BROKER_NAME | Existing test03 broker resource name |
| SIDB_TEST03_PRIMARY_NAME / STANDBY_NAME / BROKER_NAME | Shared defaults also honored by tests 04–06 |
| SIDB_DG_OPERATION_TIMEOUT_SECONDS | 1800 seconds per operation |
| SIDB_POLL_INTERVAL | 20 seconds |
| SIDB_TEST04_ENV_FILE | Private case environment file; isolated from test03 |
| SIDB_TEST04_RESULTS_DIR | Optional case results directory override |
| SIDB_DG_OBSERVER_NAME | Existing broker name with -observer suffix |
| SIDB_DG_OBSERVER_IMAGE | Existing broker spec.execution.image; override for a tested observer image |

For test06, no observer configuration/Pod may exist at the start. Existing
policy fields are preserved by the merge patch; fastStartFailover ends false
and the observer field is removed using null. No protection-mode change is made.

## Logs and final state

Pipeline logs use the existing per-case finalizer:
`$OPERATOR_LOG_DIR/cases/sidb-test04/` and `sidb_test04.zip`.
Direct execution writes into
`test/e2e/controllers/sidb/results/cases/sidb-test04/` by default.
Redirect or tee the direct runner output if you also need its console transcript.

Important files:

- `operations/01-switchover/patch.json`, `patch-response.json`, `broker.json`.
- `operations/02-switchback/` with the second operation's evidence.
- `operations/*/*-sql.log` with role, open mode, MRP and FSFO query results.
- `operations/*/operation-status.json` when using switchover/conversion.
- `snapshots/before/`, per-step snapshots, and `snapshots/passed/`.
- `diagnostics/dataguard/`, database/alert/operator logs from common helpers.
- For test06, `operations/01-enable-fsfo/pod-logs/` preserves observer logs
  before its Pod is removed.

On success, the starting roles are restored, FSFO is disabled and the database
resources are retained. `cleanup.sh` is a diagnostic finalizer for these shared
resources; it does not delete them. On failure, the pipeline fails and captures
state without issuing a blind compensating patch. An operation might still
be running or already committed; inspect its request/status before recovery.
Only use the existing explicit test03 or controller cleanup to discard databases.

## Validation scope

Shell syntax and mocked Kubernetes/SQL success/failure paths were checked.
These new scripts have not yet been run on the real <target-env> cluster. They target
the private v4 operation API exercised in your working manual patches.
