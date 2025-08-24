PROJECT_DIR := $(shell dirname $(abspath $(lastword $(MAKEFILE_LIST))))

# Image URL to use all building/pushing image targets
TAG ?= latest
GHCR_REPO ?= ghcr.io/timebertt/kubernetes-controller-sharding
SHARDER_IMG ?= $(GHCR_REPO)/sharder:$(TAG)
CHECKSUM_CONTROLLER_IMG ?= $(GHCR_REPO)/checksum-controller:$(TAG)
WEBHOSTING_OPERATOR_IMG ?= $(GHCR_REPO)/webhosting-operator:$(TAG)
EXPERIMENT_IMG ?= $(GHCR_REPO)/experiment:$(TAG)

# Optionally, overwrite the envtest version or assets directory to use
ENVTEST_K8S_VERSION =
KUBEBUILDER_ASSETS =

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

include hack/tools.mk

.PHONY: clean-tools-bin
clean-tools-bin: ## Empty the tools binary directory.
	rm -rf $(TOOLS_BIN_DIR)/*

##@ Development

.PHONY: tidy
tidy: ## Runs go mod to ensure modules are up to date.
	go mod tidy
	cd webhosting-operator && go mod tidy
	@# regenerate go.work.sum
	rm -f go.work.sum
	go mod download

.PHONY: generate-fast
generate-fast: $(CONTROLLER_GEN) tidy ## Run all fast code generators for the main module.
	$(CONTROLLER_GEN) rbac:roleName=sharder crd paths="./pkg/..." output:rbac:artifacts:config=config/rbac output:crd:artifacts:config=config/crds
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="./pkg/..."

.PHONY: generate-fast-webhosting
generate-fast-webhosting: $(CONTROLLER_GEN) tidy ## Run all fast code generators for the webhosting-operator module.
	$(CONTROLLER_GEN) rbac:roleName=operator crd paths="./webhosting-operator/..." output:rbac:artifacts:config=webhosting-operator/config/manager/rbac output:crd:artifacts:config=webhosting-operator/config/manager/crds
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="./webhosting-operator/..."

.PHONY: generate
generate: $(VGOPATH) generate-fast generate-fast-webhosting tidy ## Run all code generators.
	hack/update-codegen.sh

.PHONY: fmt
fmt: ## Run go fmt against code.
	go fmt ./...
	cd webhosting-operator && go fmt ./...

.PHONY: test
test: ## Run unit tests.
	./hack/test.sh ./cmd/... ./pkg/... ./webhosting-operator/pkg/...

.PHONY: test-integration
test-integration: $(SETUP_ENVTEST) ## Run integration tests.
	./hack/test-integration.sh ./test/integration/...

.PHONY: test-e2e
test-e2e: $(GINKGO) ## Run e2e tests.
	./hack/test-e2e.sh $(GINKGO_FLAGS) ./test/e2e/... ./webhosting-operator/test/e2e/...

.PHONY: skaffold-fix
skaffold-fix: $(SKAFFOLD) ## Upgrade skaffold configuration to the latest apiVersion.
	$(SKAFFOLD) fix --overwrite
	[ ! -f $(SKAFFOLD_FILENAME).v2 ] || rm $(SKAFFOLD_FILENAME).v2

##@ Verification

.PHONY: lint
lint: $(GOLANGCI_LINT) ## Run golangci-lint against code.
	$(GOLANGCI_LINT) run ./... ./webhosting-operator/...

.PHONY: check
check: lint test test-integration ## Check everything (lint + test + test-integration).

.PHONY: verify-fmt
verify-fmt: fmt ## Verify go code is formatted.
	@if !(git diff --quiet HEAD); then \
		echo "unformatted files detected, please run 'make fmt'"; exit 1; \
	fi

.PHONY: verify-generate
verify-generate: generate ## Verify generated files are up to date.
	@if !(git diff --quiet HEAD); then \
		echo "generated files are out of date, please run 'make generate'"; exit 1; \
	fi

.PHONY: verify-tidy
verify-tidy: tidy ## Verify go module files are up to date.
	@if !(git diff --quiet HEAD -- go.work.sum go.{mod,sum} webhosting-operator/go.{mod,sum}); then \
		echo "go module files are out of date, please run 'make tidy'"; exit 1; \
	fi

.PHONY: verify
verify: verify-tidy verify-fmt verify-generate check ## Verify everything (all verify-* rules + check).

.PHONY: ci-e2e-kind
ci-e2e-kind: $(KIND)
	./hack/ci-e2e-kind.sh

##@ Build

.PHONY: build
build: ## Build the sharder binary.
	go build -o bin/sharder ./cmd/sharder

.PHONY: run
run: $(KUBECTL) generate-fast ## Run the sharder from your host and deploy prerequisites.
	$(MAKE) deploy SKAFFOLD_MODULE=cert-manager
	$(KUBECTL) apply --server-side --force-conflicts -k config/crds
	$(KUBECTL) apply --server-side --force-conflicts -k hack/config/certificates/host
	go run ./cmd/sharder --config=hack/config/sharder/host/config.yaml --zap-devel

SHARD_NAME ?= checksum-controller-$(shell tr -dc bcdfghjklmnpqrstvwxz2456789 </dev/urandom | head -c 8)

.PHONY: run-checksum-controller
run-checksum-controller: $(KUBECTL) ## Run checksum-controller from your host and deploy prerequisites.
	$(KUBECTL) apply --server-side --force-conflicts -k hack/config/checksum-controller/controllerring
	go run ./cmd/checksum-controller --shard-name=$(SHARD_NAME) --lease-namespace=default --zap-devel

PUSH ?= false
images: export KO_DOCKER_REPO = $(GHCR_REPO)

.PHONY: images
images: $(KO) ## Build and push container images using ko.
	$(KO) build --push=$(PUSH) --sbom none --base-import-paths -t $(TAG) --platform linux/amd64,linux/arm64 \
		./cmd/sharder ./cmd/checksum-controller ./webhosting-operator/cmd/webhosting-operator

##@ Deployment

KIND_KUBECONFIG := $(PROJECT_DIR)/hack/kind_kubeconfig.yaml
kind-up kind-down: export KUBECONFIG = $(KIND_KUBECONFIG)

.PHONY: kind-up
kind-up: $(KIND) $(KUBECTL) ## Launch a kind cluster for local development and testing.
	$(KIND) create cluster --name sharding --config hack/config/kind-config.yaml --image kindest/node:v1.33.2@sha256:c55080dc5be4f2cc242e6966fdf97bb62282e1cd818a28223cf536db8b0fddf4
	# workaround https://kind.sigs.k8s.io/docs/user/known-issues/#pod-errors-due-to-too-many-open-files
	$(KUBECTL) get nodes -o name | cut -d/ -f2 | xargs -I {} docker exec {} sh -c "sysctl fs.inotify.max_user_instances=8192"
	# run `export KUBECONFIG=$$PWD/hack/kind_kubeconfig.yaml` to target the created kind cluster.

.PHONY: kind-down
kind-down: $(KIND) ## Tear down the kind testing cluster.
	$(KIND) delete cluster --name sharding

export SKAFFOLD_FILENAME = hack/config/skaffold.yaml
# use static label for skaffold to prevent rolling all components on every skaffold invocation
deploy up dev down: export SKAFFOLD_LABEL = skaffold.dev/run-id=sharding
# use dedicated ghcr repo for dev images to prevent spamming the "production" image repo
up dev: export SKAFFOLD_DEFAULT_REPO ?= ghcr.io/timebertt/dev-images
up dev: export SKAFFOLD_TAIL ?= true

.PHONY: deploy
deploy: $(SKAFFOLD) $(KUBECTL) $(YQ) ## Build all images and deploy everything to K8s cluster specified in $KUBECONFIG.
	$(SKAFFOLD) deploy -i $(SHARDER_IMG) -i $(CHECKSUM_CONTROLLER_IMG) -i $(WEBHOSTING_OPERATOR_IMG) -i $(EXPERIMENT_IMG)

.PHONY: up
up: $(SKAFFOLD) $(KUBECTL) $(YQ) ## Build all images, deploy everything to K8s cluster specified in $KUBECONFIG, start port-forward and tail logs.
	$(SKAFFOLD) run

.PHONY: dev
dev: $(SKAFFOLD) $(KUBECTL) $(YQ) ## Start continuous dev loop with skaffold.
	$(SKAFFOLD) dev --port-forward=user --cleanup=false --trigger=manual

.PHONY: down
down: $(SKAFFOLD) $(KUBECTL) $(YQ) ## Remove everything from K8s cluster specified in $KUBECONFIG.
	$(SKAFFOLD) delete
