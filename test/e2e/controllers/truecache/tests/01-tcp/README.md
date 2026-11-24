# True Cache test01: TCP baseline

Baseline True Cache path aligned with `sidb/test01` security posture:

- TDE enabled at day-0 (required for True Cache)
- TCPS **not** enabled

Selector: `truecache/test01`

```text
E2E_TESTS=truecache/test01,truecache/cleanup
```

Resource defaults: `sidb-e2e-tc-primary`, `sidb-e2e-truecache`.
Templates: [`primary.yaml`](primary.yaml) and [`truecache.yaml`](truecache.yaml).
Shared runner: [`../common/run.sh`](../common/run.sh).
