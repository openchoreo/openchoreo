#!/usr/bin/env bash
set -eo pipefail

# Get the absolute path of the script directory
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Source helper functions
source "${SCRIPT_DIR}/.helpers.sh"

# Parse command line arguments
FORCE_UNINSTALL=false
DEBUG=false

while [[ $# -gt 0 ]]; do
    case $1 in
        --force)
            FORCE_UNINSTALL=true
            shift
            ;;
        --debug)
            DEBUG=true
            export DEBUG
            shift
            ;;
        --help|-h)
            echo "Usage: $0 [OPTIONS]"
            echo ""
            echo "Options:"
            echo "  --force        Skip confirmation prompt"
            echo "  --debug        Enable debug mode"
            echo "  --help, -h     Show this help message"
            echo ""
            echo "Examples:"
            echo "  $0                # Uninstall with confirmation"
            echo "  $0 --force        # Uninstall without confirmation"
            echo "  $0 --debug        # Uninstall with debug logging"
            echo "  $0 --force --debug   # Force uninstall with debug mode"
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
    echo "  - k3d cluster '$CLUSTER_NAME'"
    echo "  - All OpenChoreo components"
    echo ""
    read -p "Are you sure you want to continue? (y/N): " -r
    if [[ ! $REPLY =~ ^[Yy]$ ]]; then
        log_info "Uninstall cancelled"
        exit 0
    fi
fi

# Delete k3d cluster if it exists
if cluster_exists; then
    log_info "Deleting k3d cluster '$CLUSTER_NAME'..."
    if run_command k3d cluster delete "$CLUSTER_NAME"; then
        log_success "k3d cluster '$CLUSTER_NAME' deleted successfully"
    else
        log_error "Failed to delete k3d cluster '$CLUSTER_NAME'"
    fi
else
    log_warning "k3d cluster '$CLUSTER_NAME' does not exist"
fi

log_success "OpenChoreo uninstall completed successfully!"
