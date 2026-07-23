# Oracle Connection Manager (CMAN) TrafficManager samples

Focused YAML examples for **Oracle Database Operator** `TrafficManager` CMAN mode on Kubernetes. Use these manifests to deploy Oracle Connection Manager, route client SQL\*Net traffic to SIDB or RAC listeners, and configure generated rules, global `next_hop`, or service-alias file-mode routing.

**Do not put every pattern in one mega YAML.** Each file is a single valid, apply-ready scenario. For decision guidance, read the [CMAN configuration pattern guide](../README.md#choosing-a-cman-configuration-pattern).

## Canonical templates (deep reference)

| Template | When to open it |
| --- | --- |
| [config/samples/trafficmanager/cman-sidb-peer-filemode.yaml](../../../config/samples/trafficmanager/cman-sidb-peer-filemode.yaml) | Multi-backend file mode, multi-replica CMAN, service-aware aliases; richest comments |
| [config/samples/trafficmanager/cman-sidb-filemode.yaml](../../../config/samples/trafficmanager/cman-sidb-filemode.yaml) | Single-backend user-managed `cman.ora` |

## Focused samples (copy and apply)

| Use case | Sample | Mode |
| --- | --- | --- |
| One SIDB, operator-generated CMAN rules | [cman-sidb.yaml](cman-sidb.yaml) | Generated |
| Two or more SIDBs, generated rules (`dst=*`) | [cman-sidb-peer.yaml](cman-sidb-peer.yaml) | Generated |
| Two or more SIDBs, generated rules (explicit `dst`) | [cman-sidb-default.yaml](cman-sidb-default.yaml) | Generated |
| One SIDB, global `next_hop` | [cman-sidb-nexthop.yaml](cman-sidb-nexthop.yaml) | File |
| One SIDB, plain user-managed `cman.ora` | [cman-sidb-filemode.yaml](cman-sidb-filemode.yaml) | File |
| Two or more SIDBs, service-alias `next_hop` | [cman-sidb-peer-nexthop.yaml](cman-sidb-peer-nexthop.yaml) | File |
| One RAC, generated rules + `remote_listener` | [cman-rac.yaml](cman-rac.yaml) | Generated |
| One RAC SCAN, global `next_hop` | [cman-rac-nexthop.yaml](cman-rac-nexthop.yaml) | File |
| Standalone `cman.ora` snippet | [cman-sidb-filemode.cman.ora](cman-sidb-filemode.cman.ora) | File fragment |

## How to choose a sample

1. Open the [pattern table](../README.md#choosing-a-cman-configuration-pattern).
2. Pick the row that matches backends (SIDB vs RAC), ownership of `cman.ora` (generated vs file), and routing (`remote_listener`, global `next_hop`, or service-alias).
3. Copy the matching file from this directory (or a canonical template above).
4. Replace `<cman-container-image>`, subnet OCIDs, namespaces, Service hostnames, and database service names.

## Apply from the repository root

```bash
kubectl apply -f docs/trafficmanager/samples/<sample-file>.yaml
```

Deploy every backend database and confirm its listener Service is reachable **before** applying multi-backend CMAN samples.

## Related documentation

- [Traffic Manager CMAN guide](../README.md) — prerequisites, fields, verification, troubleshooting
- [Oracle Database Operator README](../../../README.md)
- [SIDB documentation](../../sidb/README.md)
- [RAC documentation](../../rac/README.md)

## Keywords

Oracle Connection Manager, CMAN, TrafficManager, `network.oracle.com/v4`, Oracle Database Operator, Kubernetes, SIDB, RAC, SCAN, `cman.ora`, `next_hop`, `remote_listener`, Easy Connect, SQL\*Net proxy.
