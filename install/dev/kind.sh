#!/usr/bin/env bash

set -eo pipefail

# Configuration defaults
CLUSTER_NAME="${1:-openchoreo}"
NETWORK="${2:-openchoreo}"
EXTERNAL_DNS="${3:-8.8.8.8}"

# Get script directory for config files
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Node configuration function
node_config() {
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
    node_config
}

worker_config() {
    echo "- role: worker"
    node_config
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

echo "Creating Kind cluster '${CLUSTER_NAME}'..."

# Create cluster with custom configuration
if cat <<EOF | kind create cluster --name "${CLUSTER_NAME}" --config=-
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
EOF
then
    echo "Kind cluster '${CLUSTER_NAME}' created successfully"
else
    echo "Error: Failed to create Kind cluster '${CLUSTER_NAME}'" >&2
    exit 1
fi

# Setup nodes
echo "Setting up nodes..."
for node in $(kind get nodes --name "${CLUSTER_NAME}"); do
    echo "Configuring node: ${node}"
    
    # Set unprivileged port range
    if ! docker exec "${node}" sysctl -w net.ipv4.ip_unprivileged_port_start=1024; then
        echo "Warning: Failed to set unprivileged port range on node: ${node}" >&2
    fi
done

# Patch CoreDNS to use external DNS
echo "Patching CoreDNS to use external DNS: ${EXTERNAL_DNS}"
NewCoreFile=$(kubectl get cm -n kube-system coredns -o jsonpath='{.data.Corefile}' | \
    sed "s,forward . /etc/resolv.conf,forward . ${EXTERNAL_DNS}," | \
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

echo "Kind cluster '${CLUSTER_NAME}' is ready!"
