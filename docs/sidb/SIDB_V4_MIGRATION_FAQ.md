# SingleInstanceDatabase v4 Migration and Support FAQ

This runbook explains how to migrate `SingleInstanceDatabase` (SIDB) resources from `database.oracle.com/v1alpha1` to `database.oracle.com/v4`, and how to troubleshoot common migration issues.

## Scope

This document is specific to SIDB resources.

- SIDB `v4` is the supported API.
- SIDB `v1alpha1` is deprecated.
- SIDB webhook defaulting and validation are enforced in `v4`.

This does not imply every other operator resource is `v4`-only.

## Customer Migration Steps

1. Update SIDB manifests from:

```yaml
apiVersion: database.oracle.com/v1alpha1
kind: SingleInstanceDatabase
```

to:

```yaml
apiVersion: database.oracle.com/v4
kind: SingleInstanceDatabase
```

2. Update all automation that applies SIDB resources:
- GitOps repos
- CI/CD pipelines
- Helm charts / templates
- Generated YAML from internal tooling

3. Re-apply SIDB resources using `v4`.

4. Validate cluster state:

```bash
kubectl get singleinstancedatabases -A -o custom-columns='NAMESPACE:.metadata.namespace,NAME:.metadata.name,APIVERSION:.apiVersion'
```

5. Validate CRD version settings:

```bash
kubectl get crd singleinstancedatabases.database.oracle.com -o jsonpath='{range .spec.versions[*]}{.name}{" served="}{.served}{" storage="}{.storage}{" deprecated="}{.deprecated}{"\n"}{end}'
```

Expected:
- `v1alpha1 served=false deprecated=true`
- `v4 served=true storage=true`

## Bulk Rewrite Script (Repository YAML)

Use this script to rewrite SIDB manifests in a repo.

```bash
#!/usr/bin/env bash
set -euo pipefail

ROOT="${1:-.}"

echo "Scanning for SIDB v1alpha1 manifests under: $ROOT"
mapfile -t files < <(rg -l --glob '*.{yaml,yml}' \
  'apiVersion:[[:space:]]*database\.oracle\.com/v1alpha1' "$ROOT" | tr -d '\r')

if [ "${#files[@]}" -eq 0 ]; then
  echo "No v1alpha1 manifests found."
  exit 0
fi

changed=0
for f in "${files[@]}"; do
  if rg -q 'kind:[[:space:]]*SingleInstanceDatabase' "$f"; then
    cp "$f" "$f.bak_sidb_v1alpha1"
    sed -i 's#apiVersion:[[:space:]]*database\.oracle\.com/v1alpha1#apiVersion: database.oracle.com/v4#g' "$f"
    echo "Updated: $f"
    changed=$((changed+1))
  fi
done

echo "Done. SIDB files updated: $changed"
echo "Backups created as: *.bak_sidb_v1alpha1"
```

## Support FAQ

### 1) Error: `no matches for kind "SingleInstanceDatabase" in version "database.oracle.com/v1alpha1"`

Cause:
- Cluster no longer serves SIDB `v1alpha1`.

Resolution:
- Change manifest API version to `database.oracle.com/v4`.
- Re-apply resource.

### 2) Are existing SIDB databases deleted after cutover?

No. Existing resources remain. The failure is for new/updated requests that still use `v1alpha1`.

### 3) How to find remaining `v1alpha1` SIDB files in a repo?

```bash
rg -n 'apiVersion:\s*database\.oracle\.com/v1alpha1|kind:\s*SingleInstanceDatabase' <repo-path>
```

### 4) `v4` apply fails with webhook validation errors

Cause:
- `v4` admission validation is enforced.

Resolution:
- Fix fields reported in the validation error message.
- Re-apply.

### 5) How to confirm SIDB cutover status in-cluster?

```bash
kubectl get crd singleinstancedatabases.database.oracle.com -o yaml
kubectl get mutatingwebhookconfiguration,validatingwebhookconfiguration -o yaml
```

Look for:
- SIDB CRD `v1alpha1` with `served: false` and `deprecated: true`
- SIDB webhook rules targeting `v4` paths

### 6) Are all operator webhooks `v4`-only?

No. This runbook is only for SIDB. Other resources may still support older API versions.

### 7) Rollback plan if customer workloads are blocked

1. Re-deploy the prior operator/CRD bundle where SIDB `v1alpha1 served=true`.
2. Keep migration guidance active for customers.
3. Reattempt cutover after all customer manifests are upgraded to `v4`.

### 8) Recommended cutover gate

1. Repo scans show zero SIDB `v1alpha1` manifests.
2. CI/CD generators produce only SIDB `v4`.
3. Server-side dry-run for customer manifests succeeds:

```bash
kubectl apply --dry-run=server -f <manifest-or-directory>
```
