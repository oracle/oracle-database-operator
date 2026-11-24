# SIDB test02

SIDB test02 provisions an independent secure primary for the wallet-based
Agent Factory test. It enables operator-managed TCPS and TDE, generates the
test TCPS TLS Secret when it does not already exist, and validates that the
database exposes the expected security and resource settings.

The manifest sets 2 CPU and 8 GiB requests and limits, a 3 GiB SGA, a 1 GiB
PGA, 1000 processes, and MAX_STRING_SIZE=EXTENDED. The default resource name
is sidb-e2e-primary-test02, so test01 and test02 can be run in the same
pipeline after an explicit cleanup.

## Run

~~~
E2E_TESTS=sidb/test02
~~~

For the Agent Factory wallet flow, run the dependency sequence:

~~~
E2E_TESTS=sidb/test02,agentfactory/test02
~~~

## Inputs and defaults

| Variable | Default | Purpose |
| --- | --- | --- |
| SIDB_CASE_NAMESPACE | SIDB_E2E_NAMESPACE, otherwise sidb-ns | Namespace for this case. |
| SIDB_TEST02_NAME | sidb-e2e-primary-test02 | SIDB resource name. |
| SIDB_TEST_SID | ORCL1 | SID value. |
| SIDB_TEST_PDB | ORCLPDB1 | PDB name and service name. |
| SIDB_IMAGE | <registry>/oracle/database:<tag> | Oracle database image. |
| SIDB_PULL_SECRET | Empty; field is omitted | Registry Secret for a private image. |
| SIDB_STORAGE_CLASS | oci-bv | Storage class for the oradata PVC. |
| SIDB_STORAGE_SIZE | 100Gi | Oradata PVC size. |
| SIDB_ACCESS_MODE | ReadWriteOnce | Oradata PVC access mode. |
| SIDB_EDITION | enterprise | Database edition. |
| SIDB_CHARSET | AL32UTF8 | Database character set. |
| SIDB_ADMIN_PASSWORD | Temporary generated value | SIDB admin password. |
| SIDB_ADMIN_SECRET | sidb-e2e-admin | Admin Secret name. |
| SIDB_ADMIN_SECRET_KEY | oracle_pwd | Admin Secret key. |
| SIDB_TDE_SECRET | sidb-e2e-tde-test02 | TDE wallet password Secret. |
| SIDB_TDE_SECRET_KEY | tde_wallet_pwd | TDE Secret key. |
| SIDB_TDE_PASSWORD | E2e_Tde_<pipeline>_Aa1 | TDE wallet password. |
| SIDB_TCPS_TLS_SECRET | sidb-e2e-tcps-tls-test02 | TLS Secret used by SIDB TCPS. |
| SIDB_IMAGE_PULL_POLICY | IfNotPresent | Image pull policy. |
| SIDB_TIMEOUT_SECONDS | 1500 | Overall readiness timeout. |
| SIDB_UNHEALTHY_GRACE_SECONDS | 600 | Workload failure grace period. |
| SIDB_POLL_INTERVAL | 20 seconds | Readiness polling interval. |
| SIDB_TEST02_MANIFEST | tests/02-provision/test02.yaml | Optional replacement template. |

The following values are intentionally fixed by the test02 template:
requests/limits 2 CPU and 8Gi, SGA 3072 MB, PGA 1024 MB, processes 1000,
TCPS enabled, TDE enabled, archive logging enabled, and
MAX_STRING_SIZE=EXTENDED.

Assertions check both the CR `pdbParameters` value and the live PDB
`v$parameter` value for `max_string_size`. The live SQL check is the manager
requirement that previously failed when only the CR field was inspected.

## Outputs and handoff

The case archives the generated admin, TDE, and TCPS Secret manifests, source
and rendered SIDB YAML, the live MAX_STRING_SIZE query output, and all
diagnostics. Agent Factory test02 reads the SIDB status and Pod wallet,
connects to the PDB through TCPS, and uses the resulting wallet for schema
preparation.
