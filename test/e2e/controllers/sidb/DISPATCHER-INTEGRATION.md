# Register sidb/test03 in the full repository

The supplied ZIP contains the SIDB subtree only. It does not contain the
unified dispatcher that registers sidb/test01 and sidb/test02. No unverified
edit to that external dispatcher is included.

Find its existing test02 registration from the full repository root:

```bash
rg -n 'sidb/test02|02-provision' .gitlab test/e2e
```

Add sidb/test03 to the same selector validation/registry and case directory
mapping, using:

| Registration value | test03 value |
| --- | --- |
| Selector | `sidb/test03` |
| Controller | `sidb` |
| Case ID | `test03` |
| Case name/directory | `03-dataguard` / `test/e2e/controllers/sidb/tests/03-dataguard` |
| Environment state | `${SIDB_TEST03_ENV_FILE:-${TMPDIR:-/tmp}/sidb-ns-test03.env}` |
| Primary template | `test03.yaml` |
| Rendered manifest applied by dispatcher | `$SIDB_RENDERED_MANIFEST` from the saved env |
| Namespace | `sidb-ns`, or the configured watched namespace |

Follow the exact existing test02 dispatch stages:

1. Run test03/prerequisites.sh.
2. Run test03/verify-prerequisites.sh.
3. Load the test03 env file and apply **only** `$SIDB_RENDERED_MANIFEST` with the configured context/namespace.
4. Run test03/wait-ready.sh; this now handles standby provisioning and broker generation/application.
5. Run test03/assert-topology.sh.
6. Retain resources according to the existing numbered-case convention.

If the dispatcher routes selectors to standalone scripts instead of individual
stages, the following branch is the complete command to call from repository
root. Adapt the enclosing variable/function name to that actual dispatcher:

```bash
sidb/test03)
  bash test/e2e/controllers/sidb/tests/03-dataguard/run.sh
  ;;
```

Do not both call run.sh and dispatch the individual lifecycle stages: choose
the path matching your existing dispatcher. Do not call legacy
`scripts/run.sh --tests dataguard` for this new numbered case.

After registration, use:

```bash
E2E_TESTS=sidb/test03
```

For an ordered sequence that clears retained resources first:

```bash
E2E_TESTS=sidb/test02,sidb/cleanup,sidb/test03
```

The existing SIDB gitlab-ci.yml invokes the legacy runner; it is unchanged.
Your unified operator job needs the selector registration above to run this
case through E2E_TESTS.
