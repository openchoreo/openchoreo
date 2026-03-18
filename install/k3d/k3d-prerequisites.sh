#!/usr/bin/env bash
set -euo pipefail

# Installs all OpenChoreo prerequisites into the current k3d cluster:
# Gateway API CRDs, cert-manager, External Secrets Operator, kgateway,
# OpenBao (with ClusterSecretStore), and CoreDNS rewrite rules.
#
# Designed to work with curl | bash:
#   curl -sL https://raw.githubusercontent.com/openchoreo/openchoreo/main/install/k3d/k3d-prerequisites.sh | bash
#
# Or run from a local checkout:
#   install/k3d/k3d-prerequisites.sh

# -- versions (update these on release branches) --
OPENCHOREO_REF="main"
GATEWAY_API_VERSION="v1.4.1"
CERT_MANAGER_VERSION="v1.19.2"
ESO_VERSION="1.3.2"
KGATEWAY_VERSION="v2.2.1"
OPENBAO_CHART_VERSION="0.25.6"

# -- derived constants --
RAW_BASE="https://raw.githubusercontent.com/openchoreo/openchoreo/${OPENCHOREO_REF}"
CONTROL_PLANE_NS="openchoreo-control-plane"
OPENBAO_NS="openbao"

step() {
    echo ""
    echo "==> $1"
}

step "Installing Gateway API CRDs ($GATEWAY_API_VERSION)..."
kubectl apply --server-side \
    -f "https://github.com/kubernetes-sigs/gateway-api/releases/download/${GATEWAY_API_VERSION}/experimental-install.yaml"

step "Installing cert-manager ($CERT_MANAGER_VERSION)..."
helm upgrade --install cert-manager oci://quay.io/jetstack/charts/cert-manager \
    --namespace cert-manager \
    --create-namespace \
    --version "$CERT_MANAGER_VERSION" \
    --set crds.enabled=true \
    --wait --timeout 180s

step "Installing External Secrets Operator ($ESO_VERSION)..."
helm upgrade --install external-secrets oci://ghcr.io/external-secrets/charts/external-secrets \
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

step "Installing OpenBao ($OPENBAO_CHART_VERSION)..."
helm upgrade --install openbao oci://ghcr.io/openbao/charts/openbao \
    --namespace "$OPENBAO_NS" \
    --create-namespace \
    --version "$OPENBAO_CHART_VERSION" \
    --values "${RAW_BASE}/install/k3d/common/values-openbao.yaml" \
    --wait --timeout 300s

step "Creating ClusterSecretStore and ServiceAccount..."
kubectl apply -f - <<EOF
apiVersion: v1
kind: ServiceAccount
metadata:
  name: external-secrets-openbao
  namespace: ${OPENBAO_NS}
---
apiVersion: external-secrets.io/v1
kind: ClusterSecretStore
metadata:
  name: default
spec:
  provider:
    vault:
      server: "http://openbao.${OPENBAO_NS}.svc:8200"
      path: "secret"
      version: "v2"
      auth:
        kubernetes:
          mountPath: "kubernetes"
          role: "openchoreo-secret-writer-role"
          serviceAccountRef:
            name: "external-secrets-openbao"
            namespace: "${OPENBAO_NS}"
EOF

step "Configuring CoreDNS rewrite..."
kubectl apply -f "${RAW_BASE}/install/k3d/common/coredns-custom.yaml"

echo ""
echo "==> All prerequisites installed successfully."
