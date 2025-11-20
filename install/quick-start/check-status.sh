#!/usr/bin/env bash
set -eo pipefail

# Source shared configuration
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/.config.sh"

# Component groups organized by architectural layers (bash 3.2 compatible)
get_component_group() {
    local group="$1"
    case "$group" in
        "Control_Plane") echo "cert_manager_cp controller_manager" ;; # TODO: add api_server, backstage and thunder
        "Data_Plane") echo "envoy_gateway" ;;
        "Build_Plane") echo "argo_workflow_controller registry" ;;
        "Observability_Plane") echo "opensearch opensearch_dashboard observer" ;;
        *) echo "" ;;
    esac
}

# Group order for display (using underscores for bash compatibility)
group_order=("Control_Plane" "Data_Plane" "Build_Plane" "Observability_Plane")

# Group display names
get_group_display_name() {
    local group="$1"
    case "$group" in
        "Control_Plane") echo "Control Plane" ;;
        "Data_Plane") echo "Data Plane" ;;
        "Build_Plane") echo "Build Plane" ;;
        "Observability_Plane") echo "Observability Plane" ;;
        *) echo "$group" ;;
    esac
}

# Component lists for multi-cluster mode (kept for backward compatibility)
components_cp=("cert_manager_cp" "controller_manager" "api_server")
components_dp=("envoy_gateway")

# Core vs optional component classification (used in multi-cluster mode)
core_components=("cert_manager_cp" "controller_manager" "api_server" "envoy_gateway")
optional_components=("opensearch" "opensearch_dashboard" "observer")

# Function to get component configuration (namespace:label)
get_component_config() {
    local component="$1"
    case "$component" in
        "cert_manager_cp") echo "$CONTROL_PLANE_NS:app.kubernetes.io/name=cert-manager" ;;
        "controller_manager") echo "$CONTROL_PLANE_NS:app.kubernetes.io/name=openchoreo-control-plane,app.kubernetes.io/component=controller-manager" ;;
        "api_server") echo "$CONTROL_PLANE_NS:app.kubernetes.io/name=openchoreo-control-plane,app.kubernetes.io/component=api-server" ;;
        "envoy_gateway") echo "$DATA_PLANE_NS:app.kubernetes.io/name=gateway-helm" ;;
        "argo_workflow_controller") echo "$BUILD_PLANE_NS:app.kubernetes.io/name=argo-workflows-workflow-controller" ;;
        "registry") echo "$BUILD_PLANE_NS:app=registry" ;;
        "opensearch") echo "$OBSERVABILITY_NS:app.kubernetes.io/component=opensearch-master" ;;
        "opensearch_dashboard") echo "$OBSERVABILITY_NS:app.kubernetes.io/name=opensearch-dashboards" ;;
        "observer") echo "$OBSERVABILITY_NS:app.kubernetes.io/component=observer" ;;
        *) echo "unknown:unknown" ;;
    esac
}

# Helper function to check if a namespace exists
namespace_exists() {
    local namespace="$1"
    local context="$2"
    kubectl --context="$context" get namespace "$namespace" >/dev/null 2>&1
}

# Check the status of pods for a given component
check_component_status() {
    local component="$1"
    local context="$2"

    # Get namespace and label from component config
    local config
    config=$(get_component_config "$component")
    if [[ "$config" == "unknown:unknown" ]]; then
        echo "unknown"
        return
    fi

    local namespace="${config%%:*}"
    local label="${config##*:}"

    # Check if namespace exists
    if ! namespace_exists "$namespace" "$context"; then
        echo "not installed"
        return
    fi

    # Get pod status
    local pod_status
    pod_status=$(kubectl --context="$context" get pods -n "$namespace" -l "$label" \
        -o jsonpath="{.items[*].status.conditions[?(@.type=='Ready')].status}" 2>/dev/null)

    if [[ -z "$pod_status" ]]; then
        echo "not started"
        return
    fi

    if [[ "$pod_status" =~ "False" ]]; then
        echo "pending"
    elif [[ "$pod_status" =~ "True" ]]; then
        echo "ready"
    else
        echo "unknown"
    fi
}

# Get status text for a component
get_status_text() {
    local status="$1"
    case "$status" in
        "ready") echo "[READY]" ;;
        "not installed") echo "[NOT_INSTALLED]" ;;
        "pending") echo "[PENDING]" ;;
        "not started") echo "[ERROR]" ;;
        "unknown") echo "[UNKNOWN]" ;;
        *) echo "[ERROR]" ;;
    esac
}

# Print components grouped by architectural layers
print_grouped_components() {
    local context="$1"

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

        # Determine group type
        local group_type=""
        case "$group" in
            "Control_Plane")
                group_type="Core"
                ;;
            "Data_Plane")
                group_type="Core"
                ;;
            "Build_Plane")
                group_type="Optional"
                ;;
            "Observability_Plane")
                group_type="Optional"
                ;;
        esac

        echo ""
        # Fixed width: 70 characters total
        local total_width=70
        local header_text="${group_display_name} (${group_type})"
        local header_length=${#header_text}
        local dashes_length=$((total_width - header_length - 5))  # 5 for "+- ", " ", and "+"
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

            # Fixed layout: "| component (25 chars) status (rest) |"
            local content="${component} ${status_text}"
            local content_length=${#content}
            local padding_length=$((total_width - content_length - 4))  # 4 for "| " and " |"
            local padding=""
            for ((i=0; i<padding_length; i++)); do
                padding="${padding} "
            done

            printf "| %s %s%s |\n" "$component" "$status_text" "$padding"
        done

        # Bottom border - exact width
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

    # Use eval to get the array contents by name
    eval "local comp_list=(\"\${${comp_list_name}[@]}\")"

    for component in "${comp_list[@]}"; do
        local status
        local component_type="core"

        # Check if this is an optional component
        if [[ " ${optional_components[*]} " =~ " ${component} " ]]; then
            component_type="optional"
        fi

        status=$(check_component_status "$component" "$context")

        printf "%-30s %-15s %-15s\n" "$component" "$status" "$component_type"
    done
}

# --------------------------
# Main
# --------------------------

SINGLE_CLUSTER=true

# Parse command line arguments
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

    read -r -p "Enter DataPlane Kubernetes context (default: kind-choreo-dp): " dataplane_context
    dataplane_context=${dataplane_context:-"kind-choreo-dp"}

    read -r -p "Enter Control Plane Kubernetes context (default: kind-choreo-cp): " control_plane_context
    control_plane_context=${control_plane_context:-"kind-choreo-cp"}

    print_component_status components_cp "Control Plane Components" "$control_plane_context"
    print_component_status components_dp "Data Plane Components" "$dataplane_context"
fi
