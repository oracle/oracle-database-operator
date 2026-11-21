# PrivateAI test01

This test provisions the minimal PrivateAI Services Container described in the
[PrivateAI Quick Start](../../../../../../docs/privateai/README.md#quick-start):

- one runtime replica;
- authentication enabled with an auth Secret;
- TLS enabled with a TLS Secret;
- HTTPS listener on port `8443`;
- an in-cluster `ClusterIP` Service on port `8443`;
- no model ConfigMap, so the image's default pre-approved models are used;
- no public or private LoadBalancer.

The test creates the Quick Start-compatible auth and TLS Secrets in the test
namespace by default. It generates an API key, PKCS#12 password, self-signed
certificate, private key, and `keystore.p12`. Set
`PRIVATEAI_CREATE_SECRETS=false` to use existing Secrets instead.

## Run

Use the generic E2E selector:

```text
E2E_TESTS=privateai/test01
```

The on-demand GitLab preset is `privateai/test01`. The default namespace is
`pai-ns`, and the default resource is `privateai-e2e-service`.

## Inputs and defaults

| Variable | Default | Purpose |
| --- | --- | --- |
| `PRIVATEAI_E2E_NAMESPACE` | `pai-ns` | Namespace for the test resource and Secrets. |
| `PRIVATEAI_NAME` | `privateai-e2e-service` | PrivateAI resource and ClusterIP Service name. |
| `PRIVATEAI_IMAGE` | `PRIVATE_AI_IMAGE`, `PAI_IMAGE`, or `<registry>/oracle/database:<privateai-tag>` | PrivateAI runtime image. |
| `PRIVATEAI_PULL_SECRET` | Empty | Optional image pull Secret; the manifest omits the field when empty. |
| `PRIVATEAI_AUTH_SECRET` | `paisecret` | Auth Secret containing `api-key` and `privateai-ssl-pwd`. |
| `PRIVATEAI_TLS_SECRET` | `privateai-tls` | TLS Secret containing `tls.crt`, `tls.key`, and `keystore.p12`. |
| `PRIVATEAI_CREATE_SECRETS` | `true` | Generate and apply test Secrets; set `false` to use existing Secrets. |
| `PRIVATEAI_API_KEY` | Generated | API key written to the auth Secret. |
| `PRIVATEAI_SSL_PASSWORD` | Generated | PKCS#12 password written to `privateai-ssl-pwd`. |
| `PRIVATEAI_TLS_COMMON_NAME` | `<name>.<namespace>.svc` | Certificate common name and DNS SAN. |
| `PRIVATEAI_TIMEOUT_SECONDS` | `1800` | Readiness timeout. |
| `PRIVATEAI_POLL_INTERVAL` | `15` | Readiness polling interval. |

The generated Secrets are test inputs and their values are never printed by
the test. Diagnostics record only resource metadata and do not archive Secret
data.

## Assertions

The test waits for `status.status=Healthy`, one available and ready
Deployment replica, a Ready Pod, the expected Quick Start security fields,
the generated auth/TLS Secret keys, the `ClusterIP` Service on `8443`, and
non-empty Service endpoints. It also verifies that no public LoadBalancer
Service is created.

Resources are retained after diagnostics by the common E2E lifecycle. Use the
explicit `privateai/cleanup` entry before another test when the namespace must
be emptied.
