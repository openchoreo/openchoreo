# Component with Traits Example

This example demonstrates the complete end-to-end workflow of the OpenChoreo component rendering system.

## Overview

This example shows:

- Component Types with templated resources
- Trait composition with creates and patches
- Environment-specific overrides
- Automatic name/namespace generation

## Components

The example includes a `component-with-traits.yaml` file containing all resources for easy deployment.

### ComponentType

Defines a `web-service` component type that creates:

- **Deployment**: With configurable replicas, resources, and container image
- **Service**: ClusterIP service exposing the deployment

Key features:

- Uses `${metadata.name}` and `${metadata.namespace}` for computed names
- Uses `${metadata.labels}` and `${metadata.podSelectors}` for labels/selectors
- Uses `${workload.containers["app"].image}` for the container image
- Supports environment-specific resource overrides

### Trait

Defines a `persistent-volume` trait that:

- **Creates**: A PersistentVolumeClaim
- **Patches**: Adds a volume and volumeMount to the Deployment

Key features:

- Parameterized volume name, mount path, and container name
- Environment-specific size and storage class overrides
- Uses `${metadata.name}-${trait.instanceName}` for PVC naming

### Component

Defines a `demo-app` component that:

- Uses the `web-service` component type
- Sets parameters for replicas, resources, and port
- Attaches the `persistent-volume` trait with instance name `data-storage`

### Workload

Represents the build output with:

- Container image: `nginx:1.25-alpine`

### ComponentDeployment

Defines deployment settings for the `development` environment:

- Reduces resource requests/limits for cost savings
- Changes PVC size from 20Gi → 5Gi (via trait overrides)
- Changes storage class from "fast" → "standard" (via trait overrides)

## Expected Output

The rendered resources will include:

- **Deployment**: `demo-app-development-<hash>` with nginx container and mounted volumes
- **Service**: `demo-app-development-<hash>` exposing the deployment
- **PersistentVolumeClaim**: `demo-app-development-<hash>-data-storage` for persistent storage

All resources will have:

- Computed names and namespaces based on project, component, and environment
- Standard labels for project, component, and environment
- Environment-specific overrides applied (reduced resources, smaller PVC size)

## Testing on Cluster

### Prerequisites

```bash
# Ensure OpenChoreo CRDs are installed
kubectl apply -f install/helm/openchoreo-control-plane/crds/

# Ensure OpenChoreo controller is running
make run
```

### Step 1: Apply resources

```bash
kubectl apply -f samples/component-with-traits/component-with-traits.yaml
```

### Step 2: Verify the Release is created

```bash
# Check Release was created
kubectl get release -n default

# View Release details
kubectl get release demo-app-development -n default -o yaml

# Check the rendered resources
kubectl get release demo-app-development -n default -o jsonpath='{.spec.resources[*].id}'
```

### Step 3: Verify ComponentDeployment status

```bash
# Check ComponentDeployment status
kubectl get componentdeployment -n default

# View detailed status
kubectl describe componentdeployment demo-app-development -n default
```

## Expected Rendering

The Release should contain 3 rendered resources:

### 1. Deployment

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: demo-app-development-<hash>
  namespace: dp-default-demo-project-development-<hash>
  labels:
    openchoreo.org/component: demo-app
    openchoreo.org/project: demo-project
    openchoreo.org/environment: development
spec:
  replicas: 2
  selector:
    matchLabels:
      openchoreo.org/component: demo-app
      openchoreo.org/environment: development
      openchoreo.org/project: demo-project
  template:
    spec:
      containers:
        - name: app
          image: nginx:1.25-alpine
          resources:
            requests:
              cpu: 50m # Overridden by ComponentDeployment
              memory: 128Mi # Overridden by ComponentDeployment
            limits:
              cpu: 200m # Overridden by ComponentDeployment
              memory: 256Mi # Overridden by ComponentDeployment
          volumeMounts: # Added by trait
            - name: app-data
              mountPath: /var/data
      volumes: # Added by trait
        - name: app-data
          persistentVolumeClaim:
            claimName: demo-app-development-<hash>-data-storage
```

### 2. Service

```yaml
apiVersion: v1
kind: Service
metadata:
  name: demo-app-development-<hash>
  namespace: dp-default-demo-project-development-<hash>
  labels:
    openchoreo.org/component: demo-app
    openchoreo.org/project: demo-project
    openchoreo.org/environment: development
spec:
  type: ClusterIP
  selector:
    openchoreo.org/component: demo-app
    openchoreo.org/environment: development
    openchoreo.org/project: demo-project
  ports:
    - name: http
      port: 80
      targetPort: 8080
```

### 3. PersistentVolumeClaim (created by trait)

```yaml
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: demo-app-development-<hash>-data-storage
  namespace: dp-default-demo-project-development-<hash>
  labels:
    openchoreo.org/component: demo-app
    openchoreo.org/environment: development
spec:
  accessModes:
    - ReadWriteOnce
  resources:
    requests:
      storage: 5Gi # Overridden by ComponentDeployment (was 20Gi)
  storageClassName: standard # Overridden by ComponentDeployment (was "fast")
```

## Key Features Demonstrated

### 1. Component Types

- Templated resource definitions with parameters
- Reusable component types across multiple components

### 2. Environment Overrides

- Component-level overrides (resources)
- Trait-level overrides (size, storageClass)
- Environment-specific configuration

### 3. Trait Composition

- Creates new resources (PVC)
- Patches existing resources (Deployment volumes)
- Instance-specific trait configuration

### 4. Standard Labels

- Automatic labeling with project, component, and environment
- Consistent label structure across all resources

## Cleanup

```bash
# Delete all resources
kubectl delete -f samples/component-with-traits/component-with-traits.yaml
```

## Troubleshooting

### Release not created

```bash
# Check ComponentDeployment status
kubectl describe componentdeployment demo-app-development -n default

# Check controller logs
kubectl logs -n openchoreo-system deployment/openchoreo-controller
```

### Rendering errors

Look for errors in the ComponentDeployment conditions:

```bash
kubectl get componentdeployment demo-app-development -n default -o jsonpath='{.status.conditions}'
```
