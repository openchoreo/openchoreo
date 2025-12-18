# K3d Development Environment

This directory contains configuration for running OpenChoreo in a k3d cluster for local development.

## Overview

The k3d dev setup provides:

- Multi-node cluster (1 server + 2 agents) for realistic workload distribution testing
- Local image builds with `k3d image import` (no registry needed)
- Separate values files for each plane (control, data, build, observability)
- Production image repository names with `latest-dev` tag

## Quick Start

```bash
# Complete setup: cluster + build + load + install
make k3d

# Check status
make k3d.status
```

## Image Loading Strategy

Images are built locally and imported directly into k3d cluster nodes:

1. Build with production repo names: `ghcr.io/openchoreo/<component>:latest-dev`
2. Import via `k3d image import` to all cluster nodes
3. Use `pullPolicy: Never` to ensure only local images are used

This avoids the need for a registry while using production-like image names.

## Make Targets

### Lifecycle

- `make k3d` - Complete setup
- `make k3d.up` - Create cluster
- `make k3d.down` - Delete cluster

### Build and Load

- `make k3d.build` - Build all components
- `make k3d.build.<component>` - Build specific component (controller, openchoreo-api, observer)
- `make k3d.load` - Load all images
- `make k3d.load.<component>` - Load specific image

### Install and Uninstall

- `make k3d.install` - Install all planes
- `make k3d.install.<plane>` - Install specific plane (control-plane, data-plane, build-plane, observability-plane)
- `make k3d.uninstall` - Uninstall all planes
- `make k3d.uninstall.<plane>` - Uninstall specific plane

### Updates

- `make k3d.update` - Rebuild, load, and restart all components
- `make k3d.update.<component>` - Update specific component
- `make k3d.upgrade` - Upgrade all helm charts
- `make k3d.upgrade.<plane>` - Upgrade specific plane

### Utilities

- `make k3d.configure` - Create DataPlane and BuildPlane resources
- `make k3d.status` - Show status of all planes
- `make k3d.logs.<component>` - Tail logs for component

## Port Mappings

| Plane | Port | Description |
|-------|------|-------------|
| Control | 8080 | HTTP (UI/API) |
| Control | 8443 | HTTPS |
| Data | 19080 | HTTP (kgateway) |
| Data | 19443 | HTTPS |
| Build | 10081 | Argo Workflows UI |
| Build | 10082 | Container Registry |
| Observability | 11080 | Observer API |
| Observability | 11081 | OpenSearch Dashboard |
| Observability | 11082 | OpenSearch API |

## Configuration Files

- `config.yaml` - K3d cluster configuration
- `values-cp.yaml` - Control Plane helm values
- `values-dp.yaml` - Data Plane helm values
- `values-bp.yaml` - Build Plane helm values
- `values-op.yaml` - Observability Plane helm values

## Development Workflow

### Initial Setup

```bash
make k3d
```

### Update a Component

After code changes:

```bash
# Update controller
make k3d.update.controller

# Update API
make k3d.update.openchoreo-api

# Update observer
make k3d.update.observer

# Update all
make k3d.update
```

### Update Helm Charts

After helm chart changes:

```bash
# Upgrade specific plane
make k3d.upgrade.control-plane

# Upgrade all
make k3d.upgrade
```

### Teardown

```bash
make k3d.down
```

## Troubleshooting

### Check Pod Status

```bash
make k3d.status
```

### View Logs

```bash
make k3d.logs.controller
make k3d.logs.openchoreo-api
make k3d.logs.observer
```

### Verify Images are Loaded

```bash
docker exec k3d-openchoreo-dev-server-0 crictl images | grep openchoreo
```

### Reset Cluster

```bash
make k3d.down
make k3d
```
