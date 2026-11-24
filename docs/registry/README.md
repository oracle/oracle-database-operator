# Pull Images from Oracle Container Registry

This guide explains how to authenticate to the [Oracle Container Registry (OCR)](https://container-registry.oracle.com/) and create a Kubernetes image pull Secret.

## Prerequisites

- An Oracle Account with access to the required image repository.
- For licensed Oracle software, you must also accept the applicable Oracle Standard Terms and Restrictions in the OCR web interface.
- An Oracle authentication token. Since June 30, 2025, OCR CLI login requires an auth token for SSO users; an SSO password cannot be used.
- Docker or Podman installed if you want to pull images locally.
- `kubectl` configured to access the target Kubernetes cluster.
- Permission to create Secrets in the target namespace.

## 1. Accept the license terms

For licensed images (including Oracle Database Enterprise images used by SIDB
and True Cache samples):

1. Sign in to the [Oracle Container Registry](https://container-registry.oracle.com/).
2. Locate the required product repository under **Database** (for example
   `enterprise_ru`, `enterprise`, `free`, or `express`).
3. Accept the Oracle Standard Terms and Restrictions for that repository.

You can skip this step if you have already accepted the terms for the repository. This step is generally not required for open-source images unless the repository states otherwise.

SIDB True Cache samples typically need access to
`container-registry.oracle.com/database/enterprise_ru`. Accept terms for every
repository path your manifests reference under `spec.image.pullFrom`.

## 2. Generate an authentication token

After signing in to OCR, open your user profile and generate an authentication token.

Copy the token immediately and store it securely. It might not be displayed again after you leave the page.

If you use an SSO account, use an authentication token as the password.

Do not commit the token to source control or include it in scripts, logs, Kubernetes manifests, or README files.

## 3. Log in to OCR

Logging in locally verifies your credentials and is required if you create the Kubernetes Secret from an existing Docker or Podman authentication file.

With Docker:

```bash
docker login container-registry.oracle.com
```

With Podman:

```bash
podman login container-registry.oracle.com
```

When prompted, enter:

- **Username:** Your Oracle or SSO username
- **Password:** Your Oracle authentication token

## 4. Pull an image locally

OCR image references use the following format:

```text
container-registry.oracle.com/<area>/<image>:<tag>
```

### SIDB and True Cache sample images

SIDB sample YAMLs under `config/samples/sidb/` pull licensed Oracle Database
images from the OCR `database` area. Accept the license terms for that
repository before you pull.

True Cache capable SIDB samples (primary and True Cache) use Enterprise RU, for
example:

```bash
# Docker
docker pull container-registry.oracle.com/database/enterprise_ru:latest

# Podman
podman pull container-registry.oracle.com/database/enterprise_ru:latest
```

Other SIDB sample `image.pullFrom` values you may need to accept and pull:

| Sample use | Example OCR reference |
| --- | --- |
| True Cache / Enterprise RU (most SIDB and True Cache samples) | `container-registry.oracle.com/database/enterprise_ru:latest` |
| Enterprise (version-pinned samples) | `container-registry.oracle.com/database/enterprise:23.26.0.0` |
| Enterprise 19c-style tags | `container-registry.oracle.com/database/enterprise_ru:latest-19` |
| Oracle Database Free | `container-registry.oracle.com/database/free:latest` |
| Oracle Database Free lite | `container-registry.oracle.com/database/free:latest-lite` |
| Express Edition | `container-registry.oracle.com/database/express:latest` |

Match `spec.image.pullFrom` in your SIDB manifest to the repository and tag you
accepted in OCR. Prefer an explicit tag from the OCR page when you need a
predictable version; sample `:latest` tags move over time.

The OCR repository page for each product lists the exact path and available
tags. Generic OS images such as `container-registry.oracle.com/os/oraclelinux:9-slim`
are not the SIDB or True Cache database images used by OraOperator samples.

## 5. Create a Kubernetes image pull Secret

Kubernetes uses a Secret of type `kubernetes.io/dockerconfigjson` to authenticate to a private container registry.

The Secret must be created in the **same namespace** as the SIDB, True Cache,
ORDS, or other workload that references it. Creating `ocr-reg-cred` only in
`default` (or the operator namespace) does not help a SIDB in another namespace.

The following examples create a Secret named `ocr-reg-cred`. Replace
`<namespace>` with the resource’s `metadata.namespace`.

### Option 1: Provide the credentials directly

```bash
kubectl create secret docker-registry ocr-reg-cred \
  --docker-server=container-registry.oracle.com \
  --docker-username='<oracle-username>' \
  --docker-password='<auth-token>' \
  --namespace='<namespace>'
```

Replace:

- `<oracle-username>` with your Oracle or SSO username.
- `<auth-token>` with your Oracle Container Registry authentication token.
- `<namespace>` with the target Kubernetes namespace.

> **Security warning:** Supplying a token directly on the command line may store it in your shell history. It might also be visible to other users on the system while `kubectl` is running.

### Option 2: Use an existing Docker configuration

After successfully logging in with Docker, create the Secret from your Docker configuration:

```bash
kubectl create secret generic ocr-reg-cred \
  --from-file=.dockerconfigjson="$HOME/.docker/config.json" \
  --type=kubernetes.io/dockerconfigjson \
  --namespace='<namespace>'
```

Confirm that the file exists at that location before running the command.

If the Docker configuration uses an external credential store and contains a `credsStore` or `credHelpers` entry instead of an `auth` entry for OCR, Kubernetes cannot use the file directly. In that case, create the Secret by providing the credentials as shown in Option 1.

> **Security warning:** The authentication file might contain credentials for other registries. Ensure that you are not unintentionally copying unrelated credentials into the Kubernetes Secret.

### Option 3: Use an existing Podman authentication file

After successfully logging in with Podman, locate its authentication file. On many Linux systems, the default location is:

```text
${XDG_RUNTIME_DIR}/containers/auth.json
```

Create the Secret using the confirmed path:

```bash
kubectl create secret generic ocr-reg-cred \
  --from-file=.dockerconfigjson="${XDG_RUNTIME_DIR}/containers/auth.json" \
  --type=kubernetes.io/dockerconfigjson \
  --namespace='<namespace>'
```

The Podman authentication-file location can vary by operating system and configuration. Confirm the path in your environment before creating the Secret.

> **Security warning:** The authentication file might contain credentials for other registries. Ensure that you are not unintentionally copying unrelated credentials into the Kubernetes Secret.

## 6. Verify the Secret

Confirm that the Secret exists in the expected namespace:

```bash
kubectl get secret ocr-reg-cred \
  --namespace='<namespace>'
```

The Secret type should be `kubernetes.io/dockerconfigjson`.

Do not print or decode the Secret in shared terminals, logs, or support requests.

## 7. Use the Secret in a Kubernetes workload

### SIDB / True Cache (OraOperator)

Reference the Secret on the SIDB resource with `spec.image.pullSecrets`, and set
`spec.image.pullFrom` to the OCR database image used by your sample:

```yaml
spec:
  image:
    pullFrom: container-registry.oracle.com/database/enterprise_ru:latest
    pullSecrets: ocr-reg-cred
```

Licensed OCR Database images such as `database/enterprise_ru` require an
authenticated pull (accept OCR terms and use an auth token Secret). Anonymous
pull of those images is not supported. Only publicly pullable OCR images can
omit `pullSecrets`.

Create `ocr-reg-cred` in the same namespace as the SIDB (`metadata.namespace`).
If the primary and True Cache SIDBs run in different namespaces, create the
pull Secret in each of those namespaces. A Secret in another namespace is not
used for image pull.

See also [SIDB prerequisites — image pull secret](../sidb/prerequisites/PREREQUISITES.md#image-pull-secret)
and the True Cache samples under `config/samples/sidb/`.

### Generic Deployment example

For a plain Kubernetes Deployment, use `imagePullSecrets`:

```yaml
spec:
  template:
    spec:
      imagePullSecrets:
        - name: ocr-reg-cred
      containers:
        - name: <container-name>
          image: container-registry.oracle.com/database/enterprise_ru:latest
```

If you are using a provided YAML template, set its Oracle Container Registry Secret parameter to `ocr-reg-cred`.

## Troubleshooting

### `unauthorized` or `authentication required`

- Confirm that you used an authentication token rather than an SSO password.
- Verify that the username and token are correct.
- Confirm that your Oracle Account has access to the repository.
- For licensed images, verify that you accepted the terms for the exact repository.
- Log out and log in again to replace stale locally stored credentials.

### `manifest unknown` or `not found`

- Confirm the repository area, image name, and tag exactly as shown in OCR.
- Check the spelling and letter case. Repository paths are generally lowercase.
- Verify that the requested tag exists.

### `ImagePullBackOff`

Inspect the workload events:

```bash
kubectl describe pod <pod-name> \
  --namespace='<namespace>'
```

Confirm that:

- The `ocr-reg-cred` Secret exists in the workload namespace.
- For SIDB / True Cache, `spec.image.pullSecrets` is set to that Secret name
  (string, for example `ocr-reg-cred`); for a plain Deployment, check
  `imagePullSecrets`.
- `spec.image.pullFrom` (or the container `image`) matches an OCR database
  repository and tag you can access (for True Cache samples, typically
  `database/enterprise_ru`).
- The credentials stored in the Secret are current.
- The applicable OCR license terms have been accepted for that repository.

### Pulling from a restricted network

Ensure that the client machine and Kubernetes worker nodes can establish HTTPS connections to `container-registry.oracle.com` and that applicable proxies or firewalls permit registry traffic.

## Official documentation

- [Oracle Container Registry (OCR)](https://container-registry.oracle.com/)
- [Using Docker Registries — Oracle Linux](https://docs.oracle.com/en/operating-systems/oracle-linux/docker/docker-UsingDockerRegistries.html)
- [Pull an Image from a Private Registry — Kubernetes](https://kubernetes.io/docs/tasks/configure-pod-container/pull-image-private-registry/)
- [Pulling Images Using the Docker CLI — OCI Container Registry](https://docs.oracle.com/en-us/iaas/Content/Registry/Tasks/registrypullingimagesusingthedockercli.htm)
- [`kubectl create secret docker-registry` — Kubernetes](https://kubernetes.io/docs/reference/kubectl/generated/kubectl_create/kubectl_create_secret_docker-registry/)

> **Note:** The OCI Container Registry documentation covers OCI tenancy-based registry endpoints such as `ocir.<region>.oci.oraclecloud.com`. This README covers the separate public Oracle Container Registry endpoint, `container-registry.oracle.com`.
