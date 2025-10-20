# Contributing to OpenChoreo Development

## Prerequisites

- Go version v1.24.0+
- Docker version 23.0+
- Make version 3.81+
- Kubernetes cluster with version v1.30.0+
- Kubectl version v1.30.0+
- Helm version v3.16.0+


To verify the tool versions, run the following command:
   ```sh
   ./check-tools.sh
   ```

## Getting Started

The OpenChoreo project is built using the [Kubebuilder](https://book.kubebuilder.io/) framework and uses Make for build automation.
After cloning the repository following the [github_workflow.md](github_workflow.md), run the following command to see all the available make targets:

```sh
make help
```

### Quick Start

For the fastest way to get started, use the Makefile-driven workflow that sets up everything you need:

```sh
make kind.setup
```

This single command will:
1. Create a Kind cluster with optimized configuration
2. Install Cilium CNI for advanced networking
3. Build all OpenChoreo components (controller, API, UI)
4. Load images into the Kind cluster
5. Install OpenChoreo via the Helm chart

### Manual Setup (Advanced)

If you prefer to set up components manually or need more control over the installation:

#### Setting Up the Kind Kubernetes Cluster

To set up a local Kubernetes cluster using Kind, run the following command:

```sh
make kind
```

This will create a Kubernetes cluster with the configuration specified in `install/kind/kind-config.yaml`.

#### Installing Cilium CNI

After creating the cluster, install Cilium for advanced networking capabilities:

```sh
make kind.install.cilium
```

This will install Cilium CNI as per `install/dev/cilium-values.yaml`

#### Building and Loading Docker Images

Build the Docker images for OpenChoreo components and load them into the Kind cluster:

```sh
# Build all images
make docker.build

# Build and load all OpenChoreo components into Kind cluster
make kind.build.openchoreo
```

#### Installing OpenChoreo

Install OpenChoreo using the Helm chart:

```sh
make kind.install.openchoreo
```


### Accessing OpenChoreo Services

After installation, you can access the services:

```sh
# Port-forward the API server to localhost:8080
make kind.access.api

# Port-forward the Backstage UI to localhost:7007
make kind.access.ui
```

### Adding a DataPlane

OpenChoreo requires a DataPlane to deploy and manage its resources. For development, you can add the default DataPlane:

```sh
bash ./install/add-default-dataplane.sh
```

### Running Controller Manager Locally

For development, you can run the controller manager locally:

1. Scale down the deployed controller manager:
   ```sh
   kubectl -n openchoreo scale deployment openchoreo-controller-manager --replicas=0
   ```

2. Run the controller manager locally:
   ```sh
   make go.run.manager ENABLE_WEBHOOKS=false
   ```

### Development Workflow Management

The new Makefile targets provide a complete development workflow:

```sh
# Check cluster status
make kind.status

# Clean up OpenChoreo installation
make kind.down.openchoreo

# Clean up entire cluster
make kind.down

# Get help with Kind-specific commands
make kind.help
```

### Building and Running the Binaries

This project comprises multiple binaries:
- `manager` - The Kubernetes controller manager
- `choreoctl` - The CLI tool for managing OpenChoreo resources
- `openchoreo-api` - The REST API server
- `observer` - The observability plane service

To build all the binaries, run:

```sh
make go.build
```

This will produce the binaries in the `bin/dist` directory based on your OS and architecture.

### Incremental Development

Rather than building and running the binaries every time, you can use the go run make targets to run the binaries directly.

- Running the `manager` binary:
  ```sh
  make go.run.manager ENABLE_WEBHOOKS=false
  ```

- Running the `choreoctl` CLI tool:
  ```sh
  make go.run.choreoctl GO_RUN_ARGS="version"
  ```

- Running the `openchoreo-api` server:
  ```sh
  make go.run.openchoreo-api
  ```

- Running the `observer` service:
  ```sh
  make go.run.observer
  ```

### Building for Kind Development

For development with Kind clusters, you can build and load all components:

```sh
# Build all components and load into Kind cluster
make kind.build.openchoreo

# Build individual components
make go.build.multiarch.manager
make go.build.multiarch.openchoreo-api
```
  
### Testing

To run the tests, you can use the following command:

```sh
make test
```
This will run all the unit tests in the project.

### Code Quality and Generation

Before submitting your changes, please ensure that your code is properly linted and any generated code is up-to-date.

#### Linting

Run the following command to check for linting issues:

```bash
make lint
```

To automatically fix common linting issues, use:

```bash
make lint-fix
```

#### Code Generation
After linting, verify that all generated code is up-to-date by running:

```bash
make code.gen-check
```

If there are discrepancies or missing generated files, fix them by running:

```bash
make code.gen
```

#### Helm Chart Generation
After modifying CRDs or RBAC rules, you need to regenerate the Helm charts:

```bash
make helm-generate
```

The OpenChoreo chart (`install/helm/openchoreo-secure-core/`) includes all components and is used for installations.

### Development Environment Reset

If you need to reset your development environment, you can clean up and restart:

```bash
# Delete the Kind cluster
make kind.down

# Clean up Docker images and containers (optional)
docker system prune -f

# Restart the setup process
make kind.setup
```

### Submitting Changes

Once all changes are made and tested, you can submit a pull request by following the [GitHub workflow](github_workflow.md).

## Additional Resources

- [Add New CRD Guide](adding-new-crd.md) - A guide to add new CRDs to the project.
- [Build Engines Guide](build-engines.md) - Understanding build engines and build planes.
- [Configure Build Plane](../configure-build-plane.md) - Setting up build planes for CI/CD.
- [Install Guide - Multi-Cluster](../install-guide-multi-cluster.md) - Multi-cluster deployment guide.
- [Observability and Logging](../observability-logging.md) - Setting up observability components.
- [Resource Kind Reference](../resource-kind-reference-guide.md) - Complete reference for OpenChoreo resource kinds.
