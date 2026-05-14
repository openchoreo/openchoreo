#!/usr/bin/env bash
set -euo pipefail

# Installs the OpenChoreo observability plane and default modules into
# the current k3d cluster.
#
# Designed to work with curl | bash:
#   curl -sL https://raw.githubusercontent.com/openchoreo/openchoreo/main/install/k3d/k3d-observability-plane.sh | bash
#
# Or run from a local checkout:
#   install/k3d/k3d-observability-plane.sh

# -- versions (update these on release branches) --
OPENCHOREO_REF="${OPENCHOREO_REF:-main}"           # overridable via env; defaults to main
OPENCHOREO_OP_VERSION="${OPENCHOREO_OP_VERSION:-0.0.0-latest-dev}"  # overridable via env
LOGS_OPENSEARCH_VERSION="0.4.0"
TRACES_OPENSEARCH_VERSION="0.4.1"
METRICS_PROMETHEUS_VERSION="0.5.1"

# -- derived constants --
RAW_BASE="https://raw.githubusercontent.com/openchoreo/openchoreo/${OPENCHOREO_REF}"
OP_NS="openchoreo-observability-plane"

step() {
  echo ""
  echo "==> $1"
}

step "Installing observability plane core services..."
helm upgrade --install openchoreo-observability-plane oci://ghcr.io/openchoreo/helm-charts/openchoreo-observability-plane \
  --version "$OPENCHOREO_OP_VERSION" \
  --namespace "$OP_NS" \
  --values "${RAW_BASE}/install/k3d/single-cluster/values-op.yaml" \
  --timeout 25m

step "Installing OpenSearch-based logs module..."
helm upgrade --install observability-logs-opensearch \
  oci://ghcr.io/openchoreo/helm-charts/observability-logs-opensearch \
  --create-namespace \
  --namespace "$OP_NS" \
  --version "$LOGS_OPENSEARCH_VERSION" \
  --set openSearchSetup.openSearchSecretName="opensearch-admin-credentials" \
  --set adapter.openSearchSecretName="opensearch-admin-credentials"

step "Installing OpenSearch-based traces module..."
helm upgrade --install observability-traces-opensearch \
  oci://ghcr.io/openchoreo/helm-charts/observability-tracing-opensearch \
  --create-namespace \
  --namespace "$OP_NS" \
  --version "$TRACES_OPENSEARCH_VERSION" \
  --set openSearch.enabled=false \
  --set openSearchSetup.openSearchSecretName="opensearch-admin-credentials"

step "Installing Prometheus-based metrics module..."
helm upgrade --install observability-metrics-prometheus \
  oci://ghcr.io/openchoreo/helm-charts/observability-metrics-prometheus \
  --create-namespace \
  --namespace "$OP_NS" \
  --version "$METRICS_PROMETHEUS_VERSION"

step "Enabling logs collection in the configured logs module..."
helm upgrade observability-logs-opensearch \
  oci://ghcr.io/openchoreo/helm-charts/observability-logs-opensearch \
  --namespace "$OP_NS" \
  --version "$LOGS_OPENSEARCH_VERSION" \
  --reuse-values \
  --set fluent-bit.enabled=true

echo ""
echo "==> Observability plane and default modules installed successfully."
