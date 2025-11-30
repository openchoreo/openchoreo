# Multi-Cluster Setup

Production-like setup with each OpenChoreo plane running in its own k3d cluster.

## Overview

This setup creates separate k3d clusters for each plane, providing better isolation and mimicking production
architecture.

## Quick Start

> [!IMPORTANT]
> If you're using Colima, set the `K3D_FIX_DNS=0` environment variable when creating clusters.
> See [k3d-io/k3d#1449](https://github.com/k3d-io/k3d/issues/1449) for more details.
> Example: `K3D_FIX_DNS=0 k3d cluster create --config config-cp.yaml`

> [!TIP]
> For faster setup or if you have slow network, consider using [Image Preloading](#image-preloading-optional) after creating clusters.

### 1. Control Plane

Create cluster and install components:

```bash
# Create Control Plane cluster
k3d cluster create --config install/k3d/multi-cluster/config-cp.yaml

# Install Control Plane Helm chart
helm install openchoreo-control-plane install/helm/openchoreo-control-plane \
  --dependency-update \
  --kube-context k3d-openchoreo-cp \
  --namespace openchoreo-control-plane \
  --create-namespace \
  --values install/k3d/multi-cluster/values-cp.yaml
```

### 2. Data Plane

Create cluster and install components:

```bash
# Create Data Plane cluster
k3d cluster create --config install/k3d/multi-cluster/config-dp.yaml

# Install Data Plane Helm chart
helm install openchoreo-data-plane install/helm/openchoreo-data-plane \
  --dependency-update \
  --kube-context k3d-openchoreo-dp \
  --namespace openchoreo-data-plane \
  --create-namespace \
  --values install/k3d/multi-cluster/values-dp.yaml
```

### 3. Build Plane (Optional)

Create cluster and install components:

```bash
# Create Build Plane cluster
k3d cluster create --config install/k3d/multi-cluster/config-bp.yaml

# Install Build Plane Helm chart
helm install openchoreo-build-plane install/helm/openchoreo-build-plane \
  --dependency-update \
  --kube-context k3d-openchoreo-bp \
  --namespace openchoreo-build-plane \
  --create-namespace \
  --values install/k3d/multi-cluster/values-bp.yaml
```

### 4. Observability Plane (Optional)

Create cluster and install components:

```bash
# Create Observability Plane cluster
k3d cluster create --config install/k3d/multi-cluster/config-op.yaml

# Install Observability Plane Helm chart
helm install openchoreo-observability-plane install/helm/openchoreo-observability-plane \
  --dependency-update \
  --kube-context k3d-openchoreo-op \
  --namespace openchoreo-observability-plane \
  --create-namespace \
  --values install/k3d/multi-cluster/values-op.yaml
```

### 5. Create DataPlane Resource

Create a DataPlane resource to enable workload deployment:

```bash
./install/add-data-plane.sh \
  --control-plane-context k3d-openchoreo-cp \
  --target-context k3d-openchoreo-dp \
  --server https://host.k3d.internal:6551
```

### 6. Create BuildPlane Resource (optional)

Create a BuildPlane resource to enable building from source:

```bash
./install/add-build-plane.sh \
  --control-plane-context k3d-openchoreo-cp \
  --target-context k3d-openchoreo-bp \
  --server https://host.k3d.internal:6552
```

## Port Mappings

| Plane               | Cluster           | Kube API Port | Port Range |
|---------------------|-------------------|---------------|------------|
| Control Plane       | k3d-openchoreo-cp | 6550          | 8xxx       |
| Data Plane          | k3d-openchoreo-dp | 6551          | 9xxx       |
| Build Plane         | k3d-openchoreo-bp | 6552          | 10xxx      |
| Observability Plane | k3d-openchoreo-op | 6553          | 11xxx      |

> [!NOTE]
> Port ranges (e.g., 8xxx) indicate the ports exposed to your host machine for accessing services from that plane. Each
> range uses ports like 8080 (HTTP) and 8443 (HTTPS) on localhost.

## Access Services

### Control Plane

- OpenChoreo UI: http://openchoreo.localhost:8080
- OpenChoreo API: http://api.openchoreo.localhost:8080
- Asgardeo Thunder: http://thunder.openchoreo.localhost:8080

### Data Plane

- User Workloads: http://localhost:9080 (Envoy Gateway)

### Build Plane (if installed)

- Argo Workflows UI: http://localhost:10081
- Container Registry: http://localhost:10082

### Observability Plane (if installed)

- Observer API: http://localhost:11080
- OpenSearch Dashboard: http://localhost:11081
- OpenSearch API: http://localhost:11082 (for Fluent Bit and direct API access)

## Verification

Check that all components are running:

```bash
# Control Plane
kubectl --context k3d-openchoreo-cp get pods -n openchoreo-control-plane

# Data Plane
kubectl --context k3d-openchoreo-dp get pods -n openchoreo-data-plane

# Build Plane
kubectl --context k3d-openchoreo-bp get pods -n openchoreo-build-plane

# Observability Plane
kubectl --context k3d-openchoreo-op get pods -n openchoreo-observability-plane

# Verify DataPlane resource in Control Plane
kubectl --context k3d-openchoreo-cp get dataplane -n default

# Verify BuildPlane resource in Control Plane (if created)
kubectl --context k3d-openchoreo-cp get buildplane -n default
```

## Architecture

```mermaid
graph TB
    subgraph "Host Machine (Docker)"
        subgraph "Control Plane Network (k3d-openchoreo-cp) - Ports: 8xxx"
            CP_ExtLB["k3d-serverlb<br/>localhost:8080/8443/6550<br/>(host.k3d.internal for pods)"]
            CP_K8sAPI["K8s API Server<br/>:6443"]
            CP_IntLB["Traefik<br/>LoadBalancer :80/:443"]
            CP["Controller Manager"]
            API["OpenChoreo API :8080"]
            UI["OpenChoreo UI :7007"]
            Thunder["Asgardeo Thunder :8090"]

            CP_ExtLB -->|":6550→:6443"| CP_K8sAPI
            CP_ExtLB -->|":8080→:80"| CP_IntLB
            CP_IntLB --> UI
            CP_IntLB --> API
            CP_IntLB --> Thunder
        end

        subgraph "Data Plane Network (k3d-openchoreo-dp) - Ports: 9xxx"
            DP_ExtLB["k3d-serverlb<br/>localhost:9080/9443/6551<br/>(host.k3d.internal for pods)"]
            DP_K8sAPI["K8s API Server<br/>:6443"]
            DP_IntLB["Envoy Gateway<br/>LoadBalancer :80/:443"]
            Workloads["User Workloads"]
            FB_DP["Fluent Bit"]

            DP_ExtLB -->|":6551→:6443"| DP_K8sAPI
            DP_ExtLB -->|":9080→:80"| DP_IntLB
            DP_IntLB --> Workloads
            Workloads -.->|logs| FB_DP
        end

        subgraph "Build Plane Network (k3d-openchoreo-bp) - Ports: 10xxx"
            BP_ExtLB["k3d-serverlb<br/>localhost:10081/10082/6552<br/>(host.k3d.internal for pods)"]
            BP_K8sAPI["K8s API Server<br/>:6443"]
            BP_IntLB["Argo Server<br/>LoadBalancer :2746"]
            Registry["Container Registry<br/>LoadBalancer :5000"]
            FB_BP["Fluent Bit"]

            BP_ExtLB -->|":6552→:6443"| BP_K8sAPI
            BP_ExtLB -->|":10081→:2746"| BP_IntLB
            BP_ExtLB -->|":10082→:5000"| Registry
            BP_IntLB -.->|logs| FB_BP
        end

        subgraph "Observability Plane Network (k3d-openchoreo-op) - Ports: 11xxx"
            OP_ExtLB["k3d-serverlb<br/>localhost:11080/11081/11082/6553<br/>(host.k3d.internal for pods)"]
            OP_K8sAPI["K8s API Server<br/>:6443"]
            Observer["Observer API<br/>LoadBalancer :8080"]
            OSD["OpenSearch Dashboard<br/>LoadBalancer :5601"]
            OS["OpenSearch<br/>LoadBalancer :9200"]

            OP_ExtLB -->|":6553→:6443"| OP_K8sAPI
            OP_ExtLB -->|":11080→:8080"| Observer
            OP_ExtLB -->|":11081→:5601"| OSD
            OP_ExtLB -->|":11082→:9200"| OS
            Observer --> OS
            OSD --> OS
        end

        %% Inter-cluster communication
        CP -->|"host.k3d.internal:6551"| DP_ExtLB
        CP -->|"host.k3d.internal:6552"| BP_ExtLB
        FB_DP -->|"host.k3d.internal:11082"| OP_ExtLB
        FB_BP -->|"host.k3d.internal:11082"| OP_ExtLB
    end

    %% Styling
    classDef extLbStyle fill:#ffebee,stroke:#c62828,stroke-width:2px
    classDef intLbStyle fill:#fff3e0,stroke:#ff6f00,stroke-width:2px
    classDef apiStyle fill:#e1f5fe,stroke:#0277bd,stroke-width:2px

    class CP_ExtLB,DP_ExtLB,BP_ExtLB,OP_ExtLB extLbStyle
    class CP_IntLB,DP_IntLB,BP_IntLB,Registry,Observer,OSD,OS intLbStyle
    class CP_K8sAPI,DP_K8sAPI,BP_K8sAPI,OP_K8sAPI apiStyle
```

## Image Preloading (Optional)

If you have slow network or want to save bandwidth when re-creating clusters, you can preload images before installing components. This pulls images to your host machine first, then imports them to each k3d cluster, which is significantly faster than pulling from within the clusters.

**Control Plane:**
```bash
install/k3d/preload-images.sh \
  --cluster openchoreo-cp \
  --local-charts \
  --control-plane --cp-values install/k3d/multi-cluster/values-cp.yaml \
  --parallel 4
```

**Data Plane:**
```bash
install/k3d/preload-images.sh \
  --cluster openchoreo-dp \
  --local-charts \
  --data-plane --dp-values install/k3d/multi-cluster/values-dp.yaml \
  --parallel 4
```

**Build Plane (optional):**
```bash
install/k3d/preload-images.sh \
  --cluster openchoreo-bp \
  --local-charts \
  --build-plane --bp-values install/k3d/multi-cluster/values-bp.yaml \
  --parallel 4
```

**Observability Plane (optional):**
```bash
install/k3d/preload-images.sh \
  --cluster openchoreo-op \
  --local-charts \
  --observability-plane --op-values install/k3d/multi-cluster/values-op.yaml \
  --parallel 4
```

Run this after creating the clusters (step 1) but before installing components (step 2).

## Cleanup

Delete all clusters:

```bash
k3d cluster delete openchoreo-cp openchoreo-dp openchoreo-bp openchoreo-op
```
