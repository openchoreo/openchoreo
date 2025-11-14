#!/usr/bin/env bash
# Shared configuration for OpenChoreo quick-start
# This file is sourced by both interactive shells (.bashrc) and installation scripts
# to maintain a single source of truth for configuration values

# Cluster configuration
CLUSTER_NAME="openchoreo-quick-start"

# Namespace definitions
CONTROL_PLANE_NS="openchoreo-control-plane"
DATA_PLANE_NS="openchoreo-data-plane"
BUILD_PLANE_NS="openchoreo-build-plane"
OBSERVABILITY_NS="openchoreo-observability-plane"

# Helm repository
HELM_REPO="oci://ghcr.io/openchoreo/helm-charts"
