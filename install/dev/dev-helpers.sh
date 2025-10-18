#!/usr/bin/env bash

# Helper functions for OpenChoreo development scripts
# These functions provide consistent patterns and utilities

set -eo pipefail

# Color codes for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
PURPLE='\033[0;35m'
BOLD='\033[1m'
RESET='\033[0m'

# Status icons
ICON_READY="✅"
ICON_PENDING="⏳"
ICON_NOT_INSTALLED="⚠️ "
ICON_ERROR="❌"
ICON_UNKNOWN="❓"

# Configuration variables
DEFAULT_CLUSTER_NAME="openchoreo"
DEFAULT_IMAGE_TAG="dev"
DEFAULT_NAMESPACE="openchoreo"
DEFAULT_NETWORK="openchoreo"

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

log_debug() {
    if [[ "${DEBUG:-false}" == "true" ]]; then
        echo -e "${PURPLE}[DEBUG]${RESET} $1"
    fi
}

# Check if a command exists
command_exists() {
    command -v "$1" >/dev/null 2>&1
}

# Check if kind cluster exists
cluster_exists() {
    local cluster_name="$1"
    kind get clusters 2>/dev/null | grep -q "^${cluster_name}$"
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

# Wait for pods to be ready in a namespace
wait_for_pods() {
    local namespace="$1"
    local timeout="${2:-300}" # 5 minutes default
    local label_selector="${3:-}"
    
    log_info "Waiting for pods in namespace '$namespace' to be ready..."
    
    local selector_flag=""
    if [[ -n "$label_selector" ]]; then
        selector_flag="-l $label_selector"
    fi
    
    local elapsed=0
    local interval=5
    
    while [ $elapsed -lt $timeout ]; do
        if kubectl get pods -n "$namespace" $selector_flag --no-headers 2>/dev/null | grep -v 'Running\|Completed' | grep -q .; then
            echo 'Waiting for pods to be ready...'
            sleep $interval
            elapsed=$((elapsed + interval))
        else
            echo 'All pods are ready!'
            break
        fi
    done
    
    if [ $elapsed -ge $timeout ]; then
        log_error "Timeout waiting for pods in namespace '$namespace'"
        return 1
    fi
    
    log_success "All pods in namespace '$namespace' are ready"
}

# Verify prerequisites with helpful messages
verify_prerequisites() {
    local missing_tools=()
    
    if ! command_exists kind; then
        missing_tools+=("kind")
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
        echo "Please install the missing tools:"
        for tool in "${missing_tools[@]}"; do
            case $tool in
                kind)
                    echo "  - Kind: https://kind.sigs.k8s.io/docs/user/quick-start/#installation"
                    ;;
                kubectl)
                    echo "  - kubectl: https://kubernetes.io/docs/tasks/tools/#kubectl"
                    ;;
                helm)
                    echo "  - Helm: https://helm.sh/docs/intro/install/"
                    ;;
                docker)
                    echo "  - Docker: https://docs.docker.com/get-docker/"
                    ;;
            esac
        done
        return 1
    fi
    
    log_success "All prerequisites verified"
}

# Parse command line arguments with standard pattern
parse_arguments() {
    local -n args_ref=$1
    shift
    
    while [[ $# -gt 0 ]]; do
        case $1 in
            --cluster-name)
                args_ref[cluster_name]="$2"
                shift 2
                ;;
            --image-tag)
                args_ref[image_tag]="$2"
                shift 2
                ;;
            --namespace)
                args_ref[namespace]="$2"
                shift 2
                ;;
            --debug)
                export DEBUG=true
                shift
                ;;
            --help|-h)
                args_ref[show_help]=true
                shift
                ;;
            *)
                log_error "Unknown option: $1"
                return 1
                ;;
        esac
    done
}

# Show standard help message
show_help() {
    local script_name="$1"
    local description="$2"
    local usage_args="$3"
    
    echo "Usage: $script_name [OPTIONS] $usage_args"
    echo ""
    echo "Description:"
    echo "  $description"
    echo ""
    echo "Options:"
    echo "  --cluster-name NAME    Cluster name (default: $DEFAULT_CLUSTER_NAME)"
    echo "  --image-tag TAG        Image tag (default: $DEFAULT_IMAGE_TAG)"
    echo "  --namespace NAMESPACE  Target namespace (default: $DEFAULT_NAMESPACE)"
    echo "  --debug                Enable debug logging"
    echo "  --help, -h             Show this help message"
    echo ""
    echo "Examples:"
    echo "  $script_name                           # Use defaults"
    echo "  $script_name --cluster-name my-cluster # Custom cluster name"
    echo "  $script_NAME --image-tag v1.0.0        # Custom image tag"
}

# Clean up function
cleanup() {
    log_info "Cleaning up temporary files..."
    # Add any cleanup logic here
}

# Register cleanup function
trap cleanup EXIT

# Get script directory
get_script_dir() {
    echo "$(cd "$(dirname "${BASH_SOURCE[1]}")" && pwd)"
}

# Get project root directory
get_project_root() {
    local script_dir
    script_dir=$(get_script_dir)
    echo "$(cd "${script_dir}/../.." && pwd)"
}