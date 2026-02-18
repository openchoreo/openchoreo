# This makefile contains all the make targets related to the docker images.

# Define the docker command to use
DOCKER := docker

# Define the docker buildx builder name
BUILDER_NAME ?= "open-choreo-builder"

# Define general image details
IMAGE_REPO_PREFIX ?= ghcr.io/openchoreo
TAG ?= latest-dev

# Current platform for image build
# OS will be always linux
IMAGE_CURRENT_PLATFORM := linux/$(shell $(GO) env GOARCH)
# Define the target platforms for the multi arch image build
IMAGE_TARGET_PLATFORMS ?= linux/amd64 linux/arm64

# Convert spaces to comma for docker buildx
empty :=
space := $(empty) $(empty)
comma := ,
BUILDX_TARGET_PLATFORMS := $(subst $(space),$(comma),$(IMAGE_TARGET_PLATFORMS))

# Define the docker images that need to be built with corresponding dockerfile and the context.
# Format: <image_name>:<dockerfile_path>:<docker_context_path>
# NOTE: If the `controller` image is updated, make sure to update the `HELM_CONTROLLER_IMAGE` in helm.mk
DOCKER_BUILD_IMAGES := \
	controller:$(PROJECT_DIR)/Dockerfile:$(PROJECT_DIR) \
	quick-start:$(PROJECT_DIR)/install/quick-start/Dockerfile:$(PROJECT_DIR) \
	init-observability-opensearch:$(PROJECT_DIR)/install/init/observability/opensearch/Dockerfile:$(PROJECT_DIR) \
	openchoreo-api:$(PROJECT_DIR)/cmd/openchoreo-api/Dockerfile:$(PROJECT_DIR) \
	observer:$(PROJECT_DIR)/cmd/observer/Dockerfile:$(PROJECT_DIR) \
	ai-rca-agent:$(PROJECT_DIR)/rca-agent/Dockerfile:$(PROJECT_DIR)/rca-agent \
	openchoreo-cli:$(PROJECT_DIR)/cmd/occ/Dockerfile:$(PROJECT_DIR) \
	cluster-gateway:$(PROJECT_DIR)/cmd/cluster-gateway/Dockerfile:$(PROJECT_DIR) \
	cluster-agent:$(PROJECT_DIR)/cmd/cluster-agent/Dockerfile:$(PROJECT_DIR) \
	events-collector:$(PROJECT_DIR)/cmd/events-collector/Dockerfile:$(PROJECT_DIR)

DOCKER_BUILD_IMAGE_NAMES := $(foreach b,$(DOCKER_BUILD_IMAGES),$(word 1,$(subst :, ,$(b))))

# Helper functions
get_dockerfile_path = $(word 2, $(subst :, ,$(filter $(1):%, $(DOCKER_BUILD_IMAGES))))
get_docker_context_path = $(word 3, $(subst :, ,$(filter $(1):%, $(DOCKER_BUILD_IMAGES))))

# Helper function for the multi-arch build.
# 1st param ($1) = image name,
# 2nd param ($2) = target platforms (e.g., "linux/amd64,linux/arm64").
# 3rd param ($3) = extra arguments (e.g., "--push" or empty).
define docker_build
	$(eval IMAGE := $(1))
	$(eval TARGET_PLATFORMS := $(2))
	$(eval DOCKERFILE_PATH := $(call get_dockerfile_path,$(IMAGE)))
	$(eval DOCKER_CONTEXT_PATH := $(call get_docker_context_path,$(IMAGE)))
	$(call log_info, Building image '$(IMAGE)' for platform(s) $(TARGET_PLATFORMS))
	$(DOCKER) buildx build --platform $(TARGET_PLATFORMS) \
		-t $(IMAGE_REPO_PREFIX)/$(IMAGE):$(TAG) \
		-f $(DOCKERFILE_PATH) $(DOCKER_CONTEXT_PATH) $(3)
endef

##@ Docker

# Define the build target for a docker image
# This will build the docker image for the current platform's architecture
# Ex: make docker.build.controller, make docker.build.quick-start
.PHONY: docker.build.%
docker.build.%:  ## Build a docker image for the current platform. Ex: make docker.build.controller
	@if [ -z "$(filter $*,$(DOCKER_BUILD_IMAGE_NAMES))" ]; then \
		$(call log_error, Invalid image build target '$*'); \
		exit 1; \
	fi
	@$(call docker_build,$*,$(IMAGE_CURRENT_PLATFORM),"--load")

# Set dependent go build target for the docker images that are built for the current platform's architecture
docker.build.controller: go.build-multiarch.manager
docker.build.quick-start: go.build-multiarch.occ
docker.build.openchoreo-api: go.build-multiarch.openchoreo-api
docker.build.observer: go.build-multiarch.observer
docker.build.ai-rca-agent:  # Python project - no Go build dependency
docker.build.cluster-gateway: go.build-multiarch.cluster-gateway
docker.build.cluster-agent: go.build-multiarch.cluster-agent
docker.build.events-collector: go.build-multiarch.events-collector

# Set target architecture for the go build that is required for the docker image
docker.build.%: GO_TARGET_PLATFORMS:=$(IMAGE_CURRENT_PLATFORM)

.PHONY: docker.build
docker.build: $(addprefix docker.build., $(DOCKER_BUILD_IMAGE_NAMES)) ## Build all docker images for the current platform.

# Image push target for the docker images that are built for the current platform's architecture
.PHONY: docker.push.%
docker.push.%: docker.build.%
	@if [ -z "$(filter $*,$(DOCKER_BUILD_IMAGE_NAMES))" ]; then \
		$(call log_error, Invalid image push target '$*'); \
		exit 1; \
	fi
	$(eval IMAGE := $*)
	$(DOCKER) push $(IMAGE_REPO_PREFIX)/$(IMAGE):$(TAG)

.PHONY: docker.push
docker.push: $(addprefix docker.push., $(DOCKER_BUILD_IMAGE_NAMES))


# Setup the docker buildx for multi arch build
# This will create a new builder with the name $(BUILDER_NAME) and set it as the default builder
# If you are using non desktop docker, you need to setup the emulator for the target platforms
# Please refer to https://docs.docker.com/build/building/multi-platform/#install-qemu-manually
.PHONY: docker.setup-multiarch
docker.setup-multiarch:
	@$(DOCKER) buildx inspect $(BUILDER_NAME) >/dev/null 2>&1 || \
		$(DOCKER) buildx create --name $(BUILDER_NAME) --use --platform "${BUILDX_TARGET_PLATFORMS}"

# Build the docker image for the multiple platforms with docker buildx
# This assumes the docker buildx is already setup with a correct builder that supports multi arch build
# See https://docs.docker.com/build/building/multi-platform/#prerequisites for more details
# Ex: make docker.build-multiarch.controller, make docker.build-multiarch.quick-start
.PHONY: docker.build-multiarch.%
docker.build-multiarch.%: ## Build a docker image for multiple platforms. Ex: make docker.build-multiarch.controller
	@if [ -z "$(filter $*,$(DOCKER_BUILD_IMAGE_NAMES))" ]; then \
		$(call log_error, Invalid image multiarch build target '$*'); \
		exit 1; \
	fi
	@$(call docker_build,$*,$(BUILDX_TARGET_PLATFORMS),)


.PHONY: docker.build-multiarch
docker.build-multiarch: $(addprefix docker.build-multiarch., $(DOCKER_BUILD_IMAGE_NAMES)) ## Build all docker images for the multiple platforms.

# Image push target for the docker images that are built for the multiple platforms
.PHONY: docker.push-multiarch.%
docker.push-multiarch.%: ## Push a docker image for multiple platforms. Ex: make docker.push-multiarch.controller
	@if [ -z "$(filter $*,$(DOCKER_BUILD_IMAGE_NAMES))" ]; then \
		$(call log_error, Invalid image multiarch push target '$*'); \
		exit 1; \
	fi
	@# See: https://github.com/orgs/community/discussions/45969 for details on the --sbom and --provenance flags
	@$(call docker_build,$*,$(BUILDX_TARGET_PLATFORMS),--push --provenance=false --sbom=false)


.PHONY: docker.push-multiarch
docker.push-multiarch: $(addprefix docker.push-multiarch., $(DOCKER_BUILD_IMAGE_NAMES)) ## Push all docker images for the multiple platforms.

# Retag existing images in the registry from SOURCE_TAG to NEW_TAG
# This is useful for promoting images from commit SHA tags to release tags
# Usage: make docker.retag-registry SOURCE_TAG=abc123 NEW_TAG=v1.0.0
.PHONY: docker.retag-registry
docker.retag-registry: ## Retag existing registry images from SOURCE_TAG to NEW_TAG. Usage: make docker.retag-registry SOURCE_TAG=abc123 NEW_TAG=v1.0.0
	@if [ -z "$(SOURCE_TAG)" ]; then \
		$(call log_error, SOURCE_TAG is required. Usage: make docker.retag-registry SOURCE_TAG=abc123 NEW_TAG=v1.0.0); \
		exit 1; \
	fi
	@if [ -z "$(NEW_TAG)" ]; then \
		$(call log_error, NEW_TAG is required. Usage: make docker.retag-registry SOURCE_TAG=abc123 NEW_TAG=v1.0.0); \
		exit 1; \
	fi
	@$(call log_info, Retagging images from $(SOURCE_TAG) to $(NEW_TAG))
	@$(foreach image,$(DOCKER_BUILD_IMAGE_NAMES), \
		echo "Retagging $(IMAGE_REPO_PREFIX)/$(image):$(SOURCE_TAG) -> $(IMAGE_REPO_PREFIX)/$(image):$(NEW_TAG)" && \
		$(DOCKER) buildx imagetools create \
			-t $(IMAGE_REPO_PREFIX)/$(image):$(NEW_TAG) \
			$(IMAGE_REPO_PREFIX)/$(image):$(SOURCE_TAG) || exit 1; \
	)
	@$(call log_info, Successfully retagged all images)

# Quick-start dev mode - builds images from HEAD and runs quick-start with local helm charts
QUICK_START_DEV_IMAGES := controller openchoreo-api observer cluster-gateway cluster-agent
QUICK_START_CONTAINER_NAME := openchoreo-quick-start-dev

.PHONY: quick-start.dev
quick-start.dev: TAG=latest-dev
quick-start.dev: $(addprefix docker.build., $(QUICK_START_DEV_IMAGES)) docker.build.quick-start ## Build and run quick-start with HEAD images and helm charts
	@$(call log_info, Stopping any existing quick-start container)
	@$(DOCKER) rm -f $(QUICK_START_CONTAINER_NAME) 2>/dev/null || true
	@$(call log_info, Running quick-start container in dev mode)
	@$(DOCKER) run -it --rm \
		--name $(QUICK_START_CONTAINER_NAME) \
		--privileged \
		-v /var/run/docker.sock:/var/run/docker.sock \
		-v $(PROJECT_DIR)/install/helm:/helm:ro \
		-v openchoreo-quick-start-state:/state \
		--network=host \
		-e DEV_MODE=true \
		-e OPENCHOREO_VERSION=$(TAG) \
		$(IMAGE_REPO_PREFIX)/quick-start:$(TAG) \
		/app/install.sh
