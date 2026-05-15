#!/usr/bin/env bash
# Shared configuration for OpenChoreo quick-start
# This file is sourced by both interactive shells (.bashrc) and installation scripts
# to maintain a single source of truth for configuration values

# Cluster configuration
CLUSTER_NAME="openchoreo-quick-start"
KUBECONFIG_PATH="$HOME/.kube/config"

# Namespace definitions
CONTROL_PLANE_NS="openchoreo-control-plane"
DATA_PLANE_NS="openchoreo-data-plane"
WORKFLOW_PLANE_NS="openchoreo-workflow-plane"
OBSERVABILITY_NS="openchoreo-observability-plane"
THUNDER_NS="thunder"

# Helm repository
HELM_REPO="oci://ghcr.io/openchoreo/helm-charts"

# Cert-manager configuration
CERT_MANAGER_VERSION="v1.19.4"
CERT_MANAGER_REPO="oci://quay.io/jetstack/charts"

# External Secrets Operator configuration
ESO_VERSION="v2.0.1"
ESO_REPO="oci://ghcr.io/external-secrets/charts"

# kgateway configuration
KGATEWAY_VERSION="v2.2.1"

# Thunder configuration
THUNDER_VERSION="0.28.0"

# Observability module versions (community-modules)
LOGS_OPENSEARCH_VERSION="0.4.1"
TRACES_OPENSEARCH_VERSION="0.4.1"
METRICS_PROMETHEUS_VERSION="0.6.0"
