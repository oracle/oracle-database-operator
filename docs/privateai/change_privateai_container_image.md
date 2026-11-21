# Change the Container Image for an Existing Private AI Deployment

Update the container image used by an existing `PrivateAi` resource. Before applying the change, confirm that the target image is compatible with your configuration and that the image pull Secret exists in the same namespace and can access the image.

Set the namespace and name of the existing `PrivateAi` resource:

```sh
export NAMESPACE=pai
export NAME=pai-sample
```

## Update the Image

Replace `<new-privateai-tag>` with the required image tag:

```sh
kubectl patch pai "$NAME" -n "$NAMESPACE" --type=merge \
  -p '{"spec":{"runtime":{"image":{"name":"container-registry.oracle.com/database/private-ai:<new-privateai-tag>"}}}}'
```

This merge patch changes the image name and preserves other image settings, including the existing pull Secret and pull policy.

If the target image requires a different pull Secret, include its name in the patch. The Secret must exist in the `PrivateAi` resource's namespace:

```sh
kubectl patch pai "$NAME" -n "$NAMESPACE" --type=merge \
  -p '{"spec":{"runtime":{"image":{"name":"container-registry.oracle.com/database/private-ai:<new-privateai-tag>","pullSecret":"<image-pull-secret>"}}}}'
```

The operator updates the generated Deployment, and Kubernetes replaces the Pods as the Deployment rolls out.

## Check the Result

Wait for the Private AI resource to report `Healthy` and for the Deployment rollout to complete:

```sh
kubectl wait --for=jsonpath='{.status.status}'=Healthy "pai/$NAME" \
  -n "$NAMESPACE" --timeout=15m

kubectl rollout status "deployment/$NAME" \
  -n "$NAMESPACE" --timeout=15m
```

Check the configured image and pull Secret, then list the Pods for this deployment:

```sh
kubectl get pai "$NAME" -n "$NAMESPACE" \
  -o jsonpath='image={.spec.runtime.image.name} pullSecret={.spec.runtime.image.pullSecret}{"\n"}'

kubectl get pods -n "$NAMESPACE" \
  -l "app.kubernetes.io/name=$NAME" -o wide
```

Confirm that the replacement Pods are Ready and use the requested image.
