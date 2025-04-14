#
# Copyright (c) 2025, Oracle and/or its affiliates.
# Licensed under the Universal Permissive License v 1.0 as shown at http://oss.oracle.com/licenses/upl.
#

# Current Operator version
VERSION ?= 1.2.0
# Default bundle image tag
BUNDLE_IMG ?= controller-bundle:$(VERSION)
# Options for 'bundle-build'
ifneq ($(origin CHANNELS), undefined)
BUNDLE_CHANNELS := --channels=$(CHANNELS)
endif
ifneq ($(origin DEFAULT_CHANNEL), undefined)
BUNDLE_DEFAULT_CHANNEL := --default-channel=$(DEFAULT_CHANNEL)
endif
BUNDLE_METADATA_OPTS ?= $(BUNDLE_CHANNELS) $(BUNDLE_DEFAULT_CHANNEL)

# Image URL to use all building/pushing image targets
IMG ?= controller:latest
# Enable allowDangerousTypes to use float type in CRD
# Remove the Desc to avoid YAML getting too long. See the discussion:
# https://github.com/kubernetes-sigs/kubebuilder/issues/1140 
CRD_OPTIONS ?= "crd:maxDescLen=0,allowDangerousTypes=true"
# ENVTEST_K8S_VERSION refers to the version of kubebuilder assets to be downloaded by envtest binary.
ENVTEST_K8S_VERSION = 1.29.0
# Operator YAML file
OPERATOR_YAML=$$(basename $$(pwd)).yaml
# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set)
ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif

# Setting SHELL to bash allows bash commands to be executed by recipes.
# This is a requirement for 'setup-envtest.sh' in the test target.
# Options are set to exit when a recipe line exits non-zero or a piped command fails.
SHELL = /usr/bin/env bash -o pipefail
.SHELLFLAGS = -ec

all: build
##@ Development

manifests: controller-gen ## Generate WebhookConfiguration, ClusterRole and CustomResourceDefinition objects.
	$(CONTROLLER_GEN) $(CRD_OPTIONS) rbac:roleName=manager-role webhook paths="./..." output:crd:artifacts:config=config/crd/bases
 
generate: controller-gen ## Generate code containing DeepCopy, DeepCopyInto, and DeepCopyObject method implementations.
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="./..."
 
fmt: ## Run go fmt against code.
	go fmt ./...
 
vet: ## Run go vet against code.
	go vet ./...
 
TEST ?= ./apis/database/v1alpha1 ./commons/... ./controllers/...
test: manifests generate fmt vet envtest ## Run unit tests.
	KUBEBUILDER_ASSETS="$(shell $(ENVTEST) use $(ENVTEST_K8S_VERSION) -p path)" go test $(TEST) -coverprofile cover.out
 
E2ETEST ?= ./test/e2e/
e2e: manifests generate fmt vet envtest ## Run e2e tests.
	KUBEBUILDER_ASSETS="$(shell $(ENVTEST) use $(ENVTEST_K8S_VERSION) -p path)" go test $(E2ETEST) -test.timeout 0 -test.v --ginkgo.fail-fast
 
##@ Build
 
build: generate fmt vet ## Build manager binary.
	go build -o bin/manager main.go
 
run: manifests generate fmt vet ## Run a controller from your host.
	go run ./main.go
 
GOLANG_VERSION ?= 1.23.3
## Download golang in the Dockerfile if BUILD_INTERNAL is set to true.
## Otherwise, use golang image from docker hub as the builder.
ifeq ($(BUILD_INTERNAL), true)
BUILDER_IMG = oraclelinux:9
BUILD_ARGS = --build-arg BUILDER_IMG=$(BUILDER_IMG) --build-arg GOLANG_VERSION=$(GOLANG_VERSION) --build-arg INSTALL_GO=true
else
BUILDER_IMG = golang:$(GOLANG_VERSION)
BUILD_ARGS = --build-arg BUILDER_IMG=$(BUILDER_IMG) --build-arg INSTALL_GO="false" --build-arg GOLANG_VERSION=$(GOLANG_VERSION)
endif
ifeq ($(BUILD_MANIFEST), true)
BUILD_ARGS := $(BUILD_ARGS) --platform=linux/arm64,linux/amd64 --jobs=2 --manifest
PUSH_ARGS := manifest
else
BUILD_ARGS := $(BUILD_ARGS) --platform=linux/amd64 --tag
endif
docker-build: #manifests generate fmt vet #test ## Build docker image with the manager. Disable the test but keep the validations to fail fast
	docker build --no-cache=true --build-arg http_proxy=$(HTTP_PROXY) --build-arg https_proxy=$(HTTPS_PROXY) \
                     --build-arg CI_COMMIT_SHA=$(CI_COMMIT_SHA) --build-arg CI_COMMIT_BRANCH=$(CI_COMMIT_BRANCH) \
                     $(BUILD_ARGS) $(IMG) .
 
docker-push: ## Push docker image with the manager.
	docker $(PUSH_ARGS) push $(IMG)

# Push to minikube's local registry enabled by registry add-on
minikube-push:
	docker tag $(IMG) $$(minikube ip):5000/$(IMG)
	docker push --tls-verify=false $$(minikube ip):5000/$(IMG)

##@ Deployment
 
install: manifests kustomize ## Install CRDs into the K8s cluster specified in ~/.kube/config.
	$(KUSTOMIZE) build config/crd | kubectl apply -f -
 
uninstall: manifests kustomize ## Uninstall CRDs from the K8s cluster specified in ~/.kube/config.
	$(KUSTOMIZE) build config/crd | kubectl delete -f -
 
deploy: manifests kustomize ## Deploy controller to the K8s cluster specified in ~/.kube/config.
	cd config/manager && $(KUSTOMIZE) edit set image controller=$(IMG)
	$(KUSTOMIZE) build config/default | kubectl apply -f -

minikube-deploy: minikube-operator-yaml minikube-push
	kubectl apply -f $(OPERATOR_YAML)

# Bug:34265574
# Used sed to reposition the controller-manager Deployment after the certificate creation in the OPERATOR_YAML 
operator-yaml: manifests kustomize
	cd config/manager && $(KUSTOMIZE) edit set image controller=$(IMG)
	$(KUSTOMIZE) build config/default > "$(OPERATOR_YAML)"
	sed -i.bak -e '/^apiVersion: apps\/v1/,/---/d' "$(OPERATOR_YAML)"
	(echo --- && sed '/^apiVersion: apps\/v1/,/---/!d' "$(OPERATOR_YAML).bak")  >>  "$(OPERATOR_YAML)"
	rm "$(OPERATOR_YAML).bak"

minikube-operator-yaml: IMG:=localhost:5000/$(IMG)
minikube-operator-yaml: operator-yaml
	sed -i.bak 's/\(replicas.\) 3/\1 1/g' "$(OPERATOR_YAML)"
	rm "$(OPERATOR_YAML).bak"

undeploy: ## Undeploy controller from the K8s cluster specified in ~/.kube/config.
	$(KUSTOMIZE) build config/default | kubectl delete -f -
 
##@ Build Dependencies
 
## Location to install dependencies to
LOCALBIN ?= $(shell pwd)/bin
$(LOCALBIN):
	mkdir -p $(LOCALBIN)
 
## Tool Binaries
KUSTOMIZE ?= $(LOCALBIN)/kustomize
CONTROLLER_GEN ?= $(LOCALBIN)/controller-gen
ENVTEST ?= $(LOCALBIN)/setup-envtest
 
## Tool Versions
KUSTOMIZE_VERSION ?= v5.3.0
CONTROLLER_TOOLS_VERSION ?= v0.16.5
 
KUSTOMIZE_INSTALL_SCRIPT ?= "https://raw.githubusercontent.com/kubernetes-sigs/kustomize/master/hack/install_kustomize.sh"
.PHONY: kustomize
kustomize: $(KUSTOMIZE) ## Download kustomize locally if necessary.
$(KUSTOMIZE): $(LOCALBIN)
	curl -s $(KUSTOMIZE_INSTALL_SCRIPT) | bash -s -- $(subst v,,$(KUSTOMIZE_VERSION)) $(LOCALBIN)
 
.PHONY: controller-gen
controller-gen: $(CONTROLLER_GEN) ## Download controller-gen locally if necessary.
$(CONTROLLER_GEN): $(LOCALBIN)
	GOBIN=$(LOCALBIN) go install sigs.k8s.io/controller-tools/cmd/controller-gen@$(CONTROLLER_TOOLS_VERSION)
 
.PHONY: envtest
envtest: $(ENVTEST) ## Download envtest-setup locally if necessary.
$(ENVTEST): $(LOCALBIN)
	GOBIN=$(LOCALBIN) go install sigs.k8s.io/controller-runtime/tools/setup-envtest@latest
 
 
.PHONY: bundle
bundle: manifests kustomize ## Generate bundle manifests and metadata, then validate generated files.
	operator-sdk generate kustomize manifests -q
	cd config/manager && $(KUSTOMIZE) edit set image controller=$(IMG)
	$(KUSTOMIZE) build config/manifests | operator-sdk generate bundle -q --overwrite --version $(VERSION) $(BUNDLE_METADATA_OPTS)
	operator-sdk bundle validate ./bundle
 
.PHONY: bundle-build
bundle-build: ## Build the bundle image.
	docker build -f bundle.Dockerfile -t $(BUNDLE_IMG) .
 
.PHONY: bundle-push
bundle-push: ## Push the bundle image.
	$(MAKE) docker-push IMG=$(BUNDLE_IMG)
 
.PHONY: opm
OPM = ./bin/opm
opm: ## Download opm locally if necessary.
ifeq (,$(wildcard $(OPM)))
ifeq (,$(shell which opm 2>/dev/null))
	@{ \
	set -e ;\
	mkdir -p $(dir $(OPM)) ;\
	OS=$(shell go env GOOS) && ARCH=$(shell go env GOARCH) && \
	curl -sSLo $(OPM) https://github.com/operator-framework/operator-registry/releases/download/v1.15.1/$${OS}-$${ARCH}-opm ;\
	chmod +x $(OPM) ;\
        }
else
OPM = $(shell which opm)
endif
endif
 
# A comma-separated list of bundle images (e.g. make catalog-build BUNDLE_IMGS=example.com/operator-bundle:v0.1.0,example.com/operator-bundle:v0.2.0).
# These images MUST exist in a registry and be pull-able.
BUNDLE_IMGS ?= $(BUNDLE_IMG)
 
# The image tag given to the resulting catalog image (e.g. make catalog-build CATALOG_IMG=example.com/operator-catalog:v0.2.0).
CATALOG_IMG ?= $(IMAGE_TAG_BASE)-catalog:v$(VERSION)
 
# Set CATALOG_BASE_IMG to an existing catalog image tag to add $BUNDLE_IMGS to that image.
ifneq ($(origin CATALOG_BASE_IMG), undefined)
FROM_INDEX_OPT := --from-index $(CATALOG_BASE_IMG)
endif
 
# Build a catalog image by adding bundle images to an empty catalog using the operator package manager tool, 'opm'.
# This recipe invokes 'opm' in 'semver' bundle add mode. For more information on add modes, see:
# https://github.com/operator-framework/community-operators/blob/7f1438c/docs/packaging-operator.md#updating-your-existing-operator
.PHONY: catalog-build
catalog-build: opm ## Build a catalog image.
	$(OPM) index add --container-tool docker --mode semver --tag $(CATALOG_IMG) --bundles $(BUNDLE_IMGS) $(FROM_INDEX_OPT)
 
# Push the catalog image.
.PHONY: catalog-push
catalog-push: ## Push a catalog image.
	$(MAKE) docker-push IMG=$(CATALOG_IMG)
