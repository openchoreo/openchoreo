#!/usr/bin/env bash

# Helper functions for OpenChoreo installation
# These functions provide idempotent operations for setting up OpenChoreo

set -eo pipefail

# Source shared configuration
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/.config.sh"

# Color codes for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
BLUE='\033[0;34m'
RESET='\033[0m'

# Version configuration
# OPENCHOREO_VERSION is used for image tags (default: latest)
# OPENCHOREO_CHART_VERSION is derived from OPENCHOREO_VERSION
OPENCHOREO_VERSION="${OPENCHOREO_VERSION:-latest}"

# Dev mode configuration
DEV_MODE="${DEV_MODE:-false}"
DEV_HELM_CHARTS_DIR="/helm"

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

# Execute command with optional debug logging
# When DEBUG=true, logs the command before executing
run_command() {
    if [[ "${DEBUG:-false}" == "true" ]]; then
        log_info "Executing: $*"
    fi
    "$@"
}

# Function to derive chart version from image version
# This must be called AFTER OPENCHOREO_VERSION is set by the caller
derive_chart_version() {
    if [[ "$OPENCHOREO_VERSION" == "latest" ]]; then
        # Production latest: don't specify chart version (helm pulls latest)
        OPENCHOREO_CHART_VERSION=""
    elif [[ "$OPENCHOREO_VERSION" == "latest-dev" ]]; then
        # Development builds: use special dev chart version
        OPENCHOREO_CHART_VERSION="0.0.0-latest-dev"
    elif [[ "$OPENCHOREO_VERSION" =~ ^v[0-9]+\.[0-9]+\.[0-9]+ ]]; then
        # Release version with 'v' prefix: strip 'v' for chart version
        OPENCHOREO_CHART_VERSION="${OPENCHOREO_VERSION#v}"
    else
        # Assume it's already a valid chart version (e.g., "1.2.3")
        OPENCHOREO_CHART_VERSION="$OPENCHOREO_VERSION"
    fi
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

# Create k3d cluster with specific configuration
create_k3d_cluster() {
    if cluster_exists; then
        log_warning "k3d cluster '$CLUSTER_NAME' already exists, skipping creation"
        return 0
    fi

    log_info "Creating k3d cluster '$CLUSTER_NAME'..."

    # Use the k3d config file from user's home directory
    local k3d_config="$HOME/.k3d-config.yaml"

    if [[ ! -f "$k3d_config" ]]; then
        log_error "k3d config file not found at $k3d_config"
        return 1
    fi

    # Detect if running in Colima and disable k3d's DNS fix if needed
    # The DNS fix replaces Docker's embedded DNS (127.0.0.11) with the gateway IP,
    # which causes DNS timeouts in Colima due to firewall/network isolation.
    # k3d v5.9.0+ auto-detects Colima, but we handle it explicitly for older versions.
    # See https://github.com/k3d-io/k3d/issues/1449
    local dns_fix_env=""
    if docker info --format '{{.Name}}' 2>/dev/null | grep -qi "colima"; then
        log_info "Detected Colima runtime - disabling k3d DNS fix for compatibility"
        dns_fix_env="K3D_FIX_DNS=0"
    fi

    if run_command eval $dns_fix_env k3d cluster create "$CLUSTER_NAME" --config "$k3d_config" --wait; then
        log_success "k3d cluster '$CLUSTER_NAME' created successfully"
    else
        log_error "Failed to create k3d cluster '$CLUSTER_NAME'"
        return 1
    fi
}

# Install or upgrade a helm chart with idempotency
# Uses 'helm upgrade --install' which is the standard way to achieve idempotent installs
install_helm_chart() {
    local release_name="$1"
    local chart_name="$2"
    local namespace="$3"
    local create_namespace="${4:-true}"
    local wait_flag="${5:-false}"
    local timeout="${6:-1800}"
    shift 6
    local additional_args=("$@")

    log_info "Installing/upgrading Helm chart '$chart_name' as release '$release_name' in namespace '$namespace'..."

    # Determine chart reference based on dev mode
    local chart_ref
    if [[ "$DEV_MODE" == "true" && -d "$DEV_HELM_CHARTS_DIR/$chart_name" ]]; then
        chart_ref="$DEV_HELM_CHARTS_DIR/$chart_name"
        log_info "Using local chart from $chart_ref"
    else
        # For OCI repositories, construct the full chart reference
        chart_ref="${HELM_REPO}/${chart_name}"
    fi

    # Build helm upgrade --install command
    local helm_args=(
        "upgrade" "--install" "$release_name" "$chart_ref"
        "--namespace" "$namespace"
        "--timeout" "${timeout}s"
    )

    if [[ "$create_namespace" == "true" ]]; then
        helm_args+=("--create-namespace")
    fi

    if [[ "$wait_flag" == "true" ]]; then
        helm_args+=("--wait")
    fi

    if [[ -n "$OPENCHOREO_CHART_VERSION" && "$DEV_MODE" != "true" ]]; then
        helm_args+=("--version" "$OPENCHOREO_CHART_VERSION")
    fi

    helm_args+=("${additional_args[@]}")

    if run_command helm "${helm_args[@]}"; then
        log_success "Helm release '$release_name' installed/upgraded successfully"
    else
        log_error "Failed to install/upgrade Helm release '$release_name'"
        return 1
    fi
}

# Install OpenChoreo Control Plane
install_control_plane() {
    log_info "Installing OpenChoreo Control Plane..."
    install_helm_chart "openchoreo-control-plane" "openchoreo-control-plane" "$CONTROL_PLANE_NS" "true" "false" "1800" \
        "--values" "$HOME/.values-cp.yaml" \
        "--set" "controllerManager.image.tag=$OPENCHOREO_VERSION" \
        "--set" "openchoreoApi.image.tag=$OPENCHOREO_VERSION" \
        "--set" "backstage.image.tag=$OPENCHOREO_VERSION"
}

# Install OpenChoreo Data Plane
install_data_plane() {
    log_info "Installing OpenChoreo Data Plane..."
    install_helm_chart "openchoreo-data-plane" "openchoreo-data-plane" "$DATA_PLANE_NS" "true" "true" "1800" \
        "--values" "$HOME/.values-dp.yaml" \
        "--set" "observability.enabled=${ENABLE_OBSERVABILITY:-false}"
}


# Install OpenChoreo Build Plane (optional)
install_build_plane() {
    log_info "Installing OpenChoreo Build Plane..."
    install_helm_chart "openchoreo-build-plane" "openchoreo-build-plane" "$BUILD_PLANE_NS" "true" "true" "1800" \
        "--values" "$HOME/.values-bp.yaml"
}

# Install OpenChoreo Observability Plane (optional)
install_observability_plane() {
    log_info "Installing OpenChoreo Observability Plane..."
    install_helm_chart "openchoreo-observability-plane" "openchoreo-observability-plane" "$OBSERVABILITY_NS" "true" "true" "1800" \
        "--set" "observer.image.tag=$OPENCHOREO_VERSION"
}

# Print installation configuration
print_installation_config() {
    log_info "Configuration:"
    log_info "  Cluster Name: $CLUSTER_NAME"
    if [[ "$DEV_MODE" == "true" ]]; then
        log_info "  Mode: DEV (using local images and helm charts)"
    else
        log_info "  Image version: $OPENCHOREO_VERSION"
        log_info "  Chart version: ${OPENCHOREO_CHART_VERSION:-<latest from registry>}"
    fi
    log_info "  Enable Build Plane: ${ENABLE_BUILD_PLANE:-false}"
    log_info "  Enable Observability: ${ENABLE_OBSERVABILITY:-false}"
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

    if [[ ${#missing_tools[@]} -gt 0 ]]; then
        log_error "Missing required tools: ${missing_tools[*]}"
        return 1
    fi

    log_success "All prerequisites verified"
}


# =================================================================
# Docker image preloading function
# =================================================================

# Preload images to k3d using the .preload-images.sh script
preload_images() {
    local preload_script="${SCRIPT_DIR}/.preload-images.sh"

    if [[ ! -f "$preload_script" ]]; then
        log_warning "Image preload script not found at $preload_script"
        log_warning "Skipping image preloading - deployments may be slower"
        return 0
    fi

    log_info "Preloading Docker images for faster deployments..."

    # Build arguments for preload script
    local preload_args=(
        "--cluster" "$CLUSTER_NAME"
        "--control-plane"
        "--cp-values" "${SCRIPT_DIR}/.values-cp.yaml"
        "--data-plane"
        "--dp-values" "${SCRIPT_DIR}/.values-dp.yaml"
    )

    # Add --version flag only if OPENCHOREO_CHART_VERSION is not empty
    if [[ -n "$OPENCHOREO_CHART_VERSION" ]]; then
        preload_args+=("--version" "$OPENCHOREO_CHART_VERSION")
    fi

    # Add build plane if enabled
    if [[ "$ENABLE_BUILD_PLANE" == "true" ]]; then
        preload_args+=(
            "--build-plane"
            "--bp-values" "${SCRIPT_DIR}/.values-bp.yaml"
        )
    fi

    # Add observability plane if enabled
    if [[ "$ENABLE_OBSERVABILITY" == "true" ]]; then
        preload_args+=(
            "--observability-plane"
        )
    fi

    # Run the preload script
    if run_command bash "$preload_script" "${preload_args[@]}"; then
        log_success "Image preloading complete"
    else
        log_warning "Image preloading failed - continuing with installation"
        log_warning "Deployments may be slower due to image pulls"
    fi
}
