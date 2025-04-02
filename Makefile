
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
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9.%-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)


##@ Build

.PHONY: build
build: manifests generate fmt vet ## Build manager binary.
	go build -o bin/manager cmd/main.go

.PHONY: run
run: manifests generate fmt vet ## Run a controller from your host.
	go run ./cmd/main.go

# If you wish to build the manager image targeting other platforms you can use the --platform flag.
# (i.e. docker build --platform linux/arm64). However, you must enable docker buildKit for it.
# More info: https://docs.docker.com/develop/develop-images/build_enhancements/
.PHONY: docker-build
docker-build: ## Build docker image with the manager.
	$(CONTAINER_TOOL) build -t ${IMG} .

.PHONY: docker-push
docker-push: ## Push docker image with the manager.
	$(CONTAINER_TOOL) push ${IMG}

.PHONY: docker-push-latest
docker-push-latest: ## Push docker image with the manager with the latest tag.
	$(CONTAINER_TOOL) tag ${IMG} $(IMG_REPO):latest
	$(CONTAINER_TOOL) push $(IMG_REPO):latest

# PLATFORMS defines the target platforms for the manager image be built to provide support to multiple
# architectures. (i.e. make docker-buildx IMG=myregistry/mypoperator:0.0.1). To use this option you need to:
# - be able to use docker buildx. More info: https://docs.docker.com/build/buildx/
# - have enabled BuildKit. More info: https://docs.docker.com/develop/develop-images/build_enhancements/
# - be able to push the image to your registry (i.e. if you do not set a valid value via IMG=<myregistry/image:<tag>> then the export will fail)
# To adequately provide solutions that are compatible with multiple platforms, you should consider using this option.
PLATFORMS ?= linux/arm64,linux/amd64,linux/s390x,linux/ppc64le
.PHONY: docker-buildx
docker-buildx: ## Build and push docker image for the manager for cross-platform support
	# copy existing Dockerfile and insert --platform=${BUILDPLATFORM} into Dockerfile.cross, and preserve the original Dockerfile
	sed -e '1 s/\(^FROM\)/FROM --platform=\$$\{BUILDPLATFORM\}/; t' -e ' 1,// s//FROM --platform=\$$\{BUILDPLATFORM\}/' Dockerfile > Dockerfile.cross
	- $(CONTAINER_TOOL) buildx create --name choreo-builder
	$(CONTAINER_TOOL) buildx use choreo-builder
	- $(CONTAINER_TOOL) buildx build --push --platform=$(PLATFORMS) --tag ${IMG} -f Dockerfile.cross .
	- $(CONTAINER_TOOL) buildx rm choreo-builder
	rm Dockerfile.cross

.PHONY: build-installer
build-installer: manifests generate kustomize ## Generate a consolidated YAML with CRDs and deployment.
	mkdir -p dist
	cd config/manager && $(KUSTOMIZE) edit set image controller=${IMG}
	$(KUSTOMIZE) build config/default > dist/install.yaml

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
# Choreoctl targets
#-----------------------------------------------------------------------------

# Build choreoctl binary
.PHONY: choreoctl
choreoctl:
	go build -o choreoctl ./cmd/choreoctl

# Build and install choreoctl binary to $GOBIN
.PHONY: install-choreoctl
install-choreoctl: choreoctl
	go install ./cmd/choreoctl

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
# Release targets
#-----------------------------------------------------------------------------

# This target is used to prepare the release for the next version.
# It updates the VERSION file and the necessary files for the next release that should be committed.
# Run make prepare-release VERSION=x.y.z
# Example: make prepare-release VERSION=0.1.0
.PHONY: prepare-release
prepare-release:
	@if ! command -v yq >/dev/null 2>&1; then \
		echo "Error: yq is not installed. Please install yq from https://github.com/mikefarah/yq" >&2; \
		exit 1; \
	fi
	@if [ -z "$(VERSION)" ]; then \
		echo "VERSION is not set. Please set the VERSION variable"; \
		echo "Example: make prepare-release VERSION=v0.1.0"; \
		exit 1; \
	fi
	@if [[ ! "$(VERSION)" =~ ^[0-9]+\.[0-9]+\.[0-9]+$$ ]]; then \
		echo "VERSION=$(VERSION) does not match the SemVer pattern (X.X.X)"; \
		exit 1; \
	fi
	@echo "$(VERSION)" > VERSION
	@yq eval '.version = "$(VERSION)" | .appVersion = "v$(VERSION)"' install/helm/choreo/Chart.yaml -i
	@yq eval '.version = "$(VERSION)" | .appVersion = "v$(VERSION)"' install/helm/cilium/Chart.yaml -i



#-----------------------------------------------------------------------------
# quick-start build & push targets
#-----------------------------------------------------------------------------
IMAGE_NAME=ghcr.io/openchoreo/quick-start:v$(RELEASE_VERSION)
IMAGE_NAME_LATEST=ghcr.io/openchoreo/quick-start:latest
DOCKER_PATH=install/quick-start
SAMPLE_SOURCE=samples/web-applications/container-image/react-starter.yaml

.PHONY: quick-start-docker-build
quick-start-docker-build:
	@echo "Building Docker image for quick start..."
	$(CONTAINER_TOOL) build -f $(DOCKER_PATH)/Dockerfile -t $(IMAGE_NAME) .
	@echo "Build complete!"

.PHONY: quick-start-docker-push
quick-start-docker-push:
	@echo "Pushing Docker image for quick start..."
	$(CONTAINER_TOOL) push $(IMAGE_NAME)
	@echo "Push complete!"

.PHONY: quick-start-docker-push-latest
quick-start-docker-push-latest: ## Push docker image of dev container for quick start with the latest tag.
	$(CONTAINER_TOOL) tag ${IMAGE_NAME} $(IMAGE_NAME_LATEST)
	$(CONTAINER_TOOL) push $(IMAGE_NAME_LATEST)


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
