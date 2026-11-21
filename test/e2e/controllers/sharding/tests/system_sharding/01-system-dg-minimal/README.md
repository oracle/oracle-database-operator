# System-Managed Sharding with Data Guard replication

This case follows the manual `system_sharding` provisioning example. It
deploys one catalog database, two primary shards, and primary/standby GSM
instances with `shardingType: SYSTEM` and `replicationType: DG`. DBCA creates
the catalog and shard databases during provisioning, so the case uses a long
timeout.

## Run

Run through the operator E2E dispatcher using
`E2E_TEST_PRESET=sharding/dg-minimal`. The dispatcher defaults are
`SHARDING_E2E_NAMESPACE=gdd-ns` and `SHARDING_NAME=sdb-system`.

The Sharding runner dispatches profile `dg-minimal` to this directory.
It calls `prerequisites.sh`, applies the rendered manifest, then calls
`wait-ready.sh` and `assert-topology.sh`. Cleanup follows
`E2E_CLEANUP_POLICY`; use `pre` to clear the earlier test deployment and
retain the current deployment for inspection.

## Inputs

| Variable | Default / value | Purpose |
| --- | --- | --- |
| `SHARDING_E2E_NAMESPACE` | `gdd-ns` | Deployment namespace. |
| `SHARDING_NAME` | `sdb-system` | ShardingDatabase resource name. |
| `USE_19c_IMAGES` | `false` | Set to `true` to select the 19c image pair. |
| `SHARDING_DB_IMAGE` | Configured CI/CD value | Catalog and shard image when `USE_19c_IMAGES=false`. |
| `SHARDING_GSM_IMAGE` | Configured CI/CD value | GSM image when `USE_19c_IMAGES=false`. |
| `SHARDING_DB_19c_IMAGE` | Configured CI/CD value | Catalog and shard image when `USE_19c_IMAGES=true`. |
| `SHARDING_GSM_19c_IMAGE` | Configured CI/CD value | GSM image when `USE_19c_IMAGES=true`. |
| `SHARDING_STORAGE_CLASS` | `oci-bv` | PVC StorageClass. |
| `SHARDING_DB_PULL_SECRET` | `ocr-reg-cred` | Database image pull Secret. |
| `SHARDING_GSM_PULL_SECRET` | `ocr-reg-cred` | GSM image pull Secret. |
| `SHARDING_DB_PASSWORD` | No default | Database password as a masked GitLab CI/CD variable. |
| `SHARDING_DG_DB_SECRET_NAME` | `sharding-dg-e2e-db-user-pass-<pipeline-id>-<job-id>` | Per-job Secret containing the encrypted password and RSA private key. |
| `SHARDING_TIMEOUT_SECONDS` | `7200` | Database and GSM setup timeout in seconds. |
| `SHARDING_POLL_INTERVAL` | `20` | Poll interval in seconds. |
| `SHARDING_RESULTS_DIR` | `.../sharding/results/dg-minimal` | Diagnostics directory; overridden by the dispatcher. |

Expected pods are `catalog-0`, `pshard1-0`, `pshard2-0`, `gsm1-0`, and
`gsm2-0`. The test waits for the database-ready message on all three
database pods and the Catalog, Shard, and GSM completion markers in
`/var/tmp/gdd/oracle_sharding_setup.log`. Pod Ready alone is not a pass.

The sample requests approximately 10 CPU cores, 56 GiB memory, and 250 GiB
storage. Verify capacity, quota, registry access, the v4 ShardingDatabase CRD,
and operator watch scope before running.

Create the OCR pull Secret once in the test namespace using the procedure in
`docs/registry/README.md`. Set `SHARDING_DB_PASSWORD` in GitLab as a masked
variable, with expansion disabled, and protected when using a protected ref.
The pipeline generates an RSA-3072 keypair, encrypts the password using
OAEP/SHA-256 with MGF1-SHA-256, and creates the per-job DB Secret. Temporary
plaintext and key files are removed from the runner. The Kubernetes Secret
retains `pwdfile.enc` and `key.pem`; keep it while the deployment exists.
Cleanup removes the ShardingDatabase and its dependent resources before
removing the test's DB Secret. The OCR pull Secret is retained.
