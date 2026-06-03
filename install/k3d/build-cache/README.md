# Build Caching with Zot

This guide shows how to configure build caching in the OpenChoreo workflow plane using [Zot](https://zotregistry.dev/), a lightweight OCI registry.

OpenChoreo source builds pull several images during every build: builder images, run images, lifecycle images, and Dockerfile base images. Builds can also recreate the same dependency and image layers repeatedly. A cluster-internal cache reduces repeated network pulls and improves build times, especially in local, CI, and shared development environments.

This guide configures two independent cache paths:

- **Mirror cache** — a pull-through cache for upstream images such as Docker Hub, GCR, GHCR, and Quay.
- **Layer cache** — a registry-backed build layer cache for Dockerfile builds and Cloud Native Buildpacks builds.

By the end, your workflow plane will have:

- A Zot registry running inside the `openchoreo-workflow-plane` namespace
- Transparent image mirroring through Podman `registries.conf`
- Layer caching for Podman Dockerfile builds
- Layer caching for Pack CLI buildpack builds
- Component-level cache controls

## Prerequisites

| Tool                                               | Purpose                                     |
| -------------------------------------------------- | ------------------------------------------- |
| [kubectl](https://kubernetes.io/docs/tasks/tools/) | Apply manifests and verify Kubernetes state |
| [Helm](https://helm.sh/docs/intro/install/)        | Install Zot                                 |
| [yq v4](https://github.com/mikefarah/yq)           | Patch existing OC ClusterWorkflow resources |


## Step 1: Install Zot

Zot runs as an internal ClusterIP service. Build pods use it for both upstream image mirroring and layer cache storage.

Install Zot using its official Helm chart and the OpenChoreo build-cache values file. The chart creates:

- A `build-cache` Service on port `5100`
- A single-replica Zot StatefulSet
- A 10Gi `ReadWriteOnce` PVC
- A Zot configuration with on-demand mirroring and retention-based garbage collection

### 1.1 Add the Zot Helm Repository

```bash
helm repo add project-zot http://zotregistry.dev/helm-charts/
helm repo update project-zot
```

### 1.2 Install the Registry

```bash
helm install build-cache project-zot/zot \
  -n openchoreo-workflow-plane --create-namespace \
  -f https://raw.githubusercontent.com/openchoreo/openchoreo/refs/heads/main/install/k3d/build-cache/values-zot.yaml
```

To customize the image tag, storage size, storage class, retention policies, or mirrored registries, download the values file, edit your copy, and pass it with `-f`.

The registry listens inside the cluster at:

```text
build-cache.openchoreo-workflow-plane.svc.cluster.local:5100
```

:::note
Use the full Zot image, not `zot-minimal`, because mirror caching relies on Zot's `sync` extension.
:::

:::note
The `docker2s2` compatibility mode is required when Pack CLI uses `--publish`. Pack can export Docker Manifest v2 Schema 2 images, and Zot rejects that media type unless this compatibility mode is enabled.
:::

:::tip Docker Hub mirroring
For Docker Hub, use `onDemand: true` only. Avoid polled mirroring because Docker Hub is rate-limited and does not support catalog listing.
:::

### 1.3 Verify Zot

```bash
kubectl -n openchoreo-workflow-plane rollout status statefulset/build-cache
```

Port-forward the registry and check the OCI registry API:

```bash
kubectl -n openchoreo-workflow-plane port-forward svc/build-cache 5100:5100 &
sleep 2
curl -v http://localhost:5100/v2/
kill %1
```

You should see a `200 OK` response.

## Step 2: Apply the Build Template YAMLs

Patch the in-cluster `ClusterWorkflowTemplate` resources so they:

1. Accept `build-cache` and `cache-layers-mode` input parameters for layer caching.
2. Mount an optional `registries-conf` ConfigMap for transparent image mirroring.

```bash
BASE=https://raw.githubusercontent.com/openchoreo/openchoreo/refs/heads/main/install/k3d/build-cache/workflow-templates

for tmpl in containerfile-build paketo-buildpacks-build gcp-buildpacks-build ballerina-buildpack-build; do
  kubectl apply -f "$BASE/$tmpl.yaml"
done
```

This updates the following templates:

| Template                    | Purpose                              |
| --------------------------- | ------------------------------------ |
| `containerfile-build`       | Dockerfile/Containerfile builds      |
| `paketo-buildpacks-build`   | Paketo Buildpacks builds             |
| `gcp-buildpacks-build`      | Google Cloud Buildpacks builds       |
| `ballerina-buildpack-build` | Ballerina buildpack builds           |

### Verify the Template Updates

Check that each template has the cache parameters:

```bash
for tmpl in containerfile-build paketo-buildpacks-build gcp-buildpacks-build ballerina-buildpack-build; do
  echo "=== $tmpl ==="
  kubectl get clusterworkflowtemplate "$tmpl" -o yaml | \
    yq '.spec.templates[] | select(.name == "build-image").inputs.parameters[] | select(.name == "build-cache" or .name == "cache-layers-mode")'
  echo ""
done
```

Check that each template mounts the optional `registries-conf` ConfigMap:

```bash
for tmpl in containerfile-build paketo-buildpacks-build gcp-buildpacks-build ballerina-buildpack-build; do
  echo "=== $tmpl ==="
  kubectl get clusterworkflowtemplate "$tmpl" -o yaml | \
    yq '.spec.templates[] | select(.name == "build-image").volumes[]? | select(.name == "registries-conf")'
  echo ""
done
```

## Step 3: Update the CI Workflow YAMLs

Patch the existing `OpenChoreo ClusterWorkflow` resources so Component authors can configure cache behavior with a structured `cache` parameter.

This step does three things:

1. Adds the `cache` parameter schema.
2. Passes layer cache settings to the build templates.
3. Creates a per-WorkflowRun `registries-conf` ConfigMap when mirror caching is enabled.

### 3.1 Patch All CI Workflows

```bash
CACHE_DESC="Build cache configuration. mirror.enabled controls upstream image pull-through caching via a mounted registries.conf. layers.mode controls layer cache behavior: disabled, reuse, or rebuild."

REGISTRIES_CONF='[[registry]]
location = "build-cache.openchoreo-workflow-plane.svc.cluster.local:5100"
insecure = true

[[registry]]
location = "docker.io"
[[registry.mirror]]
location = "build-cache.openchoreo-workflow-plane.svc.cluster.local:5100/mirror/docker.io"
insecure = true

[[registry]]
location = "gcr.io"
[[registry.mirror]]
location = "build-cache.openchoreo-workflow-plane.svc.cluster.local:5100/mirror/gcr.io"
insecure = true

[[registry]]
location = "ghcr.io"
[[registry.mirror]]
location = "build-cache.openchoreo-workflow-plane.svc.cluster.local:5100/mirror/ghcr.io"
insecure = true

[[registry]]
location = "quay.io"
[[registry.mirror]]
location = "build-cache.openchoreo-workflow-plane.svc.cluster.local:5100/mirror/quay.io"
insecure = true'

for workflow in dockerfile-builder paketo-buildpacks-builder gcp-buildpacks-builder ballerina-buildpack-builder; do
  kubectl get clusterworkflow "$workflow" -o yaml | \
    yq '
      del(
        .metadata.creationTimestamp,
        .metadata.generation,
        .metadata.managedFields,
        .metadata.resourceVersion,
        .metadata.uid,
        .status
      ) |

      .spec.parameters.openAPIV3Schema.properties.cache = {
        "type": "object",
        "default": {
          "mirror": {"enabled": true},
          "layers": {"mode": "reuse"}
        },
        "description": "'"$CACHE_DESC"'",
        "properties": {
          "mirror": {
            "type": "object",
            "default": {"enabled": true},
            "properties": {
              "enabled": {
                "type": "boolean",
                "default": true,
                "description": "Pull upstream builder, run, lifecycle, and base images through the workflow-plane cache registry when available"
              }
            }
          },
          "layers": {
            "type": "object",
            "default": {"mode": "reuse"},
            "properties": {
              "mode": {
                "type": "string",
                "default": "reuse",
                "enum": ["disabled", "reuse", "rebuild"],
                "description": "Layer cache mode: disabled skips cache, reuse reads and writes cache, rebuild ignores existing cache and writes a fresh cache"
              }
            }
          }
        }
      } |

      .spec.runTemplate.spec.arguments.parameters |=
        [.[] | select(.name != "build-cache" and .name != "cache-layers-mode")] +
        [
          {"name": "build-cache", "value": "build-cache.openchoreo-workflow-plane.svc.cluster.local:5100"},
          {"name": "cache-layers-mode", "value": "${parameters.cache.layers.mode}"}
        ] |

      .spec.runTemplate.spec.templates[0].steps[1][0].arguments.parameters |=
        [.[] | select(.name != "build-cache" and .name != "cache-layers-mode")] +
        [
          {"name": "build-cache", "value": "{{workflow.parameters.build-cache}}"},
          {"name": "cache-layers-mode", "value": "{{workflow.parameters.cache-layers-mode}}"}
        ] |

      .spec.resources = (.spec.resources // []) |
      del(.spec.resources[] | select(.id == "build-registries-conf")) |
      .spec.resources += [
        {
          "id": "build-registries-conf",
          "includeWhen": "${parameters.cache.mirror.enabled}",
          "template": {
            "apiVersion": "v1",
            "kind": "ConfigMap",
            "metadata": {
              "name": "${metadata.workflowRunName}-registries-conf",
              "namespace": "${metadata.namespace}"
            },
            "data": {
              "mirrors.conf": ""
            }
          }
        }
      ]
    ' | \
    REGISTRIES_CONF="$REGISTRIES_CONF" yq '
      (.spec.resources[] | select(.id == "build-registries-conf")).template.data."mirrors.conf" = strenv(REGISTRIES_CONF) |
      (.spec.resources[] | select(.id == "build-registries-conf")).template.data."mirrors.conf" style="literal"
    ' | kubectl apply -f -

  echo "Patched $workflow"
done
```

### 3.2 Verify the Workflow Schema

```bash
for workflow in dockerfile-builder paketo-buildpacks-builder gcp-buildpacks-builder ballerina-buildpack-builder; do
  echo "=== $workflow ==="
  kubectl get clusterworkflow "$workflow" -o yaml | yq '.spec.parameters.openAPIV3Schema.properties.cache'
  echo ""
done
```

Each workflow should show `cache.mirror.enabled` and `cache.layers.mode` with enum values:

```yaml
enum:
  - disabled
  - reuse
  - rebuild
```

### 3.3 Verify the Mirror Config Resource

```bash
for workflow in dockerfile-builder paketo-buildpacks-builder gcp-buildpacks-builder ballerina-buildpack-builder; do
  echo "=== $workflow ==="
  kubectl get clusterworkflow "$workflow" -o yaml | yq '.spec.resources[] | select(.id == "build-registries-conf")'
  echo ""
done
```

Each workflow should show the `build-registries-conf` resource with `includeWhen` and the `mirrors.conf` data.

## Step 4: Configure a Component

The default cache configuration enables both mirror caching and layer cache reuse:

```yaml
parameters:
  cache:
    mirror:
      enabled: true
    layers:
      mode: reuse
```

Use `rebuild` when you want a clean build that refreshes the layer cache for future builds:

```yaml
parameters:
  cache:
    mirror:
      enabled: true
    layers:
      mode: rebuild
```

Use `disabled` and turn off the mirror when you want a fully uncached build:

```yaml
parameters:
  cache:
    mirror:
      enabled: false
    layers:
      mode: disabled
```

## Step 5: Disable or Remove Build Cache

### 5.1 Disable Cache for One Component

```yaml
parameters:
  cache:
    mirror:
      enabled: false
    layers:
      mode: disabled
```

### 5.2 Remove Zot

```bash
helm uninstall build-cache -n openchoreo-workflow-plane
```

Helm leaves the StatefulSet PVC behind by design. Delete it if you want to reclaim storage:

```bash
kubectl delete pvc build-cache-pvc-build-cache-0 -n openchoreo-workflow-plane
```

Builds continue to work normally after Zot is removed. The `optional: true` ConfigMap mount means build pods start without the mirror configuration, and the cache probe falls back when Zot is unreachable.

### 5.3 Restore Original Workflow Templates and CI Workflows

If the live objects were previously patched from `kubectl get -o yaml`, their `kubectl.kubernetes.io/last-applied-configuration` annotations might contain server-owned metadata such as `resourceVersion`. Remove those annotations before restoring with client-side apply:

```bash
kubectl annotate clusterworkflowtemplate \
  containerfile-build \
  paketo-buildpacks-build \
  gcp-buildpacks-build \
  ballerina-buildpack-build \
  kubectl.kubernetes.io/last-applied-configuration-

kubectl annotate clusterworkflow \
  dockerfile-builder \
  paketo-buildpacks-builder \
  gcp-buildpacks-builder \
  ballerina-buildpack-builder \
  kubectl.kubernetes.io/last-applied-configuration-
```

Then apply the original manifests:

```bash
kubectl apply -f https://raw.githubusercontent.com/openchoreo/openchoreo/refs/heads/main/samples/getting-started/workflow-templates/containerfile-build.yaml
kubectl apply -f https://raw.githubusercontent.com/openchoreo/openchoreo/refs/heads/main/samples/getting-started/workflow-templates/paketo-buildpacks-build.yaml
kubectl apply -f https://raw.githubusercontent.com/openchoreo/openchoreo/refs/heads/main/samples/getting-started/workflow-templates/gcp-buildpacks-build.yaml
kubectl apply -f https://raw.githubusercontent.com/openchoreo/openchoreo/refs/heads/main/samples/getting-started/workflow-templates/ballerina-buildpack-build.yaml

kubectl apply -f https://raw.githubusercontent.com/openchoreo/openchoreo/refs/heads/main/samples/getting-started/ci-workflows/dockerfile-builder.yaml
kubectl apply -f https://raw.githubusercontent.com/openchoreo/openchoreo/refs/heads/main/samples/getting-started/ci-workflows/paketo-buildpacks-builder.yaml
kubectl apply -f https://raw.githubusercontent.com/openchoreo/openchoreo/refs/heads/main/samples/getting-started/ci-workflows/gcp-buildpacks-builder.yaml
kubectl apply -f https://raw.githubusercontent.com/openchoreo/openchoreo/refs/heads/main/samples/getting-started/ci-workflows/ballerina-buildpack-builder.yaml
```

## How It Works

### Cache Configuration

Component authors control caching with:

```yaml
parameters:
  cache:
    mirror:
      enabled: true
    layers:
      mode: reuse # disabled | reuse | rebuild
```

Layer caching supports three modes:

| Mode       | Read layer cache | Write layer cache | Use case                                      |
| ---------- | ---------------- | ----------------- | --------------------------------------------- |
| `disabled` | No               | No                | Run a fully uncached build                    |
| `reuse`    | Yes              | Yes               | Use normal fast builds                        |
| `rebuild`  | No               | Yes               | Run a clean build and refresh cache for later |

Mirror caching and layer caching are independent. You can disable the mirror while still using layer cache, or disable layer cache while still pulling upstream images through Zot.

### Parameter Flow

```text
Developer sets cache.mirror.enabled and cache.layers.mode
  -> ClusterWorkflow schema validates cache.layers.mode enum
    -> mirror.enabled=true: ClusterWorkflow creates registries-conf ConfigMap via includeWhen
    -> runTemplate maps cache.layers.mode to an Argo parameter
      -> build step passes build-cache and cache-layers-mode to ClusterWorkflowTemplate
        -> template mounts registries-conf ConfigMap (optional: true)
          -> Podman reads registries.conf.d/mirrors.conf on every pull
            -> All image pulls try Zot first and fall back to upstream
          -> template probes Zot for layer cache
            -> layers.mode reuse: read and write layer cache
            -> layers.mode rebuild: skip read and write a fresh cache
            -> layers.mode disabled: skip layer cache
```

### Mirroring Mechanism

Mirroring uses a mounted `registries.conf` ConfigMap with `[[registry.mirror]]` entries. Podman tries the Zot mirror first and falls back to upstream if the mirror is unavailable or does not have the image.

```toml
[[registry]]
location = "docker.io"

[[registry.mirror]]
location = "build-cache.openchoreo-workflow-plane.svc.cluster.local:5100/mirror/docker.io"
insecure = true
```

Zot's sync configuration uses `onDemand: true` with a catch-all content filter for each upstream registry. Images from Docker Hub, GCR, GHCR, and Quay are fetched and cached on first pull.

```json
{
  "prefix": "/**",
  "destination": "/mirror/docker.io",
  "stripPrefix": false
}
```

The mirror path format is:

```text
mirror/<registry>/<image>
```

Examples:

```text
mirror/docker.io/paketobuildpacks/builder-jammy-full
mirror/gcr.io/buildpacks/builder
mirror/ghcr.io/openchoreo/buildpack/ballerina
mirror/quay.io/example/image
```

### What Changed in the Build Templates

Each updated `ClusterWorkflowTemplate` adds:

1. A `registries-conf` volume:

```yaml
volumes:
  - name: registries-conf
    configMap:
      name: "{{workflow.parameters.workflowrun-name}}-registries-conf"
      optional: true
```

2. A `registries-conf` volume mount:

```yaml
volumeMounts:
  - mountPath: /etc/containers/registries.conf.d/mirrors.conf
    subPath: mirrors.conf
    name: registries-conf
    readOnly: true
```

3. Simplified input parameters:

```yaml
- name: build-cache
  default: "build-cache.openchoreo-workflow-plane.svc.cluster.local:5100"
- name: cache-layers-mode
  default: "reuse"
```

4. No mirror reference rewriting in build scripts.

Builder, run, lifecycle, and Dockerfile base image references remain as upstream references. Podman handles routing through the mirror based on the mounted `registries.conf` file.

### Cache Probe and Layer Cache

The build template probes Zot before enabling layer cache:

```bash
CACHE_AVAILABLE="false"
if [ -n "$CACHE_REGISTRY" ] && [ "$CACHE_REGISTRY" != "none" ]; then
  if curl -sf --max-time 3 -o /dev/null "http://${CACHE_REGISTRY}/v2/" 2>/dev/null; then
    CACHE_AVAILABLE="true"
    mkdir -p /etc/containers/registries.conf.d
    cat > /etc/containers/registries.conf.d/build-cache.conf <<REGEOF
[[registry]]
location = "${CACHE_REGISTRY}"
insecure = true
REGEOF
  fi
fi
```

This lets the build continue even when Zot is unreachable.

### Dockerfile Layer Cache

Podman Dockerfile builds use `--cache-from` and `--cache-to` with a cache TTL:

```bash
CACHE_ARGS="--layers --cache-from=${CACHE_REF} --cache-to=${CACHE_REF} --cache-ttl=168h"
```

For Podman, the cache reference is intentionally an untagged repository. Podman writes content-addressed cache tags under that repository and rejects a tagged `--cache-to` reference such as `:buildcache`.

### Buildpacks Layer Cache

Pack CLI Buildpacks builds use `--publish` with `--cache-image`:

```bash
pack build "$PUBLISH_REF" \
  --builder "$BUILDER" \
  --run-image "$RUN_IMG" \
  --publish \
  --cache-image "$CACHE_IMAGE" \
  ...

podman pull --tls-verify=false "$PUBLISH_REF"
podman tag "$PUBLISH_REF" "$IMAGE"
podman save -o /mnt/vol/app-image.tar "$IMAGE"
```

For Pack CLI, registry cache requires `--publish`. The template publishes the built image to Zot, pulls it back into Podman with `--tls-verify=false`, tags it as the expected workflow image, and saves `/mnt/vol/app-image.tar` for the rest of the pipeline.

### Cache Reference Paths

| Cache type                 | Reference pattern                                                        |
| -------------------------- | ------------------------------------------------------------------------ |
| Dockerfile layer cache     | `build-cache/containerfile/<namespace-project-component>`                |
| CNB layer cache            | `build-cache/cnb/<namespace-project-component>:cnb-cache`                |
| CNB handoff image          | `build-tmp/cnb/<namespace-project-component>:<image-tag>-<git-revision>` |
| Docker Hub mirror          | `mirror/docker.io/<repo>`                                                |
| GCR mirror                 | `mirror/gcr.io/<repo>`                                                   |
| GHCR mirror                | `mirror/ghcr.io/<repo>`                                                  |
| Quay mirror                | `mirror/quay.io/<repo>`                                                  |

### Cache Mechanisms

| Builder                 | `reuse`                                        | `rebuild`                                  |
| ----------------------- | ---------------------------------------------- | ------------------------------------------ |
| Podman Dockerfile build | `--layers --cache-from=<ref> --cache-to=<ref>` | `--layers --cache-to=<ref>`                |
| Pack CLI Buildpacks     | `--publish --cache-image=<ref>`                | `--publish --cache-image=<ref> --clear-cache` |

### Garbage Collection

Zot handles cleanup through online garbage collection and retention policies defined in the values file. A separate Kubernetes CronJob is not required.

The default retention policy keeps recent cache entries for build layers, temporary images, and mirrored upstream images. You can tune the policy in `values-zot.yaml` based on storage availability and expected build frequency.

### Graceful Degradation

Build caching is designed to fail open.

| Scenario                      | Behavior                                                                 |
| ----------------------------- | ------------------------------------------------------------------------ |
| `cache.mirror.enabled: false` | No ConfigMap is created. Podman pulls directly from upstream registries. |
| Zot is unreachable            | Layer cache is skipped. Builds continue without layer cache.             |
| Mirror is unavailable         | Podman falls back to the upstream registry.                              |
| Image is not already cached   | Zot fetches it on demand, stores it, and serves it to the build pod.     |
| `cache.layers.mode: disabled` | Build runs without reading or writing layer cache.                       |

## Next Steps

- Try a source build with `cache.layers.mode: reuse`.
- Run a second build and compare build time and image pull logs.
- Use `cache.layers.mode: rebuild` when you want to refresh the cache.
- Disable caching for a Component when debugging build reproducibility.
