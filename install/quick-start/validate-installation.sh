#!/usr/bin/env bash
set -eo pipefail

# Get the absolute path of the script directory
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Source helper functions
source "${SCRIPT_DIR}/install-helpers.sh"

# Validation functions
validate_cluster() {
    log_info "Validating Kind cluster..."
    
    if ! cluster_exists; then
        log_error "Kind cluster '$CLUSTER_NAME' does not exist"
        return 1
    fi
    
    # Check cluster is accessible
    if ! kubectl cluster-info --context "kind-$CLUSTER_NAME" >/dev/null 2>&1; then
        log_error "Kind cluster '$CLUSTER_NAME' is not accessible"
        return 1
    fi
    
    log_success "Kind cluster validation passed"
}

validate_helm_releases() {
    log_info "Validating Helm releases..."
    
    local expected_releases=(
        "cilium:$CILIUM_NS"
        "openchoreo-data-plane:$DATA_PLANE_NS"
        "openchoreo-control-plane:$CONTROL_PLANE_NS"
        "openchoreo-build-plane:$BUILD_PLANE_NS"
        "openchoreo-platform-identity-provider:$CONTROL_PLANE_NS"
        "openchoreo-workload-identity-provider:$IDENTITY_NS"
        "openchoreo-backstage-demo:$CONTROL_PLANE_NS"
    )
    
    local failed_releases=()
    
    for release_info in "${expected_releases[@]}"; do
        local release_name="${release_info%%:*}"
        local namespace="${release_info##*:}"
        
        if ! helm_release_exists "$release_name" "$namespace"; then
            failed_releases+=("$release_name in $namespace")
        fi
    done
    
    if [[ ${#failed_releases[@]} -gt 0 ]]; then
        log_error "Missing Helm releases: ${failed_releases[*]}"
        return 1
    fi
    
    log_success "All expected Helm releases found"
}

validate_namespaces() {
    log_info "Validating namespaces..."
    
    local expected_namespaces=(
        "$CILIUM_NS"
        "$CONTROL_PLANE_NS"
        "$DATA_PLANE_NS"
        "$BUILD_PLANE_NS"
        "$IDENTITY_NS"
    )
    
    local missing_namespaces=()
    
    for ns in "${expected_namespaces[@]}"; do
        if ! namespace_exists "$ns"; then
            missing_namespaces+=("$ns")
        fi
    done
    
    if [[ ${#missing_namespaces[@]} -gt 0 ]]; then
        log_error "Missing namespaces: ${missing_namespaces[*]}"
        return 1
    fi
    
    log_success "All expected namespaces found"
}

validate_pods() {
    log_info "Validating pod readiness..."
    
    local namespaces=(
        "$CILIUM_NS"
        "$CONTROL_PLANE_NS"
        "$DATA_PLANE_NS"
        "$BUILD_PLANE_NS"
        "$IDENTITY_NS"
    )
    
    local failed_namespaces=()
    
    for ns in "${namespaces[@]}"; do
        if ! namespace_exists "$ns"; then
            continue
        fi
        
        local not_ready_pods
        not_ready_pods=$(kubectl get pods -n "$ns" --no-headers 2>/dev/null | grep -v 'Running\|Completed' | wc -l)
        
        if [[ "$not_ready_pods" -gt 0 ]]; then
            failed_namespaces+=("$ns")
        fi
    done
    
    if [[ ${#failed_namespaces[@]} -gt 0 ]]; then
        log_warning "Some pods are not ready in namespaces: ${failed_namespaces[*]}"
        log_info "This might be normal if the installation is still in progress"
        return 0
    fi
    
    log_success "All pods are ready"
}

validate_services() {
    log_info "Validating key services..."
    
    # Check external gateway service
    if ! kubectl get svc -n "$DATA_PLANE_NS" -l gateway.envoyproxy.io/owning-gateway-name=gateway-external >/dev/null 2>&1; then
        log_error "External gateway service not found"
        return 1
    fi
    
    # Check backstage service
    if ! kubectl get svc -n "$CONTROL_PLANE_NS" -l app.kubernetes.io/component=backstage >/dev/null 2>&1; then
        log_error "Backstage service not found"
        return 1
    fi
    
    log_success "Key services validation passed"
}

validate_port_forwarding() {
    log_info "Validating port forwarding..."
    
    # Check if socat processes are running
    if ! pgrep socat >/dev/null 2>&1; then
        log_warning "No socat processes found - port forwarding may not be active"
        return 0
    fi
    
    # Check if expected ports are listening
    local ports=(8443 7007)
    local failed_ports=()
    
    for port in "${ports[@]}"; do
        if ! netstat -ln 2>/dev/null | grep -q ":$port "; then
            failed_ports+=("$port")
        fi
    done
    
    if [[ ${#failed_ports[@]} -gt 0 ]]; then
        log_warning "Ports not listening: ${failed_ports[*]}"
        return 0
    fi
    
    log_success "Port forwarding validation passed"
}

validate_kubeconfig() {
    log_info "Validating kubeconfig..."
    
    if [[ ! -f "$KUBECONFIG_PATH" ]]; then
        log_error "Kubeconfig not found at $KUBECONFIG_PATH"
        return 1
    fi
    
    # Test kubeconfig works
    if ! KUBECONFIG="$KUBECONFIG_PATH" kubectl cluster-info >/dev/null 2>&1; then
        log_error "Kubeconfig at $KUBECONFIG_PATH is not working"
        return 1
    fi
    
    log_success "Kubeconfig validation passed"
}

# Main validation function
run_validation() {
    local validation_functions=(
        "validate_cluster"
        "validate_kubeconfig"
        "validate_namespaces"
        "validate_helm_releases"
        "validate_services"
        "validate_pods"
        "validate_port_forwarding"
    )
    
    local failed_validations=()
    
    log_info "Starting comprehensive validation..."
    echo ""
    
    for func in "${validation_functions[@]}"; do
        if ! $func; then
            failed_validations+=("$func")
        fi
        echo ""
    done
    
    if [[ ${#failed_validations[@]} -gt 0 ]]; then
        log_error "Validation failed for: ${failed_validations[*]}"
        return 1
    fi
    
    log_success "All validations passed!"
}

# Run validation
if ! run_validation; then
    exit 1
fi

log_success "OpenChoreo installation validation completed successfully!"
