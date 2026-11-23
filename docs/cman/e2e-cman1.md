# CMAN E2E environment `cman1`

Dedicated OKE for CMAN / Data Guard controller E2E.

## Local tunnel

```bash
ssh -N -L 6452:<BASTION_TARGET_PRIVATE_IP>:6443 bastion
# Point kubeconfig server at https://127.0.0.1:6452
```

## GitLab CI variables (environment scope `e2e/cman1`)

Configure the same Bastion / kubeconfig pattern used by `e2e/qaoke`, with:

| Variable | Example |
| --- | --- |
| `BASTION_TARGET_PRIVATE_IP` | `<BASTION_TARGET_PRIVATE_IP>` |
| `BASTION_TARGET_PORT` | `6443` |
| `BASTION_LOCAL_PORT` | `6452` (or another free port on the runner) |
| Bastion SSH key / OCID / kubeconfig | same family as other `e2e/*` scopes |

Resource lock name: `kubernetes-cman1`.

## Selectors

- `cman/test01,cman/cleanup`
- `cman/test02,cman/cleanup`
- `dataguard/test01,dataguard/cleanup`
- `dataguard/test02,dataguard/cleanup`

API: use **`ConnectionManager`**. Do not create new `TrafficManager` with `type: cman`.
