#!/bin/bash

# This script builds the OpenChoreo controller and loads the Docker image into the kind cluster.
# usage: ./kind-install-controller.sh [cluster-name]

set -euo pipefail

# Configuration
default_cluster_name="openchoreo"
default_image_name="openchoreo-controller"
default_image_tag="dev"

cluster_name="${1:-${CLUSTER_NAME:=${default_cluster_name}}}"
image_name="${IMAGE_NAME:-${default_image_name}}"
image_tag="${IMAGE_TAG:-${default_image_tag}}"
full_image_name="${image_name}:${image_tag}"

# Get script directory
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"

# Check dependencies
have_docker() {
    [[ -n "$(command -v docker)" ]]
}

have_kind() {
    [[ -n "$(command -v kind)" ]]
}

have_go() {
    [[ -n "$(command -v go)" ]]
}

if ! have_docker; then
    echo "Please install docker first:"
    echo "  https://docs.docker.com/get-docker/"
    exit 1
fi

if ! have_kind; then
    echo "Please install kind first:"
    echo "  https://kind.sigs.k8s.io/docs/user/quick-start/#installation"
    exit 1
fi

if ! have_go; then
    echo "Please install go first:"
    echo "  https://go.dev/doc/install"
    exit 1
fi

# Check if kind cluster exists
if ! kind get clusters | grep -q "^${cluster_name}$"; then
    echo "Error: Kind cluster '${cluster_name}' does not exist."
    echo "Please create it first using: ./kind.sh ${cluster_name}"
    exit 1
fi

echo "Building OpenChoreo controller for development..."
cd "${PROJECT_ROOT}"

# Build the Go binary for the current platform
echo "Building manager binary..."
make go.build-multiarch.manager

# Build the Docker image
echo "Building Docker image: ${full_image_name}"
docker build -f "${PROJECT_ROOT}/Dockerfile" -t "${full_image_name}" "${PROJECT_ROOT}"

if [ $? -ne 0 ]; then
    echo "Failed to build Docker image: ${full_image_name}"
    exit 1
fi

# Load the image into the kind cluster
echo "Loading Docker image into kind cluster: ${cluster_name}"
kind load docker-image "${full_image_name}" --name "${cluster_name}"

if [ $? -ne 0 ]; then
    echo "Failed to load image into kind cluster: ${cluster_name}"
    exit 1
fi

echo "Successfully built and loaded OpenChoreo controller image: ${full_image_name}"