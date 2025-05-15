#!/bin/bash

# Exit immediately if a command exits with a non-zero status.
set -e

# Function to uninstall a Helm release and clean up its namespace
uninstall_helm_release() {
  local release_name=$1
  local namespace=$2

  echo "Uninstalling Helm release: $release_name from namespace: $namespace..."
  helm uninstall "$release_name" --namespace "$namespace" || {
    echo "Failed to uninstall $release_name. It might not exist. Skipping..."
  }
}

uninstall_helm_release "choreo-cp" "choreo-system"

uninstall_helm_release "choreo-dp" "choreo-system"

uninstall_helm_release "cilium" "choreo-system"

echo "All Helm releases have been uninstalled successfully!"

# Clean up the choreo-system namespace
kubectl delete namespace choreo-system

echo "Namespace choreo-system has been deleted successfully!"
