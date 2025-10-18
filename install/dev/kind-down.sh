#!/usr/bin/env bash

# OpenChoreo Kind Cluster Teardown
# Companion script to kind.sh for clean cluster deletion

set -eo pipefail

# Get script directory and source helpers
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/dev-helpers.sh"

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
    show_help "$0" "Deletes a Kind cluster and cleans up resources" ""
    exit 0
fi

cluster_name="${args[cluster_name]}"

# Verify prerequisites
if ! command_exists kind; then
    log_error "Please install kind first:"
    echo "  https://kind.sigs.k8s.io/docs/user/quick-start/#installation"
    exit 1
fi

# Check if cluster exists
if ! cluster_exists "$cluster_name"; then
    log_warning "Kind cluster '${cluster_name}' does not exist"
    exit 0
fi

log_info "Deleting kind cluster '${cluster_name}'..."
if kind delete cluster --name "${cluster_name}"; then
    log_success "Kind cluster '${cluster_name}' has been deleted successfully"
else
    log_error "Failed to delete kind cluster '${cluster_name}'"
    exit 1
fi