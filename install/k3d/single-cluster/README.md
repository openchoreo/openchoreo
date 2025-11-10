# Single-Cluster Setup

All-in-one OpenChoreo setup with all planes running in a single k3d cluster.

## Overview

This setup creates one k3d cluster that hosts all OpenChoreo planes, providing a simpler development environment with
lower resource requirements.

## Quick Start

> [!IMPORTANT]
> If you're using Colima, set the `K3D_FIX_DNS=0` environment variable when creating clusters.
> See [k3d-io/k3d#1449](https://github.com/k3d-io/k3d/issues/1449) for more details.
> Example: `K3D_FIX_DNS=0 k3d cluster create --config install/k3d/single-cluster/config.yaml`

### 1. Create Cluster

```bash
# Create single OpenChoreo cluster
k3d cluster create --config install/k3d/single-cluster/config.yaml
```

### 2. Install Components

Install all planes in the single cluster:

```bash
# Control Plane
helm install openchoreo-control-plane install/helm/openchoreo-control-plane \
  --kube-context k3d-openchoreo \
  --namespace openchoreo-control-plane \
  --create-namespace \
  --values install/k3d/single-cluster/values-cp.yaml

# Data Plane
helm install openchoreo-data-plane install/helm/openchoreo-data-plane \
  --kube-context k3d-openchoreo \
  --namespace openchoreo-data-plane \
  --create-namespace \
  --values install/k3d/single-cluster/values-dp.yaml

# Build Plane (optional)
helm install openchoreo-build-plane install/helm/openchoreo-build-plane \
  --kube-context k3d-openchoreo \
  --namespace openchoreo-build-plane \
  --create-namespace \
  --values install/k3d/single-cluster/values-bp.yaml

# Observability Plane (optional)
helm install openchoreo-observability-plane install/helm/openchoreo-observability-plane \
  --kube-context k3d-openchoreo \
  --namespace openchoreo-observability-plane \
  --create-namespace \
  --values install/k3d/single-cluster/values-op.yaml
```

## Port Mappings

| Plane               | Namespace                      | Kube API Port | Port Range |
|---------------------|--------------------------------|---------------|------------|
| Control Plane       | openchoreo-control-plane       | 6550          | 8xxx       |
| Data Plane          | openchoreo-data-plane          | -             | 9xxx       |
| Build Plane         | openchoreo-build-plane         | -             | 10xxx      |
| Observability Plane | openchoreo-observability-plane | -             | 11xxx      |

> [!NOTE]
> Port ranges (e.g., 8xxx) indicate the ports exposed to your host machine for accessing services from that plane. Each
> range uses ports like 8080 (HTTP) and 8443 (HTTPS) on localhost. In single-cluster mode, all planes share the same
> Kubernetes API (port 6550).

## Access Services

### Control Plane

- OpenChoreo UI: http://openchoreo.localhost:8080
- OpenChoreo API: http://api.openchoreo.localhost:8080
- Asgardeo Thunder: http://thunder.openchoreo.localhost:8080

### Data Plane

- User Workloads: http://localhost:9080 (Envoy Gateway)

### Build Plane (if installed)

- Argo Workflows UI: http://localhost:10081

### Observability Plane (if installed)

- Observer API: http://localhost:11080
- OpenSearch Dashboard: http://localhost:11081
- OpenSearch API: http://localhost:11082 (for Fluent Bit and direct API access)

## Verification

Check that all components are running:

```bash
# Control Plane
kubectl --context k3d-openchoreo get pods -n openchoreo-control-plane

# Data Plane
kubectl --context k3d-openchoreo get pods -n openchoreo-data-plane

# Build Plane
kubectl --context k3d-openchoreo get pods -n openchoreo-build-plane

# Observability Plane
kubectl --context k3d-openchoreo get pods -n openchoreo-observability-plane
```

## Architecture

```mermaid
graph TB
    subgraph "Host Machine (Docker)"
        subgraph "Single k3d Cluster (k3d-openchoreo)"
            ExtLB["k3d-serverlb<br/>localhost:6550/8080/8443/9080/9443/10081/11080/11081/11082"]
            K8sAPI["K8s API Server<br/>:6443"]

            subgraph "Control Plane Namespace"
                CP_IntLB["Traefik<br/>LoadBalancer :80/:443"]
                CP["Controller Manager"]
                API["OpenChoreo API :8080"]
                UI["OpenChoreo UI :7007"]
                Thunder["Asgardeo Thunder :8090"]
            end

            subgraph "Data Plane Namespace"
                DP_IntLB["Envoy Gateway<br/>LoadBalancer :9080/:9443"]
                Workloads["User Workloads"]
                FB_ALL["Fluent Bit DaemonSet<br/>Collects logs from ALL namespaces<br/>(CP + DP + BP + OP)"]
            end

            subgraph "Build Plane Namespace"
                BP_IntLB["Argo Server<br/>LoadBalancer :10081"]
                ArgoWF["Argo Workflows"]
            end

            subgraph "Observability Plane Namespace"
                Observer["Observer API<br/>LoadBalancer :11080"]
                OSD["OpenSearch Dashboard<br/>LoadBalancer :11081"]
                OS["OpenSearch<br/>LoadBalancer :11082"]
            end

            %% External access via load balancer
            ExtLB -->|":6550→:6443"| K8sAPI
            ExtLB -->|":8080→:80"| CP_IntLB
            ExtLB -->|":9080→:9080"| DP_IntLB
            ExtLB -->|":10081→:10081"| BP_IntLB
            ExtLB -->|":11080→:11080"| Observer
            ExtLB -->|":11081→:11081"| OSD
            ExtLB -->|":11082→:11082"| OS

            %% Control plane internal routing
            CP_IntLB --> UI
            CP_IntLB --> API
            CP_IntLB --> Thunder

            %% Data plane internal routing
            DP_IntLB --> Workloads
            BP_IntLB --> ArgoWF

            %% Control plane to workload communication (in-cluster)
            CP -->|"In-cluster"| Workloads
            CP -->|"In-cluster"| ArgoWF

            %% Logging flow - ALL logs go through Data Plane FluentBit
            FB_ALL -->|"In-cluster<br/>opensearch.openchoreo-observability-plane:9200"| OS

            %% Observability internal
            Observer --> OS
            OSD --> OS
        end
    end

    %% Styling
    classDef extLbStyle fill:#ffebee,stroke:#c62828,stroke-width:2px
    classDef intLbStyle fill:#fff3e0,stroke:#ff6f00,stroke-width:2px
    classDef apiStyle fill:#e1f5fe,stroke:#0277bd,stroke-width:2px

    class ExtLB extLbStyle
    class CP_IntLB,DP_IntLB,BP_IntLB,Observer,OSD,OS intLbStyle
    class K8sAPI apiStyle
```

## Cleanup

Delete the cluster:

```bash
k3d cluster delete openchoreo
```
