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

### Setting Up the k3d Kubernetes Cluster

For testing and development, we recommend using k3d (Kubernetes in Docker). The k3d development environment provides a multi-node cluster (1 server + 2 agents) that closely mimics production workload distribution.

#### Prerequisites for k3d Setup

Before starting, ensure you have:
- Docker 20.10+
- k3d 5.8+
- kubectl 1.32+
- Helm 3.12+

#### Quick Start Development Workflow

For a complete setup in one command:

```sh
make k3d
```

This will create the cluster, build all components, load images, and install OpenChoreo. This typically takes 5-15 minutes depending on your internet bandwidth.

#### Step-by-Step Setup

1. Create k3d cluster:

   ```sh
   make k3d.up
   ```

2. Build all OpenChoreo components:

   ```sh
   make k3d.build
   ```

3. Load component images into the cluster:

   ```sh
   make k3d.load
   ```

4. Install OpenChoreo (Control Plane, Data Plane, Build Plane, Observability Plane):

   ```sh
   make k3d.install
   ```

> [!NOTE]
> This command installs all planes in the single k3d cluster. You can install specific planes with `make k3d.install.<plane-name>` where plane-name is control-plane, data-plane, build-plane, or observability-plane.

5. Configure the DataPlane resource:

   OpenChoreo requires a DataPlane resource to deploy and manage workloads.

   ```sh
   make k3d.configure
   ```

6. Verify the deployment:

   ```sh
   make k3d.status
   ```

   Or check individual plane pods:
   ```sh
   kubectl --context k3d-openchoreo-dev get pods -n openchoreo-control-plane
   kubectl --context k3d-openchoreo-dev get pods -n openchoreo-data-plane
   ```

7. Run controller manager locally (for development):

   To run the controller manager locally during development:

   - First, scale down the existing controller deployment:
   ```sh
   kubectl --context k3d-openchoreo-dev -n openchoreo-control-plane scale deployment openchoreo-controller-manager --replicas=0
   ```

   - Then, run the manager with webhooks disabled:
   ```sh
   make go.run.manager ENABLE_WEBHOOKS=false
   ```

> [!TIP]
> The main controller runs as a deployment in the cluster. For rapid development iteration, you can run it locally while keeping other components in the cluster.

### Component-Specific Operations

- Build specific component: `make k3d.build.<component>` (controller, openchoreo-api, observer)
- Load specific component: `make k3d.load.<component>` (controller, openchoreo-api, observer)
- Update specific component: `make k3d.update.<component>` (rebuild, load, and restart)
- Upgrade specific plane: `make k3d.upgrade.<plane>` (control-plane, data-plane, build-plane, observability-plane)
- View logs: `make k3d.logs.<component>` (controller, openchoreo-api, observer)

### Cleanup

To delete the k3d cluster:

```sh
make k3d.down
```

### Port Access

Once the cluster is running, you can access services via localhost:

- **Control Plane UI/API**: http://openchoreo.localhost:8080
- **Data Plane Workloads**: http://localhost:19080 (kgateway)
- **Build Plane**: Argo Workflows at http://localhost:10081, Registry at http://localhost:10082
- **Observability**: Observer API at http://localhost:11080, OpenSearch at http://localhost:11082

### Building and Running the Binaries

This project comprises multiple binaries, mainly the `manager` binary and the `choreoctl` CLI tool.
To build all the binaries, run:

```sh
make go.build
```

This will produce the binaries in the `bin/dist` directory based on your OS and architecture.
You can directly run the `manager` or `choreoctl` binary this location to try out.

### Incremental Development

Rather using build and run the binaries every time, you can use the go run make targets to run the binaries directly.

- Running the `manager` binary:
  ```sh
  make go.run.manager ENABLE_WEBHOOKS=false
  ```

- Running the `choreoctl` CLI tool:
  ```sh
  make go.run.choreoctl GO_RUN_ARGS="version"
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

### Submitting Changes

Once all changes are made and tested, you can submit a pull request by following the [GitHub workflow](github_workflow.md).

## Additional Resources

- [Add New CRD Guide](adding-new-crd.md) - A guide to add new CRDs to the project.
- [Adding New MCP Tools](adding-new-mcp-tools.md) - A guide to add new tools to the OpenChoreo MCP server.
