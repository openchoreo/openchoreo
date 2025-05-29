# Configuring Container Registries in OpenChoreo

This guide explains how to configure external container registries for use in **OpenChoreo**. These configurations allow the Build Plane to push built container images to the appropriate registries, whether public or private.

---

## Background: DataPlane–Component Relationship

In OpenChoreo:

- A **Component** belongs to a **Project**.
- A **Project** defines a **Deployment Pipeline**, which specifies the promotion flow (e.g., *Dev ➝ Stage ➝ Prod*).
- Each **Environment** in the pipeline runs within a **DataPlane**, and is modeled to reference it.
- Therefore, a **Component** can be deployed across **multiple environments**, and by extension, across **multiple DataPlanes**.

Each DataPlane can be configured with one or more container registries, both public (unauthenticated) and private (authenticated), to which the built images are pushed.

---

## Configuring Unauthenticated Registries

If your container registry does **not require authentication** (e.g., public repositories), you only need to declare them in the corresponding DataPlane custom resource.

### Example Configuration

Add the following to your `DataPlane` CR:

```yaml
spec:
  registry:
    unauthenticated:
      - docker.io/<your-org>
      - ghcr.io/<your-org>
      - registry.choreo-system:5000
```

> [!TIP]
> registry.choreo-system:5000 is the built-in, in-cluster registry provided by the DataPlane.

## Configuring Authenticated Registries

To push images to **private registries**, you need to set up secrets in the Build Plane and link them from the appropriate DataPlanes.

### Step-by-Step

#### 1. Create Image Push Secrets

Create Kubernetes secrets for your private registries in the **BuildPlane's** namespace:

- Namespace: `choreo-ci-<org-name>`

Follow [Kubernetes secret creation instructions](https://kubernetes.io/docs/tasks/configure-pod-container/pull-image-private-registry/) to create secrets.

> [!IMPORTANT]
> For each private registry you use must have a corresponding secret in this namespace.

#### 2. Reference Secrets in the DataPlane

Add the image push secrets to your DataPlane CR as follows:
```yaml
spec:
  registry:
    imagePushSecrets:
      - name: dev-dockerhub-push-secret
        prefix: docker.io/<your-org>
      - name: prod-ghcr-push-secret
        prefix: ghcr.io/<your-org>
```

You can link each secret to its registry prefix so the Build Plane knows which credentials to use when pushing images.

> [!NOTE]
> Currently, the BuildPlane runs within a **DataPlane** cluster. A standalone BuildPlane with CRD support is on the roadmap.

---

## Summary

| Type              | Where to Configure       | Auth Required | Notes                                                   |
|-------------------|--------------------------|---------------|----------------------------------------------------------|
| Public Registry   | DataPlane (unauthenticated section) | ❌            | No credentials needed                                   |
| Private Registry  | Secret in `choreo-ci-<org>` + reference in DataPlane | ✅            | Secret must exist in the BuildPlane namespace           |

By properly configuring registries, OpenChoreo ensures that your build outputs are securely and correctly published to the right registry endpoints, aligning with your deployment pipeline and environment setup.
