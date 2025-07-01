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
	openchoreo-api:$(PROJECT_DIR)/cmd/openchoreo-api/Dockerfile:$(PROJECT_DIR)

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
docker.build.quick-start: go.build-multiarch.choreoctl
docker.build.openchoreo-api: go.build-multiarch.openchoreo-api

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
