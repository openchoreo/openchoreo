# Minimal Kind cluster management for OpenChoreo development

# Configuration
KIND_CLUSTER_NAME ?= openchoreo
OPENCHOREO_IMAGE_TAG ?= dev
OPENCHOREO_NAMESPACE ?= openchoreo
KIND_EXTERNAL_DNS ?= 8.8.8.8
KIND_NETWORK ?= openchoreo

# Cilium configuration
CILIUM_VERSION := 1.18.2
CILIUM_ENVOY_VERSION := v1.34.4-1754895458-68cffdfa568b6b226d70a7ef81fc65dda3b890bf

# Paths
DEV_SCRIPTS_DIR := $(PROJECT_DIR)/install/dev
KIND_SCRIPT := $(DEV_SCRIPTS_DIR)/kind.sh
HELM_DIR := $(PROJECT_DIR)/install/helm/openchoreo

# Image names
IMAGE_REPO_PREFIX ?= ghcr.io/openchoreo
CONTROLLER_IMAGE := $(IMAGE_REPO_PREFIX)/controller:$(OPENCHOREO_IMAGE_TAG)
API_IMAGE := $(IMAGE_REPO_PREFIX)/openchoreo-api:$(OPENCHOREO_IMAGE_TAG)
UI_IMAGE := $(IMAGE_REPO_PREFIX)/openchoreo-ui:$(OPENCHOREO_IMAGE_TAG)
THUNDER_IMAGE := ghcr.io/brionmario/thunder:0.0.16

# UI Image configuration
# Set USE_LOCAL_UI=true to use a locally-built image from backstage-plugins repo
# Default: false (pulls from registry)
# Usage: USE_LOCAL_UI=true make kind.load.ui
USE_LOCAL_UI ?= false
UI_IMAGE_TAG ?= $(OPENCHOREO_IMAGE_TAG)
UI_IMAGE_LOCAL ?= openchoreo-ui:local

# Define OpenChoreo components for per-component operations
# KIND_BUILD_COMPONENTS: Components that can be built locally in this repo (controller, api)
# KIND_COMPONENTS: All valid components including UI (which is built in backstage-plugins repo)
KIND_BUILD_COMPONENTS := controller api
KIND_COMPONENTS := controller api ui
KIND_COMPONENT_IMAGES := controller:$(CONTROLLER_IMAGE) api:$(API_IMAGE) ui:$(UI_IMAGE)

# Helper functions
get_component_image = $(patsubst $(1):%,%,$(filter $(1):%, $(KIND_COMPONENT_IMAGES)))

# Preconditions check functions
define check_cluster_exists
	@if ! kind get clusters | grep -q "^$(KIND_CLUSTER_NAME)$$"; then \
		$(call log_error, Kind cluster '$(KIND_CLUSTER_NAME)' does not exist. Run 'make kind.up' first); \
		exit 1; \
	fi
endef

##@ Local Testing

.PHONY: kind
kind: ## Build, load, and install all OpenChoreo components
	@$(call log_info, Starting complete OpenChoreo setup...)
	@$(MAKE) kind.up
	@$(MAKE) kind.build
	@$(MAKE) kind.load
	@$(MAKE) kind.install
	@$(call log_success, OpenChoreo setup completed successfully!)

.PHONY: kind.up
kind.up: ## Create the kind cluster without cni and kube-proxy
	@$(call log_info, Creating Kind cluster '$(KIND_CLUSTER_NAME)'...)
	@if [ "$$(kind get clusters 2>/dev/null | grep -q '^$(KIND_CLUSTER_NAME)$$' && echo true || echo false)" = "true" ]; then \
		$(call log_warning, Kind cluster '$(KIND_CLUSTER_NAME)' already exists); \
	else \
		$(KIND_SCRIPT) $(KIND_CLUSTER_NAME) $(KIND_NETWORK) $(KIND_EXTERNAL_DNS); \
		$(call log_success, Kind cluster created!); \
	fi


.PHONY: kind.build.%
kind.build.%: ## Build specific component (controller, api). Note: UI is built in backstage-plugins repo.
	@if [ -z "$(filter $*,$(KIND_BUILD_COMPONENTS))" ]; then \
		$(call log_error, Invalid component '$*'. Buildable components: $(KIND_BUILD_COMPONENTS). Note: UI is built in backstage-plugins repo.); \
		exit 1; \
	fi
	@$(call log_info, Building $* component...)
	@case "$*" in \
		controller) \
			$(MAKE) docker.build.controller TAG=$(OPENCHOREO_IMAGE_TAG); \
			;; \
		api) \
			$(MAKE) docker.build.openchoreo-api TAG=$(OPENCHOREO_IMAGE_TAG); \
			;; \
	esac
	@$(call log_success, $* component built!)

.PHONY: kind.build
kind.build: ## Build all locally-buildable OpenChoreo components
	@$(call log_info, Building all OpenChoreo components...)
	@$(foreach component,$(KIND_BUILD_COMPONENTS),$(MAKE) kind.build.$(component);)
	@$(call log_success, All components built!)


.PHONY: kind.load.%
kind.load.%: ## Load specific component image. Valid components: controller, api, ui, thunder, cilium. Usage: make kind.load.<component>
	@$(call check_cluster_exists)
	@case "$*" in \
		cilium) \
			$(call log_info, Loading Cilium images into cluster...); \
			for image in "quay.io/cilium/operator-generic:v$(CILIUM_VERSION)" "quay.io/cilium/cilium:v$(CILIUM_VERSION)" "quay.io/cilium/cilium-envoy:$(CILIUM_ENVOY_VERSION)"; do \
				if ! docker image inspect $$image > /dev/null 2>&1; then \
					$(call log_info, Pulling $$image...); \
					docker pull $$image; \
				fi; \
				$(call log_info, Loading $$image into Kind...); \
				kind load docker-image $$image --name "$(KIND_CLUSTER_NAME)"; \
			done; \
			$(call log_success, Cilium images loaded!); \
			;; \
		thunder) \
			$(call log_info, Loading Thunder image into cluster...); \
			if ! docker image inspect $(THUNDER_IMAGE) > /dev/null 2>&1; then \
				$(call log_info, Pulling $(THUNDER_IMAGE)...); \
				docker pull $(THUNDER_IMAGE); \
			fi; \
			kind load docker-image $(THUNDER_IMAGE) --name $(KIND_CLUSTER_NAME); \
			$(call log_success, Thunder image loaded!); \
			;; \
		controller|api) \
			if [ -z "$(filter $*,$(KIND_BUILD_COMPONENTS))" ]; then \
				$(call log_error, Invalid component '$*'. Available components: $(KIND_BUILD_COMPONENTS)); \
				exit 1; \
			fi; \
			IMAGE=$(call get_component_image,$*); \
			if ! docker image inspect $$IMAGE > /dev/null 2>&1; then \
				$(call log_error, Image '$$IMAGE' does not exist. Run 'make kind.build.$*' first); \
				exit 1; \
			fi; \
			$(call log_info, Loading $* image into cluster...); \
			kind load docker-image $$IMAGE --name $(KIND_CLUSTER_NAME); \
			$(call log_success, $* image loaded!); \
			;; \
		ui) \
			IMAGE=$(call get_component_image,ui); \
			if [ "$(USE_LOCAL_UI)" = "true" ]; then \
				if ! docker image inspect $(UI_IMAGE_LOCAL) > /dev/null 2>&1; then \
					$(call log_error, Local UI image '$(UI_IMAGE_LOCAL)' not found. Run 'yarn build-image' in backstage-plugins repo first.); \
					exit 1; \
				fi; \
				$(call log_info, Using local UI image '$(UI_IMAGE_LOCAL)' - tagging as $$IMAGE); \
				docker tag $(UI_IMAGE_LOCAL) $$IMAGE; \
			elif ! docker image inspect $$IMAGE > /dev/null 2>&1; then \
				$(call log_info, Pulling UI image from registry...); \
				docker pull $$IMAGE || { \
					$(call log_error, Failed to pull UI image '$$IMAGE'. Set USE_LOCAL_UI=true to use local image or check registry availability.); \
					exit 1; \
				}; \
			fi; \
			$(call log_info, Loading UI image into cluster...); \
			kind load docker-image $$IMAGE --name $(KIND_CLUSTER_NAME); \
			$(call log_success, UI image loaded!); \
			;; \
		*) \
			$(call log_error, Invalid component '$*'. Available components: $(KIND_COMPONENTS), thunder, cilium); \
			exit 1; \
			;; \
	esac

.PHONY: kind.load
kind.load: ## Load all OpenChoreo component images to kind cluster
	@$(call log_info, Loading all OpenChoreo component images...)
	@$(MAKE) kind.load.cilium
	@$(foreach component,$(KIND_COMPONENTS),$(MAKE) kind.load.$(component);)
	@$(call log_success, All component images loaded!)


.PHONY: kind.install.%
kind.install.%: ## Install specific component. Valid components: cilium, openchoreo. Usage: make kind.install.<component>
	@$(call check_cluster_exists)
	@case "$*" in \
		cilium) \
			$(call log_info, Installing Cilium CNI...); \
			helm repo add cilium https://helm.cilium.io/ || true; \
			helm repo update; \
			K8S_API_IP=$$(docker inspect "$(KIND_CLUSTER_NAME)-control-plane" --format='{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}'); \
			helm upgrade --install cilium cilium/cilium \
				--version "$(CILIUM_VERSION)" \
				--namespace cilium \
				--values "$(DEV_SCRIPTS_DIR)/cilium-values.yaml" \
				--set k8sServiceHost="$$K8S_API_IP" \
				--kube-context "kind-$(KIND_CLUSTER_NAME)" \
				--create-namespace \
				--wait \
				--timeout=10m; \
			$(call log_success, Cilium installed successfully!); \
			;; \
		openchoreo) \
			if ! helm list --namespace cilium --kube-context kind-$(KIND_CLUSTER_NAME) | grep -q "^cilium"; then \
				$(call log_error, Cilium is not installed. Run 'make kind.install.cilium' first); \
				exit 1; \
			fi; \
			$(call log_info, Installing OpenChoreo to cluster...); \
			helm upgrade --install openchoreo $(HELM_DIR) \
				--namespace $(OPENCHOREO_NAMESPACE) \
				--values $(DEV_SCRIPTS_DIR)/openchoreo-values.yaml \
				--set controllerManager.image.tag=$(OPENCHOREO_IMAGE_TAG) \
				--set openchoreoApi.image.tag=$(OPENCHOREO_IMAGE_TAG) \
				--set backstage.image.tag=$(OPENCHOREO_IMAGE_TAG) \
				--create-namespace \
				--wait \
				--timeout=10m; \
			$(call log_success, OpenChoreo installed successfully!); \
			;; \
		*) \
			$(call log_error, Invalid component '$*'. Available components: cilium, openchoreo); \
			exit 1; \
			;; \
	esac

.PHONY: kind.install
kind.install: ## Install Cilium CNI and OpenChoreo to kind cluster
	@$(call log_info, Installing all components to cluster...)
	@$(MAKE) kind.install.cilium
	@$(MAKE) kind.install.openchoreo
	@$(call log_success, All components installed successfully!)


.PHONY: kind.uninstall.%
kind.uninstall.%: ## Uninstall specific component. Valid components: cilium, openchoreo. Usage: make kind.uninstall.<component>
	@case "$*" in \
		openchoreo) \
			$(call log_info, Uninstalling OpenChoreo...); \
			helm uninstall openchoreo --namespace $(OPENCHOREO_NAMESPACE) || true; \
			$(call log_success, OpenChoreo uninstalled!); \
			;; \
		cilium) \
			$(call log_info, Uninstalling Cilium...); \
			helm uninstall cilium --namespace cilium || true; \
			$(call log_success, Cilium uninstalled!); \
			;; \
		*) \
			$(call log_error, Invalid component '$*'. Available components: cilium, openchoreo); \
			exit 1; \
			;; \
	esac

.PHONY: kind.uninstall
kind.uninstall: ## Uninstall OpenChoreo and Cilium from kind cluster
	@$(call log_info, Uninstalling all components from cluster...)
	@$(MAKE) kind.uninstall.openchoreo
	@$(MAKE) kind.uninstall.cilium
	@$(call log_success, All components uninstalled!)


.PHONY: kind.update.%
kind.update.%: ## Update specific component (controller, api, ui). UI skips build step.
	@if [ -z "$(filter $*,$(KIND_COMPONENTS))" ]; then \
		$(call log_error, Invalid component '$*'. Available components: $(KIND_COMPONENTS)); \
		exit 1; \
	fi
	@$(call log_info, Updating $* component...)
	@if [ "$*" != "ui" ]; then \
		$(MAKE) kind.build.$*; \
	fi
	@$(MAKE) kind.load.$*
	@$(call log_info, Performing rollout restart for $*...)
	@case "$*" in \
		controller) \
			kubectl rollout restart deployment/openchoreo-controller-manager -n $(OPENCHOREO_NAMESPACE) || true; \
			kubectl rollout status deployment/openchoreo-controller-manager -n $(OPENCHOREO_NAMESPACE) --timeout=300s || true; \
			;; \
		api) \
			kubectl rollout restart deployment/openchoreo-api -n $(OPENCHOREO_NAMESPACE) || true; \
			kubectl rollout status deployment/openchoreo-api -n $(OPENCHOREO_NAMESPACE) --timeout=300s || true; \
			;; \
		ui) \
			kubectl rollout restart deployment/openchoreo-ui -n $(OPENCHOREO_NAMESPACE) || true; \
			kubectl rollout status deployment/openchoreo-ui -n $(OPENCHOREO_NAMESPACE) --timeout=300s || true; \
			;; \
	esac
	@$(call log_success, $* component updated!)

.PHONY: kind.update
kind.update: ## Rebuild and reload all OpenChoreo components
	@$(call log_info, Starting OpenChoreo rebuild and reload process...)
	@$(MAKE) kind.build
	@$(MAKE) kind.load
	@$(call log_info, Performing rollout restart for all components...)
	@kubectl rollout restart deployment/openchoreo-controller-manager -n $(OPENCHOREO_NAMESPACE) || true
	@kubectl rollout restart deployment/openchoreo-api -n $(OPENCHOREO_NAMESPACE) || true
	@kubectl rollout restart deployment/openchoreo-ui -n $(OPENCHOREO_NAMESPACE) || true
	@$(call log_info, Waiting for rollout restarts to complete...)
	@kubectl rollout status deployment/openchoreo-controller-manager -n $(OPENCHOREO_NAMESPACE) --timeout=300s || true
	@kubectl rollout status deployment/openchoreo-api -n $(OPENCHOREO_NAMESPACE) --timeout=300s || true
	@kubectl rollout status deployment/openchoreo-ui -n $(OPENCHOREO_NAMESPACE) --timeout=300s || true
	@$(call log_success, OpenChoreo update completed successfully!)


.PHONY: kind.down
kind.down: ## Delete Kind cluster
	@$(call log_info, Deleting Kind cluster '$(KIND_CLUSTER_NAME)'...)
	@if kind get clusters | grep -q "^$(KIND_CLUSTER_NAME)$$"; then \
		kind delete cluster --name "$(KIND_CLUSTER_NAME)"; \
		$(call log_success, Kind cluster deleted!); \
	else \
		$(call log_info, Kind cluster '$(KIND_CLUSTER_NAME)' does not exist); \
	fi
