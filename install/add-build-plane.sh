#!/bin/bash

# OpenChoreo BuildPlane Creation Script
# Creates a BuildPlane resource in the control plane using cluster agent mode

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
Create an OpenChoreo BuildPlane resource using cluster agent mode.

The cluster agent enables secure communication between the control plane and build plane
without requiring direct Kubernetes API access. The build plane cluster must have a
cluster agent installed and configured.

Usage:
  $script_name [OPTIONS]

Options:
  --control-plane-context CONTEXT   Kubernetes context where BuildPlane resource will be created
                                    Default: current context

  --name NAME                       Name for the BuildPlane resource
                                    Default: default

  --namespace NAMESPACE             Namespace for the BuildPlane resource
                                    Default: default

  --agent-ca-secret NAME            Secret name containing agent client CA certificate
                                    Default: cluster-gateway-ca

  --agent-ca-namespace NAMESPACE    Namespace of agent CA secret
                                    Default: same as --namespace

  --dry-run                         Preview the YAML without applying changes

  --help, -h                        Show this help message

Examples:

  # Create BuildPlane with defaults
  $script_name

  # Create BuildPlane with custom context
  $script_name --control-plane-context k3d-openchoreo

  # Create BuildPlane with custom name and namespace
  $script_name --name prod-buildplane --namespace production

  # Use custom agent CA secret
  $script_name --agent-ca-secret my-agent-ca --agent-ca-namespace openchoreo-control-plane

  # Preview without applying
  $script_name --dry-run

Note:
  This script creates a BuildPlane resource that uses cluster agent mode for communication.
  The build plane cluster must have:
    1. Cluster agent installed and running
    2. Network connectivity to the control plane cluster gateway
    3. Valid mTLS certificates for authentication

  For build plane agent installation, see:
    install/cluster-agent/README.md
EOF
}

# Defaults
CONTROL_PLANE_CONTEXT=""
BUILDPLANE_NAME="default"
NAMESPACE="default"
DRY_RUN=false
AGENT_CA_SECRET="cluster-gateway-ca"
AGENT_CA_NAMESPACE=""

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
  echo "Please configure kubectl or specify --control-plane-context explicitly"
  exit 1
fi

# Set defaults
if [ -z "$CONTROL_PLANE_CONTEXT" ]; then
  CONTROL_PLANE_CONTEXT="$CURRENT_CONTEXT"
fi

# Set agent CA namespace default
if [ -z "$AGENT_CA_NAMESPACE" ]; then
  AGENT_CA_NAMESPACE="$NAMESPACE"
fi

# Validate control plane context exists
if ! kubectl config get-contexts "$CONTROL_PLANE_CONTEXT" &>/dev/null; then
  error "Error: Control plane context '$CONTROL_PLANE_CONTEXT' not found in kubeconfig"
  echo "Available contexts:"
  kubectl config get-contexts -o name
  exit 1
fi

# Validate agent CA secret exists
if ! kubectl --context="$CONTROL_PLANE_CONTEXT" get secret "$AGENT_CA_SECRET" -n "$AGENT_CA_NAMESPACE" >/dev/null 2>&1; then
  error "Error: Cluster gateway CA secret '$AGENT_CA_SECRET' not found in namespace '$AGENT_CA_NAMESPACE'"
  echo ""
  echo "The cluster gateway CA secret is required for agent-based communication."
  echo "This secret should be created automatically by the cluster-gateway component."
  echo ""
  echo "Please ensure:"
  echo "  1. Cluster gateway is enabled in the control plane Helm chart"
  echo "  2. The cluster-gateway pod is running and healthy"
  echo "  3. The CA secret has been created"
  echo ""
  echo "You can check the status with:"
  echo "  kubectl --context=$CONTROL_PLANE_CONTEXT get pods -n openchoreo-control-plane | grep cluster-gateway"
  echo "  kubectl --context=$CONTROL_PLANE_CONTEXT get secret $AGENT_CA_SECRET -n $AGENT_CA_NAMESPACE"
  echo ""
  exit 1
fi

# Generate the BuildPlane YAML with agent configuration
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
  agent:
    enabled: true
    clientCA:
      secretRef:
        name: $AGENT_CA_SECRET
        namespace: $AGENT_CA_NAMESPACE
        key: ca.crt
EOF
)

# Apply or preview the manifest
if [ "$DRY_RUN" = true ]; then
  echo "$BUILDPLANE_YAML"
  echo ""
  echo "# To apply this configuration, run without --dry-run"
  exit 0
else
  echo "Creating BuildPlane resource '$BUILDPLANE_NAME' in namespace '$NAMESPACE'..."
  if echo "$BUILDPLANE_YAML" | kubectl --context="$CONTROL_PLANE_CONTEXT" apply -f - ; then
    echo ""
    echo "✓ BuildPlane '$BUILDPLANE_NAME' created successfully"
    echo ""
    echo "Next steps:"
    echo "  1. Ensure cluster agent is installed on the build plane cluster"
    echo "  2. Configure the agent with proper mTLS certificates"
    echo "  3. Verify agent connection: kubectl get buildplane $BUILDPLANE_NAME -n $NAMESPACE"
    echo ""
    echo "For build plane agent installation, see: install/cluster-agent/README.md"
  else
    error "Failed to create BuildPlane resource"
    exit 1
  fi
fi
