# This makefile contains all the make targets related to the building of Go binaries.

# Define Go command and GOPATH to use
GO := go
GOPATH ?= $(shell go env GOPATH)

# Current platform for local build
GO_CURRENT_PLATFORM := $(shell $(GO) env GOOS)/$(shell $(GO) env GOARCH)
# Define the target platforms for the go build
GO_TARGET_PLATFORMS ?= linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64 windows/arm64

# Define the binaries that need to be built.
# Format: <binary_name>:<main_file_path>
GO_BUILD_BINARIES := \
	manager:$(PROJECT_DIR)/cmd/main.go \
	choreoctl:$(PROJECT_DIR)/cmd/choreoctl/main.go \
	openchoreo-api:$(PROJECT_DIR)/cmd/openchoreo-api/main.go \
	observer:$(PROJECT_DIR)/cmd/observer/main.go

GO_BUILD_BINARY_NAMES := $(foreach b,$(GO_BUILD_BINARIES),$(word 1,$(subst :, ,$(b))))

GO_BUILD_OUTPUT_DIR := $(PROJECT_BIN_DIR)/dist

GO_VERSION_PACKAGE := github.com/openchoreo/openchoreo/internal/version

# Define link flags for the Go build
GO_LDFLAGS_COMMON ?= -s -w
GO_LDFLAGS_BUILD_DATA ?= \
	-X $(GO_VERSION_PACKAGE).buildTime=$(shell date +%Y-%m-%dT%H:%M:%S%z) \
	-X $(GO_VERSION_PACKAGE).gitRevision=$(GIT_REV) \
	-X $(GO_VERSION_PACKAGE).version=$(RELEASE_VERSION)

# Helper functions
get_go_main_package_path = $(word 2, $(subst :, ,$(filter $(1):%, $(GO_BUILD_BINARIES))))

define go_build
	$(eval COMMAND := $(1))
	$(eval MAIN_PACKAGE_PATH := $(call get_go_main_package_path,$(COMMAND)))
	$(eval OS := $(call get_platform_os,$(2)))
	$(eval ARCH := $(call get_platform_arch,$(2)))
	$(eval OUTPUT_PATH := $(GO_BUILD_OUTPUT_DIR)/$(OS)/$(ARCH))
	$(call log_info, Building binary '$(COMMAND)' for $(OS)/$(ARCH))
	mkdir -p $(OUTPUT_PATH)
	$(eval GO_LDFLAGS := $(GO_LDFLAGS_COMMON))
	$(eval GO_LDFLAGS += $(GO_LDFLAGS_BUILD_DATA))
	$(eval GO_LDFLAGS += -X $(GO_VERSION_PACKAGE).componentName=$$(COMMAND))
	CGO_ENABLED=0 GOOS=$(OS) GOARCH=$(ARCH) \
		$(GO) build -o $(OUTPUT_PATH)/$(COMMAND) -ldflags "$(GO_LDFLAGS)" \
		$(MAIN_PACKAGE_PATH)
endef

define package_binary
	$(eval BINARY_NAME := $(1))
	$(eval OS := $(call get_platform_os,$(2)))
    $(eval ARCH := $(call get_platform_arch,$(2)))
    $(eval OUTPUT_PATH := $(GO_BUILD_OUTPUT_DIR)/$(OS)/$(ARCH))
	$(eval BIN_PATH := $(OUTPUT_PATH)/$(BINARY_NAME))
	$(eval PACKAGE_FILE_NAME := $(BINARY_NAME)_v$(RELEASE_VERSION)_$(OS)_$(ARCH))
	$(call log_info, Packaging binary '$(BINARY_NAME)' for $(OS)/$(ARCH))
	if [ -f $(BIN_PATH) ]; then \
		if [ $(OS) = "windows" ]; then \
			zip -rj $(OUTPUT_PATH)/$(PACKAGE_FILE_NAME).zip $(BIN_PATH); \
		else \
			 tar -zcvf $(OUTPUT_PATH)/$(PACKAGE_FILE_NAME).tar.gz -C $(OUTPUT_PATH) $(BINARY_NAME); \
		fi; \
	else \
		$(call log_info, "Skipping binary '$(BINARY_NAME)': $(BIN_PATH) not found"); \
	fi
endef

##@ Golang

#-----------------------------------------------------------------------------
# Go Build targets
#-----------------------------------------------------------------------------

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

#-----------------------------------------------------------------------------
# Go Package targets
#-----------------------------------------------------------------------------

.PHONY: go.package.%
go.package.%: ## Package the multi arch binaries. Ex: make go.package.choreoctl
	@if [ -z "$(filter $*,$(GO_BUILD_BINARY_NAMES))" ]; then \
		$(call log_error, Invalid go package target '$*'); \
		exit 1; \
	fi
	@$(foreach platform,$(GO_TARGET_PLATFORMS), \
    	$(call package_binary, $*, $(platform)); \
    )

.PHONY: go.package
go.package: $(addprefix go.package., $(GO_BUILD_BINARY_NAMES)) ## Package all binaries for multiple platforms.

#-----------------------------------------------------------------------------
# Go Run and Install targets
#-----------------------------------------------------------------------------

.PHONY: go.run.%
go.run.%: ## Run the go program using go run. Ex: make go.run.choreoctl GO_RUN_ARGS="version"
	@if [ -z "$(filter $*,$(GO_BUILD_BINARY_NAMES))" ]; then \
		$(call log_error, Invalid go run target '$*'); \
		exit 1; \
	fi
	@$(eval COMMAND := $(word 1,$(subst ., ,$*)))
	@$(eval MAIN_PACKAGE_PATH := $(call get_go_main_package_path,$(COMMAND)))
	$(GO) run $(MAIN_PACKAGE_PATH) $(GO_RUN_ARGS)

.PHONY: go.install.%
go.install.%: go.build.% ## Install the go program to the GOBIN directory. Ex: make go.install.choreoctl
	@if [ -z "$(filter $*,$(GO_BUILD_BINARY_NAMES))" ]; then \
		$(call log_error, Invalid go install target '$*'); \
		exit 1; \
	fi
	@$(eval COMMAND := $(word 1,$(subst ., ,$*)))
	@$(eval OS := $(call get_platform_os,$(GO_CURRENT_PLATFORM)))
	@$(eval ARCH := $(call get_platform_arch,$(GO_CURRENT_PLATFORM)))
	@cp $(GO_BUILD_OUTPUT_DIR)/$(OS)/$(ARCH)/$(COMMAND) $(GOPATH)/bin/$(COMMAND)

.PHONY: go.install
go.install: $(addprefix go.install., $(GO_BUILD_BINARY_NAMES)) ## Install all binaries to the GOBIN directory.

#-----------------------------------------------------------------------------
# Go Other targets
#-----------------------------------------------------------------------------

.PHONY: fmt
fmt: ## Run go fmt against code.
	go fmt ./...

.PHONY: vet
vet: ## Run go vet against code.
	go vet ./...

# ENVTEST_K8S_VERSION refers to the version of kubebuilder assets to be downloaded by envtest binary.
ENVTEST_K8S_VERSION := 1.32.0

.PHONY: test
test: manifests generate fmt vet envtest ## Run tests.
	KUBEBUILDER_ASSETS="$(shell $(ENVTEST) use $(ENVTEST_K8S_VERSION) --bin-dir $(TOOL_BIN) -p path)" go test $$(go list ./... | grep -v /e2e) -coverprofile cover.out

.PHONY: go.mod.tidy
go.mod.tidy: ## Run go mod tidy to clean up go.mod file.
	@$(call log, "Running go mod tidy")
	@$(GO) mod tidy

.PHONY: go.mod.lint
go.mod.lint: go.mod.tidy ## Lint go.mod file.
