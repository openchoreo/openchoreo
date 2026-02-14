#!/bin/bash

# OpenChoreo BuildPlane Creation Script
# Creates a BuildPlane resource in the control plane that targets a build plane cluster
# Uses cluster agent for secure communication

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
Create an OpenChoreo BuildPlane resource that targets a Kubernetes cluster.
All BuildPlanes use cluster agent for secure communication.

Usage:
  $script_name [OPTIONS]

Options:
  --control-plane-context CONTEXT   Kubernetes context where BuildPlane resource will be created
                                    Default: current context

  --buildplane-context CONTEXT      Kubernetes context of build plane to extract client CA from
                                    Default: same as control-plane-context

  --name NAME                       Name for the BuildPlane resource
                                    Default: default

  --namespace NAMESPACE             Namespace for the BuildPlane resource
                                    Default: default

  --agent-ca-secret NAME            (Optional) Secret name containing agent client CA certificate
                                    If provided, uses secretRef mode to reference existing secret
                                    If omitted, auto-extracts CA from build plane (inline value mode)

  --agent-ca-namespace NAMESPACE    Namespace of agent CA secret (only with --agent-ca-secret)
                                    Default: same as --namespace

  --plane-id ID                     (Optional) Logical plane identifier shared across multiple CRs
                                    Default: default
                                    Use this for multi-tenant setups where multiple CRs share same physical plane

  --dry-run                         Preview the YAML without applying changes

  --help, -h                        Show this help message

Examples:

  # Single-cluster (default)
  $script_name

  # Single-cluster with custom control plane context
  $script_name --control-plane-context k3d-openchoreo

  # Multi-cluster with separate build plane
  $script_name --control-plane-context k3d-openchoreo-cp \\
    --buildplane-context k3d-openchoreo-bp \\
    --name default

  # Custom name and namespace
  $script_name --name prod-buildplane --namespace production

  # Preview without applying
  $script_name --dry-run

  # Using existing CA secret reference
  $script_name --agent-ca-secret my-agent-ca --agent-ca-namespace openchoreo-control-plane

  # Multi-tenant setup with shared planeID
  $script_name --name org-a-buildplane --namespace org-a --plane-id shared-prod
  $script_name --name org-b-buildplane --namespace org-b --plane-id shared-prod

Note:
  - All BuildPlanes use cluster agent for communication (mandatory)
  - Cluster agent establishes outbound WebSocket connection from build plane to control plane
  - No inbound ports need to be exposed on build plane clusters
EOF
}

# Defaults
CONTROL_PLANE_CONTEXT=""       # Control plane cluster (create resource here)
BUILDPLANE_CONTEXT=""          # Build plane cluster for CA extraction
BUILDPLANE_NAME="default"
NAMESPACE="default"
PLANE_ID="default"  # Optional logical plane identifier
DRY_RUN=false
AGENT_CA_SECRET=""             # Empty by default - triggers auto-extract mode
AGENT_CA_NAMESPACE=""
AGENT_CA_SECRET_EXPLICIT=false # Track if user explicitly set --agent-ca-secret

# Parse arguments
while [[ $# -gt 0 ]]; do
  case $1 in
    --dry-run)
      DRY_RUN=true
      shift
      ;;
    --agent-ca-secret)
      if [ -z "$2" ] || [[ "$2" == --* ]]; then
        error "Error: --agent-ca-secret requires a value"
        exit 1
      fi
      AGENT_CA_SECRET="$2"
      AGENT_CA_SECRET_EXPLICIT=true
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
    --buildplane-context)
      if [ -z "$2" ] || [[ "$2" == --* ]]; then
        error "Error: --buildplane-context requires a value"
        exit 1
      fi
      BUILDPLANE_CONTEXT="$2"
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
    --name)
      if [ -z "$2" ] || [[ "$2" == --* ]]; then
        error "Error: --name requires a value"
        exit 1
      fi
      BUILDPLANE_NAME="$2"
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
    --plane-id)
      if [ -z "$2" ] || [[ "$2" == --* ]]; then
        error "Error: --plane-id requires a value"
        exit 1
      fi
      PLANE_ID="$2"
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

# Set defaults
if [ -z "$CONTROL_PLANE_CONTEXT" ]; then
  CONTROL_PLANE_CONTEXT="$CURRENT_CONTEXT"
fi

# Set buildplane context default
if [ -z "$BUILDPLANE_CONTEXT" ]; then
  BUILDPLANE_CONTEXT="$CONTROL_PLANE_CONTEXT"
fi

# Validate control plane context exists
if ! kubectl config get-contexts "$CONTROL_PLANE_CONTEXT" &>/dev/null; then
  error "Error: Control plane context '$CONTROL_PLANE_CONTEXT' not found in kubeconfig"
  echo "Available contexts:"
  kubectl config get-contexts -o name
  exit 1
fi

# Validate buildplane context exists (if different from control plane)
if [ "$CONTROL_PLANE_CONTEXT" != "$BUILDPLANE_CONTEXT" ]; then
  if ! kubectl config get-contexts "$BUILDPLANE_CONTEXT" &>/dev/null; then
    error "Error: Build plane context '$BUILDPLANE_CONTEXT' not found in kubeconfig"
    echo "Available contexts:"
    kubectl config get-contexts -o name
    exit 1
  fi
fi

# Determine if we should use secretRef or extract CA value
CLIENT_CA_CONFIG=""

# Check if agent CA secret was explicitly provided
if [ "$AGENT_CA_SECRET_EXPLICIT" = true ]; then
  # User provided explicit secret reference - use secretRef mode
  # Set default namespace if not provided
  if [ -z "$AGENT_CA_NAMESPACE" ]; then
    AGENT_CA_NAMESPACE="$NAMESPACE"
  fi

  if ! kubectl --context="$CONTROL_PLANE_CONTEXT" get secret "$AGENT_CA_SECRET" -n "$AGENT_CA_NAMESPACE" >/dev/null 2>&1; then
    error "Agent CA secret '$AGENT_CA_SECRET' not found in namespace '$AGENT_CA_NAMESPACE'"
    echo ""
    echo "Please ensure the secret exists or omit --agent-ca-secret to auto-extract from build plane"
    echo ""
    exit 1
  fi

  echo "Using secret reference mode: $AGENT_CA_SECRET in namespace $AGENT_CA_NAMESPACE"
  CLIENT_CA_CONFIG="    clientCA:
      secretRef:
        name: $AGENT_CA_SECRET
        namespace: $AGENT_CA_NAMESPACE
        key: ca.crt"
else
  # Auto-extract mode - extract CA from build plane and use inline value
  echo "Extracting cluster agent client CA from build plane..."
  CLIENT_CA_CERT=$(kubectl --context="$BUILDPLANE_CONTEXT" get secret cluster-agent-tls -n openchoreo-build-plane -o jsonpath='{.data.ca\.crt}' 2>/dev/null | base64 -d)

  if [ -z "$CLIENT_CA_CERT" ]; then
    error "Failed to extract cluster agent client CA certificate from build plane"
    echo ""
    echo "Please ensure:"
    echo "  1. Build plane context '$BUILDPLANE_CONTEXT' is correct"
    echo "  2. Cluster agent is deployed in build plane (openchoreo-build-plane namespace)"
    echo "  3. The cluster-agent-tls secret exists and contains ca.crt"
    echo ""
    echo "Alternatively, use --agent-ca-secret to reference an existing secret in control plane"
    echo ""
    echo "You can check the status with:"
    echo "  kubectl --context=$BUILDPLANE_CONTEXT get secret cluster-agent-tls -n openchoreo-build-plane"
    echo ""
    exit 1
  fi
  CLIENT_CA_CONFIG="    clientCA:
      value: |
$(echo "$CLIENT_CA_CERT" | sed 's/^/        /')"
fi

# Build planeID field if specified
PLANE_ID_FIELD=""
if [ -n "$PLANE_ID" ]; then
  PLANE_ID_FIELD="  planeID: $PLANE_ID"
fi

# Generate the BuildPlane YAML with cluster agent configuration
BUILDPLANE_YAML=$(cat <<EOF
apiVersion: openchoreo.dev/v1alpha1
kind: BuildPlane
metadata:
  name: $BUILDPLANE_NAME
  namespace: $NAMESPACE
  annotations:
    openchoreo.dev/description: "BuildPlane created via $(basename $0) script with cluster agent"
    openchoreo.dev/display-name: "BuildPlane $BUILDPLANE_NAME"
spec:
$PLANE_ID_FIELD
  clusterAgent:
$CLIENT_CA_CONFIG
  secretStoreRef:
      name: openbao
EOF
)

# Apply or preview the manifest
if [ "$DRY_RUN" = true ]; then
  echo "$BUILDPLANE_YAML"
  exit 0
else
  # Ensure namespace exists
  if ! kubectl --context="$CONTROL_PLANE_CONTEXT" get namespace "$NAMESPACE" &>/dev/null; then
    echo "Creating namespace '$NAMESPACE'..."
    kubectl --context="$CONTROL_PLANE_CONTEXT" create namespace "$NAMESPACE"
  fi

  if echo "$BUILDPLANE_YAML" | kubectl --context="$CONTROL_PLANE_CONTEXT" apply -f - ; then
    echo ""
    echo "✓ BuildPlane '$BUILDPLANE_NAME' created successfully in namespace '$NAMESPACE'"
    echo ""
    echo "Next steps:"
    echo "  1. Ensure cluster agent is deployed in the build plane cluster"
    echo "  2. Verify the agent connects to the control plane's cluster gateway"
    echo "  3. Check BuildPlane status: kubectl --context=$CONTROL_PLANE_CONTEXT get buildplane $BUILDPLANE_NAME -n $NAMESPACE"
    echo ""
  else
    error "Failed to create BuildPlane resource"
    exit 1
  fi
fi
