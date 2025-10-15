#!/usr/bin/env bash

# OpenChoreo Kind Setup with Working DNS
# Based on Cilium's kind.sh but simplified for general use

set -euo pipefail

# Configuration
default_cluster_name="openchoreo"
default_network="openchoreo"

cluster_name="${1:-${CLUSTER_NAME:=${default_cluster_name}}}"

# Network configuration
bridge_dev="br-${default_network}"
v6_prefix="fc00:f111::/64"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Check dependencies
have_kind() {
    [[ -n "$(command -v kind)" ]]
}

have_kubectl() {
    [[ -n "$(command -v kubectl)" ]]
}

if ! have_kind; then
    echo "Please install kind first:"
    echo "  https://kind.sigs.k8s.io/docs/user/quick-start/#installation"
    exit 1
fi

if ! have_kubectl; then
    echo "Please install kubectl first:"
    echo "  https://kubernetes.io/docs/tasks/tools/#kubectl"
    exit 1
fi

# Build kind command
kind_cmd="kind create cluster --name ${cluster_name}"

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
}

echo "${kind_cmd}"
kind --version

# Skip custom network creation due to Docker Desktop compatibility issues
# Using default kind networking instead
# export KIND_EXPERIMENTAL_DOCKER_NETWORK="${default_network}"

# Create cluster with custom configuration
cat <<EOF | ${kind_cmd} --config=-
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

# Setup nodes (DNS is handled automatically via systemd drop-in during kubelet startup)
echo "Setting up nodes..."
for node in $(kind get nodes --name ${cluster_name}); do
    echo "Configuring node: ${node}"

    # Set unprivileged port range
    docker exec ${node} sysctl -w net.ipv4.ip_unprivileged_port_start=1024
done

# Patch CoreDNS to use external DNS
echo "Patching CoreDNS..."
external_dns="8.8.8.8"
NewCoreFile=$(kubectl get cm -n kube-system coredns -o jsonpath='{.data.Corefile}' | \
    sed "s,forward . /etc/resolv.conf,forward . ${external_dns}," | \
    sed 's/loadbalance/loadbalance\n    log/' | \
    awk ' { printf "%s\\n", $0 } ')

kubectl patch configmap/coredns -n kube-system --type merge -p \
    '{"data":{"Corefile": "'"$NewCoreFile"'"}}'

# Remove control-plane taints to allow pods on control-plane
set +e
kubectl taint nodes --all node-role.kubernetes.io/control-plane- 2>/dev/null
kubectl taint nodes --all node-role.kubernetes.io/master- 2>/dev/null
set -e

echo
echo "Kind cluster '${cluster_name}' is ready!"
echo
echo "You can now install your CNI or use the cluster as-is."
