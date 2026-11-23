# PrivateAI E2E tests

These tests provision the `PrivateAi` resource and validate the generated
workload, Secrets, Services, and status. They use the stable `pai-ns`
namespace by default and retain resources after diagnostics.

## Test matrix

| Selector | What it does | Default endpoint | Details |
| --- | --- | --- | --- |
| `privateai/test01` | Provisions the documented Quick Start configuration with auth, TLS, one replica, HTTPS on 8443, default models, and ClusterIP-only access. | ClusterIP Service on 8443 | [Test README](tests/01-provision/README.md) |
| `privateai/cleanup` | Deletes PrivateAI test-owned namespaced resources while retaining the Namespace. | Not applicable | [Cleanup script](tests/cleanup/run.sh) |

## Run

Use the ordered E2E selector:

```text
E2E_TESTS=privateai/test01
```

For an on-demand GitLab pipeline, select the `privateai/test01` preset. The
test creates its auth and TLS Secrets automatically unless
`PRIVATEAI_CREATE_SECRETS=false` is supplied. To run another case afterward,
manage the order explicitly:

```text
privateai/test01,privateai/cleanup
```

See the [PrivateAI Quick Start](../../../../docs/privateai/README.md#quick-start)
for the corresponding user-facing resource model and access procedure.
