# k3d Setup for OpenChoreo

Local development setups for OpenChoreo using k3d (k3s in Docker).

## Prerequisites

- Docker 20.10+
- k3d 5.8+
- kubectl 1.32+
- Helm 3.12+

## Setup Options

### Single-Cluster Setup

All OpenChoreo planes run in one k3d cluster.

[See single-cluster/README.md](./single-cluster/README.md)

### Multi-Cluster Setup

Each OpenChoreo plane runs in its own k3d cluster.

[See multi-cluster/README.md](./multi-cluster/README.md)
