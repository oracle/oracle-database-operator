# True Cache test02: secure TCPS

Secure True Cache path aligned with `sidb/test02` security posture:

- TDE enabled at day-0
- Operator-managed TCPS enabled at day-0 on the primary and True Cache

Selector: `truecache/test02`

```text
E2E_TESTS=truecache/test02,truecache/cleanup
```

Resource defaults: `sidb-e2e-tc-primary-test02`, `sidb-e2e-truecache-test02`,
TLS Secret `sidb-e2e-tc-tcps-tls-test02`.
Shared runner: [`../common/run.sh`](../common/run.sh).
Templates: [`primary.yaml`](primary.yaml) and [`truecache.yaml`](truecache.yaml).
