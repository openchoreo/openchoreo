# helm-gen

A tool for generating Helm template files from Kubebuilder-generated manifests.

## Usage

### Command Line

```bash
# Basic usage
helm-gen -config-dir ./config -chart-dir ./install/helm/openchoreo-control-plane

# With custom controller subdirectory
helm-gen -config-dir ./config -chart-dir ./install/helm/openchoreo-control-plane -controller-subdir controller-manager
```

### Makefile Integration

The tool is integrated into the project's build system:

```bash
# Generate helm chart
make helm-generate.openchoreo-control-plane

# The makefile runs:
# 1. make manifests (to generate CRDs and RBAC)
# 2. helm-gen (to copy CRDs and generate RBAC)
# 3. Updates values.yaml with controller image settings
# 4. Runs helm dependency update and lint
```

## What It Generates

### 1. CRDs (Custom Resource Definitions)

- **Source**: `config/crd/bases/*.yaml`
- **Destination**: `<chart-dir>/crds/`

### 2. RBAC (Role-Based Access Control)

- **Source**: `config/rbac/role.yaml`
- **Destination**: `<chart-dir>/templates/controller-manager/controller-manager-role.yaml`

