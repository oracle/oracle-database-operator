# CMAN test01 — SIDB + ConnectionManager

Single-cluster smoke for the CMAN controller using **`kind: ConnectionManager`**
(`TrafficManager` `type: cman` is deprecated on master).

## What it does

1. Provisions a SIDB primary in `CMAN_E2E_NAMESPACE` (default `cman-ns`).
2. Applies a file-mode ConnectionManager with global `next_hop` to the SIDB Service.
3. Asserts ConnectionManager `Healthy`, Deployment ready, and NodePort external Service.

File-mode next_hop does **not** require database `remote_listener` registration.

## Selector

```text
E2E_TESTS=cman/test01,cman/cleanup
E2E_TARGET_ENV=cman1
```

## Key env vars

| Variable | Default | Purpose |
| --- | --- | --- |
| `CMAN_E2E_NAMESPACE` | `cman-ns` | Target namespace |
| `CMAN_E2E_IMAGE` | `…/client-cman:RDBMS_23.26.3…TRY1` | CMAN container image. For 19c use `…/client-cman:19.33` with SIDB `:128786`. |
| `SIDB_IMAGE` | `…/database:23.26.0-ee-261014` | SIDB image (align with CMAN version: `:128786` for 19c) |
| `CMAN_E2E_NODE_PORT` | `30189` | External NodePort |
| `SIDB_STORAGE_CLASS` | `oci-bv` | PVC storage class |
