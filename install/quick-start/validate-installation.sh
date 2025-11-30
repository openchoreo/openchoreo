#!/usr/bin/env bash
set -eo pipefail

# Get the absolute path of the script directory
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Source helper functions
source "${SCRIPT_DIR}/.helpers.sh"

# Validation functions
validate_cluster() {
    log_info "Validating k3d cluster..."
    
    if ! cluster_exists; then
        log_error "k3d cluster '$CLUSTER_NAME' does not exist"
        return 1
    fi
    
    # Check cluster is accessible
    if ! kubectl cluster-info >/dev/null 2>&1; then
        log_error "k3d cluster '$CLUSTER_NAME' is not accessible"
        return 1
    fi
    
    log_success "k3d cluster validation passed"
}

validate_helm_releases() {
    log_info "Validating Helm releases..."
    
    local expected_releases=(
        "openchoreo-data-plane:$DATA_PLANE_NS"
        "openchoreo-control-plane:$CONTROL_PLANE_NS"

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
        "$CONTROL_PLANE_NS"
        "$DATA_PLANE_NS"
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
        "$CONTROL_PLANE_NS"
        "$DATA_PLANE_NS"
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
    if ! kubectl get svc -n "$DATA_PLANE_NS" -l gateway.networking.k8s.io/gateway-name=gateway-default >/dev/null 2>&1; then
        log_error "External gateway service not found"
        return 1
    fi
    
    
    log_success "Key services validation passed"
}

validate_ingress() {
    log_info "Validating Traefik ingress..."
    
    # Check if Traefik deployment exists
    if ! kubectl get deployment -n kube-system traefik >/dev/null 2>&1; then
        log_warning "Traefik deployment not found - ingress may not be active"
        return 0
    fi
    
    # Check if port 8080 is accessible on the loadbalancer
    local port=8080
    local netstat_output
    netstat_output=$(netstat -ln 2>/dev/null | grep "0.0.0.0:${port}" || true)

    if [[ -n "$netstat_output" ]]; then
        log_success "Traefik ingress validation passed"
    else
        log_warning "Port $port not listening - Traefik ingress may not be exposed"
    fi

    return 0
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
        "validate_ingress"
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
