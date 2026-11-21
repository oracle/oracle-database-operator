# Debug and Troubleshoot Private AI Pods in Kubernetes

Use the commands in this page to inspect the status, logs, and events for Private AI Pods managed by the Private AI Controller.

Set the namespace and name of the `PrivateAi` resource:

```sh
export NAMESPACE=pai
export NAME=pai-sample
```

## List the Private AI Pods

List the Pods for the deployment, including their status, restart count, IP address, and assigned node:

```sh
kubectl get pods -n "$NAMESPACE" \
  -l "app.kubernetes.io/name=$NAME" -o wide
```

## View Pod Logs

Set `POD_NAME` to a Pod name from the previous command. This command selects the first matching Pod:

```sh
POD_NAME=$(kubectl get pods -n "$NAMESPACE" \
  -l "app.kubernetes.io/name=$NAME" \
  -o jsonpath='{.items[0].metadata.name}')

printf 'Selected Pod: %s\n' "$POD_NAME"
kubectl logs -f "$POD_NAME" -n "$NAMESPACE"
```

If the deployment has multiple replicas, repeat the command with the name of each Pod you want to inspect.

To view logs from the previous container instance after a restart, run:

```sh
kubectl logs --previous "$POD_NAME" -n "$NAMESPACE"
```

## Describe a Pod

Use `kubectl describe` to review the Pod's container state, volume mounts, scheduling details, and recent events:

```sh
kubectl describe pod "$POD_NAME" -n "$NAMESPACE"
```
