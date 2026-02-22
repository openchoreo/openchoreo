#!/usr/bin/env bash
set -eo pipefail

# Source shared configuration and helpers
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/.helpers.sh"

YAML_FILE="samples/from-source/services/go-docker-greeter/greeting-service.yaml"
NAMESPACE="default"
COMPONENT_NAME="greeting-service"
WORKFLOWRUN_NAME="greeting-service-build-01"
RELEASE_BINDING_NAME="greeting-service-development"
MAX_WAIT=600  # Maximum wait time in seconds (10 minutes for build + deploy)
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
            echo "Build and deploy the Go Greeter sample service to OpenChoreo."
            echo "This demonstrates building a container image from source code using"
            echo "the Build Plane (Argo Workflows + Container Registry)."
            echo ""
            echo "Prerequisites:"
            echo "  - Build Plane must be installed (./install.sh --with-build)"
            echo ""
            echo "Options:"
            echo "  --clean        Delete the deployed application and exit"
            echo "  --help, -h     Show this help message"
            echo ""
            echo "Examples:"
            echo "  $0             # Build and deploy the greeter service"
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
    log_info "Deleting the Go Greeter service..."

    if ! kubectl delete -f "$YAML_FILE" 2>&1; then
        log_warning "Some resources may not exist or were already deleted"
    else
        log_success "Application deleted successfully"
    fi

    exit 0
fi

# Check if Build Plane is installed
if ! kubectl get namespace openchoreo-build-plane &>/dev/null; then
    log_error "Build Plane is not installed!"
    log_info "The Build Plane is required to build container images from source code."
    log_info "Please reinstall with: ./install.sh --with-build"
    exit 1
fi

# Check if YAML file exists
if [[ ! -f "$YAML_FILE" ]]; then
    log_error "YAML file not found: $YAML_FILE"
    exit 1
fi

log_info "Sample: $YAML_FILE"

# Apply the YAML file
log_info "Deploying the Go Greeter service (build from source)..."

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
    elif [[ "$line" == *"workflowrun.openchoreo.dev/${WORKFLOWRUN_NAME}"* ]]; then
        if [[ "$line" == *"created"* ]]; then
            log_success "WorkflowRun '${WORKFLOWRUN_NAME}' created"
        elif [[ "$line" == *"configured"* ]] || [[ "$line" == *"unchanged"* ]]; then
            log_info "WorkflowRun '${WORKFLOWRUN_NAME}' already exists"
        fi
    elif [[ "$line" == *"releasebinding.openchoreo.dev/${RELEASE_BINDING_NAME}"* ]]; then
        if [[ "$line" == *"created"* ]]; then
            log_success "ReleaseBinding '${RELEASE_BINDING_NAME}' created"
        elif [[ "$line" == *"configured"* ]] || [[ "$line" == *"unchanged"* ]]; then
            log_info "ReleaseBinding '${RELEASE_BINDING_NAME}' already exists"
        fi
    fi
done

# Wait for WorkflowRun to complete (image build)
log_info "Building container image from source (this may take few minutes)..."

# Track which build phases we've seen
declare -A build_phases_seen

elapsed=0
while true; do
    # Get WorkflowRun namespace (where the Argo Workflow runs)
    WORKFLOW_NS=$(kubectl get workflowrun "$WORKFLOWRUN_NAME" -n "$NAMESPACE" -o json 2>/dev/null | jq -r '.status.runReference.namespace' || echo "")

    # Check build phase by looking at pods in the workflow namespace
    if [[ -n "$WORKFLOW_NS" ]] && [[ "$WORKFLOW_NS" != "null" ]]; then
        # Check for clone phase
        if [[ ! -v build_phases_seen[clone] ]]; then
            if kubectl get pods -n "$WORKFLOW_NS" -l "workflows.argoproj.io/workflow=${WORKFLOWRUN_NAME}" --field-selector="status.phase=Running" 2>/dev/null | grep -q "clone-step"; then
                log_info "  ↳ Cloning source repository..."
                build_phases_seen[clone]=1
            elif kubectl get pods -n "$WORKFLOW_NS" -l "workflows.argoproj.io/workflow=${WORKFLOWRUN_NAME}" --field-selector="status.phase=Succeeded" 2>/dev/null | grep -q "clone-step"; then
                log_success "  ✓ Source repository cloned"
                build_phases_seen[clone]=1
            fi
        fi

        # Check for build phase
        if [[ ! -v build_phases_seen[build] ]]; then
            if kubectl get pods -n "$WORKFLOW_NS" -l "workflows.argoproj.io/workflow=${WORKFLOWRUN_NAME}" --field-selector="status.phase=Running" 2>/dev/null | grep -q "build-step"; then
                log_info "  ↳ Building Docker image..."
                build_phases_seen[build]=1
            elif kubectl get pods -n "$WORKFLOW_NS" -l "workflows.argoproj.io/workflow=${WORKFLOWRUN_NAME}" --field-selector="status.phase=Succeeded" 2>/dev/null | grep -q "build-step"; then
                log_success "  ✓ Docker image built"
                build_phases_seen[build]=1
            fi
        fi

        # Check for push phase
        if [[ ! -v build_phases_seen[push] ]]; then
            if kubectl get pods -n "$WORKFLOW_NS" -l "workflows.argoproj.io/workflow=${WORKFLOWRUN_NAME}" --field-selector="status.phase=Running" 2>/dev/null | grep -q "publish-image"; then
                log_info "  ↳ Pushing image to registry..."
                build_phases_seen[push]=1
            elif kubectl get pods -n "$WORKFLOW_NS" -l "workflows.argoproj.io/workflow=${WORKFLOWRUN_NAME}" --field-selector="status.phase=Succeeded" 2>/dev/null | grep -q "publish-image"; then
                log_success "  ✓ Image pushed to registry"
                build_phases_seen[push]=1
            fi
        fi
    fi

    WORKFLOW_COMPLETED=$(kubectl get workflowrun "$WORKFLOWRUN_NAME" -n "$NAMESPACE" -o json 2>/dev/null | jq -r '.status.conditions[]? | select(.type=="WorkflowCompleted") | .status' || echo "")

    if [[ "$WORKFLOW_COMPLETED" == "True" ]]; then
        log_success "Container image build completed successfully"

        # Get the built image reference
        IMAGE_REF=$(kubectl get workflowrun "$WORKFLOWRUN_NAME" -n "$NAMESPACE" -o json 2>/dev/null | jq -r '.status.imageStatus.imageRef' || echo "")
        if [[ -n "$IMAGE_REF" ]] && [[ "$IMAGE_REF" != "null" ]]; then
            log_info "Built image: $IMAGE_REF"
        fi
        break
    elif [[ "$WORKFLOW_COMPLETED" == "False" ]]; then
        # Check if there's an error
        WORKFLOW_ERROR=$(kubectl get workflowrun "$WORKFLOWRUN_NAME" -n "$NAMESPACE" -o json 2>/dev/null | jq -r '.status.conditions[]? | select(.type=="WorkflowCompleted" and .status=="False") | .message' || echo "")
        if [[ -n "$WORKFLOW_ERROR" ]] && [[ "$WORKFLOW_ERROR" != "Workflow has not completed yet" ]]; then
            log_error "Build failed: $WORKFLOW_ERROR"
            exit 1
        fi
    fi

    if [[ $elapsed -ge $MAX_WAIT ]]; then
        log_error "Timeout waiting for build to complete (${MAX_WAIT}s)"
        log_info "Check build logs with: kubectl logs -n openchoreo-ci-default -l workflows.argoproj.io/workflow=${WORKFLOWRUN_NAME}"
        exit 1
    fi

    sleep $SLEEP_INTERVAL
    elapsed=$((elapsed + SLEEP_INTERVAL))
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
PATH_PREFIX=$(kubectl get release "$RELEASE_BINDING_NAME" -n "$NAMESPACE" -o json 2>/dev/null | jq -r '.spec.resources[]? | select(.id | startswith("httproute-")) | .object.spec.rules[0].matches[0].path.value' || echo "")

if [[ -z "$HOSTNAME" ]] || [[ "$HOSTNAME" == "null" ]]; then
    log_error "Failed to retrieve hostname from HTTPRoute"
    exit 1
fi

# Construct the full service URL
BASE_URL="http://${HOSTNAME}:19080"
SERVICE_URL="${BASE_URL}${PATH_PREFIX}/greeter/greet"

echo ""
log_success "Go Greeter service is ready!"
echo "🚀 Service built from source and deployed successfully!"
echo ""
echo "🌍 Test the service with:"
echo "   curl ${SERVICE_URL}"
echo ""
echo "   Example response: Hello, Stranger!"
echo ""
log_info "To clean up and delete this application, run:"
log_info "  ./build-deploy-greeter.sh --clean"
