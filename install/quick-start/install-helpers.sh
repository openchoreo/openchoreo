#!/usr/bin/env bash

# Helper functions for OpenChoreo installation
# These functions provide idempotent operations for setting up OpenChoreo

set -eo pipefail

# Color codes for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
BLUE='\033[0;34m'
RESET='\033[0m'

# Configuration variables
CLUSTER_NAME="openchoreo-quick-start"
K3S_IMAGE="rancher/k3s:v1.31.5-k3s1"
KUBECONFIG_PATH="/state/kube/config-internal.yaml"
HELM_REPO="oci://ghcr.io/openchoreo/helm-charts"
OPENCHOREO_VERSION="${OPENCHOREO_VERSION:-}"

# Namespace definitions
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

# Check if k3d cluster exists
cluster_exists() {
    k3d cluster list 2>/dev/null | grep -q "^${CLUSTER_NAME} "
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
    
    if ! timeout "$timeout" bash -c "
        while true; do
            if kubectl get pods -n '$namespace' $selector_flag --no-headers 2>/dev/null | grep -v 'Running\|Completed' | grep -q .; then
                echo 'Waiting for pods to be ready...'
                sleep 5
            else
                echo 'All pods are ready!'
                break
            fi
        done
    "; then
        log_error "Timeout waiting for pods in namespace '$namespace'"
        return 1
    fi
    
    log_success "All pods in namespace '$namespace' are ready"
}

# Create k3d cluster with specific configuration
create_k3d_cluster() {
    if cluster_exists; then
        log_warning "k3d cluster '$CLUSTER_NAME' already exists, skipping creation"
        return 0
    fi
    
    log_info "Creating k3d cluster '$CLUSTER_NAME'..."
    
    # Create the /tmp/k3d-shared directory if it doesn't exist
    mkdir -p /tmp/k3d-shared
    
    if k3d cluster create "$CLUSTER_NAME" \
        --image "$K3S_IMAGE" \
        --agents 1 \
        --servers 1 \
        --volume /tmp/k3d-shared:/mnt/shared@agent:0 \
        --k3s-arg "--disable=traefik@server:*" \
        --k3s-arg "--disable=metrics-server@server:0" \
        --wait; then
        log_success "k3d cluster '$CLUSTER_NAME' created successfully"
    else
        log_error "Failed to create k3d cluster '$CLUSTER_NAME'"
        return 1
    fi
}

# Export kubeconfig for the cluster
setup_kubeconfig() {
    log_info "Setting up kubeconfig..."
    
    # Create directory if it doesn't exist
    mkdir -p "$(dirname "$KUBECONFIG_PATH")"
    
    if k3d kubeconfig get "$CLUSTER_NAME" > "$KUBECONFIG_PATH"; then
        log_success "Kubeconfig exported to $KUBECONFIG_PATH"
        export KUBECONFIG="$KUBECONFIG_PATH"
    else
        log_error "Failed to export kubeconfig"
        return 1
    fi
}

# Connect container to k3d network
connect_to_k3d_network() {
    local container_id
    container_id="$(cat /etc/hostname)"
    
    log_info "Connecting container to k3d network..."
    
    # Check if the "k3d-$CLUSTER_NAME" network exists
    if ! docker network inspect "k3d-${CLUSTER_NAME}" &>/dev/null; then
        log_warning "Docker network 'k3d-${CLUSTER_NAME}' does not exist yet. Will be created with cluster."
        return 0
    fi
    
    # Check if the container is already connected
    if [ "$(docker inspect -f '{{json .NetworkSettings.Networks.k3d-'"${CLUSTER_NAME}"'}}' "${container_id}")" = "null" ]; then
        if docker network connect "k3d-${CLUSTER_NAME}" "${container_id}"; then
            log_success "Connected container ${container_id} to k3d-${CLUSTER_NAME} network"
        else
            log_error "Failed to connect container to k3d network"
            return 1
        fi
    else
        log_warning "Container ${container_id} is already connected to k3d-${CLUSTER_NAME} network"
    fi
}

# Install a helm chart with idempotency
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
    
    # For OCI repositories, construct the full chart reference
    local chart_ref="${HELM_REPO}/${chart_name}"
    
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
            upgrade_args+=("--wait")
        fi
        
        if [[ -n "$OPENCHOREO_VERSION" ]]; then
            upgrade_args+=("--version" "$OPENCHOREO_VERSION")
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
            install_args+=("--create-namespace")
        fi
        
        if [[ "$wait_flag" == "true" ]]; then
            install_args+=("--wait")
        fi
        
        if [[ -n "$OPENCHOREO_VERSION" ]]; then
            install_args+=("--version" "$OPENCHOREO_VERSION")
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

# Install OpenChoreo Identity Provider
install_identity_provider() {
    log_info "Installing OpenChoreo Identity Provider..."
    install_helm_chart "openchoreo-identity-provider" "openchoreo-identity-provider" "$IDENTITY_NS" "true" "false" "1800"
}

# Install OpenChoreo Observability Plane (optional)
install_observability_plane() {
    log_info "Installing OpenChoreo Observability Plane..."
    install_helm_chart "openchoreo-observability-plane" "openchoreo-observability-plane" "$OBSERVABILITY_NS" "true" "false" "1800"
}

# Install Backstage Demo
install_backstage_demo() {
    log_info "Installing Backstage Demo..."
    install_helm_chart "openchoreo-backstage-demo" "backstage-demo" "$CONTROL_PLANE_NS" "false" "false" "1800" \
        "--set" "backstage.service.type=NodePort"
}

# Setup port forwarding for services
setup_port_forwarding() {
    log_info "Setting up port forwarding..."
    
    # Kill existing socat processes
    pkill socat 2>/dev/null || true
    
    log_info "Finding external gateway nodeport..."
    local nodeport_eg
    for i in {1..30}; do
        nodeport_eg=$(kubectl get svc -n "$DATA_PLANE_NS" -l gateway.envoyproxy.io/owning-gateway-name=gateway-external \
            -o jsonpath='{.items[0].spec.ports[0].nodePort}' 2>/dev/null) || true
        
        if [[ -n "$nodeport_eg" ]]; then
            break
        fi
        
        log_info "Waiting for external gateway service... (attempt $i/30)"
        sleep 10
    done
    
    if [[ -z "$nodeport_eg" ]]; then
        log_error "Could not retrieve external gateway NodePort"
        return 1
    fi
    
    log_info "Setting up port-forwarding proxy from 8443 to the gateway NodePort..."
    socat TCP-LISTEN:8443,fork TCP:k3d-${CLUSTER_NAME}-agent-0:$nodeport_eg &
    
    log_info "Finding backstage nodeport..."
    local nodeport_backstage
    for i in {1..30}; do
        nodeport_backstage=$(kubectl get svc -n "$CONTROL_PLANE_NS" -l app.kubernetes.io/component=backstage \
            -o jsonpath='{.items[0].spec.ports[0].nodePort}' 2>/dev/null) || true
        
        if [[ -n "$nodeport_backstage" ]]; then
            break
        fi
        
        log_info "Waiting for backstage service... (attempt $i/30)"
        sleep 10
    done
    
    if [[ -z "$nodeport_backstage" ]]; then
        log_error "Could not retrieve Backstage NodePort"
        return 1
    fi
    
    log_info "Setting up port-forwarding proxy from 7007 to the Backstage NodePort..."
    socat TCP-LISTEN:7007,fork TCP:k3d-${CLUSTER_NAME}-agent-0:$nodeport_backstage &
    
    log_success "Port forwarding setup complete"
}

# Setup choreoctl auto-completion
setup_choreoctl_completion() {
    if [ -f "$KUBECONFIG_PATH" ]; then
        log_info "Enabling choreoctl auto-completion..."
        if /usr/local/bin/choreoctl completion bash > /usr/local/bin/choreoctl-completion; then
            chmod +x /usr/local/bin/choreoctl-completion
            if ! grep -q "source /usr/local/bin/choreoctl-completion" /etc/profile; then
                echo "source /usr/local/bin/choreoctl-completion" >> /etc/profile
            fi
            log_success "choreoctl auto-completion enabled"
        else
            log_warning "Failed to setup choreoctl auto-completion"
        fi
    else
        log_warning "Kubeconfig not found, skipping choreoctl auto-completion setup"
    fi
}

# Verify prerequisites
verify_prerequisites() {
    log_info "Verifying prerequisites..."
    
    local missing_tools=()
    
    if ! command_exists k3d; then
        missing_tools+=("k3d")
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
    
    if ! command_exists socat; then
        missing_tools+=("socat")
    fi
    
    if [[ ${#missing_tools[@]} -gt 0 ]]; then
        log_error "Missing required tools: ${missing_tools[*]}"
        return 1
    fi
    
    log_success "All prerequisites verified"
}


# Get list of docker images used by OpenChoreo
get_openchoreo_images() {
    # Core images that are always needed
    local images=(
        # Control Plane
        "docker.io/rancher/local-path-provisioner:v0.0.30"
        "docker.io/rancher/mirrored-coredns-coredns:1.12.0"
        "ghcr.io/openchoreo/controller:v0.3.2"
        "ghcr.io/openchoreo/openchoreo-api:v0.3.2"
        "quay.io/jetstack/cert-manager-webhook:v1.16.2"
        "quay.io/jetstack/cert-manager-cainjector:v1.16.2"
    )
    
    # Add observability images if enabled
    if [[ "$ENABLE_OBSERVABILITY" == "true" ]]; then
        images+=(
            "docker.io/opensearchproject/opensearch:2.18.0"
            "docker.io/opensearchproject/opensearch-dashboards:2.18.0"
        )
    fi
    
    echo "${images[@]}"
}

# Pull docker images
pull_images() {
    log_info "Pulling required docker images..."
    
    local images=($@)
    local total=${#images[@]}
    local current=0
    
    for image in "${images[@]}"; do
        current=$((current + 1))
        log_info "[$current/$total] Pulling $image..."
        
        if docker pull "$image" >/dev/null 2>&1; then
            log_success "Pulled $image"
        else
            log_warning "Failed to pull $image (may already exist locally)"
        fi
    done
    
    log_success "Image pull complete"
}

# Load images into k3d cluster
load_images_to_cluster() {
    if ! cluster_exists; then
        log_error "Cluster '$CLUSTER_NAME' does not exist. Cannot load images."
        return 1
    fi
    
    log_info "Loading images into k3d cluster..."
    
    local images=($@)
    local total=${#images[@]}
    local current=0
    
    for image in "${images[@]}"; do
        current=$((current + 1))
        log_info "[$current/$total] Loading $image into cluster..."
        
        if k3d image import "$image" -c "$CLUSTER_NAME" >/dev/null 2>&1; then
            log_success "Loaded $image"
        else
            log_warning "Failed to load $image (may already exist in cluster)"
        fi
    done
    
    log_success "All images loaded into cluster"
}

# Pull and load all required images
prepare_images() {
    log_info "Preparing docker images for installation..."
    
    local images=($(get_openchoreo_images))
    
    # Pull images
    pull_images "${images[@]}"
    
    # Load images into cluster
    load_images_to_cluster "${images[@]}"
    
    log_success "Image preparation complete"
}

# Clean up function
cleanup() {
    log_info "Cleaning up temporary files..."
    rm -f /tmp/k3d-config.yaml
}

# Register cleanup function
trap cleanup EXIT
