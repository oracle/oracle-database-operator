# SIDB e2e tests

These tests run against a real Kubernetes cluster and Oracle database image. They follow the shell-driven e2e pattern used by the sharding tests.

Provisioning cases own their manifests; operation cases reuse test03. Each
numbered case owns its configuration wrapper and `assert-topology.sh`. The shared lifecycle scripts and helper library are in
`tests/common/`; test01 and test02 call those scripts without delegating to
each other.

## Numbered operator E2E cases

The unified operator E2E dispatcher uses the selectors below. Each numbered
case retains its resources after diagnostics; use the explicit
'sidb/cleanup' entry before a later case when the namespace must be emptied.

| Selector | What it does | Main defaults | Details |
| --- | --- | --- | --- |
| sidb/test01 | Creates a baseline primary SIDB, then patches TCPS onto the ready database so Agent Factory can use the client wallet on port 2484. | sidb-e2e-primary, ORCL1, ORCLPDB1, 100 GiB, oci-bv, post-ready TCPS | [Test README](tests/01-provision/README.md) |
| sidb/test02 | Creates a separate secure primary with TDE, day-0 TCPS, resource limits, and live PDB MAX_STRING_SIZE=EXTENDED verification. | sidb-e2e-primary-test02, 2 CPU, 8 GiB, 3 GiB SGA, 1 GiB PGA, 1000 processes | [Test README](tests/02-provision/README.md) |
| sidb/test03 | Basic same-cluster Data Guard: healthy primary, healthy databaseRef standby, generated healthy broker. | PRIMDB, STBYDB, 100/200 GiB, TCP | [Test README](tests/03-dataguard/README.md) |
| sidb/test04 | Switch to the current physical standby, verify both roles, then switch back to the primary discovered at the start. | Reuses test03; restores its starting state on success | [Test README](tests/04-switchover/README.md) |
| sidb/test05 | Convert the current physical standby to SNAPSHOT_STANDBY, verify READ WRITE, then convert it to PHYSICAL_STANDBY and verify managed recovery resumes. | Reuses test03; restores its starting state on success | [Test README](tests/05-snapshot-standby/README.md) |
| sidb/test06 | Enable FSFO and the observer, verify both database views and the observer Pod, then disable FSFO and verify that the observer Pod disappears. | Reuses test03; restores its starting state on success | [Test README](tests/06-fsfo/README.md) |
| sidb/cleanup | Clears the dedicated test namespace while retaining the Namespace and protected access objects. | Namespace sidb-ns | [Cleanup script](tests/cleanup/run.sh) |

Use E2E_TESTS for an ordered sequence. For example:

~~~
sidb/test01,agentfactory/test01,sidb/cleanup,sidb/test02
~~~

Data Guard operations run after provisioning, without an intervening cleanup:

```text
sidb/test03,sidb/test04,sidb/test05,sidb/test06
```

Tests 04–06 can also run directly against the existing Healthy test03 resources.
They discover roles/targets, use new request IDs and validate live SQL alongside
broker/SIDB status. Each case uses the existing diagnostic collection and archive
pattern. `sidb/all` keeps its existing test01/test02 behavior; select the Data Guard
suite explicitly.

### Fresh setup versus reuse

Test03 provisions the primary, standby and broker. Tests04–06 reuse those
resources; selecting an operation case never automatically runs test03. Pipeline
operator installation alone does not create the databases.

For a fresh setup, use `E2E_TEST_PRESET=custom` and select:

| Operations to test | E2E_TESTS | E2E_CLEANUP_POLICY |
| --- | --- | --- |
| Provisioning only | sidb/test03 | pre |
| Switchover | sidb/test03,sidb/test04 | pre |
| Snapshot conversion | sidb/test03,sidb/test05 | pre |
| FSFO | sidb/test03,sidb/test06 | pre |
| Switchover and snapshot | sidb/test03,sidb/test04,sidb/test05 | pre |
| Switchover and FSFO | sidb/test03,sidb/test04,sidb/test06 | pre |
| Snapshot and FSFO | sidb/test03,sidb/test05,sidb/test06 | pre |
| All Data Guard cases | sidb/test03,sidb/test04,sidb/test05,sidb/test06 | pre |

With an existing Healthy test03 setup, select `sidb/test04`, `sidb/test05`,
`sidb/test06`, or an ordered combination of them, and set
`E2E_CLEANUP_POLICY=none`. The baseline needs one primary, one physical standby,
a Healthy broker, Ready database Pods, and FSFO/observer disabled. Using `pre`
with only operation cases deletes their prerequisites. On an empty namespace,
operation-only selections fail prerequisites instead of provisioning databases.

Tests04–06 wait up to `SIDB_DG_PREFLIGHT_TIMEOUT_SECONDS=600` for two consecutive
successful checks, using `SIDB_POLL_INTERVAL`. Checks cover current broker Ready
generation, member health and roles, Ready Pods, and live SQL for CDB/PDB modes,
managed recovery and FSFO disabled. The timeout is evaluated between checks;
in-flight kubectl/SQL commands retain their existing timeout behavior.
Resource replacement/deletion, active operations and incompatible FSFO/observer
settings are rejected. Readiness retries do not create missing resources.

### Cleanup and credential ownership

`pre` cleans before a run and retains its results afterward. `none` retains
the existing setup for reuse. `post` cleans afterward; `both` cleans before and
after. Do not insert `sidb/cleanup` between test03 and its operation cases.

The common prerequisites label newly created admin, TDE and TLS test Secrets
with owner `operator-e2e`, controller `sidb`, test-id and pipeline. Existing
caller-provided Secrets are not newly labeled as cleanup-owned on reuse.
After managed CR deletion succeeds, pipeline pre-cleanup removes labeled SIDB
credentials for tests01–03 in its cleanup namespaces, verifying no SIDBs remain
there. It preserves unlabeled credentials, registry credentials and unrelated
Secrets. A failed database deletion blocks credential deletion.

Older unlabeled Secrets, including `sidb-e2e-admin-test03` from runs before this
change, require one explicit cleanup after their databases are removed. Do not
automatically adopt or delete an arbitrary existing Secret by its name.

Test03 explicit cleanup deletes broker, standby, primary and the credential names
recorded in its saved environment. Common cleanup preserves credentials and the
saved environment if primary deletion fails; it preserves the saved environment
on later cleanup failures so cleanup can be retried. Tests04–06 collect diagnostics
and retain shared resources. Deleting CRs relies on the operator/garbage collector
for managed Pods, Services and PVCs; inspect remaining objects afterward if an
operator deletion or finalizer fails. This change does not force-delete finalizers
or broaden pre-cleanup to unlabeled resources.

`sidb/cleanup` is a broad cleaner for the dedicated SIDB namespace. It preserves
the Namespace and protected access objects, but deletes other discovered resources
there, including Secrets. Use it only when all workloads in that namespace can
be discarded. Keep the operator running until managed CR finalizers complete.

The older provision, clone, and dataguard selectors documented below belong
to the legacy SIDB runner and are separate from sidb/test01 and sidb/test02.

Tests:

- `provision`: provision a v4 primary SIDB and validate status, Pod, PVC, Service, and webhook rejection.
- `clone`: reuse the run's primary and create a `createAs: clone` SIDB using `primarySource.databaseRef`.
- `dataguard`: reuse the run's primary, create a physical standby using `primarySource.connectString`, then apply the rendered Data Guard Broker resource.
- `all`: run all tests in order.

Resources are retained by default for diagnosis. Add `--cleanup` only for disposable namespaces.

## Run

Install the operator and SIDB CRDs first, then use the dedicated `sidb-ns` namespace watched by the operator. The test defaults to the Oracle 23.26 image and `oci-bv` storage class; override them when the cluster requires different values:

```bash
export E2E_NAMESPACE=sidb-ns
export SIDB_IMAGE=<registry>/oracle/database:<tag>
export SIDB_PULL_SECRET=oracle-container-registry-secret
export SIDB_STORAGE_CLASS=oci-bv
export SIDB_ADMIN_PASSWORD='replace-me'

test/e2e/controllers/sidb/scripts/run.sh \
  --namespace "$E2E_NAMESPACE" \
  --tests provision
```

The GitLab operator E2E flow uses the stable `sidb-ns` namespace. The
operator watch list also reserves `gdd-ns` for Sharding, `paf-ns` for Private
Agent Factory, `pai-ns` for Private AI, `dbcs-ns` for DBCS, `cman-ns` for CMAN,
and `rac-ns` for Oracle Restart/RAC. Agent Factory connects to the SIDB
service at `sidb-e2e-primary.sidb-ns.svc.cluster.local`.

Run the clone case:

```bash
test/e2e/controllers/sidb/scripts/run.sh --namespace "$E2E_NAMESPACE" --tests clone
```

Run the standby and Data Guard case. The runner chooses the primary connect string in this order:

1. `SIDB_PRIMARY_EXTERNAL_CONNECT_STRING`, when explicitly supplied.
2. The primary SIDB `status.externalConnectString`.
3. The primary SIDB `status.connectString`.

Therefore, the primary endpoint can be created dynamically by the test. The selected endpoint must still be reachable from the standby pod. To override it explicitly:

```bash
export SIDB_PRIMARY_EXTERNAL_CONNECT_STRING='primary.example.com:1521/ORCL1'
test/e2e/controllers/sidb/scripts/run.sh \
  --namespace "$E2E_NAMESPACE" \
  --tests dataguard
```

If the variable is omitted, the primary is created with a NodePort endpoint and the generated status connect string is used.

Run all profiles in sequence:

```bash
test/e2e/controllers/sidb/scripts/run.sh --namespace "$E2E_NAMESPACE" --tests all
```

Use `--cleanup` only in a disposable namespace:

```bash
test/e2e/controllers/sidb/scripts/run.sh --namespace "$E2E_NAMESPACE" --tests provision --cleanup
```

Results and rendered manifests are written below `test/e2e/controllers/sidb/results/<UTC-timestamp>-<profile>-<namespace>/` (or the equivalent timestamped subdirectory under `SIDB_E2E_RESULTS_DIR`). The Data Guard profile expects the standby status field `status.dataguard.renderedBrokerSpec` to contain an applicable `DataguardBroker` manifest.

`sidb/test01` starts TCP-only and enables TCPS with `enable-tcps.sh` after the
primary is ready. Use `E2E_TESTS=sidb/test01,agentfactory/test01` for the
patch-TCPS plus Agent Factory wallet/2484 path.

The ordered operator E2E dispatcher also provides `sidb/test02`. It creates
an independent primary with TDE and TCPS enabled at day-0, a generated test
TLS Secret, 2 CPU/8 GiB resource requests and limits, 3 GiB SGA, 1 GiB PGA,
1000 processes, and verifies live PDB `MAX_STRING_SIZE=EXTENDED` with SQL
against `v$parameter`. Use `E2E_TESTS=sidb/test02` or run
`sidb/test02,agentfactory/test02` when the wallet-based Agent Factory test
should consume this database.


## CI/CD scheduling

Include `test/e2e/controllers/sidb/gitlab-ci.yml` from the repository GitLab pipeline. Configure these as protected/masked CI/CD variables:

- `SIDB_IMAGE` (default: `<registry>/oracle/database:<tag>`)
- `SIDB_STORAGE_CLASS` (default: `oci-bv`)
- `SIDB_PULL_SECRET` when the image is private
- `SIDB_ADMIN_PASSWORD` (optional; the preparation script generates a temporary value when omitted)
- `KUBE_CONTEXT_FILE` (optional GitLab **File** variable containing kubeconfig; when omitted, the runner's default kubeconfig is used)
- `KUBE_CONTEXT_NAME` (optional context name)
- `SIDB_PRIMARY_EXTERNAL_CONNECT_STRING` for the external Data Guard profile

Create GitLab schedules in **Build > Pipeline schedules**. For a daily run, use a cron such as `0 2 * * *`; for a weekly run, use `0 2 * * 0`. Set `SIDB_TESTS` as a schedule variable (`provision`, `clone`, `dataguard`, comma-separated combinations, or `all`). `SIDB_PROFILE` remains a compatibility alias. The same jobs can be started manually from **Run pipeline**; the on-demand job is manual and accepts the same variables.

The preparation script writes a shell-safe environment file, defaults the resource names, image, storage class, and timeouts, selects `KUBE_CONTEXT_FILE` when supplied, and otherwise leaves `KUBECONFIG` pointed at the normal kubectl default. Override the image or storage class when the cluster does not provide access to the defaults.

## GitHub Actions

The repository includes `.github/workflows/sidb-e2e.yml`. GitHub checks out the repository into `$GITHUB_WORKSPACE`, so the runner invokes the checked-in script with the relative path `test/e2e/controllers/sidb/scripts/run.sh`.

Configure these GitHub repository or environment variables/secrets:

- Variables: `SIDB_IMAGE` (default `<registry>/oracle/database:<tag>`), `SIDB_STORAGE_CLASS` (default `oci-bv`), optional `SIDB_PULL_SECRET`, `SIDB_PROFILE`, `KUBE_CONTEXT_NAME`, and `SIDB_PRIMARY_EXTERNAL_CONNECT_STRING`.
- Secrets: `SIDB_ADMIN_PASSWORD` (optional) and `KUBECONFIG_B64` (optional base64-encoded kubeconfig).

`KUBECONFIG_B64` takes precedence through a temporary `$RUNNER_TEMP/kubeconfig`. If it is absent, the workflow uses the runner's normal kubeconfig. `KUBE_CONTEXT_NAME` selects a context without modifying the kubeconfig file.

Start manually from **Actions → SIDB e2e → Run workflow**, choosing `profile` and `namespace`. The workflow also runs daily at `02:00 UTC`; change the cron in `.github/workflows/sidb-e2e.yml` or duplicate the workflow for a weekly schedule.

A GitHub-hosted runner must have network reachability to the Kubernetes API and the database endpoint. For private clusters, use a self-hosted runner, VPN, or equivalent network path.

### Conditional GitLab execution and images

The SIDB jobs are included in the pipeline but disabled by default. Enable them explicitly with a CI/CD or schedule variable:

```text
SIDB_E2E_ENABLED=true
```

Pass the database image with:

```text
SIDB_IMAGE=<registry>/oracle/database:<tag>
```

To install or update the operator before running the tests, pass:

```text
OPERATOR_MANIFEST=oracle-database-operator.yaml
OPERATOR_IMAGE=container-registry.example/operator:tag
OPERATOR_NAMESPACE=oracle-database-operator-system
```

`OPERATOR_IMAGE` is applied to the Deployment selected by the `control-plane=controller-manager` label. If discovery is ambiguous, set `OPERATOR_DEPLOYMENT` explicitly. The container defaults to `manager`; override it with `OPERATOR_CONTAINER` if needed.

For a manual GitLab run, set `SIDB_E2E_ENABLED=true` and the desired `SIDB_PROFILE` in **Run pipeline**. For a schedule, set the same variables on the schedule. When `SIDB_E2E_ENABLED` is false or absent, both SIDB jobs are skipped.

### Test selection and resource reuse

Use `--tests` with a comma-separated list:

```bash
# Only provisioning and validation
test/e2e/controllers/sidb/scripts/run.sh --namespace "$E2E_NAMESPACE" --tests provision

# Clone only; the runner creates or reuses the primary first
test/e2e/controllers/sidb/scripts/run.sh --namespace "$E2E_NAMESPACE" --tests clone

# Run clone and Data Guard in one invocation
test/e2e/controllers/sidb/scripts/run.sh --namespace "$E2E_NAMESPACE" --tests clone,dataguard

# Run all tests (default)
test/e2e/controllers/sidb/scripts/run.sh --namespace "$E2E_NAMESPACE" --tests all
```

For every invocation, the runner ensures one healthy primary named by `SIDB_PRIMARY_NAME`. The selected tests then reuse it: clone creates `SIDB_CLONE_NAME`, and Data Guard creates `SIDB_STANDBY_NAME` and applies the rendered Broker manifest. The primary is not recreated between tests. `--profile` remains accepted as a backward-compatible alias for `--tests`, and `SIDB_TESTS` takes precedence over the older `SIDB_PROFILE` environment variable.

### Selecting tests in CI/CD

For GitLab manual pipelines or schedules, set the variable `SIDB_TESTS`:

```text
SIDB_TESTS=all
SIDB_TESTS=clone
SIDB_TESTS=dataguard
SIDB_TESTS=clone,dataguard
```

For GitHub Actions, choose the `profile` input when starting **Run workflow**, or set the repository variable `SIDB_TESTS`. The workflow passes that value to `run.sh --tests`.

### CRD and operator update flow

When SIDB fields or validation change, the CI flow automatically applies the checked-in SIDB CRD:

```text
config/crd/bases/database.oracle.com_singleinstancedatabases.yaml
```

You only need to override it when using a custom/generated bundle:

```text
CRD_MANIFEST=oracle-database-operator-system.yaml
OPERATOR_MANIFEST=oracle-database-operator.yaml
OPERATOR_IMAGE=registry.example/operator:feature-build
```

The CI flow applies `CRD_MANIFEST` first, then applies `OPERATOR_MANIFEST`, overrides the operator Deployment image when `OPERATOR_IMAGE` is set, waits for rollout, and only then runs SIDB tests. Keep the CRD and operator image from the same source revision.

`CRD_MANIFEST` defaults to the checked-in SIDB CRD and `OPERATOR_MANIFEST` remains optional. If omitted, the workflow uses the operator already installed in the cluster. Normal cleanup never deletes CRDs or the operator; use a disposable cluster for destructive schema-install experiments.

## New basic Data Guard numbered case

The `tests/03-dataguard` case follows the supplied working primary → standby →
`gen_dg.sh` → DataguardBroker workflow and reuses the common lifecycle. See
[the complete case](tests/03-dataguard/README.md) for direct execution and
[dispatcher integration notes](DISPATCHER-INTEGRATION.md). The unified dispatcher supports `sidb/test03` through `sidb/test06`.
