# Sharding E2E Layout

This folder contains manifest-driven end-to-end tests for the Sharding controller.

## Profiles

- `smoke/manifest-order.txt`: fast validation flow for on-demand/scheduled checks.
- `full/manifest-order.txt`: extended flow; add update/delete scenarios here over time.

Each profile file lists manifests in strict apply order.

## Scripts

- `scripts/run.sh`: renders templates, applies manifests in sequence, and runs assertions.
- `scripts/assert-system.sh`: waits for `ShardingDatabase` success state and validates basic resources.

## Template Variables

`scripts/run.sh` supports CI overrides and falls back to defaults:

- `DB_IMAGE`
- `GSM_IMAGE`
- `SHARDING_STORAGE_CLASS`
- `SHARDING_SCRIPTS_URL`
- `SHARDING_DB_SECRET`
- `SHARDING_DB_PWD_KEY`
- `SHARDING_DB_PRIV_KEY`
