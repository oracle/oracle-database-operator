# SIDB test03 — basic Data Guard

This case follows the supplied working, same-cluster TCP workflow:

1. Apply the primary manifest.
2. Wait for primary `Healthy` with role `PRIMARY`.
3. Apply the standby manifest with `primarySource.databaseRef` pointing to the primary.
4. Wait for standby `Healthy` with role `PHYSICAL_STANDBY` (also accepts `PHYSICAL STANDBY`).
5. Wait for standby `status.dataguard.readyForBroker=true` and a rendered broker spec.
6. Run `scripts/gen_dg.sh`, based on the supplied `gen_deg.sh` and its multi-member renderer.
7. Server-dry-run and apply the generated `DataguardBroker` manifest.
8. Wait for broker `Healthy`, then assert both databases, Pods, PVCs, Services and broker topology.

Both SIDBs enable `dataguard.prereqs.enabled`, one replica and `loadBalancer`.
The primary enables archive logging, force logging and flashback. The primary
uses 100Gi and the standby 200Gi, matching the supplied examples. The shared
admin Secret is retained by both CRs (`keepSecret: true`). No TDE/TCPS is enabled.

This is a provisioning/health test. It does not perform switchover, failover,
FSFO, snapshot standby conversion or a SQL redo-apply workload.

## Files and common functions

| File | Responsibility | Common reuse |
| --- | --- | --- |
| `test03.yaml` | Primary SIDB template | `sidb_case_render_manifest` |
| `standby.yaml` | Standby SIDB template | Same renderer, scoped standby values |
| `lib.sh` | test03 names/configuration | Sources `../common/lib.sh` and `../common/dataguard-lib.sh` |
| `prerequisites.sh` | Collision checks; namespace/admin Secret; render both manifests; persist settings | Invokes common prerequisites, `sidb_case_write_env` |
| `verify-prerequisites.sh` | SIDB/broker CRDs, storage, Secret, RBAC, primary admission | Invokes common verification |
| `wait-ready.sh` | Ordered primary → standby → generator → broker lifecycle | Invokes common readiness twice |
| `assert-topology.sh` | Final health, roles, SID/PDB, image, prerequisites, reference, ready Pods, mounted Bound PVCs, Services; exactly two TCP broker members | Shared Data Guard assertions and common diagnostics |
| `cleanup.sh` | Explicit broker → standby → primary cleanup | Invokes common cleanup after deleting dependents |
| `run.sh` | Standalone runner for these stages | Uses `sidb_case_kubectl` to apply only the rendered primary |
| `../../scripts/gen_dg.sh` | Discover standby, call renderer, normalize output | Same topology workflow as supplied `gen_deg.sh` |
| `../../scripts/render-dg-broker-from-status.sh` | Status-based member/Secret/endpoint merging | Exact supplied script |

The new `../common/dataguard-lib.sh` defines:

- `sidb_dg_init_config`, `sidb_dg_write_env`, `sidb_dg_render_standby`.
- `sidb_dg_wait_rendered_spec`, `sidb_dg_generate_broker`, `sidb_dg_wait_broker`.
- `sidb_dg_assert_database`, `sidb_dg_assert_broker_spec`.
- `sidb_dg_collect_diagnostics`, `sidb_dg_exit_snapshot`.

Existing common functions remain authoritative for context-aware kubectl,
scalar validation, namespace/admin Secret setup, manifest rendering, env
persistence, workload startup failure handling and Kubernetes/Pod/alert logs.
Only two existing common files changed:

- `common/lib.sh`: render namespace and primary-reference tokens.
- `common/wait-ready.sh`: optional target/expected-role arguments and strict
  Healthy gate. Calling it with no arguments retains the test01/test02 behavior.

The old `scripts/run.sh` and legacy GitLab job are separate workflows and were
not changed to call test03.

## Run directly in the full operator repository

The ZIP contains the SIDB subtree. Use it in your existing full operator
checkout: common/lib.sh depends on the existing repository file
`.gitlab/ci/e2e-workload-health.sh`, which was not in the uploaded SIDB archive.
Use Bash 4+ on the Linux development host, with `kubectl` and `jq` installed.
The operator must watch the test namespace and support the supplied v4 fields;
both SIDB and DataguardBroker CRDs/controllers must already be installed.
The cluster must support your chosen image, storage class and LoadBalancer
Services. The generator inherits `spec.execution` from operator-rendered
status, including its execution image; it does not invent a broker runner image.

From repository root:

```bash
export CI_PROJECT_DIR="$PWD"
export SIDB_CASE_NAMESPACE=sidb-ns
export SIDB_IMAGE='your-current-working-database-image'
export SIDB_STORAGE_CLASS=oci-bv
export SIDB_PULL_SECRET=ocr-reg-cred  # omit/unset for a public image
# The registry Secret must already exist in SIDB_CASE_NAMESPACE.
# Optional: export SIDB_ADMIN_PASSWORD='your-test-password'
# Optional: export KUBE_CONTEXT_NAME='your-context'
export SIDB_TIMEOUT_SECONDS=3600

bash test/e2e/controllers/sidb/tests/03-dataguard/run.sh
```

If your checked-out directory is the `sidb` directory itself, set
`CI_PROJECT_DIR` to the full operator repository root and run:

```bash
bash tests/03-dataguard/run.sh
```

By default all resources are retained, matching the numbered case convention.
For one disposable run with automatic final cleanup:

```bash
bash test/e2e/controllers/sidb/tests/03-dataguard/run.sh --cleanup
```

Explicit cleanup after a retained run:

```bash
SIDB_TEST03_CLEANUP=true bash test/e2e/controllers/sidb/tests/03-dataguard/cleanup.sh
```

Cleanup loads the saved test03 state, waits for broker deletion, waits for
standby deletion, then delegates primary/admin Secret cleanup to common.
A dependent deletion failure stops cleanup before the primary/Secret are
removed. A namespace created by this test is removed by common cleanup;
an existing namespace is retained. The explicit `sidb/cleanup` selector
remains the existing broader dispatcher cleanup path.

Prerequisites refuse pre-existing target CRs/admin Secret and a stale test03
env file. Clean up the previous case before rerunning. To retain multiple
independent runs, use distinct names, namespaces, result directories and
`SIDB_TEST03_ENV_FILE` paths.

## Optional manual stage execution

From repository root, with the exports above:

```bash
case_dir=test/e2e/controllers/sidb/tests/03-dataguard
bash "$case_dir/prerequisites.sh"
bash "$case_dir/verify-prerequisites.sh"
source "${SIDB_TEST03_ENV_FILE:-${TMPDIR:-/tmp}/sidb-ns-test03.env}"
if [ -n "${KUBE_CONTEXT_NAME:-}" ]; then
  kubectl --context "$KUBE_CONTEXT_NAME" -n "$SIDB_CASE_NAMESPACE" apply -f "$SIDB_RENDERED_MANIFEST"
else
  kubectl -n "$SIDB_CASE_NAMESPACE" apply -f "$SIDB_RENDERED_MANIFEST"
fi
bash "$case_dir/wait-ready.sh"
bash "$case_dir/assert-topology.sh"
```

Do not apply the primary and standby together: wait-ready.sh is responsible
for applying the standby after the primary becomes Healthy, and for
applying the generated broker after the standby becomes Healthy.

## Inputs

| Variable | Default | Meaning |
| --- | --- | --- |
| `SIDB_CASE_NAMESPACE` | `SIDB_E2E_NAMESPACE`, then `E2E_NAMESPACE`, then `sidb-ns` | Test namespace; match operator watch list |
| `SIDB_TEST03_PRIMARY_NAME` | `sidb-e2e-primary-test03` | Dedicated primary CR |
| `SIDB_TEST03_PRIMARY_SID` | `PRIMDB` | Primary SID/dbUniqueName |
| `SIDB_TEST03_PRIMARY_PDB` | `PRIMPDB` | Primary spec.pdbName |
| `SIDB_TEST03_STANDBY_NAME` | `sidb-e2e-standby-test03` | Dedicated standby CR |
| `SIDB_TEST03_STANDBY_SID` | `STBYDB` | Standby SID/dbUniqueName |
| `SIDB_TEST03_STANDBY_PDB` | `STBYPDB` | Standby spec.pdbName from supplied YAML |
| `SIDB_TEST03_BROKER_NAME` | `sidb-e2e-dg-test03` | Broker CR; enforced in generated manifest |
| `SIDB_TEST03_ADMIN_SECRET` | `sidb-e2e-admin-test03` | Dedicated Secret shared by both DBs |
| `SIDB_ADMIN_SECRET_KEY` | `oracle_pwd` | Shared password key |
| `SIDB_ADMIN_PASSWORD` | Common generated temporary password | Admin password |
| `SIDB_IMAGE` | Existing common `23.26-latest` image default | Set your working image explicitly |
| `SIDB_PULL_SECRET` | Empty | Referenced on both primary and standby; field omitted when empty |
| `SIDB_IMAGE_PULL_POLICY` | `Always` | Image pull policy |
| `SIDB_STORAGE_CLASS` | `oci-bv` | Oradata storage class |
| `SIDB_TEST03_PRIMARY_STORAGE_SIZE` | `100Gi` | Primary oradata PVC size |
| `SIDB_TEST03_STANDBY_STORAGE_SIZE` | `200Gi` | Standby oradata PVC size |
| `SIDB_TIMEOUT_SECONDS` | `1500` | Readiness timeout per database; increase for slow provisioning |
| `SIDB_DG_RENDER_TIMEOUT_SECONDS` | `SIDB_TIMEOUT_SECONDS` | Timeout for standby status rendering |
| `SIDB_DG_BROKER_TIMEOUT_SECONDS` | `SIDB_TIMEOUT_SECONDS` | Broker timeout |
| `SIDB_UNHEALTHY_GRACE_SECONDS` | `600` | Unhealthy grace for SIDB/broker |
| `SIDB_POLL_INTERVAL` | `20` | Poll interval in seconds |
| `SIDB_DG_BROKER_HEALTH_FIELD` | `status` | Checks `.status.status`; set `phase` only if your actual broker CRD uses `.status.phase` |
| `SIDB_TEST03_ENV_FILE` | `/tmp/sidb-ns-test03.env` (`TMPDIR` honored) | Saved stage configuration |
| `SIDB_CASE_RESULTS_DIR` | Common numbered-case result path | Manifests and diagnostics |
| `SIDB_TEST03_PRIMARY_MANIFEST`, `SIDB_TEST03_STANDBY_MANIFEST` | This case's templates | Optional templates using the same tokens |
| `KUBE_CONTEXT_NAME`, `KUBECONFIG` | Current kubectl configuration | Selected context; generator flattens the same context into a private temporary kubeconfig |

The generated broker is JSON stored with a `.yaml` extension; kubectl accepts
it directly. This keeps validation independent of the installed `yq` variant.
The original renderer remains unchanged and can also be called directly.

## Dispatcher integration

See `../../DISPATCHER-INTEGRATION.md`. The actual unified dispatcher was not
included in the supplied archive; the selector is not registered by this ZIP
alone. The direct run.sh above is ready to use without that registration.

## Validation performed for this delivery

All bundled shell scripts passed `bash -n`. A temporary simulated-kubectl
harness exercised success, spaced standby role/named context, delayed status
rendering, failed standby, wrong standby role, failed broker, unbound PVC,
ordered cleanup and cleanup deletion failure. It verified manifest application
order and ensured failed standby checks never apply the broker. These are
workflow checks, not a real Oracle/OKE test run. Real image startup, admission
behavior, status schema, Service selectors and Data Guard health must be
validated on your operator/cluster. Final broker health defaults to the same
`.status.status` convention used by the existing SIDB runner.

The existing common setup writes the test admin Secret manifest and password
into internal test artifacts/state. Keep those results private, as described
by the existing test01 README.
