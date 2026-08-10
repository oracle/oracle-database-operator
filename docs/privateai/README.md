# Managing Oracle Private AI Services Container with Oracle Database Operator for Kubernetes

Oracle Database Operator for Kubernetes includes the `PrivateAi` controller for deploying and operating Oracle Private AI Services Container on Kubernetes. This guide is organized around the current `privateai.oracle.com/v4` API and the sample manifests under [`docs/privateai/provisioning`](./provisioning/).


Use this document when you want to:

- deploy a new PrivateAI instance on Kubernetes
- start with a minimal HTTPS-only configuration
- expose a deployment through public or internal load balancers
- scale replicas, adjust resources, or change the container image
- use config map based model catalogs, filesystem-backed models, or Traffic Manager integration

For product background, see the [Oracle Private AI Services Container documentation](https://docs.oracle.com/en/database/oracle/oracle-database/26/prvai/index.html). For operator installation, see the [main operator README](../../README.md).

## Contents

- [Before You Begin](#before-you-begin)
- [Quick Start](#quick-start)
- [Scenario Guide](#scenario-guide)
- [PrivateAI v4 Resource Model](#privateai-v4-resource-model)
- [Traffic Manager](#traffic-manager)
- [Status and Verification](#status-and-verification)
- [Advanced Scenarios](#advanced-scenarios)
- [Compatibility Notes](#compatibility-notes)

## Before You Begin

Complete these steps before using the examples in this directory. Other PrivateAI how-to guides assume this section is already complete.

### Create the namespace

Create the namespace to deploy the PrivateAI Services Container in your Kubernetes Cluster. In this example, the namespace is `pai`.

```sh
kubectl create namespace pai
```

### Basic prerequisites

Make sure the following are already in place:

1. Oracle Database Operator is installed in the cluster.
2. You have accepted the Oracle Container Registry license for the Private AI image.
3. In case of Namespace-scoped deployment, the required role binding is created and the namespace `pai` is added to `WATCH_NAMESPACE` during the Oracle Database Operator Deployment.

### Create the image pull secret

Create a Kubernetes Secret to be used during pulling container image from Oracle Container Registry:

```sh
kubectl create secret docker-registry oracle-container-registry-secret \
  --docker-server=container-registry.oracle.com \
  --docker-username='<oracle-sso-email-address>' \
  --docker-password='<container-registry-auth-token>' \
  --docker-email='<oracle-sso-email-address>' \
  -n pai
```

If you already logged in with `podman`, you can also create the secret from your local auth file.

### Create an auth secret for authenticated deployments

The quickest path uses `spec.security.authEnabled: true`. Create or provision the auth secret before applying the `PrivateAi` resource with `spec.security.authEnabled: true`.

Quick creation example: After cloning the GitHub repository, run the below command:

```sh
cd oracle-database-operator-system/docs/privateai/auth-secret

./create_auth_secret.sh \
  --namespace pai \
  --secret-name paisecret \
  --generate-api-key \
  --ssl-pwd-value '<PASSWORD>' \
  --list-secret
```

For detailed examples, secret updates, multiple API keys, and manual patch commands, see [PrivateAI Auth Secret](./auth-secret/README.md).

The legacy helper assets under [`provisioning`](./provisioning/) are still available for teams that use a shared secret containing `api-key`, `cert.pem`, `key.pem`, and related files.

### Create a TLS secret for HTTPS deployments

The quickest path uses HTTPS. You can create the TLS secret manually, but the recommended flow is to use cert-manager and the helper under [TLS certificate generation](./tls-cert-manager/README.md).

Quick creation example:

`PrivateAi` deployment also needs `keystore.p12` in the same TLS secret, use the helper with PKCS#12 enabled and point the password reference at an existing secret such as the auth secret:

```sh
cd oracle-database-operator-system/docs/privateai/tls-cert-manager

NAMESPACE=pai \
TARGET=pai \
ISSUER_NAME=tcps-selfsigned-bootstrap \
ISSUER_KIND=ClusterIssuer \
COMMON_NAME=api.example.com \
DNS_NAMES="api.example.com,pai-sample.pai.svc,pai-sample.pai.svc.cluster.local" \
IP_ADDRESSES="<IP_ADDRESS>" \
GENERATE_PKCS12=true \
PASSWORD_SECRET_NAME=paisecret \
PASSWORD_SECRET_KEY=privateai-ssl-pwd \
./tr_cert_manager.sh
```

List the generated certificate and secret:

```sh
kubectl get certificate -n pai
kubectl get secret privateai-tls -n pai
```

Use that guide when you need either of these:

- a `PrivateAi` TLS secret wired through `spec.security.tls.secretName`
- a `TrafficManager` nginx TLS secret wired through `spec.security.tls.secretName`

The guide includes:

- a target-aware script so you can create either `pai` or `nginx` certificates
- SAN and IP examples for internal and external endpoints
- renewal behavior, forced renewal steps, and secret update notes
- common pitfalls such as hostname mismatch and wrong secret wiring

For detailed procedure, renewal steps, and troubleshooting notes, see [TLS certificate generation](./tls-cert-manager/README.md).

Some older provisioning manifests in this directory still reference `paisecret`. You can either keep using that secret name when you run the helper, or update the manifest to point at your newer TLS secret name.

### Optional: Create model config maps

When no configuration file is specified during the deployment, the PrivateAI container will start and make all the pre-approved models available.

Use these helper guides when you want config map based model catalogs:

- [Use only Specific Pre-approved model](./configmap_specific_preapproved_model.md)
- [Single model using HTTPS URL](./configmap_single_model_https.md)
- [Multiple models using HTTPS URL](./configmap_multi_model_https.md)
- [Multiple models on file system](./configmap_multi_model_filesystem.md)

## Quick Start

This example demonstrates the simplest working `PrivateAI` deployment using the grouped `v4` configuration fields. The deployment is configured with:

- A single replica
- HTTPS listener only
- Internal service only
- Default models included in the container image
- No external load balancer

Apply this minimal manifest:

```yaml
apiVersion: privateai.oracle.com/v4
kind: PrivateAi
metadata:
  name: pai-quickstart
  namespace: pai
spec:
  security:
    authEnabled: true
    secret:
      name: paisecret
      mountLocation: /privateai/ssl
    tls:
      secretName: privateai-tls
      mountLocation: /privateai/ssl
  runtime:
    image:
      name: container-registry.oracle.com/database/private-ai:latest
      pullSecret: oracle-container-registry-secret
    replicas: 1
  networking:
    service:
      ports:
      - port: 8443
        targetPort: 8443
        protocol: TCP
```

Apply and verify:

```sh
kubectl apply -f pai-quickstart.yaml
kubectl get pai,pods,svc -n pai
kubectl get pai pai-quickstart -n pai -o jsonpath='{.status.status}{"\n"}'
```

When the deployment is ready, the status becomes `Healthy`.

To access it locally, first start port forwarding in one session as below:

```sh
kubectl port-forward svc/pai-quickstart 8443:8443 -n pai
```

Once port forwarding is started, from another session, run below command to check the health of the deployment:

```sh
curl -k -v https://127.0.0.1:8443/health
```

You can get the AI Models deployed using below command:

```sh
curl -k --noproxy '*' --header "Authorization: Bearer `cat <PATH of the api-key file>/api-key`" https://127.0.0.1:8443/v1/models
```

For detailed instructions on accessing the deployed PrivateAI service, refer to [Access the deployed service](./access_privateai.md)

## Scenario Guide

Use this section as a quick entry point for the most common PrivateAI scenarios.

| Scenario | Where to start |
| --- | --- |
| Minimal deployment | [Quick Start](#quick-start) |
| Generate or update auth secret | [PrivateAI Auth Secret](./auth-secret/README.md) |
| Generate TLS secret for PrivateAI or nginx | [TLS certificate generation](./tls-cert-manager/README.md) |
| Public load balancer | [Deploy with public load balancer](./deploy_privateai_publiclb.md) |
| Public load balancer without config map | [Deploy with public load balancer and no config map](./deploy_privateai_publiclb_without_configmap.md) |
| Internal load balancer | [Deploy with internal load balancer](./deploy_privateai_internallb.md) |
| Multiple HTTPS models | [Deploy with multiple HTTPS models and internal load balancer](./deploy_privateai_multi_model_https_internallb.md) |
| Multiple filesystem-backed models | [Deploy with multiple filesystem-backed models and internal load balancer](./deploy_privateai_multi_model_filesystem_internallb.md) |
| Scale out or scale in | [Scale out](./scale_out_privateai.md) or [Scale in](./scale_in_privateai.md) |
| Change CPU and memory | [Deploy with memory and CPU limits](./deploy_privateai_publiclb_mem_cpu_limit.md) or [Change memory and CPU limits](./change_privateai_publiclb_mem_cpu_limit.md) |
| Change the container image | [Change the container image](./change_privateai_container_image.md) |
| Worker node placement | [Deploy with worker node selection](./deploy_privateai_publiclb_worker_node.md) |
| Access the service | [Access the deployed service](./access_privateai.md) |
| PVC-backed model storage | [Create OCI FSS based PVCs](./create_oci_fss_based_pvc.md) |
| Traffic Manager with NGINX or CMAN | [Traffic Manager documentation](../trafficmanager/README.md) |
| Read logs on OKE | [Read PrivateAI logs with OKE Logging](./oke-logging.md) |
| Debugging | [Debug and troubleshoot](./debug_privateai.md) |

## PrivateAI v4 Resource Model

The current `v4` API is organized around grouped configuration blocks. New manifests and updated samples should use this structure.

| Area | v4 fields | Purpose |
| --- | --- | --- |
| Authentication and TLS | `spec.security` | Auth secret and TLS secret configuration |
| Runtime | `spec.runtime` | Image, pull secret, replicas, env, resources, debug, worker nodes |
| Model config | `spec.configuration` | ConfigMap-based runtime configuration |
| Storage | `spec.storage` | Storage class, PVC mounts, size, log location, delete-on-delete |
| Networking | `spec.networking` | HTTP/HTTPS listeners, local service, external service, nodeports, Traffic Manager |
| Logging | `spec.runtime.env` | Optional application log level or format settings supported by the image |

On OKE, log collection should use the main PrivateAI container `stdout` and
`stderr` stream together with cluster workload logging. See [Read PrivateAI
logs with OKE Logging](./oke-logging.md).

### Key grouped fields

| Need | Preferred fields |
| --- | --- |
| Auth secret | `spec.security.authEnabled`, `spec.security.secret` |
| TLS secret | `spec.security.tls` |
| Image and pull secret | `spec.runtime.image` |
| Replica count | `spec.runtime.replicas` |
| Resource limits | `spec.runtime.resources` |
| Worker node selection | `spec.runtime.workerNodes` |
| Workload and pod annotations | `spec.runtime.annotations` |
| Config map | `spec.configuration.configFile` |
| PVC mounts | `spec.storage.pvcList` |
| Service ports | `spec.networking.service.ports` |
| Public load balancer | `spec.networking.service.publicLoadBalancer` |
| Private load balancer | `spec.networking.service.privateLoadBalancer` |
| Traffic Manager integration | `spec.networking.trafficManager` |

### Defaults and validation

The PrivateAI controller applies several important defaults and checks:

- If neither listener is explicitly enabled, HTTPS is used by default.
- HTTPS defaults to port `8443` and HTTP defaults to `8080`.
- `spec.configuration.configFile.mountLocation` defaults to `/privateai/config`.
- `spec.security.secret.mountLocation` and `spec.security.tls.mountLocation` default to `/privateai/ssl`.
- `spec.networking.service.ports` defaults to HTTPS service port `443` targeting the active PrivateAI listener.
- `spec.networking.service.cluster.enabled` defaults to `true`.
- `spec.networking.service.publicLoadBalancer.enabled` and `spec.networking.service.privateLoadBalancer.enabled` default to `false`.
- `spec.networking.trafficManager.routePath` defaults to `/<resource-name>/v1/` when omitted.
- Secret and config map mount locations must be absolute paths.

### Networking services

PrivateAI uses the `spec.networking.service` structure:

```yaml
networking:
  service:
    ports:
    - name: https
      port: 443
      targetPort: 8443
      protocol: TCP
    cluster:
      enabled: true
    publicLoadBalancer:
      enabled: false
    privateLoadBalancer:
      enabled: false
```

The `ports` entries are the service port mappings. `port` is the exposed Service port and `targetPort` is the container listener port. Use `publicLoadBalancer` and `privateLoadBalancer` independently when both public and private access are required.

For `privateLoadBalancer`, the controller automatically adds:

```text
service.beta.kubernetes.io/oci-load-balancer-internal: "true"
```

## Traffic Manager

Use Traffic Manager when multiple PrivateAI deployments should share one routed endpoint. The full NGINX and CMAN field reference, examples, and use cases are in [Traffic Manager documentation](../trafficmanager/README.md). This section shows only one PrivateAI use case.

The example below creates one NGINX Traffic Manager and routes two PrivateAI deployments through different paths:

```yaml
apiVersion: network.oracle.com/v4
kind: TrafficManager
metadata:
  name: pai-nginx
  namespace: pai
spec:
  type: nginx
  runtime:
    image: nginx:1.27
    replicas: 1
  security:
    tls:
      enabled: true
      secretName: nginx-tls
      mountLocation: /etc/nginx/tls
  service:
    internal:
      enabled: true
    external:
      enabled: true
      serviceType: LoadBalancer
      port: 443
      targetPort: 8443
---
apiVersion: privateai.oracle.com/v4
kind: PrivateAi
metadata:
  name: pai-finance
  namespace: pai
spec:
  security:
    authEnabled: true
    secret:
      name: paisecret
      mountLocation: /privateai/ssl
    tls:
      secretName: privateai-tls
      mountLocation: /privateai/ssl
  runtime:
    image:
      name: container-registry.oracle.com/database/private-ai:latest
      pullSecret: oracle-container-registry-secret
    replicas: 1
  networking:
    trafficManager:
      ref: pai-nginx
      routePath: /finance/v1/
---
apiVersion: privateai.oracle.com/v4
kind: PrivateAi
metadata:
  name: pai-hr
  namespace: pai
spec:
  security:
    authEnabled: true
    secret:
      name: paisecret
      mountLocation: /privateai/ssl
    tls:
      secretName: privateai-tls
      mountLocation: /privateai/ssl
  runtime:
    image:
      name: container-registry.oracle.com/database/private-ai:latest
      pullSecret: oracle-container-registry-secret
    replicas: 1
  networking:
    trafficManager:
      ref: pai-nginx
      routePath: /hr/v1/
```

Requests sent to `/finance/v1/` are routed to `pai-finance`; requests sent to `/hr/v1/` are routed to `pai-hr`. For NGINX TLS, backend TLS verification, private load balancer examples, CMAN generated config, CMAN file config, CMAN REST API, and all `TrafficManager` fields, use [Traffic Manager documentation](../trafficmanager/README.md).

Verify the routed endpoint:

```sh
kubectl get trafficmanager pai-nginx -n pai
kubectl get trafficmanager pai-nginx -n pai \
  -o jsonpath='{.status.status}{"\n"}{.status.externalEndpoint}{"\n"}{.status.nginx.routes}{"\n"}'
```

### Workload and pod annotations

User annotations can be applied to the generated Deployment object and pod template:

```yaml
runtime:
  annotations:
    workload:
      company.com/change-ticket: CHG-12345
    pod:
      prometheus.io/scrape: "true"
      prometheus.io/port: "8443"
```

Annotations under the `privateai.oracle.com/` prefix are reserved for the operator.

## Status and Verification

Use these commands to check deployment health and discover access details:

```sh
kubectl get pai -n pai
kubectl describe pai <name> -n pai
kubectl get pai <name> -n pai -o jsonpath='{.status.status}{"\n"}{.status.mode}{"\n"}{.status.localService}{"\n"}{.status.publicLoadBalancerService}{"\n"}{.status.privateLoadBalancerService}{"\n"}{.status.loadBalancerIP}{"\n"}'
```

Typical status fields include:

- `status.status`
- `status.replicas`
- `status.localService`
- `status.publicLoadBalancerService`
- `status.privateLoadBalancerService`
- `status.loadBalancerIP`
- `status.mode`
- `status.trafficManager`

## Advanced Scenarios

After the quick start, these are the most common production follow-on scenarios.

### Deployment patterns

- [Deploy with public load balancer](./deploy_privateai_publiclb.md)
- [Deploy with public load balancer and no config map](./deploy_privateai_publiclb_without_configmap.md)
- [Deploy with internal load balancer](./deploy_privateai_internallb.md)
- [Deploy with multiple HTTPS models and internal load balancer](./deploy_privateai_multi_model_https_internallb.md)
- [Deploy with multiple filesystem-backed models and internal load balancer](./deploy_privateai_multi_model_filesystem_internallb.md)

### Model configuration

- [Single model using HTTPS URL](./configmap_single_model_https.md)
- [Multiple models using HTTPS URL](./configmap_multi_model_https.md)
- [Multiple models on file system](./configmap_multi_model_filesystem.md)
- [Add a model to an existing multi-model HTTPS deployment](./deploy_privateai_multi_model_https_internallb_add_model.md)
- [Remove a model from an existing multi-model HTTPS deployment](./deploy_privateai_multi_model_https_internallb_remove_model.md)

### Scaling and updates

- [Deploy with multiple replicas](./deploy_privateai_publiclb_multi_replica.md)
- [Scale out](./scale_out_privateai.md)
- [Scale in](./scale_in_privateai.md)
- [Deploy with memory and CPU limits](./deploy_privateai_publiclb_mem_cpu_limit.md)
- [Change memory and CPU limits](./change_privateai_publiclb_mem_cpu_limit.md)
- [Change the container image](./change_privateai_container_image.md)
- [Deploy with worker node selection](./deploy_privateai_publiclb_worker_node.md)

### Access, storage, and troubleshooting

- [Access the deployed service](./access_privateai.md)
- [Create OCI FSS based PVCs](./create_oci_fss_based_pvc.md)
- [Read PrivateAI logs with OKE Logging](./oke-logging.md)
- [Debug and troubleshoot](./debug_privateai.md)

## Compatibility Notes

The `v4` API still accepts older flat fields such as `paiConfigFile`, `paiEnableAuthentication`, `paiSecret`, `paiImage`, `replicas`, `paiService`, and `isExternalSvc`, but those fields are deprecated.

The documentation and provisioning samples in this directory now prefer the grouped layout:

- `spec.security`
- `spec.runtime`
- `spec.configuration`
- `spec.storage`
- `spec.networking`

For new manifests, avoid mixing deprecated flat fields with grouped `v4` fields unless the example is specifically about backward compatibility.
