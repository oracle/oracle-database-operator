# Agent Factory test03

Test03 validates the standard TCP `Details` database connection together with
certificate IP configuration for a pre-allocated public OCI LoadBalancer IP.
It creates fresh runtime and administrator Secrets, applies the AgentFactory
resource, and validates the Deployment, database setup, PVC, ClusterIP
Service, public LoadBalancer, and readiness status.

The value supplied through `AGENTFACTORY_PUBLIC_LB_IP` is used in both places:

- `spec.configuration.certIpAddress`
- `spec.networking.service.publicLoadBalancer.ip`

The assertions verify that the AgentFactory certificate IP, generated Service
requested IP, and assigned LoadBalancer IP match the same pre-allocated IP.
The test does not use a Wallet connection or wallet preparation.

## Run

Use the ordered dependency sequence:

```text
E2E_TESTS=sidb/test01,agentfactory/test03
```

The database must be a ready SIDB primary with the `sidb-e2e-primary` name by
default. To run AgentFactory independently, set
`AGENTFACTORY_REQUIRES_SIDB=false` and provide the database connection and
administrator inputs.

## Inputs and defaults

| Variable | Default | Purpose |
| --- | --- | --- |
| `AGENTFACTORY_E2E_NAMESPACE` | `paf-ns` | AgentFactory namespace. |
| `AGENTFACTORY_NAME` | `agentfactory-e2e-test03` | AgentFactory resource name. |
| `AGENTFACTORY_RUNTIME_SECRET` | `agentfactory-runtime-secret-test03` | Runtime Secret. |
| `AGENTFACTORY_BOOTSTRAP_SECRET` | `paf-db-bootstrap-admin-test03` | Temporary administrator Secret. |
| `AGENTFACTORY_DB_CONNECTION_TYPE` | `Details` | Normal database connection mode. |
| `AGENTFACTORY_DB_HOST` | `sidb-e2e-primary.sidb-ns.svc.cluster.local` | SIDB service host. |
| `AGENTFACTORY_DB_SERVICE_NAME` | `ORCLPDB1` | Target PDB service. |
| `AGENTFACTORY_DB_PORT` | `1521` | TCP database port. |
| `AGENTFACTORY_DB_PROTOCOL` | `TCP` | Details protocol. |
| `AGENTFACTORY_PUBLIC_LB_ENABLED` | `true` | Enables the public LoadBalancer Service. |
| `AGENTFACTORY_PUBLIC_LB_IP` | Required | Pre-allocated public LoadBalancer IP and certificate IP. |
| `AGENTFACTORY_PUBLIC_LB_SUBNET` | Empty | Optional OCI LoadBalancer subnet OCID. |
| `AGENTFACTORY_PUBLIC_LB_EXTERNAL_TRAFFIC_POLICY` | `Cluster` | Public Service traffic policy. |
| `AGENTFACTORY_CERT_FQDN` | Empty | Optional certificate FQDN. |
| `AGENTFACTORY_BOOTSTRAP_USERNAME` | `SYS` | Database administrator username. |
| `AGENTFACTORY_BOOTSTRAP_PRIVILEGE` | `SYSDBA` | Separate database privilege. |
| `AGENTFACTORY_BOOTSTRAP_PASSWORD` | `SIDB_ADMIN_PASSWORD` | Administrator password. |
| `AGENTFACTORY_BOOTSTRAP_CLEANUP_POLICY` | `DeleteOnSuccess` | Deletes the bootstrap Secret after successful setup. |
| `AGENTFACTORY_BOOTSTRAP_PRESERVE_FAILED_JOB` | `true` | Retains failed setup Pod diagnostics. |
| `AGENTFACTORY_PVC_NAME` | `agentfactory-e2e-test03-pvc` | Existing PVC claim. |

`AGENTFACTORY_PUBLIC_LB_IP` must be a valid pre-allocated IP. The test fails
before provisioning when this value is missing, and fails its assertions when
the configured certificate IP or generated LoadBalancer does not use it.

## Expected result

The AgentFactory phase becomes `Ready`, the database setup Job succeeds, the
runtime Deployment and PVC are healthy, the normal TCP Details connection is
reported, and the public LoadBalancer uses the supplied pre-allocated IP.
The rendered resource must contain `certIpAddress` with the same value as the
LoadBalancer IP.
