#!/bin/bash

# Color codes
RED='\033[0;31m'
GREEN='\033[0;32m'
DARK_YELLOW='\033[0;33m'
RESET='\033[0m'

DEFAULT_CONTEXT="kind-openchoreo-dp"
DEFAULT_TARGET_CONTEXT="kind-openchoreo-cp"
SERVER_URL=""
DEFAULT_BUILDPLANE_KIND_NAME="default"

KUBECONFIG=${KUBECONFIG:-~/.kube/config}

echo -e "\nSetting up OpenChoreo BuildPlane\n"
SEPARATE=false
if [[ "$1" == "--separate" ]]; then
  SEPARATE=true
fi

if [[ "$SEPARATE" == false ]]; then
  CONTEXT=$(kubectl config current-context)
  TARGET_CONTEXT=$CONTEXT
  BUILDPLANE_KIND_NAME=$DEFAULT_BUILDPLANE_KIND_NAME
  
  # Get server URL from kubeconfig
  CLUSTER_NAME=$(kubectl config view -o jsonpath="{.contexts[?(@.name=='$CONTEXT')].context.cluster}")
  KUBE_SERVER_URL=$(kubectl config view -o jsonpath="{.clusters[?(@.name=='$CLUSTER_NAME')].cluster.server}")
  
  # Check if server URL is loopback address
  if [[ "$KUBE_SERVER_URL" =~ ^https?://(127\.0\.0\.1|localhost) ]]; then
    SERVER_URL="https://kubernetes.default.svc.cluster.local"
    echo "Running in single-cluster mode using context '$CONTEXT' with loopback server, using Kubernetes service DNS"
  else
    NODE_NAME_PREFIX=${CONTEXT#kind-}
    SERVER_URL="https://$NODE_NAME_PREFIX-control-plane:6443"
  fi
else
  read -p "Enter BuildPlane Kubernetes context (default: $DEFAULT_CONTEXT): " INPUT_CONTEXT
  CONTEXT=${INPUT_CONTEXT:-$DEFAULT_CONTEXT}
  TARGET_CONTEXT=$DEFAULT_TARGET_CONTEXT

  echo -e "\n${DARK_YELLOW}Using Kubernetes context '$CONTEXT' as BuildPlane.${RESET}"
  NODE_NAME_PREFIX=${CONTEXT#kind-}
  SERVER_URL="https://$NODE_NAME_PREFIX-control-plane:6443"

  read -p "Enter BuildPlane kind name (default: $DEFAULT_BUILDPLANE_KIND_NAME): " INPUT_BUILDPLANE_NAME
  BUILDPLANE_KIND_NAME=${INPUT_BUILDPLANE_NAME:-$DEFAULT_BUILDPLANE_KIND_NAME}
fi

# Extract info from chosen context
CLUSTER_NAME=$(kubectl config view -o jsonpath="{.contexts[?(@.name=='$CONTEXT')].context.cluster}")
USER_NAME=$(kubectl config view -o jsonpath="{.contexts[?(@.name=='$CONTEXT')].context.user}")

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

# Apply the BuildPlane manifest in the target context
echo -e "\nApplying BuildPlane to context: $TARGET_CONTEXT"

if kubectl --context="$TARGET_CONTEXT" apply -f - <<EOF
apiVersion: openchoreo.dev/v1alpha1
kind: BuildPlane
metadata:
  annotations:
    openchoreo.dev/description: BuildPlane "$BUILDPLANE_KIND_NAME" was created through the script.
    openchoreo.dev/display-name: BuildPlane "$BUILDPLANE_KIND_NAME"
  name: $BUILDPLANE_KIND_NAME
  namespace: default
spec:
  kubernetesCluster:
    server: $SERVER_URL
    tls:
      ca:
        value: $CA_CERT
    auth:
      $AUTH_CONFIG
EOF
then
    echo -e "\n${GREEN}BuildPlane applied to 'default' successfully!${RESET}"
else
    echo -e "\n${RED}Failed to apply BuildPlane manifest to context: $TARGET_CONTEXT${RESET}"
    exit 1
fi
