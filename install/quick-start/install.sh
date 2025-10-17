#!/usr/bin/env bash
set -eo pipefail

# Get the absolute path of the script directory
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Source helper functions
source "${SCRIPT_DIR}/install-helpers.sh"

# Parse command line arguments
ENABLE_OBSERVABILITY=false
SKIP_STATUS_CHECK=false

while [[ $# -gt 0 ]]; do
    case $1 in
        --enable-observability)
            ENABLE_OBSERVABILITY=true
            shift
            ;;
        --skip-status-check)
            SKIP_STATUS_CHECK=true
            shift
            ;;
        --openchoreo-version)
            OPENCHOREO_VERSION="$2"
            export OPENCHOREO_VERSION
            shift 2
            ;;
        --help|-h)
            echo "Usage: $0 [OPTIONS]"
            echo ""
            echo "Options:"
            echo "  --enable-observability    Enable OpenChoreo Observability Plane"
            echo "  --skip-status-check       Skip the status check at the end"
            echo "  --openchoreo-version VER  Specify OpenChoreo version to install"
            echo "  --help, -h                Show this help message"
            echo ""
            echo "Examples:"
            echo "  $0                                    # Install with defaults (latest version)"
            echo "  $0 --enable-observability             # Install with observability plane"
            echo "  $0 --openchoreo-version v1.2.3        # Install specific version"
            exit 0
            ;;
        *)
            log_error "Unknown option: $1"
            echo "Use --help for usage information"
            exit 1
            ;;
    esac
done

log_info "Starting OpenChoreo installation..."
log_info "Configuration:"
log_info "  Cluster Name: $CLUSTER_NAME"
log_info "  Node Image: $NODE_IMAGE"
log_info "  Kubeconfig Path: $KUBECONFIG_PATH"
if [[ -n "$OPENCHOREO_VERSION" ]]; then
    log_info "  OpenChoreo Version: $OPENCHOREO_VERSION"
else
    log_info "  OpenChoreo Version: latest"
fi
log_info "  Enable Observability: $ENABLE_OBSERVABILITY"

# Verify prerequisites
verify_prerequisites

# Step 1: Connect container to kind network
connect_to_kind_network

# Step 2: Create Kind cluster
create_kind_cluster

# Step 3: Setup kubeconfig
setup_kubeconfig

# Step 4: Install Cilium (networking)
install_cilium

# Step 5: Install OpenChoreo Control Plane
install_control_plane

# Step 5.1: Install Platform Identity Provider (part of control plane)
install_platform_identity_provider

# Step 6-8: Install OpenChoreo Data Plane, Build Plane, and Workload Identity Provider in parallel
log_info "Installing Data Plane, Build Plane, and Workload Identity Provider in parallel..."

# Start installations in background
install_data_plane &
DATA_PLANE_PID=$!

install_build_plane &
BUILD_PLANE_PID=$!

install_workload_identity_provider &
WORKLOAD_IDENTITY_PID=$!

# Wait for all installations to complete
log_info "Waiting for parallel installations to complete..."
wait $DATA_PLANE_PID
DATA_PLANE_EXIT=$?

wait $BUILD_PLANE_PID
BUILD_PLANE_EXIT=$?

wait $WORKLOAD_IDENTITY_PID
WORKLOAD_IDENTITY_EXIT=$?

# Check if any installation failed
if [[ $DATA_PLANE_EXIT -ne 0 ]]; then
    log_error "Data Plane installation failed with exit code $DATA_PLANE_EXIT"
    exit 1
fi

if [[ $BUILD_PLANE_EXIT -ne 0 ]]; then
    log_error "Build Plane installation failed with exit code $BUILD_PLANE_EXIT"
    exit 1
fi

if [[ $WORKLOAD_IDENTITY_EXIT -ne 0 ]]; then
    log_error "Workload Identity Provider installation failed with exit code $WORKLOAD_IDENTITY_EXIT"
    exit 1
fi

log_info "All parallel installations completed successfully"

# Step 9: Install OpenChoreo Observability Plane (optional)
if [[ "$ENABLE_OBSERVABILITY" == "true" ]]; then
    install_observability_plane
fi

# Step 10: Install Backstage Demo
install_backstage_demo

# Step 11: Setup port forwarding
setup_port_forwarding

# Step 12: Setup choreoctl auto-completion
setup_choreoctl_completion

# Step 13: Check installation status
if [[ "$SKIP_STATUS_CHECK" != "true" ]]; then
    bash "${SCRIPT_DIR}/check-status.sh"
fi

# Step 14: Add default dataplane
if [[ -f "${SCRIPT_DIR}/add-default-dataplane.sh" ]]; then
    bash "${SCRIPT_DIR}/add-default-dataplane.sh" --single-cluster
else
    log_warning "add-default-dataplane.sh not found, skipping dataplane configuration"
fi

# Step 15: Add default BuildPlane
if [[ -f "${SCRIPT_DIR}/add-build-plane.sh" ]]; then
    bash "${SCRIPT_DIR}/add-build-plane.sh"
else
    log_warning "add-build-plane.sh not found, skipping build plane configuration"
fi

log_success "OpenChoreo installation completed successfully!"
log_info "Access URLs:"
log_info "  External Gateway: https://localhost:8443"
log_info "  Backstage Demo: http://localhost:7007"

exec /bin/bash -l
