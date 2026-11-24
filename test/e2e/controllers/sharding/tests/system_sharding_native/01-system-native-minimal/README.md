# System-Managed Sharding with Native (Raft) replication

This case follows the manual `system_sharding_native` provisioning example. It
deploys one catalog database, three shards, and primary/standby GSM instances.
DBCA creates the catalog and shard databases during provisioning, so the case
uses a long timeout.

## Run

Run through the operator E2E dispatcher as `sharding/native-minimal` after
adding that selector to `.gitlab/ci/operator-e2e-run.sh` and the preset list in
`.gitlab/ci/operator-e2e-on-demand.yml`. The dispatcher defaults are
`SHARDING_E2E_NAMESPACE=gdd-ns` and `SHARDING_NAME=sdb-system`.

The Sharding runner must dispatch profile `native-minimal` to this directory.
The runner should call `prerequisites.sh`, apply its rendered manifest, then
call `wait-ready.sh` and `assert-topology.sh`. It should call `cleanup.sh` only
when cleanup is requested.

## Inputs

| Variable | Default | Purpose |
| --- | --- | --- |
| `SHARDING_E2E_NAMESPACE` | `gdd-ns` | Deployment namespace. |
| `SHARDING_NAME` | `sdb-system` | ShardingDatabase resource name. |
| `SHARDING_DB_IMAGE` | Oracle 26ai Enterprise RU image | Catalog and shard image. |
| `SHARDING_GSM_IMAGE` | Oracle 26ai GSM RU image | GSM image. |
| `SHARDING_STORAGE_CLASS` | `oci-bv` | PVC StorageClass. |
| `SHARDING_DB_PULL_SECRET` | `ocr-reg-cred` | Database image pull Secret. |
| `SHARDING_GSM_PULL_SECRET` | `ocr-reg-cred` | GSM image pull Secret. |
| `SHARDING_DB_PASSWORD` | No default | Database password as a masked GitLab CI/CD variable. The prerequisite script encrypts it and creates the Kubernetes Secret. |
| `SHARDING_DB_SECRET_NAME` | `sharding-native-e2e-db-user-pass-<pipeline-id>` | Per-pipeline Secret containing the encrypted password and generated RSA private key. |
| `SHARDING_TIMEOUT_SECONDS` | `7200` | Overall setup timeout. |
| `SHARDING_POLL_INTERVAL` | `20` | Poll interval in seconds. |
| `SHARDING_RESULTS_DIR` | `.../sharding/results/native-minimal` | Diagnostics directory. |

Expected pods are `catalog-0`, `pshard1-0` through `pshard3-0`, and `gsm1-0`
and `gsm2-0`. The test waits for the database-ready message on all four
database pods and the Catalog, Shard, and GSM completion markers in
`/var/tmp/gdd/oracle_sharding_setup.log`. Pod Ready alone is not a pass.

The sample requests approximately 12 CPU cores, 72 GiB memory, and 300 GiB
storage. Verify capacity, quota, registry access, the v4 ShardingDatabase CRD,
and operator watch scope before running.

Create the OCR pull Secret once in the test namespace using the procedure in
`docs/registry/README.md`; it should use a dedicated OCR-only Docker config.
Set `SHARDING_DB_PASSWORD` in GitLab as a masked variable (and protected when
the pipeline runs on a protected ref). The pipeline creates a temporary RSA
3072 keypair, encrypts the password using the documented OAEP/SHA-256 settings,
and creates a uniquely named DB Secret. The plaintext password and temporary
key files are removed from the runner; the Kubernetes Secret retains the
encrypted password and private key required by the operator. Secret data is
never written to diagnostics. Cleanup removes the ShardingDatabase resource
first, then removes that pipeline's DB Secret. Keep the DB Secret while the CR
exists because the operator may need it during reconciliation.
