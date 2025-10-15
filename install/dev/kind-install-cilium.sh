#!/bin/bash

# This pulls the Cilium images to the local docker context (if not exists), loads them into the kind cluster,
# and installs Cilium via Helm (version 1.18.2).
# usage: ./kind-install-cilium.sh [cluster-name]

set -euo pipefail

# Configuration
default_cluster_name="openchoreo"

cluster_name="${1:-${CLUSTER_NAME:=${default_cluster_name}}}"

# Check dependencies
have_helm() {
    [[ -n "$(command -v helm)" ]]
}

if ! have_helm; then
    echo "Please install helm first:"
    echo "  https://helm.sh/docs/intro/install/"
    exit 1
fi

images=(
  "quay.io/cilium/operator-generic:v1.18.2"
  "quay.io/cilium/cilium:v1.18.2"
  "quay.io/cilium/cilium-envoy:v1.34.4-1754895458-68cffdfa568b6b226d70a7ef81fc65dda3b890bf"
)

for image in "${images[@]}"; do
  if ! docker image inspect "$image" > /dev/null 2>&1; then
    echo "Image not found locally. Pulling image: $image"
    docker pull "$image"
    if [ $? -ne 0 ]; then
      echo "Failed to pull image: $image"
      exit 1
    fi
  else
    echo "Image already exists locally: $image"
  fi

  echo "Loading image: $image into kind cluster: $cluster_name"
  kind load docker-image "$image" --name "$cluster_name"
  if [ $? -ne 0 ]; then
    echo "Failed to load image: $image"
    exit 1
  fi
done

echo "All images have been successfully processed and loaded into the kind cluster: $cluster_name."

# Add Cilium Helm repository
echo "Adding Cilium Helm repository..."
helm repo add cilium https://helm.cilium.io/
helm repo update

# Get the Kubernetes API server IP from the kind cluster
echo "Getting Kubernetes API server IP for cluster: $cluster_name"
K8S_API_IP=$(docker inspect "${cluster_name}-control-plane" --format='{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}')

if [ -z "$K8S_API_IP" ]; then
  echo "Error: Failed to get Kubernetes API server IP for cluster: $cluster_name"
  exit 1
fi

echo "Kubernetes API server IP: $K8S_API_IP"

# Install Cilium via Helm with dynamic API server IP
echo "Installing Cilium via Helm..."
helm upgrade --install cilium cilium/cilium \
  --version 1.18.2 \
  --namespace cilium \
  --values cilium-values.yaml \
  --set k8sServiceHost="$K8S_API_IP" \
  --kube-context kind-"$cluster_name" \
  --create-namespace

if [ $? -eq 0 ]; then
  echo "Cilium has been successfully installed in the kind cluster: $cluster_name"
else
  echo "Failed to install Cilium"
  exit 1
fi
