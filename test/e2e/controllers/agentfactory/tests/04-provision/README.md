# Agent Factory test04

This case provisions Agent Factory with:

- a normal TCP Details database connection;
- automatic schema provisioning through the controller database setup Job;
- three runtime replicas;
- a controller-managed ReadWriteMany PVC backed by an OCI FSS CSI StorageClass;
- three distinct Ready worker nodes selected through a durable label selector;
  they may be in the same Availability Domain;
- optional configuration.certFqdn and configuration.certIpAddress, omitted from
  the rendered CR when their variables are empty;
- ClusterIP service exposure by default; when AGENTFACTORY_PUBLIC_LB_IP is
  supplied, a public LoadBalancer is enabled and the same IP is used as certIpAddress.

## Run

Use the baseline SIDB dependency and run the cases in this order:

    sidb/test01,agentfactory/test04

For an explicit on-demand run, select agentfactory/test04. The dispatcher
runs sidb/test01 first when AGENTFACTORY_REQUIRES_SIDB=true.

## Required environment-specific values

| Variable | Required | Meaning |
| --- | --- | --- |
| AGENTFACTORY_FSS_AVAILABILITY_DOMAIN | Required when creating a StorageClass | OCI Availability Domain used by the FSS CSI StorageClass. Not needed when AGENTFACTORY_FSS_STORAGE_CLASS is reused. |
| AGENTFACTORY_FSS_MOUNT_TARGET_OCID or AGENTFACTORY_FSS_MOUNT_TARGET_SUBNET_OCID | One required when creating a StorageClass | Existing OCI FSS mount target OCID, or subnet OCID from which the CSI driver can create/use a mount target. Do not set both. Not needed when AGENTFACTORY_FSS_STORAGE_CLASS is reused. |
| SIDB_ADMIN_PASSWORD or AGENTFACTORY_BOOTSTRAP_PASSWORD | Yes | SYS password used by the baseline SIDB and the Agent Factory bootstrap Secret. |
| AGENTFACTORY_WORKER_NODE_SELECTOR_KEY | No | Label key used to select eligible worker nodes. Defaults to `kubernetes.io/os`; override it for a node-pool label in your environment. |
| AGENTFACTORY_WORKER_NODE_SELECTOR_VALUE | No | Label value used with the selector key. Defaults to `linux`; at least three Ready nodes must match. |

Optional values:

| Variable | Default | Meaning |
| --- | --- | --- |
| AGENTFACTORY_CERT_FQDN | Empty | Adds configuration.certFqdn when set. |
| AGENTFACTORY_CERT_IP_ADDRESS | Empty | Adds configuration.certIpAddress when set, unless AGENTFACTORY_PUBLIC_LB_IP is supplied. |
| AGENTFACTORY_PUBLIC_LB_IP | Empty | Enables the public LoadBalancer and is used for both the Service IP and configuration.certIpAddress. |
| AGENTFACTORY_PUBLIC_LB_SUBNET | Empty | Optional OCI LoadBalancer subnet OCID. |
| AGENTFACTORY_PUBLIC_LB_EXTERNAL_TRAFFIC_POLICY | Cluster | Public Service traffic policy. |
| AGENTFACTORY_FSS_STORAGE_CLASS | Empty | Reuses an existing FSS StorageClass; otherwise a test-owned class is created dynamically. |
| AGENTFACTORY_FSS_COMPARTMENT_OCID | Empty | Optional compartment parameter for dynamic FSS provisioning. |
| AGENTFACTORY_FSS_EXPORT_PATH | Empty | Optional FSS export path; when set it must start with /. |
| AGENTFACTORY_PVC_SIZE | 100Gi | Requested managed PVC size. |

The test also requires OCI FSS CSI permissions for dynamic provisioning. The
StorageClass uses reclaimPolicy Delete and is labeled as test04-owned. A
provided AGENTFACTORY_FSS_STORAGE_CLASS is validated and reused, not deleted.
At least three Ready nodes must carry the configured worker-node label. The
controller writes the label selector into required node affinity and applies
best-effort `ScheduleAnyway` spreading across hostname labels.

## Defaults and cleanup

The case forces three replicas, Details/TCP on port 1521, automatic bootstrap
with SYS AS SYSDBA, a 100Gi ReadWriteMany PVC, DeleteOnSuccess for a successful
setup Job, and ClusterIP service exposure unless AGENTFACTORY_PUBLIC_LB_IP is
set. When that IP is set, test04 enables the public LoadBalancer and validates
that the Service and certIpAddress use the same value. The default image and
database host/service values come from the common AgentFactory E2E defaults.

The AgentFactory CR sets deletePvcOnDelete: true for its controller-managed
PVC. agentfactory/cleanup deletes the test-owned AgentFactory resources and any
test04-created StorageClass while retaining the namespace. It does not delete
a user-provided StorageClass.
