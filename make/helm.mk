# This makefile contains all the make targets related to Helm charts.

HELM_CHARTS_DIR := $(PROJECT_DIR)/install/helm
HELM_CHARTS := $(wildcard $(HELM_CHARTS_DIR)/*)
HELM_CHART_NAMES := $(foreach c,$(HELM_CHARTS),$(notdir $(c)))
HELM_CHART_VERSION ?= 0.0.0-latest-dev

HELM_CHARTS_OUTPUT_DIR := $(PROJECT_BIN_DIR)/dist/charts
HELM_OCI_REGISTRY ?= oci://ghcr.io/openchoreo/helm-charts

# Define the controller image that is used in the Choreo helm chart.
# This value should be equal to the controller image define in `DOCKER_BUILD_IMAGES` in docker.mk
HELM_CONTROLLER_IMAGE := $(IMAGE_REPO_PREFIX)/controller
HELM_CONTROLLER_IMAGE_PULL_POLICY ?= Always

# Define the CRDs to be applied only to the observability plane chart
OBSERVABILITY_PLANE_CRDS := \
	observabilityalertrules

##@ Helm

# Define the generation targets for the helm charts that are required for the helm package and push.
# Ex: make helm-generate.cilium, make helm-generate.choreo
.PHONY: helm-generate.%
helm-generate.%: yq ## Generate helm chart for the specified chart name.
	@if [ -z "$(filter $*,$(HELM_CHART_NAMES))" ]; then \
    		$(call log_error, Invalid helm generate target '$*'); \
    		exit 1; \
	fi
	$(eval CHART_NAME := $(word 1,$(subst ., ,$*)))
	$(eval CHART_PATH := $(HELM_CHARTS_DIR)/$(CHART_NAME))
	@$(call log_info, Generating helm chart '$(CHART_NAME)')
	@# Update backstage image tag for openchoreo-backstage chart
	@if [ ${CHART_NAME} == "openchoreo-backstage" ]; then \
		VALUES_FILE=$(CHART_PATH)/values.yaml; \
		if [ -f "$$VALUES_FILE" ]; then \
		  $(YQ) eval '.backstage.backstage.image.tag = "$(TAG)"' -i $$VALUES_FILE; \
		fi \
	fi
	@case ${CHART_NAME} in \
	"openchoreo-control-plane") \
		$(call log_info, Generating resources for control-plane chart); \
		$(MAKE) manifests; \
		$(call log_info, Running helm-gen for openchoreo-control-plane chart); \
		$(KUBEBUILDER_HELM_GEN) -config-dir $(PROJECT_DIR)/config -chart-dir $(CHART_PATH) -controller-subdir controller-manager; \
		$(call log_info, Removing ObservabilityPlane related CRDs and RBAC from control-plane chart); \
		for crd in $(OBSERVABILITY_PLANE_CRDS); do \
			$(call log_info, Removing $$crd CRD from control-plane chart); \
			rm -f $(CHART_PATH)/crds/openchoreo.dev_$$crd.yaml; \
			$(call log_info, Removing $$crd RBAC from control-plane chart); \
			sed -i '' "/$$crd/d" $(CHART_PATH)/templates/controller-manager/controller-manager-role.yaml; \
		done; \
		;; \
	"openchoreo-observability-plane") \
		$(call log_info, Generating CRDs for observability plane chart); \
		$(KUBEBUILDER_HELM_GEN) -config-dir $(PROJECT_DIR)/config -chart-dir $(CHART_PATH) -controller-subdir controller-manager; \
		$(call log_info, Keeping only ObservabilityPlane related CRDs); \
		for crd in $(OBSERVABILITY_PLANE_CRDS); do \
      find $(CHART_PATH)/crds -maxdepth 1 -type f ! -name openchoreo.dev_$$crd.yaml -delete; \
		done; \
		: "TODO: Automate this in future"; \
		$(call log_warning, Please remove RBAC for CRDs other than $(OBSERVABILITY_PLANE_CRDS) from the observability plane chart); \
		;; \
	esac
	helm dependency update $(CHART_PATH)
	helm lint $(CHART_PATH)



.PHONY: helm-generate
helm-generate: $(addprefix helm-generate., $(HELM_CHART_NAMES)) ## Generate all helm charts.


.PHONY: helm-package.%
helm-package.%: helm-generate.% ## Package helm chart for the specified chart name.
	@if [ -z "$(filter $*,$(HELM_CHART_NAMES))" ]; then \
    		$(call log_error, Invalid helm package target '$*'); \
    		exit 1; \
	fi
	$(eval CHART_NAME := $(word 1,$(subst ., ,$*)))
	$(eval CHART_PATH := $(HELM_CHARTS_DIR)/$(CHART_NAME))
	helm package $(CHART_PATH) --app-version ${TAG} --version ${HELM_CHART_VERSION} --destination $(HELM_CHARTS_OUTPUT_DIR)

.PHONY: helm-package
helm-package: $(addprefix helm-package., $(HELM_CHART_NAMES)) ## Package all helm charts.

.PHONY: helm-push.%
helm-push.%: helm-package.% ## Push helm chart for the specified chart name.
	@if [ -z "$(filter $*,$(HELM_CHART_NAMES))" ]; then \
    		$(call log_error, Invalid helm package target '$*'); \
    		exit 1; \
	fi
	$(eval CHART_NAME := $(word 1,$(subst ., ,$*)))
	helm push $(HELM_CHARTS_OUTPUT_DIR)/$(CHART_NAME)-$(HELM_CHART_VERSION).tgz $(HELM_OCI_REGISTRY)

.PHONY: helm-push
helm-push: $(addprefix helm-push., $(HELM_CHART_NAMES)) ## Push all helm charts.
