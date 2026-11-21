# True Cache E2E cases

True Cache follows the same `test01` / `test02` security split as SIDB and
Agent Factory. Its templates live under the standalone `truecache` controller
directory, outside the SIDB directory:

| Selector | Security posture | Defaults |
| --- | --- | --- |
| `truecache/test01` | TDE day-0, baseline TCP path, **no TCPS** | `sidb-e2e-tc-primary`, `sidb-e2e-truecache` |
| `truecache/test02` | TDE day-0 + **day-0 TCPS** | `sidb-e2e-tc-primary-test02`, `sidb-e2e-truecache-test02` |

The implementation directories are `tests/01-tcp` and `tests/02-tcps`.
The old `sidb/truecache/*` selectors remain compatibility aliases.

Both cases provision a True Cache-capable primary, wait for the generated
blob ConfigMap, provision a True Cache consumer, and assert Healthy /
`TRUE_CACHE` plus PVC and Service presence.

## Examples

Baseline weekly-style run:

```text
E2E_TESTS=sidb/test01,truecache/test01,truecache/cleanup,sidb/cleanup
```

Secure TCPS + TDE True Cache path:

```text
E2E_TESTS=truecache/test02,truecache/cleanup
```

Both cases use `SIDB_TRUECACHE_IMAGE` when set, then `SIDB_IMAGE` when set;
otherwise they default to `23.26-latest` in the shared PHX registry. Each database uses 60 GiB
on `SIDB_STORAGE_CLASS` (default `oci-bv`).
