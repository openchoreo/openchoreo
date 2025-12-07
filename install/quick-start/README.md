# OpenChoreo Quick-Start

A containerized environment to quickly try OpenChoreo without installing any tools on your machine. Everything you need
is pre-configured inside a Docker container.

## What's Included

- **Pre-installed Tools**: k3d, kubectl and helm
- **Interactive Environment**: Bash shell with helpful aliases and auto-completion
- **Sample Applications**: Ready-to-deploy examples to explore OpenChoreo capabilities
- **Installation Scripts**: Automated setup for local k3d cluster with OpenChoreo
- **Optional Build Plane**: Argo Workflows + Container Registry for building from source

## Prerequisites

- Docker

## Getting Started

### 1. Start the Quick-Start Container

Run the following command to start the containerized environment:

```bash
docker run -it --rm \
  --name openchoreo-quick-start \
  --privileged \
  --network=host \
  -v /var/run/docker.sock:/var/run/docker.sock \
  ghcr.io/openchoreo/quick-start:latest
```

**Important:** The `--privileged` flag is required for k3d to run properly inside the container (Docker-in-Docker with containerd). This is especially necessary when using Colima or other container runtimes with cgroup v2.

**Available Tags:**

- `latest` - Latest stable release
- `latest-dev` - Latest development build
- `v0.4.0` - Specific release version

You'll see a welcome message with quick commands to get started.

### 2. Install OpenChoreo

Inside the container, run the installation script:

```bash
./install.sh
```

Installation typically takes 5-10 minutes depending on your network speed.

**Installation Options:**

- `./install.sh` - Install with Control Plane and Data Plane (minimal setup)
- `./install.sh --with-build` - Install with Build Plane (Argo Workflows + Container Registry)
- `./install.sh --with-observability` - Install with OpenSearch and observability components
- `./install.sh --with-build --with-observability` - Install full platform with all optional components
- `./install.sh --version v0.4.0` - Install a specific OpenChoreo version
- `./install.sh --version latest-dev --debug` - Install latest development build with verbose logging
- `./install.sh --skip-preload` - Skip image preloading from local Docker to k3d
- `./install.sh --debug` - Enable verbose logging
- `./install.sh --help` - Show all available options

The installer will:

- Create a local k3d Kubernetes cluster
- Install OpenChoreo Control Plane
- Install OpenChoreo Data Plane
- Optionally install Build Plane (for building from source)
- Optionally install Observability Plane (for monitoring and logs)
- Configure the environment for sample deployments

### 3. Try a Sample Application

After installation completes, deploy a sample application to see OpenChoreo in action.

#### Deploy from Pre-built Images

**Simple React Web Application:**

```bash
./deploy-react-starter.sh
```

A lightweight single-component application to quickly verify your setup.

**GCP Microservices Demo (11 Services):**

```bash
./deploy-gcp-demo.sh
```

A complex microservices application demonstrating multi-component deployments with service-to-service communication.

#### Build and Deploy from Source (Requires --with-build)

**Go Greeter Service:**

```bash
./build-deploy-greeter.sh
```

Demonstrates the complete build-to-deploy workflow:
- Clones source code from GitHub
- Builds a Docker image using the Build Plane
- Pushes the image to the container registry
- Deploys the service to the Data Plane

The deployment scripts will show you the application URLs when ready.

## Exploring Further

**Browse Sample Applications:**

```bash
ls samples/
```

Explore additional sample applications and deployment configurations.

## Troubleshooting

### k3d Cluster Creation Stuck

If the k3d cluster creation gets stuck at "Injecting records for hostAliases..." or "Waiting for containerd startup", this usually means:

**Solution:** Ensure you started the container with the `--privileged` flag:

```bash
docker run -it --rm \
  --name openchoreo-quick-start \
  --privileged \
  --network=host \
  -v /var/run/docker.sock:/var/run/docker.sock \
  ghcr.io/openchoreo/quick-start:latest
```

If the cluster is already broken, delete it and recreate:

```bash
k3d cluster delete openchoreo-quick-start
./install.sh
```

### Cluster Agent Pod Stuck in Pending

In single-cluster setups, the cluster agent should start automatically after Helm completes the installation. The Helm post-install jobs automatically:
- Copy the cluster-gateway CA from control plane to data/build plane namespaces
- Create TLS certificates using cert-manager

If the agent pod is stuck, check the Helm job logs:

```bash
kubectl get jobs -n openchoreo-data-plane
kubectl logs job/<job-name> -n openchoreo-data-plane
```

## Cleaning Up

**Remove a Sample Application:**

```bash
./deploy-react-starter.sh --clean
./deploy-gcp-demo.sh --clean
./build-deploy-greeter.sh --clean
```

**Uninstall OpenChoreo:**

```bash
./uninstall.sh
```

This removes the k3d cluster and all OpenChoreo components.

**Exit the Container:**

```bash
exit
```

The container will be automatically removed (thanks to the `--rm` flag).
