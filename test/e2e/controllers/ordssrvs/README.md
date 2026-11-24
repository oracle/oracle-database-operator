# OrdsSrvs E2E

Creates `ordssrvs-e2e-base` in `ordssrvs-ns`, connected to a retained SIDB.
ORDS is the external product; OrdsSrvs is the controller and custom resource.

## Run

The operator must watch the testcase namespace. SIDB must run first.

```text
E2E_TEST_PRESET=custom
E2E_TESTS=sidb/test01,ordssrvs/test01,ordssrvs/cleanup,sidb/cleanup
```

To retain resources, omit the cleanup entries and set the shared cleanup
policy accordingly. `ordssrvs/01` is an alias for `ordssrvs/test01`.

## test01 checklist

1. Read SIDB configuration, readiness, PDB connection string and admin Secret reference.
2. Create the testcase namespace if missing.
3. Prepare `ordssrvs-auth`: copy the SIDB admin credential to `adminAuth`;
   generate `dbAuth` once and reuse it on reruns. Passwords are not logged.
4. Server dry-run and apply the CR: one `default` pool, `autoUpgradeORDS=true`,
   SYS admin credentials and the default Kubernetes service account.
5. Wait for `Healthy`, `Available=True`, a Ready Pod and healthy pools:
   **1 total / 1 OK / 0 failed**.
6. Print full init-container logs and the last 100 main-container lines,
   surrounded by `=` separators. Log-read failures are reported without failing the case.
7. Check HTTPS on port **8443**, with `Host: localhost` and `--insecure`:
   - `/favicon.ico`: **HTTP 200**, nonempty content.
   - `/ords/`: **2xx**, or **3xx with a redirect destination**.
   - Print status, download size and redirect destination.

On failure, print CR, Pod/Service and event diagnostics. HTTPS checks establish
endpoint reachability; they do not verify certificate trust or database-backed REST queries.

## Cleanup

`ordssrvs/cleanup`:

1. List OrdsSrvs resources across all namespaces.
2. Run shared cleanup in the configured testcase namespace.
3. List OrdsSrvs resources across all namespaces again and report success/failure.

Cleanup deletes non-protected resources throughout the target namespace,
including Secrets and workloads. It retains the namespace and protected
RBAC/service accounts. Use a dedicated testcase namespace.

## Inputs

| Input | Default |
| --- | --- |
| `SIDB_E2E_NAMESPACE` | `sidb-ns`; `SIDB_CASE_NAMESPACE` takes precedence |
| `SIDB_TEST_NAME` | `sidb-e2e-primary` |
| `ORDSSRVS_E2E_NAMESPACE` | `ordssrvs-ns` |
| `ORDSSRVS_TEST_NAME` | `ordssrvs-e2e-base` |
| `ORDSSRVS_AUTH_SECRET` | `ordssrvs-auth` |
| `ORDS_IMAGE` | `container-registry.oracle.com/database/ords:latest` |
| `ORDSSRVS_TIMEOUT_SECONDS` | 900 seconds per readiness wait |
| `ORDSSRVS_CURL_ATTEMPTS` | 5 attempts; 10-second request timeout, 2-second retry interval |

`ORDSSRVS_IMAGE` and `ORDSIMG` remain fallback image inputs.
When changing `ORDSSRVS_E2E_NAMESPACE`, also update `E2E_WATCH_NAMESPACES`.
Requires `kubectl`, `openssl`, and curl in the ORDS main container.
