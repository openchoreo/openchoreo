#!/usr/bin/env bash
set -eo pipefail

# Get the absolute path of the script directory
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Source helper functions
source "${SCRIPT_DIR}/install-helpers.sh"

# Parse command line arguments
FORCE_UNINSTALL=false

while [[ $# -gt 0 ]]; do
    case $1 in
        --force)
            FORCE_UNINSTALL=true
            shift
            ;;
        --help|-h)
            echo "Usage: $0 [OPTIONS]"
            echo ""
            echo "Options:"
            echo "  --force        Force uninstall without confirmation"
            echo "  --help, -h     Show this help message"
            echo ""
            echo "Examples:"
            echo "  $0             # Uninstall with confirmation"
            echo "  $0 --force     # Force uninstall without confirmation"
            exit 0
            ;;
        *)
            log_error "Unknown option: $1"
            echo "Use --help for usage information"
            exit 1
            ;;
    esac
done

log_info "OpenChoreo Uninstall Process"

if [[ "$FORCE_UNINSTALL" != "true" ]]; then
    echo ""
    echo "This will completely remove:"
    echo "  - Kind cluster '$CLUSTER_NAME'"
    echo "  - All OpenChoreo components"
    echo "  - Kubeconfig at $KUBECONFIG_PATH"
    echo "  - Port forwarding processes"
    echo ""
    read -p "Are you sure you want to continue? (y/N): " -r
    if [[ ! $REPLY =~ ^[Yy]$ ]]; then
        log_info "Uninstall cancelled"
        exit 0
    fi
fi

log_info "Starting uninstall process..."

# Stop port forwarding processes
log_info "Stopping port forwarding processes..."
pkill socat 2>/dev/null || log_warning "No socat processes found"

# Delete Kind cluster if it exists
if cluster_exists; then
    log_info "Deleting Kind cluster '$CLUSTER_NAME'..."
    if kind delete cluster --name "$CLUSTER_NAME"; then
        log_success "Kind cluster '$CLUSTER_NAME' deleted successfully"
    else
        log_error "Failed to delete Kind cluster '$CLUSTER_NAME'"
    fi
else
    log_warning "Kind cluster '$CLUSTER_NAME' does not exist"
fi

# Clean up kubeconfig
if [[ -f "$KUBECONFIG_PATH" ]]; then
    log_info "Removing kubeconfig at $KUBECONFIG_PATH..."
    rm -f "$KUBECONFIG_PATH"
    log_success "Kubeconfig removed"
else
    log_warning "Kubeconfig not found at $KUBECONFIG_PATH"
fi

# Clean up kubeconfig directory if empty
if [[ -d "$(dirname "$KUBECONFIG_PATH")" ]]; then
    if [[ -z "$(ls -A "$(dirname "$KUBECONFIG_PATH")")" ]]; then
        log_info "Removing empty kubeconfig directory..."
        rmdir "$(dirname "$KUBECONFIG_PATH")" || log_warning "Failed to remove kubeconfig directory"
    fi
fi

# Clean up choreoctl completion
if [[ -f "/usr/local/bin/choreoctl-completion" ]]; then
    log_info "Removing choreoctl completion..."
    rm -f "/usr/local/bin/choreoctl-completion"
    # Remove from profile (if exists)
    if [[ -f "/etc/profile" ]]; then
        sed -i '/source \/usr\/local\/bin\/choreoctl-completion/d' /etc/profile 2>/dev/null || true
    fi
    log_success "choreoctl completion removed"
fi

# Clean up temporary files
cleanup

log_success "OpenChoreo uninstall completed successfully!"
log_info "All resources have been removed."
