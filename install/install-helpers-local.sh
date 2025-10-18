#!/usr/bin/env bash

# Helper functions for OpenChoreo local installation

set -eo pipefail

# Color codes for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
BLUE='\033[0;34m'
RESET='\033[0m'

# Configuration variables - adapted for local use
CLUSTER_NAME="openchoreo-local"
NODE_IMAGE="kindest/node:v1.32.0@sha256:c48c62eac5da28cdadcf560d1d8616cfa6783b58f0d94cf63ad1bf49600cb027"
KUBECONFIG_PATH="${HOME}/.kube/config-openchoreo-local"
HELM_REPO_BASE="${SCRIPT_DIR}/helm"
OPENCHOREO_VERSION="${OPENCHOREO_VERSION:-}"

# Namespace definitions
CILIUM_NS="cilium"
CONTROL_PLANE_NS="openchoreo-control-plane"
DATA_PLANE_NS="openchoreo-data-plane"
BUILD_PLANE_NS="openchoreo-build-plane"
IDENTITY_NS="openchoreo-identity-system"
OBSERVABILITY_NS="openchoreo-observability-plane"

# Logging functions
log_info() {
    echo -e "${BLUE}[INFO]${RESET} $1"
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${RESET} $1"
}

log_warning() {
    echo -e "${YELLOW}[WARNING]${RESET} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${RESET} $1"
}

# Check if a command exists
command_exists() {
    command -v "$1" >/dev/null 2>&1
}

# Check if kind cluster exists
cluster_exists() {
    kind get clusters 2>/dev/null | grep -q "^${CLUSTER_NAME}$"
}

# Check if namespace exists
namespace_exists() {
    local namespace="$1"
    kubectl get namespace "$namespace" >/dev/null 2>&1
}

# Check if helm release exists
helm_release_exists() {
    local release="$1"
    local namespace="$2"
    helm list -n "$namespace" --short | grep -q "^${release}$"
}

# Wait for pods to be ready in a namespace
wait_for_pods() {
    local namespace="$1"
    local timeout="${2:-300}" # 5 minutes default
    local label_selector="${3:-}"
    
    log_info "Waiting for pods in namespace '$namespace' to be ready..."
    
    local selector_flag=""
    if [[ -n "$label_selector" ]]; then
        selector_flag="-l $label_selector"
    fi
    
    local elapsed=0
    local interval=5
    
    while [ $elapsed -lt $timeout ]; do
        if kubectl get pods -n "$namespace" $selector_flag --no-headers 2>/dev/null | grep -v 'Running\|Completed' | grep -q .; then
            echo 'Waiting for pods to be ready...'
            sleep $interval
            elapsed=$((elapsed + interval))
        else
            echo 'All pods are ready!'
            break
        fi
    done
    
    if [ $elapsed -ge $timeout ]; then
        log_error "Timeout waiting for pods in namespace '$namespace'"
        return 1
    fi
    
    log_success "All pods in namespace '$namespace' are ready"
}

# Create Kind cluster with specific configuration
create_kind_cluster() {
    if cluster_exists; then
        log_warning "Kind cluster '$CLUSTER_NAME' already exists, skipping creation"
        return 0
    fi
    
    log_info "Creating Kind cluster '$CLUSTER_NAME'..."
    
    # Create the /tmp/kind-shared directory if it doesn't exist
    mkdir -p /tmp/kind-shared
    
    # Create kind cluster config
    cat > /tmp/kind-config-local.yaml << EOF
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
nodes:
- role: control-plane
- role: worker
  labels:
    openchoreo.dev/noderole: workflow-runner
  extraMounts:
  - hostPath: /tmp/kind-shared
    containerPath: /mnt/shared
networking:
  disableDefaultCNI: true
EOF
    
    if kind create cluster --name "$CLUSTER_NAME" --image "$NODE_IMAGE" --config /tmp/kind-config-local.yaml; then
        log_success "Kind cluster '$CLUSTER_NAME' created successfully"
        rm -f /tmp/kind-config-local.yaml
    else
        log_error "Failed to create Kind cluster '$CLUSTER_NAME'"
        rm -f /tmp/kind-config-local.yaml
        return 1
    fi
}

# Export kubeconfig for the cluster
setup_kubeconfig() {
    log_info "Setting up kubeconfig..."
    
    # Create directory if it doesn't exist
    mkdir -p "$(dirname "$KUBECONFIG_PATH")"
    
    if kind export kubeconfig --name "$CLUSTER_NAME" --kubeconfig "$KUBECONFIG_PATH"; then
        log_success "Kubeconfig exported to $KUBECONFIG_PATH"
        export KUBECONFIG="$KUBECONFIG_PATH"
    else
        log_error "Failed to export kubeconfig"
        return 1
    fi
}

# Install a helm chart with idempotency - adapted for local helm charts
install_helm_chart() {
    local release_name="$1"
    local chart_name="$2"
    local namespace="$3"
    local create_namespace="${4:-true}"
    local wait_flag="${5:-false}"
    local timeout="${6:-1800}"
    shift 6
    local additional_args=("$@")
    
    log_info "Installing Helm chart '$chart_name' as release '$release_name' in namespace '$namespace'..."
    
    # Use local chart directory
    local chart_ref="${HELM_REPO_BASE}/${chart_name}"
    
    # Check if chart directory exists
    if [[ ! -d "$chart_ref" ]]; then
        log_error "Chart directory '$chart_ref' does not exist"
        return 1
    fi
    
    # Update dependencies first
    log_info "Updating Helm dependencies for '$chart_name'..."
    if ! helm dependency update "$chart_ref"; then
        log_warning "Failed to update dependencies for '$chart_name', continuing anyway"
    fi
    
    # Check if release already exists
    if helm_release_exists "$release_name" "$namespace"; then
        log_warning "Helm release '$release_name' already exists in namespace '$namespace'"
        
        # Try to upgrade the release
        local upgrade_args=(
            "upgrade" "$release_name" "$chart_ref"
            "--namespace" "$namespace"
            "--timeout" "${timeout}s"
        )
        
        if [[ "$wait_flag" == "true" ]]; then
            upgrade_args+=(--wait)
        fi
        
        upgrade_args+=("${additional_args[@]}")
        
        if helm "${upgrade_args[@]}"; then
            log_success "Helm release '$release_name' upgraded successfully"
        else
            log_error "Failed to upgrade Helm release '$release_name'"
            return 1
        fi
    else
        # Install new release
        local install_args=(
            "install" "$release_name" "$chart_ref"
            "--namespace" "$namespace"
            "--timeout" "${timeout}s"
        )
        
        if [[ "$create_namespace" == "true" ]]; then
            install_args+=(--create-namespace)
        fi
        
        if [[ "$wait_flag" == "true" ]]; then
            install_args+=(--wait)
        fi
        
        install_args+=("${additional_args[@]}")
        
        if helm "${install_args[@]}"; then
            log_success "Helm release '$release_name' installed successfully"
        else
            log_error "Failed to install Helm release '$release_name'"
            return 1
        fi
    fi
}

# Install Cilium
install_cilium() {
    log_info "Installing Cilium networking..."
    install_helm_chart "cilium" "cilium" "$CILIUM_NS" "true" "true" "1800"
    wait_for_pods "$CILIUM_NS" 300 "k8s-app=cilium"
}

# Install OpenChoreo Data Plane
install_data_plane() {
    log_info "Installing OpenChoreo Data Plane..."
    install_helm_chart "openchoreo-data-plane" "openchoreo-data-plane" "$DATA_PLANE_NS" "true" "false" "1800" \
        "--set" "cert-manager.enabled=false" \
        "--set" "cert-manager.crds.enabled=false"
}

# Install OpenChoreo Control Plane
install_control_plane() {
    log_info "Installing OpenChoreo Control Plane..."
    install_helm_chart "openchoreo-control-plane" "openchoreo-control-plane" "$CONTROL_PLANE_NS" "true" "false" "1800"
}

# Install OpenChoreo Build Plane
install_build_plane() {
    log_info "Installing OpenChoreo Build Plane..."
    install_helm_chart "openchoreo-build-plane" "openchoreo-build-plane" "$BUILD_PLANE_NS" "true" "false" "1800"
}

# Install OpenChoreo Platform Identity Provider (Control Plane)
install_platform_identity_provider() {
    log_info "Installing OpenChoreo Platform Identity Provider..."
    install_helm_chart "openchoreo-platform-identity-provider" "openchoreo-platform-identity-provider" "$CONTROL_PLANE_NS" "true" "false" "1800"
}

# Install OpenChoreo Workload Identity Provider
install_workload_identity_provider() {
    log_info "Installing OpenChoreo Workload Identity Provider..."
    install_helm_chart "openchoreo-workload-identity-provider" "openchoreo-workload-identity-provider" "$IDENTITY_NS" "true" "false" "1800"
}

# Install OpenChoreo Observability Plane (optional)
install_observability_plane() {
    log_info "Installing OpenChoreo Observability Plane..."
    install_helm_chart "openchoreo-observability-plane" "openchoreo-observability-plane" "$OBSERVABILITY_NS" "true" "false" "1800"
}

# Setup choreoctl auto-completion
setup_choreoctl_completion() {
    if [ -f "$KUBECONFIG_PATH" ]; then
        log_info "Enabling choreoctl auto-completion..."
        if command_exists choreoctl && choreoctl completion bash > ~/.choreoctl-completion 2>/dev/null; then
            chmod +x ~/.choreoctl-completion
            if ! grep -q "source ~/.choreoctl-completion" ~/.bashrc 2>/dev/null; then
                echo "source ~/.choreoctl-completion" >> ~/.bashrc
            fi
            log_success "choreoctl auto-completion enabled"
        else
            log_warning "choreoctl not found or completion failed, skipping auto-completion setup"
        fi
    else
        log_warning "Kubeconfig not found, skipping choreoctl auto-completion setup"
    fi
}

# Verify prerequisites
verify_prerequisites() {
    log_info "Verifying prerequisites..."
    
    local missing_tools=()
    
    if ! command_exists kind; then
        missing_tools+=("kind")
    fi
    
    if ! command_exists kubectl; then
        missing_tools+=("kubectl")
    fi
    
    if ! command_exists helm; then
        missing_tools+=("helm")
    fi
    
    if ! command_exists docker; then
        missing_tools+=("docker")
    fi
    
    if [[ ${#missing_tools[@]} -gt 0 ]]; then
        log_error "Missing required tools: ${missing_tools[*]}"
        return 1
    fi
    
    log_success "All prerequisites verified"
}

# Clean up function
cleanup() {
    log_info "Cleaning up temporary files..."
    rm -f /tmp/kind-config-local.yaml
}

# Register cleanup function
trap cleanup EXIT
