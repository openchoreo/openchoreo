# Introduce BuildPlane for Build Workloads

**Authors**:  
@chalindukodikara

**Reviewers**:  
@Mirage20  
@sameerajayasoma

**Created Date**:  
2025-06-12

**Status**:  
Submitted

**Related Issues/PRs**:  
[Issue #245 – openchoreo/openchoreo](https://github.com/openchoreo/openchoreo/issues/245)

---

## Summary

Currently, all Argo Workflows run on a fixed Kubernetes cluster. This rigid setup limits flexibility in configuring build infrastructure, which can lead to inefficient resource usage or violation of security boundaries.

This proposal introduces a new `BuildPlane` kind that allows users to define a dedicated cluster for executing build workloads. With this, users can:

- Isolate build workloads from runtime environments to improve security and reliability.
- Scale build infrastructure independently of application runtime clusters.
- Select a specific DataPlane or an external cluster to serve as the build environment.

---

## Motivation

Enabling builds to run on any specified cluster gives users greater control over their CI/CD pipelines. This is especially useful when infrastructure separation is required for governance, compliance, or scalability.

---

## Goals

- Allow users to configure the target cluster for Argo Workflows, enabling control over where builds are executed.
- Provide a mechanism to specify container registry configurations for pushing built images.

---

## Non-Goals

- No current non-goals.

---

## Impact

- **Build Controller**: Needs updates to support scheduling builds on a specified `BuildPlane`.
- **Build Plane Client**: Logic must be standardized to work across both DataPlane and BuildPlane clusters.
- **DataPlane Resource**: Push registry configuration will be removed; the API will be refactored accordingly.
- **Installation Scripts**: The installation process will need to support creation of the `BuildPlane` resource.

---

## Design

The `BuildPlane` is a dedicated environment for executing CI workflows such as:

- Building container images
- Running unit/integration tests
- Publishing build artifacts

It operates via Argo Workflows within its own Kubernetes cluster, separate from both the Control Plane and Data Plane. Each `BuildPlane` is registered using a `BuildPlane` CR, which contains the connection details needed by the Control Plane to delegate build executions.

**Key Benefit**:  
Resource isolation: build workloads do not compete with runtime workloads for cluster resources.

> **Note**: A DataPlane can also serve as a BuildPlane in certain configurations.

---

### Considerations

1. Each Component is linked to a single `BuildPlane`.
2. An organization may have multiple `BuildPlanes`, but must define one as the default.
    - Projects or future sub-organization features can override this default.
3. All container registries listed in the `BuildPlane` are used for image pushing — even if not all are linked to the current component.

> **Initial Limitation**: Only one `BuildPlane` per organization will be supported at launch. Multi-`BuildPlane` support will be added in future phases.

---

### CRDs

#### BuildPlane

```yaml
apiVersion: core.choreo.dev
kind: BuildPlane
metadata:
  name: example-buildplane
spec:
  # References to ContainerRegistry CRs used for image push operations
  registries:
    - prefix: docker.io/namespace
      secretRef: docker-push-secret
    - prefix: ghcr.io/namespace
      secretRef: ghcr-push-secret
  kubernetesCluster:
    name: test-cluster
    credentials:
      apiServerUrl: https://api.example-cluster
      caCert: <base64-ca-cert>
      clientCert: <base64-client-cert>
      clientKey: <base64-client-key>
```

#### DataPlane

```yaml
apiVersion: core.choreo.dev
kind: DataPlane
metadata:
  name: example-dataplane
spec:
  # Reference to ContainerRegistry CR used for pulling images
  registry:
    prefix: docker.io/namespace
    secretRef: dockerhub-pull-secret
  kubernetesCluster:
    name: test-cluster
    credentials:
      apiServerUrl: https://api.example-cluster
      caCert: <base64-ca-cert>
      clientCert: <base64-client-cert>
      clientKey: <base64-client-key>
```
