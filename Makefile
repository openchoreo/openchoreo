
PROJECT_DIR := $(realpath $(dir $(abspath $(lastword $(MAKEFILE_LIST)))))

# Read the version from the VERSION file
RELEASE_VERSION ?= $(shell cat VERSION)
# Default image repository to use for building/pushing images
IMG_REPO ?= ghcr.io/openchoreo/controller
# Image URL to use all building/pushing image targets
IMG ?= $(IMG_REPO):v$(RELEASE_VERSION)

# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set)
ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif

# CONTAINER_TOOL defines the container tool to be used for building images.
# Be aware that the target commands are only tested with Docker which is
# scaffolded by default. However, you might want to replace it to use other
# tools. (i.e. podman)
CONTAINER_TOOL ?= docker

# Setting SHELL to bash allows bash commands to be executed by recipes.
# Options are set to exit when a recipe line exits non-zero or a piped command fails.
SHELL = /usr/bin/env bash -o pipefail
.SHELLFLAGS = -ec

.PHONY: all
all: build

##@ General

# The help target prints out all targets with their descriptions organized
# beneath their categories. The categories are represented by '##@' and the
# target descriptions by '##'. The awk command is responsible for reading the
# entire set of makefiles included in this invocation, looking for lines of the
# file as xyz: ## something, and then pretty-format the target and help. Then,
# if there's a line with ##@ something, that gets pretty-printed as a category.
# More info on the usage of ANSI control characters for terminal formatting:
# https://en.wikipedia.org/wiki/ANSI_escape_code#SGR_parameters
# More info on the awk command:
# http://linuxcommand.org/lc3_adv_awk.php

.PHONY: help
help: ## Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9.%-]+:.*?##/ { printf "  \033[36m%-24s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

##@ Deployment

ifndef ignore-not-found
  ignore-not-found = false
endif

.PHONY: install
install: manifests kustomize ## Install CRDs into the K8s cluster specified in ~/.kube/config.
	$(KUSTOMIZE) build config/crd | $(KUBECTL) apply -f -

.PHONY: uninstall
uninstall: manifests kustomize ## Uninstall CRDs from the K8s cluster specified in ~/.kube/config. Call with ignore-not-found=true to ignore resource not found errors during deletion.
	$(KUSTOMIZE) build config/crd | $(KUBECTL) delete --ignore-not-found=$(ignore-not-found) -f -

.PHONY: deploy
deploy: manifests kustomize ## Deploy controller to the K8s cluster specified in ~/.kube/config.
	cd config/manager && $(KUSTOMIZE) edit set image controller=${IMG}
	$(KUSTOMIZE) build config/default | $(KUBECTL) apply -f -

.PHONY: undeploy
undeploy: kustomize ## Undeploy controller from the K8s cluster specified in ~/.kube/config. Call with ignore-not-found=true to ignore resource not found errors during deletion.
	$(KUSTOMIZE) build config/default | $(KUBECTL) delete --ignore-not-found=$(ignore-not-found) -f -

#-----------------------------------------------------------------------------
# Choreoctl Distribution targets
#-----------------------------------------------------------------------------
VERSION ?= $(shell git describe --tags --always --dirty)
DATE = $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
LDFLAGS = -ldflags "-X github.com/openchoreo/openchoreo/pkg/cli/version.Version=$(VERSION) -X github.com/openchoreo/openchoreo/pkg/cli/version.BuildDate=$(DATE)"

# Supported platforms - space separated list for proper iteration
PLATFORMS = darwin/amd64 darwin/arm64 linux/amd64 linux/arm64 windows/amd64

BUILD_DIR = dist/choreoctl

.PHONY: choreoctl-clean
choreoctl-clean:
	rm -rf $(BUILD_DIR)

.PHONY: choreoctl-prepare
choreoctl-prepare: choreoctl-clean
	mkdir -p $(BUILD_DIR)

.PHONY: choreoctl-dist
choreoctl-dist: choreoctl-prepare
	$(foreach platform,$(PLATFORMS),$(call build-choreoctl-platform,$(platform)))

define build-choreoctl-platform
    $(eval OS := $(word 1,$(subst /, ,$(1))))
    $(eval ARCH := $(word 2,$(subst /, ,$(1))))
    $(eval OUTPUT := $(BUILD_DIR)/$(OS)-$(ARCH)/choreoctl$(if $(filter windows,$(OS)),.exe))
    @echo "Building choreoctl for $(OS)/$(ARCH)..."
    @mkdir -p $(BUILD_DIR)/$(OS)-$(ARCH)
    @CGO_ENABLED=0 GOOS=$(OS) GOARCH=$(ARCH) go build $(LDFLAGS) -o $(OUTPUT) ./cmd/choreoctl
    @if [ "$(OS)" = "linux" ]; then \
        cd $(BUILD_DIR)/$(OS)-$(ARCH) && tar -czf ../choreoctl-$(VERSION)-$(OS)-$(ARCH).tar.gz choreoctl; \
    elif [ "$(OS)" = "windows" ]; then \
        cd $(BUILD_DIR)/$(OS)-$(ARCH) && zip ../choreoctl-$(VERSION)-$(OS)-$(ARCH).zip choreoctl.exe; \
    else \
        cd $(BUILD_DIR)/$(OS)-$(ARCH) && zip ../choreoctl-$(VERSION)-$(OS)-$(ARCH).zip choreoctl; \
    fi
endef

.PHONY: choreoctl-checksums
choreoctl-checksums: choreoctl-dist
	@echo "Generating checksums for choreoctl packages..."
	@cd $(BUILD_DIR) && find . -maxdepth 1 -type f \( -name "*.zip" -o -name "*.tar.gz" \) -exec sh -c 'shasum -a 256 {} > {}.sha256' \;

.PHONY: choreoctl-installer
choreoctl-installer: choreoctl-dist
	@echo "Preparing choreoctl installer script..."
	@cp install/choreoctl-install.sh $(BUILD_DIR)/
	@sed -i.bak "s/CHOREOCTL_VERSION=.*/CHOREOCTL_VERSION=\"$(VERSION)\"/" $(BUILD_DIR)/choreoctl-install.sh
	@rm $(BUILD_DIR)/choreoctl-install.sh.bak
	@chmod +x $(BUILD_DIR)/choreoctl-install.sh

.PHONY: choreoctl-release
choreoctl-release: choreoctl-dist choreoctl-checksums choreoctl-installer ## Prepare choreoctl release with all artifacts
	@echo "Choreoctl release v$(VERSION) prepared in $(BUILD_DIR)"
	@echo "Date: $(DATE)"
	@ls -la $(BUILD_DIR)

#-----------------------------------------------------------------------------
# Makefile includes
#-----------------------------------------------------------------------------
include make/common.mk
include make/tools.mk
include make/golang.mk
include make/lint.mk
include make/docker.mk
include make/kube.mk
include make/helm.mk
