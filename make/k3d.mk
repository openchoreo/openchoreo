# K3d-based development workflow for OpenChoreo
# Uses k3d image import for loading locally-built images

# Configuration
K3D_CLUSTER_NAME ?= openchoreo
OPENCHOREO_IMAGE_TAG := latest-dev

# Namespaces for each plane
K3D_CP_NAMESPACE := openchoreo-control-plane
K3D_DP_NAMESPACE := openchoreo-data-plane
K3D_WP_NAMESPACE := openchoreo-workflow-plane
K3D_OP_NAMESPACE := openchoreo-observability-plane

# Components that can be built locally
K3D_BUILD_COMPONENTS := controller openchoreo-api observer cluster-gateway cluster-agent

# Helper functions
define k3d_check_cluster
	@if ! k3d cluster list | grep -q "^$(K3D_CLUSTER_NAME)"; then \
		$(call log_error, K3d cluster '$(K3D_CLUSTER_NAME)' does not exist); \
		exit 1; \
	fi
endef

##@ K3d Development

# Build Targets
.PHONY: k3d.build
k3d.build: ## Build all OpenChoreo components with latest-dev tag
	@$(call log_info, Building all OpenChoreo components...)
	@$(MAKE) docker.build.controller TAG=$(OPENCHOREO_IMAGE_TAG)
	@$(MAKE) docker.build.openchoreo-api TAG=$(OPENCHOREO_IMAGE_TAG)
	@$(MAKE) docker.build.observer TAG=$(OPENCHOREO_IMAGE_TAG)
	@$(MAKE) docker.build.cluster-gateway TAG=$(OPENCHOREO_IMAGE_TAG)
	@$(MAKE) docker.build.cluster-agent TAG=$(OPENCHOREO_IMAGE_TAG)
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

.PHONY: k3d.build.cluster-gateway
k3d.build.cluster-gateway: ## Build cluster-gateway image
	@$(MAKE) docker.build.cluster-gateway TAG=$(OPENCHOREO_IMAGE_TAG)

.PHONY: k3d.build.cluster-agent
k3d.build.cluster-agent: ## Build cluster-agent image
	@$(MAKE) docker.build.cluster-agent TAG=$(OPENCHOREO_IMAGE_TAG)

# Image Loading
.PHONY: k3d.load
k3d.load: ## Import all images into k3d cluster (bulk load for speed)
	$(call k3d_check_cluster)
	@$(call log_info, Loading all OpenChoreo images into k3d cluster...)
	@k3d image import \
		$(IMAGE_REPO_PREFIX)/controller:$(OPENCHOREO_IMAGE_TAG) \
		$(IMAGE_REPO_PREFIX)/openchoreo-api:$(OPENCHOREO_IMAGE_TAG) \
		$(IMAGE_REPO_PREFIX)/observer:$(OPENCHOREO_IMAGE_TAG) \
		$(IMAGE_REPO_PREFIX)/cluster-gateway:$(OPENCHOREO_IMAGE_TAG) \
		$(IMAGE_REPO_PREFIX)/cluster-agent:$(OPENCHOREO_IMAGE_TAG) \
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

.PHONY: k3d.load.cluster-gateway
k3d.load.cluster-gateway: ## Import cluster-gateway image into k3d
	$(call k3d_check_cluster)
	@$(call log_info, Loading cluster-gateway image...)
	@k3d image import $(IMAGE_REPO_PREFIX)/cluster-gateway:$(OPENCHOREO_IMAGE_TAG) --cluster $(K3D_CLUSTER_NAME)
	@$(call log_success, Cluster-gateway image loaded!)

.PHONY: k3d.load.cluster-agent
k3d.load.cluster-agent: ## Import cluster-agent image into k3d
	$(call k3d_check_cluster)
	@$(call log_info, Loading cluster-agent image...)
	@k3d image import $(IMAGE_REPO_PREFIX)/cluster-agent:$(OPENCHOREO_IMAGE_TAG) --cluster $(K3D_CLUSTER_NAME)
	@$(call log_success, Cluster-agent image loaded!)

# Uninstall Targets
.PHONY: k3d.uninstall
k3d.uninstall: ## Uninstall all planes
	@$(call log_info, Uninstalling all planes...)
	@$(MAKE) k3d.uninstall.observability-plane
	@$(MAKE) k3d.uninstall.workflow-plane
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

.PHONY: k3d.uninstall.workflow-plane
k3d.uninstall.workflow-plane: ## Uninstall Workflow Plane
	@$(call log_info, Uninstalling Workflow Plane...)
	@helm uninstall openchoreo-workflow-plane --namespace $(K3D_WP_NAMESPACE) --kube-context k3d-$(K3D_CLUSTER_NAME) || true
	@$(call log_success, Workflow Plane uninstalled!)

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
	@kubectl rollout restart deployment/cluster-gateway -n $(K3D_CP_NAMESPACE) --context k3d-$(K3D_CLUSTER_NAME) || true
	@kubectl rollout restart deployment/observer -n $(K3D_OP_NAMESPACE) --context k3d-$(K3D_CLUSTER_NAME) || true
	@for ns in $(K3D_DP_NAMESPACE) $(K3D_WP_NAMESPACE) $(K3D_OP_NAMESPACE); do \
		dep=$$(kubectl get deployment -n $$ns --context k3d-$(K3D_CLUSTER_NAME) -l app=cluster-agent -o name 2>/dev/null); \
		if [ -n "$$dep" ]; then kubectl rollout restart $$dep -n $$ns --context k3d-$(K3D_CLUSTER_NAME) || true; fi; \
	done
	@$(call log_info, Waiting for rollouts to complete...)
	@kubectl rollout status deployment/controller-manager -n $(K3D_CP_NAMESPACE) --context k3d-$(K3D_CLUSTER_NAME) --timeout=300s || true
	@kubectl rollout status deployment/openchoreo-api -n $(K3D_CP_NAMESPACE) --context k3d-$(K3D_CLUSTER_NAME) --timeout=300s || true
	@kubectl rollout status deployment/cluster-gateway -n $(K3D_CP_NAMESPACE) --context k3d-$(K3D_CLUSTER_NAME) --timeout=300s || true
	@kubectl rollout status deployment/observer -n $(K3D_OP_NAMESPACE) --context k3d-$(K3D_CLUSTER_NAME) --timeout=300s || true
	@for ns in $(K3D_DP_NAMESPACE) $(K3D_WP_NAMESPACE) $(K3D_OP_NAMESPACE); do \
		dep=$$(kubectl get deployment -n $$ns --context k3d-$(K3D_CLUSTER_NAME) -l app=cluster-agent -o name 2>/dev/null); \
		if [ -n "$$dep" ]; then kubectl rollout status $$dep -n $$ns --context k3d-$(K3D_CLUSTER_NAME) --timeout=300s || true; fi; \
	done
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

.PHONY: k3d.update.cluster-gateway
k3d.update.cluster-gateway: ## Update cluster-gateway: build, load, restart
	@$(call log_info, Updating cluster-gateway...)
	@$(MAKE) k3d.build.cluster-gateway
	@$(MAKE) k3d.load.cluster-gateway
	@kubectl rollout restart deployment/cluster-gateway -n $(K3D_CP_NAMESPACE) --context k3d-$(K3D_CLUSTER_NAME)
	@kubectl rollout status deployment/cluster-gateway -n $(K3D_CP_NAMESPACE) --context k3d-$(K3D_CLUSTER_NAME) --timeout=300s
	@$(call log_success, Cluster-gateway updated!)

.PHONY: k3d.update.cluster-agent
k3d.update.cluster-agent: ## Update cluster-agent: build, load, restart across all planes
	@$(call log_info, Updating cluster-agent across all planes...)
	@$(MAKE) k3d.build.cluster-agent
	@$(MAKE) k3d.load.cluster-agent
	@for ns in $(K3D_DP_NAMESPACE) $(K3D_WP_NAMESPACE) $(K3D_OP_NAMESPACE); do \
		dep=$$(kubectl get deployment -n $$ns --context k3d-$(K3D_CLUSTER_NAME) -l app=cluster-agent -o name 2>/dev/null); \
		if [ -n "$$dep" ]; then \
			kubectl rollout restart $$dep -n $$ns --context k3d-$(K3D_CLUSTER_NAME); \
			kubectl rollout status $$dep -n $$ns --context k3d-$(K3D_CLUSTER_NAME) --timeout=300s; \
		fi; \
	done
	@$(call log_success, Cluster-agent updated across all planes!)

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
	@echo "=== Workflow Plane ==="
	@kubectl get pods -n $(K3D_WP_NAMESPACE) --context k3d-$(K3D_CLUSTER_NAME) 2>/dev/null || echo "Not installed"
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
