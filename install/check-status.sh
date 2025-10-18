#!/bin/bash

# Color codes
RED="\e[0;31m"
GREEN="\e[0;32m"
DARK_YELLOW="\e[0;33m"
BLUE="\e[0;34m"
CYAN="\e[0;36m"
PURPLE="\e[0;35m"
BOLD="\e[1m"
RESET="\e[0m"

# Status icons
ICON_READY="✅"
ICON_PENDING="⏳"
ICON_NOT_INSTALLED="⚠️ "
ICON_ERROR="❌"
ICON_UNKNOWN="❓"

# Namespace definitions
CONTROL_PLANE_NS="openchoreo-control-plane"
DATA_PLANE_NS="openchoreo-data-plane"
BUILD_PLANE_NS="openchoreo-build-plane"
IDENTITY_NS="openchoreo-identity-system"
OBSERVABILITY_NS="openchoreo-observability-plane"
CILIUM_NS="cilium"

# Component groups organized by architectural layers (bash 3.2 compatible)
get_component_group() {
    local group="$1"
    case "$group" in
        "Networking") echo "cilium" ;;
        "Control_Plane") echo "cert_manager_cp controller_manager api_server platform_identity_provider" ;;
        "Data_Plane") echo "vault registry redis envoy_gateway external_gateway internal_gateway fluent_bit_dp" ;;
        "Build_Plane") echo "build_plane" ;;
        "Identity_Provider") echo "workload_identity_provider" ;;
        "Observability_Plane") echo "opensearch opensearch_dashboard observer" ;;
        *) echo "" ;;
    esac
}

# Group order for display (using underscores for bash compatibility)
group_order=("Networking" "Control_Plane" "Data_Plane" "Build_Plane" "Identity_Provider" "Observability_Plane")

# Group display names
get_group_display_name() {
    local group="$1"
    case "$group" in
        "Networking") echo "Networking" ;;
        "Control_Plane") echo "Control Plane" ;;
        "Data_Plane") echo "Data Plane" ;;
        "Build_Plane") echo "Build Plane" ;;
        "Identity_Provider") echo "Identity Provider" ;;
        "Observability_Plane") echo "Observability Plane" ;;
        *) echo "$group" ;;
    esac
}

# Component lists for multi-cluster mode (kept for backward compatibility)
components_cp=("cert_manager_cp" "controller_manager" "api_server" "platform_identity_provider")
components_dp=(
    "cilium" "vault" "registry" "redis" "envoy_gateway"
    "external_gateway" "internal_gateway" "fluent_bit_dp"
    "build_plane" "workload_identity_provider" "opensearch" "opensearch_dashboard" "observer"
)

# Core vs optional component classification
core_components=("cilium" "cert_manager_cp" "controller_manager" "api_server" "vault" "registry" "redis" "envoy_gateway" "external_gateway" "internal_gateway" "fluent_bit_dp")
optional_components=("build_plane" "platform_identity_provider" "workload_identity_provider" "opensearch" "opensearch_dashboard" "observer")

# Function to get component configuration (namespace:label)
get_component_config() {
    local component="$1"
    case "$component" in
        "cilium") echo "$CILIUM_NS:k8s-app=cilium" ;;
        "cert_manager_cp") echo "$CONTROL_PLANE_NS:app.kubernetes.io/name=cert-manager" ;;
        "controller_manager") echo "$CONTROL_PLANE_NS:app.kubernetes.io/name=openchoreo-control-plane,app.kubernetes.io/component=controller-manager" ;;
        "api_server") echo "$CONTROL_PLANE_NS:app.kubernetes.io/name=openchoreo-control-plane,app.kubernetes.io/component=api-server" ;;
        "vault") echo "$DATA_PLANE_NS:app.kubernetes.io/name=hashicorp-vault" ;;
        "registry") echo "$DATA_PLANE_NS:app=registry" ;;
        "redis") echo "$DATA_PLANE_NS:app=redis" ;;
        "envoy_gateway") echo "$DATA_PLANE_NS:app.kubernetes.io/name=gateway-helm" ;;
        "external_gateway") echo "$DATA_PLANE_NS:gateway.envoyproxy.io/owning-gateway-name=gateway-external" ;;
        "internal_gateway") echo "$DATA_PLANE_NS:gateway.envoyproxy.io/owning-gateway-name=gateway-internal" ;;
        "fluent_bit_dp") echo "$DATA_PLANE_NS:app.kubernetes.io/component=fluent-bit" ;;
        "build_plane") echo "$BUILD_PLANE_NS:app.kubernetes.io/name=argo" ;;
        "platform_identity_provider") echo "$CONTROL_PLANE_NS:app.kubernetes.io/name=platform-identity-provider" ;;
        "workload_identity_provider") echo "$IDENTITY_NS:app.kubernetes.io/name=workload-identity-provider" ;;
        "opensearch") echo "$OBSERVABILITY_NS:app.kubernetes.io/component=opensearch" ;;
        "opensearch_dashboard") echo "$OBSERVABILITY_NS:app.kubernetes.io/component=opensearch-dashboard" ;;
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

# Get status icon for a component
get_status_icon() {
    local status="$1"
    case "$status" in
        "ready") echo "${ICON_READY}" ;;
        "not installed") echo "${ICON_NOT_INSTALLED}" ;;
        "pending") echo "${ICON_PENDING}" ;;
        "not started") echo "${ICON_ERROR}" ;;
        "unknown") echo "${ICON_UNKNOWN}" ;;
        *) echo "${ICON_ERROR}" ;;
    esac
}

# Get status color for a component
get_status_color() {
    local status="$1"
    case "$status" in
        "ready") echo "${GREEN}" ;;
        "not installed") echo "${DARK_YELLOW}" ;;
        "pending") echo "${DARK_YELLOW}" ;;
        "not started") echo "${RED}" ;;
        "unknown") echo "${PURPLE}" ;;
        *) echo "${RED}" ;;
    esac
}

# Print components grouped by architectural layers
print_grouped_components() {
    local context="$1"

    printf "\n%b%b╔══════════════════════════════════════════════════════════════════════╗%b\n" "$BOLD" "$BLUE" "$RESET"
    printf "%b%b║                     OpenChoreo Component Status                     ║%b\n" "$BOLD" "$BLUE" "$RESET"
    printf "%b%b╚══════════════════════════════════════════════════════════════════════╝%b\n" "$BOLD" "$BLUE" "$RESET"

    for group in "${group_order[@]}"; do
        local components_str
        components_str=$(get_component_group "$group")
        read -r -a components <<< "$components_str"

        local group_display_name
        group_display_name=$(get_group_display_name "$group")

        # Determine group color and type
        local group_color=""
        local group_type=""
        case "$group" in
            "Networking")
                group_color="${CYAN}"
                group_type="Infrastructure"
                ;;
            "Control_Plane")
                group_color="${BLUE}"
                group_type="Core"
                ;;
            "Data_Plane")
                group_color="${GREEN}"
                group_type="Core"
                ;;
            "Build_Plane")
                group_color="${PURPLE}"
                group_type="Optional"
                ;;
            "Identity_Provider")
                group_color="${DARK_YELLOW}"
                group_type="Optional"
                ;;
            "Observability_Plane")
                group_color="${RED}"
                group_type="Optional"
                ;;
        esac

        echo ""
        # Calculate the proper line length for consistent borders
        local line_length=65
        local header_padding=""
        local remaining_length=$((line_length - ${#group_display_name} - ${#group_type} - 6))  # 6 for "┌─ " + " (" + ") "
        for ((i=0; i<remaining_length; i++)); do
            header_padding="${header_padding}─"
        done

        printf "%b%b┌─ %s (%s) %s┐%b\n" "$group_color" "$BOLD" "$group_display_name" "$group_type" "$header_padding" "$RESET"

        for component in "${components[@]}"; do
            local status
            status=$(check_component_status "$component" "$context")
            local status_icon
            status_icon=$(get_status_icon "$status")
            local status_color
            status_color=$(get_status_color "$status")

            # Calculate padding for right border alignment
            local content_length=$((25 + ${#status} + 3))  # 25 for component width, 3 for icon
            local padding_needed=$((line_length - content_length - 4))  # 4 for "│ " + " │"
            local padding=""
            for ((i=0; i<padding_needed; i++)); do
                padding="${padding} "
            done

            printf "%b│%b %-25s %s %b%s%b%s%b│%b\n" "$group_color" "$RESET" "$component" "$status_icon" "$status_color" "$status" "$RESET" "$padding" "$group_color" "$RESET"
        done

        # Bottom border
        local bottom_line=""
        for ((i=0; i<line_length; i++)); do
            bottom_line="${bottom_line}─"
        done
        printf "%b└%s┘%b\n" "$group_color" "$bottom_line" "$RESET"
    done

    echo ""
    printf "%bLegend:%b %s Ready  %s Pending  %sNot Installed  %s Error  %s Unknown\n" "$BOLD" "$RESET" "$ICON_READY" "$ICON_PENDING" "$ICON_NOT_INSTALLED" "$ICON_ERROR" "$ICON_UNKNOWN"
}

# Legacy function for multi-cluster mode
print_component_status() {
    local comp_list_name="$1"
    local header="$2"
    local context="$3"

    echo -e "\n${BLUE}$header${RESET}"
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

        case "$status" in
            "ready")
                color=$GREEN
                ;;
            "not installed")
                color=$DARK_YELLOW
                ;;
            "pending" | "not started" | "unknown")
                color=$RED
                ;;
            *)
                color=$RED
                ;;
        esac

        printf "%-30s ${color}%-15s${RESET} %-15s\n" "$component" "$status" "$component_type"
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
