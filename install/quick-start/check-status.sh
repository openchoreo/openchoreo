#!/bin/bash

# color codes for console formatting
RED="\033[0;31m"
GREEN="\033[0;32m"
DARK_YELLOW="\033[0;33m"
RESET="\033[0m"

NAMESPACE="choreo-system"

# High level components
components=("cilium" "vault" "argo" "cert_manager" "choreo_controller" "choreo_image_registry" "envoy_gateway" "redis" "external_gateway" "internal_gateway")

# Unique label per each component
cilium_deps=("app.kubernetes.io/name=cilium-agent" "app.kubernetes.io/name=cilium-operator")
vault_deps=("app.kubernetes.io/name=vault" "app.kubernetes.io/name=vault-agent-injector")
argo_deps=("app.kubernetes.io/name=argo-workflows-server" "app.kubernetes.io/name=argo-workflows-workflow-controller")
cert_manager_deps=("app.kubernetes.io/name=certmanager" "app.kubernetes.io/name=cainjector" "app.kubernetes.io/name=webhook")
choreo_controller_deps=("app.kubernetes.io/name=choreo")
choreo_image_registry_deps=("app=registry")
redis_deps=("app=redis")
envoy_gateway_deps=("app.kubernetes.io/name=gateway-helm")
external_gateway_deps=("gateway.envoyproxy.io/owning-gateway-name=gateway-external")
internal_gateway_deps=("gateway.envoyproxy.io/owning-gateway-name=gateway-internal")

check_status() {
    local label=$1

    pod_status=$(kubectl get pods -n "$NAMESPACE" -l "$label" \
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
    local component=$1
    local worst_status="ready"

    eval "deps=(\"\${${component}_deps[@]}\")"

    for workload in "${deps[@]}"; do
        status=$(check_status "$workload")

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

container_id="$(cat /etc/hostname)"

# Check if the "kind" network exists
if docker network inspect kind &>/dev/null; then
  # Check if the container is already connected
  if [ "$(docker inspect -f '{{json .NetworkSettings.Networks.kind}}' "${container_id}")" = "null" ]; then
    docker network connect "kind" "${container_id}"
    echo "Connected container ${container_id} to kind network."
  else
    echo "Container ${container_id} is already connected to kind network."
  fi
else
  echo "Docker network 'kind' does not exist. Skipping connection."
fi

overall_status="ready"

echo -e "\nChoreo Installation Status:\n"
printf "\n%-25s %-15s\n" "Component" "Status"
printf "%-25s %-15s\n" "------------------------" "---------------"

for component in "${components[@]}"; do
    status=$(get_component_status "$component")

    case "$status" in
        "ready")
            icon="✅"
            color=$GREEN
            ;;
        "pending")
            icon="🕑"
            color=$DARK_YELLOW
            overall_status="not ready"
            ;;
        "not started")
            icon="❌"
            color=$RED
            overall_status="not ready"
            ;;
        "unknown")
            icon="⚠"
            color=$RED
            overall_status="not ready"
            ;;
        *)
            icon="❓"
            color=$RED
            overall_status="not ready"
            ;;
    esac

    printf "%-25s %b\n" "$component" "${color}${icon} $status${RESET}"
done

if [[ "$overall_status" == "ready" ]]; then
    echo -e "\nOverall Status: ${GREEN}✅ READY${RESET}"
    echo -e "${GREEN}🎉 Choreo has been successfully installed and is ready to use! 🚀${RESET}"
else
    echo -e "\nOverall Status: ${RED}❌ NOT READY${RESET}"
    echo -e "${DARK_YELLOW}⚠ Some components are still initializing. Please wait a few minutes and try again. 🕑${RESET}"
fi
