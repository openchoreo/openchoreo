#!/usr/bin/env bash

# OpenChoreo Kind Setup - Minimal script for cluster creation only
# This script only handles cluster creation with node configuration.
# All other setup (prerequisites, DNS patching, etc.) is handled by make/kind.mk

set -eo pipefail

# Configuration defaults
DEFAULT_CLUSTER_NAME="openchoreo"
DEFAULT_NETWORK="openchoreo"
DEFAULT_EXTERNAL_DNS="8.8.8.8"

# Parse command line arguments
declare -A args=(
    [cluster_name]="$DEFAULT_CLUSTER_NAME"
    [network]="$DEFAULT_NETWORK"
    [external_dns]="$DEFAULT_EXTERNAL_DNS"
    [show_help]=false
)

while [[ $# -gt 0 ]]; do
    case $1 in
        --cluster-name)
            args[cluster_name]="$2"
            shift 2
            ;;
        --network)
            args[network]="$2"
            shift 2
            ;;
        --external-dns)
            args[external_dns]="$2"
            shift 2
            ;;
        --help|-h)
            args[show_help]=true
            shift
            ;;
        *)
            echo "Error: Unknown option: $1" >&2
            echo "Use --help for usage information" >&2
            exit 1
            ;;
    esac
done

# Show help if requested
if [[ "${args[show_help]}" == "true" ]]; then
    echo "Usage: $0 [OPTIONS]"
    echo ""
    echo "Description:"
    echo "  Creates a Kind cluster with full setup including DNS patching and node configuration"
    echo ""
    echo "Options:"
    echo "  --cluster-name NAME    Cluster name (default: $DEFAULT_CLUSTER_NAME)"
    echo "  --network NAME         Network name (default: $DEFAULT_NETWORK)"
    echo "  --external-dns IP      External DNS server (default: $DEFAULT_EXTERNAL_DNS)"
    echo "  --help, -h             Show this help message"
    echo ""
    echo "Examples:"
    echo "  $0                                    # Use defaults"
    echo "  $0 --cluster-name my-cluster         # Custom cluster name"
    echo "  $0 --external-dns 1.1.1.1            # Use Cloudflare DNS"
    exit 0
fi

cluster_name="${args[cluster_name]}"
network="${args[network]}"
external_dns="${args[external_dns]}"

# Get script directory for config files
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Node configuration function
node_config() {
    local node_type="$1"
    local node_num="$2"

    echo "  extraMounts:"
    # Mount systemd drop-in config for DNS setup
    echo "  - hostPath: ${SCRIPT_DIR}/kind-kubelet.conf"
    echo "    containerPath: /etc/systemd/system/kubelet.service.d/12-dns.conf"
    echo "    readOnly: true"
    # Mount DNS setup script
    echo "  - hostPath: ${SCRIPT_DIR}/kind-dns-setup.sh"
    echo "    containerPath: /tmp/dns-setup.sh"
    echo "    readOnly: true"
}

# Generate node configurations
control_plane_config() {
    echo "- role: control-plane"
    node_config "control-plane" "1"
}

worker_config() {
    echo "- role: worker"
    node_config "worker" "1"
    echo "  extraPortMappings:"
    echo "  - containerPort: 32000"
    echo "    hostPort: 80"
    echo "    listenAddress: \"127.0.0.1\""
    echo "    protocol: TCP"
    echo "  - containerPort: 32001"
    echo "    hostPort: 443"
    echo "    listenAddress: \"127.0.0.1\""
    echo "    protocol: TCP"
}

echo "Creating Kind cluster '${cluster_name}'..."
kind --version

# Build kind command
kind_cmd="kind create cluster --name ${cluster_name}"

# Create cluster with custom configuration
echo "Creating cluster with custom configuration..."
if cat <<EOF | ${kind_cmd} --config=-
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
nodes:
$(control_plane_config)
$(worker_config)
networking:
  disableDefaultCNI: true
  kubeProxyMode: none
  ipFamily: dual
  apiServerAddress: 127.0.0.1
  apiServerPort: 6443

kubeadmConfigPatches:
  - |
    kind: ClusterConfiguration
    metadata:
      name: config
    apiServer:
      extraArgs:
        "v": "3"
    controllerManager:
      extraArgs:
        authorization-always-allow-paths: /healthz,/readyz,/livez,/metrics
        bind-address: 0.0.0.0
    scheduler:
      extraArgs:
        authorization-always-allow-paths: /healthz,/readyz,/livez,/metrics
        bind-address: 0.0.0.0
EOF
then
    echo "Kind cluster '${cluster_name}' created successfully"
else
    echo "Error: Failed to create Kind cluster '${cluster_name}'" >&2
    exit 1
fi

# Setup nodes (DNS is handled automatically via systemd drop-in during kubelet startup)
echo "Setting up nodes..."
for node in $(kind get nodes --name "${cluster_name}"); do
    echo "Configuring node: ${node}"

    # Set unprivileged port range
    if ! docker exec "${node}" sysctl -w net.ipv4.ip_unprivileged_port_start=1024; then
        echo "Warning: Failed to set unprivileged port range on node: ${node}" >&2
    fi
done

# Patch CoreDNS to use external DNS
echo "Patching CoreDNS to use external DNS: ${external_dns}"
NewCoreFile=$(kubectl get cm -n kube-system coredns -o jsonpath='{.data.Corefile}' | \
    sed "s,forward . /etc/resolv.conf,forward . ${external_dns}," | \
    sed 's/loadbalance/loadbalance\n    log/' | \
    awk ' { printf "%s\\n", $0 } ')

if kubectl patch configmap/coredns -n kube-system --type merge -p \
    '{"data":{"Corefile": "'"$NewCoreFile"'"}}'; then
    echo "CoreDNS patched successfully"
else
    echo "Warning: Failed to patch CoreDNS" >&2
fi

# Remove control-plane taints to allow pods on control-plane
echo "Removing control-plane taints..."
set +e
kubectl taint nodes --all node-role.kubernetes.io/control-plane- 2>/dev/null
kubectl taint nodes --all node-role.kubernetes.io/master- 2>/dev/null
set -e

echo
echo "Kind cluster '${cluster_name}' is ready!"
echo
echo "Next steps:"
echo "  1. Install a CNI (e.g., ./kind-install-cilium.sh)"
echo "  2. Install OpenChoreo (e.g., make kind.setup)"
echo
echo "Cluster access:"
echo "  - kubeconfig: Automatically configured"
echo "  - context: kind-${cluster_name}"
