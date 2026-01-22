# Configuring the Build Plane

The Build Plane executes CI workflows using Argo Workflows. Built container images are pushed to a container registry.

## Container Registry

The Build Plane requires a container registry to store built images. You have two options:

### Option 1: Install a Local Registry

For local development, install a container registry in the same cluster:

```bash
helm repo add twuni https://twuni.github.io/docker-registry.helm
helm repo update

helm install registry twuni/docker-registry \
  --namespace openchoreo-build-plane \
  --create-namespace \
  --set persistence.enabled=true \
  --set persistence.size=10Gi \
  --set service.type=ClusterIP
```

Then configure the build plane:

```yaml
global:
  defaultResources:
    registry:
      host: "registry.openchoreo-build-plane.svc.cluster.local:5000"
      tlsVerify: false
```

### Option 2: Use an External Registry

For production, use an external registry (ECR, GCR, GHCR, Docker Hub):

```yaml
global:
  defaultResources:
    registry:
      host: "gcr.io/my-project"
      repoPath: "openchoreo"
      tlsVerify: true
```

#### Registry Authentication

Create a secret with registry credentials:

```bash
kubectl create secret docker-registry registry-push-secret \
  --namespace openchoreo-build-plane \
  --docker-server=gcr.io \
  --docker-username=_json_key \
  --docker-password="$(cat service-account.json)"
```

## Installation

```bash
helm install openchoreo-build-plane oci://ghcr.io/openchoreo/helm-charts/openchoreo-build-plane \
  --namespace openchoreo-build-plane \
  --create-namespace \
  --set global.defaultResources.registry.host=<your-registry-host>
```

## Configuration Options

| Parameter | Description | Default |
|-----------|-------------|---------|
| `global.defaultResources.registry.host` | Container registry host (REQUIRED) | `""` |
| `global.defaultResources.registry.repoPath` | Repository path prefix | `""` |
| `global.defaultResources.registry.tlsVerify` | Enable TLS verification | `false` |
