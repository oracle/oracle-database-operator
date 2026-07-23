# Pattern-Based ShardingDatabase Samples

These samples use your requested baseline pattern:
- `namespace: shns`
- OCI image references
- `dbSecret` with encrypted password/key fields
- `shardInfo` style topology definitions

## Apply examples

```bash
kubectl apply -f docs/examples/sharding-pattern-samples/valid/
```

## Negative tests (expected webhook rejection)

```bash
kubectl apply -f docs/examples/sharding-pattern-samples/invalid/09-invalid-user-with-shardgroup.yaml
```


## Single-source scenario

Use this when users set only top-level sharding and omit duplicate catalog fields:

```bash
kubectl apply -f docs/examples/sharding-pattern-samples/valid/12-single-source-defaults.yaml
```

Behavior:
- `catalog[].sharding` is auto-set from `spec.shardingType`
- `spec.replicationType` defaults to `DG` when omitted
- `catalog[].repl` is auto-set from effective `spec.replicationType`
