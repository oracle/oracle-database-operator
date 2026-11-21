# Deploy Private AI in an OKE Cluster with Multiple Models from HTTPS URLs and an Internal Load Balancer

This example deploys the Oracle Private AI container on an OCI OKE cluster. It configures multiple models using HTTPS URLs specified in a ConfigMap and enables an internal load balancer.

**Note:** OKE supports assigning a reserved private IPv4 address to an internal load balancer on clusters running Kubernetes 1.32 or later. The reserved IP address must belong to the subnet used by the load balancer. For details, see [Configuring Load Balancers and Network Load Balancers](https://docs.oracle.com/en-us/iaas/Content/ContEng/Tasks/contengconfiguringloadbalancersnetworkloadbalancers-subtopic.htm).

**Important:** Complete [Before You Begin](./README.md#before-you-begin) before deploying the Private AI resource. Use the reserved private IP address as the `IP_ADDRESS` value when creating the certificate. Ensure that the certificate is valid for the address clients use to connect. For instructions on reserving a private IP address, see [Adding a Reserved IPv4 Address](https://docs.oracle.com/en-us/iaas/Content/Network/Tasks/reserved-ipv4-adding.htm).

Use [`pai_sample_multi_model_https_internallb.yaml`](./provisioning/pai_sample_multi_model_https_internallb.yaml) for this use case.

1. Create the ConfigMap by following the [multiple-model HTTPS ConfigMap instructions](./configmap_multi_model_https.md).

2. Start with [`pai_sample_multi_model_https_internallb.yaml`](./provisioning/pai_sample_multi_model_https_internallb.yaml).

3. Replace the reserved private IP placeholder in the manifest. Ensure the certificate SAN contains the same IP address.

4. From the repository root, apply the manifest and wait for the Private AI resource to become healthy:

   ```sh
   kubectl apply -f docs/privateai/provisioning/pai_sample_multi_model_https_internallb.yaml
   kubectl wait --for=jsonpath='{.status.status}'=Healthy pai/pai-sample \
     -n pai --timeout=15m
   ```

5. Verify the Private AI resource, load balancer, and Pods:

   ```sh
   kubectl get pai pai-sample -n pai -o yaml
   kubectl get pods,services -n pai
   ```

6. Check the logs of the Private AI Pod:

   ```sh
   POD_NAME=$(kubectl get pods -n pai \
     -l app.kubernetes.io/name=pai-sample \
     -o jsonpath='{.items[0].metadata.name}')

   kubectl logs -n pai "$POD_NAME"
   ```

7. Confirm that `status.paiConfigMap` identifies the expected ConfigMap, the private load balancer address matches the reserved IP, and the Pods are Ready. To inspect the mounted model configuration, check `/privateai/config/config.json` in a running Pod:

   ```sh
   POD_NAME=$(kubectl get pods -n pai \
     -l app.kubernetes.io/name=pai-sample \
     -o jsonpath='{.items[0].metadata.name}')

   kubectl exec -n pai "$POD_NAME" -- cat /privateai/config/config.json
   ```
