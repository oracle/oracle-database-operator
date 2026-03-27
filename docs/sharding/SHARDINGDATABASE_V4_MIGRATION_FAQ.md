# ShardingDatabase v4 Migration Strategy and FAQ

This runbook explains how to migrate `ShardingDatabase` resources from `database.oracle.com/v1alpha1` to `database.oracle.com/v4`, and how to support customers during rollout.

## Scope

This document is specific to `ShardingDatabase`.

- `v4` is the supported API for new sharding operations.
- `v1alpha1` is deprecated.
- Admission webhook checks for sharding are enforced through `v4`.

## Migration Strategy

Use a phased rollout to reduce risk.

### Phase 1: Discover and prepare

1. Scan repos and templates for `ShardingDatabase` `v1alpha1` manifests.
2. Update all generators/charts/pipelines to emit `database.oracle.com/v4`.
3. Share deprecation notice and migration deadline.

Repo scan example:

```bash
rg -n 'apiVersion:\s*database\.oracle\.com/v1alpha1|kind:\s*ShardingDatabase' <repo-path>
```

### Phase 2: Soft migration (recommended when customers need time)

1. Keep `v1alpha1` served.
2. Mark `v1alpha1` as deprecated with warning in CRD.
3. Validate all new customer submissions are `v4`.

### Phase 3: Hard cutover

1. Set `v1alpha1 served: false` for `shardingdatabases.database.oracle.com`.
2. Keep `v4 served: true` and `storage: true`.
3. Remove `v1alpha1` webhook registration for `ShardingDatabase`.

### Phase 4: Post-cutover guardrails

1. Block merges introducing `v1alpha1` ShardingDatabase manifests.
2. Add CI validation for server-side dry-run against `v4`.
3. Monitor admission failures and share fast remediation steps.

## Customer Migration Steps

1. Change:

```yaml
apiVersion: database.oracle.com/v1alpha1
kind: ShardingDatabase
```

to:

```yaml
apiVersion: database.oracle.com/v4
kind: ShardingDatabase
```

2. Re-apply manifests via existing deployment pipeline.
3. Validate object status/events after apply.

## Bulk Rewrite Script (YAML Repos)

```bash
#!/usr/bin/env bash
set -euo pipefail

ROOT="${1:-.}"

echo "Scanning for ShardingDatabase v1alpha1 manifests under: $ROOT"
mapfile -t files < <(rg -l --glob '*.{yaml,yml}' \
  'apiVersion:[[:space:]]*database\.oracle\.com/v1alpha1' "$ROOT" | tr -d '\r')

if [ "${#files[@]}" -eq 0 ]; then
  echo "No v1alpha1 manifests found."
  exit 0
fi

changed=0
for f in "${files[@]}"; do
  if rg -q 'kind:[[:space:]]*ShardingDatabase' "$f"; then
    cp "$f" "$f.bak_sharding_v1alpha1"
    sed -i 's#apiVersion:[[:space:]]*database\.oracle\.com/v1alpha1#apiVersion: database.oracle.com/v4#g' "$f"
    echo "Updated: $f"
    changed=$((changed+1))
  fi
done

echo "Done. ShardingDatabase files updated: $changed"
echo "Backups created as: *.bak_sharding_v1alpha1"
```

## Cluster Validation Commands

Check CRD version state:

```bash
kubectl get crd shardingdatabases.database.oracle.com -o jsonpath='{range .spec.versions[*]}{.name}{" served="}{.served}{" storage="}{.storage}{" deprecated="}{.deprecated}{"\n"}{end}'
```

Expected after hard cutover:
- `v1alpha1 served=false deprecated=true`
- `v4 served=true storage=true`

Check resources and apiVersion:

```bash
kubectl get shardingdatabases -A -o custom-columns='NAMESPACE:.metadata.namespace,NAME:.metadata.name,APIVERSION:.apiVersion'
```

## FAQ

### 1) Why do I see `no matches for kind "ShardingDatabase" in version "database.oracle.com/v1alpha1"`?

`v1alpha1` is no longer served. Update manifests to `database.oracle.com/v4`.

### 2) Will existing ShardingDatabase objects be deleted during cutover?

No. Existing objects remain. Failures happen when clients still submit `v1alpha1`.

### 3) How do I identify remaining `v1alpha1` usage quickly?

```bash
rg -n 'apiVersion:\s*database\.oracle\.com/v1alpha1|kind:\s*ShardingDatabase' <repo-path>
```

### 4) What if a customer is blocked right after cutover?

1. Confirm they are applying `v4` manifests.
2. Check admission error details from `kubectl apply` output/events.
3. If emergency rollback is required, temporarily restore a build where `v1alpha1 served=true`.

### 5) Are all operator resources now v4-only?

No. This runbook is only for `ShardingDatabase`.

### 6) What should support request from customers for triage?

1. Manifest used for apply.
2. Full apply error output.
3. `kubectl get shardingdatabases -A -o yaml` (or object excerpt).
4. Operator controller logs around the failure timestamp.

### 7) Recommended pre-cutover gate

1. Zero `v1alpha1` ShardingDatabase manifests in supported repos.
2. CI/generator templates emit only `v4`.
3. `kubectl apply --dry-run=server` succeeds for customer `v4` manifests.
