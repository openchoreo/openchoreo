# This makefile contains all the make targets related to Kind cluster management for OpenChoreo development.

# Kind cluster configuration
KIND_CLUSTER_NAME ?= openchoreo
OPENCHOREO_IMAGE_TAG ?= dev
OPENCHOREO_NAMESPACE ?= openchoreo
KIND_EXTERNAL_DNS ?= 8.8.8.8
KIND_NETWORK ?= openchoreo
# Cilium configuration
CILIUM_VERSION := 1.18.2
CILIUM_ENVOY_VERSION := v1.34.4-1754895458-68cffdfa568b6b226d70a7ef81fc65dda3b890bf

# Paths to development scripts and Helm chart
DEV_SCRIPTS_DIR := $(PROJECT_DIR)/install/dev
HELM_DIR := $(PROJECT_DIR)/install/helm/openchoreo-secure-core
KIND_SCRIPT := $(DEV_SCRIPTS_DIR)/kind.sh


# Image names
CONTROLLER_IMAGE := openchoreo-controller:$(OPENCHOREO_IMAGE_TAG)
API_IMAGE := openchoreo-api:$(OPENCHOREO_IMAGE_TAG)
UI_IMAGE := openchoreo-ui:$(OPENCHOREO_IMAGE_TAG)

##@ Kind Cluster Management

# Check if Kind cluster exists
.PHONY: kind.exists
kind.exists: ## Check if Kind cluster exists
	@if kind get clusters 2>/dev/null | grep -q "^$(KIND_CLUSTER_NAME)$$"; then \
		echo "true"; \
	else \
		echo "false"; \
	fi

# Create Kind cluster
.PHONY: kind
kind: ## Create a Kind cluster for OpenChoreo development
	@$(call log_info, Creating Kind cluster '$(KIND_CLUSTER_NAME)'...)
	@if [ "$$(make kind.exists)" = "true" ]; then \
		$(call log_warning, Kind cluster '$(KIND_CLUSTER_NAME)' already exists); \
		read -p "Do you want to delete it and recreate? [y/N]: " -r; \
		if [[ $$REPLY =~ ^[Yy]$$ ]]; then \
			$(call log_info, Deleting existing cluster...); \
			kind delete cluster --name "$(KIND_CLUSTER_NAME)"; \
		else \
			$(call log_info, Using existing cluster); \
			exit 0; \
		fi; \
	fi
	@$(KIND_SCRIPT) --cluster-name $(KIND_CLUSTER_NAME) --external-dns $(KIND_EXTERNAL_DNS) --network $(KIND_NETWORK)

# Install Cilium CNI
.PHONY: kind.install.cilium
kind.install.cilium: ## Install Cilium CNI in the Kind cluster
	@$(call log_info, Installing Cilium CNI in cluster '$(KIND_CLUSTER_NAME)'...)
	@$(call log_info, Checking if cluster exists...)
	@if ! kind get clusters | grep -q "^$(KIND_CLUSTER_NAME)$$"; then \
		$(call log_error, Kind cluster '$(KIND_CLUSTER_NAME)' does not exist); \
		$(call log_info, Please create it first using: make kind); \
		exit 1; \
	fi
	@$(call log_info, Processing Cilium images...)
	@if ! docker image inspect "quay.io/cilium/operator-generic:v$(CILIUM_VERSION)" > /dev/null 2>&1; then \
		$(call log_info, Pulling Cilium operator image...); \
		docker pull "quay.io/cilium/operator-generic:v$(CILIUM_VERSION)"; \
	fi
	@if ! docker image inspect "quay.io/cilium/cilium:v$(CILIUM_VERSION)" > /dev/null 2>&1; then \
		$(call log_info, Pulling Cilium image...); \
		docker pull "quay.io/cilium/cilium:v$(CILIUM_VERSION)"; \
	fi
	@if ! docker image inspect "quay.io/cilium/cilium-envoy:$(CILIUM_ENVOY_VERSION)" > /dev/null 2>&1; then \
		$(call log_info, Pulling Cilium Envoy image...); \
		docker pull "quay.io/cilium/cilium-envoy:$(CILIUM_ENVOY_VERSION)"; \
	fi
	@$(call log_info, Loading Cilium images into Kind cluster...)
	@kind load docker-image "quay.io/cilium/operator-generic:v$(CILIUM_VERSION)" --name "$(KIND_CLUSTER_NAME)"
	@kind load docker-image "quay.io/cilium/cilium:v$(CILIUM_VERSION)" --name "$(KIND_CLUSTER_NAME)"
	@kind load docker-image "quay.io/cilium/cilium-envoy:$(CILIUM_ENVOY_VERSION)" --name "$(KIND_CLUSTER_NAME)"
	@$(call log_info, Adding Cilium Helm repository...)
	@helm repo add cilium https://helm.cilium.io/ || true
	@helm repo update
	@$(call log_info, Getting Kubernetes API server IP...)
	@K8S_API_IP=$$(docker inspect "$(KIND_CLUSTER_NAME)-control-plane" --format='{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}'); \
	if [ -z "$$K8S_API_IP" ]; then \
		$(call log_error, Failed to get Kubernetes API server IP); \
		exit 1; \
	fi; \
	$(call log_info, Kubernetes API server IP: $$K8S_API_IP); \
	$(call log_info, Installing Cilium via Helm...); \
	helm upgrade --install cilium cilium/cilium \
		--version "$(CILIUM_VERSION)" \
		--namespace cilium \
		--values "$(DEV_SCRIPTS_DIR)/cilium-values.yaml" \
		--set k8sServiceHost="$$K8S_API_IP" \
		--kube-context "kind-$(KIND_CLUSTER_NAME)" \
		--create-namespace \
		--wait \
		--timeout=10m
	@$(call log_success, Cilium has been successfully installed in the kind cluster '$(KIND_CLUSTER_NAME)')

# Build and load OpenChoreo components into Kind cluster
.PHONY: kind.build.openchoreo
kind.build.openchoreo: ## Build all OpenChoreo components and load them into the Kind cluster
	@$(call log_info, Building OpenChoreo components with image tag '$(OPENCHOREO_IMAGE_TAG)'...)
	@$(call log_info, Building controller...)
	@$(MAKE) go.build-multiarch.manager
	@docker build -f $(PROJECT_DIR)/Dockerfile -t $(CONTROLLER_IMAGE) $(PROJECT_DIR)
	@$(call log_info, Building API...)
	@$(MAKE) go.build-multiarch.openchoreo-api
	@docker build -f $(PROJECT_DIR)/cmd/openchoreo-api/Dockerfile -t $(API_IMAGE) $(PROJECT_DIR)
	@$(call log_info, Building UI...)
	@cd $(PROJECT_DIR)/ui && yarn install --immutable && yarn build:all
	@docker build -f $(PROJECT_DIR)/ui/packages/backend/Dockerfile -t $(UI_IMAGE) $(PROJECT_DIR)/ui
	@$(call log_success, All OpenChoreo components built successfully!)
	@$(call log_info, Loading OpenChoreo images into cluster '$(KIND_CLUSTER_NAME)'...)
	@kind load docker-image $(CONTROLLER_IMAGE) --name $(KIND_CLUSTER_NAME)
	@kind load docker-image $(API_IMAGE) --name $(KIND_CLUSTER_NAME)
	@kind load docker-image $(UI_IMAGE) --name $(KIND_CLUSTER_NAME)
	@$(call log_success, All images loaded into Kind cluster!)

# Install OpenChoreo via Helm
.PHONY: kind.install.openchoreo
kind.install.openchoreo: ## Install OpenChoreo using Helm chart
	@$(call log_info, Installing OpenChoreo via Helm into namespace '$(OPENCHOREO_NAMESPACE)'...)
	@kubectl create namespace $(OPENCHOREO_NAMESPACE) --dry-run=client -o yaml | kubectl apply -f -
	@helm upgrade --install openchoreo-secure-core $(HELM_DIR) \
		--namespace $(OPENCHOREO_NAMESPACE) \
		--values $(DEV_SCRIPTS_DIR)/openchoreo-values.yaml \
		--set controllerManager.image.repository=openchoreo-controller \
		--set controllerManager.image.tag=$(OPENCHOREO_IMAGE_TAG) \
		--set openchoreoApi.image.repository=openchoreo-api \
		--set openchoreoApi.image.tag=$(OPENCHOREO_IMAGE_TAG) \
		--set backstage.image.repository=openchoreo-ui \
		--set backstage.image.tag=$(OPENCHOREO_IMAGE_TAG) \
		--wait \
		--timeout=10m
	@$(call log_success, OpenChoreo installed successfully!)
	@$(call log_info, To install the default dataplane, run: ./install/add-default-dataplane.sh)

# Uninstall OpenChoreo via Helm
.PHONY: kind.down.openchoreo
kind.down.openchoreo: ## Uninstall OpenChoreo using Helm chart
	@$(call log_info, Uninstalling OpenChoreo from namespace '$(OPENCHOREO_NAMESPACE)'...)
	@helm uninstall openchoreo-secure-core --namespace $(OPENCHOREO_NAMESPACE) || true
	@$(call log_success, OpenChoreo uninstalled successfully!)

# Complete setup
.PHONY: kind.setup
kind.setup: kind kind.install.cilium kind.build.openchoreo kind.install.openchoreo ## Complete Kind cluster setup with OpenChoreo
	@$(call log_success, OpenChoreo development environment is ready!)
	@$(call log_info, Access OpenChoreo services:)
	@$(call log_info,   - API: kubectl port-forward -n $(OPENCHOREO_NAMESPACE) svc/openchoreo-api 8080:8080)
	@$(call log_info,   - UI: kubectl port-forward -n $(OPENCHOREO_NAMESPACE) svc/openchoreo-backstage 7007:7007)

# Clean up everything
.PHONY: kind.down
kind.down: ## Delete Kind cluster
	@$(call log_info, Deleting Kind cluster '$(KIND_CLUSTER_NAME)'...)
	@if ! command -v kind >/dev/null 2>&1; then \
		$(call log_error, Please install kind first:); \
		$(call log_info,   https://kind.sigs.k8s.io/docs/user/quick-start/#installation); \
		exit 1; \
	fi
	@if ! kind get clusters | grep -q "^$(KIND_CLUSTER_NAME)$$"; then \
		$(call log_info, Kind cluster '$(KIND_CLUSTER_NAME)' does not exist); \
		exit 0; \
	fi
	@if kind delete cluster --name "$(KIND_CLUSTER_NAME)"; then \
		$(call log_success, Kind cluster '$(KIND_CLUSTER_NAME)' has been deleted successfully); \
	else \
		$(call log_error, Failed to delete kind cluster '$(KIND_CLUSTER_NAME)'); \
		exit 1; \
	fi
	@$(call log_info, Development environment cleaned up)

# Check cluster status
.PHONY: kind.status
kind.status: ## Check the status of the Kind cluster and OpenChoreo components
	@$(call log_info, Checking Kind cluster status...)
	@if kind get clusters | grep -q "^$(KIND_CLUSTER_NAME)$$"; then \
		$(call log_success, Kind cluster '$(KIND_CLUSTER_NAME)' exists); \
		echo ""; \
		$(call log_info, Cluster nodes:); \
		kind get nodes --name $(KIND_CLUSTER_NAME); \
		echo ""; \
		if kubectl get namespace $(OPENCHOREO_NAMESPACE) >/dev/null 2>&1; then \
			$(call log_success, Namespace '$(OPENCHOREO_NAMESPACE)' exists); \
			echo ""; \
			$(call log_info, OpenChoreo pods:); \
			kubectl get pods -n $(OPENCHOREO_NAMESPACE) -l app.kubernetes.io/part-of=openchoreo 2>/dev/null || $(call log_info, No OpenChoreo pods found); \
			echo ""; \
			$(call log_info, OpenChoreo services:); \
			kubectl get services -n $(OPENCHOREO_NAMESPACE) -l app.kubernetes.io/part-of=openchoreo 2>/dev/null || $(call log_info, No OpenChoreo services found); \
		else \
			$(call log_error, Namespace '$(OPENCHOREO_NAMESPACE)' does not exist); \
		fi; \
	else \
		$(call log_error, Kind cluster '$(KIND_CLUSTER_NAME)' does not exist); \
		$(call log_info, Create it with: make kind); \
	fi

# Access services
.PHONY: kind.access.api
kind.access.api: ## Port-forward OpenChoreo API service to localhost:8080
	@$(call log_info, Port-forwarding OpenChoreo API to localhost:8080...)
	@kubectl port-forward -n $(OPENCHOREO_NAMESPACE) svc/openchoreo-api 8080:8080

.PHONY: kind.access.ui
kind.access.ui: ## Port-forward OpenChoreo UI service to localhost:7007
	@$(call log_info, Port-forwarding OpenChoreo UI to localhost:7007...)
	@kubectl port-forward -n $(OPENCHOREO_NAMESPACE) svc/openchoreo-backstage 7007:7007

# Help target for Kind commands
.PHONY: kind.help
kind.help: ## Show help for Kind development commands
	@echo "OpenChoreo Kind Development Commands:"
	@echo ""
	@echo "Main Commands:"
	@echo "  make kind                    Create Kind cluster"
	@echo "  make kind.exists             Check if cluster exists"
	@echo "  make kind.install.cilium     Install Cilium CNI"
	@echo "  make kind.build.openchoreo   Build and load OpenChoreo components"
	@echo "  make kind.install.openchoreo Install OpenChoreo via Helm"
	@echo "  make kind.setup              Complete setup (cluster + Cilium + build + install)"
	@echo "  make kind.down.openchoreo    Uninstall OpenChoreo from cluster"
	@echo "  make kind.down               Clean up everything"
	@echo ""
	@echo "Management Commands:"
	@echo "  make kind.status             Check cluster status"
	@echo ""
	@echo "Access Commands:"
	@echo "  make kind.access.api         Port-forward API to localhost:8080"
	@echo "  make kind.access.ui          Port-forward UI to localhost:7007"
	@echo ""
	@echo "Configuration Variables:"
	@echo "  KIND_CLUSTER_NAME=$(KIND_CLUSTER_NAME)"
	@echo "  OPENCHOREO_IMAGE_TAG=$(OPENCHOREO_IMAGE_TAG)"
	@echo "  OPENCHOREO_NAMESPACE=$(OPENCHOREO_NAMESPACE)"
	@echo "  KIND_EXTERNAL_DNS=$(KIND_EXTERNAL_DNS)"
	@echo "  CILIUM_VERSION=$(CILIUM_VERSION)"
	@echo "  CILIUM_ENVOY_VERSION=$(CILIUM_ENVOY_VERSION)"
	@echo ""
	@echo "Examples:"
	@echo "  make kind.setup                              # Complete setup with defaults"
	@echo "  make kind.setup OPENCHOREO_IMAGE_TAG=v1.0.0        # Custom image tag"
	@echo "  make kind.setup KIND_CLUSTER_NAME=my-cluster # Custom cluster name"
