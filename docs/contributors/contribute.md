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

### Setting Up the KinD Kubernetes Cluster

For testing and development, we recommend using a KinD (Kubernetes in Docker) cluster.

1. Run the following command to create a KinD cluster:

   ```sh
   make kind.up
   ```

2. Build all OpenChoreo components:

   ```sh
   make kind.build
   ```

3. Load component images into the cluster:

   ```sh
   make kind.load
   ```

4. Install Cilium CNI and OpenChoreo:

   ```sh
   make kind.install
   ```

   This may take around 5-15 minutes to complete depending on the internet bandwidth.

> [!NOTE]
> This command installs both the control plane and data plane components in the same cluster.

5. Once completed, you can verify the deployment by running:

   ```sh
   ./install/check-status.sh
   ```

6. Add default DataPlane to the cluster:

    OpenChoreo requires a DataPlane to deploy and manage its resources.

   ```sh
   bash ./install/add-default-dataplane.sh
   ```

7. Run controller manager locally (for development):
   
   To run the controller manager locally during development:

   - First, scale down the existing controller deployment:
   ```sh
   kubectl -n openchoreo scale deployment openchoreo-controller-manager --replicas=0
   ```
   
   - Then, run the following command to configure DataPlane resource:
   ```sh
   kubectl get dataplane default-dataplane -n default-org -o json | jq --arg url "$(kubectl config view --raw -o jsonpath="{.clusters[?(@.name=='kind-openchoreo')].cluster.server}")" '.spec.kubernetesCluster.credentials.apiServerURL = $url' | kubectl apply -f -
   ```

Note: The main controller runs as a deployment in the cluster. For local development, you typically work with the API server and CLI tools.

### Quick Start Development Workflow

For a complete setup in one command:

```sh
make kind
```

This will create the cluster, build all components, load images, and install OpenChoreo.

### Component-Specific Operations

- Build specific component: `make kind.build.<component>` (controller, api, ui)
- Load specific component: `make kind.load.<component>` (controller, api, ui, cilium)
- Install specific component: `make kind.install.<component>` (cilium, openchoreo)
- Update specific component: `make kind.update.<component>` (controller, api, ui)

### Cleanup

To delete the KinD cluster:

```sh
make kind.down
```

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
