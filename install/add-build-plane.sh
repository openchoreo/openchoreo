#!/bin/bash

# Color codes
RED='\033[0;31m'
GREEN='\033[0;32m'
DARK_YELLOW='\033[0;33m'
RESET='\033[0m'

DEFAULT_CONTEXT="kind-choreo-bp"
DEFAULT_TARGET_CONTEXT="kind-choreo"
SERVER_URL=""
DEFAULT_BUILDPLANE_KIND_NAME="default-buildplane"

KUBECONFIG=${KUBECONFIG:-~/.kube/config}

echo -e "\nSetting up Choreo BuildPlane\n"
SEPARATE=false
if [[ "$1" == "--separate" ]]; then
  SEPARATE=true
fi

if [[ "$SEPARATE" == false ]]; then
  CONTEXT=$(kubectl config current-context)
  TARGET_CONTEXT=$CONTEXT
  BUILDPLANE_KIND_NAME=$DEFAULT_BUILDPLANE_KIND_NAME
  NODE_NAME_PREFIX=${CONTEXT#kind-}
  SERVER_URL="https://$NODE_NAME_PREFIX-control-plane:6443"
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
echo "$CLUSTER_NAME"
# Try to get base64-encoded values directly
CA_CERT=$(kubectl config view --raw -o jsonpath="{.clusters[?(@.name=='$CLUSTER_NAME')].cluster.certificate-authority-data}")
CLIENT_CERT=$(kubectl config view --raw -o jsonpath="{.users[?(@.name=='$USER_NAME')].user.client-certificate-data}")
CLIENT_KEY=$(kubectl config view --raw -o jsonpath="{.users[?(@.name=='$USER_NAME')].user.client-key-data}")

# Fallback: encode file contents
if [ -z "$CA_CERT" ]; then
  CA_PATH=$(kubectl config view -o jsonpath="{.clusters[?(@.name=='$CLUSTER_NAME')].cluster.certificate-authority}")
  CA_CERT=$(base64 "$CA_PATH" | tr -d '\n')
fi

if [ -z "$CLIENT_CERT" ]; then
  CERT_PATH=$(kubectl config view -o jsonpath="{.users[?(@.name=='$USER_NAME')].user.client-certificate}")
  CLIENT_CERT=$(base64 "$CERT_PATH" | tr -d '\n')
fi

if [ -z "$CLIENT_KEY" ]; then
  KEY_PATH=$(kubectl config view -o jsonpath="{.users[?(@.name=='$USER_NAME')].user.client-key}")
  CLIENT_KEY=$(base64 "$KEY_PATH" | tr -d '\n')
fi

# Apply the BuildPlane manifest in the target context
echo -e "\nApplying BuildPlane to context: $TARGET_CONTEXT"

if kubectl --context="$TARGET_CONTEXT" apply -f - <<EOF
apiVersion: core.choreo.dev/v1
kind: BuildPlane
metadata:
  annotations:
    core.choreo.dev/description: BuildPlane "$BUILDPLANE_KIND_NAME" was created through the script.
    core.choreo.dev/display-name: BuildPlane "$BUILDPLANE_KIND_NAME"
  labels:
    core.choreo.dev/name: $BUILDPLANE_KIND_NAME
    core.choreo.dev/organization: default-org
  name: $BUILDPLANE_KIND_NAME
  namespace: default-org
spec:
  kubernetesCluster:
    name: $CLUSTER_NAME
    credentials:
      apiServerURL: $SERVER_URL
      caCert: $CA_CERT
      clientCert: $CLIENT_CERT
      clientKey: $CLIENT_KEY
EOF
then
    echo -e "\n${GREEN}BuildPlane applied to 'default-org' successfully!${RESET}"
else
    echo -e "\n${RED}Failed to apply BuildPlane manifest to context: $TARGET_CONTEXT${RESET}"
    exit 1
fi
