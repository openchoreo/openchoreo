#!/usr/bin/env bash
set -euo pipefail

# Get the absolute path of the script directory
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Source helper functions
source "${SCRIPT_DIR}/.helpers.sh"

log_info "Configuring OCC CLI login..."

# System app credentials
SYSTEM_APP_CLIENT_ID="openchoreo-system-app"
SYSTEM_APP_CLIENT_SECRET="openchoreo-system-app-secret"

# CLI app credentials
CLI_CLIENT_ID="openchoreo-cli-quickstart"
# Thunder never returns the client_secret in GET/PUT responses (only echoes
# back whatever it was given), so we generate it ourselves and send it
# explicitly in the create/update payload.
CLI_CLIENT_SECRET=$(openssl rand -base64 32 | tr '+/' '-_' | tr -d '=\n')

# Discover API server URL from HTTPRoute
API_URL=$(kubectl get httproute -n openchoreo-control-plane openchoreo-api -o jsonpath='{.spec.hostnames[0]}' 2>/dev/null || echo "")

if [ -z "$API_URL" ]; then
  log_error "Failed to discover API server URL from HTTPRoute"
  log_error "The 'openchoreo-api' HTTPRoute in namespace 'openchoreo-control-plane' is not available yet."
  log_error "The control plane may still be starting. Wait a minute and retry: bash occ-login.sh"
  exit 1
fi

CHOREO_API_ENDPOINT="http://${API_URL}:8080"

# Discover Thunder URL from HTTPRoute
THUNDER_URL=$(kubectl get httproute -n thunder thunder-httproute -o jsonpath='{.spec.hostnames[0]}' 2>/dev/null || echo "")

if [ -z "$THUNDER_URL" ]; then
  log_error "Failed to discover Thunder URL from HTTPRoute"
  log_error "The 'thunder-httproute' HTTPRoute in namespace 'thunder' is not available yet."
  log_error "Thunder may still be starting. Wait a minute and retry: bash occ-login.sh"
  exit 1
fi

THUNDER_ENDPOINT="http://${THUNDER_URL}:8080"

# Step 1: Get token using system app with scope=system
TOKEN_RESPONSE=$(curl -s --max-time 10 -X POST "${THUNDER_ENDPOINT}/oauth2/token" \
  -H "Content-Type: application/x-www-form-urlencoded" \
  -d "grant_type=client_credentials" \
  -d "client_id=${SYSTEM_APP_CLIENT_ID}" \
  -d "client_secret=${SYSTEM_APP_CLIENT_SECRET}" \
  -d "scope=system")

SYSTEM_TOKEN=$(echo "${TOKEN_RESPONSE}" | jq -r '.access_token')

if [ -z "$SYSTEM_TOKEN" ] || [ "$SYSTEM_TOKEN" = "null" ]; then
  log_error "Failed to get system token"
  log_error "Response: ${TOKEN_RESPONSE}"
  exit 1
fi

log_success "System token obtained"

# Step 2: Check if CLI application already exists
log_info "Checking if CLI application exists..."
EXISTING_APPS=$(curl -s --max-time 10 -X GET "${THUNDER_ENDPOINT}/applications" \
  -H "Authorization: Bearer ${SYSTEM_TOKEN}")

APP_ID=$(echo "${EXISTING_APPS}" | jq -r --arg cid "${CLI_CLIENT_ID}" '.applications[] | select(.client_id == $cid) | .id // empty')

# Application payload
APP_PAYLOAD=$(cat <<EOF
{
  "name": "QuickStart CLI Application",
  "description": "OpenChoreo CLI for quickstart",
  "inbound_auth_config": [
    {
      "type": "oauth2",
      "config": {
        "client_id": "${CLI_CLIENT_ID}",
        "grant_types": ["client_credentials"],
        "token_endpoint_auth_method": "client_secret_post",
        "pkce_required": false,
        "public_client": false,
        "client_secret": "${CLI_CLIENT_SECRET}",
        "token": {
          "access_token": {
            "validity_period": 3600
          }
        }
      }
    }
  ]
}
EOF
)

if [ -n "$APP_ID" ] && [ "$APP_ID" != "null" ]; then
  log_info "CLI application already exists (id: ${APP_ID}), updating..."
  HTTP_RESPONSE=$(curl -s --max-time 10 -w '\n%{http_code}' -X PUT "${THUNDER_ENDPOINT}/applications/${APP_ID}" \
    -H "Authorization: Bearer ${SYSTEM_TOKEN}" \
    -H "Content-Type: application/json" \
    -d "${APP_PAYLOAD}")
  ACTION="updated"
else
  log_info "Creating CLI application..."
  HTTP_RESPONSE=$(curl -s --max-time 10 -w '\n%{http_code}' -X POST "${THUNDER_ENDPOINT}/applications" \
    -H "Authorization: Bearer ${SYSTEM_TOKEN}" \
    -H "Content-Type: application/json" \
    -d "${APP_PAYLOAD}")
  ACTION="created"
fi

HTTP_STATUS=$(echo "${HTTP_RESPONSE}" | tail -n1)
APP_RESPONSE=$(echo "${HTTP_RESPONSE}" | sed '$d')

if [ "$HTTP_STATUS" -lt 200 ] || [ "$HTTP_STATUS" -ge 300 ]; then
  log_error "Thunder returned HTTP ${HTTP_STATUS} while creating/updating the CLI application"
  log_error "Response: ${APP_RESPONSE}"
  exit 1
fi

log_success "CLI application ${ACTION}"

# Step 4: Save CLI credentials to file
log_info "Saving CLI credentials..."
ENV_FILE="${SCRIPT_DIR}/.occ-credentials"

cat > "${ENV_FILE}" <<EOF
# OCC CLI Credentials
# Source this file to set environment variables: source ${ENV_FILE}
export OCC_CLIENT_ID="${CLI_CLIENT_ID}"
export OCC_CLIENT_SECRET="${CLI_CLIENT_SECRET}"
EOF
