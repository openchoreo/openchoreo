# K3d-based development workflow for OpenChoreo
# Uses k3d image import for loading locally-built images

# Configuration
K3D_CLUSTER_NAME ?= openchoreo-dev
K3D_DEV_DIR := $(PROJECT_DIR)/install/k3d/dev
K3D_HELM_DIR := $(PROJECT_DIR)/install/helm
OPENCHOREO_IMAGE_TAG := latest-dev

# Namespaces for each plane
K3D_CP_NAMESPACE := openchoreo-control-plane
K3D_DP_NAMESPACE := openchoreo-data-plane
K3D_BP_NAMESPACE := openchoreo-build-plane
K3D_OP_NAMESPACE := openchoreo-observability-plane

# Components that can be built locally
K3D_BUILD_COMPONENTS := controller openchoreo-api observer

# Helper functions
define k3d_check_cluster
	@if ! k3d cluster list | grep -q "^$(K3D_CLUSTER_NAME)"; then \
		$(call log_error, K3d cluster '$(K3D_CLUSTER_NAME)' does not exist. Run 'make k3d.up' first); \
		exit 1; \
	fi
endef

##@ K3d Development

.PHONY: k3d
k3d: ## Complete setup: create cluster, preload dependencies, build, load, and install all planes
	@$(call log_info, Starting complete OpenChoreo k3d setup...)
	@$(MAKE) k3d.up
	@$(MAKE) k3d.preload
	@$(MAKE) k3d.build
	@$(MAKE) k3d.load
	@$(MAKE) k3d.install
	@$(call log_success, OpenChoreo k3d setup completed!)

# Image Preloading (uses Docker cache for fast cluster recreation)
.PHONY: k3d.preload
k3d.preload: ## Preload all dependency images into k3d cluster (uses Docker cache)
	$(call k3d_check_cluster)
	@$(call log_info, Preloading dependency images into k3d cluster...)
	@$(PROJECT_DIR)/install/k3d/preload-images.sh \
		--cluster $(K3D_CLUSTER_NAME) \
		--local-charts \
		--control-plane --cp-values $(K3D_DEV_DIR)/values-cp.yaml \
		--data-plane --dp-values $(K3D_DEV_DIR)/values-dp.yaml \
		--build-plane --bp-values $(K3D_DEV_DIR)/values-bp.yaml \
		--observability-plane --op-values $(K3D_DEV_DIR)/values-op.yaml \
		--parallel 6
	@$(call log_success, Dependency images preloaded!)

.PHONY: k3d.preload.control-plane
k3d.preload.control-plane: ## Preload Control Plane images only
	$(call k3d_check_cluster)
	@$(call log_info, Preloading Control Plane images...)
	@$(PROJECT_DIR)/install/k3d/preload-images.sh \
		--cluster $(K3D_CLUSTER_NAME) \
		--local-charts \
		--control-plane --cp-values $(K3D_DEV_DIR)/values-cp.yaml \
		--parallel 6
	@$(call log_success, Control Plane images preloaded!)

.PHONY: k3d.preload.data-plane
k3d.preload.data-plane: ## Preload Data Plane images only
	$(call k3d_check_cluster)
	@$(call log_info, Preloading Data Plane images...)
	@$(PROJECT_DIR)/install/k3d/preload-images.sh \
		--cluster $(K3D_CLUSTER_NAME) \
		--local-charts \
		--data-plane --dp-values $(K3D_DEV_DIR)/values-dp.yaml \
		--parallel 6
	@$(call log_success, Data Plane images preloaded!)

.PHONY: k3d.preload.build-plane
k3d.preload.build-plane: ## Preload Build Plane images only
	$(call k3d_check_cluster)
	@$(call log_info, Preloading Build Plane images...)
	@$(PROJECT_DIR)/install/k3d/preload-images.sh \
		--cluster $(K3D_CLUSTER_NAME) \
		--local-charts \
		--build-plane --bp-values $(K3D_DEV_DIR)/values-bp.yaml \
		--parallel 6
	@$(call log_success, Build Plane images preloaded!)

.PHONY: k3d.preload.observability-plane
k3d.preload.observability-plane: ## Preload Observability Plane images only
	$(call k3d_check_cluster)
	@$(call log_info, Preloading Observability Plane images...)
	@$(PROJECT_DIR)/install/k3d/preload-images.sh \
		--cluster $(K3D_CLUSTER_NAME) \
		--local-charts \
		--observability-plane --op-values $(K3D_DEV_DIR)/values-op.yaml \
		--parallel 6
	@$(call log_success, Observability Plane images preloaded!)

# Cluster Management
.PHONY: k3d.up
k3d.up: ## Create k3d cluster (1 server + 2 agents)
	@$(call log_info, Creating k3d cluster '$(K3D_CLUSTER_NAME)'...)
	@if k3d cluster list | grep -q "^$(K3D_CLUSTER_NAME)"; then \
		$(call log_warning, K3d cluster '$(K3D_CLUSTER_NAME)' already exists); \
	else \
		k3d cluster create --config $(K3D_DEV_DIR)/config.yaml; \
		$(call log_success, K3d cluster created!); \
	fi

.PHONY: k3d.down
k3d.down: ## Delete k3d cluster
	@$(call log_info, Deleting k3d cluster '$(K3D_CLUSTER_NAME)'...)
	@if k3d cluster list | grep -q "^$(K3D_CLUSTER_NAME)"; then \
		k3d cluster delete $(K3D_CLUSTER_NAME); \
		$(call log_success, K3d cluster deleted!); \
	else \
		$(call log_info, K3d cluster '$(K3D_CLUSTER_NAME)' does not exist); \
	fi

# Build Targets
.PHONY: k3d.build
k3d.build: ## Build all OpenChoreo components with latest-dev tag
	@$(call log_info, Building all OpenChoreo components...)
	@$(MAKE) docker.build.controller TAG=$(OPENCHOREO_IMAGE_TAG)
	@$(MAKE) docker.build.openchoreo-api TAG=$(OPENCHOREO_IMAGE_TAG)
	@$(MAKE) docker.build.observer TAG=$(OPENCHOREO_IMAGE_TAG)
	@$(call log_success, All components built!)

.PHONY: k3d.build.controller
k3d.build.controller: ## Build controller image
	@$(MAKE) docker.build.controller TAG=$(OPENCHOREO_IMAGE_TAG)

.PHONY: k3d.build.openchoreo-api
k3d.build.openchoreo-api: ## Build openchoreo-api image
	@$(MAKE) docker.build.openchoreo-api TAG=$(OPENCHOREO_IMAGE_TAG)

.PHONY: k3d.build.observer
k3d.build.observer: ## Build observer image
	@$(MAKE) docker.build.observer TAG=$(OPENCHOREO_IMAGE_TAG)

# Image Loading
.PHONY: k3d.load
k3d.load: ## Import all images into k3d cluster (bulk load for speed)
	$(call k3d_check_cluster)
	@$(call log_info, Loading all OpenChoreo images into k3d cluster...)
	@k3d image import \
		$(IMAGE_REPO_PREFIX)/controller:$(OPENCHOREO_IMAGE_TAG) \
		$(IMAGE_REPO_PREFIX)/openchoreo-api:$(OPENCHOREO_IMAGE_TAG) \
		$(IMAGE_REPO_PREFIX)/observer:$(OPENCHOREO_IMAGE_TAG) \
		--cluster $(K3D_CLUSTER_NAME)
	@$(call log_success, All images loaded!)

.PHONY: k3d.load.controller
k3d.load.controller: ## Import controller image into k3d
	$(call k3d_check_cluster)
	@$(call log_info, Loading controller image...)
	@k3d image import $(IMAGE_REPO_PREFIX)/controller:$(OPENCHOREO_IMAGE_TAG) --cluster $(K3D_CLUSTER_NAME)
	@$(call log_success, Controller image loaded!)

.PHONY: k3d.load.openchoreo-api
k3d.load.openchoreo-api: ## Import openchoreo-api image into k3d
	$(call k3d_check_cluster)
	@$(call log_info, Loading openchoreo-api image...)
	@k3d image import $(IMAGE_REPO_PREFIX)/openchoreo-api:$(OPENCHOREO_IMAGE_TAG) --cluster $(K3D_CLUSTER_NAME)
	@$(call log_success, openchoreo-api image loaded!)

.PHONY: k3d.load.observer
k3d.load.observer: ## Import observer image into k3d
	$(call k3d_check_cluster)
	@$(call log_info, Loading observer image...)
	@k3d image import $(IMAGE_REPO_PREFIX)/observer:$(OPENCHOREO_IMAGE_TAG) --cluster $(K3D_CLUSTER_NAME)
	@$(call log_success, Observer image loaded!)

# Installation Targets
.PHONY: k3d.install
k3d.install: ## Install all planes (CP, DP, BP, OP)
	$(call k3d_check_cluster)
	@$(call log_info, Installing all planes...)
	@$(MAKE) k3d.install.control-plane
	@$(MAKE) k3d.install.data-plane
	@$(MAKE) k3d.install.build-plane
	@$(MAKE) k3d.install.observability-plane
	@$(call log_success, All planes installed!)

.PHONY: k3d.install.control-plane
k3d.install.control-plane: ## Install Control Plane
	$(call k3d_check_cluster)
	@$(call log_info, Installing Control Plane...)
	@helm upgrade --install openchoreo-control-plane $(K3D_HELM_DIR)/openchoreo-control-plane \
		--namespace $(K3D_CP_NAMESPACE) \
		--values $(K3D_DEV_DIR)/values-cp.yaml \
		--kube-context k3d-$(K3D_CLUSTER_NAME) \
		--create-namespace \
		--timeout=10m
	@$(call log_success, Control Plane installed!)

.PHONY: k3d.install.data-plane
k3d.install.data-plane: ## Install Data Plane
	$(call k3d_check_cluster)
	@$(call log_info, Installing Data Plane...)
	@helm upgrade --install openchoreo-data-plane $(K3D_HELM_DIR)/openchoreo-data-plane \
		--namespace $(K3D_DP_NAMESPACE) \
		--values $(K3D_DEV_DIR)/values-dp.yaml \
		--kube-context k3d-$(K3D_CLUSTER_NAME) \
		--create-namespace \
		--timeout=10m
	@$(call log_success, Data Plane installed!)
	@$(call log_info, Setting up default DataPlane resource...)
	@if ! kubectl get dataplane default -n default --context k3d-$(K3D_CLUSTER_NAME) &>/dev/null; then \
		OLD_CONTEXT=$$(kubectl config current-context 2>/dev/null || echo ""); \
		kubectl config use-context k3d-$(K3D_CLUSTER_NAME) >/dev/null 2>&1 || true; \
		$(PROJECT_DIR)/install/add-default-dataplane.sh 2>&1 || true; \
		if [ -n "$$OLD_CONTEXT" ]; then \
			kubectl config use-context $$OLD_CONTEXT >/dev/null 2>&1 || true; \
		fi; \
		if kubectl get dataplane default -n default --context k3d-$(K3D_CLUSTER_NAME) &>/dev/null; then \
			$(call log_success, Default DataPlane resource created!); \
		else \
			$(call log_warning, Failed to create DataPlane resource. You can create it manually later.); \
		fi; \
	else \
		$(call log_info, Default DataPlane resource already exists); \
	fi
	@echo ""
	@$(call log_info, To test the deployment, deploy the React starter app:)
	@echo "  kubectl apply -f $(PROJECT_DIR)/samples/from-image/react-starter-web-app/react-starter.yaml --context k3d-$(K3D_CLUSTER_NAME)"
	@echo ""
	@$(call log_info, Once deployed, access it at:)
	@echo "  http://react-starter-development-default.openchoreoapis.localhost:19080"
	@echo ""

.PHONY: k3d.install.build-plane
k3d.install.build-plane: ## Install Build Plane
	$(call k3d_check_cluster)
	@$(call log_info, Installing Build Plane...)
	@helm upgrade --install openchoreo-build-plane $(K3D_HELM_DIR)/openchoreo-build-plane \
		--namespace $(K3D_BP_NAMESPACE) \
		--values $(K3D_DEV_DIR)/values-bp.yaml \
		--kube-context k3d-$(K3D_CLUSTER_NAME) \
		--create-namespace \
		--timeout=10m
	@$(call log_success, Build Plane installed!)
	@$(call log_info, Setting up default BuildPlane resource...)
	@if ! kubectl get buildplane default -n default --context k3d-$(K3D_CLUSTER_NAME) &>/dev/null; then \
		$(PROJECT_DIR)/install/add-build-plane.sh --control-plane-context k3d-$(K3D_CLUSTER_NAME) 2>&1 || true; \
		if kubectl get buildplane default -n default --context k3d-$(K3D_CLUSTER_NAME) &>/dev/null; then \
			$(call log_success, Default BuildPlane resource created!); \
		else \
			$(call log_warning, Failed to create BuildPlane resource. You can create it manually later.); \
		fi; \
	else \
		$(call log_info, Default BuildPlane resource already exists); \
	fi

.PHONY: k3d.install.observability-plane
k3d.install.observability-plane: ## Install Observability Plane
	$(call k3d_check_cluster)
	@$(call log_info, Installing Observability Plane...)
	@helm upgrade --install openchoreo-observability-plane $(K3D_HELM_DIR)/openchoreo-observability-plane \
		--namespace $(K3D_OP_NAMESPACE) \
		--values $(K3D_DEV_DIR)/values-op.yaml \
		--kube-context k3d-$(K3D_CLUSTER_NAME) \
		--create-namespace \
		--timeout=10m
	@$(call log_success, Observability Plane installed!)

# Uninstall Targets
.PHONY: k3d.uninstall
k3d.uninstall: ## Uninstall all planes
	@$(call log_info, Uninstalling all planes...)
	@$(MAKE) k3d.uninstall.observability-plane
	@$(MAKE) k3d.uninstall.build-plane
	@$(MAKE) k3d.uninstall.data-plane
	@$(MAKE) k3d.uninstall.control-plane
	@$(call log_success, All planes uninstalled!)

.PHONY: k3d.uninstall.control-plane
k3d.uninstall.control-plane: ## Uninstall Control Plane
	@$(call log_info, Uninstalling Control Plane...)
	@helm uninstall openchoreo-control-plane --namespace $(K3D_CP_NAMESPACE) --kube-context k3d-$(K3D_CLUSTER_NAME) || true
	@$(call log_success, Control Plane uninstalled!)

.PHONY: k3d.uninstall.data-plane
k3d.uninstall.data-plane: ## Uninstall Data Plane
	@$(call log_info, Uninstalling Data Plane...)
	@helm uninstall openchoreo-data-plane --namespace $(K3D_DP_NAMESPACE) --kube-context k3d-$(K3D_CLUSTER_NAME) || true
	@$(call log_success, Data Plane uninstalled!)

.PHONY: k3d.uninstall.build-plane
k3d.uninstall.build-plane: ## Uninstall Build Plane
	@$(call log_info, Uninstalling Build Plane...)
	@helm uninstall openchoreo-build-plane --namespace $(K3D_BP_NAMESPACE) --kube-context k3d-$(K3D_CLUSTER_NAME) || true
	@$(call log_success, Build Plane uninstalled!)

.PHONY: k3d.uninstall.observability-plane
k3d.uninstall.observability-plane: ## Uninstall Observability Plane
	@$(call log_info, Uninstalling Observability Plane...)
	@helm uninstall openchoreo-observability-plane --namespace $(K3D_OP_NAMESPACE) --kube-context k3d-$(K3D_CLUSTER_NAME) || true
	@$(call log_success, Observability Plane uninstalled!)

# Update Targets (Component Updates - rebuild, load, restart)
.PHONY: k3d.update
k3d.update: ## Rebuild, load, and restart all components
	@$(call log_info, Updating all components...)
	@$(MAKE) k3d.build
	@$(MAKE) k3d.load
	@$(call log_info, Performing rollout restarts...)
	@kubectl rollout restart deployment/controller-manager -n $(K3D_CP_NAMESPACE) --context k3d-$(K3D_CLUSTER_NAME) || true
	@kubectl rollout restart deployment/openchoreo-api -n $(K3D_CP_NAMESPACE) --context k3d-$(K3D_CLUSTER_NAME) || true
	@kubectl rollout restart deployment/observer -n $(K3D_OP_NAMESPACE) --context k3d-$(K3D_CLUSTER_NAME) || true
	@$(call log_info, Waiting for rollouts to complete...)
	@kubectl rollout status deployment/controller-manager -n $(K3D_CP_NAMESPACE) --context k3d-$(K3D_CLUSTER_NAME) --timeout=300s || true
	@kubectl rollout status deployment/openchoreo-api -n $(K3D_CP_NAMESPACE) --context k3d-$(K3D_CLUSTER_NAME) --timeout=300s || true
	@kubectl rollout status deployment/observer -n $(K3D_OP_NAMESPACE) --context k3d-$(K3D_CLUSTER_NAME) --timeout=300s || true
	@$(call log_success, All components updated!)

.PHONY: k3d.update.controller
k3d.update.controller: ## Update controller: build, load, restart
	@$(call log_info, Updating controller...)
	@$(MAKE) k3d.build.controller
	@$(MAKE) k3d.load.controller
	@kubectl rollout restart deployment/controller-manager -n $(K3D_CP_NAMESPACE) --context k3d-$(K3D_CLUSTER_NAME)
	@kubectl rollout status deployment/controller-manager -n $(K3D_CP_NAMESPACE) --context k3d-$(K3D_CLUSTER_NAME) --timeout=300s
	@$(call log_success, Controller updated!)

.PHONY: k3d.update.openchoreo-api
k3d.update.openchoreo-api: ## Update openchoreo-api: build, load, restart
	@$(call log_info, Updating openchoreo-api...)
	@$(MAKE) k3d.build.openchoreo-api
	@$(MAKE) k3d.load.openchoreo-api
	@kubectl rollout restart deployment/openchoreo-api -n $(K3D_CP_NAMESPACE) --context k3d-$(K3D_CLUSTER_NAME)
	@kubectl rollout status deployment/openchoreo-api -n $(K3D_CP_NAMESPACE) --context k3d-$(K3D_CLUSTER_NAME) --timeout=300s
	@$(call log_success, openchoreo-api updated!)

.PHONY: k3d.update.observer
k3d.update.observer: ## Update observer: build, load, restart
	@$(call log_info, Updating observer...)
	@$(MAKE) k3d.build.observer
	@$(MAKE) k3d.load.observer
	@kubectl rollout restart deployment/observer -n $(K3D_OP_NAMESPACE) --context k3d-$(K3D_CLUSTER_NAME)
	@kubectl rollout status deployment/observer -n $(K3D_OP_NAMESPACE) --context k3d-$(K3D_CLUSTER_NAME) --timeout=300s
	@$(call log_success, Observer updated!)

# Upgrade Targets (Helm Chart Updates)
.PHONY: k3d.upgrade
k3d.upgrade: ## Upgrade all planes with current helm charts
	@$(call log_info, Upgrading all planes...)
	@$(MAKE) k3d.upgrade.control-plane
	@$(MAKE) k3d.upgrade.data-plane
	@$(MAKE) k3d.upgrade.build-plane
	@$(MAKE) k3d.upgrade.observability-plane
	@$(call log_success, All planes upgraded!)

.PHONY: k3d.upgrade.control-plane
k3d.upgrade.control-plane: ## Upgrade Control Plane helm chart
	@$(MAKE) k3d.install.control-plane

.PHONY: k3d.upgrade.data-plane
k3d.upgrade.data-plane: ## Upgrade Data Plane helm chart
	@$(MAKE) k3d.install.data-plane

.PHONY: k3d.upgrade.build-plane
k3d.upgrade.build-plane: ## Upgrade Build Plane helm chart
	@$(MAKE) k3d.install.build-plane

.PHONY: k3d.upgrade.observability-plane
k3d.upgrade.observability-plane: ## Upgrade Observability Plane helm chart
	@$(MAKE) k3d.install.observability-plane

# Post-Install Configuration
.PHONY: k3d.configure
k3d.configure: ## Create DataPlane and BuildPlane resources (placeholder)
	$(call k3d_check_cluster)
	@$(call log_info, Configure step placeholder - create DataPlane/BuildPlane resources as needed)
	@$(call log_info, Example: kubectl apply -f your-dataplane.yaml --context k3d-$(K3D_CLUSTER_NAME))

# Helper Targets
.PHONY: k3d.status
k3d.status: ## Check status of all planes
	@$(call log_info, Checking k3d cluster status...)
	@echo ""
	@echo "=== Cluster Info ==="
	@k3d cluster list | grep -E "^NAME|$(K3D_CLUSTER_NAME)" || echo "Cluster not found"
	@echo ""
	@echo "=== Control Plane ==="
	@kubectl get pods -n $(K3D_CP_NAMESPACE) --context k3d-$(K3D_CLUSTER_NAME) 2>/dev/null || echo "Not installed"
	@echo ""
	@echo "=== Data Plane ==="
	@kubectl get pods -n $(K3D_DP_NAMESPACE) --context k3d-$(K3D_CLUSTER_NAME) 2>/dev/null || echo "Not installed"
	@echo ""
	@echo "=== Build Plane ==="
	@kubectl get pods -n $(K3D_BP_NAMESPACE) --context k3d-$(K3D_CLUSTER_NAME) 2>/dev/null || echo "Not installed"
	@echo ""
	@echo "=== Observability Plane ==="
	@kubectl get pods -n $(K3D_OP_NAMESPACE) --context k3d-$(K3D_CLUSTER_NAME) 2>/dev/null || echo "Not installed"

.PHONY: k3d.logs.controller
k3d.logs.controller: ## Tail controller logs
	@kubectl logs -f deployment/controller-manager -n $(K3D_CP_NAMESPACE) --context k3d-$(K3D_CLUSTER_NAME)

.PHONY: k3d.logs.openchoreo-api
k3d.logs.openchoreo-api: ## Tail openchoreo-api logs
	@kubectl logs -f deployment/openchoreo-api -n $(K3D_CP_NAMESPACE) --context k3d-$(K3D_CLUSTER_NAME)

.PHONY: k3d.logs.observer
k3d.logs.observer: ## Tail observer logs
	@kubectl logs -f deployment/observer -n $(K3D_OP_NAMESPACE) --context k3d-$(K3D_CLUSTER_NAME)
