TOOLS_BIN_DIR ?= hack/tools/bin
export PATH := $(abspath $(TOOLS_BIN_DIR)):$(PATH)

# We use a file per tool and version as an indicator for make whether we need to install the tool or a different version
# of the tool (make doesn't rerun the rule if the rule is changed).

# Use this "function" to add the version file as a prerequisite for the tool target, e.g.:
#   $(KUBECTL): $(call tool_version_file,$(KUBECTL),$(KUBECTL_VERSION))
tool_version_file = $(TOOLS_BIN_DIR)/.version_$(subst $(TOOLS_BIN_DIR)/,,$(1))_$(2)

# Use this "function" to get the version of a go module from go.mod, e.g.:
#   GINKGO_VERSION ?= $(call version_gomod,github.com/onsi/ginkgo/v2)
version_gomod = $(shell go list -f '{{ .Version }}' -m $(1))

# This target cleans up any previous version files for the given tool and creates the given version file.
# This way, we can generically determine, which version was installed without calling each and every binary explicitly.
$(TOOLS_BIN_DIR)/.version_%:
	@version_file=$@; rm -f $${version_file%_*}*
	@touch $@

CONTROLLER_GEN := $(TOOLS_BIN_DIR)/controller-gen
# renovate: datasource=github-releases depName=kubernetes-sigs/controller-tools
CONTROLLER_GEN_VERSION ?= v0.18.0
$(CONTROLLER_GEN): $(call tool_version_file,$(CONTROLLER_GEN),$(CONTROLLER_GEN_VERSION))
	GOBIN=$(abspath $(TOOLS_BIN_DIR)) go install sigs.k8s.io/controller-tools/cmd/controller-gen@$(CONTROLLER_GEN_VERSION)

GINKGO := $(TOOLS_BIN_DIR)/ginkgo
GINKGO_VERSION ?= $(call version_gomod,github.com/onsi/ginkgo/v2)
$(GINKGO): $(call tool_version_file,$(GINKGO),$(GINKGO_VERSION))
	go build -o $(GINKGO) github.com/onsi/ginkgo/v2/ginkgo

GOLANGCI_LINT := $(TOOLS_BIN_DIR)/golangci-lint
# renovate: datasource=github-releases depName=golangci/golangci-lint
GOLANGCI_LINT_VERSION ?= v2.1.6
$(GOLANGCI_LINT): $(call tool_version_file,$(GOLANGCI_LINT),$(GOLANGCI_LINT_VERSION))
	curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $(TOOLS_BIN_DIR) $(GOLANGCI_LINT_VERSION)

KIND := $(TOOLS_BIN_DIR)/kind
# renovate: datasource=github-releases depName=kubernetes-sigs/kind
KIND_VERSION ?= v0.29.0
$(KIND): $(call tool_version_file,$(KIND),$(KIND_VERSION))
	curl -Lo $(KIND) https://kind.sigs.k8s.io/dl/$(KIND_VERSION)/kind-$(shell uname -s | tr '[:upper:]' '[:lower:]')-$(shell uname -m | sed 's/x86_64/amd64/;s/aarch64/arm64/')
	chmod +x $(KIND)

KO := $(TOOLS_BIN_DIR)/ko
# renovate: datasource=github-releases depName=ko-build/ko
KO_VERSION ?= v0.18.0
$(KO): $(call tool_version_file,$(KO),$(KO_VERSION))
	GOBIN=$(abspath $(TOOLS_BIN_DIR)) go install github.com/google/ko@$(KO_VERSION)

KUBECTL := $(TOOLS_BIN_DIR)/kubectl
# renovate: datasource=github-releases depName=kubectl packageName=kubernetes/kubernetes
KUBECTL_VERSION ?= v1.33.1
$(KUBECTL): $(call tool_version_file,$(KUBECTL),$(KUBECTL_VERSION))
	curl -Lo $(KUBECTL) https://dl.k8s.io/release/$(KUBECTL_VERSION)/bin/$(shell uname -s | tr '[:upper:]' '[:lower:]')/$(shell uname -m | sed 's/x86_64/amd64/')/kubectl
	chmod +x $(KUBECTL)

KYVERNO := $(TOOLS_BIN_DIR)/kyverno
# renovate: datasource=github-releases depName=kyverno/kyverno
KYVERNO_VERSION ?= v1.14.2
$(KYVERNO): $(call tool_version_file,$(KYVERNO),$(KYVERNO_VERSION))
	curl -Lo - https://github.com/kyverno/kyverno/releases/download/$(KYVERNO_VERSION)/kyverno-cli_$(KYVERNO_VERSION)_$(shell uname -s | tr '[:upper:]' '[:lower:]')_$(shell uname -m | sed 's/aarch64/arm64/').tar.gz | tar -xzmf - -C $(TOOLS_BIN_DIR) kyverno
	chmod +x $(KYVERNO)

SETUP_ENVTEST := $(TOOLS_BIN_DIR)/setup-envtest
CONTROLLER_RUNTIME_VERSION ?= $(call version_gomod,sigs.k8s.io/controller-runtime)
$(SETUP_ENVTEST): $(call tool_version_file,$(SETUP_ENVTEST),$(CONTROLLER_RUNTIME_VERSION))
	curl -Lo $(SETUP_ENVTEST) https://github.com/kubernetes-sigs/controller-runtime/releases/download/$(CONTROLLER_RUNTIME_VERSION)/setup-envtest-$(shell uname -s | tr '[:upper:]' '[:lower:]')-$(shell uname -m | sed 's/x86_64/amd64/;s/aarch64/arm64/')
	chmod +x $(SETUP_ENVTEST)

SKAFFOLD := $(TOOLS_BIN_DIR)/skaffold
# renovate: datasource=github-releases depName=GoogleContainerTools/skaffold
SKAFFOLD_VERSION ?= v2.16.1
$(SKAFFOLD): $(call tool_version_file,$(SKAFFOLD),$(SKAFFOLD_VERSION))
	curl -Lo $(SKAFFOLD) https://storage.googleapis.com/skaffold/releases/$(SKAFFOLD_VERSION)/skaffold-$(shell uname -s | tr '[:upper:]' '[:lower:]')-$(shell uname -m | sed 's/x86_64/amd64/')
	chmod +x $(SKAFFOLD)

VGOPATH := $(TOOLS_BIN_DIR)/vgopath
# renovate: datasource=github-releases depName=ironcore-dev/vgopath
VGOPATH_VERSION ?= v0.1.8
$(VGOPATH): $(call tool_version_file,$(VGOPATH),$(VGOPATH_VERSION))
	GOBIN=$(abspath $(TOOLS_BIN_DIR)) go install github.com/ironcore-dev/vgopath@$(VGOPATH_VERSION)

YQ := $(TOOLS_BIN_DIR)/yq
# renovate: datasource=github-releases depName=mikefarah/yq
YQ_VERSION ?= v4.45.4
$(YQ): $(call tool_version_file,$(YQ),$(YQ_VERSION))
	curl -Lo $(YQ) https://github.com/mikefarah/yq/releases/download/$(YQ_VERSION)/yq_$(shell uname -s | tr '[:upper:]' '[:lower:]')_$(shell uname -m | sed 's/x86_64/amd64/;s/aarch64/arm64/')
	chmod +x $(YQ)
