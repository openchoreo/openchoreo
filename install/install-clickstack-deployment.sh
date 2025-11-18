#!/bin/bash
# ClickStack Deployment Test Script
# This script helps diagnose and test ClickStack observability plane deployment

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Configuration
NAMESPACE="${NAMESPACE:-openchoreo-observability-clickstack}"
HELM_CHART_PATH="$(dirname "$0")/helm/openchoreo-observability-clickstack"
RELEASE_NAME="${RELEASE_NAME:-openchoreo-observability-clickstack}"

echo "=================================================="
echo "ClickStack Deployment Test Script"
echo "=================================================="
echo ""

# Function to print colored messages
print_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

print_warning() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

print_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Function to check prerequisites
check_prerequisites() {
    print_info "Checking prerequisites..."

    # Check kubectl
    if ! command -v kubectl &> /dev/null; then
        print_error "kubectl not found. Please install kubectl."
        exit 1
    fi
    print_info "✓ kubectl found: $(kubectl version --client --short 2>/dev/null || kubectl version --client)"

    # Check helm
    if ! command -v helm &> /dev/null; then
        print_error "helm not found. Please install Helm 3.x."
        exit 1
    fi
    print_info "✓ helm found: $(helm version --short)"

    # Check cluster connectivity
    if ! kubectl cluster-info &> /dev/null; then
        print_error "Cannot connect to Kubernetes cluster. Please check your kubeconfig."
        exit 1
    fi
    print_info "✓ Kubernetes cluster is accessible"

    echo ""
}

# Function to add Helm repositories
add_helm_repos() {
    print_info "Adding Helm repositories..."

    # Add HyperDX repo
    if helm repo list | grep -q "hyperdxio"; then
        print_info "✓ HyperDX repository already added"
    else
        print_info "Adding HyperDX repository..."
        helm repo add hyperdxio https://hyperdxio.github.io/helm-charts/ || {
            print_error "Failed to add HyperDX repository"
            exit 1
        }
        print_info "✓ HyperDX repository added"
    fi

    # Update repos
    print_info "Updating Helm repositories..."
    helm repo update || {
        print_error "Failed to update Helm repositories"
        exit 1
    }
    print_info "✓ Helm repositories updated"

    echo ""
}

# Function to verify HyperDX chart availability
verify_hyperdx_chart() {
    print_info "Verifying HyperDX chart availability..."

    if helm search repo hyperdxio/hdx-oss-v2 --version 0.8.4 | grep -q "hdx-oss-v2"; then
        print_info "✓ HyperDX chart 0.8.4 is available"
    else
        print_error "HyperDX chart 0.8.4 not found in repository"
        print_info "Available versions:"
        helm search repo hyperdxio/hdx-oss-v2 --versions
        exit 1
    fi

    echo ""
}

# Function to update chart dependencies
update_dependencies() {
    print_info "Updating chart dependencies..."

    if [ ! -d "$HELM_CHART_PATH" ]; then
        print_error "Chart directory not found: $HELM_CHART_PATH"
        exit 1
    fi

    cd "$HELM_CHART_PATH"

    print_info "Running 'helm dependency update'..."
    helm dependency update || {
        print_error "Failed to update dependencies"
        exit 1
    }

    print_info "✓ Dependencies updated successfully"

    # List dependencies
    if [ -f "Chart.lock" ]; then
        print_info "Locked dependencies:"
        cat Chart.lock | grep -E "name:|version:" | sed 's/^/  /'
    fi

    cd - > /dev/null
    echo ""
}

# Function to create namespace
create_namespace() {
    print_info "Checking namespace: $NAMESPACE"

    if kubectl get namespace "$NAMESPACE" &> /dev/null; then
        print_info "✓ Namespace $NAMESPACE already exists"
    else
        print_info "Creating namespace: $NAMESPACE"
        kubectl create namespace "$NAMESPACE" || {
            print_error "Failed to create namespace"
            exit 1
        }
        print_info "✓ Namespace created"
    fi

    echo ""
}

# Function to create values file for testing
create_test_values() {
    local values_file="/tmp/clickstack-test-values.yaml"

    # Output to stderr to avoid interfering with function return value
    print_info "Creating test values file..." >&2

    cat > "$values_file" <<'EOF'
# Test values for ClickStack deployment
global:
  storageClassName: "standard"  # Use cluster's default StorageClass

# ClickStack configuration (using hdx-oss-v2 chart structure)
hyperdx:
  enabled: true

  hyperdx:
    image:
      repository: docker.hyperdx.io/hyperdx/hyperdx
      tag: "2.7.1"
      pullPolicy: IfNotPresent

    apiKey: "test-api-key-12345"
    apiPort: 8000
    appPort: 3000
    opampPort: 4320
    frontendUrl: "http://localhost:3000"
    logLevel: "info"
    usageStatsEnabled: false

    resources:
      limits:
        cpu: 500m
        memory: 512Mi
      requests:
        cpu: 250m
        memory: 256Mi

    ingress:
      enabled: false
      tls:
        enabled: false
        secretName: "hyperdx-tls"

  clickhouse:
    enabled: true

    # Image (string format as expected by hdx-oss-v2 chart)
    image: "clickhouse/clickhouse-server:25.7-alpine"

    httpPort: 8123
    nativePort: 9000

    persistence:
      enabled: true
      data:
        storageClassName: ""
        size: 5Gi
      logs:
        storageClassName: ""
        size: 2Gi

    config:
      users:
        appUserPassword: ""
      clusterCidrs:
        - "10.0.0.0/8"
        - "172.16.0.0/12"
        - "192.168.0.0/16"
      maxMemoryUsage: "2000000000"  # 2GB for testing
      maxMemoryUsageForAllQueries: "4000000000"  # 4GB for testing

    resources:
      limits:
        cpu: 1000m
        memory: 2Gi
      requests:
        cpu: 500m
        memory: 1Gi

  otel:
    enabled: true

    image:
      repository: docker.hyperdx.io/hyperdx/hyperdx-otel-collector
      tag: "2.7.1"
      pullPolicy: IfNotPresent

    grpcPort: 4317
    httpPort: 4318
    fluentdPort: 24225
    prometheusPort: 8888

    # customConfig is optional, omit if not needed

    resources:
      limits:
        cpu: 500m
        memory: 512Mi
      requests:
        cpu: 250m
        memory: 256Mi

  mongodb:
    enabled: true

    # Image (string format as expected by hdx-oss-v2 chart)
    image: "mongo:5.0.14-focal"

    port: 27017

    persistence:
      enabled: true
      storageClassName: ""
      size: 2Gi

    resources:
      limits:
        cpu: 500m
        memory: 512Mi
      requests:
        cpu: 250m
        memory: 256Mi

  tasks:
    enabled: false

  additionalIngresses: []

clickstackObserver:
  enabled: false  # Not implemented yet
EOF

    # Output to stderr to avoid interfering with function return value
    print_info "✓ Test values file created: $values_file" >&2
    echo "" >&2
    # Only output the file path to stdout for function return
    echo "$values_file"
}

# Function to dry-run deployment
dry_run_deployment() {
    print_info "Running dry-run deployment..."

    local values_file=$(create_test_values)

    cd "$HELM_CHART_PATH"

    helm upgrade --install "$RELEASE_NAME" . \
        --namespace "$NAMESPACE" \
        --values "$values_file" \
        --dry-run --debug 2>&1 | tee /tmp/helm-dry-run.log

    if [ ${PIPESTATUS[0]} -eq 0 ]; then
        print_info "✓ Dry-run successful"
    else
        print_error "Dry-run failed. Check /tmp/helm-dry-run.log for details"
        exit 1
    fi

    cd - > /dev/null
    echo ""
}

# Function to deploy ClickStack
deploy_clickstack() {
    print_info "Deploying ClickStack..."

    local values_file=$(create_test_values)

    cd "$HELM_CHART_PATH"

    helm upgrade --install "$RELEASE_NAME" . \
        --namespace "$NAMESPACE" \
        --create-namespace \
        --values "$values_file" \
        --timeout 10m \
        --wait

    if [ $? -eq 0 ]; then
        print_info "✓ Deployment successful"
    else
        print_error "Deployment failed"
        exit 1
    fi

    cd - > /dev/null
    echo ""
}

# Function to check deployment status
check_deployment_status() {
    print_info "Checking deployment status..."

    print_info "Pods in namespace $NAMESPACE:"
    kubectl get pods -n "$NAMESPACE" -o wide

    echo ""
    print_info "Services in namespace $NAMESPACE:"
    kubectl get svc -n "$NAMESPACE"

    echo ""
    print_info "PVCs in namespace $NAMESPACE:"
    kubectl get pvc -n "$NAMESPACE"

    echo ""

    # Check for non-running pods
    local failing_pods=$(kubectl get pods -n "$NAMESPACE" --field-selector=status.phase!=Running -o name 2>/dev/null)

    if [ -n "$failing_pods" ]; then
        print_warning "Found non-running pods:"
        echo "$failing_pods"
        echo ""

        for pod in $failing_pods; do
            print_warning "Logs for $pod:"
            kubectl logs -n "$NAMESPACE" "$pod" --tail=50 2>&1 || true
            echo ""
            print_warning "Events for $pod:"
            kubectl describe -n "$NAMESPACE" "$pod" | tail -20
            echo ""
        done
    else
        print_info "✓ All pods are running"
    fi
}

# Function to verify ClickHouse
verify_clickhouse() {
    print_info "Verifying ClickHouse..."

    # Wait for ClickHouse to be ready
    print_info "Waiting for ClickHouse pod to be ready..."
    kubectl wait --for=condition=ready pod -l app=clickhouse -n "$NAMESPACE" --timeout=300s || {
        print_warning "ClickHouse pod not ready yet"
        return 1
    }

    print_info "✓ ClickHouse pod is ready"

    # Check tables
    print_info "Checking ClickHouse tables..."
    kubectl exec -n "$NAMESPACE" -it $(kubectl get pod -n "$NAMESPACE" -l app=clickhouse -o name | head -1) -- \
        clickhouse-client --query "SHOW TABLES" || {
        print_warning "Could not check ClickHouse tables"
        return 1
    }

    echo ""
}

# Function to show access instructions
show_access_instructions() {
    echo ""
    echo "=================================================="
    echo "ClickStack Deployment Successful!"
    echo "=================================================="
    echo ""
    echo "Access Instructions:"
    echo ""
    echo "1. HyperDX UI:"
    echo "   kubectl port-forward -n $NAMESPACE svc/hyperdx-app 3000:3000"
    echo "   Then open: http://localhost:3000"
    echo ""
    echo "2. ClickStack Observer API:"
    echo "   kubectl port-forward -n $NAMESPACE svc/clickstack-observer 8080:8080"
    echo "   Then use: http://localhost:8080/api/logs/..."
    echo ""
    echo "3. Check logs:"
    echo "   kubectl logs -n $NAMESPACE -l app=hyperdx-app"
    echo "   kubectl logs -n $NAMESPACE -l app=clickhouse"
    echo "   kubectl logs -n $NAMESPACE -l app=otel-collector"
    echo ""
    echo "4. Verify data flow:"
    echo "   See: docs/clickstack-e2e-verification.md"
    echo ""
    echo "=================================================="
}

# Main execution
main() {
    local action="${1:-deploy}"

    case "$action" in
        check)
            check_prerequisites
            verify_hyperdx_chart
            ;;
        deps)
            check_prerequisites
            add_helm_repos
            verify_hyperdx_chart
            update_dependencies
            ;;
        dry-run)
            check_prerequisites
            add_helm_repos
            verify_hyperdx_chart
            update_dependencies
            create_namespace
            dry_run_deployment
            ;;
        deploy)
            check_prerequisites
            add_helm_repos
            verify_hyperdx_chart
            update_dependencies
            create_namespace
            deploy_clickstack
            check_deployment_status
            verify_clickhouse
            show_access_instructions
            ;;
        status)
            check_deployment_status
            ;;
        verify)
            verify_clickhouse
            ;;
        clean)
            print_warning "Cleaning up ClickStack deployment..."
            helm uninstall "$RELEASE_NAME" -n "$NAMESPACE" || true
            kubectl delete namespace "$NAMESPACE" || true
            print_info "✓ Cleanup complete"
            ;;
        *)
            echo "Usage: $0 {check|deps|dry-run|deploy|status|verify|clean}"
            echo ""
            echo "Commands:"
            echo "  check    - Check prerequisites and chart availability"
            echo "  deps     - Update Helm dependencies"
            echo "  dry-run  - Run deployment dry-run"
            echo "  deploy   - Deploy ClickStack (default)"
            echo "  status   - Check deployment status"
            echo "  verify   - Verify ClickHouse"
            echo "  clean    - Remove ClickStack deployment"
            exit 1
            ;;
    esac
}

# Run main function
main "$@"
