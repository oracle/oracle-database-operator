# Autonomous Database Controller E2E

Creates `adb-e2e-base` in `adb-ns`, connected to a retained SIDB.
ORDS is the external product; OrdsSrvs is the controller and custom resource.

## Run

The following CI vairables are used to run the test

```text
RUN_OPERATOR_E2E_ON_DEMAND=true
E2E_TARGET_ENV=adbenv
CI_ENVIRONMENT_NAME=adbenv
E2E_TEST_PRESET=adb/test01,adb/cleanup
```

## test01 checklist

1. Provision an ADB
2. Wait for .status.lifecycleState=AVAILABLE
3. Check if .spec.details.id populates

On failure, print CR and event diagnostics.

## Cleanup

`adb/cleanup`:

1. List ADB resources across all namespaces.
2. Run shared cleanup in the configured testcase namespace.

Cleanup deletes non-protected resources throughout the target namespace,
including Secrets and workloads. It retains the namespace and protected
RBAC/service accounts.

## Inputs

| Input | Default |
| --- | --- |
| `ADB_ADMIN_PASSWORD_SECRET_VALUE` | None; this parameter is required. |
| `ADB_COMPARTMENT_OCID` | None; this parameter is required. |
| `ADB_TEST_NAME` | `test01` |
| `ADB_E2E_NAMESPACE` | `adb-ns` |
| `ADB_OCI_CLI_PROFILE` | `adb` |
| `ADB_OCI_CONFIG_CONFIG_MAP_NAME` | `oci-cred` |
| `ADB_OCI_CONFIG_SECRET_NAME` | `oci-privatekey` |
| `ADB_ADMIN_PASSWORD_SECRET_NAME` | `adb-admin-password` |
| `ADB_ADMIN_PASSWORD_SECRET_VALUE` | None; this parameter is required. |
| `ADB_TIMEOUT_SECONDS` | `300` |

When changing `ADB_E2E_NAMESPACE`, also update `E2E_WATCH_NAMESPACES`.
