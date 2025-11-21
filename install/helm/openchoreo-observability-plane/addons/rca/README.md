# OpenChoreo RCA Agent

Root Cause Analysis addon for OpenChoreo Observability Plane.

## Installation

From the root of the repository:

```bash
helm install rca-agent install/helm/openchoreo-observability-plane/addons/rca \
  --namespace openchoreo-observability-plane
```

Make sure the namespace is the namespace where your OpenChoreo Observability Plane is installed

## Prerequisites

- OpenChoreo Observability Plane must be installed in the target namespace
- Observer service must be running

## Configuration

The RCA agent will be deployed with default settings. To customize, create a `values.yaml` file and pass it during installation:

```bash
helm install rca-agent install/helm/openchoreo-observability-plane/addons/rca \
  --namespace <observability-namespace> \
  --values custom-values.yaml
```

See `values.yaml` for available configuration options.
