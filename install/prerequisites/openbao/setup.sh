#!/usr/bin/env bash
set -eo pipefail

# Standalone OpenBao setup for OpenChoreo.
# Installs OpenBao into the "openbao" namespace and creates a ClusterSecretStore
# named "default" backed by the Vault provider.
#
# Usage:
#   ./setup.sh [--dev] [--seed-dev-secrets] [--kube-context CONTEXT]
#
# Options:
#   --dev               Enable dev mode (in-memory, auto-unsealed, root token = "root")
#   --seed-dev-secrets  Write placeholder secrets for local development
#   --kube-context      kubectl/helm context to use

OPENBAO_NAMESPACE="openbao"
OPENBAO_CHART_VERSION="0.25.6"
OPENBAO_IMAGE_TAG="2.4.4"
CLUSTER_SECRET_STORE_NAME="default"
DEV_MODE=false
SEED_DEV_SECRETS=false
KUBE_CONTEXT_FLAG=""
DEV_ROOT_TOKEN="root"

while [[ $# -gt 0 ]]; do
    case $1 in
        --dev)
            DEV_MODE=true
            shift
            ;;
        --seed-dev-secrets)
            SEED_DEV_SECRETS=true
            shift
            ;;
        --kube-context)
            KUBE_CONTEXT_FLAG="--kube-context $2"
            shift 2
            ;;
        *)
            echo "Unknown option: $1"
            exit 1
            ;;
    esac
done

echo "Installing OpenBao into namespace ${OPENBAO_NAMESPACE}..."

DEV_FLAGS=""
if [[ "$DEV_MODE" == "true" ]]; then
    DEV_FLAGS="--set server.dev.enabled=true --set server.dev.devRootToken=${DEV_ROOT_TOKEN}"
fi

# shellcheck disable=SC2086
helm upgrade --install openbao oci://ghcr.io/openbao/charts/openbao \
    ${KUBE_CONTEXT_FLAG} \
    --namespace "${OPENBAO_NAMESPACE}" \
    --create-namespace \
    --version "${OPENBAO_CHART_VERSION}" \
    --set server.image.tag="${OPENBAO_IMAGE_TAG}" \
    --set injector.enabled=false \
    --set server.resources.requests.memory=64Mi \
    --set server.resources.requests.cpu=50m \
    --set server.resources.limits.memory=128Mi \
    --set server.resources.limits.cpu=100m \
    ${DEV_FLAGS}

echo "Waiting for OpenBao to be ready..."
# shellcheck disable=SC2086
kubectl ${KUBE_CONTEXT_FLAG} wait --namespace "${OPENBAO_NAMESPACE}" \
    --for=condition=Ready pods \
    -l app.kubernetes.io/name=openbao,component=server \
    --timeout=300s

echo "Configuring OpenBao..."
# shellcheck disable=SC2086
kubectl ${KUBE_CONTEXT_FLAG} exec -n "${OPENBAO_NAMESPACE}" openbao-0 -- sh -c "
    export BAO_ADDR=http://127.0.0.1:8200
    export BAO_TOKEN=${DEV_ROOT_TOKEN}

    # Enable Kubernetes authentication
    bao auth enable kubernetes 2>/dev/null || true

    # Configure Kubernetes authentication
    bao write auth/kubernetes/config \
        kubernetes_host=\"https://\${KUBERNETES_PORT_443_TCP_ADDR}:443\"

    # Reader policy (data plane)
    bao policy write openchoreo-secret-reader-policy - <<'POLICY'
path \"secret/data/*\" {
  capabilities = [\"read\"]
}
path \"secret/metadata/*\" {
  capabilities = [\"list\", \"read\"]
}
POLICY

    # Writer policy (build plane / ESO PushSecrets)
    bao policy write openchoreo-secret-writer-policy - <<'POLICY'
path \"secret/data/*\" {
  capabilities = [\"create\", \"read\", \"update\", \"delete\"]
}
path \"secret/metadata/*\" {
  capabilities = [\"create\", \"read\", \"update\", \"delete\", \"list\"]
}
POLICY

    # Reader role for data plane namespaces
    bao write auth/kubernetes/role/openchoreo-secret-reader-role \
        bound_service_account_names=default \
        bound_service_account_namespaces='dp*' \
        policies=openchoreo-secret-reader-policy \
        ttl=20m

    # Writer role for build plane and openbao namespace
    bao write auth/kubernetes/role/openchoreo-secret-writer-role \
        bound_service_account_names='*' \
        bound_service_account_namespaces='${OPENBAO_NAMESPACE},openchoreo-build-plane' \
        policies=openchoreo-secret-writer-policy \
        ttl=20m
"

echo "Creating ServiceAccount for ESO..."
# shellcheck disable=SC2086
kubectl ${KUBE_CONTEXT_FLAG} apply -f - <<EOF
apiVersion: v1
kind: ServiceAccount
metadata:
  name: external-secrets-openbao
  namespace: ${OPENBAO_NAMESPACE}
EOF

echo "Creating ClusterSecretStore '${CLUSTER_SECRET_STORE_NAME}'..."
# shellcheck disable=SC2086
kubectl ${KUBE_CONTEXT_FLAG} apply -f - <<EOF
apiVersion: external-secrets.io/v1
kind: ClusterSecretStore
metadata:
  name: ${CLUSTER_SECRET_STORE_NAME}
spec:
  provider:
    vault:
      server: "http://openbao.${OPENBAO_NAMESPACE}.svc:8200"
      path: "secret"
      version: "v2"
      auth:
        kubernetes:
          mountPath: "kubernetes"
          role: "openchoreo-secret-writer-role"
          serviceAccountRef:
            name: "external-secrets-openbao"
            namespace: "${OPENBAO_NAMESPACE}"
EOF

if [[ "$SEED_DEV_SECRETS" == "true" ]]; then
    echo "Seeding development placeholder secrets..."
    # shellcheck disable=SC2086
    kubectl ${KUBE_CONTEXT_FLAG} exec -n "${OPENBAO_NAMESPACE}" openbao-0 -- sh -c "
        export BAO_ADDR=http://127.0.0.1:8200
        export BAO_TOKEN=${DEV_ROOT_TOKEN}

        bao kv put secret/backstage-backend-secret value='local-dev-backend-secret'
        bao kv put secret/backstage-client-secret value='backstage-portal-secret'
        bao kv put secret/backstage-jenkins-api-key value='placeholder-not-in-use'
        bao kv put secret/opensearch-username value='admin'
        bao kv put secret/opensearch-password value='ThisIsTheOpenSearchPassword1'
    "
    echo "Development secrets seeded."
fi

echo "OpenBao setup complete."
echo "  Namespace:          ${OPENBAO_NAMESPACE}"
echo "  ClusterSecretStore: ${CLUSTER_SECRET_STORE_NAME}"
echo "  Dev mode:           ${DEV_MODE}"
echo "  Dev secrets seeded: ${SEED_DEV_SECRETS}"
