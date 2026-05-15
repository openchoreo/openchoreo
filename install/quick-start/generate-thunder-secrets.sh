#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/.config.sh"

log_info "Generating random Thunder secrets..."

# Verify required tools are available
for cmd in kubectl openssl base64; do
    if ! command -v "$cmd" >/dev/null; then
        log_error "$cmd required but not found"
        exit 1
    fi
done

# ---------------------------------------------------------------------------
# Read existing secret if present — reuse values, only fill in missing keys
# ---------------------------------------------------------------------------
base64_decode() {
    openssl base64 -d -A 2>/dev/null || base64 -d 2>/dev/null || true
}

read_existing_secret() {
    local key="$1" default="$2"
    local value
    value=$( (kubectl get secret openchoreo-initial-credentials -n thunder \
        -o jsonpath="{.data[\"${key}\"]}" 2>/dev/null || true) | base64_decode)
    if [ -n "$value" ]; then
        echo "$value"
    else
        echo "$default"
    fi
}

if kubectl get secret openchoreo-initial-credentials -n thunder >/dev/null 2>&1; then
    log_info "Existing openchoreo-initial-credentials found, reusing values..."
fi

# ---------------------------------------------------------------------------
# Helper — write a key-value pair to OpenBao
# NOTE: BAO_TOKEN=root is the current dev-mode default. Issue #3 will
# randomize it and store the token in the openbao-root-token K8s secret.
# When that change lands, update this function to read from that secret.
# ---------------------------------------------------------------------------
openbao_write() {
    local path="$1" value="$2"
    local bao_token="${BAO_TOKEN:-root}"
    echo -n "$value" | kubectl exec -i -n openbao openbao-0 -- sh -c "
        export BAO_ADDR=http://127.0.0.1:8200
        export BAO_TOKEN=${bao_token}
        bao kv put secret/${path} value=\"\$(cat -)\"
    " >/dev/null || log_warning "Failed to write ${path} to OpenBao"
}

# ---------------------------------------------------------------------------
# Section: Thunder — platform user passwords
# ---------------------------------------------------------------------------
ADMIN_PASSWORD=$(read_existing_secret "admin-password" "$(openssl rand -base64 16)")
DEVELOPER_PASSWORD=$(read_existing_secret "developer-password" "$(openssl rand -base64 16)")
PE_PASSWORD=$(read_existing_secret "pe-password" "$(openssl rand -base64 16)")
SRE_PASSWORD=$(read_existing_secret "sre-password" "$(openssl rand -base64 16)")

# ---------------------------------------------------------------------------
# Section: Thunder — OAuth2 client secrets
# ---------------------------------------------------------------------------
BACKSTAGE_CLIENT_SECRET=$(read_existing_secret "backstage-client-secret" "$(openssl rand -hex 32)")
CUSTOMER_PORTAL_CLIENT_SECRET=$(read_existing_secret "customer-portal-client-secret" "$(openssl rand -hex 32)")
RCA_CLIENT_SECRET=$(read_existing_secret "rca-client-secret" "$(openssl rand -hex 32)")
SYSTEM_APP_CLIENT_SECRET=$(read_existing_secret "system-app-client-secret" "$(openssl rand -hex 32)")
SERVICE_MCP_CLIENT_SECRET=$(read_existing_secret "service-mcp-client-secret" "$(openssl rand -hex 32)")
WORKLOAD_PUBLISHER_CLIENT_SECRET=$(read_existing_secret "workload-publisher-client-secret" "$(openssl rand -hex 32)")
OBSERVER_CLIENT_SECRET=$(read_existing_secret "observer-client-secret" "$(openssl rand -hex 32)")
FINOPS_AGENT_CLIENT_SECRET=$(read_existing_secret "finops-agent-client-secret" "$(openssl rand -hex 32)")

# ---------------------------------------------------------------------------
# Ensure the thunder namespace exists
# ---------------------------------------------------------------------------
kubectl create namespace thunder --dry-run=client -o yaml | kubectl apply -f - >/dev/null

# ---------------------------------------------------------------------------
# Create the shared K8s secret in the thunder namespace
# ---------------------------------------------------------------------------
log_info "Creating openchoreo-initial-credentials secret in thunder namespace..."

kubectl create secret generic openchoreo-initial-credentials \
  --namespace thunder \
  --from-literal=admin-password="${ADMIN_PASSWORD}" \
  --from-literal=developer-password="${DEVELOPER_PASSWORD}" \
  --from-literal=pe-password="${PE_PASSWORD}" \
  --from-literal=sre-password="${SRE_PASSWORD}" \
  --from-literal=backstage-client-secret="${BACKSTAGE_CLIENT_SECRET}" \
  --from-literal=customer-portal-client-secret="${CUSTOMER_PORTAL_CLIENT_SECRET}" \
  --from-literal=rca-client-secret="${RCA_CLIENT_SECRET}" \
  --from-literal=system-app-client-secret="${SYSTEM_APP_CLIENT_SECRET}" \
  --from-literal=service-mcp-client-secret="${SERVICE_MCP_CLIENT_SECRET}" \
  --from-literal=workload-publisher-client-secret="${WORKLOAD_PUBLISHER_CLIENT_SECRET}" \
  --from-literal=observer-client-secret="${OBSERVER_CLIENT_SECRET}" \
  --from-literal=finops-agent-client-secret="${FINOPS_AGENT_CLIENT_SECRET}" \
  -o yaml --dry-run=client | kubectl apply --server-side -f - >/dev/null

log_success "openchoreo-initial-credentials secret created in thunder namespace"

# ---------------------------------------------------------------------------
# Write to OpenBao for ExternalSecret-backed installs
# ---------------------------------------------------------------------------
log_info "Writing secrets to OpenBao..."

openbao_write "backstage-client-secret" "${BACKSTAGE_CLIENT_SECRET}"
openbao_write "observer-oauth-client-secret" "${OBSERVER_CLIENT_SECRET}"
openbao_write "rca-oauth-client-secret" "${RCA_CLIENT_SECRET}"
openbao_write "finops-agent-oauth-client-secret" "${FINOPS_AGENT_CLIENT_SECRET}"

log_success "Secrets written to OpenBao"
log_success "Thunder secrets generated successfully"
