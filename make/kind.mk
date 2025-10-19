# This makefile contains all the make targets related to Kind cluster management for OpenChoreo development.

# Kind cluster configuration
KIND_CLUSTER_NAME ?= openchoreo
KIND_IMAGE_TAG ?= dev
KIND_NAMESPACE ?= openchoreo
KIND_EXTERNAL_DNS ?= 8.8.8.8
KIND_NETWORK ?= openchoreo

# Paths to development scripts
DEV_SCRIPTS_DIR := $(PROJECT_DIR)/install/dev
KIND_SCRIPT := $(DEV_SCRIPTS_DIR)/kind.sh
KIND_INSTALL_CILIUM_SCRIPT := $(DEV_SCRIPTS_DIR)/kind-install-cilium.sh
KIND_INSTALL_OPENCHOREO_SCRIPT := $(DEV_SCRIPTS_DIR)/kind-install-openchoreo.sh
KIND_DOWN_SCRIPT := $(DEV_SCRIPTS_DIR)/kind-down.sh

# Cilium configuration
CILIUM_VERSION := 1.18.2

##@ Kind Cluster Management

# Create Kind cluster
.PHONY: kind
kind: ## Create a Kind cluster for OpenChoreo development
	@$(call log_info, Creating Kind cluster '$(KIND_CLUSTER_NAME)'...)
	@$(KIND_SCRIPT) --cluster-name $(KIND_CLUSTER_NAME) --external-dns $(KIND_EXTERNAL_DNS) --network $(KIND_NETWORK)

# Delete Kind cluster
.PHONY: kind.delete
kind.delete: ## Delete the Kind cluster
	@$(call log_info, Deleting Kind cluster '$(KIND_CLUSTER_NAME)'...)
	@$(KIND_DOWN_SCRIPT) --cluster-name $(KIND_CLUSTER_NAME)

# Install Cilium CNI
.PHONY: kind.install.cilium
kind.install.cilium: ## Install Cilium CNI in the Kind cluster
	@$(call log_info, Installing Cilium CNI in cluster '$(KIND_CLUSTER_NAME)'...)
	@$(KIND_INSTALL_CILIUM_SCRIPT) --cluster-name $(KIND_CLUSTER_NAME)

# Build and install OpenChoreo
.PHONY: kind.install.openchoreo
kind.install.openchoreo: ## Build and install OpenChoreo components in the Kind cluster
	@$(call log_info, Building and installing OpenChoreo in cluster '$(KIND_CLUSTER_NAME)' with image tag '$(KIND_IMAGE_TAG)'...)
	@$(KIND_INSTALL_OPENCHOREO_SCRIPT) --cluster-name $(KIND_CLUSTER_NAME) --image-tag $(KIND_IMAGE_TAG) --namespace $(KIND_NAMESPACE)

# Complete development setup (cluster + Cilium + OpenChoreo)
.PHONY: kind.dev.setup
kind.dev.setup: kind kind.install.cilium kind.install.openchoreo ## Complete development environment setup
	@$(call log_success, OpenChoreo development environment is ready!)
	@echo "Access OpenChoreo services:"
	@echo "  - API: kubectl port-forward -n $(KIND_NAMESPACE) svc/openchoreo-api 8080:8080"
	@echo "  - UI: kubectl port-forward -n $(KIND_NAMESPACE) svc/openchoreo-backstage 7007:7007"

# Clean up development environment
.PHONY: kind.dev.cleanup
kind.dev.cleanup: kind.delete ## Clean up the complete development environment
	@$(call log_info, Development environment cleaned up)

# Check Kind cluster status
.PHONY: kind.status
kind.status: ## Check the status of the Kind cluster and OpenChoreo components
	@$(call log_info, Checking Kind cluster status...)
	@if kind get clusters | grep -q "^$(KIND_CLUSTER_NAME)$$"; then \
		echo "✅ Kind cluster '$(KIND_CLUSTER_NAME)' exists"; \
		echo ""; \
		echo "Cluster nodes:"; \
		kind get nodes --name $(KIND_CLUSTER_NAME); \
		echo ""; \
		if kubectl get namespace $(KIND_NAMESPACE) >/dev/null 2>&1; then \
			echo "✅ Namespace '$(KIND_NAMESPACE)' exists"; \
			echo ""; \
			echo "OpenChoreo pods:"; \
			kubectl get pods -n $(KIND_NAMESPACE) -l app.kubernetes.io/part-of=openchoreo 2>/dev/null || echo "No OpenChoreo pods found"; \
			echo ""; \
			echo "OpenChoreo services:"; \
			kubectl get services -n $(KIND_NAMESPACE) -l app.kubernetes.io/part-of=openchoreo 2>/dev/null || echo "No OpenChoreo services found"; \
		else \
			echo "❌ Namespace '$(KIND_NAMESPACE)' does not exist"; \
		fi; \
	else \
		echo "❌ Kind cluster '$(KIND_CLUSTER_NAME)' does not exist"; \
		echo "Create it with: make kind"; \
	fi

# Reload OpenChoreo components (rebuild and reinstall)
.PHONY: kind.reload.openchoreo
kind.reload.openchoreo: ## Rebuild and reinstall OpenChoreo components
	@$(call log_info, Rebuilding and reinstalling OpenChoreo components...)
	@$(KIND_INSTALL_OPENCHOREO_SCRIPT) --cluster-name $(KIND_CLUSTER_NAME) --image-tag $(KIND_IMAGE_TAG) --namespace $(KIND_NAMESPACE)

# Access OpenChoreo services
.PHONY: kind.access.api
kind.access.api: ## Port-forward OpenChoreo API service to localhost:8080
	@$(call log_info, Port-forwarding OpenChoreo API to localhost:8080...)
	@kubectl port-forward -n $(KIND_NAMESPACE) svc/openchoreo-api 8080:8080

.PHONY: kind.access.ui
kind.access.ui: ## Port-forward OpenChoreo UI service to localhost:7007
	@$(call log_info, Port-forwarding OpenChoreo UI to localhost:7007...)
	@kubectl port-forward -n $(KIND_NAMESPACE) svc/openchoreo-backstage 7007:7007

# Logs for OpenChoreo components
.PHONY: kind.logs.controller
kind.logs.controller: ## Show logs for OpenChoreo controller
	@kubectl logs -n $(KIND_NAMESPACE) -l control-plane=controller-manager -f

.PHONY: kind.logs.api
kind.logs.api: ## Show logs for OpenChoreo API
	@kubectl logs -n $(KIND_NAMESPACE) -l app=openchoreo-api -f

.PHONY: kind.logs.ui
kind.logs.ui: ## Show logs for OpenChoreo UI
	@kubectl logs -n $(KIND_NAMESPACE) -l app.kubernetes.io/component=backstage -f

# Shell access to cluster
.PHONY: kind.shell
kind.shell: ## Open a shell in the Kind cluster control-plane node
	@$(call log_info, Opening shell in control-plane node...)
	@docker exec -it $(KIND_CLUSTER_NAME)-control-plane bash

# Help target for Kind commands
.PHONY: kind.help
kind.help: ## Show help for Kind development commands
	@echo "OpenChoreo Kind Development Commands:"
	@echo ""
	@echo "Setup Commands:"
	@echo "  make kind                    Create Kind cluster"
	@echo "  make kind.install.cilium     Install Cilium CNI"
	@echo "  make kind.install.openchoreo Install OpenChoreo"
	@echo "  make kind.dev.setup          Complete setup (cluster + Cilium + OpenChoreo)"
	@echo ""
	@echo "Management Commands:"
	@echo "  make kind.status             Check cluster status"
	@echo "  make kind.reload.openchoreo  Rebuild and reinstall OpenChoreo"
	@echo "  make kind.delete             Delete cluster"
	@echo "  make kind.dev.cleanup        Clean up everything"
	@echo ""
	@echo "Access Commands:"
	@echo "  make kind.access.api         Port-forward API to localhost:8080"
	@echo "  make kind.access.ui          Port-forward UI to localhost:7007"
	@echo ""
	@echo "Debug Commands:"
	@echo "  make kind.logs.controller    Show controller logs"
	@echo "  make kind.logs.api           Show API logs"
	@echo "  make kind.logs.ui            Show UI logs"
	@echo "  make kind.shell              Access cluster node shell"
	@echo ""
	@echo "Configuration Variables:"
	@echo "  KIND_CLUSTER_NAME=$(KIND_CLUSTER_NAME)"
	@echo "  KIND_IMAGE_TAG=$(KIND_IMAGE_TAG)"
	@echo "  KIND_NAMESPACE=$(KIND_NAMESPACE)"
	@echo "  KIND_EXTERNAL_DNS=$(KIND_EXTERNAL_DNS)"
	@echo ""
	@echo "Examples:"
	@echo "  make kind                                    # Use defaults"
	@echo "  make kind KIND_CLUSTER_NAME=my-cluster       # Custom cluster name"
	@echo "  make kind.install.openchoreo KIND_IMAGE_TAG=v1.0.0  # Custom image tag"
