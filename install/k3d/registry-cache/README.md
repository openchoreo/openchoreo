# Registry Pull-Through Cache

A Compose setup that runs local pull-through caches for container registries used by OpenChoreo.
This speeds up repeated image pulls during local development.

## How It Works

Each cache is a `registry:2` instance configured as a pull-through proxy for one upstream registry.
On first pull, the image is fetched from the upstream and cached locally. Subsequent pulls (by any
client using the cache) are served from local disk.

```mermaid
flowchart LR
    A[k3d node] -->|pull ghcr.io/openchoreo/controller| B[ghcr-cache :5601]
    B -->|HIT| C[Serve from local disk]
    B -->|MISS| D[Fetch from ghcr.io]
    D --> E[Cache locally]
    E --> C
```

If the caches are not running, containerd falls back to pulling directly from the upstream
registry.

## Ports

| Port | Upstream        |
|------|-----------------|
| 5601 | ghcr.io         |
| 5602 | docker.io       |
| 5603 | quay.io         |
| 5604 | cr.kgateway.dev |
| 5605 | gcr.io          |

## Quick Start

```bash
# Start all caches
docker compose up -d

# Verify they're running
docker compose ps
```

Then create your k3d cluster with the registry mirrors. Add this to your k3d config's
`registries` section (or pass via `--registry-config`):

```yaml
registries:
  config: |
    mirrors:
      "host.k3d.internal:10082":
        endpoint:
          - http://host.k3d.internal:10082
      "ghcr.io":
        endpoint:
          - http://host.k3d.internal:5601
      "docker.io":
        endpoint:
          - http://host.k3d.internal:5602
      "quay.io":
        endpoint:
          - http://host.k3d.internal:5603
      "cr.kgateway.dev":
        endpoint:
          - http://host.k3d.internal:5604
      "gcr.io":
        endpoint:
          - http://host.k3d.internal:5605
```

## Browsing Cached Images

```bash
# List all cached repo:tag pairs
./list-cached.sh

# List only ghcr cache
./list-cached.sh ghcr-cache
```

## Refreshing Mutable Tags

Mutable tags like `latest-dev` get cached on first pull. If upstream publishes a new image
under the same tag, the cache will continue serving the old version.

Use the purge script to invalidate stale images:

```bash
# Purge a specific image:tag
./purge-cache.sh openchoreo/controller:latest-dev

# Purge all tags of a repo
./purge-cache.sh openchoreo/controller

# Purge all openchoreo repos
./purge-cache.sh openchoreo/*

# Purge from any registry (auto-detected)
./purge-cache.sh external-secrets/external-secrets:v2.0.1

# Purge everything from all caches
./purge-cache.sh --all
```

## Build Pipeline Integration

The registry cache can also accelerate build-time image pulls (buildpacks, base images, tools).
This requires patching two resource types:

1. **ClusterWorkflows** (control plane) — injects a `registries-conf` ConfigMap resource into each
   WorkflowRun so the cache mirror configuration is available to build pods.
2. **ClusterWorkflowTemplates** (data/workflow plane) — mounts that ConfigMap at
   `/etc/containers/registries.conf` so podman/buildah in build containers pull through the cache.

Both scripts are idempotent and support `--revert`. They require `kubectl` and `docker`.

### Single-cluster setup

```bash
# Patch ClusterWorkflows (control plane context)
./patch-build-workflows.sh

# Patch ClusterWorkflowTemplates (same context for single-cluster)
./patch-build-templates.sh
```

### Multi-cluster setup

```bash
# Patch ClusterWorkflows on the control plane cluster
./patch-build-workflows.sh --context k3d-openchoreo-cp

# Patch ClusterWorkflowTemplates on the workflow plane cluster
./patch-build-templates.sh --context k3d-openchoreo-wp
```

### Reverting

```bash
./patch-build-workflows.sh --revert
./patch-build-templates.sh --revert
```

### Preloading Build Images

After patching, you can warm the cache by running a Job that pulls common build images
(buildpacks, git, yq, jq, etc.) through the mirrors:

```bash
kubectl apply -f preload-build-images.yaml
```

The Job runs in the `default` namespace and auto-deletes after 5 minutes.

## Cleanup

```bash
# Stop caches (cached data preserved in Docker volumes)
docker compose down

# Stop and clear all cached data
docker compose down -v
```
