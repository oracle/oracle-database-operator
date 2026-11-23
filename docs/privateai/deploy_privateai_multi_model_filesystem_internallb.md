# Deploy Private AI in OKE cluster using Multiple AI Models on File System and an Internal LoadBalancer

Deploy Oracle Private AI Container on your Cloud-based Kubernetes cluster. The Private AI Container is deployed using multiple AI Models. The model files are accessible to the Private AI Pod using Mounted Persistent Volume (PV). In this example, the deployment uses the YAML file based on the `OCI OKE` cluster.

See [Create OCI FSS based PVC](./create_oci_fss_based_pvc.md) for the steps to create an `oci-fss` based PVC to use in this example.

**NOTE:** The option to reserve a Private IP and use that with an OCI Internal LoadBalancer is available from `OCI OKE` cluster with Kubernetes version 1.32 onwards. Please check the [Configuring Load Balancers and Network Load Balancers documentation](https://docs.oracle.com/en-us/iaas/Content/ContEng/Tasks/contengconfiguringloadbalancersnetworkloadbalancers-subtopic.htm).

**IMPORTANT:** Complete [Before You Begin](./README.md#before-you-begin) before using Oracle Private AI Controller. Make sure you use the Reserved Private IP Address for the Internal LoadBalancer to the parameter `IP_ADDRESS` while creating the certificate for this `PrivateAi` Deployment. Please refer to the [Adding a Reserved IPv4 Address documentation](https://docs.oracle.com/en-us/iaas/Content/Network/Tasks/reserved-ipv4-adding.htm) for the steps to reserve an IP in the subnet you want.

**NOTE:** Modify the file `pai_sample_multi_model_filesystem_internallb.yaml` with the actual Reserved Private IP before deployment.

1. Start with [pai_sample_multi_model_filesystem_internallb.yaml](./provisioning/pai_sample_multi_model_filesystem_internallb.yaml).

2. Create the OCI FSS PV and PVC. For the example files, apply them from the repository root:

   ```sh
   kubectl apply -f docs/privateai/provisioning/oke-pv.yaml
   kubectl apply -f docs/privateai/provisioning/oke-pvc.yaml
   ```

   Confirm that the PVC is bound:

   ```sh
   kubectl get pv oke-fsspv
   kubectl get pvc oke-fsspvc -n pai
   ```

3. Create or update `multiconfigjson` from the repository root. The ConfigMap provides the model configuration; the model files remain on the PVC:

   ```sh
   kubectl create configmap multiconfigjson \
     --from-file=config.json=docs/privateai/provisioning/multi_model_filesystem_config.json \
     --namespace pai \
     --dry-run=client -o yaml | kubectl apply -f -
   ```
 
   Refer to [configmap](./configmap_multi_model_filesystem.md) for more details.
   
4. Replace the reserved private IP placeholder in the manifest. Apply the manifest and wait for the PrivateAi resource to become healthy:

   ```sh
   kubectl apply -f docs/privateai/provisioning/pai_sample_multi_model_filesystem_internallb.yaml
   kubectl wait --for=jsonpath='{.status.status}'=Healthy pai/pai-sample -n pai --timeout=15m
   ```

5. Verify the PrivateAi status, Load Balancer address, and Pod readiness:

   ```sh
   kubectl get pai pai-sample -n pai
   kubectl get pods,svc -n pai -l app.kubernetes.io/instance=pai-sample
   ```

Proceed when the resource reports `Healthy`, the private load balancer address matches the reserved IP, and the Pod or Pods are Ready.

**NOTE:** The file `/privateai/config/config.json` inside the running Kubernetes Pod will have the details of the AI Models currently deployed. To confirm, use the following steps:

```sh
kubectl exec -it -n pai $(kubectl get pod -n pai -l app.kubernetes.io/name=pai-sample -o jsonpath='{.items[0].metadata.name}') -- /bin/bash
cat /privateai/config/config.json
```
