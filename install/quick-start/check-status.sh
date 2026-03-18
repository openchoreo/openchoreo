#!/usr/bin/env bash
set -eo pipefail

# Source shared configuration
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/.config.sh"

# Component groups organized by architectural layers
get_component_group() {
    local group="$1"
    case "$group" in
        "Infrastructure") echo "cert_manager kgateway external_secrets openbao thunder" ;;
        "Control_Plane") echo "controller_manager api_server backstage cluster_gateway" ;;
        "Data_Plane") echo "cluster_agent_dp gateway_proxy" ;;
        "Workflow_Plane") echo "cluster_agent_wp argo_workflow_controller registry" ;;
        "Observability_Plane") echo "obs_controller_manager cluster_agent_op opensearch observer" ;;
        *) echo "" ;;
    esac
}

# Group order for display
group_order=("Infrastructure" "Control_Plane" "Data_Plane" "Workflow_Plane" "Observability_Plane")

# Group display names
get_group_display_name() {
    local group="$1"
    case "$group" in
        "Infrastructure") echo "Infrastructure" ;;
        "Control_Plane") echo "Control Plane" ;;
        "Data_Plane") echo "Data Plane" ;;
        "Workflow_Plane") echo "Workflow Plane" ;;
        "Observability_Plane") echo "Observability Plane" ;;
        *) echo "$group" ;;
    esac
}

# Human-readable component names for display
get_component_display_name() {
    local component="$1"
    case "$component" in
        "cert_manager") echo "Cert Manager" ;;
        "kgateway") echo "KGateway" ;;
        "external_secrets") echo "External Secrets" ;;
        "openbao") echo "OpenBao" ;;
        "thunder") echo "Thunder" ;;
        "controller_manager") echo "Controller Manager" ;;
        "api_server") echo "API Server" ;;
        "backstage") echo "Backstage" ;;
        "cluster_gateway") echo "Cluster Gateway" ;;
        "cluster_agent_dp") echo "Cluster Agent" ;;
        "gateway_proxy") echo "Gateway Proxy" ;;
        "cluster_agent_wp") echo "Cluster Agent" ;;
        "argo_workflow_controller") echo "Argo Workflow Controller" ;;
        "registry") echo "Docker Registry" ;;
        "obs_controller_manager") echo "Controller Manager" ;;
        "cluster_agent_op") echo "Cluster Agent" ;;
        "opensearch") echo "OpenSearch" ;;
        "observer") echo "Observer" ;;
        *) echo "$component" ;;
    esac
}

# Component lists for multi-cluster mode
components_cp=("cert_manager" "kgateway" "external_secrets" "openbao" "thunder" "controller_manager" "api_server" "backstage" "cluster_gateway")
components_dp=("cluster_agent_dp" "gateway_proxy")

# Core vs optional classification
core_components=("cert_manager" "kgateway" "external_secrets" "openbao" "thunder" "controller_manager" "api_server" "backstage" "cluster_gateway" "cluster_agent_dp" "gateway_proxy")
optional_components=("cluster_agent_wp" "argo_workflow_controller" "registry" "obs_controller_manager" "cluster_agent_op" "opensearch" "observer")

# Component configuration: namespace and pod label selector
# Format is namespace:label-selector
get_component_config() {
    local component="$1"
    case "$component" in
        # Infrastructure
        "cert_manager") echo "cert-manager:app.kubernetes.io/name=cert-manager" ;;
        "kgateway") echo "$CONTROL_PLANE_NS:app.kubernetes.io/name=kgateway" ;;
        "external_secrets") echo "external-secrets:app.kubernetes.io/name=external-secrets" ;;
        "openbao") echo "openbao:app.kubernetes.io/name=openbao,component=server" ;;
        "thunder") echo "$THUNDER_NS:app.kubernetes.io/name=thunder" ;;
        # Control Plane
        "controller_manager") echo "$CONTROL_PLANE_NS:app.kubernetes.io/name=openchoreo-control-plane,app.kubernetes.io/component=controller-manager" ;;
        "api_server") echo "$CONTROL_PLANE_NS:app.kubernetes.io/name=openchoreo-control-plane,app.kubernetes.io/component=api-server" ;;
        "backstage") echo "$CONTROL_PLANE_NS:app.kubernetes.io/name=openchoreo-control-plane,app.kubernetes.io/component=backstage" ;;
        "cluster_gateway") echo "$CONTROL_PLANE_NS:app.kubernetes.io/name=openchoreo-control-plane,app.kubernetes.io/component=cluster-gateway" ;;
        # Data Plane
        "cluster_agent_dp") echo "$DATA_PLANE_NS:app.kubernetes.io/name=openchoreo-data-plane,app.kubernetes.io/component=cluster-agent" ;;
        "gateway_proxy") echo "$DATA_PLANE_NS:gateway.networking.k8s.io/gateway-name=gateway-default" ;;
        # Workflow Plane
        "cluster_agent_wp") echo "$WORKFLOW_PLANE_NS:app.kubernetes.io/name=openchoreo-workflow-plane,app.kubernetes.io/component=cluster-agent" ;;
        "argo_workflow_controller") echo "$WORKFLOW_PLANE_NS:app.kubernetes.io/name=argo-workflows-workflow-controller" ;;
        "registry") echo "$WORKFLOW_PLANE_NS:app=docker-registry" ;;
        # Observability Plane
        "obs_controller_manager") echo "$OBSERVABILITY_NS:app.kubernetes.io/name=openchoreo-observability-plane,app.kubernetes.io/component=controller-manager" ;;
        "cluster_agent_op") echo "$OBSERVABILITY_NS:app.kubernetes.io/name=openchoreo-observability-plane,app.kubernetes.io/component=cluster-agent" ;;
        "opensearch") echo "$OBSERVABILITY_NS:opster.io/opensearch-cluster=opensearch" ;;
        "observer") echo "$OBSERVABILITY_NS:app.kubernetes.io/name=openchoreo-observability-plane,app.kubernetes.io/component=observer" ;;
        *) echo "unknown:unknown" ;;
    esac
}

namespace_exists() {
    local namespace="$1"
    local context="$2"
    kubectl --context="$context" get namespace "$namespace" >/dev/null 2>&1
}

# Check the status of pods for a given component
check_component_status() {
    local component="$1"
    local context="$2"

    local config
    config=$(get_component_config "$component")
    if [[ "$config" == "unknown:unknown" ]]; then
        echo "unknown"
        return
    fi

    local namespace="${config%%:*}"
    local label="${config##*:}"

    if ! namespace_exists "$namespace" "$context"; then
        echo "not installed"
        return
    fi

    local pod_status rc
    set +e
    pod_status=$(kubectl --context="$context" get pods -n "$namespace" -l "$label" \
        -o jsonpath="{.items[*].status.conditions[?(@.type=='Ready')].status}" 2>/dev/null)
    rc=$?
    set -e

    if (( rc != 0 )); then
        echo "unknown"
        return
    fi

    if [[ -z "$pod_status" ]]; then
        echo "not started"
        return
    fi

    if [[ "$pod_status" =~ False ]]; then
        echo "pending"
    elif [[ "$pod_status" =~ True ]]; then
        echo "ready"
    else
        echo "unknown"
    fi
}

get_status_text() {
    local status="$1"
    case "$status" in
        "ready") echo "[READY]" ;;
        "not installed") echo "[NOT INSTALLED]" ;;
        "pending") echo "[PENDING]" ;;
        "not started") echo "[NOT STARTED]" ;;
        "unknown") echo "[UNKNOWN]" ;;
        *) echo "[UNKNOWN]" ;;
    esac
}

# Print components grouped by architectural layers
print_grouped_components() {
    local context="$1"
    local total_width=70

    printf "\n"
    printf "======================================================================\n"
    printf "                     OpenChoreo Component Status                     \n"
    printf "======================================================================\n"

    for group in "${group_order[@]}"; do
        local components_str
        components_str=$(get_component_group "$group")
        read -r -a components <<< "$components_str"

        local group_display_name
        group_display_name=$(get_group_display_name "$group")

        local group_type=""
        case "$group" in
            "Infrastructure"|"Control_Plane"|"Data_Plane") group_type="Core" ;;
            "Workflow_Plane"|"Observability_Plane") group_type="Optional" ;;
        esac

        echo ""
        local header_text="${group_display_name} (${group_type})"
        local header_length=${#header_text}
        local dashes_length=$((total_width - header_length - 5))
        local header_padding=""
        for ((i=0; i<dashes_length; i++)); do
            header_padding="${header_padding}-"
        done

        printf "+- %s %s+\n" "$header_text" "$header_padding"

        for component in "${components[@]}"; do
            local status
            status=$(check_component_status "$component" "$context")
            local status_text
            status_text=$(get_status_text "$status")

            local display_name
            display_name=$(get_component_display_name "$component")

            local content="${display_name} ${status_text}"
            local content_length=${#content}
            local padding_length=$((total_width - content_length - 4))
            local padding=""
            for ((i=0; i<padding_length; i++)); do
                padding="${padding} "
            done

            printf "| %s %s%s |\n" "$display_name" "$status_text" "$padding"
        done

        printf "+"
        for ((i=0; i<total_width-2; i++)); do
            printf "-"
        done
        printf "+\n"
    done

    echo ""
    printf "======================================================================\n"
}

# Legacy function for multi-cluster mode
print_component_status() {
    local comp_list_name="$1"
    local header="$2"
    local context="$3"

    echo ""
    echo "$header"
    printf "\n%-30s %-15s %-15s\n" "Component" "Status" "Type"
    printf "%-30s %-15s %-15s\n" "-----------------------------" "---------------" "---------------"

    eval "local comp_list=(\"\${${comp_list_name}[@]}\")"

    for component in "${comp_list[@]}"; do
        local status
        local component_type="core"

        if [[ " ${optional_components[*]} " =~ " ${component} " ]]; then
            component_type="optional"
        fi

        status=$(check_component_status "$component" "$context")
        local display_name
        display_name=$(get_component_display_name "$component")

        printf "%-30s %-15s %-15s\n" "$display_name" "$status" "$component_type"
    done
}

# --------------------------
# Main
# --------------------------

SINGLE_CLUSTER=true

while [[ $# -gt 0 ]]; do
    case $1 in
        --multi-cluster)
            SINGLE_CLUSTER=false
            shift
            ;;
        --help|-h)
            echo "Usage: $0 [OPTIONS]"
            echo ""
            echo "Options:"
            echo "  --multi-cluster    Check multi-cluster installation"
            echo "  --help, -h         Show this help message"
            echo ""
            echo "Examples:"
            echo "  $0                   # Check single cluster (default)"
            echo "  $0 --multi-cluster   # Check multi-cluster setup"
            exit 0
            ;;
        *)
            echo "Unknown option: $1"
            echo "Use --help for usage information"
            exit 1
            ;;
    esac
done

if [[ "$SINGLE_CLUSTER" == "true" ]]; then
    cluster_context=$(kubectl config current-context)
    echo "OpenChoreo Installation Status: Single-Cluster Mode"
    echo "Using current context: $cluster_context"
    print_grouped_components "$cluster_context"
else
    echo "OpenChoreo Installation Status: Multi-Cluster Mode"

    read -r -p "Enter DataPlane Kubernetes context (default: k3d-openchoreo-dp): " dataplane_context
    dataplane_context=${dataplane_context:-"k3d-openchoreo-dp"}

    read -r -p "Enter Control Plane Kubernetes context (default: k3d-openchoreo-cp): " control_plane_context
    control_plane_context=${control_plane_context:-"k3d-openchoreo-cp"}

    print_component_status components_cp "Control Plane Components" "$control_plane_context"
    print_component_status components_dp "Data Plane Components" "$dataplane_context"
fi
