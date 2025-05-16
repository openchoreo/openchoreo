#!/bin/bash

# Exit immediately if a command exits with a non-zero status.
set -e

SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )"

# Function to install a Helm chart and wait until it's ready
install_helm_chart() {
  local chart_dir=$1
  local release_name=$2
  local namespace=$3
  local extra_args=$4

  echo "Installing Helm chart: $release_name from $chart_dir..."
  helm dependency update "$chart_dir"
  helm upgrade --install "$release_name" "$chart_dir" \
    --namespace "$namespace" \
    --create-namespace \
    --timeout 30m \
    $extra_args
}

# Install Cilium
install_helm_chart "$SCRIPT_DIR/helm/cilium" "cilium" "choreo-system"

# Install Choreo Control Plane
install_helm_chart "$SCRIPT_DIR/helm/choreo-control-plane" "choreo-cp" "choreo-system"

# Install Choreo Data Plane (disable Cert Manager since it's already installed by the control plane)
install_helm_chart "$SCRIPT_DIR/helm/choreo-dataplane" "choreo-dp" "choreo-system" \
  "--set certmanager.enabled=false --set certmanager.crds.enabled=false"

echo "Helm charts have been installed successfully!"
echo "Please note: the full installation process may take several minutes to complete."
