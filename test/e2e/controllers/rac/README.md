# Oracle RAC controller E2E

Selectors are numbered to match the provisioning list in
[`docs/rac/README.md`](../../../docs/rac/README.md).

| Selector | Doc # | Theme | Status |
|----------|------:|-------|--------|
| `rac/test01` | 1 | Basic provision | stub |
| `rac/test02` | 2 | Security contexts | stub |
| `rac/test03` | 3 | CPU/memory resources | stub |
| `rac/test04` | 4 | Diff ASM DG | stub |
| `rac/test05` | 5 | Diff DG + redundancy | stub |
| `rac/test06` | 6 | NodePort | stub |
| `rac/test07` | 7 | Scale out | stub |
| `rac/test08` | 8 | Scale in | stub |
| `rac/test09` | 9 | ASM disk add | stub |
| `rac/test10` | 10 | ASM disk delete | stub |
| `rac/test11` | 11 | RU patch | stub |
| `rac/test12` | 12 | RU + oneoffs | stub |
| `rac/test13` | 13 | Response file | stub |
| `rac/test14` | 14 | Huge pages | stub |
| `rac/test15` | 15 | PVC software | stub |
| `rac/test16` | 16 | Encoded secrets | stub |
| `rac/test17` | 17 | **SC ASM + PVC software** | implemented |
| `rac/cleanup` | — | Test-owned RAC resource cleanup | implemented |

## Environment

Use dedicated `E2E_TARGET_ENV=sarac1` (GitLab env `e2e/sarac1`). See
[`docs/rac/e2e-sarac1.md`](../../../docs/rac/e2e-sarac1.md).

Do **not** Play these schedules until test17 (at least) is implemented.

## PVC size changes

Doc #17 documents expanding `asmStorageSizeInGb` on storage-class-backed ASM
PVCs (separate from ASM disk add #9). That day-2 check belongs inside
`rac/test17`, not a new test number.
