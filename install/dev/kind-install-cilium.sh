#!/usr/bin/env bash

# This pulls the Cilium images to the local docker context (if not exists), loads them into the kind cluster,
# and installs Cilium via Helm (version 1.18.2).
# usage: ./kind-install-cilium.sh [cluster-name]

set -eo pipefail

# Get script directory and source helpers
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/dev-helpers.sh"

# Configuration
CILIUM_VERSION="1.18.2"
CILIUM_NAMESPACE="cilium"

# Parse command line arguments
declare -A args=(
    [cluster_name]="$DEFAULT_CLUSTER_NAME"
    [show_help]=false
)

while [[ $# -gt 0 ]]; do
    case $1 in
        --cluster-name)
            args[cluster_name]="$2"
            shift 2
            ;;
        --help|-h)
            args[show_help]=true
            shift
            ;;
        *)
            log_error "Unknown option: $1"
            echo "Use --help for usage information"
            exit 1
            ;;
    esac
done

# Show help if requested
if [[ "${args[show_help]}" == "true" ]]; then
    show_help "$0" "Installs Cilium CNI in a Kind cluster" ""
    exit 0
fi

cluster_name="${args[cluster_name]}"

# Verify prerequisites
verify_prerequisites

# Check if cluster exists
if ! cluster_exists "$cluster_name"; then
    log_error "Kind cluster '${cluster_name}' does not exist"
    echo "Please create it first using: ./kind.sh ${cluster_name}"
    exit 1
fi

# Cilium images to load
images=(
  "quay.io/cilium/operator-generic:v${CILIUM_VERSION}"
  "quay.io/cilium/cilium:v${CILIUM_VERSION}"
  "quay.io/cilium/cilium-envoy:v1.34.4-1754895458-68cffdfa568b6b226d70a7ef81fc65dda3b890bf"
)

# Function to pull and load image
pull_and_load_image() {
    local image="$1"
    
    if ! docker image inspect "$image" > /dev/null 2>&1; then
        log_info "Image not found locally. Pulling image: $image"
        if ! docker pull "$image"; then
            log_error "Failed to pull image: $image"
            return 1
        fi
    else
        log_info "Image already exists locally: $image"
    fi

    log_info "Loading image: $image into kind cluster: $cluster_name"
    if ! kind load docker-image "$image" --name "$cluster_name"; then
        log_error "Failed to load image: $image"
        return 1
    fi
    
    return 0
}

# Process all images
log_info "Processing Cilium images..."
for image in "${images[@]}"; do
    if ! pull_and_load_image "$image"; then
        log_error "Failed to process image: $image"
        exit 1
    fi
done

log_success "All images have been successfully processed and loaded into the kind cluster: $cluster_name"

# Add Cilium Helm repository
log_info "Adding Cilium Helm repository..."
if ! helm repo add cilium https://helm.cilium.io/; then
    log_error "Failed to add Cilium Helm repository"
    exit 1
fi

if ! helm repo update; then
    log_error "Failed to update Helm repositories"
    exit 1
fi

# Get the Kubernetes API server IP from the kind cluster
log_info "Getting Kubernetes API server IP for cluster: $cluster_name"
K8S_API_IP=$(docker inspect "${cluster_name}-control-plane" --format='{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}')

if [ -z "$K8S_API_IP" ]; then
    log_error "Failed to get Kubernetes API server IP for cluster: $cluster_name"
    exit 1
fi

log_info "Kubernetes API server IP: $K8S_API_IP"

# Install Cilium via Helm with dynamic API server IP
log_info "Installing Cilium via Helm..."
if helm upgrade --install cilium cilium/cilium \
  --version "$CILIUM_VERSION" \
  --namespace "$CILIUM_NAMESPACE" \
  --values "${SCRIPT_DIR}/cilium-values.yaml" \
  --set k8sServiceHost="$K8S_API_IP" \
  --kube-context "kind-$cluster_name" \
  --create-namespace; then
    log_success "Cilium has been successfully installed in the kind cluster: $cluster_name"
    
    # Wait for Cilium pods to be ready
    wait_for_pods "$CILIUM_NAMESPACE" 300 "k8s-app=cilium"
else
    log_error "Failed to install Cilium"
    exit 1
fi
