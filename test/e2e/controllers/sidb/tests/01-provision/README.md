# SIDB test01

SIDB test01 provisions the baseline primary database used by Agent Factory
test01. It creates the sidb-e2e-primary SIDB with the default Oracle image and
storage class, waits for the database to become ready, then patches
`spec.security.tcps` onto the ready primary so TCPS listens on port 2484 and a
client wallet is generated. Assertions validate the SIDB status, role, Pod,
PVC, Service (including port 2484), image, TCP connect string, TCPS connect
string, and client wallet location.

Day-0 `test01.yaml` stays TCP-only. The optional `enable-tcps.sh` stage is what
turns TCPS on after readiness. The test creates the admin Secret required by
the SIDB resource and a TLS Secret for the later TCPS patch. The generated
admin password is retained in the internal test diagnostics archive, so
protect the archive.

## Run

Run it through the ordered operator E2E dispatcher:

~~~
E2E_TESTS=sidb/test01
~~~

If Agent Factory test01 will consume this database over TCPS/wallet, keep the
dependency in the sequence:

~~~
E2E_TESTS=sidb/test01,agentfactory/test01
~~~

Resources are retained after diagnostics. Run sidb/cleanup explicitly before
another test that needs an empty sidb-ns.

## Inputs and defaults

| Variable | Default | Purpose |
| --- | --- | --- |
| SIDB_CASE_NAMESPACE | SIDB_E2E_NAMESPACE, otherwise sidb-ns | Namespace for this case. |
| SIDB_TEST_NAME | sidb-e2e-primary | SIDB name used by the lifecycle and assertions. The checked-in test01 template uses this same name. |
| SIDB_TEST_SID | ORCL1 | SID value used by the lifecycle. |
| SIDB_TEST_PDB | ORCLPDB1 | PDB value used by the lifecycle. |
| SIDB_IMAGE | <registry>/oracle/database:<tag> | Oracle database image. |
| SIDB_PULL_SECRET | Empty; image pull secret field is omitted | Registry Secret for a private image. |
| SIDB_STORAGE_CLASS | oci-bv | Storage class for the 100 GiB oradata PVC. |
| SIDB_ADMIN_PASSWORD | Temporary generated value | Password stored under oracle_pwd in sidb-e2e-admin. |
| SIDB_ADMIN_SECRET | sidb-e2e-admin | Admin Secret name. The default template references this name. |
| SIDB_ADMIN_SECRET_KEY | oracle_pwd | Admin Secret key. |
| SIDB_TCPS_TLS_SECRET | sidb-e2e-tcps-tls-test01 | TLS Secret used by the post-ready TCPS patch. |
| SIDB_IMAGE_PULL_POLICY | IfNotPresent | Image pull policy. |
| SIDB_REPLICAS | 1 | Number of SIDB replicas. |
| SIDB_TIMEOUT_SECONDS | 1500 | Overall readiness and TCPS enable timeout. |
| SIDB_UNHEALTHY_GRACE_SECONDS | 600 | Grace period for a workload failure before the test fails. |
| SIDB_POLL_INTERVAL | 20 seconds | Readiness polling interval. |
| SIDB_CASE_PIPELINE | CI_PIPELINE_ID, otherwise local | Pipeline label written to the rendered resource. |
| SIDB_CASE_MANIFEST | tests/01-provision/test01.yaml | Optional replacement template when using a custom manifest. |

The standard test01 template fixes the SID, PDB, database name, charset,
edition, 100 GiB size, ReadWriteOnce, one replica, archive logging, and
MAX_STRING_SIZE=EXTENDED. To change those fixed values, provide a custom
SIDB_CASE_MANIFEST and keep the lifecycle variables aligned with it.

Live PDB `MAX_STRING_SIZE=EXTENDED` verification belongs to sidb/test02.

## Outputs

The case archive contains the rendered manifest, source manifest, generated
admin and TLS Secret YAML, the TCPS patch payload, resource YAML, workload
descriptions, sorted events, and current/previous Pod and operator logs.
Secret data is included in the explicit internal test artifacts; do not
publish the archive.
