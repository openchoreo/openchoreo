#!/bin/bash
set -x

# Detect architecture
ARCH=$(uname -m)
case $ARCH in
  x86_64)
    ARCH_SUFFIX="amd64"
    ;;
  aarch64|arm64)
    ARCH_SUFFIX="arm64"
    ;;
  *)
    echo "Unsupported architecture: $ARCH"
    exit 1
    ;;
esac

curl -Lo ./k3d "https://github.com/k3d-io/k3d/releases/download/v5.8.0/k3d-linux-${ARCH_SUFFIX}"
chmod +x ./k3d
mv ./k3d /usr/local/bin/k3d

curl -L -o kubebuilder "https://go.kubebuilder.io/dl/latest/linux/${ARCH_SUFFIX}"
chmod +x kubebuilder
mv kubebuilder /usr/local/bin/

KUBECTL_VERSION=$(curl -L -s https://dl.k8s.io/release/stable.txt)
curl -LO "https://dl.k8s.io/release/$KUBECTL_VERSION/bin/linux/${ARCH_SUFFIX}/kubectl"
chmod +x kubectl
mv kubectl /usr/local/bin/kubectl

k3d version
kubebuilder version
docker --version
go version
kubectl version --client
