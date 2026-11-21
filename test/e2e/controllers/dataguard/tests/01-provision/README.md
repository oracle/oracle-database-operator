# Data Guard test01 — same-cluster SIDB DG

Selector `dataguard/test01` runs the existing same-cluster Data Guard lifecycle
implemented by `sidb/test03` (`test/e2e/controllers/sidb/tests/03-dataguard`).

`sidb/test03` remains a supported alias.

```text
E2E_TESTS=dataguard/test01,dataguard/cleanup
E2E_TARGET_ENV=cman1
```

Optional: `DATAGUARD_E2E_NAMESPACE` (defaults to `SIDB_E2E_NAMESPACE` / `sidb-ns`).
