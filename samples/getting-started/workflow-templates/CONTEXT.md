# Rootless Workflow Templates — Context & Changes

## Problem

All OpenChoreo CI workflow templates run with `securityContext: privileged: true` because they use `ghcr.io/openchoreo/podman-runner:v1.1` for container image building, pushing, and CLI tooling (jq, yq, occ via `podman run`). This blocks deployment on managed Kubernetes (EKS, GKE, AKS) that enforce Pod Security Standards, creates host-level blast radius from compromised builds, and fails SOC 2/PCI-DSS compliance reviews.

### Why Podman Needed Privileged Mode

1. `podman build` / `podman run` create nested containers requiring kernel namespace manipulation
2. Storage config uses `fuse-overlayfs` which needs FUSE device access and elevated mount capabilities
3. Buildpack workflows (`pack build`) start `podman system service` as a daemon that needs namespace/cgroup access

## Approach

Replace the monolithic privileged podman-runner with purpose-built rootless images for each workflow step. Each scenario lives in its own folder under `samples/getting-started/workflow-templates/` for independent testing. No existing files were modified.

### Kubernetes Prerequisite

Kubernetes 1.33+ where `UserNamespacesSupport` is GA (enabled by default). The k3d cluster version was already updated to `rancher/k3s:v1.34.7-k3s1` in `install/k3d/single-cluster/config.yaml`.

### User Namespaces (`hostUsers: false`)

All templates that perform container image building or manipulation (Buildah, BuildKit, Podman publish) use `podSpecPatch: '{"hostUsers": false}'` to enable Kubernetes user namespaces. This is critical for two reasons:

1. **Performance**: Enables the **overlay** storage driver for Buildah and Podman (instead of VFS, which copies all layers and is much slower for large images).
2. **Security**: Container UIDs map to unprivileged host UIDs. Even `privileged: true` (used by BuildKit) only grants capabilities within the user namespace, not on the host.

Templates that don't perform container-in-container operations (CNB lifecycle builds, generate-workload) do not need `hostUsers: false`.

## New Files

### Base Image

| File | Purpose |
|---|---|
| `install/base-images/Dockerfile.ci-tools` | Slim Alpine 3.23 with curl, jq, yq. No Podman, no pack, no fuse-overlayfs. Does NOT include occ — the occ version is controlled via init container in the workflow template. |

Build and load:
```bash
docker build -f install/base-images/Dockerfile.ci-tools -t ghcr.io/openchoreo/ci-tools:dev .
k3d image import ghcr.io/openchoreo/ci-tools:dev -c openchoreo
```

Published for testing at: `docker.io/chalindukodikara/ci-tools:dev`

### Workflow Templates

| Folder | File | Replaces | Image | Strategy |
|---|---|---|---|---|
| `rootless-buildah/` | `containerfile-build.yaml` | `containerfile-build.yaml` | `quay.io/buildah/stable:v1.43.0` | Buildah with overlay storage, `--isolation chroot`, `hostUsers: false` |
| `rootless-buildkit/` | `containerfile-build.yaml` | `containerfile-build.yaml` | `moby/buildkit:v0.29.0` | Rootful `buildctl-daemonless.sh`, `privileged: true` in user namespace |
| `rootless-cnb/` | `paketo-buildpacks-build.yaml` | `paketo-buildpacks-build.yaml` | `paketobuildpacks/builder-jammy-full:0.3.603` + Podman sidecar | CNB `-daemon` export to rootless local Podman, outputs docker archive |
| `rootless-cnb/` | `gcp-buildpacks-build.yaml` | `gcp-buildpacks-build.yaml` | `gcr.io/buildpacks/builder@sha256:5977b4...` + Podman sidecar | CNB `-daemon` export to rootless local Podman, outputs docker archive |
| `rootless-cnb/` | `ballerina-buildpack-build.yaml` | `ballerina-buildpack-build.yaml` | `ghcr.io/openchoreo/buildpack/ballerina:18` + Podman sidecar | CNB `-daemon` export to rootless local Podman, outputs docker archive |
| `rootless-publish/` | `publish-image-k3d.yaml` | `publish-image-k3d.yaml` | `quay.io/podman/stable:v5.7.1` | Rootless Podman with overlay storage, accepts docker archive, `hostUsers: false` |
| `rootless-tooling/` | `generate-workload-k3d.yaml` | `generate-workload-k3d.yaml` | `ghcr.io/openchoreo/ci-tools:dev` | Direct jq/yq/occ calls, occ via init container |

### CI Workflows

| File | ClusterWorkflow Name | Pipeline Steps |
|---|---|---|
| `ci-workflows/dockerfile-builder-buildah.yaml` | `dockerfile-builder-buildah` | checkout → `containerfile-build-buildah` → `publish-image-rootless` → `generate-workload-rootless` |
| `ci-workflows/dockerfile-builder-buildkit.yaml` | `dockerfile-builder-buildkit` | checkout → `containerfile-build-buildkit` → `publish-image-rootless` → `generate-workload-rootless` |
| `ci-workflows/paketo-buildpacks-builder-rootless.yaml` | `paketo-buildpacks-builder-rootless` | checkout → `paketo-buildpacks-build-rootless` → `publish-image-rootless` → `generate-workload-rootless` |
| `ci-workflows/gcp-buildpacks-builder-rootless.yaml` | `gcp-buildpacks-builder-rootless` | checkout → `gcp-buildpacks-build-rootless` → `publish-image-rootless` → `generate-workload-rootless` |
| `ci-workflows/ballerina-buildpack-builder-rootless.yaml` | `ballerina-buildpack-builder-rootless` | checkout → `ballerina-buildpack-build-rootless` → `publish-image-rootless` → `generate-workload-rootless` |

CNB workflows route through the same `publish-image-rootless` step as Dockerfile builds. The CNB build step exports only to a local Podman daemon and writes `/mnt/vol/app-image.tar`; the publish step owns registry authentication and remote push.

### ClusterWorkflowTemplate Names

| Template Name | Purpose |
|---|---|
| `containerfile-build-buildah` | Dockerfile build via Buildah |
| `containerfile-build-buildkit` | Dockerfile build via BuildKit |
| `paketo-buildpacks-build-rootless` | Paketo buildpack build via CNB lifecycle |
| `gcp-buildpacks-build-rootless` | GCP buildpack build via CNB lifecycle |
| `ballerina-buildpack-build-rootless` | Ballerina buildpack build via CNB lifecycle |
| `publish-image-rootless` | Image publish via rootless Podman |
| `generate-workload-rootless` | Workload generation via direct CLI tools |

## Design Decisions

### 1. User Namespaces + Overlay Storage (Buildah, Podman, BuildKit)

All templates that build or manipulate container images use `podSpecPatch: '{"hostUsers": false}'` to enable Kubernetes user namespaces. This enables the **overlay** storage driver for Buildah and Podman, replacing VFS.

**Why overlay over VFS:**
- VFS copies every layer on every operation — usable but slow, especially for large images
- Overlay uses copy-on-write at the kernel level — fast and memory-efficient
- Overlay requires user namespace support (Linux 5.11+, K8s 1.33+ GA) which `hostUsers: false` provides
- No FUSE devices or `fuse-overlayfs` needed — native kernel overlay works in user namespaces

**Storage configuration** uses a dedicated emptyDir volume at `/storage` (10Gi limit) with `storage.conf`:
```ini
[storage]
driver = "overlay"
runroot = "/storage/run"
graphroot = "/storage/graph"
rootless_storage_path = "/storage/graph"
[storage.options]
pull_options = {enable_partial_images = "true", use_hard_links = "false", ostree_repos=""}
[storage.options.overlay]
```

### 2. Rootful BuildKit in User Namespace (not rootless BuildKit)

BuildKit uses the **rootful** image (`moby/buildkit:v0.29.0`) with `privileged: true` + `hostUsers: false`, instead of the rootless variant with `seccomp: Unconfined`.

**Why rootful over rootless:**
- Rootless BuildKit uses rootlesskit to create user mappings, which **conflicts with Kubernetes user namespaces** — rootlesskit cannot nest inside an existing user namespace
- Rootful BuildKit with `hostUsers: false` maps UID 0 inside to an unprivileged host UID — `privileged: true` grants full capabilities **within the user namespace only**, not on the host
- No need for `seccomp: Unconfined` (which relaxes syscall filtering on the host) or `--oci-worker-no-process-sandbox` (which disables process isolation)
- This is the approach recommended by the Kubernetes community for BuildKit on K8s 1.33+

**Kaniko** was ruled out — archived by Google (June 2025).

### 3. Buildah with `--isolation chroot` (Dockerfile builds)

Buildah uses `--isolation chroot` for building inside containers:
- Daemonless by design, simplest migration from Podman (nearly 1:1 CLI)
- Overlay storage via user namespaces for good performance
- `runAsUser: 1000, runAsGroup: 1000` — non-root even inside the user namespace
- Installs jq at runtime via `dnf` (Fedora-based image)
- Outputs `docker-archive` tarball compatible with the Podman publish step
- No privileged, no capabilities

### 4. CNB Local Daemon Export (Buildpack builds)

The CNB templates build through a rootless local Podman daemon and leave registry push to `publish-image-rootless`:
- The Kubernetes build container is the CNB builder image; a `quay.io/podman/stable:v5.7.1` sidecar owns the local daemon.
- `podSpecPatch: '{"hostUsers": false}'` enables user namespaces and native overlay storage.
- The sidecar starts `podman system service` on a shared Unix socket.
- The CNB builder container invokes `/cnb/lifecycle/creator -daemon` against that socket.
- The lifecycle exports the app image into the local Podman daemon only.
- A separate Podman client container in the same Argo `containerSet` saves `/mnt/vol/app-image.tar`.
- No registry credentials are exposed to the CNB build container or buildpacks.
- No experimental CNB OCI-layout export is used.

### 5. Rootless Podman for Publish (not Crane)

The publish step uses rootless Podman (`quay.io/podman/stable:v5.7.1`) with overlay storage via `hostUsers: false`:
- Same `podman load/tag/push` flow the team already knows
- Overlay storage driver for fast layer handling
- `runAsUser: 1000, runAsGroup: 1000` — non-root
- Consistent tooling across the project
- Accepts `/mnt/vol/app-image.tar` from Buildah, BuildKit, and CNB builds

### 6. occ CLI Decoupled from Base Image

The `ci-tools` image does NOT bundle occ. Instead, the generate-workload template uses:
- An `initContainers` block that copies `/usr/local/bin/occ` from a configurable CLI image
- `openchoreo-cli-image` input parameter (default: `ghcr.io/openchoreo/openchoreo-cli:latest-dev`)
- Shared `occ-bin` emptyDir volume mounted read-only in the main container

This means the occ version can be changed by updating the workflow template parameter without rebuilding any image.

### 7. All Images Pinned to Specific Versions

No floating tags (`latest`, `stable`). Every image reference uses a version tag or sha256 digest:

| Image | Pinned Version |
|---|---|
| `quay.io/buildah/stable` | `v1.43.0` |
| `moby/buildkit` | `v0.29.0` |
| `quay.io/podman/stable` | `v5.7.1` |
| `paketobuildpacks/builder-jammy-full` | `0.3.603` |
| `paketobuildpacks/run-jammy-full` | `0.1.130` |
| `gcr.io/buildpacks/builder` | `@sha256:5977b4bd47d3e9ff729eefe9eb99d321d4bba7aa3b14986323133f40b622aef1` |
| `gcr.io/buildpacks/google-22/run` | `@sha256:a8ccb6641b4d98b0adf6397f954e7194611d1ae61310f0561f1c00fdf7f9ba96` |
| `ghcr.io/openchoreo/buildpack/ballerina` | `18` / `18-run` |
| `ghcr.io/openchoreo/ci-tools` | `dev` |

## Security Context Summary

| Template | Before | After |
|---|---|---|
| `containerfile-build` (Buildah) | `privileged: true` | `hostUsers: false`, `runAsUser: 1000`, overlay storage |
| `containerfile-build` (BuildKit) | `privileged: true` | `hostUsers: false`, `privileged: true` in user NS, rootful image |
| `*-buildpacks-build` | `privileged: true` | `runAsUser: 1000, runAsGroup: 1000`, `fsGroup: 1000` |
| `publish-image` | `privileged: true` | `hostUsers: false`, `runAsUser: 1000`, overlay storage |
| `generate-workload` | `privileged: true` | `runAsUser: 1000, runAsNonRoot: true, capabilities: drop: [ALL]` |

## Testing

### Apply Templates

```bash
kubectl apply -f samples/getting-started/workflow-templates/rootless-buildah/
kubectl apply -f samples/getting-started/workflow-templates/rootless-buildkit/
kubectl apply -f samples/getting-started/workflow-templates/rootless-cnb/
kubectl apply -f samples/getting-started/workflow-templates/rootless-publish/
kubectl apply -f samples/getting-started/workflow-templates/rootless-tooling/
```

### Apply CI Workflows

```bash
kubectl apply -f samples/getting-started/ci-workflows/dockerfile-builder-buildah.yaml
kubectl apply -f samples/getting-started/ci-workflows/dockerfile-builder-buildkit.yaml
kubectl apply -f samples/getting-started/ci-workflows/paketo-buildpacks-builder-rootless.yaml
kubectl apply -f samples/getting-started/ci-workflows/gcp-buildpacks-builder-rootless.yaml
kubectl apply -f samples/getting-started/ci-workflows/ballerina-buildpack-builder-rootless.yaml
```

### What to Validate

1. **Buildah containerfile-build**: Does overlay storage work with `hostUsers: false`? Does `--isolation chroot` build succeed?
2. **BuildKit containerfile-build**: Does rootful `buildctl-daemonless.sh` work with `privileged: true` + `hostUsers: false`? Is it faster than VFS-based rootless?
3. **CNB lifecycle builds**: Does rootless Podman work with `hostUsers: false`, does `/cnb/lifecycle/creator -daemon` export to the local daemon, and does the step save `/mnt/vol/app-image.tar`?
4. **Podman rootless publish**: Can `podman load/tag/push` work with overlay storage and `hostUsers: false`?
5. **Generate workload**: Does the init container pattern work for occ injection? Do direct jq/yq calls work?
6. **CI workflow pipelines**: Does each end-to-end workflow (checkout → build → publish → generate) complete successfully?

### Potential Issues

- **k3d user namespace support**: k3d runs k3s inside Docker, which may not support nested user namespaces depending on the host kernel and Docker configuration. If `hostUsers: false` fails on k3d, the templates can fall back to VFS by changing `driver = "overlay"` to `driver = "vfs"` in the storage config and removing `podSpecPatch`.
- **BuildKit privileged in user NS**: Some container runtimes may not fully support `privileged: true` with `hostUsers: false`. This is GA in containerd 2.x but older runtimes may reject it.
- **CNB user permissions**: The checkout-source step runs as root, but CNB lifecycle runs as UID 1000. Templates include `chmod -R a+rX` as a workaround; `fsGroup: 1000` at pod level is the cleaner fix.
- **Overlay performance on emptyDir**: emptyDir backed by `tmpfs` may not support overlay. Ensure the node's kubelet uses disk-backed emptyDir (the default).

## Reference

- Task description: `samples/try-out-2/4-root-podman/TASK.md`
- Original runner image: `install/base-images/Dockerfile`
- Original workflow templates: `samples/getting-started/workflow-templates/*.yaml`
- CI workflow definitions: `samples/getting-started/ci-workflows/`
- Kubernetes user namespaces blog: https://kubernetes.web.cern.ch/blog/2025/06/19/rootless-container-builds-on-kubernetes/
