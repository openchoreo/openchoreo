#!/bin/bash

# Color codes
RED="\033[0;31m"
GREEN="\033[0;32m"
DARK_YELLOW="\033[0;33m"
RESET="\033[0m"

DEFAULT_CONTEXT="kind-choreo-dp"
DEFAULT_TARGET_CONTEXT="kind-choreo-cp"
SERVER_URL=""
DEFAULT_DATAPLANE_KIND_NAME="default-dataplane"

KUBECONFIG=${KUBECONFIG:-~/.kube/config}

echo "Setting up Choreo DataPlane \n"

# Ask if this is a multi-cluster setup
read -p "Is this a multi-cluster setup? (y/n): " IS_MULTI_CLUSTER

if [[ "$IS_MULTI_CLUSTER" =~ ^[Yy]$ ]]; then
  # Prompt user for the source context (where the remote cluster credentials are from)
  read -p "Enter DataPlane kubernetes context (default: $DEFAULT_CONTEXT): " INPUT_CONTEXT
  CONTEXT=${INPUT_CONTEXT:-$DEFAULT_CONTEXT}
  TARGET_CONTEXT=$DEFAULT_TARGET_CONTEXT
  echo "\nUsing Kubernetes context '$CONTEXT' as DataPlane."
  NODE_NAME=${CONTEXT#kind-}
  SERVER_URL="https://$NODE_NAME-control-plane:6443"
else
  # Default to current context for both credentials and target
  CONTEXT=$(kubectl config current-context)
  TARGET_CONTEXT=$CONTEXT
  echo "\nSingle-cluster mode. Using current context '$CONTEXT' as default DataPlane"
  SERVER_URL="https://choreo-control-plane:6443"
fi

read -p "Enter DataPlane kind name (default: $DEFAULT_DATAPLANE_KIND_NAME): " INPUT_DATAPLANE_NAME
DATAPLANE_KIND_NAME=${INPUT_DATAPLANE_NAME:-$DEFAULT_DATAPLANE_KIND_NAME}

# Extract info from chosen context
CLUSTER_NAME=$(kubectl config view -o jsonpath="{.contexts[?(@.name=='$CONTEXT')].context.cluster}")
USER_NAME=$(kubectl config view -o jsonpath="{.contexts[?(@.name=='$CONTEXT')].context.user}")

# Try to get base64-encoded values directly
CA_CERT=$(kubectl config view --raw -o jsonpath="{.clusters[?(@.name=='$CLUSTER_NAME')].cluster.certificate-authority-data}")
CLIENT_CERT=$(kubectl config view --raw -o jsonpath="{.users[?(@.name=='$USER_NAME')].user.client-certificate-data}")
CLIENT_KEY=$(kubectl config view --raw -o jsonpath="{.users[?(@.name=='$USER_NAME')].user.client-key-data}")

# If data is missing (not inlined), try encoding from file paths
if [ -z "$CA_CERT" ]; then
  CA_PATH=$(kubectl config view -o jsonpath="{.clusters[?(@.name=='$CLUSTER_NAME')].cluster.certificate-authority}")
  CA_CERT=$(base64 -w 0 "$CA_PATH")
fi

if [ -z "$CLIENT_CERT" ]; then
  CERT_PATH=$(kubectl config view -o jsonpath="{.users[?(@.name=='$USER_NAME')].user.client-certificate}")
  CLIENT_CERT=$(base64 -w 0 "$CERT_PATH")
fi

if [ -z "$CLIENT_KEY" ]; then
  KEY_PATH=$(kubectl config view -o jsonpath="{.users[?(@.name=='$USER_NAME')].user.client-key}")
  CLIENT_KEY=$(base64 -w 0 "$KEY_PATH")
fi

# Apply the DataPlane manifest in the target context
echo "\nApplying DataPlane to context: $TARGET_CONTEXT"

if kubectl --context="$TARGET_CONTEXT" apply -f - <<EOF
apiVersion: core.choreo.dev/v1
kind: DataPlane
metadata:
  annotations:
    core.choreo.dev/description: DataPlane "$DATAPLANE_KIND_NAME" was created through the script.
    core.choreo.dev/display-name: DataPlane "$DATAPLANE_KIND_NAME"
  labels:
    core.choreo.dev/name: $DATAPLANE_KIND_NAME
    core.choreo.dev/organization: default-org
  name: $DATAPLANE_KIND_NAME
  namespace: default-org
spec:
  gateway:
    organizationVirtualHost: choreoapis.internal
    publicVirtualHost: choreoapis.localhost
  kubernetesCluster:
    name: $CLUSTER_NAME
    credentials:
      apiServerURL: $SERVER_URL
      caCert: $CA_CERT
      clientCert: $CLIENT_CERT
      clientKey: $CLIENT_KEY
EOF
then
    echo "\n${GREEN}DataPlane applied to 'default-org' successfully!${RESET}"
else
    echo "\n${RED}Failed to apply DataPlane manifest to context: $TARGET_CONTEXT${RESET}"
    exit 1
fi
