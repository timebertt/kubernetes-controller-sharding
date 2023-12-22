PROJECT_DIR := $(shell dirname $(abspath $(lastword $(MAKEFILE_LIST))))

# Image URL to use all building/pushing image targets
TAG ?= latest
GHCR_REPO ?= ghcr.io/timebertt/kubernetes-controller-sharding
SHARDER_IMG ?= $(GHCR_REPO)/sharder:$(TAG)
SHARD_IMG ?= $(GHCR_REPO)/shard:$(TAG)
JANITOR_IMG ?= $(GHCR_REPO)/janitor:$(TAG)

# ENVTEST_K8S_VERSION refers to the version of kubebuilder assets to be downloaded by envtest binary.
ENVTEST_K8S_VERSION = 1.27

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
clean-tools-bin: ## Empty the tools binary directory
	rm -rf $(TOOLS_BIN_DIR)/*

##@ Development

.PHONY: modules
modules: ## Runs go mod to ensure modules are up to date.
	go mod tidy

.PHONY: generate-fast
generate-fast: $(CONTROLLER_GEN) modules ## Run all fast code generators
	$(CONTROLLER_GEN) rbac:roleName=sharder crd paths="./pkg/..." output:rbac:artifacts:config=config/rbac output:crd:artifacts:config=config/crds
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="./pkg/..."

.PHONY: generate
generate: generate-fast modules ## Run all code generators
	hack/update-codegen.sh

.PHONY: fmt
fmt: ## Run go fmt against code.
	go fmt ./...

.PHONY: test
test: $(SETUP_ENVTEST) ## Run tests.
	KUBEBUILDER_ASSETS="$(shell $(SETUP_ENVTEST) use $(ENVTEST_K8S_VERSION) -p path)" go test -race ./...

.PHONY: test-kyverno
test-kyverno: $(KYVERNO) ## Run kyverno policy tests.
	$(KYVERNO) test --remove-color -v 4 .

.PHONY: skaffold-fix
skaffold-fix: $(SKAFFOLD) ## Upgrade skaffold configuration to the latest apiVersion.
	$(SKAFFOLD) fix --overwrite
	[ ! -f $(SKAFFOLD_FILENAME).v2 ] || rm $(SKAFFOLD_FILENAME).v2

##@ Verification

.PHONY: lint
lint: $(GOLANGCI_LINT) ## Run golangci-lint against code.
	$(GOLANGCI_LINT) run ./...

.PHONY: check
check: lint test test-kyverno ## Check everything (lint + test + test-kyverno).

.PHONY: verify-fmt
verify-fmt: fmt ## Verify go code is formatted.
	@if !(git diff --quiet HEAD); then \
		echo "unformatted files are out of date, please run 'make fmt'"; exit 1; \
	fi

.PHONY: verify-generate
verify-generate: generate ## Verify generated files are up to date.
	@if !(git diff --quiet HEAD); then \
		echo "generated files are out of date, please run 'make generate'"; exit 1; \
	fi

.PHONY: verify-modules
verify-modules: modules ## Verify go module files are up to date.
	@if !(git diff --quiet HEAD -- go.sum go.mod); then \
		echo "go module files are out of date, please run 'make modules'"; exit 1; \
	fi

.PHONY: verify
verify: verify-fmt verify-generate verify-modules check ## Verify everything (all verify-* rules + check).

##@ Build

.PHONY: build
build: ## Build the sharder binary.
	go build -o bin/sharder ./cmd/sharder

.PHONY: run
run: $(KUBECTL) generate-fast ## Run the sharder from your host and deploy prerequisites.
	$(MAKE) deploy SKAFFOLD_MODULE=cert-manager
	$(KUBECTL) apply --server-side --force-conflicts -k config/crds
	$(KUBECTL) apply --server-side --force-conflicts -k hack/config/certificates/host
	go run ./cmd/sharder --config=hack/config/sharder/host/config.yaml --zap-log-level=debug

SHARD_NAME ?= shard-$(shell tr -dc bcdfghjklmnpqrstvwxz2456789 </dev/urandom | head -c 8)

.PHONY: run-shard
run-shard: $(KUBECTL) ## Run a shard from your host and deploy prerequisites.
	$(KUBECTL) apply --server-side --force-conflicts -k hack/config/shard/clusterring
	go run ./cmd/shard --shard=$(SHARD_NAME) --lease-namespace=default --zap-log-level=debug

PUSH ?= false
images: export KO_DOCKER_REPO = $(GHCR_REPO)

.PHONY: images
images: $(KO) ## Build and push container images using ko.
	$(KO) build --push=$(PUSH) --sbom none --base-import-paths -t $(TAG) --platform linux/amd64,linux/arm64 \
		./cmd/sharder ./cmd/shard ./hack/cmd/janitor

##@ Deployment

KIND_KUBECONFIG := $(PROJECT_DIR)/hack/kind_kubeconfig.yaml
kind-up kind-down: export KUBECONFIG = $(KIND_KUBECONFIG)

.PHONY: kind-up
kind-up: $(KIND) $(KUBECTL) ## Launch a kind cluster for local development and testing.
	$(KIND) create cluster --name sharding --config hack/config/kind-config.yaml
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

.PHONY: deploy
deploy: $(SKAFFOLD) $(KUBECTL) $(YQ) ## Build all images and deploy everything to K8s cluster specified in $KUBECONFIG.
	$(SKAFFOLD) deploy -i $(SHARDER_IMG) -i $(SHARD_IMG) -i $(JANITOR_IMG)

.PHONY: up
up: $(SKAFFOLD) $(KUBECTL) $(YQ) ## Build all images, deploy everything to K8s cluster specified in $KUBECONFIG, start port-forward and tail logs.
	$(SKAFFOLD) run --port-forward=user --tail

.PHONY: dev
dev: $(SKAFFOLD) $(KUBECTL) $(YQ) ## Start continuous dev loop with skaffold.
	$(SKAFFOLD) dev --port-forward=user --cleanup=false --trigger=manual

.PHONY: down
down: $(SKAFFOLD) $(KUBECTL) $(YQ) ## Remove everything from K8s cluster specified in $KUBECONFIG.
	$(SKAFFOLD) delete
