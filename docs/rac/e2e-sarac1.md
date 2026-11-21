# RAC E2E environment `sarac1`

Dedicated OKE for Oracle RAC controller E2E (PVC / storage-class path).

## Local tunnel

Fill Bastion / API endpoint details when the cluster is ready (same pattern as
`docs/cman/e2e-cman1.md` and `e2e/saoke`):

```bash
ssh -N -L <BASTION_LOCAL_PORT>:<BASTION_TARGET_PRIVATE_IP>:<BASTION_TARGET_PORT> bastion
# Point kubeconfig server at https://127.0.0.1:<BASTION_LOCAL_PORT>
```

## GitLab CI variables (environment scope `e2e/sarac1`)

Configure the same Bastion / kubeconfig / OCI pattern used by `e2e/saoke` and
`e2e/cman1`:

| Variable | Notes |
| --- | --- |
| `BASTION_TARGET_PRIVATE_IP` | RAC OKE API private IP |
| `BASTION_TARGET_PORT` | usually `6443` |
| `BASTION_LOCAL_PORT` | free port on the runner |
| `OCI_CONFIG_FILE` / `OCI_API_PRIVATE_KEY_FILE` | File variables |
| `KUBE_CONTEXT_FILE` / `KUBE_CONTEXT_NAME` | File / string |

Resource lock name: `kubernetes-sarac1`.

## Selectors (stubs until implemented)

- `rac/test17` — Storage Class ASM + PVC software (**first**)
- `rac/test03` — resources
- `rac/test09` / `rac/test10` — ASM add / delete
- `rac/test11` / `rac/test12` — RU / RU+oneoffs
- `rac/cleanup`

Full map: `test/e2e/controllers/rac/README.md`.

## Manual setup (out of repo)

Create protected GitLab environment `e2e/sarac1` and attach the variables above
before any Play. No RAC schedule is created until test17 is green.
