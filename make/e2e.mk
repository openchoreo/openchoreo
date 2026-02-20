# This makefile contains all the make targets related to e2e testing.

# Cluster
E2E_CLUSTER_NAME       ?= openchoreo-e2e
E2E_KUBECONTEXT        := k3d-$(E2E_CLUSTER_NAME)

# "local" uses chart dirs from install/helm/, "oci" pulls published charts from HELM_OCI_REGISTRY
E2E_HELM_SOURCE        ?= local
# Set to "true" to include build plane and observability plane in the e2e setup
E2E_WITH_BUILD         ?= false
E2E_WITH_OBSERVABILITY ?= false
# Go duration for the test suite (go test -timeout)
E2E_TEST_TIMEOUT       ?= 20m
# Go duration for each individual helm install and kubectl wait (not the overall setup timeout)
E2E_SETUP_TIMEOUT      ?= 5m

# Directories
E2E_DIR                := $(PROJECT_DIR)/test/e2e
E2E_K3D_DIR            := $(E2E_DIR)/k3d
E2E_DIAGNOSTICS_DIR    ?= $(E2E_DIR)/_diagnostics

# Namespaces
E2E_CP_NS              := openchoreo-control-plane
E2E_DP_NS              := openchoreo-data-plane
E2E_BP_NS              := openchoreo-build-plane
E2E_OP_NS              := openchoreo-observability-plane

# Dependency versions (keep in sync with install/k3d/single-cluster/README.md)
GATEWAY_API_VERSION    ?= v1.4.1
CERT_MANAGER_VERSION   ?= v1.19.2
ESO_VERSION            ?= 1.3.2
KGATEWAY_VERSION       ?= v2.1.1
THUNDER_VERSION        ?= 0.21.0

# Helm chart references: local chart dirs or OCI registry
ifeq ($(E2E_HELM_SOURCE),oci)
  E2E_HELM_DEP_UPDATE :=
  E2E_CP_CHART := $(HELM_OCI_REGISTRY)/openchoreo-control-plane --version $(HELM_CHART_VERSION)
  E2E_DP_CHART := $(HELM_OCI_REGISTRY)/openchoreo-data-plane --version $(HELM_CHART_VERSION)
  E2E_BP_CHART := $(HELM_OCI_REGISTRY)/openchoreo-build-plane --version $(HELM_CHART_VERSION)
  E2E_OP_CHART := $(HELM_OCI_REGISTRY)/openchoreo-observability-plane --version $(HELM_CHART_VERSION)
else
  E2E_HELM_DEP_UPDATE := --dependency-update
  E2E_CP_CHART := $(HELM_CHARTS_DIR)/openchoreo-control-plane
  E2E_DP_CHART := $(HELM_CHARTS_DIR)/openchoreo-data-plane
  E2E_BP_CHART := $(HELM_CHARTS_DIR)/openchoreo-build-plane
  E2E_OP_CHART := $(HELM_CHARTS_DIR)/openchoreo-observability-plane
endif

# Shorthand for kubectl/helm with e2e context
E2E_KUBECTL := kubectl --context $(E2E_KUBECONTEXT)
E2E_HELM    := helm --kube-context $(E2E_KUBECONTEXT)

# ---------------------------------------------------------------------------
# Helper: copy cluster-gateway certs from CP namespace to a target namespace.
# The CP chart creates a CA cert (via cert-manager) and a Job that extracts it
# into a ConfigMap. Both the ConfigMap and Secret must be copied to each plane
# namespace for the cluster-agent mTLS handshake to work.
# Usage: $(call e2e_copy_gateway_certs,<target-namespace>)
# ---------------------------------------------------------------------------
define e2e_copy_gateway_certs
	@$(call log_info, Copying cluster-gateway certs to $(1))
	@$(E2E_KUBECTL) create namespace $(1) --dry-run=client -o yaml | $(E2E_KUBECTL) apply -f -
	@CA_CRT=$$($(E2E_KUBECTL) get configmap cluster-gateway-ca \
		-n $(E2E_CP_NS) -o jsonpath='{.data.ca\.crt}') && \
	$(E2E_KUBECTL) create configmap cluster-gateway-ca \
		--from-literal=ca.crt="$$CA_CRT" \
		-n $(1) --dry-run=client -o yaml | $(E2E_KUBECTL) apply -f -
	@TLS_CRT=$$($(E2E_KUBECTL) get secret cluster-gateway-ca \
		-n $(E2E_CP_NS) -o jsonpath='{.data.tls\.crt}' | base64 -d) && \
	TLS_KEY=$$($(E2E_KUBECTL) get secret cluster-gateway-ca \
		-n $(E2E_CP_NS) -o jsonpath='{.data.tls\.key}' | base64 -d) && \
	CA_CRT=$$($(E2E_KUBECTL) get configmap cluster-gateway-ca \
		-n $(E2E_CP_NS) -o jsonpath='{.data.ca\.crt}') && \
	$(E2E_KUBECTL) create secret generic cluster-gateway-ca \
		--from-literal=tls.crt="$$TLS_CRT" \
		--from-literal=tls.key="$$TLS_KEY" \
		--from-literal=ca.crt="$$CA_CRT" \
		-n $(1) --dry-run=client -o yaml | $(E2E_KUBECTL) apply -f -
endef

# ---------------------------------------------------------------------------
# Helper: patch gateway-default deployment with /tmp volume (kgateway#9800).
# Usage: $(call e2e_patch_gateway,<namespace>)
# ---------------------------------------------------------------------------
define e2e_patch_gateway
	@$(call log_info, Waiting for gateway-default deployment in $(1))
	@for i in $$(seq 1 30); do \
		$(E2E_KUBECTL) get deployment gateway-default -n $(1) >/dev/null 2>&1 && break; \
		if [ $$i -eq 30 ]; then echo "gateway-default not found in $(1), skipping patch"; exit 0; fi; \
		sleep 2; \
	done
	@$(call log_info, Patching gateway-default in $(1) with /tmp volume)
	@$(E2E_KUBECTL) patch deployment gateway-default -n $(1) \
		--type='json' \
		-p='[{"op":"add","path":"/spec/template/spec/volumes/-","value":{"name":"tmp","emptyDir":{}}},{"op":"add","path":"/spec/template/spec/containers/0/volumeMounts/-","value":{"name":"tmp","mountPath":"/tmp"}}]'
endef

# ---------------------------------------------------------------------------
# Helper: register a plane CR by injecting the cluster-agent CA cert via yq.
# Waits for the agent TLS secret, extracts CA, substitutes into template YAML.
# Usage: $(call e2e_register_plane,<namespace>,<template-yaml>)
# ---------------------------------------------------------------------------
define e2e_register_plane
	@$(call log_info, Waiting for cluster-agent TLS cert in $(1))
	@for i in $$(seq 1 60); do \
		$(E2E_KUBECTL) get secret cluster-agent-tls -n $(1) >/dev/null 2>&1 && break; \
		if [ $$i -eq 60 ]; then echo "Timed out waiting for cluster-agent-tls in $(1)"; exit 1; fi; \
		sleep 2; \
	done
	@export AGENT_CA=$$($(E2E_KUBECTL) get secret cluster-agent-tls \
		-n $(1) -o jsonpath='{.data.ca\.crt}' | base64 -d) && \
	yq '.spec.clusterAgent.clientCA.value = strenv(AGENT_CA)' $(2) | \
	$(E2E_KUBECTL) apply -f -
endef

##@ E2E Testing

# ---------------------------------------------------------------------------
# Lifecycle target
# ---------------------------------------------------------------------------

.PHONY: e2e
e2e: ## Full e2e lifecycle: setup → test → down (collects diagnostics on failure)
	@setup_ok=0; \
	$(MAKE) e2e.setup && setup_ok=1; \
	if [ $$setup_ok -eq 1 ]; then \
		$(MAKE) e2e.test; test_exit=$$?; \
		if [ $$test_exit -ne 0 ]; then $(MAKE) e2e.diagnostics || true; fi; \
	else \
		test_exit=1; \
		$(MAKE) e2e.diagnostics || true; \
	fi; \
	$(MAKE) e2e.down || true; \
	exit $$test_exit

# ---------------------------------------------------------------------------
# Setup targets
# ---------------------------------------------------------------------------

.PHONY: e2e.setup
e2e.setup: ## All setup: cluster + prerequisites + install + configure
	@$(MAKE) e2e.setup-cluster
	@$(MAKE) e2e.setup-prerequisites
	@$(MAKE) e2e.setup-install
	@$(MAKE) e2e.setup-configure
	@$(call log_success, E2E setup complete)

.PHONY: e2e.setup-cluster
e2e.setup-cluster: ## Create k3d cluster
	@$(call log_info, Creating k3d cluster '$(E2E_CLUSTER_NAME)')
	k3d cluster create --config $(E2E_K3D_DIR)/config.yaml
	@$(call log_info, Applying CoreDNS rewrite for e2e domains)
	$(E2E_KUBECTL) apply -f $(E2E_K3D_DIR)/coredns-custom.yaml
	@$(call log_success, k3d cluster '$(E2E_CLUSTER_NAME)' created)

.PHONY: e2e.setup-prerequisites
e2e.setup-prerequisites: ## Install Gateway API, cert-manager, ESO, kgateway
	@$(call log_info, Installing Gateway API CRDs $(GATEWAY_API_VERSION))
	$(E2E_KUBECTL) apply --server-side \
		-f https://github.com/kubernetes-sigs/gateway-api/releases/download/$(GATEWAY_API_VERSION)/experimental-install.yaml
	@$(call log_info, Installing cert-manager $(CERT_MANAGER_VERSION))
	$(E2E_HELM) upgrade --install cert-manager oci://quay.io/jetstack/charts/cert-manager \
		--namespace cert-manager --create-namespace \
		--version $(CERT_MANAGER_VERSION) --set crds.enabled=true \
		--wait --timeout $(E2E_SETUP_TIMEOUT)
	@$(call log_info, Installing External Secrets Operator $(ESO_VERSION))
	$(E2E_HELM) upgrade --install external-secrets oci://ghcr.io/external-secrets/charts/external-secrets \
		--namespace external-secrets --create-namespace \
		--version $(ESO_VERSION) --set installCRDs=true \
		--wait --timeout $(E2E_SETUP_TIMEOUT)
	@$(call log_info, Installing kgateway $(KGATEWAY_VERSION))
	$(E2E_HELM) upgrade --install kgateway-crds oci://cr.kgateway.dev/kgateway-dev/charts/kgateway-crds \
		--version $(KGATEWAY_VERSION)
	$(E2E_HELM) upgrade --install kgateway oci://cr.kgateway.dev/kgateway-dev/charts/kgateway \
		--namespace $(E2E_CP_NS) --create-namespace \
		--version $(KGATEWAY_VERSION) \
		--wait --timeout $(E2E_SETUP_TIMEOUT)
	@$(call log_info, Creating ClusterSecretStore)
	$(E2E_KUBECTL) apply -f $(E2E_K3D_DIR)/secretstore.yaml
	@$(call log_success, Prerequisites installed)

.PHONY: e2e.setup-install
e2e.setup-install: ## Install all planes via Helm
	@$(MAKE) _e2e.install-thunder
	@$(MAKE) _e2e.install-cp
	@$(MAKE) _e2e.install-dp
	@if [ "$(E2E_WITH_BUILD)" = "true" ]; then $(MAKE) _e2e.install-bp; fi
	@if [ "$(E2E_WITH_OBSERVABILITY)" = "true" ]; then $(MAKE) _e2e.install-op; fi
	@$(call log_success, All planes installed)

.PHONY: e2e.setup-configure
e2e.setup-configure: ## Apply default resources, register planes, and link observability
	@$(call log_info, Applying default resources)
	$(E2E_KUBECTL) label namespace default openchoreo.dev/controlplane-namespace=true --overwrite
	$(E2E_KUBECTL) apply -f $(PROJECT_DIR)/samples/getting-started/all.yaml
	@$(MAKE) _e2e.configure-dp
	@if [ "$(E2E_WITH_BUILD)" = "true" ]; then $(MAKE) _e2e.configure-bp; fi
	@if [ "$(E2E_WITH_OBSERVABILITY)" = "true" ]; then $(MAKE) _e2e.configure-op; fi
	@if [ "$(E2E_WITH_OBSERVABILITY)" = "true" ]; then $(MAKE) _e2e.link-observability; fi
	@$(call log_success, E2E configuration complete)

# ---------------------------------------------------------------------------
# Internal install targets
# ---------------------------------------------------------------------------

.PHONY: _e2e.install-thunder
_e2e.install-thunder:
	@# Thunder requires a valid /etc/machine-id on the node
	docker exec k3d-$(E2E_CLUSTER_NAME)-server-0 sh -c \
		"cat /proc/sys/kernel/random/uuid | tr -d '-' > /etc/machine-id"
	@$(call log_info, Installing Thunder $(THUNDER_VERSION))
	$(E2E_HELM) upgrade --install thunder oci://ghcr.io/asgardeo/helm-charts/thunder \
		--namespace $(E2E_CP_NS) --create-namespace \
		--version $(THUNDER_VERSION) \
		--values $(PROJECT_DIR)/install/k3d/common/values-thunder.yaml \
		--values $(E2E_K3D_DIR)/values-thunder.yaml \
		--wait --timeout $(E2E_SETUP_TIMEOUT)

.PHONY: _e2e.install-cp
_e2e.install-cp:
	@$(call log_info, Installing Control Plane)
	$(E2E_HELM) upgrade --install openchoreo-control-plane $(E2E_CP_CHART) \
		$(E2E_HELM_DEP_UPDATE) \
		--namespace $(E2E_CP_NS) --create-namespace \
		--values $(E2E_K3D_DIR)/values-cp.yaml \
		--timeout $(E2E_SETUP_TIMEOUT)
	$(call e2e_patch_gateway,$(E2E_CP_NS))
	$(E2E_KUBECTL) wait -n $(E2E_CP_NS) \
		--for=condition=available --timeout=$(E2E_SETUP_TIMEOUT) deployment --all
	@# Wait for the CA extractor job to create the cluster-gateway-ca configmap
	@$(call log_info, Waiting for cluster-gateway CA)
	@for i in $$(seq 1 60); do \
		$(E2E_KUBECTL) get configmap cluster-gateway-ca -n $(E2E_CP_NS) >/dev/null 2>&1 && break; \
		if [ $$i -eq 60 ]; then echo "Timed out waiting for cluster-gateway-ca configmap"; exit 1; fi; \
		sleep 2; \
	done

.PHONY: _e2e.install-dp
_e2e.install-dp:
	$(call e2e_copy_gateway_certs,$(E2E_DP_NS))
	@$(call log_info, Installing Data Plane)
	$(E2E_HELM) upgrade --install openchoreo-data-plane $(E2E_DP_CHART) \
		$(E2E_HELM_DEP_UPDATE) \
		--namespace $(E2E_DP_NS) --create-namespace \
		--values $(E2E_K3D_DIR)/values-dp.yaml \
		--timeout $(E2E_SETUP_TIMEOUT)
	$(call e2e_patch_gateway,$(E2E_DP_NS))
	$(E2E_KUBECTL) wait -n $(E2E_DP_NS) \
		--for=condition=available --timeout=$(E2E_SETUP_TIMEOUT) deployment --all

.PHONY: _e2e.install-bp
_e2e.install-bp:
	$(call e2e_copy_gateway_certs,$(E2E_BP_NS))
	@$(call log_info, Installing container registry)
	helm repo add twuni https://twuni.github.io/docker-registry.helm 2>/dev/null || true
	helm repo update twuni
	$(E2E_HELM) upgrade --install registry twuni/docker-registry \
		--namespace $(E2E_BP_NS) --create-namespace \
		--values $(PROJECT_DIR)/install/k3d/single-cluster/values-registry.yaml
	@$(call log_info, Installing Build Plane)
	$(E2E_HELM) upgrade --install openchoreo-build-plane $(E2E_BP_CHART) \
		$(E2E_HELM_DEP_UPDATE) \
		--namespace $(E2E_BP_NS) --create-namespace \
		--values $(E2E_K3D_DIR)/values-bp.yaml \
		--timeout $(E2E_SETUP_TIMEOUT)
	$(E2E_KUBECTL) wait -n $(E2E_BP_NS) \
		--for=condition=available --timeout=$(E2E_SETUP_TIMEOUT) deployment --all

.PHONY: _e2e.install-op
_e2e.install-op:
	$(call e2e_copy_gateway_certs,$(E2E_OP_NS))
	@$(call log_info, Creating OpenSearch credentials)
	@$(E2E_KUBECTL) create namespace $(E2E_OP_NS) --dry-run=client -o yaml | $(E2E_KUBECTL) apply -f -
	@$(E2E_KUBECTL) create secret generic observer-opensearch-credentials \
		-n $(E2E_OP_NS) \
		--from-literal=username="admin" \
		--from-literal=password="ThisIsTheOpenSearchPassword1" \
		--dry-run=client -o yaml | $(E2E_KUBECTL) apply -f -
	@$(call log_info, Installing Observability Plane)
	$(E2E_HELM) upgrade --install openchoreo-observability-plane $(E2E_OP_CHART) \
		$(E2E_HELM_DEP_UPDATE) \
		--namespace $(E2E_OP_NS) --create-namespace \
		--values $(E2E_K3D_DIR)/values-op.yaml \
		--timeout $(E2E_SETUP_TIMEOUT)
	$(call e2e_patch_gateway,$(E2E_OP_NS))
	@$(call log_info, Installing observability modules)
	$(E2E_HELM) upgrade --install observability-logs-opensearch \
		oci://ghcr.io/openchoreo/charts/observability-logs-opensearch \
		--namespace $(E2E_OP_NS) \
		--wait --timeout $(E2E_SETUP_TIMEOUT)
	$(E2E_HELM) upgrade --install observability-metrics-prometheus \
		oci://ghcr.io/openchoreo/charts/observability-metrics-prometheus \
		--namespace $(E2E_OP_NS) \
		--wait --timeout $(E2E_SETUP_TIMEOUT)
	$(E2E_KUBECTL) wait -n $(E2E_OP_NS) \
		--for=condition=available --timeout=$(E2E_SETUP_TIMEOUT) deployment --all

# ---------------------------------------------------------------------------
# Internal configure targets
# ---------------------------------------------------------------------------

.PHONY: _e2e.configure-dp
_e2e.configure-dp:
	@$(call log_info, Registering DataPlane)
	$(call e2e_register_plane,$(E2E_DP_NS),$(E2E_K3D_DIR)/dataplane.yaml)

.PHONY: _e2e.configure-bp
_e2e.configure-bp:
	@$(call log_info, Registering BuildPlane)
	$(call e2e_register_plane,$(E2E_BP_NS),$(E2E_K3D_DIR)/buildplane.yaml)

.PHONY: _e2e.configure-op
_e2e.configure-op:
	@$(call log_info, Registering ObservabilityPlane)
	$(call e2e_register_plane,$(E2E_OP_NS),$(E2E_K3D_DIR)/observabilityplane.yaml)

.PHONY: _e2e.link-observability
_e2e.link-observability:
	@$(call log_info, Linking ObservabilityPlane to other planes)
	$(E2E_KUBECTL) patch dataplane default -n default --type merge \
		-p '{"spec":{"observabilityPlaneRef":{"kind":"ObservabilityPlane","name":"default"}}}'
	@if [ "$(E2E_WITH_BUILD)" = "true" ]; then \
		$(E2E_KUBECTL) patch buildplane default -n default --type merge \
			-p '{"spec":{"observabilityPlaneRef":{"kind":"ObservabilityPlane","name":"default"}}}'; \
	fi

# ---------------------------------------------------------------------------
# Test target
# ---------------------------------------------------------------------------

.PHONY: e2e.test
e2e.test: ## Run e2e test suite
	@$(call log_info, Running e2e tests)
	go test $(E2E_DIR)/ -v -ginkgo.v -timeout $(E2E_TEST_TIMEOUT) \
		--e2e.kubecontext=$(E2E_KUBECONTEXT)

# ---------------------------------------------------------------------------
# Utility targets
# ---------------------------------------------------------------------------

.PHONY: e2e.status
e2e.status: ## Check status of all planes and agent connections
	@echo "=== Pods ==="
	@$(E2E_KUBECTL) get pods -A
	@echo ""
	@echo "=== Plane Resources ==="
	@$(E2E_KUBECTL) get dataplane,buildplane,observabilityplane -n default 2>/dev/null || true
	@echo ""
	@echo "=== Agent Connections ==="
	@for ns in $(E2E_DP_NS) $(E2E_BP_NS) $(E2E_OP_NS); do \
		echo "--- $$ns ---"; \
		$(E2E_KUBECTL) logs -n $$ns -l app=cluster-agent --tail=3 2>/dev/null || echo "(no agent)"; \
	done

.PHONY: e2e.diagnostics
e2e.diagnostics: ## Collect logs, events, and resource dumps from all namespaces
	@$(call log_info, Collecting diagnostics to $(E2E_DIAGNOSTICS_DIR))
	@mkdir -p $(E2E_DIAGNOSTICS_DIR)
	@for ns in $(E2E_CP_NS) $(E2E_DP_NS) $(E2E_BP_NS) $(E2E_OP_NS) default; do \
		$(E2E_KUBECTL) get pods -n $$ns -o wide > $(E2E_DIAGNOSTICS_DIR)/pods-$$ns.txt 2>&1 || true; \
		$(E2E_KUBECTL) get events -n $$ns --sort-by=.lastTimestamp > $(E2E_DIAGNOSTICS_DIR)/events-$$ns.txt 2>&1 || true; \
		for pod in $$($(E2E_KUBECTL) get pods -n $$ns -o jsonpath='{.items[*].metadata.name}' 2>/dev/null); do \
			$(E2E_KUBECTL) logs $$pod -n $$ns --all-containers --tail=200 > $(E2E_DIAGNOSTICS_DIR)/logs-$$ns-$$pod.txt 2>&1 || true; \
		done; \
	done
	@$(E2E_KUBECTL) get dataplane,buildplane,observabilityplane -n default -o yaml > $(E2E_DIAGNOSTICS_DIR)/plane-resources.yaml 2>&1 || true
	@$(E2E_KUBECTL) get component,componentrelease,releasebinding,release -A -o yaml > $(E2E_DIAGNOSTICS_DIR)/release-chain.yaml 2>&1 || true
	@$(call log_success, Diagnostics collected to $(E2E_DIAGNOSTICS_DIR))

.PHONY: e2e.down
e2e.down: ## Delete k3d cluster
	@$(call log_info, Deleting k3d cluster '$(E2E_CLUSTER_NAME)')
	k3d cluster delete $(E2E_CLUSTER_NAME)
	@$(call log_success, k3d cluster '$(E2E_CLUSTER_NAME)' deleted)
