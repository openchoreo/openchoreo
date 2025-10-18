#!/usr/bin/env bash

# OpenChoreo Kind Setup with Working DNS
# Based on Cilium's kind.sh but simplified for general use

set -eo pipefail

# Get script directory and source helpers
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/dev-helpers.sh"

# Configuration
DEFAULT_EXTERNAL_DNS="8.8.8.8"
V6_PREFIX="fc00:f111::/64"

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
            log_error "Unknown option: $1"
            echo "Use --help for usage information"
            exit 1
            ;;
    esac
done

# Show help if requested
if [[ "${args[show_help]}" == "true" ]]; then
    echo "Usage: $0 [OPTIONS]"
    echo ""
    echo "Description:"
    echo "  Creates a Kind cluster with proper DNS configuration for OpenChoreo development"
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

# Network configuration
bridge_dev="br-${network}"

# Verify prerequisites
verify_prerequisites

# Check if cluster already exists
if cluster_exists "$cluster_name"; then
    log_warning "Kind cluster '${cluster_name}' already exists"
    read -p "Do you want to delete it and recreate? [y/N]: " -r
    if [[ $REPLY =~ ^[Yy]$ ]]; then
        log_info "Deleting existing cluster..."
        kind delete cluster --name "$cluster_name"
    else
        log_info "Using existing cluster"
        exit 0
    fi
fi

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

log_info "Creating Kind cluster '${cluster_name}'..."
kind --version

# Build kind command
kind_cmd="kind create cluster --name ${cluster_name}"

# Create cluster with custom configuration
log_info "Creating cluster with custom configuration..."
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
    log_success "Kind cluster '${cluster_name}' created successfully"
else
    log_error "Failed to create Kind cluster '${cluster_name}'"
    exit 1
fi

# Setup nodes (DNS is handled automatically via systemd drop-in during kubelet startup)
log_info "Setting up nodes..."
for node in $(kind get nodes --name "${cluster_name}"); do
    log_info "Configuring node: ${node}"

    # Set unprivileged port range
    if ! docker exec "${node}" sysctl -w net.ipv4.ip_unprivileged_port_start=1024; then
        log_warning "Failed to set unprivileged port range on node: ${node}"
    fi
done

# Patch CoreDNS to use external DNS
log_info "Patching CoreDNS to use external DNS: ${external_dns}"
NewCoreFile=$(kubectl get cm -n kube-system coredns -o jsonpath='{.data.Corefile}' | \
    sed "s,forward . /etc/resolv.conf,forward . ${external_dns}," | \
    sed 's/loadbalance/loadbalance\n    log/' | \
    awk ' { printf "%s\\n", $0 } ')

if kubectl patch configmap/coredns -n kube-system --type merge -p \
    '{"data":{"Corefile": "'"$NewCoreFile"'"}}'; then
    log_success "CoreDNS patched successfully"
else
    log_warning "Failed to patch CoreDNS"
fi

# Remove control-plane taints to allow pods on control-plane
log_info "Removing control-plane taints..."
set +e
kubectl taint nodes --all node-role.kubernetes.io/control-plane- 2>/dev/null
kubectl taint nodes --all node-role.kubernetes.io/master- 2>/dev/null
set -e

echo
log_success "Kind cluster '${cluster_name}' is ready!"
echo
log_info "Next steps:"
echo "  1. Install a CNI (e.g., ./kind-install-cilium.sh)"
echo "  2. Install OpenChoreo (e.g., ./kind-install-openchoreo.sh)"
echo
log_info "Cluster access:"
echo "  - kubeconfig: Automatically configured"
echo "  - context: kind-${cluster_name}"
