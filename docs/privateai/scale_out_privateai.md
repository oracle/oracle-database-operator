## Scale Out an Existing Private AI Deployment

Increase the replica count of an existing `PrivateAi` resource. Before scaling, confirm that the eligible worker nodes have enough capacity and that every replica can access the configured models. If the models are on a PVC, confirm that the volume supports mounting by the additional Pods.

Set the namespace and name of the existing `PrivateAi` resource:

```sh
export NAMESPACE=pai
export NAME=pai-sample
```

### Increase the Replica Count

This example increases the desired replica count from one to three. Adjust `3` to the required total:

```sh
kubectl patch pai "$NAME" -n "$NAMESPACE" --type=merge \
  -p '{"spec":{"runtime":{"replicas":3}}}'
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

Confirm that the resource is `Healthy`, the observed replica count matches the desired count, and all listed Pods are Ready.

To reduce the replica count, see [Scale In](./scale_in_privateai.md).
