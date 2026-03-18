#!/usr/bin/env bash
set -euo pipefail

# Installs all OpenChoreo prerequisites into the current k3d cluster:
# Gateway API CRDs, cert-manager, External Secrets Operator, kgateway,
# OpenBao (with ClusterSecretStore), and CoreDNS rewrite rules.
#
# Versions are sourced from install/quick-start/.config.sh to stay in sync
# with the rest of the install tooling.
#
# Usage:
#   install/k3d/k3d-prerequisites.sh

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

# shellcheck source=../quick-start/.config.sh
source "$REPO_ROOT/install/quick-start/.config.sh"

GATEWAY_API_VERSION="v1.4.1"

step() {
    echo ""
    echo "==> $1"
}

step "Installing Gateway API CRDs ($GATEWAY_API_VERSION)..."
kubectl apply --server-side \
    -f "https://github.com/kubernetes-sigs/gateway-api/releases/download/${GATEWAY_API_VERSION}/experimental-install.yaml"

step "Installing cert-manager ($CERT_MANAGER_VERSION)..."
helm upgrade --install cert-manager "$CERT_MANAGER_REPO/cert-manager" \
    --namespace cert-manager \
    --create-namespace \
    --version "$CERT_MANAGER_VERSION" \
    --set crds.enabled=true \
    --wait --timeout 180s

step "Installing External Secrets Operator ($ESO_VERSION)..."
helm upgrade --install external-secrets "$ESO_REPO/external-secrets" \
    --namespace external-secrets \
    --create-namespace \
    --version "$ESO_VERSION" \
    --set installCRDs=true \
    --wait --timeout 180s

step "Installing kgateway CRDs ($KGATEWAY_VERSION)..."
helm upgrade --install kgateway-crds oci://cr.kgateway.dev/kgateway-dev/charts/kgateway-crds \
    --create-namespace --namespace "$CONTROL_PLANE_NS" \
    --version "$KGATEWAY_VERSION"

step "Installing kgateway ($KGATEWAY_VERSION)..."
helm upgrade --install kgateway oci://cr.kgateway.dev/kgateway-dev/charts/kgateway \
    --namespace "$CONTROL_PLANE_NS" --create-namespace \
    --version "$KGATEWAY_VERSION" \
    --set controller.extraEnv.KGW_ENABLE_GATEWAY_API_EXPERIMENTAL_FEATURES=true

step "Installing OpenBao and ClusterSecretStore..."
"$REPO_ROOT/install/prerequisites/openbao/setup.sh" --dev --seed-dev-secrets

step "Configuring CoreDNS rewrite..."
kubectl apply -f "$REPO_ROOT/install/k3d/common/coredns-custom.yaml"

echo ""
echo "==> All prerequisites installed successfully."
