# Ordered controller E2E cases

The unified operator E2E job accepts an ordered, comma-separated sequence in
`E2E_TESTS`. The runner executes entries exactly in the order supplied; it does
not infer or reorder controller dependencies.

For example:

```
sidb/test01,agentfactory/test01,sidb/cleanup,agentfactory/cleanup,agentfactory/test02
```

`controller/cleanup` is an explicit test case. It deletes test-owned
namespaced resources from that controller's configured namespace, waits for
deletion, and keeps the Namespace object. Kubernetes system objects and
operator-required namespace RBAC are retained so the operator can continue
reconciling. Cleanup fails if a non-protected resource remains.

Normal test cases retain their resources after diagnostics are collected. Each
case produces a controller/test archive under the pipeline log directory.
Those resources remain available for debugging until a later cleanup case
deletes them. Set `E2E_TEST_PRESET=custom` and pass the sequence in
`E2E_TESTS` when starting an on-demand pipeline.

SIDB test01 provisions a baseline primary, then patches TCPS onto the ready
database so Agent Factory test01 can copy the client wallet and connect on
port 2484:

sidb/test01,agentfactory/test01

SIDB test02 provisions a separate primary with TDE, day-0 TCPS, 2 CPU/8 GiB
resource requests and limits, 3 GiB SGA, 1 GiB PGA, 1000 processes, and a live
PDB MAX_STRING_SIZE=EXTENDED check. To run it with the wallet-based Agent
Factory case, use:

sidb/test02,agentfactory/test02

True Cache uses standalone TCP/TCPS cases with checked-in YAML templates:

truecache/test01,truecache/test02

Use `truecache/cleanup` after either case when the shared `sidb-ns` namespace
must be emptied of True Cache resources.

On-demand presets for the two wallet paths (trigger separately):

sidb/test01,agentfactory/test01,sidb/cleanup,agentfactory/cleanup

sidb/test02,agentfactory/test02,sidb/cleanup,agentfactory/cleanup

For a weekly/scheduled orchestrated run that cleans up afterward, use the
on-demand preset:

sidb/test02,agentfactory/test02,agentfactory/cleanup,sidb/cleanup

That sequence is also the `weekly-agentfactory-test02-e2e` job when
`RUN_OPERATOR_E2E_ORCHESTRATOR=true`.

Agent Factory test03 uses the baseline SIDB test01 database with a normal TCP
Details connection and validates that `certIpAddress` matches a pre-allocated
public LoadBalancer IP. Run it with:

sidb/test01,agentfactory/test03

Agent Factory test04 uses the baseline SIDB test01 database with automatic
schema bootstrap, three TCP Details replicas, and a controller-managed RWX OCI
FSS PVC. It selects three distinct Ready nodes through a configurable label
selector by default and keeps certificate fields optional. Run it with:

sidb/test01,agentfactory/test04


PrivateAI test01 provisions the documented Quick Start configuration with
authentication, TLS, one replica, HTTPS on port 8443, default image models,
and ClusterIP-only access. Run it with:

privateai/test01

The per-case diagnostics include namespaced resource YAML, workload and custom
resource descriptions, timestamp-sorted events, current and previous logs for
all Pod containers including init containers, and operator Deployment/Pod
diagnostics. Secret data is never collected; only Secret metadata is recorded.
