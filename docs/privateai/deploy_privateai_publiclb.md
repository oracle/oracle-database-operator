# Deploy Oracle Private AI with a Public Load Balancer

This example deploys Oracle Private AI on an OCI Kubernetes Engine (OKE) cluster using a public Load Balancer and a ConfigMap containing a single model configuration.

**Important:** Complete [Before You Begin](./README.md#before-you-begin) before deploying the `PrivateAi` resource. Configure the reserved public IP address in `pai_sample_publiclb.yaml` and use that same address as the `IP_ADDRESS` value when creating the TLS certificate. The certificate must include the address clients use to connect in its Subject Alternative Name (SAN).

1. Create or update the ConfigMap in the `pai` namespace. Run this command from the repository root:

   ```sh
   kubectl create configmap privateaiconfigjson \
     --from-file=config.json=docs/privateai/provisioning/single_model_https_config.json \
     --namespace pai \
     --dry-run=client -o yaml | kubectl apply -f -
   ```

2. Start with [pai_sample_publiclb.yaml](./provisioning/pai_sample_publiclb.yaml). Replace the public IP placeholder and confirm that the manifest references the `privateaiconfigjson` ConfigMap in the `pai` namespace.

3. From the repository root, apply the manifest and wait for the `PrivateAi` resource to become healthy and its Deployment rollout to complete:

   ```sh
   kubectl apply -f docs/privateai/provisioning/pai_sample_publiclb.yaml
   kubectl wait --for=jsonpath='{.status.status}'=Healthy pai/pai-sample \
     -n pai --timeout=15m
   kubectl rollout status deployment/pai-sample -n pai --timeout=15m
   ```

4. Verify the resource status, public Load Balancer address, Deployment, Pods, and Services:

   ```sh
   kubectl get pai pai-sample -n pai \
     -o jsonpath='status={.status.status} publicLoadBalancerService={.status.publicLoadBalancerService} loadBalancerIP={.status.loadBalancerIP}{"\n"}'
   kubectl get deployment pai-sample -n pai
   kubectl get pods -n pai -l app.kubernetes.io/name=pai-sample -o wide
   kubectl get services -n pai -o wide
   ```

Proceed when the resource reports `Healthy`, the Deployment has one ready replica, the Pod is Ready, and the public Load Balancer Service has an address matching the reserved public IP.
