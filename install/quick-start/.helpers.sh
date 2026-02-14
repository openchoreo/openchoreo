#!/usr/bin/env bash

# Helper functions for OpenChoreo installation
# These functions provide idempotent operations for setting up OpenChoreo

set -eo pipefail

# Source shared configuration
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/.config.sh"

# Color codes for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
BLUE='\033[0;34m'
RESET='\033[0m'

# Version configuration
# OPENCHOREO_VERSION is used for image tags (default: latest)
# OPENCHOREO_CHART_VERSION is derived from OPENCHOREO_VERSION
OPENCHOREO_VERSION="${OPENCHOREO_VERSION:-latest}"

# Dev mode configuration
DEV_MODE="${DEV_MODE:-false}"
DEV_HELM_CHARTS_DIR="/helm"

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

# Check if a command exists
command_exists() {
    command -v "$1" >/dev/null 2>&1
}

# Execute command with optional debug logging
# When DEBUG=true, logs the command before executing
run_command() {
    if [[ "${DEBUG:-false}" == "true" ]]; then
        log_info "Executing: $*"
    fi
    "$@"
}

# Function to derive chart version from image version
# This must be called AFTER OPENCHOREO_VERSION is set by the caller
# Sets:
#   OPENCHOREO_CHART_VERSION - the helm chart version to install
#   BACKSTAGE_VERSION - the backstage image tag (latest-dev for commit SHAs since backstage is in a separate repo)
derive_chart_version() {
    if [[ "$OPENCHOREO_VERSION" == "latest" ]]; then
        # Production latest: don't specify chart version (helm pulls latest)
        OPENCHOREO_CHART_VERSION=""
        BACKSTAGE_VERSION="$OPENCHOREO_VERSION"
    elif [[ "$OPENCHOREO_VERSION" == "latest-dev" ]]; then
        # Development builds: use special dev chart version
        OPENCHOREO_CHART_VERSION="0.0.0-latest-dev"
        BACKSTAGE_VERSION="$OPENCHOREO_VERSION"
    elif [[ "$OPENCHOREO_VERSION" =~ ^v[0-9]+\.[0-9]+\.[0-9]+ ]]; then
        # Release version with 'v' prefix: strip 'v' for chart version
        OPENCHOREO_CHART_VERSION="${OPENCHOREO_VERSION#v}"
        BACKSTAGE_VERSION="$OPENCHOREO_VERSION"
    elif [[ "$OPENCHOREO_VERSION" =~ ^[0-9a-f]{8}$ ]]; then
        # Commit SHA (8 hex chars): prefix with 0.0.0-
        OPENCHOREO_CHART_VERSION="0.0.0-${OPENCHOREO_VERSION}"
        # Backstage is in a separate repo with different commits, use latest-dev
        BACKSTAGE_VERSION="latest-dev"
    else
        # Assume it's already a valid chart version (e.g., "1.2.3")
        OPENCHOREO_CHART_VERSION="$OPENCHOREO_VERSION"
        BACKSTAGE_VERSION="$OPENCHOREO_VERSION"
    fi
}

# Check if k3d cluster exists
cluster_exists() {
    k3d cluster list 2>/dev/null | grep -q "^${CLUSTER_NAME} "
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

# Create k3d cluster with specific configuration
create_k3d_cluster() {
    if cluster_exists; then
        log_warning "k3d cluster '$CLUSTER_NAME' already exists, skipping creation"
        return 0
    fi

    log_info "Creating k3d cluster '$CLUSTER_NAME'..."

    # Use the k3d config file from user's home directory
    local k3d_config="$HOME/.k3d-config.yaml"

    if [[ ! -f "$k3d_config" ]]; then
        log_error "k3d config file not found at $k3d_config"
        return 1
    fi

    # Detect if running in Colima and disable k3d's DNS fix if needed
    # The DNS fix replaces Docker's embedded DNS (127.0.0.11) with the gateway IP,
    # which causes DNS timeouts in Colima due to firewall/network isolation.
    # k3d v5.9.0+ auto-detects Colima, but we handle it explicitly for older versions.
    # See https://github.com/k3d-io/k3d/issues/1449
    local use_dns_fix=false
    if docker info --format '{{.Name}}' 2>/dev/null | grep -qi "colima"; then
        log_info "Detected Colima runtime - disabling k3d DNS fix for compatibility"
        use_dns_fix=true
    fi

    local create_ok=false
    if [[ "$use_dns_fix" == "true" ]]; then
        K3D_FIX_DNS=0 run_command k3d cluster create "$CLUSTER_NAME" --config "$k3d_config" --wait && create_ok=true
    else
        run_command k3d cluster create "$CLUSTER_NAME" --config "$k3d_config" --wait && create_ok=true
    fi

    if [[ "$create_ok" == "true" ]]; then
        log_success "k3d cluster '$CLUSTER_NAME' created successfully"
    else
        log_error "Failed to create k3d cluster '$CLUSTER_NAME'"
        return 1
    fi
}

# Clear N lines from the terminal (used for updating status display)
_clear_terminal_lines() {
    local count="$1"
    local use_tput="$2"
    if [[ "$use_tput" == "true" && "$count" -gt 0 ]]; then
        local i
        for ((i = 0; i < count; i++)); do
            tput cuu1 2>/dev/null || true
            tput el 2>/dev/null || true
        done
    fi
}

# Monitor pod status in a namespace while helm is running
# Supports interactive mode where users can press 'v' to see detailed pod list
# Shows container readiness and detects blocking resources (LoadBalancers, Jobs, etc.)
monitor_pod_status_with_helm() {
    local namespace="$1"
    local helm_pid="$2"
    local start_time
    start_time=$(date +%s)

    # Check if tput is available for fancy display
    local use_fancy_display="false"
    if command -v tput >/dev/null 2>&1; then
        use_fancy_display="true"
        export TERM="${TERM:-xterm}"
    fi

    # Check if running in interactive terminal (both stdin and stdout must be terminals)
    local is_interactive="false"
    if [[ -t 0 && -t 1 ]]; then
        is_interactive="true"
    fi

    local show_details=0
    local lines_printed=0

    while true; do
        local current_time
        current_time=$(date +%s)
        local elapsed=$((current_time - start_time))

        # Get pod info with READY column (format: NAME READY STATUS RESTARTS AGE)
        local pod_info
        pod_info=$(kubectl get pods -n "$namespace" --no-headers 2>/dev/null) || pod_info=""

        # Clear previous output
        _clear_terminal_lines "$lines_printed" "$use_fancy_display"
        lines_printed=0

        if [[ -z "$pod_info" ]]; then
            # No pods yet
            if [[ "$is_interactive" == "true" ]]; then
                echo "Waiting for pods to be created... (${elapsed}s)"
            else
                echo "Waiting for pods to be created... (${elapsed}s)"
            fi
            lines_printed=1
        else
            local total=0
            local ready=0
            local pending=0

            # Count pods by analyzing READY column (e.g., "1/1", "0/1", "2/3")
            while IFS= read -r line; do
                total=$((total + 1))
                local ready_col status_col
                ready_col=$(echo "$line" | awk '{print $2}')
                status_col=$(echo "$line" | awk '{print $3}')

                # Check if all containers are ready (e.g., "1/1", "2/2")
                local ready_count total_count
                ready_count=$(echo "$ready_col" | cut -d'/' -f1)
                total_count=$(echo "$ready_col" | cut -d'/' -f2)

                if [[ "$ready_count" == "$total_count" && "$status_col" == "Running" ]]; then
                    ready=$((ready + 1))
                else
                    pending=$((pending + 1))
                fi
            done <<< "$pod_info"

            # Check for blocking resources when all pods are ready but helm is still waiting
            local blocking_info=""
            if [[ $pending -eq 0 && $ready -gt 0 ]]; then
                blocking_info=$(get_blocking_resources "$namespace") || blocking_info=""
            fi

            if [[ $show_details -eq 1 ]]; then
                # Detailed view with container readiness
                echo "Pods: $ready/$total Ready (${elapsed}s)"
                echo ""
                printf "  %-45s %-12s %-15s\n" "NAME" "READY" "STATUS"
                printf "  %-45s %-12s %-15s\n" "---------------------------------------------" "------------" "---------------"
                lines_printed=4

                while IFS= read -r line; do
                    local pod_name ready_col status_col
                    pod_name=$(echo "$line" | awk '{print $1}')
                    ready_col=$(echo "$line" | awk '{print $2}')
                    status_col=$(echo "$line" | awk '{print $3}')
                    printf "  %-45s %-12s %-15s\n" "$pod_name" "$ready_col" "$status_col"
                    lines_printed=$((lines_printed + 1))
                done <<< "$pod_info"

                # Show blocking resources if any
                if [[ -n "$blocking_info" ]]; then
                    echo ""
                    echo "Waiting on:"
                    lines_printed=$((lines_printed + 2))
                    while IFS= read -r block_line; do
                        if [[ -n "$block_line" ]]; then
                            echo "  $block_line"
                            lines_printed=$((lines_printed + 1))
                        fi
                    done <<< "$blocking_info"
                fi

                echo ""
                echo "Press 'x' to hide details."
                lines_printed=$((lines_printed + 2))
            else
                # Summary view
                local summary_msg="Pods: $ready/$total Ready (${elapsed}s)"
                local suffix=""

                if [[ -n "$blocking_info" ]]; then
                    suffix=" waiting on other resources..."
                fi

                if [[ "$is_interactive" == "true" ]]; then
                    echo "$summary_msg$suffix [Press 'v' for details]"
                else
                    echo "$summary_msg$suffix"
                fi
                lines_printed=1
            fi
        fi

        # Check if helm is still running - if not, stop monitoring
        if ! kill -0 "$helm_pid" 2>/dev/null; then
            # Helm has finished, clear the output and exit
            _clear_terminal_lines "$lines_printed" "$use_fancy_display"
            return 0
        fi

        # Handle input: interactive mode reads keys, non-interactive just sleeps
        if [[ "$is_interactive" == "true" ]]; then
            local key=""
            # Read single key with 1 second timeout
            if read -rsn1 -t 1 key 2>/dev/null; then
                case "$key" in
                    v|V)
                        show_details=1
                        ;;
                    x|X|q|Q)
                        show_details=0
                        ;;
                esac
            fi
        else
            # Non-interactive: just wait
            sleep 2
        fi
    done
}

# Get resources that might be blocking helm --wait completion
# Called when all pods are ready but helm is still waiting
get_blocking_resources() {
    local namespace="$1"
    local blocking=""

    # Check for pending Jobs
    local jobs
    jobs=$(kubectl get jobs -n "$namespace" --no-headers 2>/dev/null || echo "")
    if [[ -n "$jobs" ]]; then
        while IFS= read -r line; do
            local job_name completions
            job_name=$(echo "$line" | awk '{print $1}')
            completions=$(echo "$line" | awk '{print $2}')
            local complete_count total_count
            complete_count=$(echo "$completions" | cut -d'/' -f1)
            total_count=$(echo "$completions" | cut -d'/' -f2)
            if [[ "$complete_count" != "$total_count" ]]; then
                blocking+="Job: $job_name ($completions)"$'\n'
            fi
        done <<< "$jobs"
    fi

    # Check for LoadBalancer services without external IP
    local services
    services=$(kubectl get svc -n "$namespace" --no-headers 2>/dev/null | grep LoadBalancer || echo "")
    if [[ -n "$services" ]]; then
        while IFS= read -r line; do
            local svc_name external_ip
            svc_name=$(echo "$line" | awk '{print $1}')
            external_ip=$(echo "$line" | awk '{print $4}')
            if [[ "$external_ip" == "<pending>" || "$external_ip" == "<none>" ]]; then
                blocking+="Service (LoadBalancer): $svc_name (pending external IP)"$'\n'
            fi
        done <<< "$services"
    fi

    # Check for PVCs that are not bound
    local pvcs
    pvcs=$(kubectl get pvc -n "$namespace" --no-headers 2>/dev/null || echo "")
    if [[ -n "$pvcs" ]]; then
        while IFS= read -r line; do
            local pvc_name pvc_status
            pvc_name=$(echo "$line" | awk '{print $1}')
            pvc_status=$(echo "$line" | awk '{print $2}')
            if [[ "$pvc_status" != "Bound" ]]; then
                blocking+="PVC: $pvc_name ($pvc_status)"$'\n'
            fi
        done <<< "$pvcs"
    fi

    # Check for Deployments not fully available
    local deployments
    deployments=$(kubectl get deployments -n "$namespace" --no-headers 2>/dev/null || echo "")
    if [[ -n "$deployments" ]]; then
        while IFS= read -r line; do
            local dep_name ready_replicas
            dep_name=$(echo "$line" | awk '{print $1}')
            ready_replicas=$(echo "$line" | awk '{print $2}')
            local ready_count desired_count
            ready_count=$(echo "$ready_replicas" | cut -d'/' -f1)
            desired_count=$(echo "$ready_replicas" | cut -d'/' -f2)
            if [[ "$ready_count" != "$desired_count" ]]; then
                blocking+="Deployment: $dep_name ($ready_replicas ready)"$'\n'
            fi
        done <<< "$deployments"
    fi

    # Check for StatefulSets not fully ready
    local statefulsets
    statefulsets=$(kubectl get statefulsets -n "$namespace" --no-headers 2>/dev/null || echo "")
    if [[ -n "$statefulsets" ]]; then
        while IFS= read -r line; do
            local sts_name ready_replicas
            sts_name=$(echo "$line" | awk '{print $1}')
            ready_replicas=$(echo "$line" | awk '{print $2}')
            local ready_count desired_count
            ready_count=$(echo "$ready_replicas" | cut -d'/' -f1)
            desired_count=$(echo "$ready_replicas" | cut -d'/' -f2)
            if [[ "$ready_count" != "$desired_count" ]]; then
                blocking+="StatefulSet: $sts_name ($ready_replicas ready)"$'\n'
            fi
        done <<< "$statefulsets"
    fi

    # Remove trailing newline
    echo "${blocking%$'\n'}"
}

# Install or upgrade a helm chart with idempotency
# Uses 'helm upgrade --install' which is the standard way to achieve idempotent installs
install_helm_chart() {
    local release_name="$1"
    local chart_name="$2"
    local namespace="$3"
    local create_namespace="${4:-true}"
    local wait_flag="${5:-false}"
    local monitor_flag="${6:-false}"
    local timeout="${7:-1800}"
    shift 7
    local additional_args=("$@")

    # Determine chart reference based on dev mode and chart type
    # Third-party charts (containing "/") are used as-is (e.g., "twuni/docker-registry")
    # OpenChoreo charts are prefixed with HELM_REPO (e.g., "openchoreo-control-plane")
    local chart_ref
    local is_third_party=false
    if [[ "$chart_name" == *"/"* ]]; then
        chart_ref="$chart_name"
        is_third_party=true
    elif [[ "$DEV_MODE" == "true" && -d "$DEV_HELM_CHARTS_DIR/$chart_name" ]]; then
        chart_ref="$DEV_HELM_CHARTS_DIR/$chart_name"
    else
        chart_ref="${HELM_REPO}/${chart_name}"
    fi

    log_info "Installing '$release_name' in namespace '$namespace'..."

    # Build helm upgrade --install command
    local helm_args=(
        "upgrade" "--install" "$release_name" "$chart_ref"
        "--namespace" "$namespace"
        "--timeout" "${timeout}s"
    )

    if [[ "$create_namespace" == "true" ]]; then
        helm_args+=("--create-namespace")
    fi

    # Add --wait flag when wait_flag is true so helm waits for resources
    # This allows us to monitor pod status while helm is waiting
    if [[ "$wait_flag" == "true" ]]; then
        helm_args+=("--wait")
    fi

    # Only add version for OpenChoreo charts, not third-party charts
    if [[ -n "$OPENCHOREO_CHART_VERSION" && "$DEV_MODE" != "true" && "$is_third_party" != "true" ]]; then
        helm_args+=("--version" "$OPENCHOREO_CHART_VERSION")
    fi

    helm_args+=("${additional_args[@]}")

    # If monitor flag is requested, run helm in background and monitor pods in real-time
    if [[ "$monitor_flag" == "true" ]]; then
        # Create a temp file for helm output
        local temp_dir
        temp_dir=$(mktemp -d)
        local helm_output_file="$temp_dir/helm_output.txt"
        local helm_pid_file="$temp_dir/helm.pid"

        # Start helm in background
        if [[ "${DEBUG:-false}" == "true" ]]; then
            helm "${helm_args[@]}" > "$helm_output_file" 2>&1 &
        else
            helm "${helm_args[@]}" > "$helm_output_file" 2>&1 &
        fi
        local helm_pid=$!
        echo "$helm_pid" > "$helm_pid_file"

        # Monitor pods while helm is running
        monitor_pod_status_with_helm "$namespace" "$helm_pid"

        # Wait for helm to finish (should already be done since monitor stopped)
        wait "$helm_pid" 2>/dev/null
        local helm_exit_code=$?

        if [[ $helm_exit_code -eq 0 ]]; then
            log_success "Helm release '$release_name' installed/upgraded successfully"
        else
            log_error "Failed to install/upgrade Helm release '$release_name'"
            if [[ -f "$helm_output_file" ]]; then
                cat "$helm_output_file"
            fi
            log_warning "Please use 'kubectl get pods -n $namespace' to list pods, then use 'kubectl describe pod <pod-name> -n $namespace' on failing pods to investigate issues."
        fi

        # Clean up temp files after reading output
        rm -rf "$temp_dir"

        if [[ $helm_exit_code -ne 0 ]]; then
            return 1
        fi
    else
        # No monitor flag: run helm normally without monitoring
        local helm_output
        local helm_exit_code=0
        if [[ "${DEBUG:-false}" == "true" ]]; then
            helm "${helm_args[@]}"
            helm_exit_code=$?
        else
            helm_output=$(helm "${helm_args[@]}" 2>&1)
            helm_exit_code=$?
            if [[ $helm_exit_code -ne 0 ]]; then
                echo "$helm_output"
            fi
        fi

        if [[ $helm_exit_code -eq 0 ]]; then
            log_success "Helm release '$release_name' installed/upgraded successfully"
        else
            log_error "Failed to install/upgrade Helm release '$release_name'"
            return 1
        fi
    fi
}

# Install cert-manager (prerequisite for TLS certificate management)
install_cert_manager() {
    log_info "Installing cert-manager..."

    install_helm_chart "cert-manager" "$CERT_MANAGER_REPO/cert-manager" "cert-manager" "true" "true" "true" "300" \
        "--version" "$CERT_MANAGER_VERSION" \
        "--set" "crds.enabled=true"

    # Wait for cert-manager webhook to be ready
    log_info "Waiting for cert-manager webhook to be ready..."
    if kubectl wait --for=condition=available deployment/cert-manager-webhook \
        -n "cert-manager" --timeout=120s >/dev/null 2>&1; then
        log_success "cert-manager webhook is ready"
    else
        log_warning "cert-manager webhook readiness check timed out, but continuing..."
    fi
}

# Install External Secrets Operator (prerequisite for secret management)
install_eso() {
    log_info "Installing External Secrets Operator..."

    install_helm_chart "external-secrets" "$ESO_REPO/external-secrets" "external-secrets" "true" "true" "true" "300" \
        "--version" "${ESO_VERSION#v}" \
        "--set" "installCRDs=true"
}

install_gateway_crds() {
    log_info "Installing Gateway API CRDs..."
    kubectl apply --server-side \
        -f https://github.com/kubernetes-sigs/gateway-api/releases/download/v1.4.1/experimental-install.yaml >/dev/null
    log_success "Gateway API CRDs installed"
}

# Install kgateway CRDs and controller in both CP and DP namespaces
install_kgateway() {
    log_info "Installing kgateway ($KGATEWAY_VERSION)..."

    local chart_ref="oci://cr.kgateway.dev/kgateway-dev/charts"

    install_helm_chart "kgateway-crds" "$chart_ref/kgateway-crds" "default" "false" "false" "false" "300" \
        "--version" "$KGATEWAY_VERSION"

    install_helm_chart "kgateway" "$chart_ref/kgateway" "$CONTROL_PLANE_NS" "true" "false" "false" "300" \
        "--version" "$KGATEWAY_VERSION"

    install_helm_chart "kgateway-dp" "$chart_ref/kgateway" "$DATA_PLANE_NS" "true" "false" "false" "300" \
        "--version" "$KGATEWAY_VERSION"

    log_success "kgateway installed"
}

# Install Thunder identity provider
install_thunder() {
    log_info "Installing Thunder ($THUNDER_VERSION)..."

    local thunder_values="$SCRIPT_DIR/../k3d/common/values-thunder.yaml"
    if [[ ! -f "$thunder_values" ]]; then
        # Inside container
        thunder_values="/home/openchoreo/install/k3d/common/values-thunder.yaml"
    fi

    install_helm_chart "thunder" "oci://ghcr.io/asgardeo/helm-charts/thunder" "$CONTROL_PLANE_NS" "true" "false" "true" "600" \
        "--version" "$THUNDER_VERSION" \
        "--values" "$thunder_values"

    log_success "Thunder installed"
}

# Apply CoreDNS custom config for *.openchoreo.localhost resolution
apply_coredns_config() {
    log_info "Applying CoreDNS custom config..."

    local coredns_config="$SCRIPT_DIR/../k3d/common/coredns-custom.yaml"
    if [[ ! -f "$coredns_config" ]]; then
        coredns_config="/home/openchoreo/install/k3d/common/coredns-custom.yaml"
    fi

    kubectl apply -f "$coredns_config" >/dev/null
    log_success "CoreDNS config applied"
}

# Copy cluster-gateway CA from control plane to data plane namespace
setup_data_plane_ca() {
    log_info "Setting up Data Plane CA..."

    if ! namespace_exists "$DATA_PLANE_NS"; then
        kubectl create namespace "$DATA_PLANE_NS" >/dev/null
    fi

    # Copy CA ConfigMap
    local ca_crt
    ca_crt=$(kubectl get configmap cluster-gateway-ca -n "$CONTROL_PLANE_NS" -o jsonpath='{.data.ca\.crt}')

    kubectl create configmap cluster-gateway-ca \
        --from-literal=ca.crt="$ca_crt" \
        -n "$DATA_PLANE_NS" -o yaml --dry-run=client | kubectl apply --server-side -f - >/dev/null 2>&1

    # Copy CA Secret (needed by cluster-agent CA issuer)
    local tls_crt tls_key
    tls_crt=$(kubectl get secret cluster-gateway-ca -n "$CONTROL_PLANE_NS" -o jsonpath='{.data.tls\.crt}' | base64 -d)
    tls_key=$(kubectl get secret cluster-gateway-ca -n "$CONTROL_PLANE_NS" -o jsonpath='{.data.tls\.key}' | base64 -d)

    kubectl create secret generic cluster-gateway-ca \
        --from-literal=tls.crt="$tls_crt" \
        --from-literal=tls.key="$tls_key" \
        --from-literal=ca.crt="$ca_crt" \
        -n "$DATA_PLANE_NS" -o yaml --dry-run=client | kubectl apply --server-side -f - >/dev/null 2>&1

    log_success "Data Plane CA configured"
}

# Copy cluster-gateway CA from control plane to build plane namespace
setup_build_plane_ca() {
    log_info "Setting up Build Plane CA..."

    if ! namespace_exists "$BUILD_PLANE_NS"; then
        kubectl create namespace "$BUILD_PLANE_NS" >/dev/null
    fi

    # Copy CA ConfigMap
    local ca_crt
    ca_crt=$(kubectl get configmap cluster-gateway-ca -n "$CONTROL_PLANE_NS" -o jsonpath='{.data.ca\.crt}')

    kubectl create configmap cluster-gateway-ca \
        --from-literal=ca.crt="$ca_crt" \
        -n "$BUILD_PLANE_NS" -o yaml --dry-run=client | kubectl apply --server-side -f - >/dev/null 2>&1

    # Copy CA Secret (needed by cluster-agent CA issuer)
    local tls_crt tls_key
    tls_crt=$(kubectl get secret cluster-gateway-ca -n "$CONTROL_PLANE_NS" -o jsonpath='{.data.tls\.crt}' | base64 -d)
    tls_key=$(kubectl get secret cluster-gateway-ca -n "$CONTROL_PLANE_NS" -o jsonpath='{.data.tls\.key}' | base64 -d)

    kubectl create secret generic cluster-gateway-ca \
        --from-literal=tls.crt="$tls_crt" \
        --from-literal=tls.key="$tls_key" \
        --from-literal=ca.crt="$ca_crt" \
        -n "$BUILD_PLANE_NS" -o yaml --dry-run=client | kubectl apply --server-side -f - >/dev/null 2>&1

    log_success "Build Plane CA configured"
}

# Create fake ClusterSecretStore for development
create_fake_secret_store() {
    log_info "Creating development ClusterSecretStore..."

    kubectl apply --server-side -f - >/dev/null 2>&1 <<CSSEOF
apiVersion: external-secrets.io/v1
kind: ClusterSecretStore
metadata:
  name: default
spec:
  provider:
    fake:
      data:
      - key: npm-token
        value: "fake-npm-token-for-development"
      - key: docker-username
        value: "dev-user"
      - key: docker-password
        value: "dev-password"
      - key: github-pat
        value: "fake-github-token-for-development"
      - key: username
        value: "dev-user"
      - key: password
        value: "dev-password"
      - key: RCA_LLM_API_KEY
        value: "fake-llm-api-key-for-development"
CSSEOF

    log_success "ClusterSecretStore created"
}

# Patch kgateway envoy deployment to mount /tmp as emptyDir
# Ref: https://github.com/kgateway-dev/kgateway/issues/9800
patch_gateway_tmp_volume() {
    local namespace="$1"
    log_info "Patching gateway /tmp volume in $namespace..."

    local patch_file="$SCRIPT_DIR/../k3d/common/gateway-tmp-volume-patch.json"
    if [[ ! -f "$patch_file" ]]; then
        patch_file="/home/openchoreo/install/k3d/common/gateway-tmp-volume-patch.json"
    fi

    if [[ -f "$patch_file" ]]; then
        if kubectl patch deployment gateway-default -n "$namespace" \
            --type='json' -p="$(cat "$patch_file")" >/dev/null 2>&1; then
            log_info "Waiting for gateway rollout in $namespace..."
            kubectl rollout status deployment/gateway-default -n "$namespace" --timeout=120s >/dev/null 2>&1 || true
        fi
    fi
}

# Extract cluster-agent CA and create DataPlane CR
create_dataplane_resource() {
    log_info "Creating DataPlane resource..."

    # Wait for cluster-agent-tls secret
    local max_attempts=60
    local attempt=0
    while ! kubectl get secret cluster-agent-tls -n "$DATA_PLANE_NS" >/dev/null 2>&1; do
        attempt=$((attempt + 1))
        if [[ $attempt -ge $max_attempts ]]; then
            log_warning "Timed out waiting for cluster-agent-tls secret"
            return 1
        fi
        sleep 2
    done

    local agent_ca
    agent_ca=$(kubectl get secret cluster-agent-tls -n "$DATA_PLANE_NS" -o jsonpath='{.data.ca\.crt}' | base64 -d)

    kubectl apply -f - >/dev/null <<DPEOF
apiVersion: openchoreo.dev/v1alpha1
kind: DataPlane
metadata:
  name: default
  namespace: default
spec:
  planeID: default-dataplane
  clusterAgent:
    clientCA:
      value: |
$(echo "$agent_ca" | sed 's/^/        /')
  secretStoreRef:
    name: default
  gateway:
    publicVirtualHost: openchoreoapis.localhost
    publicHTTPPort: 19080
    publicHTTPSPort: 19443
DPEOF

    log_success "DataPlane resource created"
}

# Create backstage secret with required credentials
create_backstage_secret() {
    local namespace="$1"
    log_info "Creating backstage secret..."

    local backend_secret
    backend_secret=$(head -c 32 /dev/urandom | base64 | tr -d '\n')

    kubectl create secret generic backstage-secrets \
        --namespace "$namespace" \
        --from-literal=backend-secret="$backend_secret" \
        --from-literal=client-secret="backstage-portal-secret" \
        --from-literal=jenkins-api-key="placeholder-not-in-use" \
        -o yaml --dry-run=client | kubectl apply --server-side -f - >/dev/null 2>&1

    log_success "Backstage secret created"
}

# Install OpenChoreo Control Plane
install_control_plane() {
    log_info "Installing OpenChoreo Control Plane..."
    # Disable --wait for control plane to avoid deadlock with webhook cert hooks
    # But keep monitoring enabled to track pod status
    install_helm_chart "openchoreo-control-plane" "openchoreo-control-plane" "$CONTROL_PLANE_NS" "true" "false" "true" "1800" \
        "--values" "$HOME/.values-cp.yaml" \
        "--set" "controllerManager.image.tag=$OPENCHOREO_VERSION" \
        "--set" "openchoreoApi.image.tag=$OPENCHOREO_VERSION" \
        "--set" "backstage.image.tag=$BACKSTAGE_VERSION"

    # Wait for cluster-gateway to be ready (required for agent connections)
    log_info "Waiting for cluster-gateway to be ready..."
    if kubectl wait --for=condition=available deployment/cluster-gateway \
        -n "$CONTROL_PLANE_NS" --timeout=120s >/dev/null 2>&1; then
        log_success "Cluster gateway is ready"
    else
        log_warning "Cluster gateway readiness check timed out, but continuing..."
    fi
}

# Install OpenChoreo Data Plane
install_data_plane() {
    log_info "Installing OpenChoreo Data Plane..."
    install_helm_chart "openchoreo-data-plane" "openchoreo-data-plane" "$DATA_PLANE_NS" "true" "true" "true" "1800" \
        "--values" "$HOME/.values-dp.yaml" \
        "--set" "observability.enabled=${ENABLE_OBSERVABILITY:-false}"
}

# Configure the dataplane and buildplane with observabilityplane reference
configure_observabilityplane_reference() {
    log_info "Configuring OpenChoreo Data Plane with observabilityplane reference..."
    kubectl patch dataplane default -n default --type merge -p '{"spec":{"observabilityPlaneRef":{"kind":"ObservabilityPlane","name":"default"}}}' >/dev/null
    if [[ "$ENABLE_BUILD_PLANE" == "true" ]]; then
        log_info "Configuring OpenChoreo Build Plane with observabilityplane reference..."
        kubectl patch buildplane default -n default --type merge -p '{"spec":{"observabilityPlaneRef":{"kind":"ObservabilityPlane","name":"default"}}}' >/dev/null
    fi
}

# Install Container Registry (required for Build Plane)
install_registry() {
    log_info "Installing Container Registry..."

    # Add twuni helm repo if not present
    if ! helm repo list 2>/dev/null | grep -q "twuni"; then
        helm repo add twuni https://twuni.github.io/docker-registry.helm
    fi
    helm repo update twuni

    install_helm_chart "registry" "twuni/docker-registry" "$BUILD_PLANE_NS" "true" "true" "true" "300" \
        "--values" "$HOME/.values-registry.yaml"
}

# Install OpenChoreo Build Plane (optional)
install_build_plane() {
    log_info "Installing OpenChoreo Build Plane..."
    install_helm_chart "openchoreo-build-plane" "openchoreo-build-plane" "$BUILD_PLANE_NS" "true" "true" "true" "1800" \
        "--values" "$HOME/.values-bp.yaml"
}

# Install OpenChoreo Observability Plane (optional)
install_observability_plane() {
    log_info "Installing OpenChoreo Observability Plane..."

    # Generate machine-id for fluent-bit
    log_info "Generating machine-id for observability..."
    docker exec "k3d-${CLUSTER_NAME}-server-0" sh -c "cat /proc/sys/kernel/random/uuid | tr -d '-' > /etc/machine-id"

    install_helm_chart "openchoreo-observability-plane" "openchoreo-observability-plane" "$OBSERVABILITY_NS" "true" "true" "true" "1800" \
        "--values" "$HOME/.values-op.yaml" \
        "--set" "observer.image.tag=$OPENCHOREO_VERSION"
}

# Install default OpenChoreo resources (Project, Environments, ComponentTypes, etc.)
install_default_resources() {
    log_info "Installing default OpenChoreo resources..."

    local resources_file="${SCRIPT_DIR}/../samples/getting-started/all.yaml"

    if [[ ! -f "$resources_file" ]]; then
        # Try alternative path inside container
        resources_file="/home/openchoreo/samples/getting-started/all.yaml"
    fi

    if [[ ! -f "$resources_file" ]]; then
        log_warning "Default resources file not found, skipping"
        return 0
    fi

    # Wait for CRDs to be available
    log_info "Waiting for OpenChoreo CRDs to be ready..."
    local max_attempts=30
    local attempt=0
    while ! kubectl get crd projects.openchoreo.dev >/dev/null 2>&1; do
        attempt=$((attempt + 1))
        if [[ $attempt -ge $max_attempts ]]; then
            log_warning "Timed out waiting for CRDs, skipping default resources"
            return 0
        fi
        sleep 2
    done

    local apply_attempts=3
    local apply_attempt=0
    while [[ $apply_attempt -lt $apply_attempts ]]; do
        apply_attempt=$((apply_attempt + 1))
        local apply_output
        if apply_output=$(kubectl apply -f "$resources_file" 2>&1); then
            log_success "Default resources installed"
            return 0
        fi
        if [[ $apply_attempt -lt $apply_attempts ]]; then
            log_info "Retrying resource apply (attempt $((apply_attempt + 1))/$apply_attempts)..."
            sleep 5
        fi
    done

    log_warning "Failed to install default resources after $apply_attempts attempts:"
    echo "$apply_output"
    log_info "You can apply them manually: kubectl apply -f samples/getting-started/all.yaml"
}

# Label the default namespace as a control plane namespace
label_default_namespace() {
    log_info "Labeling default namespace..."

    if kubectl label namespace default openchoreo.dev/controlplane-namespace=true --overwrite >/dev/null 2>&1; then
        log_success "Default namespace labeled successfully"
    else
        log_warning "Failed to label default namespace"
        return 1
    fi
}

# Print installation configuration
print_installation_config() {
    log_info "Configuration:"
    log_info "  Cluster Name: $CLUSTER_NAME"
    if [[ "$DEV_MODE" == "true" ]]; then
        log_info "  Mode: DEV (using local images and helm charts)"
    else
        log_info "  Image version: $OPENCHOREO_VERSION"
        log_info "  Chart version: ${OPENCHOREO_CHART_VERSION:-<latest from registry>}"
    fi
    log_info "  Enable Build Plane: ${ENABLE_BUILD_PLANE:-false}"
    log_info "  Enable Observability: ${ENABLE_OBSERVABILITY:-false}"
    if [[ "${SKIP_RESOURCE_CHECK:-false}" == "true" ]]; then
        log_warning "  Resource Check: DISABLED (--skip-resource-check flag provided)"
    fi
}

# Check system resources (CPU, Memory, Disk)
# Calculates requirements based on enabled planes and validates system has sufficient resources
check_system_resources() {
    # Skip resource check if explicitly disabled
    if [[ "${SKIP_RESOURCE_CHECK:-false}" == "true" ]]; then
        log_warning "System resource validation skipped (--skip-resource-check flag provided)"
        return 0
    fi

    log_info "Checking system resources..."

    # Get system resource information
    local cpus
    local memory_gb
    local disk_gb

    # Detect OS and get CPU count (only needed for Linux containers)
    case "$(uname -s)" in
        Linux)
            # Linux: use /proc/cpuinfo and /proc/meminfo
            cpus=$(grep -c ^processor /proc/cpuinfo 2>/dev/null || echo "0")
            memory_gb=$(grep MemTotal /proc/meminfo 2>/dev/null | awk '{print int($2/1024/1024)}' || echo "0")
            ;;
        *)
            log_warning "System resource check is only supported on Linux containers"
            return 0
            ;;
    esac

    # Get disk space for home directory (where k3d cluster will be created)
    local disk_available_kb
    disk_available_kb=$(df "$HOME" 2>/dev/null | tail -1 | awk '{print $4}' || echo "0")
    disk_gb=$((disk_available_kb / 1024 / 1024))

    # Calculate required resources based on actual measured usage:
    # - k3s kubernetes control plane: ~1.5GB baseline
    # - Control Plane pods: ~350MB (controller, api, ui, thunder)
    # - Data Plane pods: ~200MB (envoy-gateway, gateway)
    # - Build Plane pods: ~70MB steady + ~500MB during builds (argo, registry)
    # - Observability pods: ~1GB (OpenSearch is memory intensive)
    #
    # Base requirements (Control + Data Planes)
    local required_cpus_min=1
    local required_cpus_rec=2
    local required_memory_min=2
    local required_memory_rec=3
    local required_disk=10

    # Add build plane requirements
    # Build plane adds ~70MB steady state but needs headroom for concurrent builds
    if [[ "$ENABLE_BUILD_PLANE" == "true" ]]; then
        required_cpus_min=$((required_cpus_min + 1))
        required_cpus_rec=$((required_cpus_rec + 1))
        required_memory_min=$((required_memory_min + 1))
        required_memory_rec=$((required_memory_rec + 1))
        required_disk=$((required_disk + 5))
    fi

    # Add observability plane requirements
    # OpenSearch requires significant memory for indexing and queries
    if [[ "$ENABLE_OBSERVABILITY" == "true" ]]; then
        required_cpus_min=$((required_cpus_min + 1))
        required_cpus_rec=$((required_cpus_rec + 1))
        required_memory_min=$((required_memory_min + 1))
        required_memory_rec=$((required_memory_rec + 2))
        required_disk=$((required_disk + 10))
    fi

    # Display current system resources
    log_info "System Resources:"
    echo "  CPU Cores: $cpus"
    echo "  Memory: ${memory_gb}GB"
    echo "  Available Disk Space (home): ${disk_gb}GB"

    # Display required resources
    log_info "Required Resources:"
    local config_desc="Base (Control + Data Planes)"
    if [[ "$ENABLE_OBSERVABILITY" == "true" && "$ENABLE_BUILD_PLANE" == "true" ]]; then
        config_desc="Base + Observability + Build Planes"
    elif [[ "$ENABLE_OBSERVABILITY" == "true" ]]; then
        config_desc="Base + Observability Plane"
    elif [[ "$ENABLE_BUILD_PLANE" == "true" ]]; then
        config_desc="Base + Build Plane"
    fi
    echo "  Configuration: $config_desc"
    echo "  Minimum: ${required_cpus_min} vCPUs, ${required_memory_min}GB RAM, ${required_disk}GB Disk"
    echo "  Recommended: ${required_cpus_rec} vCPUs, ${required_memory_rec}GB RAM, ${required_disk}GB Disk"

    local has_errors=false
    local has_warnings=false

    # Check CPU
    if [[ $cpus -lt $required_cpus_min ]]; then
        log_error "Insufficient CPU: have $cpus vCPUs, minimum required is $required_cpus_min vCPUs"
        has_errors=true
    elif [[ $cpus -lt $required_cpus_rec ]]; then
        log_warning "CPU below recommended: have $cpus vCPUs, recommended is $required_cpus_rec vCPUs"
        has_warnings=true
    fi

    # Check Memory
    if [[ $memory_gb -lt $required_memory_min ]]; then
        log_error "Insufficient Memory: you have ${memory_gb}GB allocated to the container runtime; the minimum required is ${required_memory_min}GB"
        has_errors=true
    elif [[ $memory_gb -lt $required_memory_rec ]]; then
        log_warning "Memory below recommended: you have ${memory_gb}GB allocated to the container runtime; the recommended is ${required_memory_rec}GB"
        has_warnings=true
    fi

    # Check Disk Space
    if [[ $disk_gb -lt $required_disk ]]; then
        log_error "Insufficient Disk Space: have ${disk_gb}GB available, minimum required is ${required_disk}GB"
        has_errors=true
    fi

    if [[ "$has_errors" == "true" ]]; then
        log_error "System resources do not meet minimum requirements"
        log_error "Please increase the resource allocations in your container runtime (Docker Desktop, Colima, Rancher Desktop, etc.) and re-run this command."
        return 1
    fi

    if [[ "$has_warnings" == "true" ]]; then
        log_warning "System resources are below recommended levels - installation may be slow or experience issues"
    else
        log_success "System resources meet requirements"
    fi

    return 0
}

# Verify prerequisites
verify_prerequisites() {
    log_info "Verifying prerequisites..."

    local missing_tools=()

    if ! command_exists k3d; then
        missing_tools+=("k3d")
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
        return 1
    fi

    log_success "All prerequisites verified"

    # Check system resources
    if ! check_system_resources; then
        return 1
    fi
}


# =================================================================
# Docker image preloading function
# =================================================================

# Preload images to k3d using the .preload-images.sh script
preload_images() {
    local preload_script="${SCRIPT_DIR}/.preload-images.sh"

    if [[ ! -f "$preload_script" ]]; then
        log_warning "Image preload script not found at $preload_script"
        log_warning "Skipping image preloading - deployments may be slower"
        return 0
    fi

    log_info "Preloading Docker images for faster deployments..."

    # Build arguments for preload script
    local preload_args=(
        "--cluster" "$CLUSTER_NAME"
        "--control-plane"
        "--cp-values" "${SCRIPT_DIR}/.values-cp.yaml"
        "--data-plane"
        "--dp-values" "${SCRIPT_DIR}/.values-dp.yaml"
    )

    # Use local charts when in dev mode
    if [[ "$DEV_MODE" == "true" ]]; then
        preload_args+=("--local-charts")
        log_info "Using local Helm charts (DEV_MODE enabled)"
    fi

    # Add --version flag only if OPENCHOREO_CHART_VERSION is not empty
    # Skip version in dev mode since we're using local charts
    if [[ -n "$OPENCHOREO_CHART_VERSION" && "$DEV_MODE" != "true" ]]; then
        preload_args+=("--version" "$OPENCHOREO_CHART_VERSION")
    fi

    # Add build plane if enabled
    if [[ "$ENABLE_BUILD_PLANE" == "true" ]]; then
        preload_args+=(
            "--build-plane"
            "--bp-values" "${SCRIPT_DIR}/.values-bp.yaml"
        )
    fi

    # Add observability plane if enabled
    if [[ "$ENABLE_OBSERVABILITY" == "true" ]]; then
        preload_args+=(
            "--observability-plane"
            "--op-values" "${SCRIPT_DIR}/.values-op.yaml"
        )
    fi

    # Run the preload script
    if run_command bash "$preload_script" "${preload_args[@]}"; then
        log_success "Image preloading complete"
    else
        log_warning "Image preloading failed - continuing with installation"
        log_warning "Deployments may be slower due to image pulls"
    fi
}
