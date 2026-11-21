# Oracle Restart provisioning E2E

## Selector numbering (docs 1:1)

Selectors follow the provisioning list in
[`docs/oraclerestart/README.md`](../../../docs/oraclerestart/README.md) — the
same rule as RAC (`rac/test0N` = doc #N).

| Selector | Doc # | Theme | Status |
|----------|------:|-------|--------|
| `oraclerestart/test01` | 1 | Provision | **implemented** |
| `oraclerestart/test05` | 5 | Memory/CPU | future |
| `oraclerestart/test06` | 6 | SW storage size | future |
| `oraclerestart/test07` | 7 | Custom storage class | future |
| `oraclerestart/test08` | 8 | RU on filesystem | future |
| `oraclerestart/test09` | 9 | RU on existing PVC | future |
| `oraclerestart/test10` | 10 | RU + oneoffs + SC | future |
| `oraclerestart/test14` | 14 | ASM disk add | future |
| `oraclerestart/test15` | 15 | ASM disk delete | future |
| … | 2–4, 11–13, 16 | remaining README items | future |

Standalone GitLab schedule: **SIDB Oracle Restart test01 weekly** (Play-ready;
Combined also runs this case first).

---

This first case implements the documented “1. Provisioning an Oracle Restart
Database” flow. It is intentionally isolated from DBCS and uses the dedicated
Oracle Restart E2E namespace. All cluster, kubeconfig, tunnel, OCI, and Bastion
values are supplied by the selected GitLab environment and are not stored in
the repository.

The case creates the documented DB, SSH, and TDE secrets, renders and
server-validates the simple provisioning YAML, waits for `AVAILABLE` with
`dbState=OPEN` and `role=PRIMARY`, verifies the pod is Ready, then deletes the
OracleRestart CR and waits for the CR to disappear. An EXIT trap performs the
same deletion after failures.

The case uses those six `ORESTART_*_FILE` values when supplied. Otherwise it
copies the documented prerequisite secrets from `ORESTART_SECRET_SOURCE_NAMESPACE`
(`orestart` by default) into the dedicated test namespace. Only the copied test
secrets are deleted; the source namespace and its existing database are never
modified.
The worker node is discovered from the target cluster at runtime unless
`ORESTART_WORKER_NODE` is explicitly set. This prevents the documented
manifest's node affinity from referring to a node in a different OKE cluster.
ASM uses fresh OCI Block Volume provisioning through `oci-bv` with
`asmStorageSizeInGb: 50`. This avoids wiping host devices that may belong to
an existing Oracle Restart database; newly provisioned volumes are clean for
the installation.
The target worker's cached 19.28 software is staged from
`/scratch/software/19c/1928GoldImage` using `grid_home.zip` and `db_home.zip`.
The E2E image is the fully qualified
`<registry>/oracle/database-rac:<tag>` image used by the
healthy target deployment.

In CI, select this case through the generic environment workflow with
`RUN_OPERATOR_E2E_ON_DEMAND=true`, `E2E_TARGET_ENV=saoke`, and
`E2E_TEST_PRESET=oraclerestart/test01`. The shared OCI and tunnel helpers
consume the protected `e2e/saoke` variables (`OCI_CONFIG_FILE`,
`OCI_API_PRIVATE_KEY_FILE`, `KUBE_CONTEXT_FILE`, `KUBE_CONTEXT_NAME`, and the
`BASTION_*` variables). Their values, including cluster and host details, must
remain in GitLab CI/CD settings rather than this repository.

For local execution, supply the same environment-scoped kubeconfig and tunnel
variables used by CI, then source the shared context and tunnel helpers. Do
not add those values to this repository.

```bash
source .gitlab/ci/oci-context.sh
source .gitlab/ci/oci-tunnel.sh
test/e2e/controllers/oraclerestart/tests/01-provision/prerequisites.sh
```

Then run `run.sh` with the same environment. Use only the selected Oracle
Restart E2E environment; do not point this case at another cluster.
