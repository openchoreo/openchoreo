#!/bin/bash

# OpenChoreo DataPlane Creation Script
# Creates a DataPlane resource in the control plane that targets a data plane cluster

set -e

# Color codes for errors
RED='\033[0;31m'
RESET='\033[0m'

# Helper function for error output
error() {
  echo -e "${RED}$*${RESET}" >&2
}

# Show help text
show_help() {
  local script_name
  script_name=$(basename "$0")
  cat << EOF
Create an OpenChoreo DataPlane resource that targets a Kubernetes cluster.

Usage:
  $script_name [OPTIONS]

Options:
  --control-plane-context CONTEXT   Kubernetes context where DataPlane resource will be created
                                    Default: current context

  --target-context CONTEXT          Kubernetes context to extract credentials from
                                    Default: same as control-plane-context
                                    Cannot be specified without --control-plane-context

  --server URL                      Kubernetes API server URL of the target cluster
                                    Default: https://kubernetes.default.svc.cluster.local
                                    Required when control-plane and target contexts differ

  --name NAME                       Name for the DataPlane resource
                                    Default: default

  --namespace NAMESPACE             Namespace for the DataPlane resource
                                    Default: default (Note: DataPlane CRs are typically created in 'default')

  --enable-agent                    Use cluster agent for data plane communication
                                    When enabled, skips kubernetesCluster configuration

  --agent-ca-secret NAME            Secret name containing agent client CA certificate
                                    Default: cluster-agent-ca (only used with --enable-agent)

  --agent-ca-namespace NAMESPACE    Namespace of agent CA secret
                                    Default: same as --namespace (only used with --enable-agent)

  --dry-run                         Preview the YAML without applying changes

  --help, -h                        Show this help message

Examples:

  # Single-cluster (default)
  $script_name

  # Single-cluster with custom context
  $script_name --control-plane-context k3d-openchoreo

  # Single-cluster with external server URL
  $script_name --control-plane-context k3d-openchoreo --server https://localhost:6443

  # Multi-cluster
  $script_name --control-plane-context k3d-openchoreo-cp \\
     --target-context k3d-openchoreo-dp \\
     --server https://k3d-openchoreo-dp-server-0:6443

  # Custom name and namespace
  $script_name --name prod-dataplane --namespace production

  # Preview without applying
  $script_name --dry-run

  # Using cluster agent (recommended for production)
  $script_name --enable-agent --control-plane-context k3d-openchoreo

  # Using cluster agent with custom CA secret and namespace
  $script_name --enable-agent --agent-ca-secret my-agent-ca --agent-ca-namespace openchoreo-control-plane --control-plane-context k3d-openchoreo

Note:
  Single-cluster: Both control plane and data plane in same cluster
  Multi-cluster:  Different clusters, requires explicit --server URL
  Agent mode:     Uses cluster agent for secure communication (recommended)
EOF
}

# Defaults
TARGET_CONTEXT=""              # Data plane cluster (extract creds, the "target")
CONTROL_PLANE_CONTEXT=""       # Control plane cluster (create resource here)
SERVER_URL="https://kubernetes.default.svc.cluster.local"
DATAPLANE_NAME="default"
NAMESPACE="default"
DRY_RUN=false
ENABLE_AGENT=false
AGENT_CA_SECRET="cluster-agent-ca"
AGENT_CA_NAMESPACE=""

# Parse arguments
while [[ $# -gt 0 ]]; do
  case $1 in
    --dry-run)
      DRY_RUN=true
      shift
      ;;
    --enable-agent)
      ENABLE_AGENT=true
      shift
      ;;
    --agent-ca-secret)
      if [ -z "$2" ] || [[ "$2" == --* ]]; then
        error "Error: --agent-ca-secret requires a value"
        exit 1
      fi
      AGENT_CA_SECRET="$2"
      shift 2
      ;;
    --agent-ca-namespace)
      if [ -z "$2" ] || [[ "$2" == --* ]]; then
        error "Error: --agent-ca-namespace requires a value"
        exit 1
      fi
      AGENT_CA_NAMESPACE="$2"
      shift 2
      ;;
    --target-context)
      if [ -z "$2" ] || [[ "$2" == --* ]]; then
        error "Error: --target-context requires a value"
        exit 1
      fi
      TARGET_CONTEXT="$2"
      shift 2
      ;;
    --control-plane-context)
      if [ -z "$2" ] || [[ "$2" == --* ]]; then
        error "Error: --control-plane-context requires a value"
        exit 1
      fi
      CONTROL_PLANE_CONTEXT="$2"
      shift 2
      ;;
    --server)
      if [ -z "$2" ] || [[ "$2" == --* ]]; then
        error "Error: --server requires a value"
        exit 1
      fi
      SERVER_URL="$2"
      shift 2
      ;;
    --name)
      if [ -z "$2" ] || [[ "$2" == --* ]]; then
        error "Error: --name requires a value"
        exit 1
      fi
      DATAPLANE_NAME="$2"
      shift 2
      ;;
    --namespace)
      if [ -z "$2" ] || [[ "$2" == --* ]]; then
        error "Error: --namespace requires a value"
        exit 1
      fi
      NAMESPACE="$2"
      shift 2
      ;;
    --help|-h)
      show_help
      exit 0
      ;;
    *)
      error "Unknown option: $1"
      echo "Use --help for usage information"
      exit 1
      ;;
  esac
done

# Use current context if not specified
CURRENT_CONTEXT=$(kubectl config current-context 2>/dev/null || echo "")

if [ -z "$CURRENT_CONTEXT" ]; then
  error "Error: No current kubectl context found"
  echo "Please configure kubectl or specify contexts explicitly"
  exit 1
fi

# Rule 1: Reject target-only (cannot specify target without control plane)
if [ -n "$TARGET_CONTEXT" ] && [ -z "$CONTROL_PLANE_CONTEXT" ]; then
  error "Error: Cannot specify --target-context without --control-plane-context"
  echo "Use --help for more information"
  exit 1
fi

# Set defaults
if [ -z "$CONTROL_PLANE_CONTEXT" ]; then
  CONTROL_PLANE_CONTEXT="$CURRENT_CONTEXT"
fi

if [ -z "$TARGET_CONTEXT" ]; then
  TARGET_CONTEXT="$CONTROL_PLANE_CONTEXT"
fi

# Set agent CA namespace default
if [ -z "$AGENT_CA_NAMESPACE" ]; then
  AGENT_CA_NAMESPACE="$NAMESPACE"
fi

# Rule 2: Multi-cluster requires explicit external server URL (unless using agent)
if [ "$ENABLE_AGENT" = false ] && [ "$CONTROL_PLANE_CONTEXT" != "$TARGET_CONTEXT" ] && [ "$SERVER_URL" == "https://kubernetes.default.svc.cluster.local" ]; then
  error "Error: Multi-cluster mode requires --server with external URL"
  echo "Use --help for more information"
  exit 1
fi

# Validate contexts exist
# Check control plane context first
if ! kubectl config get-contexts "$CONTROL_PLANE_CONTEXT" &>/dev/null; then
  error "Error: Control plane context '$CONTROL_PLANE_CONTEXT' not found in kubeconfig"
  echo "Available contexts:"
  kubectl config get-contexts -o name
  exit 1
fi

# Only check target if different from control plane and not using agent mode
if [ "$ENABLE_AGENT" = false ] && [ "$CONTROL_PLANE_CONTEXT" != "$TARGET_CONTEXT" ]; then
  if ! kubectl config get-contexts "$TARGET_CONTEXT" &>/dev/null; then
    error "Error: Target context '$TARGET_CONTEXT' not found in kubeconfig"
    echo "Available contexts:"
    kubectl config get-contexts -o name
    exit 1
  fi
fi

# If using agent mode, skip credential extraction
if [ "$ENABLE_AGENT" = true ]; then
  if ! kubectl --context="$CONTROL_PLANE_CONTEXT" get secret "$AGENT_CA_SECRET" -n "$AGENT_CA_NAMESPACE" >/dev/null 2>&1; then
    error "Cluster gateway CA secret '$AGENT_CA_SECRET' not found in namespace '$AGENT_CA_NAMESPACE'"
    echo ""
    echo "The cluster gateway CA secret is required for agent-based communication."
    echo "This secret should be created automatically by the cluster-gateway component."
    echo ""
    echo "Please ensure:"
    echo "  1. Cluster gateway is enabled in the control plane Helm chart"
    echo "  2. The cluster-gateway pod is running and healthy"
    echo "  3. The CA extraction job has completed successfully"
    echo ""
    echo "You can check the status with:"
    echo "  kubectl --context=$CONTROL_PLANE_CONTEXT get pods -n $AGENT_CA_NAMESPACE | grep cluster-gateway"
    echo "  kubectl --context=$CONTROL_PLANE_CONTEXT get secret $AGENT_CA_SECRET -n $AGENT_CA_NAMESPACE"
    echo ""
    exit 1
  fi

  # Generate the DataPlane YAML with agent configuration
  DATAPLANE_YAML=$(cat <<EOF
apiVersion: openchoreo.dev/v1alpha1
kind: DataPlane
metadata:
  name: $DATAPLANE_NAME
  namespace: $NAMESPACE
  annotations:
    openchoreo.dev/description: "DataPlane created via $(basename $0) script with cluster agent"
    openchoreo.dev/display-name: "DataPlane $DATAPLANE_NAME"
spec:
  agent:
    enabled: true
    clientCA:
      secretRef:
        name: $AGENT_CA_SECRET
        namespace: $AGENT_CA_NAMESPACE
        key: ca.crt
  secretStoreRef:
    name: default
  gateway:
    organizationVirtualHost: openchoreoapis.internal
    publicVirtualHost: openchoreoapis.localhost
EOF
)

  # Apply or preview the manifest
  if [ "$DRY_RUN" = true ]; then
    echo "$DATAPLANE_YAML"
    exit 0
  else
    if echo "$DATAPLANE_YAML" | kubectl --context="$CONTROL_PLANE_CONTEXT" apply -f - ; then
      :
    else
      error "Failed to create DataPlane resource"
      exit 1
    fi
  fi
  exit 0
fi

# Extract cluster and user info from target context
CLUSTER_NAME=$(kubectl config view -o jsonpath="{.contexts[?(@.name=='$TARGET_CONTEXT')].context.cluster}")
USER_NAME=$(kubectl config view -o jsonpath="{.contexts[?(@.name=='$TARGET_CONTEXT')].context.user}")

if [ -z "$CLUSTER_NAME" ] || [ -z "$USER_NAME" ]; then
  error "Error: Could not find cluster or user for context '$TARGET_CONTEXT'"
  exit 1
fi

# Extract base64-encoded credentials
CA_CERT=$(kubectl config view --raw -o jsonpath="{.clusters[?(@.name=='$CLUSTER_NAME')].cluster.certificate-authority-data}" 2>/dev/null)
CLIENT_CERT=$(kubectl config view --raw -o jsonpath="{.users[?(@.name=='$USER_NAME')].user.client-certificate-data}" 2>/dev/null)
CLIENT_KEY=$(kubectl config view --raw -o jsonpath="{.users[?(@.name=='$USER_NAME')].user.client-key-data}" 2>/dev/null)
USER_TOKEN=$(kubectl config view --raw -o jsonpath="{.users[?(@.name=='$USER_NAME')].user.token}" 2>/dev/null)

# Fallback: encode from file paths if not already base64-encoded
if [ -z "$CA_CERT" ]; then
  CA_PATH=$(kubectl config view -o jsonpath="{.clusters[?(@.name=='$CLUSTER_NAME')].cluster.certificate-authority}" 2>/dev/null)
  if [ -n "$CA_PATH" ] && [ -f "$CA_PATH" ]; then
    CA_CERT=$(base64 < "$CA_PATH" | tr -d '\n')
  fi
fi

if [ -z "$CLIENT_CERT" ]; then
  CERT_PATH=$(kubectl config view -o jsonpath="{.users[?(@.name=='$USER_NAME')].user.client-certificate}" 2>/dev/null)
  if [ -n "$CERT_PATH" ] && [ -f "$CERT_PATH" ]; then
    CLIENT_CERT=$(base64 < "$CERT_PATH" | tr -d '\n')
  fi
fi

if [ -z "$CLIENT_KEY" ]; then
  KEY_PATH=$(kubectl config view -o jsonpath="{.users[?(@.name=='$USER_NAME')].user.client-key}" 2>/dev/null)
  if [ -n "$KEY_PATH" ] && [ -f "$KEY_PATH" ]; then
    CLIENT_KEY=$(base64 < "$KEY_PATH" | tr -d '\n')
  fi
fi

# Determine authentication method and build YAML sections
AUTH_SECTION=""
TLS_SECTION=""

# Determine auth method
if [ -n "$CLIENT_CERT" ] && [ -n "$CLIENT_KEY" ]; then
  AUTH_SECTION="    auth:
      mtls:
        clientCert:
          value: $CLIENT_CERT
        clientKey:
          value: $CLIENT_KEY"
elif [ -n "$USER_TOKEN" ]; then
  AUTH_SECTION="    auth:
      bearerToken:
        value: $USER_TOKEN"
else
  error "Error: No valid authentication credentials found in context '$TARGET_CONTEXT'"
  echo "Need either:"
  echo "  - Client certificate and key (mTLS), or"
  echo "  - Bearer token"
  exit 1
fi

# CA certificate (required for TLS)
if [ -z "$CA_CERT" ]; then
  error "Error: CA certificate not found in context '$TARGET_CONTEXT'"
  echo "CA certificate is required for secure connection to the cluster"
  exit 1
fi

TLS_SECTION="    tls:
      ca:
        value: $CA_CERT"

# Generate the DataPlane YAML
DATAPLANE_YAML=$(cat <<EOF
apiVersion: openchoreo.dev/v1alpha1
kind: DataPlane
metadata:
  name: $DATAPLANE_NAME
  namespace: $NAMESPACE
  annotations:
    openchoreo.dev/description: "DataPlane created via $(basename $0) script"
    openchoreo.dev/display-name: "DataPlane $DATAPLANE_NAME"
spec:
  secretStoreRef:
    name: default
  gateway:
    organizationVirtualHost: openchoreoapis.internal
    publicVirtualHost: openchoreoapis.localhost
  kubernetesCluster:
    server: $SERVER_URL
$TLS_SECTION
$AUTH_SECTION
EOF
)

# Apply or preview the manifest
if [ "$DRY_RUN" = true ]; then
  echo "$DATAPLANE_YAML"
  exit 0
else
  if echo "$DATAPLANE_YAML" | kubectl --context="$CONTROL_PLANE_CONTEXT" apply -f - ; then
    :
  else
    error "Failed to create DataPlane resource"
    exit 1
  fi
fi
