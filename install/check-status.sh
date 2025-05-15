#!/bin/bash

# Color codes
RED="\033[0;31m"
GREEN="\033[0;32m"
DARK_YELLOW="\033[0;33m"
RESET="\033[0m"

NAMESPACE="choreo-system"

# Component lists
components=("cilium" "vault" "argo" "cert_manager" "choreo_controller" "choreo_image_registry" "envoy_gateway" "redis" "external_gateway" "internal_gateway")
components_cp=("cert_manager" "choreo_controller")
components_dp=("cilium" "vault" "argo" "cert_manager" "choreo_image_registry" "envoy_gateway" "redis" "external_gateway" "internal_gateway")

# Labels
cilium_deps=("app.kubernetes.io/name=cilium-agent" "app.kubernetes.io/name=cilium-operator")
vault_deps=("app.kubernetes.io/name=vault")
argo_deps=("app.kubernetes.io/name=argo-workflows-server" "app.kubernetes.io/name=argo-workflows-workflow-controller")
cert_manager_deps=("app.kubernetes.io/name=certmanager" "app.kubernetes.io/name=cainjector" "app.kubernetes.io/name=webhook")
choreo_controller_deps=("app.kubernetes.io/name=choreo-cp")
choreo_image_registry_deps=("app=registry")
redis_deps=("app=redis")
envoy_gateway_deps=("app.kubernetes.io/name=gateway-helm")
external_gateway_deps=("gateway.envoyproxy.io/owning-gateway-name=gateway-external")
internal_gateway_deps=("gateway.envoyproxy.io/owning-gateway-name=gateway-internal")

# Global
overall_status="ready"

check_status() {
    local label="$1"
    local context="$2"

    pod_status=$(kubectl --context="$context" get pods -n "$NAMESPACE" -l "$label" \
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

get_component_status() {
    local component="$1"
    local context="$2"
    local worst_status="ready"

    eval "deps=(\"\${${component}_deps[@]}\")"

    for workload in "${deps[@]}"; do
        status=$(check_status "$workload" "$context")

        if [[ "$status" == "not started" ]]; then
            worst_status="not started"
        elif [[ "$status" == "unknown" && "$worst_status" != "not started" ]]; then
            worst_status="unknown"
        elif [[ "$status" == "pending" && "$worst_status" != "not started" && "$worst_status" != "unknown" ]]; then
            worst_status="pending"
        fi
    done

    echo "$worst_status"
}

print_component_status() {
    local comp_list_name="$1"
    local header="$2"
    local context="$3"

    eval "comp_list=(\"\${${comp_list_name}[@]}\")"

    echo "\n$header"
    printf "\n%-25s %-15s\n" "Component" "Status"
    printf "%-25s %-15s\n" "------------------------" "---------------"

    for component in "${comp_list[@]}"; do
        status=$(get_component_status "$component" "$context")

        case "$status" in
            "ready")
                color=$GREEN
                ;;
            "pending")
                color=$DARK_YELLOW
                overall_status="not ready"
                ;;
            "not started")
                color=$RED
                overall_status="not ready"
                ;;
            "unknown")
                color=$RED
                overall_status="not ready"
                ;;
            *)
                color=$RED
                overall_status="not ready"
                ;;
        esac

        printf "%-25s %b\n" "$component" "${color}${status} ${icon}${RESET}"
    done
}

# --------------------------
# Main
# --------------------------

SINGLE_CLUSTER=false
# Detect if running in single-cluster mode via env var
if [[ "$1" == "--single-cluster" ]]; then
  SINGLE_CLUSTER=true
fi

if [[ "$SINGLE_CLUSTER" == "true" ]]; then
    cluster_context=$(kubectl config current-context)
    echo "Choreo Installation Status: Single-Cluster Mode"
    echo "Using current context - "$cluster_context""
    print_component_status "components" "Single Cluster Components" "$cluster_context"
else
    echo "Choreo Installation Status: Multi-Cluster Mode"
    read -p "Enter DataPlane kubernetes context (default: kind-choreo-dp): " dataplane_context
    dataplane_context=${dataplane_context:-"kind-choreo-dp"}

    read -p "Enter Control Plane kubernetes context (default: kind-choreo-cp): " control_plane_context
    control_plane_context=${control_plane_context:-"kind-choreo-cp"}

    print_component_status "components_cp" "Control Plane Components" "$control_plane_context"
    print_component_status "components_dp" "Data Plane Components" "$dataplane_context"
fi

# Overall
if [[ "$overall_status" == "ready" ]]; then
    echo "\nOverall Status: ${GREEN}READY${RESET}"
    echo "${GREEN}🎉 Choreo has been successfully installed and is ready to use! ${RESET}"
else
    echo "\nOverall Status: ${RED}NOT READY${RESET}"
    echo "${DARK_YELLOW}⚠ Some components are still initializing. Please wait a few minutes and try again. ${RESET}"
fi
