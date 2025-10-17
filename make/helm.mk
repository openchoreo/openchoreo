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
	@# Run helm-gen to generate CRDs and RBAC for the helm chart
	@if [ ${CHART_NAME} == "openchoreo-control-plane" ]; then \
		$(MAKE) manifests; \
		$(KUBEBUILDER_HELM_GEN) -config-dir $(PROJECT_DIR)/config -chart-dir $(CHART_PATH); \
		VALUES_FILE=$(CHART_PATH)/values.yaml; \
		if [ -f "$$VALUES_FILE" ]; then \
		  $(YQ) eval '.controllerManager.manager.image.repository = "$(HELM_CONTROLLER_IMAGE)"' -i $$VALUES_FILE; \
		  $(YQ) eval '.controllerManager.manager.image.tag = "$(TAG)"' -i $$VALUES_FILE; \
		  $(YQ) eval '.controllerManager.manager.imagePullPolicy = "$(HELM_CONTROLLER_IMAGE_PULL_POLICY)"' -i $$VALUES_FILE; \
		fi \
	fi
	@# Update backstage image tag for openchoreo-backstage chart
	@if [ ${CHART_NAME} == "openchoreo-backstage" ]; then \
		VALUES_FILE=$(CHART_PATH)/values.yaml; \
		if [ -f "$$VALUES_FILE" ]; then \
		  $(YQ) eval '.backstage.backstage.image.tag = "$(TAG)"' -i $$VALUES_FILE; \
		fi \
	fi
	@# Copy CRDs and RBAC to openchoreo-secure-core chart
	@if [ ${CHART_NAME} == "openchoreo-secure-core" ]; then \
		$(call log_info, Generating resources for secure-core chart); \
		$(MAKE) manifests; \
		mkdir -p $(CHART_PATH)/crds; \
		mkdir -p $(CHART_PATH)/templates/controller-manager; \
		$(call log_info, Copying CRDs); \
		cp -f $(PROJECT_DIR)/config/crd/bases/*.yaml $(CHART_PATH)/crds/; \
		$(call log_info, Generating RBAC templates from kubebuilder output); \
		( \
			awk 'BEGIN {in_metadata=0; skip_labels=0} \
			/^---$$/ {print; next} \
			/^apiVersion:/ {print; next} \
			/^kind:/ {print; next} \
			/^metadata:$$/ {print; in_metadata=1; next} \
			in_metadata && /^  labels:$$/ {skip_labels=1; next} \
			skip_labels && /^    / {next} \
			skip_labels && /^  / {skip_labels=0} \
			in_metadata && /^  name: manager-role$$/ {print "  name: {{ include \"openchoreo-secure-core.fullname\" . }}-controller-manager-role"; next} \
			in_metadata && /^[^ ]/ {print "  labels:"; print "    app.kubernetes.io/component: controller-manager"; print "    app.kubernetes.io/part-of: openchoreo"; print "    app.kubernetes.io/name: {{ include \"openchoreo-secure-core.fullname\" . }}"; print "    app.kubernetes.io/instance: {{ .Release.Name }}"; in_metadata=0; print; next} \
			{print}' $(PROJECT_DIR)/config/rbac/role.yaml; \
		) > $(CHART_PATH)/templates/controller-manager/clusterrole.yaml; \
		$(call log_info, Copied $(shell ls -1 $(PROJECT_DIR)/config/crd/bases/*.yaml | wc -l) CRDs and generated RBAC templates); \
		VALUES_FILE=$(CHART_PATH)/values.yaml; \
		if [ -f "$$VALUES_FILE" ]; then \
		  $(YQ) eval '.controllerManager.image.repository = "$(HELM_CONTROLLER_IMAGE)"' -i $$VALUES_FILE; \
		  $(YQ) eval '.controllerManager.image.tag = "$(TAG)"' -i $$VALUES_FILE; \
		  $(YQ) eval '.controllerManager.image.pullPolicy = "$(HELM_CONTROLLER_IMAGE_PULL_POLICY)"' -i $$VALUES_FILE; \
		fi \
	fi
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
