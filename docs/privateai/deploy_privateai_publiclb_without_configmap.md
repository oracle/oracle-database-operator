# Deploy Oracle Private AI with a Public Load Balancer and Image-Default Models

This example deploys Oracle Private AI on an OCI Kubernetes Engine (OKE) cluster using a public Load Balancer. It does not specify model or model-file details in a ConfigMap. The deployment uses the default model files included in the Private AI container image.

**Important:** Complete [Before You Begin](./README.md#before-you-begin) before deploying the `PrivateAi` resource. Use the reserved public Load Balancer IP address as the `IP_ADDRESS` value when creating the certificate. The certificate must be valid for the address clients use to connect.

1. Start with [pai_sample_publiclb_no_configmap.yaml](./provisioning/pai_sample_publiclb_no_configmap.yaml).

2. Replace the public IP placeholder in the manifest with the reserved public IP address. Ensure that the TLS certificate includes this address in its Subject Alternative Name (SAN).

3. From the repository root, apply the manifest and wait for the `PrivateAi` resource to become healthy:

   ```sh
   kubectl apply -f docs/privateai/provisioning/pai_sample_publiclb_no_configmap.yaml
   kubectl wait --for=jsonpath='{.status.status}'=Healthy pai/pai-sample \
     -n pai --timeout=15m
   kubectl rollout status deployment/pai-sample -n pai --timeout=15m
   ```

4. Verify the `PrivateAi` status, public Load Balancer address, Deployment, Pod, and Services:

   ```sh
   kubectl get pai pai-sample -n pai \
     -o jsonpath='status={.status.status} publicLoadBalancerService={.status.publicLoadBalancerService} loadBalancerIP={.status.loadBalancerIP}{"\n"}'
   kubectl get deployment pai-sample -n pai
   kubectl get pods -n pai -l app.kubernetes.io/name=pai-sample -o wide
   kubectl get services -n pai -o wide
   ```

Proceed when the resource reports `Healthy`, the Load Balancer address matches the reserved public IP, the Deployment has one ready replica, and the Pod is Ready.
