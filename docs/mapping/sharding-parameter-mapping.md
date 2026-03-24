# Sharding Parameter Mapping Sheet

This sheet maps CR fields (camelCase) to runtime builder keys and the effective `oragsm.py` parameter strings, including sharding-type gates, replication gates, enums, defaults, and runtime-derived behavior.

## Global Normalization and Defaults

| Source | Runtime behavior |
|---|---|
| `spec.replicationType` missing | Defaults to `DG` |
| `RAFT`, `RAFTREPLICATION`, `RAFTREPLICATIN` | Normalized to native behavior (`NATIVE`) |
| `catalog[].repl` missing | Inherits effective top-level replication |
| `catalog[].sharding` missing | Inherits top-level sharding type |
| `spec.shardingType` missing | Inferred from shard/shardInfo shape (`SYSTEM` / `USER` / `COMPOSITE`) |
| `shardInfo[].shardNum` missing | Falls back to deprecated `replicas`; then defaults by flow |
| top-level vs catalog mismatch | Webhook rejects mismatched `sharding`/`repl` when both are set |

## Add Shard (`spec.shard[]`)

Builder: `BuildShardParams` / `BuildShardParamsForAdd`

| CR field | Runtime key | Gate / condition | Enum / default / notes |
|---|---|---|---|
| `name` | `shard_host`, default DB naming | required | host becomes `<name>-0.<name>` |
| `shardGroup` | `shard_group` | required in `SYSTEM`/`COMPOSITE`; forbidden in `USER` | validated in webhook |
| `shardSpace` | `shard_space` | required in `USER`/`COMPOSITE`; forbidden in `SYSTEM` | validated in webhook |
| `shardRegion` | `shard_region` | required by controller fallback checks | mode-aware in controller |
| `deployAs` | `deploy_as` (add flow only) | forbidden with `shardGroup` in `SYSTEM`/`COMPOSITE`; forbidden for native replication | enum `PRIMARY/STANDBY/ACTIVE_STANDBY`; user+DG defaults to `PRIMARY` in add-shard rule |
| `cdbName` | `cdb` | optional | mapped |
| `cloneSchemas` | `clone_schemas=true` | native-only | webhook rejects if non-native |
| `ggService` | `gg_service` | native-only | webhook rejects if non-native |
| `replace` | `replace` | native-only | webhook rejects if non-native |
| `pwd` | `pwd` | optional | mapped |
| `connect` | `connect` | optional | mapped |
| `force` | `force=true` | optional | mapped |
| `savename` | `savename=true` | optional | mapped |
| `rack` | `rack` | optional | mapped |
| `validateNetwork` | `validate_network=true` | optional | mapped |
| `cpuThreshold` | `cpu_threshold` | optional | minimum `1` |
| `diskThreshold` | `disk_threshold` | optional | minimum `1` |
| derived shard port | `shard_port` | always emitted | default `1521` when env not provided |

## Add ShardGroup (`spec.shardGroup[]` -> `SHARD<n>_GROUP_PARAMS`)

| CR field | Runtime key | Gate / condition | Enum / default / notes |
|---|---|---|---|
| `name` | `group_name` | required to emit group entry | non-empty |
| `region` | `group_region` | optional | mapped when set |
| `shardSpace` | `shardspace` | optional | useful for system/composite linking |
| `repFactor` | `repfactor` | optional | minimum `1` |
| `deployAs` | `deploy_as` | DG behavior only | enum `PRIMARY/STANDBY/ACTIVE_STANDBY`; default standby when omitted under DG |
| fallback default group | `group_name=shardgroup1;group_region=primary` (+`deploy_as=primary` only for DG) | used only when no valid group is generated and user didn't provide group env | no user override clobbering |

## Add ShardSpace (`spec.shardSpace[]` -> `SHARD<n>_SPACE_PARAMS`)

| CR field | Runtime key | Gate / condition | Enum / default / notes |
|---|---|---|---|
| `name` | `sspace_name` | required to emit space entry | non-empty |
| `chunks` | `chunks` | optional | minimum `1` |
| `repFactor` | `repfactor` | optional | minimum `1` |
| `repUnits` | `repunits` | optional | minimum `1` |
| `protectMode` | `protectedmode` | optional | enum `MAXPROTECTION/MAXAVAILABILITY/MAXPERFORMANCE` |
| sharding-type gate | n/a | emitted only in `USER`/`COMPOSITE` | builder gate |

## Create Catalog (`spec.catalog[]` -> `CATALOG_PARAMS`)

Builder: `buildCatalogParams`

| CR field | Runtime key | Gate / condition | Enum / default / notes |
|---|---|---|---|
| `name` | `catalog_host`, `catalog_db`, `catalog_pdb`, `catalog_name` | required | name length validated |
| `sharding` | `sharding_type` | optional | inherits top-level if omitted |
| `repl` | `repl_type=native` (native only) | optional | inherits top-level if omitted |
| `region[]` | `catalog_region` | optional | fallback from shard/gsm regions |
| `configname` | `shard_configname` | optional | fallback to top-level `shardConfigName` |
| `chunks` | `catalog_chunks` | not applicable for `USER` | webhook rejects for user mode |
| `repFactor` | `repl_factor` | native-only | webhook rejects for non-native |
| `repUnits` | `repl_unit` | native-only | webhook rejects for non-native |
| `autoVncr` | `autovncr` | optional | enum `on/off`; default `off` |
| `agentPort` | `agent_port` | optional | default `8080` |
| `validateNetwork` | `validate_network=true` | optional | mapped |
| `force` | `force=true` | optional | mapped |
| `gdsPool` | `gdspool` | optional | mapped |
| `protectMode` | `protectmode` | DG-oriented | webhook rejects when native replication |
| `agentPassword` | `agent_password` | optional | mapped |
| `multiwriter` | `multiwriter=true` | native-only | webhook rejects for non-native |
| `forFederatedDatabase` | `for_federated_database=true` | optional | mapped |
| `encryption` | `encryption` | optional | mapped |
| `sdb` | `sdb` | optional | mapped |
| `useExistingCatalog` | `use_existing_catalog=true` | requires `catalogDatabaseRef` | webhook enforces required ref |
| `createAs` | `create_as` | optional | mapped |
| `primaryDatabaseRef.host` | `primary_db_host` | optional | mapped |
| `primaryDatabaseRef.port` | `primary_db_port` | optional | mapped if > 0 |
| `primaryDatabaseRef.cdbName` | `primary_cdb_name` | optional | mapped |
| `primaryDatabaseRef.pdbName` | `primary_pdb_name` | optional | mapped |
| `catalogDatabaseRef.host` | `catalog_db_host` | optional | mapped |
| `catalogDatabaseRef.port` | `catalog_db_port` | optional | mapped if > 0 |
| `catalogDatabaseRef.cdbName` | `catalog_ref_cdb_name` | optional | mapped |
| `catalogDatabaseRef.pdbName` | `catalog_ref_pdb_name` | optional | mapped |

## Validation Gates Added (Webhook)

### Shard advanced gates
- `ggService`, `replace`, `cloneSchemas`: allowed only when effective replication is native.

### Catalog advanced gates
- `multiwriter`, `repFactor`, `repUnits`: allowed only for native replication.
- `protectMode`: rejected when replication is native (DG-only intent).
- `chunks`: rejected for user sharding catalog mode.
- `useExistingCatalog=true`: requires `catalogDatabaseRef`.

## Notes on Naming Conventions

- CR fields remain camelCase (existing API convention).
- Runtime builder keys remain snake_case because they target GSM/GDSCTL parameter formats and `oragsm.py` key parsing.
