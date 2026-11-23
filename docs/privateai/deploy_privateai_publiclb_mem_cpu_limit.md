## Set or Change CPU and Memory for an Existing Private AI Deployment

Use this procedure to set CPU and memory requests and limits for the first time or to update existing values.

Set the namespace and name of the existing `PrivateAi` resource:

```sh
export NAMESPACE=pai
export NAME=pai-sample
```

### Update Requests and Limits

This example sets the CPU request and limit to `8` and the memory request and limit to `32Gi` for each Private AI container. These values match the starting capacity described in [Before You Begin](./README.md#resources-shared-memory-and-worker-nodes).

Adjust the values for your workload. Ensure the eligible worker nodes have enough allocatable capacity for all replicas and the rollout. Each request must be less than or equal to its corresponding limit.

```sh
kubectl patch pai "$NAME" -n "$NAMESPACE" --type=merge -p '{
  "spec": {
    "runtime": {
      "resources": {
        "requests": {"cpu": "8", "memory": "32Gi"},
        "limits": {"cpu": "8", "memory": "32Gi"}
      }
    }
  }
}'
```

The operator updates the generated Deployment with the new resource settings. Kubernetes then replaces the Pods as the updated Deployment rolls out. Resource keys omitted from this merge patch, such as `nvidia.com/gpu`, are retained.

### Check the Result

Check the values in the `PrivateAi` resource:

```sh
kubectl get pai "$NAME" -n "$NAMESPACE" \
  -o jsonpath='requests: cpu={.spec.runtime.resources.requests.cpu} memory={.spec.runtime.resources.requests.memory}{"\n"}limits: cpu={.spec.runtime.resources.limits.cpu} memory={.spec.runtime.resources.limits.memory}{"\n"}'
```

List the Pods for this deployment and check their rollout status:

```sh
kubectl get pods -n "$NAMESPACE" \
  -l "app.kubernetes.io/name=$NAME" -o wide

kubectl rollout status "deployment/$NAME" \
  -n "$NAMESPACE" --timeout=15m
```

Inspect a Pod to confirm that its container requests and limits match the configured values:

```sh
POD_NAME=$(kubectl get pods -n "$NAMESPACE" \
  -l "app.kubernetes.io/name=$NAME" \
  -o jsonpath='{.items[0].metadata.name}')

kubectl describe pod "$POD_NAME" -n "$NAMESPACE"
```

Confirm that the replacement Pods are Ready and that the Private AI container shows the expected CPU and memory requests and limits.