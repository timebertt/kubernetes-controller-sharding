PROJECT_DIR := $(shell dirname $(abspath $(lastword $(MAKEFILE_LIST))))

# Image URL to use all building/pushing image targets
IMG ?= ghcr.io/timebertt/kubernetes-controller-sharding/webhosting-operator:latest
EXPORTER_IMG ?= ghcr.io/timebertt/kubernetes-controller-sharding/webhosting-exporter:latest
# ENVTEST_K8S_VERSION refers to the version of kubebuilder assets to be downloaded by envtest binary.
ENVTEST_K8S_VERSION = 1.24
# if WITH_DNS is set to true, we configure the operator to make ingresses available via public dns
WITH_DNS = false

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
	$(CONTROLLER_GEN) rbac:roleName=manager-role crd webhook paths="./..." output:crd:artifacts:config=config/crd/bases

.PHONY: generate
generate: $(CONTROLLER_GEN) ## Generate code containing DeepCopy, DeepCopyInto, and DeepCopyObject method implementations.
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="./..."

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
	go build -o bin/webhosting-operator main.go
	go build -o bin/webhosting-exporter ./cmd/webhosting-exporter

.PHONY: run
run: manifests generate fmt vet ## Run the webhosting-operator from your host.
	go run ./main.go --config=config/manager/controller_manager_config.yaml

.PHONY: run-exporter
run-exporter: ## Run the webhosting-exporter from your host.
	go run ./cmd/webhosting-exporter --port=8180 --telemetry-port=8181 --kubeconfig=$(KUBECONFIG)

.PHONY: docker-build
docker-build: test ## Build docker image with the manager.
	docker build -t ${IMG} --target webhosting-operator .
	docker build -t ${EXPORTER_IMG} --target webhosting-exporter .

.PHONY: docker-push
docker-push: ## Push docker image with the manager.
	docker push ${IMG}
	docker push ${EXPORTER_IMG}

##@ Deployment

ifndef ignore-not-found
  ignore-not-found = false
endif

.PHONY: install
install: $(KUSTOMIZE) $(KUBECTL) manifests ## Install CRDs into K8s cluster specified in $KUBECONFIG.
	$(KUSTOMIZE) build config/crd | $(KUBECTL) apply -f -

.PHONY: uninstall
uninstall: $(KUSTOMIZE) $(KUBECTL) manifests ## Uninstall CRDs from K8s cluster specified in $KUBECONFIG. Call with ignore-not-found=true to ignore resource not found errors during deletion.
	$(KUSTOMIZE) build config/crd | $(KUBECTL) delete --ignore-not-found=$(ignore-not-found) -f -

.PHONY: deploy
deploy: $(KUSTOMIZE) $(KUBECTL) manifests ## Deploy controller to K8s cluster specified in $KUBECONFIG.
	cd config/manager && $(abspath $(KUSTOMIZE)) edit set image controller=${IMG}
ifneq ($(WITH_DNS), true)
	$(KUSTOMIZE) build config/default | $(KUBECTL) apply -f -
else
	$(KUSTOMIZE) build config/default-with-dns | $(KUBECTL) apply -f -
endif

.PHONY: undeploy
undeploy: $(KUSTOMIZE) $(KUBECTL) ## Undeploy controller from K8s cluster specified in $KUBECONFIG. Call with ignore-not-found=true to ignore resource not found errors during deletion.
	$(KUSTOMIZE) build config/default | $(KUBECTL) delete --ignore-not-found=$(ignore-not-found) -f -

k3d-up k3d-down: export KUBECONFIG = $(PROJECT_DIR)/dev/k3d_kubeconfig.yaml

.PHONY: k3d-up
k3d-up: $(K3D) ## Launch a k3d cluster for testing the operator.
	$(K3D) cluster create webhosting -i "docker.io/rancher/k3s:v1.24.2-k3s2" -p "8088:80@loadbalancer" --k3s-arg "--disable=traefik@server:0"
	# run `export KUBECONFIG=$$PWD/dev/k3d_kubeconfig.yaml` to target the created k3d cluster.
	$(MAKE) deploy-ingress-nginx

.PHONY: k3d-down
k3d-down: $(K3D) ## Tear down the k3d testing cluster.
	$(K3D) cluster delete webhosting

.PHONY: deploy-ingress-nginx
deploy-ingress-nginx: $(KUSTOMIZE) $(KUBECTL) ## Deploy ingress-nginx to K8s cluster specified in $KUBECONFIG.
ifneq ($(WITH_DNS), true)
	$(KUSTOMIZE) build config/ingress-nginx/default | $(KUBECTL) apply --server-side -f -
else
	$(KUSTOMIZE) build config/ingress-nginx/with-dns | $(KUBECTL) apply --server-side -f -
endif
	$(KUBECTL) -n ingress-nginx wait deploy ingress-nginx-controller --for=condition=Available --timeout=2m

.PHONY: deploy-monitoring
deploy-monitoring: $(KUSTOMIZE) $(KUBECTL) ## Deploy monitoring stack to K8s cluster specified in $KUBECONFIG.
	[ -f config/monitoring/grafana_admin_pass.secret.txt ] || cat /dev/urandom | tr -dc "a-zA-Z0-9" | head -c 32 > config/monitoring/grafana_admin_pass.secret.txt
	# generated grafana admin password to config/monitoring/grafana_admin_pass.secret.txt
	$(KUSTOMIZE) build config/monitoring/crds | $(KUBECTL) apply --server-side -f -
	$(KUBECTL) wait crd -l app.kubernetes.io/name=prometheus-operator --for=condition=NamesAccepted --for=condition=Established
	cd config/monitoring/webhosting-exporter && $(abspath $(KUSTOMIZE)) edit set image exporter=${EXPORTER_IMG}
	$(KUSTOMIZE) build config/monitoring | $(KUBECTL) apply --server-side -f -
