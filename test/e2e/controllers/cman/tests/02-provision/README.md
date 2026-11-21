# cman/test02 — SIDB TCPS+TDE + ConnectionManager next_hop TCPS

Provisions a SIDB primary with TDE and TCPS (ClusterIP TCP 1521 + TCPS 2484),
then a ConnectionManager whose file-mode `next_hop` targets the SIDB TCPS
port (`protocol=tcps`, port 2484).

CMAN listener remains TCP :1521. Full CMAN TCPS documentation is deferred
until this e2e is green.

## Run

```bash
E2E_TESTS=cman/test02,cman/cleanup
E2E_TARGET_ENV=cman1
```

## Defaults

| Variable | Default |
|----------|---------|
| `CMAN_E2E_NAMESPACE` | `cman-ns` |
| `CMAN_E2E_IMAGE` | `…/client-cman:RDBMS_23.26.3…TRY1` (use `…/client-cman:19.33` with SIDB `:128786`) |
| `SIDB_IMAGE` | align with CMAN version (`23.26-latest` or `:128786`) |
| `CMAN_E2E_TCPS_SIDB_NAME` | `sidb-e2e-cman-tcps` |
| `CMAN_E2E_TCPS_NAME` | `cman-e2e-tcps` |
| `CMAN_E2E_TCPS_NODE_PORT` | `30190` |
