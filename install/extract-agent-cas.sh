#!/bin/bash

# OpenChoreo Agent CA Certificate Extraction Script
# Extracts CA certificates for cluster agent mTLS configuration

set -e

# Color codes
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
RESET='\033[0m'

# Helper functions
error() {
  echo -e "${RED}Error: $*${RESET}" >&2
}

success() {
  echo -e "${GREEN}$*${RESET}"
}

info() {
  echo -e "${BLUE}$*${RESET}"
}

warn() {
  echo -e "${YELLOW}$*${RESET}"
}

show_help() {
  cat << EOF
Extract CA certificates for OpenChoreo cluster agent configuration.

Usage:
  $(basename "$0") [OPTIONS] COMMAND

Commands:
  server-ca              Extract cluster-gateway server CA from control plane
  dataplane-client-ca    Extract data plane agent client CA
  buildplane-client-ca   Extract build plane agent client CA
  all                    Extract all CAs (requires all contexts)

Options:
  --control-plane-context CONTEXT    Kubernetes context for control plane
                                     Default: k3d-openchoreo-cp

  --dataplane-context CONTEXT        Kubernetes context for data plane
                                     Default: k3d-openchoreo-dp

  --buildplane-context CONTEXT       Kubernetes context for build plane
                                     Default: k3d-openchoreo-bp

  --output-dir DIR                   Directory to save extracted certificates
                                     Default: ./agent-cas

  --verify                           Verify extracted certificates

  --help, -h                         Show this help message

Examples:
  # Extract server CA from control plane
  $(basename "$0") server-ca

  # Extract all CAs with custom contexts
  $(basename "$0") --control-plane-context prod-cp --dataplane-context prod-dp all

  # Extract and verify data plane client CA
  $(basename "$0") --verify dataplane-client-ca

  # Save to custom directory
  $(basename "$0") --output-dir /tmp/cas server-ca

EOF
}

# Defaults
CONTROL_PLANE_CONTEXT="k3d-openchoreo-cp"
DATAPLANE_CONTEXT="k3d-openchoreo-dp"
BUILDPLANE_CONTEXT="k3d-openchoreo-bp"
OUTPUT_DIR="./agent-cas"
VERIFY=false
COMMAND=""

# Parse arguments
while [[ $# -gt 0 ]]; do
  case $1 in
    --control-plane-context)
      CONTROL_PLANE_CONTEXT="$2"
      shift 2
      ;;
    --dataplane-context)
      DATAPLANE_CONTEXT="$2"
      shift 2
      ;;
    --buildplane-context)
      BUILDPLANE_CONTEXT="$2"
      shift 2
      ;;
    --output-dir)
      OUTPUT_DIR="$2"
      shift 2
      ;;
    --verify)
      VERIFY=true
      shift
      ;;
    --help|-h)
      show_help
      exit 0
      ;;
    server-ca|dataplane-client-ca|buildplane-client-ca|all)
      COMMAND="$1"
      shift
      ;;
    *)
      error "Unknown option: $1"
      echo "Use --help for usage information"
      exit 1
      ;;
  esac
done

# Validate command
if [ -z "$COMMAND" ]; then
  error "No command specified"
  show_help
  exit 1
fi

# Create output directory
mkdir -p "$OUTPUT_DIR"

# Verify certificate function
verify_cert() {
  local cert_file="$1"
  local cert_type="$2"

  if [ ! -f "$cert_file" ]; then
    error "Certificate file not found: $cert_file"
    return 1
  fi

  info "Verifying $cert_type..."
  echo ""

  if ! openssl x509 -in "$cert_file" -text -noout > /dev/null 2>&1; then
    error "Invalid certificate format"
    return 1
  fi

  openssl x509 -in "$cert_file" -text -noout | grep -E "Subject:|Issuer:|Not Before|Not After|CA:"
  echo ""
  success "✓ Certificate is valid"
  echo ""
}

# Extract server CA
extract_server_ca() {
  info "Extracting cluster-gateway server CA from control plane..."

  if ! kubectl --context="$CONTROL_PLANE_CONTEXT" get secret cluster-gateway-ca \
    -n openchoreo-control-plane > /dev/null 2>&1; then
    error "Cluster gateway CA secret not found in control plane"
    echo ""
    echo "Please ensure:"
    echo "  1. Control plane context '$CONTROL_PLANE_CONTEXT' is correct"
    echo "  2. Cluster gateway is installed and running"
    echo "  3. The cluster-gateway-ca secret exists in openchoreo-control-plane namespace"
    echo ""
    exit 1
  fi

  kubectl --context="$CONTROL_PLANE_CONTEXT" get secret cluster-gateway-ca \
    -n openchoreo-control-plane \
    -o jsonpath='{.data.ca\.crt}' | base64 -d > "$OUTPUT_DIR/server-ca.crt"

  success "✓ Extracted server CA to: $OUTPUT_DIR/server-ca.crt"

  if [ "$VERIFY" = true ]; then
    verify_cert "$OUTPUT_DIR/server-ca.crt" "Server CA"
  fi

  echo ""
  info "Usage:"
  echo "  Add this certificate to agent Helm values under 'clusterAgent.tls.serverCAValue'"
  echo ""
}

# Extract dataplane client CA
extract_dataplane_client_ca() {
  info "Extracting data plane agent client CA..."

  if ! kubectl --context="$DATAPLANE_CONTEXT" get secret cluster-agent-ca \
    -n openchoreo-data-plane > /dev/null 2>&1; then
    error "Cluster agent CA secret not found in data plane"
    echo ""
    echo "Please ensure:"
    echo "  1. Data plane context '$DATAPLANE_CONTEXT' is correct"
    echo "  2. Data plane with cluster agent is installed and running"
    echo "  3. The cluster-agent-ca secret exists in openchoreo-data-plane namespace"
    echo ""
    echo "Hint: The cluster agent must be deployed first to generate its CA certificate"
    echo ""
    exit 1
  fi

  kubectl --context="$DATAPLANE_CONTEXT" get secret cluster-agent-ca \
    -n openchoreo-data-plane \
    -o jsonpath='{.data.ca\.crt}' | base64 -d > "$OUTPUT_DIR/dataplane-client-ca.crt"

  success "✓ Extracted data plane client CA to: $OUTPUT_DIR/dataplane-client-ca.crt"

  if [ "$VERIFY" = true ]; then
    verify_cert "$OUTPUT_DIR/dataplane-client-ca.crt" "Data Plane Client CA"
  fi

  echo ""
  info "Next steps:"
  echo "  1. Create secret in control plane:"
  echo "     kubectl --context=$CONTROL_PLANE_CONTEXT create secret generic dataplane-default-ca \\"
  echo "       --from-file=ca.crt=$OUTPUT_DIR/dataplane-client-ca.crt -n default"
  echo ""
  echo "  2. Add to DataPlane CR spec:"
  echo "     agent:"
  echo "       enabled: true"
  echo "       clientCA:"
  echo "         secretRef:"
  echo "           name: dataplane-default-ca"
  echo "           namespace: default"
  echo "           key: ca.crt"
  echo ""
}

# Extract buildplane client CA
extract_buildplane_client_ca() {
  info "Extracting build plane agent client CA..."

  if ! kubectl --context="$BUILDPLANE_CONTEXT" get secret cluster-agent-ca \
    -n openchoreo-build-plane > /dev/null 2>&1; then
    error "Cluster agent CA secret not found in build plane"
    echo ""
    echo "Please ensure:"
    echo "  1. Build plane context '$BUILDPLANE_CONTEXT' is correct"
    echo "  2. Build plane with cluster agent is installed and running"
    echo "  3. The cluster-agent-ca secret exists in openchoreo-build-plane namespace"
    echo ""
    echo "Hint: The cluster agent must be deployed first to generate its CA certificate"
    echo ""
    exit 1
  fi

  kubectl --context="$BUILDPLANE_CONTEXT" get secret cluster-agent-ca \
    -n openchoreo-build-plane \
    -o jsonpath='{.data.ca\.crt}' | base64 -d > "$OUTPUT_DIR/buildplane-client-ca.crt"

  success "✓ Extracted build plane client CA to: $OUTPUT_DIR/buildplane-client-ca.crt"

  if [ "$VERIFY" = true ]; then
    verify_cert "$OUTPUT_DIR/buildplane-client-ca.crt" "Build Plane Client CA"
  fi

  echo ""
  info "Next steps:"
  echo "  1. Create secret in control plane:"
  echo "     kubectl --context=$CONTROL_PLANE_CONTEXT create secret generic buildplane-default-ca \\"
  echo "       --from-file=ca.crt=$OUTPUT_DIR/buildplane-client-ca.crt -n default"
  echo ""
  echo "  2. Add to BuildPlane CR spec:"
  echo "     agent:"
  echo "       enabled: true"
  echo "       clientCA:"
  echo "         secretRef:"
  echo "           name: buildplane-default-ca"
  echo "           namespace: default"
  echo "           key: ca.crt"
  echo ""
}

# Execute command
case $COMMAND in
  server-ca)
    extract_server_ca
    ;;
  dataplane-client-ca)
    extract_dataplane_client_ca
    ;;
  buildplane-client-ca)
    extract_buildplane_client_ca
    ;;
  all)
    extract_server_ca
    extract_dataplane_client_ca
    extract_buildplane_client_ca
    success "✓ All certificates extracted successfully"
    echo ""
    info "Summary:"
    ls -lh "$OUTPUT_DIR"/*.crt
    ;;
esac
