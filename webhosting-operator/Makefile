PROJECT_DIR := $(shell dirname $(abspath $(lastword $(MAKEFILE_LIST))))

# Image URL to use all building/pushing image targets
TAG ?= latest
GHCR_REPO ?= ghcr.io/timebertt/kubernetes-controller-sharding
OPERATOR_IMG ?= $(GHCR_REPO)/webhosting-operator:$(TAG)

# ENVTEST_K8S_VERSION refers to the version of kubebuilder assets to be downloaded by envtest binary.
ENVTEST_K8S_VERSION = 1.24
# set OVERLAY to shoot to configure ingress-nginx with public dns and a TLS certificate
OVERLAY = default

# Setting SHELL to bash allows bash commands to be executed by recipes.
# Options are set to exit when a recipe line exits non-zero or a piped command fails.
SHELL = /usr/bin/env bash -o pipefail
.SHELLFLAGS = -ec

.PHONY: all
all: build

##@ General

# The help target prints out all targets with their descriptions organized
# beneath their categories. The categories are represented by '##@' and the
# target descriptions by '##'. The awk commands is responsible for reading the
# entire set of makefiles included in this invocation, looking for lines of the
# file as xyz: ## something, and then pretty-format the target and help. Then,
# if there's a line with ##@ something, that gets pretty-printed as a category.
# More info on the usage of ANSI control characters for terminal formatting:
# https://en.wikipedia.org/wiki/ANSI_escape_code#SGR_parameters
# More info on the awk command:
# http://linuxcommand.org/lc3_adv_awk.php

.PHONY: help
help: ## Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

##@ Tools

include tools.mk

.PHONY: clean-tools-bin
clean-tools-bin: ## Empty the tools binary directory
	rm -rf $(TOOLS_BIN_DIR)/*

##@ Development

.PHONY: manifests
manifests: $(CONTROLLER_GEN) ## Generate WebhookConfiguration, ClusterRole and CustomResourceDefinition objects.
	$(CONTROLLER_GEN) rbac:roleName=operator crd paths="./..." output:rbac:artifacts:config=config/manager/rbac output:crd:artifacts:config=config/manager/crds

.PHONY: generate
generate: $(CONTROLLER_GEN) ## Generate code containing DeepCopy, DeepCopyInto, and DeepCopyObject method implementations.
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="./..."
	hack/update-codegen.sh

.PHONY: fmt
fmt: ## Run go fmt against code.
	go fmt ./...

.PHONY: vet
vet: ## Run go vet against code.
	go vet ./...

.PHONY: modules
modules: ## Runs go mod to ensure modules are up to date.
	go mod tidy

.PHONY: test
test: $(SETUP_ENVTEST) manifests generate fmt vet ## Run tests.
	KUBEBUILDER_ASSETS="$(shell $(SETUP_ENVTEST) use $(ENVTEST_K8S_VERSION) -p path)" go test -race ./...

##@ Verification

.PHONY: verify-fmt
verify-fmt: fmt ## Verify go code is formatted.
	@if !(git diff --quiet HEAD); then \
		echo "unformatted files are out of date, please run 'make fmt'"; exit 1; \
	fi

.PHONY: verify-generate
verify-generate: manifests generate ## Verify generated files are up to date.
	@if !(git diff --quiet HEAD); then \
		echo "generated files are out of date, please run 'make manifests generate'"; exit 1; \
	fi

.PHONY: verify-modules
verify-modules: modules ## Verify go module files are up to date.
	@if !(git diff --quiet HEAD -- go.sum go.mod); then \
		echo "go module files are out of date, please run 'make modules'"; exit 1; \
	fi

.PHONY: verify
verify: verify-fmt verify-generate verify-modules test ## Verify everything (all verify-* rules + test).

##@ Build

.PHONY: build
build: generate fmt vet ## Build manager binary.
	go build -o bin/webhosting-operator ./cmd/webhosting-operator

.PHONY: run
run: manifests generate fmt vet ## Run the webhosting-operator from your host.
	go run ./cmd/webhosting-operator

PUSH ?= false
images: export KO_DOCKER_REPO = $(GHCR_REPO)

.PHONY: images
images: $(KO) ## Build and push container images using ko.
	$(KO) build --push=$(PUSH) --sbom none --base-import-paths -t $(TAG) --platform linux/amd64,linux/arm64 ./cmd/webhosting-operator

##@ Deployment

KIND_KUBECONFIG := $(PROJECT_DIR)/dev/kind_kubeconfig.yaml
kind-up kind-down: export KUBECONFIG = $(KIND_KUBECONFIG)

.PHONY: kind-up
kind-up: $(KIND) ## Launch a kind cluster for testing the operator.
	$(KIND) create cluster --name webhosting --config hack/kind-config.yaml
	# run `export KUBECONFIG=$$PWD/dev/kind_kubeconfig.yaml` to target the created kind cluster.
	$(MAKE) deploy-ingress-nginx OVERLAY=kind

.PHONY: kind-down
kind-down: $(KIND) ## Tear down the kind testing cluster.
	$(KIND) delete cluster --name webhosting

.PHONY: deploy-ingress-nginx
deploy-ingress-nginx: $(KUSTOMIZE) $(KUBECTL) ## Deploy ingress-nginx to K8s cluster specified in $KUBECONFIG.
	@# job template is immutable, delete old jobs to prepare for upgrade
	$(KUBECTL) -n ingress-nginx delete job --ignore-not-found ingress-nginx-admission-create ingress-nginx-admission-patch
	$(KUSTOMIZE) build config/ingress-nginx/$(OVERLAY) | $(KUBECTL) apply --server-side -f -
	$(KUBECTL) -n ingress-nginx wait deploy ingress-nginx-controller --for=condition=Available --timeout=2m

# use static label for skaffold to prevent rolling all components on every skaffold invocation
deploy up dev down: export SKAFFOLD_LABEL = skaffold.dev/run-id=webhosting-operator

.PHONY: deploy
deploy: $(SKAFFOLD) $(KUBECTL) $(YQ) ## Build all images and deploy everything to K8s cluster specified in $KUBECONFIG.
	$(SKAFFOLD) deploy --port-forward=user --tail -i $(OPERATOR_IMG)

.PHONY: up
up: $(SKAFFOLD) $(KUBECTL) $(YQ) ## Build all images, deploy everything to K8s cluster specified in $KUBECONFIG, start port-forward and tail logs.
	$(SKAFFOLD) run --port-forward=user --tail

.PHONY: dev
dev: $(SKAFFOLD) $(KUBECTL) $(YQ) ## Start continuous dev loop with skaffold.
	$(SKAFFOLD) dev --port-forward=user --cleanup=false --trigger=manual

.PHONY: down
down: $(SKAFFOLD) $(KUBECTL) $(YQ) ## Remove everything from K8s cluster specified in $KUBECONFIG.
	$(SKAFFOLD) delete
