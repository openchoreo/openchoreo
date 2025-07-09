#!/bin/bash

# Color codes
RED='\033[0;31m'
GREEN='\033[0;32m'
DARK_YELLOW='\033[0;33m'
RESET='\033[0m'

DEFAULT_CONTEXT="kind-choreo-dp"
DEFAULT_TARGET_CONTEXT="kind-choreo-cp"
SERVER_URL=""
DEFAULT_DATAPLANE_KIND_NAME="default-dataplane"

KUBECONFIG=${KUBECONFIG:-~/.kube/config}

echo -e "\nSetting up Choreo DataPlane\n"

SINGLE_CLUSTER=true

# Detect if running in single-cluster mode via env var
if [[ "$1" == "--multi-cluster" ]]; then
  SINGLE_CLUSTER=false
fi

if [[ "$SINGLE_CLUSTER" == "true" ]]; then
  CONTEXT=$(kubectl config current-context)
  TARGET_CONTEXT=$CONTEXT
  DATAPLANE_KIND_NAME=$DEFAULT_DATAPLANE_KIND_NAME
  NODE_NAME_PREFIX=${CONTEXT#kind-}
  SERVER_URL="https://$NODE_NAME_PREFIX-control-plane:6443"
  echo "Running in single-cluster mode using context '$CONTEXT'"
else
  read -p "Enter DataPlane Kubernetes context (default: $DEFAULT_CONTEXT): " INPUT_CONTEXT
  CONTEXT=${INPUT_CONTEXT:-$DEFAULT_CONTEXT}
  TARGET_CONTEXT=$DEFAULT_TARGET_CONTEXT

  echo -e "\nUsing Kubernetes context '$CONTEXT' as DataPlane."
  NODE_NAME_PREFIX=${CONTEXT#kind-}
  SERVER_URL="https://$NODE_NAME_PREFIX-control-plane:6443"

  read -p "Enter DataPlane kind name (default: $DEFAULT_DATAPLANE_KIND_NAME): " INPUT_DATAPLANE_NAME
  DATAPLANE_KIND_NAME=${INPUT_DATAPLANE_NAME:-$DEFAULT_DATAPLANE_KIND_NAME}
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
    openchoreo.dev/organization: default-org
    openchoreo.dev/build-plane: "true"
  name: $DATAPLANE_KIND_NAME
  namespace: default-org
spec:
  registry:
    unauthenticated:
      - registry.choreo-system:5000
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
    echo -e "\n${GREEN}DataPlane applied to 'default-org' successfully!${RESET}"
else
    echo -e "\n${RED}Failed to apply DataPlane manifest to context: $TARGET_CONTEXT${RESET}"
    exit 1
fi
