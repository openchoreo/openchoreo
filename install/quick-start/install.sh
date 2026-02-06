#!/usr/bin/env bash
set -eo pipefail

# Get the absolute path of the script directory
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Source helper functions
source "${SCRIPT_DIR}/.helpers.sh"

# Parse command line arguments
ENABLE_BUILD_PLANE=false
ENABLE_OBSERVABILITY=false
SKIP_STATUS_CHECK=false
SKIP_PRELOAD=false
SKIP_RESOURCE_CHECK=false
DEBUG=false

while [[ $# -gt 0 ]]; do
    case $1 in
        --with-build)
            ENABLE_BUILD_PLANE=true
            shift
            ;;
        --with-observability)
            ENABLE_OBSERVABILITY=true
            shift
            ;;
        --skip-status-check)
            SKIP_STATUS_CHECK=true
            shift
            ;;
        --skip-preload)
            SKIP_PRELOAD=true
            shift
            ;;
        --skip-resource-check)
            SKIP_RESOURCE_CHECK=true
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
            echo "  --with-build              Install with Build Plane (Argo Workflows + Registry)"
            echo "  --with-observability      Install with Observability Plane"
            echo "  --skip-status-check       Skip status check at the end"
            echo "  --skip-preload            Skip image preloading from host Docker"
            echo "  --skip-resource-check     Skip system resource validation"
            echo "  --debug                   Enable debug mode"
            echo "  --help, -h                Show this help message"
            echo ""
            echo "Examples:"
            echo "  $0                                     # Install with defaults"
            echo "  $0 --version v1.2.3                    # Install specific version"
            echo "  $0 --with-build                        # Install with build capabilities"
            echo "  $0 --with-observability                # Install with observability"
            echo "  $0 --with-build --with-observability   # Full platform"
            echo "  $0 --skip-preload                      # Skip image preloading"
            echo "  $0 --debug --version latest-dev        # Debug with dev version"
            exit 0
            ;;
        *)
            log_error "Unknown option: $1"
            echo "Use --help for usage information"
            exit 1
            ;;
    esac
done

# Export flags for use in helper functions
export SKIP_RESOURCE_CHECK
export ENABLE_BUILD_PLANE
export ENABLE_OBSERVABILITY

# Derive chart version from the (possibly user-provided) OPENCHOREO_VERSION
derive_chart_version

log_info "Starting OpenChoreo installation..."
print_installation_config

# Verify prerequisites
verify_prerequisites

# Step 1: Create k3d cluster
create_k3d_cluster

# Step 2: Preload Docker images (unless skipped)
if [[ "$SKIP_PRELOAD" != "true" ]]; then
    preload_images
fi

# Step 3: Install cert-manager (prerequisite for TLS certificate management)
install_cert_manager

# Step 3.5: Install External Secrets Operator (prerequisite for secret management)
install_eso

# Step 4: Install OpenChoreo Control Plane
install_control_plane

# Step 5: Install OpenChoreo Data Plane
install_data_plane

# Step 6: Create TLS certificates for gateways
create_control_plane_certificate
create_data_plane_certificate

# Step 7: Install Container Registry and Build Plane (optional)
if [[ "$ENABLE_BUILD_PLANE" == "true" ]]; then
    install_registry
    install_build_plane
fi

# Step 8: Install OpenChoreo Observability Plane (optional)
if [[ "$ENABLE_OBSERVABILITY" == "true" ]]; then
    install_observability_plane
fi

# Step 9: Check installation status
if [[ "$SKIP_STATUS_CHECK" != "true" ]]; then
    bash "${SCRIPT_DIR}/check-status.sh"
fi

# Step 10: Add default dataplane
if [[ -f "${SCRIPT_DIR}/add-data-plane.sh" ]]; then
    bash "${SCRIPT_DIR}/add-data-plane.sh" --name default
else
    log_warning "add-data-plane.sh not found, skipping dataplane configuration"
fi

# Step 11: Install default OpenChoreo resources (Project, Environments, ComponentTypes, etc.)
install_default_resources

# Step 12: Label default namespace
label_default_namespace

# Step 13: Add default buildplane (if build plane enabled)
if [[ "$ENABLE_BUILD_PLANE" == "true" ]]; then
    if [[ -f "${SCRIPT_DIR}/add-build-plane.sh" ]]; then
        bash "${SCRIPT_DIR}/add-build-plane.sh" --name default
    else
        log_warning "add-build-plane.sh not found, skipping buildplane configuration"
    fi
fi

# Step 14: Add default observabilityplane (if observability plane enabled)
if [[ "$ENABLE_OBSERVABILITY" == "true" ]]; then
    if [[ -f "${SCRIPT_DIR}/add-observability-plane.sh" ]]; then
        bash "${SCRIPT_DIR}/add-observability-plane.sh" --name default
    else
        log_warning "add-observability-plane.sh not found, skipping observabilityplane configuration"
    fi
fi

# Step 15: Configure the dataplane and buildplane with observabilityplane reference
if [[ "$ENABLE_OBSERVABILITY" == "true" ]]; then
    configure_observabilityplane_reference
fi

# Step 16: Configure OCC CLI login
if [[ -f "${SCRIPT_DIR}/occ-login.sh" ]]; then
    bash "${SCRIPT_DIR}/occ-login.sh"
else
    log_warning "occ-login.sh not found, skipping OCC CLI login"
fi

log_success "OpenChoreo installation completed successfully!"
log_info "Access URLs:"
log_info "  Backstage UI: http://openchoreo.localhost:8080/"
log_info "    Logins:"
log_info "      Username: admin@openchoreo.dev"
log_info "      Password: Admin@123"
log_info "  OpenChoreo API: http://api.openchoreo.localhost:8080/"
log_info "  Thunder Identity Provider: http://thunder.openchoreo.localhost:8080/"
log_info "  Thunder Identity Provider UI: http://thunder.openchoreo.localhost:8080/develop"
log_info "    Logins:"
log_info "      Username: admin"
log_info "      Password: admin"
echo ""
log_info "OCC CLI Login:"
log_info "  Run the following commands to login:"
log_info "    source .occ-credentials"
log_info "    occ login --client-credentials --url http://api.openchoreo.localhost:8080"
echo ""
log_info "Next Steps:"
log_info "  Deploy sample applications:"
log_info "    ./deploy-react-starter.sh      # Simple React web application"
log_info "    ./deploy-gcp-demo.sh           # GCP Microservices Demo (11 services)"

if [[ "$ENABLE_BUILD_PLANE" == "true" ]]; then
    log_info "    ./build-deploy-greeter.sh      # Build from source (Go greeter service)"
fi
