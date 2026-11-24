## Scale In an Existing Private AI Deployment

Reduce the replica count of an existing `PrivateAi` resource. Before scaling in, confirm that the remaining replicas can handle the workload and continue to access the configured models.

Set the namespace and name of the existing `PrivateAi` resource:

```sh
export NAMESPACE=pai
export NAME=pai-sample
```

### Reduce the Replica Count

This example reduces the desired replica count from three to two. Adjust `2` to the required total:

```sh
kubectl patch pai "$NAME" -n "$NAMESPACE" --type=merge \
  -p '{"spec":{"runtime":{"replicas":2}}}'
```

### Check the Result

Wait for the Private AI resource to report `Healthy` and for the Deployment rollout to complete:

```sh
kubectl wait --for=jsonpath='{.status.status}'=Healthy "pai/$NAME" \
  -n "$NAMESPACE" --timeout=15m

kubectl rollout status "deployment/$NAME" \
  -n "$NAMESPACE" --timeout=15m
```

Check the desired and observed replica counts, then list the Pods for this deployment:

```sh
kubectl get pai "$NAME" -n "$NAMESPACE" \
  -o jsonpath='desired={.spec.runtime.replicas}{"\n"}observed={.status.replicas}{"\n"}status={.status.status}{"\n"}'

kubectl get pods -n "$NAMESPACE" \
  -l "app.kubernetes.io/name=$NAME" -o wide
```

Confirm that the resource is `Healthy`, the observed replica count matches the desired count, and the remaining Pods are Ready.

To increase the replica count, see [Scale Out](./scale_out_privateai.md).