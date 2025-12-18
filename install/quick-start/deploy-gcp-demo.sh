#!/usr/bin/env bash
set -eo pipefail

# Source shared configuration and helpers
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/.helpers.sh"

SAMPLE_DIR="samples/gcp-microservices-demo"
NAMESPACE="default"
PROJECT_NAME="gcp-microservice-demo"
DEPLOYMENT_NAME_SUFFIX="-development"
MAX_WAIT=600  # Maximum wait time in seconds (10 minutes for multiple services)
SLEEP_INTERVAL=5
CLEAN_MODE=false

# Parse command line arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        --clean)
            CLEAN_MODE=true
            shift
            ;;
        --help|-h)
            echo "Usage: $0 [OPTIONS]"
            echo ""
            echo "Deploy the GCP Microservices Demo to OpenChoreo."
            echo "This demo includes 11 microservices (frontend, cart, checkout, payment, etc.)"
            echo ""
            echo "Options:"
            echo "  --clean        Delete all deployed services and exit"
            echo "  --help, -h     Show this help message"
            echo ""
            echo "Examples:"
            echo "  $0             # Deploy all GCP microservices"
            echo "  $0 --clean     # Delete all deployed services"
            exit 0
            ;;
        *)
            log_error "Unknown option: $1"
            echo "Use --help for usage information"
            exit 1
            ;;
    esac
done

# Check if sample directory exists
if [[ ! -d "$SAMPLE_DIR" ]]; then
    log_error "Sample directory not found: $SAMPLE_DIR"
    exit 1
fi

# Clean mode - delete all deployments
if [[ "$CLEAN_MODE" == "true" ]]; then
    log_info "Deleting the GCP Microservices Demo..."

    # Delete components first
    if [[ -d "$SAMPLE_DIR/components" ]]; then
        log_info "Deleting components..."
        kubectl delete -f "$SAMPLE_DIR/components/" 2>/dev/null || log_warning "  Components may not exist"
    fi

    # Delete project last
    if [[ -f "$SAMPLE_DIR/gcp-microservice-demo-project.yaml" ]]; then
        log_info "Deleting project..."
        kubectl delete -f "$SAMPLE_DIR/gcp-microservice-demo-project.yaml" 2>/dev/null || log_warning "  Project may not exist"
    fi

    log_success "All resources deleted successfully"
    exit 0
fi

log_info "Sample: $SAMPLE_DIR"
log_info "This will deploy 11 microservices to demonstrate a complex application."

# Apply the project first
PROJECT_FILE="$SAMPLE_DIR/gcp-microservice-demo-project.yaml"
if [[ -f "$PROJECT_FILE" ]]; then
    log_info "Creating project..."
    if kubectl apply -f "$PROJECT_FILE" >/dev/null 2>&1; then
        log_success "Project created/configured"
    else
        log_warning "Project may already exist"
    fi
fi

# Apply all component files
log_info "Deploying microservices..."
component_count=0
for file in "$SAMPLE_DIR"/components/*-component.yaml; do
    if [[ -f "$file" ]]; then
        component_name=$(basename "$file" | sed 's/-component.yaml//')
        log_info "  Deploying $component_name..."

        if kubectl apply -f "$file" >/dev/null 2>&1; then
            log_success "    $component_name deployed"
            component_count=$((component_count + 1))
        else
            log_error "    Failed to deploy $component_name"
        fi
    fi
done

log_success "Deployed $component_count microservices"

# Wait for all ReleaseBindings to be synced
log_info "Waiting for ReleaseBindings to sync..."
elapsed=0
while true; do
    # Count total ReleaseBindings for this project
    total=$(kubectl get releasebinding -n "$NAMESPACE" -o json 2>/dev/null | jq -r "[.items[] | select(.spec.owner.projectName == \"$PROJECT_NAME\")] | length" || echo "0")

    # Count synced ReleaseBindings
    synced=$(kubectl get releasebinding -n "$NAMESPACE" -o json 2>/dev/null | jq -r "[.items[] | select(.spec.owner.projectName == \"$PROJECT_NAME\") | select(.status.conditions[]? | select(.type==\"ReleaseSynced\" and .status==\"True\"))] | length" || echo "0")

    if [[ "$total" -gt 0 ]] && [[ "$synced" -eq "$total" ]]; then
        log_success "All $total ReleaseBindings synced with Releases"
        break
    fi

    if [[ $elapsed -ge $MAX_WAIT ]]; then
        log_error "Timeout waiting for ReleaseBindings to sync (${MAX_WAIT}s)"
        log_info "Synced: $synced / Total: $total"
        exit 1
    fi

    if [[ $elapsed -gt 0 ]] && [[ $((elapsed % 15)) -eq 0 ]]; then
        log_info "  Progress: $synced / $total synced (${elapsed}s elapsed)"
    fi

    sleep $SLEEP_INTERVAL
    elapsed=$((elapsed + SLEEP_INTERVAL))
done

# Wait for all Deployments to be available
log_info "Waiting for all Deployments to be available..."
elapsed=0
while true; do
    # Count total releases for this project
    total_releases=$(kubectl get release -n "$NAMESPACE" -o json 2>/dev/null | jq -r "[.items[] | select(.metadata.labels.\"openchoreo.dev/project\" == \"$PROJECT_NAME\")] | length" || echo "0")

    # Count releases with healthy deployments
    available=$(kubectl get release -n "$NAMESPACE" -o json 2>/dev/null | jq -r "[.items[] | select(.metadata.labels.\"openchoreo.dev/project\" == \"$PROJECT_NAME\") | select(.status.resources[]? | select(.kind==\"Deployment\" and .healthStatus==\"Healthy\"))] | length" || echo "0")

    if [[ "$total_releases" -gt 0 ]] && [[ "$available" -eq "$total_releases" ]]; then
        log_success "All $total_releases Deployments are available"
        break
    fi

    if [[ $elapsed -ge $MAX_WAIT ]]; then
        log_error "Timeout waiting for Deployments to be available (${MAX_WAIT}s)"
        log_info "Available: $available / Total: $total_releases"
        exit 1
    fi

    if [[ $elapsed -gt 0 ]] && [[ $((elapsed % 15)) -eq 0 ]]; then
        log_info "  Progress: $available / $total_releases available (${elapsed}s elapsed)"
    fi

    sleep $SLEEP_INTERVAL
    elapsed=$((elapsed + SLEEP_INTERVAL))
done

# Get the frontend URL
FRONTEND_DEPLOYMENT="frontend${DEPLOYMENT_NAME_SUFFIX}"
HOSTNAME=$(kubectl get release "$FRONTEND_DEPLOYMENT" -n "$NAMESPACE" -o json 2>/dev/null | jq -r '.spec.resources[]? | select(.id | startswith("httproute-")) | .object.spec.hostnames[0]' || echo "")

echo ""
log_success "GCP Microservices Demo is ready!"
echo ""
log_info "Deployed services:"
log_info "  • Frontend (Web UI)"
log_info "  • Cart Service"
log_info "  • Checkout Service"
log_info "  • Payment Service"
log_info "  • Email Service"
log_info "  • Shipping Service"
log_info "  • Product Catalog Service"
log_info "  • Currency Service"
log_info "  • Recommendation Service"
log_info "  • Ad Service"
log_info "  • Redis (Cache)"
echo ""

if [[ -n "$HOSTNAME" ]] && [[ "$HOSTNAME" != "null" ]]; then
    FRONTEND_URL="http://${HOSTNAME}:19080"
    echo "🌍 Access the frontend application at: $FRONTEND_URL"
    echo "   Open this URL in your browser to explore the microservices demo."
else
    log_warning "Could not retrieve frontend URL"
    log_info "You can find the URL with: kubectl get release $FRONTEND_DEPLOYMENT -n $NAMESPACE -o yaml"
fi

echo ""
log_info "To clean up and delete all services, run:"
log_info "  ./deploy-gcp-demo.sh --clean"
