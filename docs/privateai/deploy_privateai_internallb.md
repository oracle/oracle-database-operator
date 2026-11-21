# Deploy Oracle Private AI in an OKE Cluster with an Internal Load Balancer

This example deploys the Oracle Private AI container on an OCI OKE cluster using a YAML manifest.

**Note:** OKE supports assigning a reserved private IPv4 address to an internal load balancer on clusters running Kubernetes 1.32 or later. The reserved IP address must belong to the subnet used by the load balancer. For details, see [Configuring Load Balancers and Network Load Balancers](https://docs.oracle.com/en-us/iaas/Content/ContEng/Tasks/contengconfiguringloadbalancersnetworkloadbalancers-subtopic.htm).

**Important:** Complete [Before You Begin](./README.md#before-you-begin) before deploying the Private AI resource. Use the reserved private IP address as the `IP_ADDRESS` value when creating the certificate. The certificate must be valid for the address clients use to connect. For instructions on reserving a private IP address, see [Adding a Reserved IPv4 Address](https://docs.oracle.com/en-us/iaas/Content/Network/Tasks/reserved-ipv4-adding.htm).

1. Start with [`pai_sample_internallb.yaml`](./provisioning/pai_sample_internallb.yaml).

2. Replace the reserved private IP placeholder. Ensure the certificate SAN contains the same IP address.

3. From the repository root, apply the manifest and wait for the Private AI resource to become healthy:

   ```sh
   kubectl apply -f docs/privateai/provisioning/pai_sample_internallb.yaml
   kubectl wait --for=jsonpath='{.status.status}'=Healthy pai/pai-sample \
     -n pai --timeout=15m
   ```

4. Verify the Private AI resource status, load balancer details, and Pod readiness:

   ```sh
   kubectl get pai pai-sample -n pai -o yaml
   kubectl get pods,services -n pai
   ```

Proceed when the resource reports `Healthy`, the private load balancer address matches the reserved IP, and the Pod or Pods are Ready.
