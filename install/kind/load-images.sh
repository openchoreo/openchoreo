#!/bin/bash

# This pulls the following images to the local docker context (if not exists) and load them into the kind cluster.
# This can be used to speedup the installation.
# usage: ./load-images.sh --kind-cluster-name <kind-cluster-name>

while [[ "$#" -gt 0 ]]; do
  case $1 in
    --kind-cluster-name)
      KIND_CLUSTER_NAME="$2"
      shift
      shift
      ;;
    *)
      echo "Unknown option: $1"
      exit 1
      ;;
  esac
done

if [ -z "$KIND_CLUSTER_NAME" ]; then
  echo "Error: --kind-cluster-name is required."
  exit 1
fi

images=(
  "ghcr.io/openchoreo/controller:latest"
  "quay.io/cilium/operator-generic:v1.15.10"
  "quay.io/cilium/cilium:v1.15.10"
  "quay.io/argoproj/argocli:v3.6.2"
  "quay.io/argoproj/workflow-controller:v3.6.2"
  "docker.io/hashicorp/vault:1.18.1"
  "docker.io/hashicorp/vault-k8s:1.5.0"
  "docker.io/library/registry:2"
  "docker.io/envoyproxy/gateway:v1.2.3"
  "docker.io/library/redis:6.0.6"
  "docker.io/envoyproxy/envoy:distroless-v1.32.1"
  "quay.io/jetstack/cert-manager-controller:v1.16.2"
  "quay.io/jetstack/cert-manager-cainjector:v1.16.2"
  "quay.io/jetstack/cert-manager-webhook:v1.16.2"
  "docker.io/bitnami/kubectl:latest"
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

  echo "Loading image: $image into kind cluster: $KIND_CLUSTER_NAME"
  kind load docker-image "$image" --name "$KIND_CLUSTER_NAME"
  if [ $? -ne 0 ]; then
    echo "Failed to load image: $image"
    exit 1
  fi
done

echo "All images have been successfully processed and loaded into the kind cluster: $KIND_CLUSTER_NAME."
