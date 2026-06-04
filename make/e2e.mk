# This makefile contains all the make targets related to e2e testing.

# Cluster
E2E_CLUSTER_NAME       ?= openchoreo-e2e
E2E_KUBECONTEXT        := k3d-$(E2E_CLUSTER_NAME)

# "local" uses chart dirs from install/helm/, "oci" pulls published charts from HELM_OCI_REGISTRY
E2E_HELM_SOURCE        ?= local
# Set to "true" to include workflow plane and observability plane in the e2e setup
E2E_WITH_BUILD         ?= false
E2E_WITH_OBSERVABILITY ?= false
# Set to "true" to enable Backstage in the control plane and run the Tier 5 UI suite
# (see test/ui/). Off by default so the existing e2e workflow keeps the lighter
# Backstage-disabled install.
E2E_WITH_UI            ?= false
# Go duration for the test suite (go test -timeout)
E2E_TEST_TIMEOUT       ?= 20m
# Go duration for each individual helm install and kubectl wait (not the overall setup timeout)
E2E_SETUP_TIMEOUT      ?= 5m
# Ginkgo label-filter expression to select which specs run. Empty = run everything.
# Suites are labeled `tier1`, `tier2`, … on their top-level Describe; see proposal #3509.
# Examples: `tier1`, `tier1 || tier2`, `tier1 && !tier2`.
E2E_LABEL_FILTER       ?=
# Optional job-local fixture set to run after e2e.setup and before the Go
# suite package fan-out. Current supported value: tier3.
E2E_JOB_FIXTURE_SET    ?=

# Conditionally render the Ginkgo label-filter flag so the unfiltered command line stays clean.
# Single-quote the value so shell metacharacters in the expression (e.g. `||`, `&&`) are not
# interpreted by the shell when Make substitutes the variable into the recipe.
ifneq ($(strip $(E2E_LABEL_FILTER)),)
  E2E_GINKGO_LABEL_FLAG := --ginkgo.label-filter='$(E2E_LABEL_FILTER)'
else
  E2E_GINKGO_LABEL_FLAG :=
endif

# Directories
E2E_DIR                := $(PROJECT_DIR)/test/e2e
E2E_K3D_DIR            := $(E2E_DIR)/k3d
E2E_DIAGNOSTICS_DIR    ?= $(E2E_DIR)/_diagnostics
UI_DIR                 := $(PROJECT_DIR)/test/ui
UI_K3D_DIR             := $(UI_DIR)/k3d

# When the UI suite is enabled, layer the cp-ui overlay on top of the default
# control-plane values so Backstage gets switched on.
ifeq ($(E2E_WITH_UI),true)
  E2E_CP_EXTRA_VALUES := --values $(UI_K3D_DIR)/values-cp-ui.yaml
else
  E2E_CP_EXTRA_VALUES :=
endif

# Namespaces
E2E_CP_NS              := openchoreo-control-plane
E2E_DP_NS              := openchoreo-data-plane
E2E_WP_NS              := openchoreo-workflow-plane
E2E_OP_NS              := openchoreo-observability-plane

# Dependency versions (keep in sync with install/k3d/single-cluster/README.md)
GATEWAY_API_VERSION    ?= v1.4.1
CERT_MANAGER_VERSION   ?= v1.19.4
ESO_VERSION            ?= 2.0.1
KGATEWAY_VERSION       ?= v2.2.1
OPENBAO_CHART_VERSION  ?= 0.25.6
THUNDER_VERSION        ?= 0.28.0
OBSERVABILITY_LOGS_OPENSEARCH_VERSION     ?= 0.4.1
OBSERVABILITY_TRACES_OPENSEARCH_VERSION   ?= 0.4.1
OBSERVABILITY_METRICS_PROMETHEUS_VERSION  ?= 0.6.1

# Helm chart references: local chart dirs or OCI registry
ifeq ($(E2E_HELM_SOURCE),oci)
  E2E_HELM_DEP_UPDATE :=
  E2E_CP_CHART := $(HELM_OCI_REGISTRY)/openchoreo-control-plane --version $(HELM_CHART_VERSION)
  E2E_DP_CHART := $(HELM_OCI_REGISTRY)/openchoreo-data-plane --version $(HELM_CHART_VERSION)
  E2E_WP_CHART := $(HELM_OCI_REGISTRY)/openchoreo-workflow-plane --version $(HELM_CHART_VERSION)
  E2E_OP_CHART := $(HELM_OCI_REGISTRY)/openchoreo-observability-plane --version $(HELM_CHART_VERSION)
else
  E2E_HELM_DEP_UPDATE := --dependency-update
  E2E_CP_CHART := $(HELM_CHARTS_DIR)/openchoreo-control-plane
  E2E_DP_CHART := $(HELM_CHARTS_DIR)/openchoreo-data-plane
  E2E_WP_CHART := $(HELM_CHARTS_DIR)/openchoreo-workflow-plane
  E2E_OP_CHART := $(HELM_CHARTS_DIR)/openchoreo-observability-plane
endif

# Shorthand for kubectl/helm with e2e context
E2E_KUBECTL := kubectl --context $(E2E_KUBECONTEXT)
E2E_HELM    := helm --kube-context $(E2E_KUBECONTEXT)

# ---------------------------------------------------------------------------
# Helper: copy cluster-gateway server CA from CP namespace to a target namespace.
# The agent needs the server CA to verify the gateway's TLS certificate.
# The CA is extracted from the cert-manager-issued secret during _e2e.install-cp
# and stored in the cluster-gateway-ca ConfigMap.
# Usage: $(call e2e_copy_gateway_certs,<target-namespace>)
# ---------------------------------------------------------------------------
define e2e_copy_gateway_certs
	@$(call log_info, Copying cluster-gateway CA to $(1))
	@$(E2E_KUBECTL) create namespace $(1) --dry-run=client -o yaml | $(E2E_KUBECTL) apply -f -
	@CA_CRT=$$($(E2E_KUBECTL) get configmap cluster-gateway-ca \
		-n $(E2E_CP_NS) -o jsonpath='{.data.ca\.crt}') && \
	$(E2E_KUBECTL) create configmap cluster-gateway-ca \
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
	test_exit=0; \
	if [ $$setup_ok -eq 1 ]; then \
		$(MAKE) e2e.setup-tier-fixtures || test_exit=$$?; \
		if [ $$test_exit -eq 0 ]; then $(MAKE) e2e.test || test_exit=$$?; fi; \
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
e2e.setup: ## All setup: cluster + prerequisites + install + configure (+ UI when E2E_WITH_UI=true)
	@$(MAKE) e2e.setup-cluster
	@$(MAKE) e2e.setup-prerequisites
	@$(MAKE) e2e.setup-install
	@$(MAKE) e2e.setup-configure
	@if [ "$(E2E_WITH_UI)" = "true" ]; then $(MAKE) e2e.setup-ui; fi
	@$(call log_success, E2E setup complete)

.PHONY: e2e.setup-tier-fixtures
e2e.setup-tier-fixtures: ## Run optional job-local shared fixture setup before tests
	@if [ -z "$(strip $(E2E_JOB_FIXTURE_SET))" ]; then exit 0; fi; \
	case "$(E2E_JOB_FIXTURE_SET)" in \
		tier3) $(MAKE) _e2e.setup-tier3-fixtures ;; \
		*) echo "Unsupported E2E_JOB_FIXTURE_SET='$(E2E_JOB_FIXTURE_SET)'"; exit 1 ;; \
	esac

.PHONY: _e2e.setup-tier3-fixtures
_e2e.setup-tier3-fixtures:
	@$(call log_info, Setting up Tier 3 shared e2e fixtures)
	go run $(E2E_DIR)/cmd/tier3-fixtures --e2e.kubecontext=$(E2E_KUBECONTEXT)

.PHONY: e2e.setup-ui
e2e.setup-ui: ## Enable Backstage on the control plane and wait for it to become Ready (idempotent)
	@# When run standalone against a cluster installed without E2E_WITH_UI=true,
	@# Backstage is not deployed yet — re-run the CP install with the UI overlay
	@# (which also provisions backstage-secrets). When invoked from e2e.setup
	@# the deployment already exists, so this branch is a no-op.
	@if ! $(E2E_KUBECTL) -n $(E2E_CP_NS) get deploy backstage >/dev/null 2>&1; then \
		$(MAKE) _e2e.install-cp E2E_WITH_UI=true; \
	fi
	@$(call log_info, Waiting for Backstage deployment to become Ready)
	$(E2E_KUBECTL) wait -n $(E2E_CP_NS) \
		--for=condition=available --timeout=$(E2E_SETUP_TIMEOUT) deploy/backstage
	@$(call log_success, UI setup complete (Backstage Ready))

.PHONY: _e2e.prepare-backstage-secret
_e2e.prepare-backstage-secret:
	@# The cp-ui overlay sets backstage.secretName=backstage-secrets. The
	@# Helm chart references it via envFrom, so it must exist before install.
	@$(call log_info, Provisioning backstage-secrets in $(E2E_CP_NS))
	$(E2E_KUBECTL) create namespace $(E2E_CP_NS) --dry-run=client -o yaml | $(E2E_KUBECTL) apply -f -
	@if ! $(E2E_KUBECTL) -n $(E2E_CP_NS) get secret backstage-secrets >/dev/null 2>&1; then \
		BACKEND_SECRET=$$(head -c 32 /dev/urandom | base64 | tr -d '\n'); \
		$(E2E_KUBECTL) -n $(E2E_CP_NS) create secret generic backstage-secrets \
			--from-literal=backend-secret="$$BACKEND_SECRET" \
			--from-literal=client-secret="backstage-portal-secret" \
			--from-literal=jenkins-api-key="placeholder-not-in-use"; \
	else \
		echo "backstage-secrets already exists, leaving in place"; \
	fi

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
	@if [ "$(E2E_WITH_BUILD)" = "true" ]; then $(MAKE) _e2e.install-openbao; fi
	@if [ "$(E2E_WITH_BUILD)" = "true" ]; then $(MAKE) _e2e.install-wp; fi
	@if [ "$(E2E_WITH_OBSERVABILITY)" = "true" ]; then $(MAKE) _e2e.install-op; fi
	@$(call log_success, All planes installed)

.PHONY: e2e.setup-configure
e2e.setup-configure: ## Apply default resources, register planes, and link observability
	@$(call log_info, Applying default resources)
	$(E2E_KUBECTL) label namespace default openchoreo.dev/control-plane=true --overwrite
	$(E2E_KUBECTL) apply -f $(PROJECT_DIR)/samples/getting-started/all.yaml
	@$(MAKE) _e2e.configure-dp
	@if [ "$(E2E_WITH_BUILD)" = "true" ]; then $(MAKE) _e2e.configure-wp; fi
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
		--namespace thunder --create-namespace \
		--version $(THUNDER_VERSION) \
		--values $(PROJECT_DIR)/install/k3d/common/values-thunder.yaml \
		--values $(E2E_K3D_DIR)/values-thunder.yaml \
		--wait --timeout $(E2E_SETUP_TIMEOUT)

.PHONY: _e2e.install-cp
_e2e.install-cp:
	@$(call log_info, Installing Control Plane)
	@if [ "$(E2E_WITH_UI)" = "true" ]; then $(MAKE) _e2e.prepare-backstage-secret; fi
	$(E2E_HELM) upgrade --install openchoreo-control-plane $(E2E_CP_CHART) \
		$(E2E_HELM_DEP_UPDATE) \
		--namespace $(E2E_CP_NS) --create-namespace \
		--values $(E2E_K3D_DIR)/values-cp.yaml \
		$(E2E_CP_EXTRA_VALUES) \
		--timeout $(E2E_SETUP_TIMEOUT)
	$(call e2e_patch_gateway,$(E2E_CP_NS))
	$(E2E_KUBECTL) wait -n $(E2E_CP_NS) \
		--for=condition=available --timeout=$(E2E_SETUP_TIMEOUT) deployment --all
	@# Wait for cert-manager to issue the cluster-gateway CA certificate, then
	@# extract the public CA cert into the ConfigMap that cluster-agents use.
	@$(call log_info, Waiting for cluster-gateway CA)
	$(E2E_KUBECTL) wait -n $(E2E_CP_NS) \
		--for=condition=Ready certificate/cluster-gateway-ca --timeout=$(E2E_SETUP_TIMEOUT)
	@$(E2E_KUBECTL) get secret cluster-gateway-ca -n $(E2E_CP_NS) \
		-o jsonpath='{.data.ca\.crt}' | base64 -d | \
	$(E2E_KUBECTL) create configmap cluster-gateway-ca \
		--from-file=ca.crt=/dev/stdin \
		-n $(E2E_CP_NS) \
		--dry-run=client -o yaml | $(E2E_KUBECTL) apply -f -

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

.PHONY: _e2e.install-openbao
_e2e.install-openbao:
	@$(call log_info, Installing OpenBao $(OPENBAO_CHART_VERSION))
	$(E2E_HELM) upgrade --install openbao oci://ghcr.io/openbao/charts/openbao \
		--namespace openbao --create-namespace \
		--version $(OPENBAO_CHART_VERSION) \
		--values $(PROJECT_DIR)/install/k3d/common/values-openbao.yaml \
		--wait --timeout $(E2E_SETUP_TIMEOUT)
	@$(call log_info, Replacing fake ClusterSecretStore with openbao-backed default)
	$(E2E_KUBECTL) delete clustersecretstore default --ignore-not-found
	$(E2E_KUBECTL) apply -f $(E2E_K3D_DIR)/openbao-secretstore.yaml

.PHONY: _e2e.install-wp
_e2e.install-wp:
	$(call e2e_copy_gateway_certs,$(E2E_WP_NS))
	@$(call log_info, Installing container registry)
	helm repo add twuni https://twuni.github.io/docker-registry.helm 2>/dev/null || true
	helm repo update twuni
	$(E2E_HELM) upgrade --install registry twuni/docker-registry \
		--namespace $(E2E_WP_NS) --create-namespace \
		--values $(PROJECT_DIR)/install/k3d/single-cluster/values-registry.yaml
	@$(call log_info, Installing Workflow Plane)
	$(E2E_HELM) upgrade --install openchoreo-workflow-plane $(E2E_WP_CHART) \
		$(E2E_HELM_DEP_UPDATE) \
		--namespace $(E2E_WP_NS) --create-namespace \
		--values $(E2E_K3D_DIR)/values-wp.yaml \
		--timeout $(E2E_SETUP_TIMEOUT)
	$(E2E_KUBECTL) wait -n $(E2E_WP_NS) \
		--for=condition=available --timeout=$(E2E_SETUP_TIMEOUT) deployment --all

.PHONY: _e2e.install-op
_e2e.install-op:
	$(call e2e_copy_gateway_certs,$(E2E_OP_NS))
	@$(call log_info, Creating OpenSearch credentials)
	@$(E2E_KUBECTL) create namespace $(E2E_OP_NS) --dry-run=client -o yaml | $(E2E_KUBECTL) apply -f -
	@# observer-opensearch-credentials provides the basic-auth pair the
	@# observer uses to talk to OpenSearch. The chart's
	@# observer-opensearch-secret.yaml failsafe requires it before install
	@# when observer.openSearchSecretName is unset (default chart value).
	@$(E2E_KUBECTL) create secret generic observer-opensearch-credentials \
		-n $(E2E_OP_NS) \
		--from-literal=username="admin" \
		--from-literal=password="ThisIsTheOpenSearchPassword1" \
		--dry-run=client -o yaml | $(E2E_KUBECTL) apply -f -
	@# observer-secret is consumed via envFrom by the observer deployment.
	@# The chart's failsafe (templates/observer/observer-deployment.yaml:1-3)
	@# rejects install when observer.secretName is unset, so it must exist
	@# before the helm upgrade. OPENSEARCH_USERNAME/PASSWORD pair with the
	@# admin credentials seeded above; UID_RESOLVER_OAUTH_CLIENT_SECRET
	@# pairs with the `openchoreo-observer-resource-reader-client`
	@# provisioned by thunder bootstrap
	@# (install/k3d/common/values-thunder.yaml).
	@$(E2E_KUBECTL) create secret generic observer-secret \
		-n $(E2E_OP_NS) \
		--from-literal=OPENSEARCH_USERNAME="admin" \
		--from-literal=OPENSEARCH_PASSWORD="ThisIsTheOpenSearchPassword1" \
		--from-literal=UID_RESOLVER_OAUTH_CLIENT_SECRET="openchoreo-observer-resource-reader-client-secret" \
		--dry-run=client -o yaml | $(E2E_KUBECTL) apply -f -
	@$(call log_info, Installing Observability Plane)
	$(E2E_HELM) upgrade --install openchoreo-observability-plane $(E2E_OP_CHART) \
		$(E2E_HELM_DEP_UPDATE) \
		--namespace $(E2E_OP_NS) --create-namespace \
		--values $(E2E_K3D_DIR)/values-op.yaml \
		--timeout $(E2E_SETUP_TIMEOUT)
	$(call e2e_patch_gateway,$(E2E_OP_NS))
	@$(call log_info, Installing observability modules)
	@$(E2E_KUBECTL) create secret generic opensearch-admin-credentials \
		-n $(E2E_OP_NS) \
		--from-literal=username="admin" \
		--from-literal=password="ThisIsTheOpenSearchPassword1" \
		--dry-run=client -o yaml | $(E2E_KUBECTL) apply -f -
	@$(call log_info, Installing logs module without Fluent Bit so index templates are ready first)
	$(E2E_HELM) upgrade --install observability-logs-opensearch \
		oci://ghcr.io/openchoreo/helm-charts/observability-logs-opensearch \
		--version $(OBSERVABILITY_LOGS_OPENSEARCH_VERSION) \
		--namespace $(E2E_OP_NS) \
		--set openSearchSetup.openSearchSecretName="opensearch-admin-credentials" \
		--set adapter.openSearchSecretName="opensearch-admin-credentials" \
		--set fluent-bit.enabled=false \
		--wait --wait-for-jobs --timeout $(E2E_SETUP_TIMEOUT)
	@$(call log_info, Enabling Fluent Bit after logs module setup)
	$(E2E_HELM) upgrade --install observability-logs-opensearch \
		oci://ghcr.io/openchoreo/helm-charts/observability-logs-opensearch \
		--version $(OBSERVABILITY_LOGS_OPENSEARCH_VERSION) \
		--namespace $(E2E_OP_NS) \
		--set openSearchSetup.openSearchSecretName="opensearch-admin-credentials" \
		--set adapter.openSearchSecretName="opensearch-admin-credentials" \
		--set fluent-bit.enabled=true \
		--wait --wait-for-jobs --timeout $(E2E_SETUP_TIMEOUT)
	$(E2E_HELM) upgrade --install observability-traces-opensearch \
		oci://ghcr.io/openchoreo/helm-charts/observability-tracing-opensearch \
		--version $(OBSERVABILITY_TRACES_OPENSEARCH_VERSION) \
		--namespace $(E2E_OP_NS) \
		--set openSearch.enabled=false \
		--set openSearchSetup.openSearchSecretName="opensearch-admin-credentials" \
		--wait --wait-for-jobs --timeout $(E2E_SETUP_TIMEOUT)
	$(E2E_HELM) upgrade --install observability-metrics-prometheus \
		oci://ghcr.io/openchoreo/helm-charts/observability-metrics-prometheus \
		--version $(OBSERVABILITY_METRICS_PROMETHEUS_VERSION) \
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
	@$(call log_info, Registering ClusterDataPlane)
	$(call e2e_register_plane,$(E2E_DP_NS),$(E2E_K3D_DIR)/clusterdataplane.yaml)
	@$(call log_info, Creating internal gateway)
	$(E2E_KUBECTL) apply -f $(E2E_K3D_DIR)/internal-gateway.yaml

.PHONY: _e2e.configure-wp
_e2e.configure-wp:
	@$(call log_info, Registering WorkflowPlane)
	$(call e2e_register_plane,$(E2E_WP_NS),$(E2E_K3D_DIR)/workflowplane.yaml)
	@$(call log_info, Registering ClusterWorkflowPlane)
	$(call e2e_register_plane,$(E2E_WP_NS),$(E2E_K3D_DIR)/clusterworkflowplane.yaml)
	@$(call log_info, Applying ClusterWorkflowTemplates used by the builder workflows)
	$(E2E_KUBECTL) apply -f $(PROJECT_DIR)/samples/getting-started/workflow-templates/checkout-source.yaml
	$(E2E_KUBECTL) apply -f $(PROJECT_DIR)/samples/getting-started/workflow-templates.yaml
	@# Apply the e2e-specific publish-image template instead of the
	@# shared sample (samples/.../publish-image-k3d.yaml hardcodes
	@# host.k3d.internal:10082, which collides with a parallel
	@# single-cluster install; e2e uses port 20082 — see config.yaml).
	$(E2E_KUBECTL) apply -f $(E2E_K3D_DIR)/workflow-templates/publish-image-e2e.yaml
	@# Same reason as publish-image-e2e — the sample template hardcodes
	@# host.k3d.internal:8080 for the CP gateway (OAuth + API). e2e uses 28080.
	$(E2E_KUBECTL) apply -f $(E2E_K3D_DIR)/workflow-templates/generate-workload-e2e.yaml

.PHONY: _e2e.configure-op
_e2e.configure-op:
	@$(call log_info, Registering ObservabilityPlane)
	$(call e2e_register_plane,$(E2E_OP_NS),$(E2E_K3D_DIR)/observabilityplane.yaml)
	@$(call log_info, Registering ClusterObservabilityPlane)
	$(call e2e_register_plane,$(E2E_OP_NS),$(E2E_K3D_DIR)/clusterobservabilityplane.yaml)

.PHONY: _e2e.link-observability
_e2e.link-observability:
	@$(call log_info, Linking ObservabilityPlane to other planes)
	@# The e2e setup uses two ClusterDataPlane CRs (`default` and
	@# `e2e-shared` — the latter is what the per-suite Environment
	@# fixtures point at). Patch both so the observability-alert-rule
	@# trait's `${has(dataplane.observabilityPlaneRef)}` CEL guard
	@# resolves true in either path.
	$(E2E_KUBECTL) patch clusterdataplane default --type merge \
		-p '{"spec":{"observabilityPlaneRef":{"kind":"ClusterObservabilityPlane","name":"default"}}}'
	$(E2E_KUBECTL) patch clusterdataplane e2e-shared --type merge \
		-p '{"spec":{"observabilityPlaneRef":{"kind":"ClusterObservabilityPlane","name":"default"}}}'
	@if [ "$(E2E_WITH_BUILD)" = "true" ]; then \
		$(E2E_KUBECTL) patch workflowplane default -n default --type merge \
			-p '{"spec":{"observabilityPlaneRef":{"kind":"ObservabilityPlane","name":"default"}}}'; \
	fi

# ---------------------------------------------------------------------------
# Test target
# ---------------------------------------------------------------------------

.PHONY: e2e.test
e2e.test: ## Run e2e test suite (set E2E_LABEL_FILTER to scope by tier)
	@$(call log_info, Running e2e tests$(if $(E2E_GINKGO_LABEL_FLAG), with label filter '$(E2E_LABEL_FILTER)'))
	go test $(E2E_DIR)/ -v -ginkgo.v -timeout $(E2E_TEST_TIMEOUT) \
		--e2e.kubecontext=$(E2E_KUBECONTEXT) $(E2E_GINKGO_LABEL_FLAG)
	go test $(E2E_DIR)/suites/... -v -ginkgo.v -timeout $(E2E_TEST_TIMEOUT) \
		--e2e.kubecontext=$(E2E_KUBECONTEXT) $(E2E_GINKGO_LABEL_FLAG)

# ---------------------------------------------------------------------------
# Utility targets
# ---------------------------------------------------------------------------

.PHONY: e2e.status
e2e.status: ## Check status of all planes and agent connections
	@echo "=== Pods ==="
	@$(E2E_KUBECTL) get pods -A
	@echo ""
	@echo "=== Plane Resources ==="
	@$(E2E_KUBECTL) get clusterdataplane,workflowplane,observabilityplane -n default 2>/dev/null || true
	@echo ""
	@echo "=== Agent Connections ==="
	@for ns in $(E2E_DP_NS) $(E2E_WP_NS) $(E2E_OP_NS); do \
		echo "--- $$ns ---"; \
		$(E2E_KUBECTL) logs -n $$ns -l app=cluster-agent --tail=3 2>/dev/null || echo "(no agent)"; \
	done

.PHONY: e2e.diagnostics
e2e.diagnostics: ## Collect logs, events, and resource dumps from all namespaces
	@$(call log_info, Collecting diagnostics to $(E2E_DIAGNOSTICS_DIR))
	@mkdir -p $(E2E_DIAGNOSTICS_DIR)
	@for ns in $(E2E_CP_NS) $(E2E_DP_NS) $(E2E_WP_NS) $(E2E_OP_NS) default; do \
		$(E2E_KUBECTL) get pods -n $$ns -o wide > $(E2E_DIAGNOSTICS_DIR)/pods-$$ns.txt 2>&1 || true; \
		$(E2E_KUBECTL) get events -n $$ns --sort-by=.lastTimestamp > $(E2E_DIAGNOSTICS_DIR)/events-$$ns.txt 2>&1 || true; \
		for pod in $$($(E2E_KUBECTL) get pods -n $$ns -o jsonpath='{.items[*].metadata.name}' 2>/dev/null); do \
			$(E2E_KUBECTL) logs $$pod -n $$ns --all-containers --tail=200 > $(E2E_DIAGNOSTICS_DIR)/logs-$$ns-$$pod.txt 2>&1 || true; \
		done; \
	done
	@$(E2E_KUBECTL) get clusterdataplane,workflowplane,observabilityplane -n default -o yaml > $(E2E_DIAGNOSTICS_DIR)/plane-resources.yaml 2>&1 || true
	@$(E2E_KUBECTL) get component,componentrelease,releasebinding,renderedrelease -A -o yaml > $(E2E_DIAGNOSTICS_DIR)/release-chain.yaml 2>&1 || true
	@$(call log_success, Diagnostics collected to $(E2E_DIAGNOSTICS_DIR))

.PHONY: e2e.down
e2e.down: ## Delete k3d cluster
	@$(call log_info, Deleting k3d cluster '$(E2E_CLUSTER_NAME)')
	k3d cluster delete $(E2E_CLUSTER_NAME)
	@$(call log_success, k3d cluster '$(E2E_CLUSTER_NAME)' deleted)
