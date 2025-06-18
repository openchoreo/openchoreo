# Support External Container Registries for Image Pulling

**Authors**:  
@chalindukodikara

**Reviewers**:  
@Mirage20  
@sameerajayasoma

**Created Date**:  
2025-06-18

**Status**:  
Submitted

**Related Issues/PRs**:  
[Issue #246 – openchoreo/openchoreo](https://github.com/openchoreo/openchoreo/issues/246)

---

## Summary
[Proposal #245](https://github.com/openchoreo/openchoreo/blob/main/docs/proposals/0245_introduce-build-plane.md) introduced support for pushing images to external container registries. This proposal builds on that by adding support for pulling images from external registries within the DataPlane.

Currently, all images are pulled from the internal registry deployed inside the cluster. However, in real-world, multi-environment deployments, it is common to rely on external registries like Docker Hub, GHCR, or private registries.

This proposal enables users to configure external registries for image pulling, enhancing flexibility, scalability, and production readiness in OpenChoreo.

---

## Motivation
- Real-world environments frequently use external registries for storing and retrieving container images.
- OpenChoreo currently supports pushing to external registries but lacks the ability to pull from them.
- Pulling from external registries requires secrets to be available in all relevant namespaces in the DataPlane.
- As environment/project creation dynamically introduces new namespaces, we need a robust way to ensure those namespaces can pull images securely.

---

## Goals
- Allow users to configure external registries for pulling container images in the DataPlane.
- Ensure image pull secrets are available in all dynamically created namespaces.
- Leverage a well-supported, declarative tool to replicate these secrets automatically.

---

## Impact

| Area                     | Description                                                                                                                    |
|--------------------------|--------------------------------------------------------------------------------------------------------------------------------|
| **DataPlane CRD**        | Will be updated to support referencing pull secrets.                                                                           |
| **DataPlane Helm Chart** | Will deploy the replicator tool to enable secret syncing.                                                                      |
| **Platform Engineers**   | Responsible for managing the registry pull secret in a single place (`choreo-system` namespace) and refer it in the DataPlane. |
| **Developers**           | Benefit from seamless registry access without managing secrets manually.                                                       |

---

## Design

### Secret Management Flow

- The **DataPlane Helm chart** includes the [kubernetes-replicator](https://github.com/mittwald/kubernetes-replicator) tool.
- During installation, the replicator operator is deployed into the `choreo-system` namespace.
- **Platform Engineers (PEs)** create the required image pull secrets in the `choreo-system` namespace. These secrets:
   - Follow a standard naming convention.
   - Are referenced in the corresponding `DataPlane` CR.
   - Include the following annotation to the secret to enable replication:

  ```yaml
  annotations:
    replicator.v1.mittwald.de/replicate-to: "dp-.*"
  ```
  This ensures that the secret is automatically propagated to any namespace starting with dp-, including those created later by users for their components and projects.

---

### Considerations
- Secrets must be stored in the DataPlane, not the Control Plane, to avoid cross-cluster secret syncing complexities.
- Namespace-based secret replication is managed declaratively via annotations.
- Using [mittwald/kubernetes-replicator](https://github.com/mittwald/kubernetes-replicator) ensures reliability and aligns with other widely adopted Kubernetes practices.
- This approach also improves security by keeping secret scopes local to the DataPlane cluster.

---

### DataPlane CRD

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
