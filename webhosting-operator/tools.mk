TOOLS_BIN_DIR := bin
export PATH := $(abspath $(TOOLS_BIN_DIR)):$(PATH)

# Tool targets should declare go.mod as a prerequisite, if the tool's version is managed via go modules. This causes
# make to rebuild the tool in the desired version, when go.mod is changed.
# For tools where the version is not managed via go.mod, we use a file per tool and version as an indicator for make
# whether we need to install the tool or a different version of the tool (make doesn't rerun the rule if the rule is
# changed).

# Use this "function" to add the version file as a prerequisite for the tool target: e.g.
#   $(KUBECTL): $(call tool_version_file,$(KUBECTL),$(KUBECTL_VERSION))
tool_version_file = $(TOOLS_BIN_DIR)/.version_$(subst $(TOOLS_BIN_DIR)/,,$(1))_$(2)

# This target cleans up any previous version files for the given tool and creates the given version file.
# This way, we can generically determine, which version was installed without calling each and every binary explicitly.
$(TOOLS_BIN_DIR)/.version_%:
	@version_file=$@; rm -f $${version_file%_*}*
	@touch $@

CONTROLLER_GEN := $(TOOLS_BIN_DIR)/controller-gen
CONTROLLER_GEN_VERSION ?= v0.9.2
$(CONTROLLER_GEN): $(call tool_version_file,$(CONTROLLER_GEN),$(CONTROLLER_GEN_VERSION))
	GOBIN=$(abspath $(TOOLS_BIN_DIR)) go install sigs.k8s.io/controller-tools/cmd/controller-gen@$(CONTROLLER_GEN_VERSION)

KIND := $(TOOLS_BIN_DIR)/kind
KIND_VERSION ?= v0.20.0
$(KIND): $(call tool_version_file,$(KIND),$(KIND_VERSION))
	curl -L -o $(KIND) https://kind.sigs.k8s.io/dl/$(KIND_VERSION)/kind-$(shell uname -s | tr '[:upper:]' '[:lower:]')-$(shell uname -m | sed 's/x86_64/amd64/;s/aarch64/arm64/')
	chmod +x $(KIND)

KUBECTL := $(TOOLS_BIN_DIR)/kubectl
KUBECTL_VERSION ?= v1.24.2
$(KUBECTL): $(call tool_version_file,$(KUBECTL),$(KUBECTL_VERSION))
	curl -Lo $(KUBECTL) https://dl.k8s.io/release/$(KUBECTL_VERSION)/bin/$(shell uname -s | tr '[:upper:]' '[:lower:]')/$(shell uname -m | sed 's/x86_64/amd64/')/kubectl
	chmod +x $(KUBECTL)

KUSTOMIZE := $(TOOLS_BIN_DIR)/kustomize
KUSTOMIZE_VERSION ?= v4.5.5
$(KUSTOMIZE): $(call tool_version_file,$(KUSTOMIZE),$(KUSTOMIZE_VERSION))
	GOBIN=$(abspath $(TOOLS_BIN_DIR)) go install sigs.k8s.io/kustomize/kustomize/v4@$(KUSTOMIZE_VERSION)

GINKGO := $(TOOLS_BIN_DIR)/ginkgo
$(GINKGO): go.mod
	go build -o $(GINKGO) github.com/onsi/ginkgo/v2/ginkgo

SETUP_ENVTEST := $(TOOLS_BIN_DIR)/setup-envtest
$(SETUP_ENVTEST): go.mod
	go build -o $(SETUP_ENVTEST) sigs.k8s.io/controller-runtime/tools/setup-envtest

SKAFFOLD := $(TOOLS_BIN_DIR)/skaffold
SKAFFOLD_VERSION ?= v2.6.1
$(SKAFFOLD): $(call tool_version_file,$(SKAFFOLD),$(SKAFFOLD_VERSION))
	curl -Lo $(SKAFFOLD) https://storage.googleapis.com/skaffold/releases/$(SKAFFOLD_VERSION)/skaffold-$(shell uname -s | tr '[:upper:]' '[:lower:]')-$(shell uname -m | sed 's/x86_64/amd64/')
	chmod +x $(SKAFFOLD)
