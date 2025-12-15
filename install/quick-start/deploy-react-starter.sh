#!/usr/bin/env bash
set -eo pipefail

# Source shared configuration and helpers
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/.helpers.sh"

YAML_FILE="samples/from-image/react-starter-web-app/react-starter.yaml"
NAMESPACE="default"
COMPONENT_NAME="react-starter"
RELEASE_BINDING_NAME="react-starter-development"
MAX_WAIT=300  # Maximum wait time in seconds (5 minutes)
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
            echo "Deploy the React Starter sample web application to OpenChoreo."
            echo ""
            echo "Options:"
            echo "  --clean        Delete the deployed application and exit"
            echo "  --help, -h     Show this help message"
            echo ""
            echo "Examples:"
            echo "  $0             # Deploy the React starter application"
            echo "  $0 --clean     # Delete the deployed application"
            exit 0
            ;;
        *)
            log_error "Unknown option: $1"
            echo "Use --help for usage information"
            exit 1
            ;;
    esac
done

# Clean mode - delete the deployment
if [[ "$CLEAN_MODE" == "true" ]]; then
    log_info "Deleting the React Starter application..."

    if ! kubectl delete -f "$YAML_FILE" 2>&1; then
        log_warning "Some resources may not exist or were already deleted"
    else
        log_success "Application deleted successfully"
    fi

    exit 0
fi

# Check if YAML file exists
if [[ ! -f "$YAML_FILE" ]]; then
    log_error "YAML file not found: $YAML_FILE"
    exit 1
fi

log_info "Sample: $YAML_FILE"

# Apply the YAML file
log_info "Deploying the React Starter web application..."

if ! apply_output=$(kubectl apply -f "$YAML_FILE" 2>&1); then
    log_error "Failed to apply YAML file"
    echo "$apply_output"
    exit 1
fi

# Parse and display the results
echo "$apply_output" | while IFS= read -r line; do
    if [[ "$line" == *"component.openchoreo.dev/${COMPONENT_NAME}"* ]]; then
        if [[ "$line" == *"created"* ]]; then
            log_success "Component '${COMPONENT_NAME}' created"
        elif [[ "$line" == *"configured"* ]] || [[ "$line" == *"unchanged"* ]]; then
            log_info "Component '${COMPONENT_NAME}' already exists"
        fi
    elif [[ "$line" == *"workload.openchoreo.dev/${COMPONENT_NAME}"* ]]; then
        if [[ "$line" == *"created"* ]]; then
            log_success "Workload '${COMPONENT_NAME}' created"
        elif [[ "$line" == *"configured"* ]] || [[ "$line" == *"unchanged"* ]]; then
            log_info "Workload '${COMPONENT_NAME}' already exists"
        fi
    elif [[ "$line" == *"releasebinding.openchoreo.dev/${RELEASE_BINDING_NAME}"* ]]; then
        if [[ "$line" == *"created"* ]]; then
            log_success "ReleaseBinding '${RELEASE_BINDING_NAME}' created"
        elif [[ "$line" == *"configured"* ]] || [[ "$line" == *"unchanged"* ]]; then
            log_info "ReleaseBinding '${RELEASE_BINDING_NAME}' already exists"
        fi
    fi
done

# Wait for ReleaseBinding to be synced
log_info "Waiting for ReleaseBinding to be synced..."
elapsed=0
while true; do
    SYNC_STATUS=$(kubectl get releasebinding "$RELEASE_BINDING_NAME" -n "$NAMESPACE" -o json 2>/dev/null | jq -r '.status.conditions[]? | select(.type=="ReleaseSynced") | .status' || echo "")

    if [[ "$SYNC_STATUS" == "True" ]]; then
        log_success "ReleaseBinding synced with Release"
        break
    fi

    if [[ $elapsed -ge $MAX_WAIT ]]; then
        log_error "Timeout waiting for ReleaseBinding to sync (${MAX_WAIT}s)"
        exit 1
    fi

    sleep $SLEEP_INTERVAL
    elapsed=$((elapsed + SLEEP_INTERVAL))
done

# Wait for Release deployment to be available
log_info "Waiting for Deployment to be available..."
elapsed=0
while true; do
    DEPLOYMENT_AVAILABLE=$(kubectl get release "$RELEASE_BINDING_NAME" -n "$NAMESPACE" -o json 2>/dev/null | jq -r '.status.resources[]? | select(.kind=="Deployment") | .status.conditions[]? | select(.type=="Available" and .reason=="MinimumReplicasAvailable") | .status' || echo "")

    if [[ "$DEPLOYMENT_AVAILABLE" == "True" ]]; then
        log_success "Deployment is available"
        break
    fi

    if [[ $elapsed -ge $MAX_WAIT ]]; then
        log_error "Timeout waiting for Deployment to be available (${MAX_WAIT}s)"
        exit 1
    fi

    sleep $SLEEP_INTERVAL
    elapsed=$((elapsed + SLEEP_INTERVAL))
done

# Wait for HTTPRoute to be ready
log_info "Waiting for HTTPRoute to be ready..."
elapsed=0
while true; do
    ROUTE_ACCEPTED=$(kubectl get release "$RELEASE_BINDING_NAME" -n "$NAMESPACE" -o json 2>/dev/null | jq -r '.status.resources[]? | select(.kind=="HTTPRoute") | .status.parents[]?.conditions[]? | select(.type=="Accepted") | .status' || echo "")

    if [[ "$ROUTE_ACCEPTED" == "True" ]]; then
        log_success "HTTPRoute is ready"
        break
    fi

    if [[ $elapsed -ge $MAX_WAIT ]]; then
        log_error "Timeout waiting for HTTPRoute to be ready (${MAX_WAIT}s)"
        exit 1
    fi

    sleep $SLEEP_INTERVAL
    elapsed=$((elapsed + SLEEP_INTERVAL))
done

# Get the public URL from HTTPRoute
HOSTNAME=$(kubectl get release "$RELEASE_BINDING_NAME" -n "$NAMESPACE" -o json 2>/dev/null | jq -r '.spec.resources[]? | select(.id | startswith("httproute-")) | .object.spec.hostnames[0]' || echo "")

if [[ -z "$HOSTNAME" ]] || [[ "$HOSTNAME" == "null" ]]; then
    log_error "Failed to retrieve hostname from HTTPRoute"
    exit 1
fi

PUBLIC_URL="http://${HOSTNAME}:19080"

echo ""
log_success "React Starter web application is ready!"
echo "🌍 Access the application at: $PUBLIC_URL"
echo "   Open this URL in your browser to see the React starter application."
echo ""
log_info "To clean up and delete this application, run:"
log_info "  ./deploy-react-starter.sh --clean"
