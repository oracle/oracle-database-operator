# Oracle AI Database Operator for Kubernetes: Managing Private AI Services Container

Oracle AI Database Operator for Kubernetes includes the `PrivateAi` controller for
deploying and operating Oracle Private AI Services Container on Kubernetes.
This guide is organized around user tasks and the sample manifests under
[`docs/privateai/provisioning`](./provisioning/), using the
`privateai.oracle.com/v4` API.

Use this guide to:

- deploy a minimal authenticated HTTPS instance;
- expose Private AI Services through ClusterIP or an OCI load balancer;
- configure authentication, TLS, models, storage, logging, and runtime limits;
- scale or update an existing deployment; and
- troubleshoot status, Pods, Services, certificates, and load balancers.

For product background, see the [Oracle Private AI Services Container documentation](https://docs.oracle.com/en/database/oracle/oracle-database/26/prvai/index.html). For operator installation, see the [main operator README](../../README.md).

## Contents

- [Quick Start](#quick-start)
- [Verify Private AI Deployment](#verify-private-ai-deployment)
- [Before You Begin](#before-you-begin)
- [Networking and Access](#networking-and-access)
- [Private AI Services Container on OpenShift](#private-ai-services-container-on-openshift)
- [Private AI v4 Resource Model](#private-ai-v4-resource-model)
- [Deployment Scenarios and Lifecycle](#deployment-scenarios-and-lifecycle)
- [Other Model Configuration](#other-model-configuration)
- [Runtime and Operations Scenarios](#runtime-and-operations-scenarios)
- [Troubleshooting](#troubleshooting)
- [Additional Information](#additional-information)
- [Known Issues](#known-issues)

## Quick Start

This path creates one authenticated HTTPS deployment with a ClusterIP Service
and the default models included in the container image. It does not create a
public load balancer.

> **Before proceeding:** Complete the applicable actions in
> [Before You Begin](#before-you-begin). The Quick Start depends on the
> namespace, registry Secret, authentication Secret, certificate issuer, and
> TLS Secret prepared there.

**In this section:**

- [Apply the PrivateAi Resource](#apply-the-privateai-resource)

**Required prerequisites and example names**

| Requirement | Values used by this Quick Start | Complete before applying |
| --- | --- | --- |
| Shared deployment prerequisites | [Before You Begin](#before-you-begin) | **[Review Before You Begin](#before-you-begin)** |
| Target namespace | `pai` | Create all referenced Secrets in this namespace. |
| PrivateAi resource | pai-quickstart | Use this name consistently in metadata and kubectl commands. |
| Operator scope | The operator watches `pai` | **[Verify the operator and namespace scope](#verify-the-operator-and-namespace-scope)** |
| Image and image-pull Secret | Private AI image / `oracle-container-registry-secret` | **[Create the image-pull Secret](#create-the-image-pull-secret)** |
| Authentication and TLS Secrets | `paisecret` / `privateai-tls` | **[Create the authentication Secret](#create-the-authentication-secret)** and **[create the TLS Secret](#create-the-tls-secret)** |

**Review Manifest Fields**

| Component | Fields to review | Required relationship |
| --- | --- | --- |
| Identity | metadata.name, metadata.namespace | The namespace must contain every referenced Secret. |
| Security | spec.security.secret, spec.security.tls | Names match paisecret and privateai-tls, or the values created in [Before You Begin](#before-you-begin). |
| Runtime | spec.runtime.image, spec.runtime.image.pullSecret, spec.runtime.replicas | The image-pull Secret exists in the same namespace. |
| Service | spec.networking.service.ports | Service port 8443 targets the HTTPS listener on 8443. |

**Review Required Topology**

| Flow | Required topology | Confirm before applying |
| --- | --- | --- |
| Client access | ClusterIP Service to one Private AI Pod | This path is in-cluster or uses kubectl port-forward; it does not create a load balancer. |
| Authentication and TLS | Client to HTTPS Service, then mounted Secrets in the Pod | The certificate is valid for the hostname or IP used by the client. |

### Apply the PrivateAi Resource

Save this as `pai-quickstart.yaml`:

The manifest uses the default values from `Before You Begin`. If you changed
`NAMESPACE`, `NAME`, or any Secret variable, update the corresponding
`metadata`, Secret, and image pull Secret fields below; Kubernetes does not
expand shell variables inside YAML files.

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
      name: container-registry.oracle.com/database/private-ai:large-cpu-infer-latest
      pullSecret: oracle-container-registry-secret
    replicas: 1
  networking:
    service:
      ports:
      - name: https
        port: 8443
        targetPort: 8443
        protocol: TCP
```

Apply and wait for the controller to report `Healthy`:

```sh
kubectl apply -f pai-quickstart.yaml

kubectl wait \
  --for=jsonpath='{.status.status}'=Healthy \
  pai/"$NAME" \
  --namespace "$NAMESPACE" \
  --timeout=15m
```

## Verify Private AI Deployment

**In this section:**

- [Verify Resource Status and Services](#verify-resource-status-and-services)
- [Status Fields and Behavior](#status-fields-and-behavior)
- [Access the ClusterIP Service](#access-the-clusterip-service)
- [Inspect the Generated Workload](#inspect-the-generated-workload)

After applying a `PrivateAi` resource, confirm that it reports `Healthy`
before using its endpoint.

### Verify Resource Status and Services

```sh
kubectl get pai -n pai
kubectl describe pai pai-quickstart -n pai

kubectl get pai pai-quickstart -n pai \
  -o jsonpath='status={.status.status}{"\n"}replicas={.status.replicas}{"\n"}mode={.status.mode}{"\n"}localService={.status.localService}{"\n"}publicService={.status.publicLoadBalancerService}{"\n"}privateService={.status.privateLoadBalancerService}{"\n"}loadBalancerIP={.status.loadBalancerIP}{"\n"}'

kubectl get pai pai-quickstart -n pai \
  -o jsonpath='{range .status.conditions[*]}{.type}={.status} reason={.reason} message={.message}{"\n"}{end}'
```

### Status Fields and Behavior

The following fields are controller-owned. Do not patch them; inspect them
with `kubectl get pai` or `kubectl describe pai` instead.

| Status field | Behavior | What to do with it |
| --- | --- | --- |
| `status.status` | Starts as `Pending`; reports `Updating` while a Deployment rollout is in progress, `Healthy` after the desired rollout is ready, and `Error` when the current reconcile fails. The API also reserves `Unhealthy`, `Patching`, and `Unknown` for health or state that cannot be determined. | Wait for `Healthy` before sending traffic. For any other steady state, inspect conditions, events, and controller logs. |
| `status.replicas` | The effective replica target derived from the `PrivateAi` resource. It is not a ready-replica count. | Compare it with `spec.runtime.replicas`, then check the Deployment and Pods for readiness. |
| `status.localService` and `status.clusterIP` | Populated when the controller creates the local ClusterIP Service. | Use the service name for port-forwarding and the ClusterIP for in-cluster clients. |
| `status.publicLoadBalancerService`, `status.privateLoadBalancerService`, and `status.externalService` | Populated only for the corresponding external, public, or private load-balancer Service. `externalService` is the active externally exposed Service. | Use the named Service to inspect provider events and annotations. |
| `status.loadBalancerIP` | Set after the Kubernetes Service provider reports an ingress IP address or hostname. It can be empty while the load balancer is provisioning. | Use it only after it is populated; otherwise inspect `kubectl describe service <service-name>`. |
| `status.podIP` and `status.nodeIP` | Record the selected Private AI Pod address and its node address when available. | Use for diagnostics only; do not use either address as a stable client endpoint. |
| `status.paiSecret` | Records the authentication Secret name, observed `resourceVersion`, and whether the expected API key and certificate data are present. A Secret version change triggers a rollout. | Confirm the name, data flags, and version when authentication changes. |
| `status.tlsSecret` | Records the TLS Secret name and observed `resourceVersion`. A version change triggers a rollout. | Confirm it after certificate rotation and wait for `Healthy`. |
| `status.paiConfigMap` | Records the model ConfigMap name and observed `resourceVersion`. A version change triggers a rollout. | Confirm it after changing model configuration and wait for replacement Pods. |
| `status.logging` | Reflects whether the optional logging sidecar is enabled and which image it uses. | Confirm the setting before checking the logging sidecar or OKE Logging integration. |
| `status.conditions` | May contain detailed controller condition type, reason, and message data. It can be empty when no condition is reported. | Use the JSONPath command above together with Kubernetes events for the most specific failure reason. |

### Access the ClusterIP Service

Start port forwarding in one terminal:

```sh
kubectl port-forward "service/$NAME" 8443:8443 -n "$NAMESPACE"
```

For a self-signed certificate, `-k` is acceptable for this development test:

```sh
curl -k --noproxy '*' https://127.0.0.1:8443/health
```

For a trusted deployment, use the appropriate CA file instead of `-k`:

```sh
export API_KEY_FILE=/secure/path/api-key
export CA_FILE=/secure/path/ca.crt

curl --cacert "$CA_FILE" --noproxy '*' \
  --header "Authorization: Bearer $(<"$API_KEY_FILE")" \
  "https://127.0.0.1:8443/v1/models"
```

The operator E2E test `privateai/test01` validates the same basic shape:
authentication, TLS, one ready replica, default image models, and ClusterIP
access. See [`test/e2e/controllers/privateai/tests/01-provision/README.md`](../../test/e2e/controllers/privateai/tests/01-provision/README.md).

### Inspect the Generated Workload

```sh
kubectl get deployment,pods,svc -n pai -l app.kubernetes.io/instance=PrivateAi-pai-quickstart
kubectl get events -n pai --sort-by=.lastTimestamp
kubectl logs deployment/pai-quickstart -n pai --all-containers=true --prefix
```

## Before You Begin

> **Important — required prerequisite:** Complete every applicable step in this
> section before creating the `PrivateAi` resource. The deployment depends on
> the namespace, image pull Secret, authentication Secret, certificate issuer,
> and TLS Secret prepared here.

**In this section:**

- [Create the Namespace](#create-the-namespace)
- [Verify the Operator and Namespace Scope](#verify-the-operator-and-namespace-scope)
- [Create the Image-Pull Secret](#create-the-image-pull-secret)
- [Install and Verify cert-manager](#install-and-verify-cert-manager)
- [Create the Certificate Issuer](#create-the-certificate-issuer)
- [Create the Authentication Secret](#create-the-authentication-secret)
- [Create the TLS Secret](#create-the-tls-secret)
- [Configuration Choices](#configuration-choices)

The commands in this section are idempotent where possible. Set the variables
once and use the same values in the `PrivateAi` resource.

```sh
export NAMESPACE=pai
export NAME=pai-quickstart
export IMAGE_PULL_SECRET=oracle-container-registry-secret
export AUTH_SECRET=paisecret
export TLS_SECRET=privateai-tls
```

### Create the Namespace

```sh
kubectl create namespace "$NAMESPACE" \
  --dry-run=client -o yaml | kubectl apply -f -
```

### Verify the Operator and Namespace Scope

The Oracle AI Database Operator must be installed and must watch the namespace in
which the `PrivateAi` resource is created:

```sh
kubectl get crd privateais.privateai.oracle.com
kubectl get namespace "$NAMESPACE"
kubectl get deployment -n oracle-database-operator-system
```

For a namespace-scoped operator deployment, ensure `$NAMESPACE` is included in
the operator watch configuration and that the operator ServiceAccount can read
and update Private AI resources in that namespace. A ClusterIP Service does not
require a load-balancer subnet or external IP.

### Create the Image-Pull Secret

Accept the Oracle Container Registry license for the selected image, then run:

```sh
kubectl create secret docker-registry "$IMAGE_PULL_SECRET" \
  --namespace "$NAMESPACE" \
  --docker-server=container-registry.oracle.com \
  --docker-username='<oracle-sso-email-address>' \
  --docker-password='<container-registry-auth-token>' \
  --docker-email='<oracle-sso-email-address>' \
  --dry-run=client -o yaml | kubectl apply -f -
```

If the image pull Secret already exists, this updates it in place. Do not
commit registry credentials to a manifest or shell script. The Secret name
must match `spec.runtime.image.pullSecret`.

### Install and Verify cert-manager

The TLS helper requires cert-manager. Install it using your cluster's approved
platform procedure, then verify that the certificate APIs and workloads are
available:

```sh
kubectl get crd certificates.cert-manager.io clusterissuers.cert-manager.io
kubectl get deployment -n cert-manager
```

The certificate must contain every DNS name and IP address that clients use. A
certificate for a DNS name does not automatically validate an IP address.

### Create the Certificate Issuer

If an approved `Issuer` or `ClusterIssuer` does not already exist, save this as
`bootstrap-issuer.yaml`:

```yaml
apiVersion: cert-manager.io/v1
kind: ClusterIssuer
metadata:
  name: tcps-selfsigned-bootstrap
spec:
  selfSigned: {}
```

Apply and verify it:

```sh
kubectl apply -f bootstrap-issuer.yaml
kubectl get clusterissuer tcps-selfsigned-bootstrap
```

Use a company-approved CA or public issuer instead of a self-signed issuer for
production clients.

### Create the Authentication Secret

This step is required because the Quick Start sets `authEnabled: true`. The
helper generates one API key for this invocation and stores the API key and
the PKCS#12 password in the Secret:

```sh
cd /path/to/oracle-database-operator/docs/privateai/auth-secret

./create_auth_secret.sh create \
  --namespace "$NAMESPACE" \
  --secret-name "$AUTH_SECRET" \
  --generate-api-key \
  --json-api-keys \
  --key-id client1 \
  --alias QuickStart \
  --active true \
  --ssl-pwd-value '<PASSWORD>'
```

If authentication is disabled for a deliberate development use case, this
Secret and the `spec.security.secret` block are not required. The script writes
generated local files below the current directory by default. Protect and
remove those files after securely storing the API key. See [Authentication and
TLS](#authentication-and-tls) and the [auth Secret guide](./auth-secret/README.md)
for rotation and multiple-key operations.

### Create the TLS Secret

Create a certificate for the in-cluster Service names. `SECRET_NAME` is a
supported helper parameter and must match `spec.security.tls.secretName`:

```sh
cd /path/to/oracle-database-operator/docs/privateai/tls-cert-manager

NAMESPACE="$NAMESPACE" \
TARGET=pai \
SECRET_NAME="$TLS_SECRET" \
ISSUER_NAME=tcps-selfsigned-bootstrap \
ISSUER_KIND=ClusterIssuer \
COMMON_NAME="${NAME}.${NAMESPACE}.svc" \
DNS_NAMES="${NAME}.${NAMESPACE}.svc,${NAME}.${NAMESPACE}.svc.cluster.local" \
IP_ADDRESSES="" \
GENERATE_PKCS12=true \
PASSWORD_SECRET_NAME="$AUTH_SECRET" \
PASSWORD_SECRET_KEY=privateai-ssl-pwd \
./tr_cert_manager.sh
```

Keep `IP_ADDRESSES` empty when no client will connect through an allocated IP.

If the client will connect using port forwarding and use IP `127.0.0.1`, then
use value `127.0.0.1` for `IP_ADDRESSES` in above command.

If a load balancer is added later, issue a new certificate containing its IP
before clients connect through that address. See [TLS certificate generation](./tls-cert-manager/README.md).

### Configuration Choices

**In this section:**

- [Authentication and TLS](#authentication-and-tls)
- [Model Configuration](#model-configuration)
- [Resources, shared memory, and worker nodes](#resources-shared-memory-and-worker-nodes)
- [Workload and Pod annotations](#workload-and-pod-annotations)
- [Storage and Security](#storage-and-security)

> **Quick Start users:** After completing the mandatory prerequisite steps
> above, continue directly to [Quick Start](#quick-start) with the documented
> defaults. You may skip the configuration subsections below unless you need
> to customize authentication, TLS, models, or networking.

#### Authentication and TLS

##### Authentication Secret

Set `spec.security.authEnabled: true` and reference the authentication Secret:

```yaml
spec:
  security:
    authEnabled: true
    secret:
      name: paisecret
      mountLocation: /privateai/ssl
```

Use [Private AI Auth Secret](./auth-secret/README.md) to create, rotate, and
manage API keys. One `--generate-api-key` invocation creates one key. Use
`add-key` to add another key without replacing the existing key.

When the referenced Secret changes, the controller observes the new resource
version and reconciles the workload. Treat the Secret as sensitive: inspect
metadata and key names, never print Secret data in support logs or diagnostics.

##### TLS Secret

Reference a TLS Secret containing the server certificate and private key:

```yaml
spec:
  security:
    tls:
      secretName: privateai-tls
      mountLocation: /privateai/ssl
```

The helper in [TLS certificate generation](./tls-cert-manager/README.md)
supports `SECRET_NAME=privateai-tls`, SAN DNS names, IP SANs, and optional
`keystore.p12` generation. The TLS private key is used by the server; a normal
browser HTTPS client does not send that key. The client only validates the
server certificate. Client certificates are a separate mTLS requirement.

If a load balancer IP is assigned after deployment, regenerate the certificate
with the final IP in `IP_ADDRESSES` and update the referenced Secret. The
certificate must contain the hostname or IP used by the client.

#### Model Configuration

Use `spec.configuration.configFile` to mount a model configuration ConfigMap:

```yaml
spec:
  configuration:
    configFile:
      name: privateaiconfigjson
      mountLocation: /privateai/config
```

Without a configuration file, the container starts with the default models
included in the image. Available examples:

- [Specific pre-approved model](./configmap_specific_preapproved_model.md)
- [Single HTTPS model](./configmap_single_model_https.md)
- [Multiple HTTPS models](./configmap_multi_model_https.md)
- [Multiple filesystem models](./configmap_multi_model_filesystem.md)
- [Add a model](./deploy_privateai_multi_model_https_internallb_add_model.md)
- [Remove a model](./deploy_privateai_multi_model_https_internallb_remove_model.md)

The controller records the observed ConfigMap name and resource version in
status. After changing a model ConfigMap, verify the Pod configuration and
application logs before treating the deployment as ready.

#### Resources, shared memory, and worker nodes

Plan the node capacity before creating the `PrivateAi` resource. As a minimum
starting point for Private AI workloads, allocate **32 GiB of memory and 8 CPU
units (8 OCPU in the target compute environment) per container**. Workloads
that use larger models or higher concurrency may require more capacity.

Set matching requests and limits when the workload must receive this minimum
capacity:

```yaml
spec:
  runtime:
    resources:
      requests:
        cpu: "8"
        memory: 32Gi
      limits:
        cpu: "8"
        memory: 32Gi
```

GPU-backed workloads must also request and limit the GPU resource. The selected
worker nodes must have compatible NVIDIA hardware and a functioning NVIDIA
device plugin:

```yaml
spec:
  runtime:
    resources:
      requests:
        cpu: "8"
        memory: 32Gi
        nvidia.com/gpu: "1"
      limits:
        cpu: "8"
        memory: 32Gi
        nvidia.com/gpu: "1"
```

Use `spec.runtime.workerNodes` only with node names that can satisfy the full
request for every scheduled replica. For GPU workloads, verify the node's
allocatable GPU capacity before deployment:

```sh
kubectl get nodes -o custom-columns='NAME:.metadata.name,CPU:.status.allocatable.cpu,MEMORY:.status.allocatable.memory,GPU:.status.allocatable.nvidia\.com/gpu'
```

Example node selection:

```yaml
spec:
  runtime:
    workerNodes:
    - gpu-worker-01
```

If the requested CPU, memory, shared memory, or GPU is not available on the
selected nodes, the Pods remain unscheduled or pending. The operator does not
add GPU capacity to a node; capacity, drivers, device plugins, taints, and
tolerations must be prepared by the cluster administrator.

Set `spec.runtime.shmSize` when the application needs more than the container
runtime default at `/dev/shm`:

```yaml
spec:
  runtime:
    shmSize: 2Gi
```

The value must be a positive Kubernetes quantity such as `128Mi` or `2Gi`. The
memory-backed volume consumes part of the Pod's memory budget and must be
included in the node-capacity plan.

#### Workload and Pod annotations

Annotations can be applied to the generated Deployment and Pod template:

```yaml
spec:
  runtime:
    annotations:
      workload:
        company.com/change-ticket: CHG-12345
      pod:
        prometheus.io/scrape: "true"
        prometheus.io/port: "8443"
```

Annotations under the `privateai.oracle.com/` prefix are reserved for the
operator.

#### Storage and Security

##### PVC-backed model storage

Use `spec.storage.pvcList` for model or application storage:

```yaml
spec:
  storage:
    pvcList:
      <PVC NAME>: /privateai/models
```

See [Create OCI FSS based PVCs](./create_oci_fss_based_pvc.md) for storage
class, PVC, and filesystem examples.

##### PVC retention and deletion

`spec.storage.deletePvcOnDelete` controls deletion behavior for PVCs managed by
Private AI. Review this setting before deleting a production resource. A PVC
provided by the user should be treated as user-owned and must be backed up
before any lifecycle change.

## Networking and Access

Private AI Services can expose one or more Services through the `spec.networking.service` fields:

```yaml
spec:
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

`port` is the Kubernetes Service port that clients use to connect. When a load balancer is enabled, it exposes this Service port. `targetPort` is the port on which the Private AI container listens. In this example, traffic to Service port `443` is forwarded to the container’s HTTPS listener on port `8443`.

To access this Service through port forwarding, forward a local port to the Service port. For example, the following command forwards local port `8443` to Service port `443`:

```sh
kubectl port-forward service/<service-name> 8443:443 -n <namespace>
```

You can then access the service at `https://localhost:8443`.

The HTTPS listener defaults to port `8443`. When enabled, the HTTP listener defaults to port `8080`. The ClusterIP Service is enabled by default; public and private load balancers are disabled by default.

Use the dedicated guides for complete manifests and provider-specific settings:

- [Public load balancer](./deploy_privateai_publiclb.md)
- [Public load balancer without a ConfigMap](./deploy_privateai_publiclb_without_configmap.md)
- [Internal load balancer](./deploy_privateai_internallb.md)
- [Access the deployed service](./access_privateai.md)

For OCI load balancers, subnet annotations and external IP allocation are
provider operations. The Service may remain pending while the cloud provider
allocates the load balancer. Certificate SANs must be updated if clients use a
newly allocated IP.

## Private AI Services Container on OpenShift

For OpenShift deployments, configure the Private AI runtime with a dedicated
ServiceAccount. OpenShift selects the Security Context Constraint (SCC) through
that ServiceAccount. Use the cluster-approved SCC by default; create a custom
SCC only when the image or storage requires a fixed UID/GID.

Configure the Private AI resource with the ServiceAccount:

```yaml
spec:
  runtime:
    serviceAccountName: pai-sa
```

Grant the required SCC to `pai-sa` separately. The operator renders and
reconciles the ServiceAccount on the generated Deployment; do not patch the
Deployment directly. Refer to [Deploy Oracle Private AI on OpenShift](./deploy_privateai_on_openshift.md) for the complete procedure.

## Private AI v4 Resource Model

**In this section:**

- [Important Defaults and Validation](#important-defaults-and-validation)

Use the grouped configuration blocks to decide which part of a `PrivateAi`
resource to change:

| Area | Main fields | Purpose |
| --- | --- | --- |
| Authentication and TLS | `spec.security` | Auth and TLS Secret references |
| Runtime | `spec.runtime` | Image, replicas, resources, identity, debug, and worker nodes |
| Model configuration | `spec.configuration` | ConfigMap-based runtime configuration |
| Storage | `spec.storage` | PVC mounts, log location, and PVC deletion behavior |
| Networking | `spec.networking` | Listener ports, Services and load balancers |
| Logging | `spec.logging`, `spec.storage.logLocation` | Optional shared log volume and forwarder sidecar |

### Important defaults and validation

- If no listener is explicitly enabled, HTTPS is used by default.
- HTTPS defaults to port `8443`; HTTP defaults to port `8080` when enabled.
- `spec.configuration.configFile.mountLocation` defaults to
  `/privateai/config`.
- Auth and TLS mount locations default to `/privateai/ssl`.
- ClusterIP Service creation defaults to enabled.
- Public and private load balancers default to disabled.
- `spec.networking.trafficManager.routePath` defaults to
  `/<resource-name>/v1/` when omitted.
- Secret and ConfigMap mount locations must be absolute paths.
- `spec.runtime.shmSize` must be a positive Kubernetes quantity.
- `fsGroup` cannot be combined with `runAsUser` or `runAsGroup`; fixed UID and
  GID values must be supplied together.

The controller owns generated Deployment and Service state. User-provided
Secret, ConfigMap, and PVC data remains user-managed; update those objects
through their normal Kubernetes lifecycle and verify the resulting status.

## Deployment Scenarios and Lifecycle

Complete [Before You Begin](#before-you-begin) once. Choose one deployment
path below, replace every `<<...>>` placeholder, and keep all referenced
Secrets, ConfigMaps, and PVCs in the PrivateAi namespace.

**In this section:**

- [Deployment Scenarios](#deployment-scenarios)

### Deployment Scenarios

**In this section:**

- [Public Load Balancer with Model ConfigMap](#public-load-balancer-with-model-configmap)
- [Public Load Balancer with Image-Default Models](#public-load-balancer-with-image-default-models)
- [Public Load Balancer with Multiple Replicas](#public-load-balancer-with-multiple-replicas)
- [Internal Load Balancer](#internal-load-balancer)
- [Multiple HTTPS Models with Internal Load Balancer](#multiple-https-models-with-internal-load-balancer)
- [Filesystem Models with an Internal Load Balancer](#filesystem-models-with-an-internal-load-balancer)

#### Public Load Balancer with Model ConfigMap

Use this workflow to deploy a Private AI resource with a public HTTPS load balancer and model configuration supplied through a ConfigMap.

**Required prerequisites and example names**

> **Required before deployment**
>
> Complete every linked action in the **Complete before applying** column before applying the manifest.

| Requirement | Example value | Complete before applying |
| --- | --- | --- |
| Shared Private AI requirements | `Before You Begin` | **[Review Before You Begin](#before-you-begin)** |
| PrivateAi resource and namespace | `pai-sample` in `pai` | Set `metadata.name` and `metadata.namespace` consistently in the manifest. |
| Image and image-pull Secret | Private AI image / `oracle-container-registry-secret` | **[Create the image-pull Secret](#create-the-image-pull-secret)** and verify the image is accessible. |
| Authentication Secret | `paisecret` | [Create the authentication Secret](#create-the-authentication-secret) in `pai`. |
| TLS Secret and public IP | `privateai-tls` / reserved public IP | Reserve the public IP, then [create the TLS Secret](#create-the-tls-secret) in `pai` with the reserved IP in the certificate SAN. |
| Model ConfigMap | `privateaiconfigjson` | **[Create the model ConfigMap](#model-configuration)** in `pai` before applying the manifest. |

**Review Manifest Fields**

| Component | Fields to review | Required relationship |
| --- | --- | --- |
| Identity | metadata.name, metadata.namespace | The resource and all referenced objects use pai-sample and pai. |
| Security | spec.security.secret, spec.security.tls | paisecret and privateai-tls exist in the same namespace. |
| Runtime | spec.runtime.image, spec.runtime.image.pullSecret, spec.runtime.replicas | The image is accessible through oracle-container-registry-secret and replicas match capacity. |
| Models | spec.configuration.configFile.name, spec.configuration.configFile.mountLocation | The name matches privateaiconfigjson and the mount path is absolute. |
| Service | `spec.networking.listeners.https`, `spec.networking.service.ports` | HTTPS is exposed on Service/load-balancer port `443` and forwarded to the container listener on port `8443`. |
| Public endpoint | spec.networking.service.publicLoadBalancer.ip | Replace the placeholder with the certificate SAN address. |

Refer to [Public load balancer with model ConfigMap](./deploy_privateai_publiclb.md) for details.

#### Public Load Balancer with Image-Default Models

Use this workflow when the selected Private AI container image includes the models required for deployment. The model configuration is omitted; no model ConfigMap or model PVC is required for this example.

**Required prerequisites and example names**

> **Required before deployment**
>
> Complete every linked action in the **Complete before applying** column before applying the manifest.

| Requirement | Example value | Complete before applying |
| --- | --- | --- |
| Shared Private AI requirements | `Before You Begin` | Complete [Before You Begin](#before-you-begin). |
| PrivateAi resource and namespace | `pai-sample` in `pai` | Set `metadata.name` and `metadata.namespace` consistently in the manifest. |
| Image and image-pull Secret | Private AI image / `oracle-container-registry-secret` | **[Create the image-pull Secret](#create-the-image-pull-secret)** in `pai` and verify the image is accessible. |
| Authentication Secret | `paisecret` | [Create the authentication Secret](#create-the-authentication-secret) in `pai`. |
| TLS Secret and public IP | `privateai-tls` / reserved public IP | Reserve the public IP, then [create the TLS Secret](#create-the-tls-secret) in `pai` with the IP in the certificate SAN. |
| Model configuration | None | Do not create or reference a model ConfigMap. |

**Review Manifest Fields**

| Component | Fields to review | Required relationship |
| --- | --- | --- |
| Identity | `metadata.name`, `metadata.namespace` | The resource name is `pai-sample` and the namespace is `pai`. |
| Security | `spec.security.secret`, `spec.security.tls` | `paisecret` and `privateai-tls` exist in the `pai` namespace. |
| Runtime | `spec.runtime.image`, `spec.runtime.image.pullSecret`, `spec.runtime.replicas` | The selected image includes the required models, is accessible through `oracle-container-registry-secret`, and the cluster has capacity for the requested replicas. |
| Model configuration | `spec.configuration` | This block is omitted; no model ConfigMap is referenced. |
| Service | `spec.networking.listeners.https`, `spec.networking.service.ports` | HTTPS is exposed on Service/load-balancer port `443` and forwarded to the container listener on port `8443`. |
| Public endpoint | `spec.networking.service.publicLoadBalancer.ip` | Replace the placeholder with the reserved public IP address included in the certificate SAN. |

Refer to [Public load balancer with image-default models](./deploy_privateai_publiclb_without_configmap.md) for details.

#### Public Load Balancer with Multiple Replicas

Use this workflow to configure multiple Private AI Pods behind a public HTTPS load balancer. Ensure the cluster has capacity for all the Pods. If you require the Pods to run on separate nodes, configure scheduling constraints to spread them across nodes; a replica count alone does not guarantee this.

**Required prerequisites and example names**

> **Required before deployment**
>
> Complete every linked action in the **Complete before applying** column before applying the manifest.

| Requirement | Example value | Complete before applying |
| --- | --- | --- |
| Shared Private AI requirements | `Before You Begin` | Complete [Before You Begin](#before-you-begin). |
| Private AI resource and namespace | `pai-sample` in `pai` | Set `metadata.name` and `metadata.namespace` consistently in the manifest. |
| Image and image-pull Secret | Private AI image / `oracle-container-registry-secret` | [Create the image-pull Secret](#create-the-image-pull-secret) in `pai` and verify that the image is accessible. |
| Authentication Secret | `paisecret` | [Create the authentication Secret](#create-the-authentication-secret) in `pai`. |
| TLS Secret and public IP | `privateai-tls` / reserved public IP | Reserve the public IP, then [create the TLS Secret](#create-the-tls-secret) in `pai` with the IP in the certificate SAN. |
| Replica capacity and model access | Three Pods | Confirm capacity for three replicas and ensure each Pod can access the configured model files. |

**Review Manifest Fields**

| Component | Fields to review | Required relationship |
| --- | --- | --- |
| Identity | `metadata.name`, `metadata.namespace` | The resource name is `pai-sample` and the namespace is `pai`. |
| Runtime | `spec.runtime.image`, `spec.runtime.image.pullSecret`, `spec.runtime.replicas` | The image is accessible through `oracle-container-registry-secret`, and `replicas` is set to `3`. |
| Security | `spec.security.secret`, `spec.security.tls` | `paisecret` and `privateai-tls` exist in `pai`. |
| Models | Model configuration and model source | The configured model files are accessible to every replica. |
| Service | `spec.networking.listeners.https`, `spec.networking.service.ports` | HTTPS is exposed on Service/load-balancer port `443` and forwarded to the container listener on port `8443`. |
| Public endpoint | `spec.networking.service.publicLoadBalancer.ip` | Replace the placeholder with the reserved public IP address included in the certificate SAN. |

Refer to [Public load balancer with multiple replicas](./deploy_privateai_publiclb_multi_replica.md) for details.

#### Internal Load Balancer

Use this workflow to deploy Private AI with a private HTTPS endpoint accessible from a network that can reach the load balancer’s subnet.

**Required prerequisites and example names**

> **Required before deployment**
>
> Complete every linked action in the **Complete before applying** column before applying the manifest.

| Requirement | Example value | Complete before applying |
| --- | --- | --- |
| Shared Private AI requirements | `Before You Begin` | Complete [Before You Begin](#before-you-begin). |
| Private AI resource and namespace | `pai-sample` in `pai` | Set `metadata.name` and `metadata.namespace` consistently in the manifest. |
| Image and image-pull Secret | Private AI image / `oracle-container-registry-secret` | [Create the image-pull Secret](#create-the-image-pull-secret) in `pai` and verify that the image is accessible. |
| Authentication Secret | `paisecret` | [Create the authentication Secret](#create-the-authentication-secret) in `pai`. |
| TLS Secret and reserved private IP | `privateai-tls` / reserved private IP | Reserve a private IP in the load balancer's subnet, then [create the TLS Secret](#create-the-tls-secret) in `pai` with that IP in the certificate SAN. |
| Reserved private IP support | OKE Kubernetes 1.32 or later | Reserved private IP assignment to an internal load balancer is supported on OKE clusters running Kubernetes 1.32 or later. See [Configuring Load Balancers and Network Load Balancers](https://docs.oracle.com/en-us/iaas/Content/ContEng/Tasks/contengconfiguringloadbalancersnetworkloadbalancers-subtopic.htm). |

**Review manifest fields**

| Component | Fields to review | Required relationship |
| --- | --- | --- |
| Runtime | `spec.runtime.image`, `spec.runtime.replicas`, `spec.runtime.workerNodes` | The image is accessible through `oracle-container-registry-secret`. Replace the worker-node placeholder with eligible node names, or remove the field if you are not specifying nodes. |
| Security | `spec.security.secret`, `spec.security.tls` | `paisecret` and `privateai-tls` exist in `pai`. |
| Models | `spec.configuration.configFile.name` | The name matches the `privateaiconfigjson` ConfigMap in `pai`. |
| Service ports | `spec.networking.listeners.https`, `spec.networking.service.ports` | HTTPS is exposed on Service/load-balancer port `443` and forwarded to the container listener on port `8443`. |
| Private endpoint | `spec.networking.service.privateLoadBalancer.annotations` | Set `oci.oraclecloud.com/reserved-private-ips` to the reserved private IP address. |

Refer to [Internal load balancer](./deploy_privateai_internallb.md) for details.

#### Multiple HTTPS Models with Internal Load Balancer

Use this path when each Pod downloads multiple models from reachable HTTPS URLs.

**Required prerequisites and example names**

> **Required before deployment**
>
> Complete every linked action in the **Complete before applying** column before applying the manifest.

| Requirement | Example value | Complete before applying |
| --- | --- | --- |
| Internal load balancer | Reserved private IP | Review the [Internal Load Balancer requirements](#internal-load-balancer), including the OKE version and subnet requirements for reserved private IP assignment. |
| Private AI resource and namespace | `pai-sample` in `pai` | Set `metadata.name` and `metadata.namespace` consistently. Create the ConfigMap in the same namespace. |
| Image and image-pull Secret | Private AI image / `oracle-container-registry-secret` | [Create the image-pull Secret](#create-the-image-pull-secret) in `pai` and verify that the image is accessible. |
| Model ConfigMap | `multiconfigjson` | Follow [Create a ConfigMap for Multiple Models with HTTPS URLs](./configmap_multi_model_https.md) and create `multiconfigjson` in `pai`. |
| Model URL access | HTTPS model URLs | Confirm that every URL is valid and reachable from each Pod, and that its TLS certificate is trusted. |
| Authentication and TLS Secrets | `paisecret` / `privateai-tls` | Create both Secrets in `pai`. Ensure the certificate is valid for the reserved private IP used by clients. |

**Review manifest fields**

| Component | Fields to review | Required relationship |
| --- | --- | --- |
| Runtime | `spec.runtime.image`, `spec.runtime.replicas`, `spec.runtime.workerNodes` | Replace the worker-node placeholder with eligible node names, or remove it if you are not specifying nodes. |
| Models | `spec.configuration.configFile.name`, `spec.configuration.configFile.mountLocation` | The ConfigMap name is `multiconfigjson`, and the mount location is an absolute path. |
| Service ports | `spec.networking.listeners.https`, `spec.networking.service.ports` | HTTPS is exposed on Service/load-balancer port `443` and forwarded to the container listener on port `8443`. |
| Private endpoint | `spec.networking.service.privateLoadBalancer.annotations` | Set `oci.oraclecloud.com/reserved-private-ips` to the reserved private IP address. |

Refer to [Multiple HTTPS Models with Internal Load Balancer](./deploy_privateai_multi_model_https_internallb.md) for details.

For subsequent model changes, follow the add-model and remove-model procedures in this guide.

#### Filesystem Models with an Internal Load Balancer

Use this path when model files are stored on a mounted PVC, such as OCI File Storage.

**Required prerequisites and example names**

> **Required before deployment**
>
> Complete the linked actions in the **Complete before applying** column before applying the manifest.

| Requirement | Example value | Complete before applying |
| --- | --- | --- |
| Internal Load Balancer | `Internal Load Balancer` | [Review the Internal Load Balancer requirements](#internal-load-balancer). |
| PrivateAi resource and namespace | `pai-sample` in `pai` | Set `metadata.name` and `metadata.namespace` consistently. Create the ConfigMap and PVC in `pai`. |
| Image and image-pull Secret | Private AI image / `oracle-container-registry-secret` | [Create the image-pull Secret](#create-the-image-pull-secret) in `pai` and verify that the image is accessible. |
| OCI FSS Persistent Volume | `oke-fsspv` | Configure the PV with the OCI FSS CSI driver, filesystem, mount target, and export path. |
| Model PVC | `oke-fsspvc` | Create the PVC and confirm that its status is `Bound` before applying the PrivateAi manifest. |
| Model ConfigMap | `multiconfigjson` | [Create the filesystem-model ConfigMap](./configmap_multi_model_filesystem.md) in `pai`. Set the model paths to locations under `/privateai/models`. |
| Authentication and TLS | `paisecret` / `privateai-tls` | Create both Secrets in `pai`. The TLS certificate must be valid for the reserved private IP used by the Load Balancer. |

**Review manifest fields**

| Component | Fields to review | Required relationship |
| --- | --- | --- |
| Storage | spec.storage.pvcList | PVC oke-fsspvc mounts at /privateai/models. |
| Models | `spec.configuration.configFile.name`, `spec.configuration.configFile.mountLocation` | Use `multiconfigjson` and mount the configuration at `/privateai/config`. Model paths in the ConfigMap must point to files under `/privateai/models`. |
| Service | `spec.networking.service.privateLoadBalancer.annotations` | Replace the reserved-private-IP placeholder with the reserved private IP. |

Refer to [Deploy Private AI in OKE cluster using Multiple AI Models on File System and an Internal LoadBalancer](./deploy_privateai_multi_model_filesystem_internallb.md) for details.

## Other Model Configuration

**In this section:**

- [Model Configuration Options](#model-configuration-options)
- [Create the ConfigMap for Each Scenario](#create-the-configmap-for-each-scenario)
- [Prepare and Inspect the ConfigMap](#prepare-and-inspect-the-configmap)
- [Reference the ConfigMap from PrivateAi](#reference-the-configmap-from-privateai)
- [Validate the Model Configuration](#validate-the-model-configuration)
- [Scenario-Specific Requirements](#scenario-specific-requirements)

Private AI Services loads model selection and model-source settings from a
`config.json` file stored in a Kubernetes ConfigMap. Choose the model source that
fits the deployment, create the ConfigMap in the same namespace as the `PrivateAi`
resource, and reference it with `spec.configuration.configFile`.

For product-level model capabilities, supported runtimes, model formats, REST
endpoints, and service behavior, see the [Oracle Private AI Services Container
documentation](https://docs.oracle.com/en/database/oracle/oracle-database/26/prvai/oracle-private-ai-services-container.html). The local guides below provide the
Kubernetes and operator-specific steps.

### Model Configuration Options

| Scenario | ConfigMap | Model source | Guide |
| --- | --- | --- | --- |
| One model from an HTTPS URL | `privateaiconfigjson` | HTTPS model URL | [Single HTTPS model](./configmap_single_model_https.md) |
| Multiple models from HTTPS URLs | `multiconfigjson` | HTTPS model URLs | [Multiple HTTPS models](./configmap_multi_model_https.md) |
| Models from a mounted filesystem | `multiconfigjson` | Paths under the mounted model directory | [Filesystem models](./configmap_multi_model_filesystem.md) |
| Selected pre-approved model | Guide-defined name | Model included in the container image | [Pre-approved model](./configmap_specific_preapproved_model.md) |

### Create the ConfigMap for Each Scenario

From the repository root, run the command for the selected model source. Each command creates or updates a ConfigMap in the `pai` namespace and stores the example file under the key `config.json`. Use the ConfigMap name referenced by the Private AI resource.

```sh
# One HTTPS model
kubectl create configmap privateaiconfigjson \
  --from-file=config.json=docs/privateai/provisioning/single_model_https_config.json \
  -n pai --dry-run=client -o yaml | kubectl apply -f -

# Multiple HTTPS models
kubectl create configmap multiconfigjson \
  --from-file=config.json=docs/privateai/provisioning/multi_model_https_config.json \
  -n pai --dry-run=client -o yaml | kubectl apply -f -

# Multiple filesystem models
kubectl create configmap multiconfigjson \
  --from-file=config.json=docs/privateai/provisioning/multi_model_filesystem_config.json \
  -n pai --dry-run=client -o yaml | kubectl apply -f -

# Selected pre-approved model
kubectl create configmap privateaiconfigjson \
  --from-file=config.json=docs/privateai/provisioning/specific_preapproved_model.json \
  -n pai --dry-run=client -o yaml | kubectl apply -f -
```

Use one block at a time. Do not apply multiple model-source configurations to the same
resource unless the selected scenario explicitly requires them.

### Prepare and Inspect the ConfigMap

Use the example JSON file from the selected guide and create or update the ConfigMap declaratively. The command is
idempotent and avoids a delete-and-recreate gap:

```sh
kubectl create configmap privateaiconfigjson \
  --from-file=config.json=docs/privateai/provisioning/single_model_https_config.json \
  -n pai --dry-run=client -o yaml | kubectl apply -f -

kubectl get configmap privateaiconfigjson -n pai
kubectl get configmap privateaiconfigjson -n pai -o jsonpath="{.data.config.json}"
```

Use `multiconfigjson` instead when the selected guide specifies that name. Do
not print model credentials, private URLs, or other sensitive configuration in
support logs. Confirm the ConfigMap contains the expected `config.json` key and
valid JSON before applying the `PrivateAi` resource.

### Reference the ConfigMap from PrivateAi

Add the reference to the same manifest used by the selected deployment scenario:

```yaml
spec:
  configuration:
    configFile:
      name: privateaiconfigjson
      mountLocation: /privateai/config
```

For a focused update to an existing resource:

```sh
kubectl patch pai pai-sample -n pai --type merge \
  -p "{\"spec\":{\"configuration\":{\"configFile\":{\"name\":\"privateaiconfigjson\",\"mountLocation\":\"/privateai/config\"}}}}"
```

The ConfigMap and `PrivateAi` must be in the same namespace. The mount location
must be absolute and should match the path expected by the image and `config.json`.
If the resource already uses the deprecated `spec.paiConfigFile`, migrate to
`spec.configuration.configFile` instead of configuring both forms.

### Validate the Model Configuration

After creating or changing the ConfigMap, check the resource and generated
workload:

```sh
kubectl get pai pai-sample -n pai \
  -o jsonpath="status={.status.status} configMap={.status.paiConfigMap.name} version={.status.paiConfigMap.resourceVersion}{\"\n\"}"
kubectl describe pai pai-sample -n pai
kubectl get deployments,pods -n pai -l app.kubernetes.io/name=pai-sample
kubectl get events -n pai --sort-by=.lastTimestamp
```

Look for the expected ConfigMap name and a changed observed resource version, a
new Deployment generation or replacement Pods, successful model downloads or
mounts, and `status.status=Healthy`. If the status remains `Pending` or `Error`,
inspect conditions, Pod events, and controller logs before changing the model
JSON.

### Scenario-Specific Requirements

- **HTTPS model URLs:** Every Pod needs network access to each URL and must
  trust the serving certificate. Keep credentials out of the ConfigMap when the
  model endpoint requires authentication; use the product-supported mechanism.
- **Mounted filesystem models:** Create and validate the PVC before applying the
  `PrivateAi` manifest. The configured paths must exist under the mounted
  directory, and every replica must be able to read them.
- **Pre-approved models:** Select only models supported by the installed Private
  AI image and follow the product documentation for the exact model identifier
  and capabilities.
- **Model changes:** Treat a ConfigMap update as a workload change. Wait for
  the observed resource version and replacement Pods before sending requests.


## Runtime and Operations Scenarios

Patch the `PrivateAi` resource, not the controller-managed Deployment. After each
change, update the source manifest or GitOps definition, then inspect the
resource, conditions, generated Deployment, Pods, and events.

**In this section:**

- [Scale Private AI replicas](#scale-private-ai-replicas)
- [Set CPU and memory](#set-cpu-and-memory)
- [Change the container image](#change-the-container-image)
- [Label nodes and select worker nodes](#label-nodes-and-select-worker-nodes)
- [Enable debug or log forwarding](#enable-debug-or-log-forwarding)
- [Access the Private AI REST API](#access-the-private-ai-rest-api)
- [Delete a Private AI Deployment](#delete-a-private-ai-deployment)

### Scale Private AI Replicas

Use `spec.runtime.replicas` to change the desired number of Pods.

```sh
kubectl patch pai pai-sample -n pai --type merge -p "{\"spec\":{\"runtime\":{\"replicas\":3}}}"
```

Or update the durable manifest:

```yaml
spec:
  runtime:
    replicas: 3
```

Check the desired count, status, Deployment, Pods, and events:

```sh
kubectl get pai pai-sample -n pai -o jsonpath="status={.status.status} replicas={.status.replicas}{\"\n\"}"
kubectl get deployment,pods -n pai -l app.kubernetes.io/name=pai-sample
kubectl get events -n pai --sort-by=.lastTimestamp
```

Wait for `status.status=Healthy`; `status.replicas` is the desired target, not a
ready-Pod count. See [Scale out](./scale_out_privateai.md) and [scale in](./scale_in_privateai.md).

### Set CPU and Memory

Use `spec.runtime.resources` for runtime-container requests and limits.

```sh
kubectl patch pai pai-sample -n pai --type merge -p "{\"spec\":{\"runtime\":{\"resources\":{\"requests\":{\"cpu\":\"2\",\"memory\":\"8Gi\"},\"limits\":{\"cpu\":\"4\",\"memory\":\"16Gi\"}}}}}"
```

Check the rendered Deployment, Pod resource values, scheduling events, and
`status.status` after the rollout. See [Requests and limits](./deploy_privateai_publiclb_mem_cpu_limit.md).

### Change the Container Image

Use `spec.runtime.image.name`; update `pullSecret` in the same operation when
the registry credentials change.

```sh
kubectl patch pai pai-sample -n pai --type=merge \
  -p '{"spec":{"runtime":{"image":{"name":"container-registry.oracle.com/database/private-ai:medium-cpu-infer-latest","pullSecret":"oracle-container-registry-secret"}}}}'
```

Check the Deployment image, `imagePullPolicy`, Pod events, and registry Secret.
Resolve `ImagePullBackOff` before diagnosing the controller. See [Update the image](./change_privateai_container_image.md).

### Label Nodes and Select Worker Nodes

Use `spec.runtime.workerNodes` to constrain scheduling. By default, values are
matched against the `kubernetes.io/hostname` label. Alternatively, use
`spec.runtime.workerNodeSelector` to select nodes by one or more labels. The
two fields are mutually exclusive.

```yaml
spec:
  runtime:
    workerNodes:
    - worker-a
    - worker-b
```

For a quick update, patch the resource with the selected node names:

```sh
kubectl patch pai pai-sample -n pai --type merge -p \
  '{"spec":{"runtime":{"workerNodes":["worker-a","worker-b"]}}}'
```

To use label-based placement instead, patch `workerNodes` to `null` and set
`workerNodeSelector`:

```sh
kubectl patch pai pai-sample -n pai --type merge -p \
  '{"spec":{"runtime":{"workerNodes":null,"workerNodeSelector":{"privateai.oracle.com/pool":"inference"}}}}'
```

Node label changes affect scheduling when Pods are created. They do not evict or
move Pods that are already running on nodes whose labels no longer match. After
changing labels, restart the generated Deployment to recreate its Pods using
the current labels:

```sh
kubectl rollout restart deployment/pai-sample -n pai
kubectl rollout status deployment/pai-sample -n pai --timeout=15m
```

Verify the rendered affinity and actual Pod placement:

```sh
kubectl get deployment pai-sample -n pai \
  -o jsonpath='{.spec.template.spec.affinity.nodeAffinity.requiredDuringSchedulingIgnoredDuringExecution.nodeSelectorTerms[0].matchExpressions[0].values}{"\n"}'

kubectl get pods -n pai -l app.kubernetes.io/name=pai-sample \
  -o custom-columns=NAME:.metadata.name,NODE:.spec.nodeName,PHASE:.status.phase

kubectl get events -n pai --sort-by=.lastTimestamp
```

Every Pod `NODE` must satisfy the selected hostname or label constraints. A
`Healthy` status alone does not prove that placement matches the requested
nodes. If a Pod remains Pending, inspect capacity, taints and tolerations,
storage compatibility, and scheduling events.
See [Update worker nodes](./change_privateai_worker_node.md).

### Enable Debug or Log Forwarding

Use `spec.runtime.debug` temporarily for runtime diagnostics:

```sh
kubectl patch pai pai-quickstart -n pai --type merge -p "{\"spec\":{\"runtime\":{\"debug\":true}}}"
```

For log forwarding, both `spec.logging.enabled: true` and a non-empty
`spec.logging.sidecarImage` are required:

```yaml
spec:
  logging:
    enabled: true
    sidecarImage: registry.example.com/platform/log-forwarder:1.0
    volumeMount: /privateai/logs
```

Check `status.logging`, Pod containers, the shared log mount, sidecar image
pull events, and the configured log path. See [Debug](./debug_privateai.md) and [OKE Logging](./oke-logging.md).

### Access the Private AI REST API

Access Private AI through the endpoint appropriate to your deployment:

- From another Pod, use the generated Kubernetes Service and its Service port.
- From a client that can reach the network, use the public or private Load Balancer IP.
- For local testing, use `kubectl port-forward`.

Check the Private AI status and generated Service endpoints:

```sh
kubectl get pai pai-sample -n pai \
  -o jsonpath='status={.status.status} localService={.status.localService} privateLBService={.status.privateLoadBalancerService} publicLBService={.status.publicLoadBalancerService} externalService={.status.externalService} loadBalancerIP={.status.loadBalancerIP}{"\n"}'

kubectl get services -n pai -o wide
```

For local testing, forward local port `8443` to the Service port. This example assumes the Service exposes port `443`:

```sh
kubectl port-forward service/pai-sample 8443:443 -n pai
```

In another terminal, check the health endpoint:

```sh
curl -k -i https://127.0.0.1:8443/health
```

The port after the colon in the `kubectl port-forward` command is the Service port. If your Service exposes port `8443`, use `8443:8443` instead.

To test through a Load Balancer, use its IP address or hostname. The following assumes it listens on port `443`:

```sh
curl -k -i https://<load-balancer-ip-or-hostname>/health
```

`-k` skips certificate verification. For certificate validation, use a hostname or IP address included in the certificate SAN and provide the trusted CA with `--cacert`. If authentication is enabled and the endpoint returns `401`, include a valid API key.

For API commands and vector-feature workflows, see [Access Private AI](./access_privateai.md).

### Delete a Private AI Deployment

Delete the PrivateAi resource first and wait for the controller to finish
reconciliation:

```sh
kubectl delete pai pai-quickstart -n pai
kubectl get pods,svc,pvc -n pai
```

Do not remove finalizers manually unless the controller is unavailable and the
storage/data-retention decision has been made. A stuck resource should first be
diagnosed through operator logs and status conditions.

## Troubleshooting

**In this section:**

- [Common Private AI Errors](#common-private-ai-errors)

### Common Private AI Errors

| Symptom | Check | Typical action |
| --- | --- | --- |
| `PrivateAi` remains pending | `kubectl describe pai`, conditions, events | Check operator watch scope, image pull, referenced Secrets, and ConfigMaps |
| `ImagePullBackOff` | Pod events and `spec.runtime.image.pullSecret` | Recreate the registry Secret in the target namespace |
| TLS hostname or IP error | Certificate SANs and the endpoint being used | Reissue the certificate with the actual DNS name or IP |
| Load balancer remains pending | Service events and OCI subnet annotations | Check subnet, permissions, quota, and provider events |
| API returns unauthorized | Auth Secret keys and API key file | Verify the Secret name, active key, and Authorization header |
| Model is missing | ConfigMap name, mount location, and container logs | Validate the model ConfigMap and application configuration |
| PVC permission denied | Pod security context, SCC, and storage backend | Prefer SCC-assigned identity or `fsGroup` on OpenShift |
| Secret or ConfigMap was updated but workload is stale | Resource version, Deployment generation, and Pod age | Check controller logs and wait for reconciliation/rollout |

For operator-level investigation, use:

```sh
kubectl logs -n oracle-database-operator-system \
  -l control-plane=controller-manager --prefix --since=30m
```

See [Debug and troubleshoot](./debug_privateai.md) for the basic Pod-level
commands and [Read Private AI logs with OKE Logging](./oke-logging.md) for OKE
logging.

## Additional Information

**In this section:**

- [Private AI Services CRD Field Reference](#private-ai-services-crd-field-reference)
- [Compatibility Notes](#compatibility-notes)
- [Related Documentation](#related-documentation)

### Private AI Services CRD Field Reference

The [annotated Private AI Services CRD field reference](./reference/privateai-crd-reference.yaml)
is a documentation-only lookup for the preferred grouped fields, deprecated
compatibility fields, defaults, validation rules, mutually exclusive settings,
controller-managed resources, and status fields.

Do not apply the reference file unchanged. Replace placeholders, remove unused
alternatives, and use the scenario guides for copy-pasteable manifests. The
generated CRD at
`config/crd/bases/privateai.oracle.com_privateais.yaml` and the Go API types are
authoritative. Status fields are controller-owned.

### Compatibility Notes

The API still accepts older flat fields such as `paiConfigFile`,
`paiEnableAuthentication`, `paiSecret`, `paiImage`, `replicas`, `paiService`,
and `isExternalSvc`, but those fields are deprecated.

For new manifests, prefer:

- `spec.security`;
- `spec.runtime`;
- `spec.configuration`;
- `spec.storage`; and
- `spec.networking`.

Avoid mixing deprecated flat fields with grouped fields unless the example is
specifically testing backward compatibility. If an older manifest conflicts
with a grouped field, migrate the manifest to the grouped form before
troubleshooting the resulting workload.

### Related Documentation

- [Private AI Services CRD field reference](./reference/privateai-crd-reference.yaml)
- [Private AI Auth Secret](./auth-secret/README.md)
- [TLS certificate generation](./tls-cert-manager/README.md)
- [Agent Factory Controller](../agentfactory/README.md)

## Known Issues

1. **Multiple active API keys:**

   When multiple API keys are configured in the authentication Secret, requests to the Private AI service may fail with `HTTP 401` (`Invalid API Key`) when using either key. The cause is under investigation.
