#!/usr/bin/env bash

# OpenChoreo Kind Cluster Teardown
# Companion script to kind.sh for clean cluster deletion

set -euo pipefail

# Configuration
default_cluster_name="openchoreo"

cluster_name="${1:-${CLUSTER_NAME:=${default_cluster_name}}}"

# Check dependencies
have_kind() {
    [[ -n "$(command -v kind)" ]]
}

if ! have_kind; then
    echo "Please install kind first:"
    echo "  https://kind.sigs.k8s.io/docs/user/quick-start/#installation"
    exit 1
fi

echo "Deleting kind cluster '${cluster_name}'..."
kind delete cluster --name "${cluster_name}"

echo "Kind cluster '${cluster_name}' has been deleted."