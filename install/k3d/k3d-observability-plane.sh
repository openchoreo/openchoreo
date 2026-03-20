#!/usr/bin/env bash
set -euo pipefail

VERSION=$1
GITHUB_REF=${2:-main}

step() {
  echo ""
  echo "==> $1"
}

step "Installing observability plane core services..."
helm upgrade --install openchoreo-observability-plane oci://ghcr.io/openchoreo/helm-charts/openchoreo-observability-plane \
  --version $VERSION \
  --namespace openchoreo-observability-plane \
  --values "https://raw.githubusercontent.com/openchoreo/openchoreo/${GITHUB_REF}/install/k3d/single-cluster/values-op.yaml" \
  --timeout 25m

step "Installing OpenSearch-based logs module..."
helm upgrade --install observability-logs-opensearch \
  oci://ghcr.io/openchoreo/helm-charts/observability-logs-opensearch \
  --create-namespace \
  --namespace openchoreo-observability-plane \
  --version 0.3.11 \
  --set openSearchSetup.openSearchSecretName="opensearch-admin-credentials"

step "Installing OpenSearch-based traces module..."
helm upgrade --install observability-traces-opensearch \
  oci://ghcr.io/openchoreo/helm-charts/observability-tracing-opensearch \
  --create-namespace \
  --namespace openchoreo-observability-plane \
  --version 0.3.10 \
  --set openSearch.enabled=false \
  --set openSearchSetup.openSearchSecretName="opensearch-admin-credentials"

step "Installing Prometheus-based metrics module..."
helm upgrade --install observability-metrics-prometheus \
  oci://ghcr.io/openchoreo/helm-charts/observability-metrics-prometheus \
  --create-namespace \
  --namespace openchoreo-observability-plane \
  --version 0.2.5

step "Enabling logs collection in the configured logs module..."
helm upgrade observability-logs-opensearch \
  oci://ghcr.io/openchoreo/helm-charts/observability-logs-opensearch \
  --namespace openchoreo-observability-plane \
  --version 0.3.11 \
  --reuse-values \
  --set fluent-bit.enabled=true

echo ""
echo "==> Observability plane and default modules installed successfully."
