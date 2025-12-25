# Private Registry Push Sample

This sample demonstrates how to configure a ComponentWorkflow to push built container images to a private registry that requires authentication.

## Overview

When pushing images to private registries (Docker Hub, GCR, ECR, Azure ACR, etc.), the build workflow needs credentials to authenticate with the registry. This sample shows two approaches:

1. **Manual Secret Creation** - Create the Kubernetes secret directly
2. **External Secrets Operator** - Use ExternalSecret to sync credentials from a secrets manager

## Files

| File | Description |
|------|-------------|
| `component-workflow.yaml` | ComponentWorkflow that references the ClusterWorkflowTemplate |
| `component-workflow-with-es.yaml` | ComponentWorkflow with ExternalSecret resource for automatic secret sync |
| `cluster-workflow-template.yaml` | Argo ClusterWorkflowTemplate with the push step configured for private registry |
| `registry-push-secret.yaml` | Kubernetes Secret template for registry credentials |
| `docker-config.json` | Docker config format for registry authentication |

## Deployment Targets

These resources are deployed to different planes:

| Resource | Plane | Namespace |
|----------|-------|-----------|
| `component-workflow.yaml` | Control Plane | Organization namespace (e.g., `default`) |
| `component-workflow-with-es.yaml` | Control Plane | Organization namespace (e.g., `default`) |
| `cluster-workflow-template.yaml` | Build Plane | Cluster-scoped |
| `registry-push-secret.yaml` | Build Plane | Build execution namespace (e.g., `openchoreo-ci-default`) |

## Setup

### Option 1: Manual Secret Creation

1. Generate your auth token (base64 of username:password):
   ```bash
   echo -n 'your-username:your-password' | base64
   ```

2. Update `docker-config.json` with your registry URL and auth token:
   ```json
   {
     "auths": {
       "https://index.docker.io/v1/": {
         "auth": "<BASE64_OF_USERNAME_COLON_PASSWORD>"
       }
     }
   }
   ```

3. Create the secret:
   ```bash
   # Using kubectl create secret
   kubectl create secret docker-registry registry-push-secret \
     --docker-server=https://index.docker.io/v1/ \
     --docker-username=your-username \
     --docker-password=your-password \
     -n openchoreo-ci-default

   # Or using the YAML template
   cat docker-config.json | tr -d '\n' | base64
   # Update registry-push-secret.yaml with the output
   kubectl apply -f registry-push-secret.yaml
   ```

4. Deploy the resources:
   ```bash
   # Apply to Build Plane
   kubectl apply -f cluster-workflow-template.yaml

   # Apply to Control Plane (organization namespace)
   kubectl apply -f component-workflow.yaml
   ```

### Option 2: External Secrets Operator

1. Ensure External Secrets Operator is installed and a ClusterSecretStore is configured

2. Store your Docker config JSON in your secrets manager (e.g., AWS Secrets Manager, HashiCorp Vault)

3. Deploy the resources:
   ```bash
   # Apply to Build Plane
   kubectl apply -f external-secrets/cluster-workflow-template.yaml

   # Apply to Control Plane (organization namespace)
   kubectl apply -f external-secrets/component-workflow-with-es.yaml
   ```

The ExternalSecret resource defined in the ComponentWorkflow will be created in the build execution namespace and automatically sync the registry credentials from your secrets manager.

## Configuration

Update the registry endpoint in `cluster-workflow-template.yaml`:

```yaml
REGISTRY_ENDPOINT="<Configure this to your private registry endpoint>"
```

Common registry endpoints:
- Docker Hub: `docker.io` or `index.docker.io`
- Google Container Registry: `gcr.io`
- AWS ECR: `<account-id>.dkr.ecr.<region>.amazonaws.com`
- Azure ACR: `<registry-name>.azurecr.io`
- GitHub Container Registry: `ghcr.io`

## Usage

Reference the workflow in your Component:

```yaml
apiVersion: openchoreo.dev/v1alpha1
kind: Component
metadata:
  name: my-app
spec:
  workflow:
    name: google-cloud-buildpacks-private-registry
    systemParameters:
      repository:
        url: "https://github.com/myorg/myapp"
        revision:
          branch: "main"
        appPath: "/"
```

## Pulling Images at Runtime

This sample covers **pushing** images to a private registry (Build Plane). **Pulling** images from a private registry at runtime is a separate concern handled in the Data Plane.

To pull images from private registries, you need to add `imagePullSecrets` to the Deployment template in your ComponentType's `resources` field.


### Option 1: Using External Secrets (Recommended)

Add an ExternalSecret resource to your ComponentType that syncs pull credentials from your secrets manager:

```yaml
apiVersion: openchoreo.dev/v1alpha1
kind: ComponentType
metadata:
  name: my-service-type
spec:
  resources:
    # ExternalSecret to sync pull credentials
    - id: registry-pull-secret
      template:
        apiVersion: external-secrets.io/v1
        kind: ExternalSecret
        metadata:
          name: registry-pull-secret
          namespace: ${metadata.namespace}
        spec:
          refreshInterval: 15s
          secretStoreRef:
            name: ${dataplane.secretStore}
            kind: ClusterSecretStore
          target:
            name: registry-pull-secret
            creationPolicy: Owner
            template:
              type: kubernetes.io/dockerconfigjson
          data:
            - secretKey: .dockerconfigjson
              remoteRef:
                key: registry-credentials
                property: dockerconfigjson

    # Deployment that uses the pull secret
    - id: deployment
      template:
        apiVersion: apps/v1
        kind: Deployment
        spec:
          template:
            spec:
              imagePullSecrets:
                - name: registry-pull-secret
              containers:
                - name: main
                  image: ${workload.containers[parameters.containerName].image}
```

### Option 2: Manual Secret Creation

1. Create the docker-registry secret in the Data Plane namespace:

```bash
kubectl create secret docker-registry registry-pull-secret \
  --docker-server=<registry-url> \
  --docker-username=<username> \
  --docker-password=<password> \
  -n openchoreo-data-plane
```

2. Add the `imagePullSecrets` field to your ComponentType's Deployment template:

```yaml
apiVersion: openchoreo.dev/v1alpha1
kind: ComponentType
metadata:
  name: my-service-type
spec:
  resources:
    - id: deployment
      template:
        apiVersion: apps/v1
        kind: Deployment
        spec:
          template:
            spec:
              # Add imagePullSecrets here
              imagePullSecrets:
                - name: registry-pull-secret
              containers:
                - name: main
                  image: ${workload.containers[parameters.containerName].image}
```

## See Also

- [ComponentWorkflow Samples](../README.md) - Overview of all ComponentWorkflow samples
- [Private Repository Sample](../with-private-repository/) - Cloning from private Git repositories
