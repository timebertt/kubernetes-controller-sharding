TOOLS_BIN_DIR ?= hack/tools/bin
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
CONTROLLER_GEN_VERSION ?= v0.12.1
$(CONTROLLER_GEN): $(call tool_version_file,$(CONTROLLER_GEN),$(CONTROLLER_GEN_VERSION))
	GOBIN=$(abspath $(TOOLS_BIN_DIR)) go install sigs.k8s.io/controller-tools/cmd/controller-gen@$(CONTROLLER_GEN_VERSION)

KIND := $(TOOLS_BIN_DIR)/kind
KIND_VERSION ?= v0.20.0
$(KIND): $(call tool_version_file,$(KIND),$(KIND_VERSION))
	curl -Lo $(KIND) https://kind.sigs.k8s.io/dl/$(KIND_VERSION)/kind-$(shell uname -s | tr '[:upper:]' '[:lower:]')-$(shell uname -m | sed 's/x86_64/amd64/;s/aarch64/arm64/')
	chmod +x $(KIND)

KO := $(TOOLS_BIN_DIR)/ko
KO_VERSION ?= v0.14.1
$(KO): $(call tool_version_file,$(KO),$(KO_VERSION))
	GOBIN=$(abspath $(TOOLS_BIN_DIR)) go install github.com/google/ko@$(KO_VERSION)

KUBECTL := $(TOOLS_BIN_DIR)/kubectl
KUBECTL_VERSION ?= v1.27.3
$(KUBECTL): $(call tool_version_file,$(KUBECTL),$(KUBECTL_VERSION))
	curl -Lo $(KUBECTL) https://dl.k8s.io/release/$(KUBECTL_VERSION)/bin/$(shell uname -s | tr '[:upper:]' '[:lower:]')/$(shell uname -m | sed 's/x86_64/amd64/')/kubectl
	chmod +x $(KUBECTL)

KYVERNO := $(TOOLS_BIN_DIR)/kyverno
KYVERNO_VERSION ?= v1.10.3
$(KYVERNO): $(call tool_version_file,$(KYVERNO),$(KYVERNO_VERSION))
	curl -Lo - https://github.com/kyverno/kyverno/releases/download/$(KYVERNO_VERSION)/kyverno-cli_$(KYVERNO_VERSION)_$(shell uname -s | tr '[:upper:]' '[:lower:]')_$(shell uname -m | sed 's/aarch64/arm64/').tar.gz | tar -xzmf - -C $(TOOLS_BIN_DIR) kyverno
	chmod +x $(KYVERNO)

SETUP_ENVTEST := $(TOOLS_BIN_DIR)/setup-envtest
$(SETUP_ENVTEST): go.mod
	go build -o $(SETUP_ENVTEST) sigs.k8s.io/controller-runtime/tools/setup-envtest

SKAFFOLD := $(TOOLS_BIN_DIR)/skaffold
SKAFFOLD_VERSION ?= v2.6.1
$(SKAFFOLD): $(call tool_version_file,$(SKAFFOLD),$(SKAFFOLD_VERSION))
	curl -Lo $(SKAFFOLD) https://storage.googleapis.com/skaffold/releases/$(SKAFFOLD_VERSION)/skaffold-$(shell uname -s | tr '[:upper:]' '[:lower:]')-$(shell uname -m | sed 's/x86_64/amd64/')
	chmod +x $(SKAFFOLD)

YQ := $(TOOLS_BIN_DIR)/yq
YQ_VERSION ?= v4.34.2
$(YQ): $(call tool_version_file,$(YQ),$(YQ_VERSION))
	curl -Lo $(YQ) https://github.com/mikefarah/yq/releases/download/$(YQ_VERSION)/yq_$(shell uname -s | tr '[:upper:]' '[:lower:]')_$(shell uname -m | sed 's/x86_64/amd64/;s/aarch64/arm64/')
	chmod +x $(YQ)