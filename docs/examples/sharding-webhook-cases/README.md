# Sharding Webhook Example Matrix

These manifests are designed to exercise the webhook rules for:
- sharding mode (`USER`, `SYSTEM`, `COMPOSITE`)
- replication type (`DG`, `NATIVE`)
- `spec.shard[]` field gating (`shardGroup`, `shardSpace`, `deployAs`)
- schema validations (catalog flags/ranges/enums)

## Usage

1. Create required secret first (same namespace as your CR):

```bash
kubectl -n default create secret generic db-secret \
  --from-literal=dbpwd='Welcome_12345' \
  --dry-run=client -o yaml | kubectl apply -f -
```

2. Apply valid examples:

```bash
kubectl apply -f docs/examples/sharding-webhook-cases/valid/
```

3. Apply invalid examples one-by-one (expected rejection):

```bash
kubectl apply -f docs/examples/sharding-webhook-cases/invalid/11-user-with-shardgroup-invalid.yaml
```

## Expected result groups

- `valid/*.yaml`: accepted by webhook validation.
- `invalid/*.yaml`: rejected with field-level validation errors.

## Notes

- These are validation examples, not full production deployment topologies.
- `dbImage`/`gsmImage` are placeholders and can be replaced.
- Names are unique so files can be applied independently.

## Automated invalid-case run

Run all invalid manifests and verify each one is rejected by webhook:

```bash
./docs/examples/sharding-webhook-cases/run-invalid-cases.sh default
```

Optional dry run:

```bash
DRY_RUN=true ./docs/examples/sharding-webhook-cases/run-invalid-cases.sh default
```
