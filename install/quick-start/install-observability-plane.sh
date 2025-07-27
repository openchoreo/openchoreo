#!/usr/bin/env bash
set -eo pipefail

echo "Installing OpenChoreo Observability Plane..."

# Check if terraform state exists
if [ ! -f "terraform/terraform.tfstate" ]; then
  echo "Error: Terraform state not found. Please run install.sh first to set up the base infrastructure."
  exit 1
fi

# Apply terraform with observability plane enabled
terraform -chdir=terraform apply -auto-approve -var="enable-observability-plane=true"

echo "OpenChoreo Observability Plane installation completed!"
