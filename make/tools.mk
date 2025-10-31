# This makefile contains all the make targets related to the tools used in the project.

# go_install_tool will 'go install' any package with custom target and name of binary, if it doesn't exist
# $1 - target path with name of binary
# $2 - package url which can be installed
# $3 - specific version of package
define go_install_tool
@[ -f "$(1)-$(3)" ] || { \
set -e; \
package=$(2)@$(3) ;\
echo "Downloading $${package}" ;\
rm -f $(1) || true ;\
GOBIN=$(TOOL_BIN) go install $${package} ;\
mv $(1) $(1)-$(3) ;\
} ;\
ln -sf $(1)-$(3) $(1)
endef

##@ Development Tools

## Location to install dependencies to
TOOL_BIN ?= $(PROJECT_BIN_DIR)/tools
$(TOOL_BIN):
	mkdir -p $(TOOL_BIN)

## Tool Binaries
KUBECTL ?= kubectl
KUSTOMIZE ?= $(TOOL_BIN)/kustomize
CONTROLLER_GEN ?= $(TOOL_BIN)/controller-gen
ENVTEST ?= $(TOOL_BIN)/setup-envtest
GOLANGCI_LINT ?= $(TOOL_BIN)/golangci-lint
HELMIFY ?= $(TOOL_BIN)/helmify
YQ ?= $(TOOL_BIN)/yq
SQLC ?= $(TOOL_BIN)/sqlc
CHECKOV ?= $(TOOL_BIN)/checkov
KUBEBUILDER_HELM_GEN ?= go run $(PROJECT_DIR)/tools/helm-gen

## Tool Versions
KUSTOMIZE_VERSION ?= v5.5.0
CONTROLLER_TOOLS_VERSION ?= v0.16.4
ENVTEST_VERSION ?= release-0.19
GOLANGCI_LINT_VERSION ?= v1.64.8
HELMIFY_VERSION ?= v0.4.17
YQ_VERSION ?= v4.45.1
SQLC_VERSION ?= v1.30.0
CHECKOV_VERSION ?= 3.2.489

.PHONY: kustomize
kustomize: $(KUSTOMIZE) ## Download kustomize locally if necessary.
$(KUSTOMIZE): $(TOOL_BIN)
	$(call go_install_tool,$(KUSTOMIZE),sigs.k8s.io/kustomize/kustomize/v5,$(KUSTOMIZE_VERSION))

.PHONY: controller-gen
controller-gen: $(CONTROLLER_GEN) ## Download controller-gen locally if necessary.
$(CONTROLLER_GEN): $(TOOL_BIN)
	$(call go_install_tool,$(CONTROLLER_GEN),sigs.k8s.io/controller-tools/cmd/controller-gen,$(CONTROLLER_TOOLS_VERSION))

.PHONY: envtest
envtest: $(ENVTEST) ## Download setup-envtest locally if necessary.
$(ENVTEST): $(TOOL_BIN)
	$(call go_install_tool,$(ENVTEST),sigs.k8s.io/controller-runtime/tools/setup-envtest,$(ENVTEST_VERSION))

.PHONY: golangci-lint
golangci-lint: $(GOLANGCI_LINT) ## Download golangci-lint locally if necessary.
$(GOLANGCI_LINT): $(TOOL_BIN)
	$(call go_install_tool,$(GOLANGCI_LINT),github.com/golangci/golangci-lint/cmd/golangci-lint,$(GOLANGCI_LINT_VERSION))

.PHONY: helmify
helmify: $(HELMIFY) ## Download helmify locally if necessary.
$(HELMIFY): $(TOOL_BIN)
	$(call go_install_tool,$(HELMIFY),github.com/arttor/helmify/cmd/helmify,$(HELMIFY_VERSION))

.PHONY: yq
yq: $(YQ) ## Download yq locally if necessary.
$(YQ): $(TOOL_BIN)
	$(call go_install_tool,$(YQ),github.com/mikefarah/yq/v4,$(YQ_VERSION))

.PHONY: sqlc
sqlc: $(SQLC) ## Download sqlc locally if necessary.
$(SQLC): $(TOOL_BIN)
	$(call go_install_tool,$(SQLC),github.com/sqlc-dev/sqlc/cmd/sqlc,$(SQLC_VERSION))

.PHONY: checkov
checkov: $(CHECKOV) ## Download checkov locally if necessary.
$(CHECKOV): $(TOOL_BIN)
	@echo "Installing checkov $(CHECKOV_VERSION) to $(TOOL_BIN)"; \
	mkdir -p $(TOOL_BIN); \
	OS=$$(uname -s); \
	ARCH=$$(uname -m); \
	case "$$OS" in \
		Darwin) \
			CHECKOV_OS="darwin"; \
			CHECKOV_ARCH="X86_64"; \
			;; \
		Linux) \
			CHECKOV_OS="linux"; \
			case "$$ARCH" in \
				x86_64|amd64) \
					CHECKOV_ARCH="X86_64"; \
					;; \
				arm64|aarch64) \
					CHECKOV_ARCH="arm64"; \
					;; \
				*) \
					echo "Unsupported architecture: $$ARCH"; \
					exit 1; \
					;; \
			esac; \
			;; \
		MINGW*|MSYS*|CYGWIN*) \
			CHECKOV_OS="windows"; \
			CHECKOV_ARCH="X86_64"; \
			;; \
		*) \
			echo "Unsupported OS: $$OS"; \
			exit 1; \
			;; \
	esac; \
	CHECKOV_URL="https://github.com/bridgecrewio/checkov/releases/download/$(CHECKOV_VERSION)/checkov_$${CHECKOV_OS}_$${CHECKOV_ARCH}.zip"; \
	echo "Downloading checkov from $$CHECKOV_URL"; \
	curl -fsSL "$$CHECKOV_URL" -o /tmp/checkov.zip; \
	mkdir -p /tmp/checkov-extract; \
	unzip -q /tmp/checkov.zip -d /tmp/checkov-extract; \
	mv /tmp/checkov-extract/dist/checkov $(CHECKOV); \
	rm -rf /tmp/checkov.zip /tmp/checkov-extract; \
	chmod +x $(CHECKOV); \
	echo "checkov $(CHECKOV_VERSION) installed successfully";
