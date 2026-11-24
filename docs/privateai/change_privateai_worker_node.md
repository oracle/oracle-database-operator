# Select Worker Nodes for an Existing Private AI Deployment

Update the worker nodes eligible to run an existing `PrivateAi` deployment. Before selecting nodes, confirm that they have sufficient CPU and memory capacity, any required GPUs, and access to the deployment's storage. Each selected value must match a `kubernetes.io/hostname` label on a node.

Set the namespace and name of the existing `PrivateAi` resource:

```sh
export NAMESPACE=pai
export NAME=pai-sample
```

## Update Worker Node Selection

List the node names and their `kubernetes.io/hostname` label values:

```sh
kubectl get nodes -L kubernetes.io/hostname
```

Replace `worker-a` and `worker-b` with the hostname label values of the nodes to allow. Use one or more values as required. This patch replaces the entire `workerNodes` list, so include every node you want the deployment to use.

```sh
kubectl patch pai "$NAME" -n "$NAMESPACE" --type=merge \
  -p '{"spec":{"runtime":{"workerNodes":["worker-a","worker-b"]}}}'
```

The operator updates the generated Deployment's node selection. Kubernetes then schedules the Pods according to the updated constraints. Pods can remain Pending if the selected nodes lack capacity, have incompatible taints, or cannot access the required storage.

## Check the Result

Wait for the Private AI resource to report `Healthy` and for the Deployment rollout to complete:

```sh
kubectl wait --for=jsonpath='{.status.status}'=Healthy "pai/$NAME" \
  -n "$NAMESPACE" --timeout=15m

kubectl rollout status "deployment/$NAME" \
  -n "$NAMESPACE" --timeout=15m
```

Check the configured worker nodes and list this deployment's Pods with their assigned nodes:

```sh
kubectl get pai "$NAME" -n "$NAMESPACE" \
  -o jsonpath='{.spec.runtime.workerNodes}{"\n"}'

kubectl get pods -n "$NAMESPACE" \
  -l "app.kubernetes.io/name=$NAME" \
  -o custom-columns='NAME:.metadata.name,NODE:.spec.nodeName,READY:.status.containerStatuses[0].ready,STATUS:.status.phase'
```

Confirm that the Pods are Ready and are scheduled on the selected nodes.

## Remove the Worker Node Restriction

To remove the explicit `workerNodes` setting and allow the deployment to use nodes permitted by its other scheduling constraints, patch the field to `null`:

```sh
kubectl patch pai "$NAME" -n "$NAMESPACE" --type=merge \
  -p '{"spec":{"runtime":{"workerNodes":null}}}'
```

The operator updates the generated Deployment. Other cluster scheduling and storage constraints still apply.
