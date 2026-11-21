# Deploy Oracle Private AI with a Public Load Balancer and Multiple Replicas

Deploy Oracle Private AI on an OCI Kubernetes Engine (OKE) cluster with a public Load Balancer. This example configures three replicas in the `PrivateAi` resource.

**Important:** Complete [Before You Begin](./README.md#before-you-begin) before using the Oracle Private AI Controller.

**Note:** Before deployment, update `pai_sample_publiclb_multi_replica.yaml` with the reserved public IP address. Ensure that the TLS certificate includes this IP address in its Subject Alternative Name (SAN).

1. Start with [pai_sample_publiclb_multi_replica.yaml](./provisioning/pai_sample_publiclb_multi_replica.yaml). Confirm that `spec.runtime.replicas` is set to `3`.

2. From the repository root, apply the manifest and wait for the `PrivateAi` resource to become healthy:

   ```sh
   kubectl apply -f docs/privateai/provisioning/pai_sample_publiclb_multi_replica.yaml
   kubectl wait --for=jsonpath='{.status.status}'=Healthy pai/pai-sample \
     -n pai --timeout=15m
   kubectl rollout status deployment/pai-sample -n pai --timeout=15m
   ```

3. Verify the `PrivateAi` status, replica count, Load Balancer address, Pods, and Services:

   ```sh
   kubectl get pai pai-sample -n pai \
     -o jsonpath='status={.status.status} replicas={.status.replicas} publicLoadBalancerService={.status.publicLoadBalancerService} loadBalancerIP={.status.loadBalancerIP}{"\n"}'
   kubectl get deployment pai-sample -n pai
   kubectl get pods -n pai -l app.kubernetes.io/name=pai-sample -o wide
   kubectl get services -n pai -o wide
   ```

Confirm that the resource is `Healthy`, the Deployment has three ready replicas, all three Pods are Ready, and the public Load Balancer Service has an address.
