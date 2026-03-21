#
# Copyright (c) 2025, Oracle and/or its affiliates.
# Licensed under the Universal Permissive License v 1.0 as shown at http://oss.oracle.com/licenses/upl.
#

# --------------------------
# Global / Defaults
# --------------------------
VERSION ?= 2.0

IMG ?= controller:latest
BUNDLE_IMG ?= controller-bundle:$(VERSION)

# Operator YAML file
OPERATOR_YAML = $$(basename $$(pwd)).yaml

# Use bash with pipefail for scripts like setup-envtest
SHELL := /usr/bin/env bash -o pipefail
.SHELLFLAGS := -ec

# Enable allowDangerousTypes to use float type in CRD
CRD_OPTIONS ?= "crd:maxDescLen=0,allowDangerousTypes=true"

# ENVTEST_K8S_VERSION refers to the version of kubebuilder assets used by envtest
ENVTEST_K8S_VERSION ?= 1.31.0

# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set)
ifeq (,$(shell go env GOBIN))
GOBIN := $(shell go env GOPATH)/bin
else
GOBIN := $(shell go env GOBIN)
endif

# --------------------------
# Bundle metadata options
# --------------------------
BUNDLE_CHANNELS :=
BUNDLE_DEFAULT_CHANNEL :=
ifneq ($(origin CHANNELS), undefined)
BUNDLE_CHANNELS := --channels=$(CHANNELS)
endif
ifneq ($(origin DEFAULT_CHANNEL), undefined)
BUNDLE_DEFAULT_CHANNEL := --default-channel=$(DEFAULT_CHANNEL)
endif
BUNDLE_METADATA_OPTS ?= $(BUNDLE_CHANNELS) $(BUNDLE_DEFAULT_CHANNEL)

# --------------------------
# Tools (local install)
# --------------------------
LOCALBIN ?= $(shell pwd)/bin
KUSTOMIZE ?= $(LOCALBIN)/kustomize
CONTROLLER_GEN ?= $(LOCALBIN)/controller-gen
ENVTEST ?= $(LOCALBIN)/setup-envtest

KUSTOMIZE_VERSION ?= v5.7.1
CONTROLLER_TOOLS_VERSION ?= v0.17
KUSTOMIZE_INSTALL_SCRIPT ?= https://raw.githubusercontent.com/kubernetes-sigs/kustomize/master/hack/install_kustomize.sh

# --------------------------
# Container build configuration
# --------------------------
GOLANG_VERSION ?= 1.25.1
DOCKER ?= podman

# ------------------------------------------------------------------------------
# Debug image support
#
# DEBUG controls whether we build a debug image (includes dlv, built with debug flags)
# or a production image.
#
# Defaults:
#   DEBUG=false  -> builds --target prod and passes --build-arg DEBUG=false
#
# Usage:
#   make image-build                 # prod image
#   make image-build DEBUG=true      # debug image (includes dlv)
#   make image-build TARGET=debug    # explicit target override
# ------------------------------------------------------------------------------
DEBUG ?= true

# TARGET controls the Dockerfile target. If not provided, derive from DEBUG.
TARGET ?=
ifeq ($(TARGET),)
  ifeq ($(DEBUG),true)
    TARGET := debug
  else
    TARGET := prod
  endif
endif

# Download golang in the Dockerfile if BUILD_INTERNAL is true, else use golang:<ver>
ifeq ($(BUILD_INTERNAL),true)
BUILDER_IMG := oraclelinux:9
BUILD_ARGS_BASE := --build-arg BUILDER_IMG=$(BUILDER_IMG) --build-arg GOLANG_VERSION=$(GOLANG_VERSION) --build-arg INSTALL_GO=true
else
BUILDER_IMG := golang:$(GOLANG_VERSION)
BUILD_ARGS_BASE := --build-arg BUILDER_IMG=$(BUILDER_IMG) --build-arg INSTALL_GO=false --build-arg GOLANG_VERSION=$(GOLANG_VERSION)
endif

# Multi-arch manifest build toggle
PUSH_ARGS :=
ifeq ($(BUILD_MANIFEST),true)
BUILD_ARGS_PLATFORM := --platform=linux/arm64,linux/amd64 --jobs=2 --manifest
PUSH_ARGS := manifest
else
BUILD_ARGS_PLATFORM := --platform=linux/amd64 --tag
endif

BUILD_ARGS := $(BUILD_ARGS_BASE) $(BUILD_ARGS_PLATFORM)

# --------------------------
# Phony Targets
# --------------------------
.PHONY: all manifests generate fmt vet test e2e build run image-build image-push minikube-push \
        install uninstall deploy minikube-deploy operator-yaml minikube-operator-yaml undeploy \
        kustomize controller-gen envtest bundle bundle-build bundle-push opm catalog-build catalog-push

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
    KUBEBUILDER_ASSETS="$$( $(ENVTEST) use $(ENVTEST_K8S_VERSION) -p path )" go test $(TEST) -coverprofile cover.out

E2ETEST ?= ./test/e2e/
e2e: manifests generate fmt vet envtest ## Run e2e tests.
    KUBEBUILDER_ASSETS="$$( $(ENVTEST) use $(ENVTEST_K8S_VERSION) -p path )" go test $(E2ETEST) -test.timeout 0 -test.v --ginkgo.fail-fast

##@ Build
build: generate fmt vet ## Build manager binary.
    go build -o bin/manager main.go

run: manifests generate fmt vet ## Run a controller from your host.
    go run ./main.go

image-build: ## Build container image with the manager. Set DEBUG=true for debug image.
    $(DOCKER) build \
        --build-arg http_proxy=$(HTTP_PROXY) \
        --build-arg https_proxy=$(HTTPS_PROXY) \
        --build-arg CI_COMMIT_SHA=$(CI_COMMIT_SHA) \
        --build-arg CI_COMMIT_BRANCH=$(CI_COMMIT_BRANCH) \
        --build-arg DEBUG=$(DEBUG) \
        --target $(TARGET) \
        $(BUILD_ARGS) $(IMG) .

image-push: ## Push container image with the manager.
    $(DOCKER) $(PUSH_ARGS) push $(IMG)

# Push to minikube's local registry enabled by registry add-on
minikube-push:
    $(DOCKER) tag $(IMG) $$(minikube ip):5000/$(IMG)
    $(DOCKER) push --tls-verify=false $$(minikube ip):5000/$(IMG)

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
    (echo --- && sed '/^apiVersion: apps\/v1/,/---/!d' "$(OPERATOR_YAML).bak") >> "$(OPERATOR_YAML)"
    rm "$(OPERATOR_YAML).bak"

minikube-operator-yaml: operator-yaml
    sed -i.bak 's/\(replicas.\) 3/\1 1/g' "$(OPERATOR_YAML)"
    rm "$(OPERATOR_YAML).bak"

undeploy: ## Undeploy controller from the K8s cluster specified in ~/.kube/config.
    $(KUSTOMIZE) build config/default | kubectl delete -f -

##@ Build Dependencies
$(LOCALBIN):
    mkdir -p $(LOCALBIN)

kustomize: $(KUSTOMIZE) ## Download kustomize locally if necessary.
$(KUSTOMIZE): $(LOCALBIN)
    curl -s $(KUSTOMIZE_INSTALL_SCRIPT) | bash -s -- $(subst v,,$(KUSTOMIZE_VERSION)) $(LOCALBIN)

controller-gen: $(CONTROLLER_GEN) ## Download controller-gen locally if necessary.
$(CONTROLLER_GEN): $(LOCALBIN)
    GOBIN=$(LOCALBIN) go install sigs.k8s.io/controller-tools/cmd/controller-gen@$(CONTROLLER_TOOLS_VERSION)

envtest: $(ENVTEST) ## Download envtest-setup locally if necessary.
$(ENVTEST): $(LOCALBIN)
    GOBIN=$(LOCALBIN) go install sigs.k8s.io/controller-runtime/tools/setup-envtest@latest

bundle: manifests kustomize ## Generate bundle manifests and metadata, then validate generated files.
    operator-sdk generate kustomize manifests -q
    cd config/manager && $(KUSTOMIZE) edit set image controller=$(IMG)
    $(KUSTOMIZE) build config/manifests | operator-sdk generate bundle -q --overwrite --version $(VERSION) $(BUNDLE_METADATA_OPTS)
    operator-sdk bundle validate ./bundle

bundle-build: ## Build the bundle image.
    $(DOCKER) build -f bundle.Dockerfile -t $(BUNDLE_IMG) .

bundle-push: ## Push the bundle image.
    $(MAKE) image-push IMG=$(BUNDLE_IMG)

##@ opm / catalog
OPM := ./bin/opm

opm: ## Download opm locally if necessary.
ifeq (,$(wildcard $(OPM)))
ifeq (,$(shell which opm 2>/dev/null))
    @{ \
        set -e ;\
        mkdir -p $(dir $(OPM)) ;\
        OS=$$(go env GOOS) && ARCH=$$(go env GOARCH) && \
        curl -sSLo $(OPM) https://github.com/operator-framework/operator-registry/releases/download/v1.15.1/$${OS}-$${ARCH}-opm ;\
        chmod +x $(OPM) ;\
    }
else
OPM := $(shell which opm)
endif
endif

# A comma-separated list of bundle images.
BUNDLE_IMGS ?= $(BUNDLE_IMG)

# The image tag given to the resulting catalog image.
CATALOG_IMG ?= $(IMAGE_TAG_BASE)-catalog:v$(VERSION)

# Set CATALOG_BASE_IMG to an existing catalog image tag to add $BUNDLE_IMGS to that image.
FROM_INDEX_OPT :=
ifneq ($(origin CATALOG_BASE_IMG), undefined)
FROM_INDEX_OPT := --from-index $(CATALOG_BASE_IMG)
endif

catalog-build: opm ## Build a catalog image.
    $(OPM) index add --container-tool docker --mode semver --tag $(CATALOG_IMG) --bundles $(BUNDLE_IMGS) $(FROM_INDEX_OPT)

catalog-push: ## Push a catalog image.
    $(MAKE) image-push IMG=$(CATALOG_IMG)
