# Agent Factory test01

Test01 validates the Agent Factory Wallet/TCPS path against SIDB test01. After
sidb/test01 patches TCPS onto the ready primary, this case synchronizes the
SIDB admin Secret, copies the SIDB client wallet from the database Pod,
patches the wallet into the runtime Secret, applies the Wallet connection
manifest, and waits for the controller `paf-db-setup` Job, Deployment,
database status, Services, and PVC to become healthy.

The preparation hook reuses the shared wallet packaging from test02. It makes
the wallet portable by rewriting `tnsnames.ora` `HOST` to the SIDB Service
FQDN and setting `sqlnet.ora` `WALLET_LOCATION` `DIRECTORY` to `.` before the
controller Job relocates it again.

## Run

~~~
E2E_TESTS=sidb/test01,agentfactory/test01
~~~

Run agentfactory/test01 alone only when the target database already has TCPS
enabled with a client wallet, or set AGENTFACTORY_REQUIRES_SIDB=false and
provide equivalent wallet inputs.

## Inputs and defaults

| Variable | Default | Purpose |
| --- | --- | --- |
| AGENTFACTORY_E2E_NAMESPACE | paf-ns | Namespace. |
| AGENTFACTORY_NAME | agentfactory-e2e-test01 | AgentFactory resource name. |
| AGENTFACTORY_IMAGE | container-registry.oracle.com/database/applied-ai:latest | Runtime image. |
| AGENTFACTORY_PULL_SECRET | Empty | Optional image pull Secret. |
| AGENTFACTORY_DB_CONNECTION_TYPE | Details initially; Wallet after preparation | Database connection mode. |
| AGENTFACTORY_DB_USERNAME | paf_user | Runtime database user. |
| AGENTFACTORY_DB_HOST | sidb-e2e-primary.sidb-ns.svc.cluster.local | SIDB Service FQDN rewritten into the wallet. |
| AGENTFACTORY_DB_SERVICE_NAME | ORCLPDB1 | PDB service used only for Details fallback. |
| AGENTFACTORY_DB_PORT | 1521 | Details fallback port. Wallet mode uses the TCPS alias on 2484. |
| AGENTFACTORY_DB_PROTOCOL | TCP | Details fallback protocol. |
| AGENTFACTORY_DB_HEALTH_PATH | /agentFactory/v1/getAppState | Readiness endpoint used by the controller. |
| AGENTFACTORY_UI_ADMIN_USERNAME | admin@example.com | UI administrator. |
| AGENTFACTORY_UI_ADMIN_PASSWORD_VALUE | AgentFactory-E2e-Aa1 | UI password. |
| AGENTFACTORY_DB_PASSWORD | AgentFactoryDb-E2e-Aa1 | PAF database password. |
| AGENTFACTORY_BOOTSTRAP_PASSWORD | Empty for the normal SIDB dependency flow | Optional fallback; preparation synchronizes from the SIDB admin Secret. |
| AGENTFACTORY_BOOTSTRAP_USERNAME | SYS | Administrator username. |
| AGENTFACTORY_BOOTSTRAP_PRIVILEGE | SYSDBA | SYSDBA or NORMAL; SYSDBA is separate from the username. |
| AGENTFACTORY_BOOTSTRAP_ENABLED | true | Enables controller-side schema bootstrap. |
| AGENTFACTORY_BOOTSTRAP_CLEANUP_POLICY | DeleteOnSuccess | Deletes the temporary administrator Secret after the Job succeeds. |
| AGENTFACTORY_BOOTSTRAP_PRESERVE_FAILED_JOB | true for test01 | Keeps a failed bootstrap Pod for SQL*Plus diagnostics. |
| AGENTFACTORY_WALLET_ZIP_KEY | db_wallet.zip | Runtime Secret key containing the wallet archive. |
| AGENTFACTORY_WALLET_TEMPLATE | tests/01-provision/test01-wallet.yaml | Wallet AgentFactory manifest selected after preparation. |
| AGENTFACTORY_SIDB_WALLET_ALIAS | Empty; discovered from tnsnames.ora | Optional explicit wallet TNS alias. |
| AGENTFACTORY_RUNTIME_SECRET | agentfactory-runtime-secret | Runtime Secret. |
| AGENTFACTORY_BOOTSTRAP_SECRET | paf-db-bootstrap-admin | Temporary/admin Secret. |
| AGENTFACTORY_PVC_NAME | agentfactory-e2e-pvc | Existing PVC claim. |
| AGENTFACTORY_PVC_SIZE | 100Gi | PVC size when created. |
| AGENTFACTORY_STORAGE_CLASS | SIDB_STORAGE_CLASS, otherwise empty | PVC storage class. |
| AGENTFACTORY_PUBLIC_LB_ENABLED | true | Public LoadBalancer Service. |
| AGENTFACTORY_PUBLIC_LB_IP | Empty | Optional reserved public IP. |
| AGENTFACTORY_PUBLIC_LB_SUBNET | Empty | Optional OCI subnet OCID. |
| AGENTFACTORY_PUBLIC_LB_EXTERNAL_TRAFFIC_POLICY | Cluster | Public Service policy. |
| AGENTFACTORY_TIMEOUT_SECONDS | 2400 | Readiness timeout. |
| AGENTFACTORY_POLL_INTERVAL | 15 seconds | Readiness polling interval. |

The initial Details template remains `tests/01-provision/test01.yaml` for
fallback rendering. When wallet preparation succeeds, the runner switches to
`test01-wallet.yaml`.

## Expected result

The AgentFactory phase becomes Ready with `connectionType=Wallet`, the
database setup Job succeeds, the runtime Secret contains the wallet zip, SIDB
reports `isTcpsEnabled=true` with a TCPS connect string on port 2484, and the
public Service/PVC assertions pass.
