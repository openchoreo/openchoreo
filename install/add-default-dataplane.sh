#!/bin/bash

# Color codes
RED='\033[0;31m'
GREEN='\033[0;32m'
DARK_YELLOW='\033[0;33m'
RESET='\033[0m'

DEFAULT_CONTEXT="kind-openchoreo-dp"
DEFAULT_TARGET_CONTEXT="kind-openchoreo-cp"
SERVER_URL=""
DEFAULT_DATAPLANE_KIND_NAME="default"

KUBECONFIG=${KUBECONFIG:-~/.kube/config}

echo -e "\nSetting up OpenChoreo DataPlane\n"

SINGLE_CLUSTER=true

# Detect if running in single-cluster mode via env var
if [[ "$1" == "--multi-cluster" ]]; then
  SINGLE_CLUSTER=false
fi

if [[ "$SINGLE_CLUSTER" == "true" ]]; then
  CONTEXT=$(kubectl config current-context)
  TARGET_CONTEXT=$CONTEXT
  DATAPLANE_KIND_NAME=$DEFAULT_DATAPLANE_KIND_NAME
  echo "Running in single-cluster mode using context '$CONTEXT'"
else
  read -p "Enter DataPlane Kubernetes context (default: $DEFAULT_CONTEXT): " INPUT_CONTEXT
  CONTEXT=${INPUT_CONTEXT:-$DEFAULT_CONTEXT}
  TARGET_CONTEXT=$DEFAULT_TARGET_CONTEXT

  echo -e "\nUsing Kubernetes context '$CONTEXT' as DataPlane."

  read -p "Enter DataPlane kind name (default: $DEFAULT_DATAPLANE_KIND_NAME): " INPUT_DATAPLANE_NAME
  DATAPLANE_KIND_NAME=${INPUT_DATAPLANE_NAME:-$DEFAULT_DATAPLANE_KIND_NAME}
fi

# Extract info from chosen context
CLUSTER_NAME=$(kubectl config view -o jsonpath="{.contexts[?(@.name=='$CONTEXT')].context.cluster}")
USER_NAME=$(kubectl config view -o jsonpath="{.contexts[?(@.name=='$CONTEXT')].context.user}")
SERVER_URL=$(kubectl config view -o jsonpath="{.clusters[?(@.name=='$CLUSTER_NAME')].cluster.server}")

# Check if SERVER_URL is a loopback address with non-node port
if [[ -n "$SERVER_URL" ]]; then
  # Extract hostname and port from SERVER_URL
  # Remove protocol prefix (http:// or https://)
  HOST_PORT=$(echo "$SERVER_URL" | sed -E 's|^https?://||')
  HOST=$(echo "$HOST_PORT" | cut -d':' -f1)
  PORT=$(echo "$HOST_PORT" | cut -d':' -f2)
  
  # Check if it's a loopback address and port is not in node port range (30000-32767)
  if [[ "$HOST" == "localhost" || "$HOST" == "127.0.0.1" || "$HOST" == "::1" ]]; then
    if [[ -n "$PORT" ]] && [[ "$PORT" != "$HOST" ]] && [[ "$PORT" =~ ^[0-9]+$ ]] && [[ "$PORT" -lt 30000 || "$PORT" -gt 32767 ]]; then
      echo "Loopback address with non-node port detected, setting SERVER_URL to null"
      SERVER_URL=""
    fi
  fi
fi

echo " "

# Try to get base64-encoded values directly
CA_CERT=$(kubectl config view --raw -o jsonpath="{.clusters[?(@.name=='$CLUSTER_NAME')].cluster.certificate-authority-data}")
CLIENT_CERT=$(kubectl config view --raw -o jsonpath="{.users[?(@.name=='$USER_NAME')].user.client-certificate-data}")
CLIENT_KEY=$(kubectl config view --raw -o jsonpath="{.users[?(@.name=='$USER_NAME')].user.client-key-data}")
USER_TOKEN=$(kubectl config view --raw -o jsonpath="{.users[?(@.name=='$USER_NAME')].user.token}")

# Fallback: encode file contents for CA cert
if [ -z "$CA_CERT" ]; then
  CA_PATH=$(kubectl config view -o jsonpath="{.clusters[?(@.name=='$CLUSTER_NAME')].cluster.certificate-authority}")
  if [ -n "$CA_PATH" ] && [ -f "$CA_PATH" ]; then
    CA_CERT=$(base64 "$CA_PATH" | tr -d '\n')
  fi
fi

# Fallback: encode file contents for client cert and key
# Fallback: encode file contents for client cert and key
if [ -z "$CLIENT_CERT" ]; then
  CERT_PATH=$(kubectl config view -o jsonpath="{.users[?(@.name=='$USER_NAME')].user.client-certificate}")
  if [ -n "$CERT_PATH" ] && [ -f "$CERT_PATH" ]; then
    CLIENT_CERT=$(base64 "$CERT_PATH" | tr -d '\n')
  fi
fi

if [ -z "$CLIENT_KEY" ]; then
  KEY_PATH=$(kubectl config view -o jsonpath="{.users[?(@.name=='$USER_NAME')].user.client-key}")
  if [ -n "$KEY_PATH" ] && [ -f "$KEY_PATH" ]; then
    CLIENT_KEY=$(base64 "$KEY_PATH" | tr -d '\n')
  fi
fi

# Determine authentication method
AUTH_CONFIG=""

if [ -n "$CLIENT_CERT" ] && [ -n "$CLIENT_KEY" ]; then
  # Use mTLS authentication
  AUTH_CONFIG="mtls:
        clientCert:
          value: $CLIENT_CERT
        clientKey:
          value: $CLIENT_KEY"
  echo "Using mTLS authentication"
elif [ -n "$USER_TOKEN" ]; then
  # Use bearer token authentication
  AUTH_CONFIG="bearerToken:
        value: $USER_TOKEN"
  echo "Using bearer token authentication"
else
  echo -e "\n${RED}Error: No valid authentication method found. Need either client certificates or user token in the kube config.${RESET}"
  exit 1
fi

# Validate CA certificate is available
if [ -z "$CA_CERT" ]; then
  echo -e "\n${RED}Error: CA certificate is required but not found in kubeconfig.${RESET}"
  exit 1
fi

# Apply the DataPlane manifest in the target context
echo -e "\nApplying DataPlane to context: $TARGET_CONTEXT"

if kubectl --context="$TARGET_CONTEXT" apply -f - <<EOF
apiVersion: openchoreo.dev/v1alpha1
kind: DataPlane
metadata:
  annotations:
    openchoreo.dev/description: DataPlane "$DATAPLANE_KIND_NAME" was created through the script.
    openchoreo.dev/display-name: DataPlane "$DATAPLANE_KIND_NAME"
  labels:
    openchoreo.dev/name: $DATAPLANE_KIND_NAME
    openchoreo.dev/organization: default
    openchoreo.dev/build-plane: "true"
  name: $DATAPLANE_KIND_NAME
  namespace: default
spec:
  registry:
    prefix: registry.openchoreo-data-plane:5000
  gateway:
    organizationVirtualHost: openchoreoapis.internal
    publicVirtualHost: openchoreoapis.localhost
  kubernetesCluster:
    server: $SERVER_URL
    tls:
      ca:
        value: $CA_CERT
    auth:
      $AUTH_CONFIG
EOF
then
    echo -e "\n${GREEN}DataPlane applied to 'default' successfully!${RESET}"
else
    echo -e "\n${RED}Failed to apply DataPlane manifest to context: $TARGET_CONTEXT${RESET}"
    exit 1
fi
