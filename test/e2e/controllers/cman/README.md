# CMAN controller E2E

Uses **`ConnectionManager`** (`network.oracle.com/v4`). Do not create new
`TrafficManager` resources with `spec.type: cman` — that path is deprecated.

| Selector | Scope |
| --- | --- |
| `cman/test01` | SIDB primary + ConnectionManager file-mode next_hop (TCP) |
| `cman/test02` | SIDB TCPS+TDE + ConnectionManager next_hop to TCPS :2484 |
| `cman/cleanup` | Namespace cleanup for `cman-ns` |

Target env for Weekly Combined: **`cman1`** (new OKE; local tunnel example
`ssh -N -L 6452:<BASTION_TARGET_PRIVATE_IP>:6443 bastion`).
