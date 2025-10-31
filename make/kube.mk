# This makefile contains all the make targets related Kubernetes development.

KUBE_DEV_DEPLOY_NAMESPACE ?= choreo-system

##@ Kubernetes Development

.PHONY: manifests
manifests: controller-gen ## Generate WebhookConfiguration, ClusterRole and CustomResourceDefinition objects.
	$(CONTROLLER_GEN) rbac:roleName=manager-role crd:generateEmbeddedObjectMeta=true webhook paths="./api/...;./internal/controller/..." output:crd:artifacts:config=config/crd/bases

.PHONY: generate
generate: controller-gen ## Generate code containing DeepCopy, DeepCopyInto, and DeepCopyObject method implementations.
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="./api/..."

.PHONY: install
install: manifests kustomize ## Install CRDs into the K8s cluster specified in ~/.kube/config.
	$(KUSTOMIZE) build config/crd | $(KUBECTL) apply -f -

# TODO(user): To use a different vendor for e2e tests, modify the setup under 'tests/e2e'.
# The default setup assumes Kind is pre-installed and builds/loads the Manager Docker image locally.
# Prometheus and CertManager are installed by default; skip with:
# - PROMETHEUS_INSTALL_SKIP=true
# - CERT_MANAGER_INSTALL_SKIP=true
.PHONY: test-e2e
test-e2e: manifests generate fmt vet ## Run the e2e tests. Expected an isolated environment using Kind.
	@command -v kind >/dev/null 2>&1 || { \
		echo "Kind is not installed. Please install Kind manually."; \
		exit 1; \
	}
	@kind get clusters | grep -q 'kind' || { \
		echo "No Kind cluster is running. Please start a Kind cluster before running the e2e tests."; \
		exit 1; \
	}
	go test ./test/e2e/ -v -ginkgo.v

.PHONY: dev-deploy
dev-deploy: ## Deploy the Choreo developer version to a Kind cluster configured in ~/.kube/config (Single Cluster Mode)
	@$(MAKE) helm-package
	helm upgrade --install cilium $(HELM_CHARTS_OUTPUT_DIR)/cilium-$(HELM_CHART_VERSION).tgz \
		--namespace "$(KUBE_DEV_DEPLOY_NAMESPACE)" --create-namespace --timeout 30m
	helm upgrade --install choreo-control-plane $(HELM_CHARTS_OUTPUT_DIR)/openchoreo-control-plane-$(HELM_CHART_VERSION).tgz \
    	--namespace "$(KUBE_DEV_DEPLOY_NAMESPACE)" --create-namespace --timeout 30m --set cert-manager.enabled=false

	helm upgrade --install choreo-dataplane $(HELM_CHARTS_OUTPUT_DIR)/openchoreo-data-plane-$(HELM_CHART_VERSION).tgz \
		--namespace "$(KUBE_DEV_DEPLOY_NAMESPACE)" --create-namespace --timeout 30m --set cert-manager.enabled=false

.PHONY: dev-undeploy
dev-undeploy: ## Undeploy the Choreo developer version from a Kind cluster configured in ~/.kube/config
	helm uninstall cilium --namespace "$(KUBE_DEV_DEPLOY_NAMESPACE)"
	helm uninstall choreo-control-plane --namespace "$(KUBE_DEV_DEPLOY_NAMESPACE)"
	helm uninstall choreo-dataplane --namespace "$(KUBE_DEV_DEPLOY_NAMESPACE)"
