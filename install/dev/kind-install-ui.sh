#!/bin/bash

# This script builds the OpenChoreo UI and loads the Docker image into the kind cluster.
# usage: ./kind-install-ui.sh [cluster-name]

set -euo pipefail

# Configuration
default_cluster_name="openchoreo"
default_image_name="openchoreo-ui"
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

have_yarn() {
    [[ -n "$(command -v yarn)" ]]
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

if ! have_yarn; then
    echo "Please install yarn first:"
    echo "  https://yarnpkg.com/getting-started/install"
    exit 1
fi

# Check if kind cluster exists
if ! kind get clusters | grep -q "^${cluster_name}$"; then
    echo "Error: Kind cluster '${cluster_name}' does not exist."
    echo "Please create it first using: ./kind.sh ${cluster_name}"
    exit 1
fi

echo "Building OpenChoreo UI for development..."
cd "${PROJECT_ROOT}/ui"

# Install dependencies if needed
if [ ! -d "node_modules" ]; then
    echo "Installing UI dependencies..."
    yarn install --immutable
fi

# Build the backend
echo "Building backstage..."
yarn build:all

# Build the Docker image
echo "Building Docker image: ${full_image_name}"
docker build -f "${PROJECT_ROOT}/ui/packages/backend/Dockerfile" -t "${full_image_name}" "${PROJECT_ROOT}/ui"

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

echo "Successfully built and loaded OpenChoreo UI image: ${full_image_name}"
