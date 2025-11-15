# OpenChoreo Quick-Start

A containerized environment to quickly try OpenChoreo without installing any tools on your machine. Everything you need
is pre-configured inside a Docker container.

## What's Included

- **Pre-installed Tools**: k3d, kubectl and helm
- **Interactive Environment**: Bash shell with helpful aliases and auto-completion
- **Sample Applications**: Ready-to-deploy examples to explore OpenChoreo capabilities
- **Installation Scripts**: Automated setup for local k3d cluster with OpenChoreo

## Prerequisites

- Docker

## Getting Started

### 1. Start the Quick-Start Container

Run the following command to start the containerized environment:

```bash
docker run -it --rm \
  --name openchoreo-quick-start \
  --network=host \
  -v /var/run/docker.sock:/var/run/docker.sock \
  ghcr.io/openchoreo/quick-start:latest
```

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

- `./install.sh --with-observability` - Install with OpenSearch and observability components
- `./install.sh --version v0.4.0` - Install a specific OpenChoreo version
- `./install.sh --version latest-dev --debug` - Install latest development build with verbose logging
- `./install.sh --skip-preload` - Skip image preloading from local Docker to k3d
- `./install.sh --debug` - Enable verbose logging
- `./install.sh --help` - Show all available options

The installer will:

- Create a local k3d Kubernetes cluster
- Install OpenChoreo Control Plane
- Install OpenChoreo Data Plane
- Configure the environment for sample deployments

### 3. Try a Sample Application

After installation completes, deploy a sample application to see OpenChoreo in action.

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

The deployment scripts will show you the application URLs when ready.

## Exploring Further

**Browse Sample Applications:**

```bash
ls samples/
```

Explore additional sample applications and deployment configurations.

## Cleaning Up

**Remove a Sample Application:**

```bash
./deploy-react-starter.sh --clean
./deploy-gcp-demo.sh --clean
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
