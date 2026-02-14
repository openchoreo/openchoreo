#!/usr/bin/env bash
set -eo pipefail

# Get the absolute path of the script directory
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Source helper functions
source "${SCRIPT_DIR}/.helpers.sh"

# Parse command line arguments
ENABLE_BUILD_PLANE=false
ENABLE_OBSERVABILITY=false
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

# Step 1: Verify prerequisites and create cluster
verify_prerequisites
create_k3d_cluster

# Step 2: Preload Docker images (unless skipped)
if [[ "$SKIP_PRELOAD" != "true" ]]; then
    preload_images
fi

# Step 3: Install prerequisites
install_cert_manager
install_eso
install_gateway_crds

# Step 4: Install kgateway and Thunder
install_kgateway
install_thunder

# Step 5: Apply CoreDNS config
apply_coredns_config

# Step 6: Create backstage secret and install Control Plane
create_backstage_secret "$CONTROL_PLANE_NS"
install_control_plane
patch_gateway_tmp_volume "$CONTROL_PLANE_NS"

# Step 7: Set up Data Plane CA and secret store
setup_data_plane_ca
create_fake_secret_store

# Step 8: Install Data Plane
install_data_plane
patch_gateway_tmp_volume "$DATA_PLANE_NS"

# Step 9: Create DataPlane resource and default resources
create_dataplane_resource
install_default_resources
label_default_namespace

# Step 10: Install Build Plane (optional)
if [[ "$ENABLE_BUILD_PLANE" == "true" ]]; then
    setup_build_plane_ca
    install_registry
    install_build_plane

    create_buildplane_resource
fi

# Step 11: Install Observability Plane (optional)
if [[ "$ENABLE_OBSERVABILITY" == "true" ]]; then
    setup_observability_plane_ca
    create_opensearch_secret "$OBSERVABILITY_NS"

    # Install kgateway in the observability namespace
    install_helm_chart "kgateway-op" "oci://cr.kgateway.dev/kgateway-dev/charts/kgateway" "$OBSERVABILITY_NS" "true" "false" "false" "300" \
        "--version" "$KGATEWAY_VERSION"

    install_observability_plane
    patch_gateway_tmp_volume "$OBSERVABILITY_NS"

    create_observabilityplane_resource

    configure_observabilityplane_reference
fi

# Step 12: Configure OCC CLI login (non-fatal, Thunder may need time to start)
if [[ -f "${SCRIPT_DIR}/occ-login.sh" ]]; then
    if ! bash "${SCRIPT_DIR}/occ-login.sh"; then
        log_warning "OCC CLI login setup failed (Thunder may not be ready yet)"
        log_info "You can retry later: bash occ-login.sh"
    fi
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
