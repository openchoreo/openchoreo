#!/bin/bash

# Optional: set to your custom kubeconfig path
KUBECONFIG=${KUBECONFIG:-~/.kube/config}

echo "🚀 Setting up Choreo DataPlane"

# Ask if this is a multi-cluster setup
read -p "Is this a multi-cluster setup? (y/n): " IS_MULTI_CLUSTER

if [[ "$IS_MULTI_CLUSTER" =~ ^[Yy]$ ]]; then
  # Prompt user for the source context (where the remote cluster credentials are from)
  read -p "Enter DataPlane Kubernetes context (e.g., kind-choreo)/Leave the context empty to use current: " INPUT_CONTEXT
  CONTEXT=${INPUT_CONTEXT:-$(kubectl config current-context)}
  TARGET_CONTEXT="kind-choreo-cp"
  echo "🔄 Using credentials from '$CONTEXT' to be applied to '$TARGET_CONTEXT'"
else
  # Default to current context for both credentials and target
  CONTEXT=$(kubectl config current-context)
  TARGET_CONTEXT=$CONTEXT
  echo "🔄 Single-cluster mode. Using current context '$CONTEXT' for default DataPlane"
fi

# Extract info from chosen context
CLUSTER_NAME=$(kubectl config view -o jsonpath="{.contexts[?(@.name=='$CONTEXT')].context.cluster}")
USER_NAME=$(kubectl config view -o jsonpath="{.contexts[?(@.name=='$CONTEXT')].context.user}")
SERVER_URL=$(kubectl config view -o jsonpath="{.clusters[?(@.name=='$CLUSTER_NAME')].cluster.server}")

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
echo "📦 Applying DataPlane to context: $TARGET_CONTEXT"
kubectl --context="$TARGET_CONTEXT" apply -f - <<EOF
apiVersion: core.choreo.dev/v1
kind: DataPlane
metadata:
  annotations:
    core.choreo.dev/description: Local development data plane
    core.choreo.dev/display-name: Default Data Plane
  labels:
    core.choreo.dev/name: default-dataplane
    core.choreo.dev/organization: default-org
  name: default-dataplane
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

echo "✅  DataPlane applied successfully!"
