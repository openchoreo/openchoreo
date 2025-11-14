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
log_info "  K3s Image: $K3S_IMAGE"
log_info "  Kubeconfig Path: $KUBECONFIG_PATH"
if [[ "$DEV_MODE" == "true" ]]; then
    log_info "  Mode: DEV (using local images and helm charts)"
elif [[ -n "$OPENCHOREO_VERSION" ]]; then
    log_info "  OpenChoreo Version: $OPENCHOREO_VERSION"
else
    log_info "  OpenChoreo Version: latest"
fi
log_info "  Enable Observability: $ENABLE_OBSERVABILITY"

# Verify prerequisites
verify_prerequisites

# Step 1: Create k3d cluster
create_k3d_cluster

# Step 2: Setup kubeconfig
setup_kubeconfig

# Step 3: Connect container to k3d network
connect_to_k3d_network

# Step 4: Pull and load docker images
prepare_images

# Step 5: Install OpenChoreo Control Plane
install_control_plane

# Step 6: Install OpenChoreo Data Plane
install_data_plane

# Step 7: Install OpenChoreo Observability Plane (optional)
if [[ "$ENABLE_OBSERVABILITY" == "true" ]]; then
    install_observability_plane
fi

# Step 8: Check installation status
if [[ "$SKIP_STATUS_CHECK" != "true" ]]; then
    bash "${SCRIPT_DIR}/check-status.sh"
fi

# Step 9: Add default dataplane
if [[ -f "${SCRIPT_DIR}/add-default-dataplane.sh" ]]; then
    bash "${SCRIPT_DIR}/add-default-dataplane.sh" --single-cluster
else
    log_warning "add-default-dataplane.sh not found, skipping dataplane configuration"
fi

log_success "OpenChoreo installation completed successfully!"
log_info "Access URLs:"
log_info "  Backstage UI: http://openchoreo.localhost:7007/"
log_info "    Logins:"
log_info "      Username: admin@openchoreo.dev"
log_info "      Password: Admin@123"
log_info "  OpenChoreo API: http://api.openchoreo.localhost:7007/"
log_info "  Thunder Identity Provider: http://thunder.openchoreo.localhost:7007/"
echo ""
log_info "Next Steps:"
log_info "  Deploy your first application by running:"
log_info "    ./deploy_web_application.sh"

exec /bin/bash -l
