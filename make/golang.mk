# This makefile contains all the make targets related to the building of Go binaries.

# Define Go command to use
GO := go

# Current platform for local build
GO_CURRENT_PLATFORM := $(shell $(GO) env GOOS)/$(shell $(GO) env GOARCH)
# Define the target platforms for the go build
GO_TARGET_PLATFORMS ?= linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64 windows/arm64

# Define the binaries that need to be built.
# Format: <binary_name>:<main_file_path>
GO_BUILD_BINARIES := \
	manager:$(PROJECT_DIR)/cmd/main.go \
	choreoctl:$(PROJECT_DIR)/cmd/choreoctl/main.go

GO_BUILD_BINARY_NAMES := $(foreach b,$(GO_BUILD_BINARIES),$(word 1,$(subst :, ,$(b))))

GO_BUILD_OUTPUT_DIR := $(PROJECT_BIN_DIR)/dist

# Define link flags for the Go build
GO_LDFLAGS ?= -s -w

# Helper functions
get_go_main_package_path = $(word 2, $(subst :, ,$(filter $(1):%, $(GO_BUILD_BINARIES))))

define go_build
	$(eval COMMAND := $(1))
	$(eval MAIN_PACKAGE_PATH := $(call get_go_main_package_path,$(COMMAND)))
	$(eval OS := $(call get_platform_os,$(2)))
	$(eval ARCH := $(call get_platform_arch,$(2)))
	$(eval OUTPUT_PATH := $(GO_BUILD_OUTPUT_DIR)/$(OS)/$(ARCH))
	$(call log_info, Building binary '$(COMMAND)' for $(OS)/$(ARCH))
	@mkdir -p $(OUTPUT_PATH)
	CGO_ENABLED=0 GOOS=$(OS) GOARCH=$(ARCH) \
		$(GO) build -o $(OUTPUT_PATH)/$(COMMAND) -ldflags "$(GO_LDFLAGS)" \
		$(MAIN_PACKAGE_PATH)
endef

##@ Golang

# Define the build target for a binary
# This will build the binary for the current platform
# Ex: make go.build.manager, make go.build.choreoctl
.PHONY: go.build.%
go.build.%: ## Build a binary for the current platform. Ex: make go.build.manager
	@if [ -z "$(filter $*,$(GO_BUILD_BINARY_NAMES))" ]; then \
		$(call log_error, Invalid go build target '$*'); \
		exit 1; \
	fi
	@$(call go_build, $*, $(GO_CURRENT_PLATFORM))

.PHONY: go.build
go.build: $(addprefix go.build., $(GO_BUILD_BINARY_NAMES)) ## Build all binaries for the current platform.

# Build the binary for the multiple platforms via cross-compilation
# Ex: make go.build.multiarch.manager, make go.build.multiarch.choreoctl
.PHONY: go.build-multiarch.%
go.build-multiarch.%: ## Build a binary for multiple platforms. Ex: make go.build-multiarch.manager
	@if [ -z "$(filter $*,$(GO_BUILD_BINARY_NAMES))" ]; then \
    		$(call log_error, Invalid go multiarch build target '$*'); \
    		exit 1; \
    fi
	@$(foreach platform,$(GO_TARGET_PLATFORMS), \
	  	$(call go_build, $*, $(platform)); \
	)

.PHONY: go.build-multiarch
go.build-multiarch: $(addprefix go.build-multiarch., $(GO_BUILD_BINARY_NAMES)) ## Build all binaries for multiple platforms.

.PHONY: fmt
fmt: ## Run go fmt against code.
	go fmt ./...

.PHONY: vet
vet: ## Run go vet against code.
	go vet ./...

# ENVTEST_K8S_VERSION refers to the version of kubebuilder assets to be downloaded by envtest binary.
ENVTEST_K8S_VERSION := 1.31.0

.PHONY: test
test: manifests generate fmt vet envtest ## Run tests.
	KUBEBUILDER_ASSETS="$(shell $(ENVTEST) use $(ENVTEST_K8S_VERSION) --bin-dir $(TOOL_BIN) -p path)" go test $$(go list ./... | grep -v /e2e) -coverprofile cover.out

.PHONY: go.mod.tidy
go.mod.tidy: ## Run go mod tidy to clean up go.mod file.
	@$(call log, "Running go mod tidy")
	@$(GO) mod tidy

.PHONY: go.mod.lint
go.mod.lint: go.mod.tidy ## Lint go.mod file.
