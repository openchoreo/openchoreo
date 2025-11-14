#!/usr/bin/env bash
set -eo pipefail

# Get the absolute path of the script directory
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Source helper functions
source "${SCRIPT_DIR}/.helpers.sh"

# Parse command line arguments
ENABLE_OBSERVABILITY=false
SKIP_STATUS_CHECK=false
DEBUG=false

while [[ $# -gt 0 ]]; do
    case $1 in
        --with-observability)
            ENABLE_OBSERVABILITY=true
            shift
            ;;
        --skip-status-check)
            SKIP_STATUS_CHECK=true
            shift
            ;;
        --debug)
            DEBUG=true
            export DEBUG
            shift
            ;;
        --version)
            OPENCHOREO_VERSION="$2"
            export OPENCHOREO_VERSION
            shift 2
            ;;
        --help|-h)
            echo "Usage: $0 [OPTIONS]"
            echo ""
            echo "Options:"
            echo "  --version VER             Specify version to install (default: latest)"
            echo "  --with-observability      Install with Observability Plane"
            echo "  --skip-status-check       Skip status check at the end"
            echo "  --debug                   Enable debug mode"
            echo "  --help, -h                Show this help message"
            echo ""
            echo "Examples:"
            echo "  $0                               # Install with defaults"
            echo "  $0 --version v1.2.3              # Install specific version"
            echo "  $0 --with-observability          # Install with observability"
            echo "  $0 --debug --version latest-dev  # Debug with dev version"
            exit 0
            ;;
        *)
            log_error "Unknown option: $1"
            echo "Use --help for usage information"
            exit 1
            ;;
    esac
done

# Derive chart version from the (possibly user-provided) OPENCHOREO_VERSION
derive_chart_version

log_info "Starting OpenChoreo installation..."
print_installation_config

# Verify prerequisites
verify_prerequisites

# Step 1: Create k3d cluster
create_k3d_cluster

# Step 2: Install OpenChoreo Control Plane
install_control_plane

# Step 3: Install OpenChoreo Data Plane
install_data_plane

# Step 4: Install OpenChoreo Observability Plane (optional)
if [[ "$ENABLE_OBSERVABILITY" == "true" ]]; then
    install_observability_plane
fi

# Step 5: Check installation status
if [[ "$SKIP_STATUS_CHECK" != "true" ]]; then
    bash "${SCRIPT_DIR}/check-status.sh"
fi

# Step 6: Add default dataplane
if [[ -f "${SCRIPT_DIR}/add-data-plane.sh" ]]; then
    bash "${SCRIPT_DIR}/add-data-plane.sh"
else
    log_warning "add-data-plane.sh not found, skipping dataplane configuration"
fi

log_success "OpenChoreo installation completed successfully!"
# TODO: Uncomment and update access URLs when backstage is available
#log_info "Access URLs:"
#log_info "  Backstage UI: http://openchoreo.localhost:8080/"
#log_info "    Logins:"
#log_info "      Username: admin@openchoreo.dev"
#log_info "      Password: Admin@123"
#log_info "  OpenChoreo API: http://api.openchoreo.localhost:8080/"
#log_info "  Thunder Identity Provider: http://thunder.openchoreo.localhost:8080/"
#echo ""
log_info "Next Steps:"
log_info "  Deploy sample applications:"
log_info "    ./deploy-react-starter.sh    # Simple React web application"
log_info "    ./deploy-gcp-demo.sh         # GCP Microservices Demo (11 services)"
