# Oracle Database Operator for Kubernetes: Manual Build and Installation

This guide describes how to build the Oracle Database Operator image from this checkout and install the generated manifests in a Kubernetes cluster.

## System requirements

Use Oracle Linux 8 (8.x) or Oracle Linux 9 (9.x). The minimum supported kernel is `5.15.0-320.202.8.4.el8uek.x86_64` or a later compatible UEK kernel.

Verify the operating system and kernel:

```sh
cat /etc/os-release
uname -r
```

The build also requires Git, Go 1.26.5, Podman or Docker with BuildKit support, `kubectl`, and access to the image registry where the operator image will be pushed.

## Install Go 1.26.5

Download Go 1.26.5 from the [official Go downloads](https://go.dev/dl/) page, extract it under `/usr/local`, and add it to `PATH`:

```sh
tar -C /usr/local -xzf go1.26.5.linux-amd64.tar.gz
export PATH=/usr/local/go/bin:$PATH
go version
```

The output must report `go1.26.5`. Add the `PATH` update to your shell profile if it should persist across sessions. The operator uses Go modules; setting `GOPATH` is optional.

## Clone the operator

```sh
git clone https://github.com/oracle/oracle-database-operator.git
cd oracle-database-operator
git checkout <branch-or-commit>
```

Run all Makefile commands from the repository root.

## Prepare the registry and image name

Set an image reference reachable by the Kubernetes nodes. The OCI registry format is `<region-key>.ocir.io/<tenancy-namespace>/<repository>/<image>:<tag>`.

```sh
export DOCKER=podman
export REGISTRY=<region-key>.ocir.io
export IMG="${REGISTRY}/<tenancy-namespace>/<repository>/oracle-database-operator:<tag>"
```

Use `DOCKER=docker` instead if Docker is the selected container tool. Log in without putting the token in shell history:

```sh
export OCI_USERNAME='<tenancy-namespace>/<oci-username>'
export OCI_AUTH_TOKEN='<oci-auth-token>'
printf '%s' "$OCI_AUTH_TOKEN" | "$DOCKER" login "$REGISTRY" \
  --username "$OCI_USERNAME" --password-stdin
```

## Build the operator image with Makefile targets

The current Makefile uses `image-build` and `image-push`. The older `make docker-build` target is not available. The Dockerfile has `prod` and `debug` targets; use `prod` for a normal deployment.

Generate code and manifests, then build a production image using the current Go version and Oracle Linux 9 builder:

```sh
export GOLANG_VERSION=1.26.5

make generate
make manifests

make image-build \
  DOCKER="$DOCKER" \
  IMG="$IMG" \
  GOLANG_VERSION="$GOLANG_VERSION" \
  BUILD_INTERNAL=true \
  DEBUG=false \
  TARGET=prod
```

`BUILD_INTERNAL=true` uses `oraclelinux:9` as the builder and downloads Go 1.26.5 inside the build when necessary. The runtime image is based on `oraclelinux:9-slim`, runs as the non-root user with UID/GID 1002, and contains the production manager binary without the Delve debugger.

Push the image:

```sh
make image-push DOCKER="$DOCKER" IMG="$IMG"
```

For a debug image, use the Dockerfile `debug` target explicitly:

```sh
make image-build \
  DOCKER="$DOCKER" \
  IMG="${IMG}-debug" \
  GOLANG_VERSION="$GOLANG_VERSION" \
  BUILD_INTERNAL=true \
  DEBUG=true \
  TARGET=debug
```

For a multi-architecture image, use manifest mode:

```sh
make image-build image-push \
  DOCKER="$DOCKER" \
  IMG="$IMG" \
  GOLANG_VERSION="$GOLANG_VERSION" \
  BUILD_INTERNAL=true \
  DEBUG=false \
  TARGET=prod \
  BUILD_MANIFEST=true
```

## Equivalent direct container build

If Make is not available, this is the equivalent production build for an internal Oracle Linux builder and an amd64 image:

```sh
"$DOCKER" build \
  --pull=missing \
  --platform=linux/amd64 \
  --build-arg BUILDER_IMG=oraclelinux:9 \
  --build-arg GOLANG_VERSION=1.26.5 \
  --build-arg INSTALL_GO=true \
  --build-arg DEBUG=false \
  --target prod \
  --tag "$IMG" .

"$DOCKER" push "$IMG"
```

## Generate installation manifests

Generate manifests after the image has been built and pushed. The `operator-yaml` target sets the manager image in Kustomize and produces:

- `<checkout-directory>-rbac.yaml`: namespace, ServiceAccount, Roles, ClusterRoles, and bindings
- `<checkout-directory>-system.yaml`: CRDs, webhooks, services, certificates, network policy, and the operator Deployment
- `<checkout-directory>.yaml`: combined compatibility manifest

For a checkout directory named `oracle-database-operator`, run:

```sh
make operator-yaml IMG="$IMG" GOLANG_VERSION="$GOLANG_VERSION"
```

Review the generated image reference before applying the manifests:

```sh
rg -n "image:|WATCH_NAMESPACE|serviceAccountName" \
  oracle-database-operator-system.yaml
```

## Create the image pull secret and install

Create the operator namespace and registry pull secret before starting the Deployment:

```sh
kubectl create namespace oracle-database-operator-system \
  --dry-run=client -o yaml | kubectl apply -f -

kubectl create secret docker-registry container-registry-secret \
  -n oracle-database-operator-system \
  --docker-server="$REGISTRY" \
  --docker-username="$OCI_USERNAME" \
  --docker-password="$OCI_AUTH_TOKEN" \
  --dry-run=client -o yaml | kubectl apply -f -
```

For the default namespace-scoped installation, apply RBAC first and the system manifest second:

```sh
kubectl apply -f oracle-database-operator-rbac.yaml
kubectl apply -f oracle-database-operator-system.yaml
```

The generated deployment is namespace-scoped by default. To watch additional namespaces, generate matching namespace RoleBindings and set the complete `WATCH_NAMESPACE` list as described in the [main installation guide](../../README.md#choose-deployment-scope).

Wait for the operator:

```sh
kubectl rollout status deployment/oracle-database-operator-controller-manager \
  -n oracle-database-operator-system --timeout=300s
kubectl get pods -n oracle-database-operator-system
```

## Install CRDs only or run locally

`make install` installs the generated CRDs into the cluster configured in `KUBECONFIG` or `~/.kube/config`. It does not build or push the image:

```sh
make install
```

To run the manager locally against the configured cluster instead of deploying the image:

```sh
make generate
make manifests
make install
make run
```

For controller-specific prerequisites and custom resources, see the guides in the repository `docs/` directory and the [main README](../../README.md).
