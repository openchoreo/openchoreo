#!/usr/bin/env bash

# This script builds all OpenChoreo components (controller, API, UI), loads them into the kind cluster,
# and installs OpenChoreo with CRDs using Helm into the openchoreo namespace.
# usage: ./kind-install-openchoreo.sh [cluster-name]

set -eo pipefail

# Get script directory and source helpers
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/dev-helpers.sh"

# Get project root
PROJECT_ROOT="$(get_project_root)"
HELM_DIR="${PROJECT_ROOT}/install/helm/openchoreo"

# Parse command line arguments
declare -A args=(
    [cluster_name]="$DEFAULT_CLUSTER_NAME"
    [image_tag]="$DEFAULT_IMAGE_TAG"
    [namespace]="$DEFAULT_NAMESPACE"
    [show_help]=false
)

while [[ $# -gt 0 ]]; do
    case $1 in
        --cluster-name)
            args[cluster_name]="$2"
            shift 2
            ;;
        --image-tag)
            args[image_tag]="$2"
            shift 2
            ;;
        --namespace)
            args[namespace]="$2"
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
    show_help "$0" "Builds and installs OpenChoreo components in a Kind cluster" ""
    exit 0
fi

cluster_name="${args[cluster_name]}"
image_tag="${args[image_tag]}"
namespace="${args[namespace]}"

# Image names
controller_image="openchoreo-controller:${image_tag}"
api_image="openchoreo-api:${image_tag}"
ui_image="openchoreo-ui:${image_tag}"

# Check additional dependencies for this script
check_additional_dependencies() {
    local missing_deps=()
    
    if ! command_exists go; then
        missing_deps+=("go")
    fi
    
    if ! command_exists yarn; then
        missing_deps+=("yarn")
    fi
    
    if [[ ${#missing_deps[@]} -gt 0 ]]; then
        log_error "Missing additional dependencies: ${missing_deps[*]}"
        echo "Please install the missing tools:"
        for dep in "${missing_deps[@]}"; do
            case $dep in
                go)
                    echo "  - Go: https://go.dev/doc/install"
                    ;;
                yarn)
                    echo "  - Yarn: https://yarnpkg.com/getting-started/install"
                    ;;
            esac
        done
        exit 1
    fi
}

# Check if kind cluster exists
check_cluster() {
    if ! cluster_exists "$cluster_name"; then
        log_error "Kind cluster '${cluster_name}' does not exist"
        echo "Please create it first using: ./kind.sh ${cluster_name}"
        exit 1
    fi
}

# Build and load controller
build_controller() {
    log_info "Building OpenChoreo controller..."
    cd "${PROJECT_ROOT}"
    
    # Build the Go binary
    log_info "Building manager binary..."
    if ! make go.build-multiarch.manager; then
        log_error "Failed to build manager binary"
        exit 1
    fi
    
    # Build the Docker image
    log_info "Building Docker image: ${controller_image}"
    if ! docker build -f "${PROJECT_ROOT}/Dockerfile" -t "${controller_image}" "${PROJECT_ROOT}"; then
        log_error "Failed to build controller Docker image"
        exit 1
    fi
    
    # Load the image into the kind cluster
    log_info "Loading controller image into kind cluster: ${cluster_name}"
    if ! kind load docker-image "${controller_image}" --name "${cluster_name}"; then
        log_error "Failed to load controller image into kind cluster"
        exit 1
    fi
    
    log_success "Successfully built and loaded controller image: ${controller_image}"
}

# Build and load API
build_api() {
    log_info "Building OpenChoreo API..."
    cd "${PROJECT_ROOT}"
    
    # Build the Go binary
    log_info "Building openchoreo-api binary..."
    if ! make go.build-multiarch.openchoreo-api; then
        log_error "Failed to build openchoreo-api binary"
        exit 1
    fi
    
    # Build the Docker image
    log_info "Building Docker image: ${api_image}"
    if ! docker build -f "${PROJECT_ROOT}/cmd/openchoreo-api/Dockerfile" -t "${api_image}" "${PROJECT_ROOT}"; then
        log_error "Failed to build API Docker image"
        exit 1
    fi
    
    # Load the image into the kind cluster
    log_info "Loading API image into kind cluster: ${cluster_name}"
    if ! kind load docker-image "${api_image}" --name "${cluster_name}"; then
        log_error "Failed to load API image into kind cluster"
        exit 1
    fi
    
    log_success "Successfully built and loaded API image: ${api_image}"
}

# Build and load UI
build_ui() {
    log_info "Building OpenChoreo UI..."
    cd "${PROJECT_ROOT}/ui"
    
    # Install dependencies if needed
    if [ ! -d "node_modules" ]; then
        log_info "Installing UI dependencies..."
        if ! yarn install --immutable; then
            log_error "Failed to install UI dependencies"
            exit 1
        fi
    fi
    
    # Build the backend
    log_info "Building backstage..."
    if ! yarn build:all; then
        log_error "Failed to build backstage"
        exit 1
    fi
    
    # Build the Docker image
    log_info "Building Docker image: ${ui_image}"
    if ! docker build -f "${PROJECT_ROOT}/ui/packages/backend/Dockerfile" -t "${ui_image}" "${PROJECT_ROOT}/ui"; then
        log_error "Failed to build UI Docker image"
        exit 1
    fi
    
    # Load the image into the kind cluster
    log_info "Loading UI image into kind cluster: ${cluster_name}"
    if ! kind load docker-image "${ui_image}" --name "${cluster_name}"; then
        log_error "Failed to load UI image into kind cluster"
        exit 1
    fi
    
    log_success "Successfully built and loaded UI image: ${ui_image}"
}

# Install OpenChoreo with Helm
install_openchoreo() {
    log_info "Installing OpenChoreo with Helm into namespace: ${namespace}"
    
    # Create namespace if it doesn't exist
    kubectl create namespace "${namespace}" --dry-run=client -o yaml | kubectl apply -f -
    
    # Install/upgrade OpenChoreo using Helm with values file
    if helm upgrade --install openchoreo "${HELM_DIR}" \
        --namespace "${namespace}" \
        --values "${SCRIPT_DIR}/openchoreo-values.yaml" \
        --set controllerManager.image.repository="${controller_image%:*}" \
        --set controllerManager.image.tag="${controller_image#*:}" \
        --set openchoreoApi.image.repository="${api_image%:*}" \
        --set openchoreoApi.image.tag="${api_image#*:}" \
        --set backstage.image.repository="${ui_image%:*}" \
        --set backstage.image.tag="${ui_image#*:}" \
        --wait \
        --timeout=10m; then
        log_success "OpenChoreo installed successfully!"
    else
        log_error "Failed to install OpenChoreo"
        exit 1
    fi
}

# Verify installation
verify_installation() {
    log_info "Verifying OpenChoreo installation..."
    
    # Check if pods are running
    local pod_status
    pod_status=$(kubectl get pods -n "${namespace}" -l app.kubernetes.io/part-of=openchoreo --no-headers 2>/dev/null | awk '{print $3}' | sort | uniq -c || echo "")
    
    if echo "${pod_status}" | grep -q "Running"; then
        log_success "OpenChoreo pods are running successfully:"
        kubectl get pods -n "${namespace}" -l app.kubernetes.io/part-of=openchoreo
    else
        log_warning "Some pods may not be ready yet. Check with:"
        echo "  kubectl get pods -n ${namespace}"
    fi
    
    # Check CRDs
    local crd_count
    crd_count=$(kubectl get crds -l app.kubernetes.io/part-of=openchoreo --no-headers 2>/dev/null | wc -l || echo "0")
    if [ "${crd_count}" -gt 0 ]; then
        log_success "OpenChoreo CRDs are installed (${crd_count} CRDs found)"
    else
        log_warning "No OpenChoreo CRDs found"
    fi
    
    echo
    log_info "Installation complete! You can now:"
    echo "  - Check pod status: kubectl get pods -n ${namespace}"
    echo "  - Check CRDs: kubectl get crds -l app.kubernetes.io/part-of=openchoreo"
    echo "  - Access OpenChoreo API: kubectl port-forward -n ${namespace} svc/openchoreo-api 8080:8080"
    echo "  - Access Backstage UI: kubectl port-forward -n ${namespace} svc/openchoreo-backstage 7007:7007"
}

# Main execution
main() {
    log_info "Starting OpenChoreo installation for cluster: ${cluster_name}"
    log_info "Using image tag: ${image_tag}"
    log_info "Target namespace: ${namespace}"
    echo
    
    verify_prerequisites
    check_additional_dependencies
    check_cluster
    echo
    
    # Build all components
    build_controller
    echo
    build_api
    echo
    build_ui
    echo
    
    # Install with Helm
    install_openchoreo
    echo
    
    # Verify installation
    verify_installation
}

# Handle script interruption
trap 'log_error "Script interrupted"; exit 1' INT TERM

# Run main function
main "$@"
