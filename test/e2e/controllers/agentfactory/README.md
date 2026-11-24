# Oracle AI Database Private Agent Factory E2E tests

These numbered tests provision the Agent Factory CR and validate the
controller-created Deployment, Services, PVC, database state, and readiness
status. The tests use the stable paf-ns namespace by default and retain
resources after diagnostics.

Each test directory contains its case-specific wrapper, manifest, database
preparation, and `assert.sh`. The shared lifecycle, readiness polling,
cleanup, and helper library are kept in `tests/common/`; each wrapper supplies
its own assertion script to the common runner.

## Test matrix

| Selector | What it does | Database mode | Default dependency | Details |
| --- | --- | --- | --- | --- |
| agentfactory/test01 | Copies the SIDB client wallet after sidb/test01 patches TCPS, then connects on port 2484 with controller bootstrap. | Wallet when SIDB TCPS is ready; otherwise Details/password fallback | sidb/test01 when AGENTFACTORY_REQUIRES_SIDB=true | [Test README](tests/01-provision/README.md) |
| agentfactory/test02 | Retrieves the SIDB TCPS wallet, lets the controller Job create/validate the PAF schema, patches the runtime Secret, and validates wallet mode. | Wallet when SIDB TCPS is ready; otherwise Details/password fallback | sidb/test02 when AGENTFACTORY_REQUIRES_SIDB=true | [Test README](tests/02-provision/README.md) |
| agentfactory/test03 | Provisions Agent Factory with the normal TCP Details connection and validates `certIpAddress` against a pre-allocated public LoadBalancer IP. | Details | sidb/test01 when AGENTFACTORY_REQUIRES_SIDB=true | [Test README](tests/03-provision/README.md) |
| agentfactory/test04 | Provisions three TCP Details replicas with automatic schema bootstrap and a controller-managed RWX OCI FSS PVC, then validates label-based placement across three worker nodes. | Details | sidb/test01 when AGENTFACTORY_REQUIRES_SIDB=true | [Test README](tests/04-provision/README.md) |
| agentfactory/cleanup | Deletes Agent Factory test-owned resources while retaining the Namespace. | Not applicable | None | [Cleanup script](tests/cleanup/run.sh) |

Dependencies are not inferred when E2E_TESTS is supplied. Use an explicit
ordered sequence such as:

~~~
sidb/test01,agentfactory/test03
~~~

On-demand presets (trigger separately):

~~~
sidb/test01,agentfactory/test01,sidb/cleanup,agentfactory/cleanup
sidb/test02,agentfactory/test02,sidb/cleanup,agentfactory/cleanup
~~~

## Common inputs and defaults

| Variable | Default | Purpose |
| --- | --- | --- |
| AGENTFACTORY_E2E_NAMESPACE | paf-ns | Agent Factory namespace. |
| AGENTFACTORY_NAME | agentfactory-e2e-test01, agentfactory-e2e-test02, agentfactory-e2e-test03, or agentfactory-e2e-test04 | AgentFactory resource name. |
| AGENTFACTORY_IMAGE | AGENT_FACTORY_IMAGE, then AGENTFACTORY_CONTAINER_IMAGE, then container-registry.oracle.com/database/applied-ai:latest | Runtime image. |
| AGENTFACTORY_PULL_SECRET | Empty; field is omitted | Private registry pull Secret. |
| AGENTFACTORY_DB_USERNAME | paf_user | Runtime database user. |
| AGENTFACTORY_DB_HOST | sidb-e2e-primary.sidb-ns.svc.cluster.local | Database service host. |
| AGENTFACTORY_DB_SERVICE_NAME | SIDB_TEST_PDB, otherwise ORCLPDB1 | Oracle service/PDB name. |
| AGENTFACTORY_DB_PORT | 1521 | Database port. |
| AGENTFACTORY_DB_PROTOCOL | TCP | Details connection protocol. |
| AGENTFACTORY_DB_AIR_GAPPED | false | Agent Factory database air-gapped setting. |
| AGENTFACTORY_UI_ADMIN_USERNAME | admin@example.com | Agent Factory UI administrator. |
| AGENTFACTORY_UI_ADMIN_PASSWORD_VALUE | AgentFactory-E2e-Aa1 | UI admin password stored in the runtime Secret. |
| AGENTFACTORY_DB_PASSWORD | AgentFactoryDb-E2e-Aa1 | PAF database password stored in the runtime Secret. |
| AGENTFACTORY_RUNTIME_SECRET | agentfactory-runtime-secret for test01 | Runtime Secret name. Numbered tests use case-specific names. |
| AGENTFACTORY_UI_ADMIN_PASSWORD_KEY | paf_ui_admin_password | UI password Secret key. |
| AGENTFACTORY_DB_PASSWORD_KEY | paf_db_password | Database password Secret key. |
| AGENTFACTORY_PVC_NAME | agentfactory-e2e-pvc for test01 | Existing PVC claim name. Numbered tests use case-specific names. |
| AGENTFACTORY_PVC_SIZE | 100Gi | PVC size when the case creates it. |
| AGENTFACTORY_STORAGE_CLASS | SIDB_STORAGE_CLASS, otherwise empty | PVC storage class. |
| AGENTFACTORY_PUBLIC_LB_ENABLED | true for test01/test02/test03; false for test04 unless an IP is supplied | Whether the public LoadBalancer Service is requested. |
| AGENTFACTORY_PUBLIC_LB_IP | Empty; required by test03 | Reserved public IP; test03 and test04 use it as `certIpAddress` when supplied. |
| AGENTFACTORY_PUBLIC_LB_SUBNET | Empty | Optional OCI Load Balancer subnet OCID. |
| AGENTFACTORY_PUBLIC_LB_EXTERNAL_TRAFFIC_POLICY | Cluster | Public Service traffic policy. |
| AGENTFACTORY_CERT_FQDN | Empty | Optional certificate FQDN. |
| AGENTFACTORY_CERT_IP_ADDRESS | Empty | Optional certificate IP; test04 derives it from `AGENTFACTORY_PUBLIC_LB_IP` when supplied. |
| AGENTFACTORY_WORKER_NODE_SELECTOR_KEY | `kubernetes.io/os` for test04 | Test04 label key used to select eligible worker nodes. |
| AGENTFACTORY_WORKER_NODE_SELECTOR_VALUE | `linux` for test04 | Test04 label value; at least three Ready nodes must match the selector. |
| AGENTFACTORY_FSS_AVAILABILITY_DOMAIN | Empty | Test04 OCI FSS StorageClass Availability Domain. |
| AGENTFACTORY_FSS_MOUNT_TARGET_OCID | Empty | Test04 FSS mount target OCID; mutually exclusive with subnet OCID. |
| AGENTFACTORY_FSS_MOUNT_TARGET_SUBNET_OCID | Empty | Test04 FSS mount target subnet OCID; mutually exclusive with mount target OCID. |
| AGENTFACTORY_FSS_STORAGE_CLASS | Empty | Test04 existing FSS StorageClass to reuse; otherwise a test-owned class is created. |
| AGENTFACTORY_TIMEOUT_SECONDS | 2400 | Agent Factory readiness timeout. |
| AGENTFACTORY_POLL_INTERVAL | 15 seconds | Readiness polling interval. |

When AGENTFACTORY_PUBLIC_LB_IP, AGENTFACTORY_PUBLIC_LB_SUBNET, or
AGENTFACTORY_CERT_FQDN is empty, the corresponding optional manifest field is
omitted. For test04, supplying AGENTFACTORY_PUBLIC_LB_IP automatically enables
the public LoadBalancer and uses the same value for `certIpAddress`. Set
AGENTFACTORY_PUBLIC_LB_SUBNET when the OCI cluster does not provide a default

## Outbound TLS trust

For an HTTPS LLM, embedding, REST, or MCP endpoint signed by a private CA,
create a Secret containing a CA bundle and reference it from the AgentFactory
resource. For example:

~~~sh
kubectl create secret generic agentfactory-outbound-ca -n paf \\
  --from-file=ca.crt=/path/to/private-ca-bundle.pem
~~~

~~~yaml
spec:
  security:
    outboundTLS:
      caBundle:
        secretName: agentfactory-outbound-ca
        key: ca.crt
~~~

The `key` defaults to `ca.crt`. The Secret can contain a PEM bundle with one
or more CA certificates; it must not contain a server private key or an mTLS
client key. The controller validates that the Secret and key exist, mounts the
bundle at `/run/agentfactory/outbound-ca/<key>`, and exposes that path through
`AGENT_FACTORY_OUTBOUND_CA_BUNDLE`. For the current
`private_ai_services` provider, it also maps the same security-owned path to
the image's internal `AGENT_FACTORY_LLM_PRIVATE_AI_SERVICES_CA_FILE` setting;
users no longer configure `configuration.llm.privateAiServices.caFile`. The
current operator scope is CA trust only; outbound mTLS client certificates and
private keys are not part of the AgentFactory API.

This CA-bundle delivery is distinct from Agent Factory certificate registration.
The controller mounts the referenced CA bundle into the container and, after
the Deployment is ready, registers the bundle through
`/agentFactory/v1/certs` using the existing Agent Factory administrator
credentials from `spec.security.secret`. The import accepts the API's `200 OK`
or `201 Created` response, is safe to repeat because Agent Factory skips
duplicate certificates, and must succeed before the AgentFactory resource is
reported Ready. The controller does not call the global `/certs/reset` endpoint
automatically because that endpoint removes all administrator-added outbound
certificates, including certificates not managed by this AgentFactory resource.
The controller reaches the API through the in-cluster ClusterIP Service, so
`networking.service.cluster.enabled` must remain enabled when this feature is
configured. The flow remains CA-only and does not require an outbound client
certificate or private key.

Check registration status with:

~~~sh
kubectl get agentfactory <name> -n <namespace> -o jsonpath='{range .status.conditions[*]}{.type}={.status} reason={.reason} message={.message}{"\n"}{end}'
~~~

Successful registration reports `OutboundTLSReady=True` with reason `Imported`.
Until registration succeeds, the workload may be running but the AgentFactory
resource is not reported Ready.

## Diagnostics

Each case archive includes the rendered and source manifest, generated Secret
manifests, workload YAML, conditions, descriptions, sorted events, current and
previous Pod/container logs, and operator logs. Internal archives also contain
the generated credentials and wallet artifacts; restrict their retention and
access.
