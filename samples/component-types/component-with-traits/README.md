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

Defines a `web-app` component type that creates:

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

### ReleaseBinding

Defines deployment settings for the `development` environment:

- Reduces resource requests/limits for cost savings
- Changes PVC size from 20Gi → 5Gi (via trait overrides)
- Changes storage class from "fast" → "standard" (via trait overrides)

## Expected Output

The rendered resources will include:

- **Deployment**: `demo-app-with-traits-development-<hash>` with nginx container and mounted volumes
- **Service**: `demo-app-with-traits-development-<hash>` exposing the deployment
- **PersistentVolumeClaim**: `demo-app-with-traits-development-<hash>-data-storage` for persistent storage

All resources will have:

- Computed names and namespaces based on project, component, and environment
- Standard labels for project, component, and environment
- Environment-specific overrides applied (reduced resources, smaller PVC size)

## Testing on Cluster

### Step 1: Apply resources

```bash
kubectl apply -f component-with-traits.yaml
```

### Step 3: Verify ReleaseBinding status

```bash
kubectl get releasebinding demo-app-with-traits-development -o yaml | grep -A 50 "^status:"
```


## Expected Rendering

The rendered resources will include:

### 1. Deployment

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: demo-app-with-traits-development-<hash>
  namespace: dp-default-default-development-<hash>
  labels:
    openchoreo.org/component: demo-app-with-traits
    openchoreo.org/environment: development
spec:
  replicas: 2
  selector:
    matchLabels:
      openchoreo.org/component: demo-app-with-traits
      openchoreo.org/environment: development
      openchoreo.org/project: default
  template:
    spec:
      containers:
        - name: app
          image: nginx:1.25-alpine
          resources:
            requests:
              cpu: 50m # Overridden by ReleaseBinding
              memory: 128Mi # Overridden by ReleaseBinding
            limits:
              cpu: 200m # Overridden by ReleaseBinding
              memory: 256Mi # Overridden by ReleaseBinding
          volumeMounts: # Added by traits
            - name: app-data
              mountPath: /var/data
            - name: cache-vol
              mountPath: /tmp/cache
            - name: workspace-vol
              mountPath: /tmp/work
      volumes: # Added by traits
        - name: app-data
          persistentVolumeClaim:
            claimName: demo-app-with-traits-development-<hash>-data-storage
        - name: cache-vol
          emptyDir: {}
        - name: workspace-vol
          emptyDir:
            sizeLimit: 1Gi
```

### 2. Service

```yaml
apiVersion: v1
kind: Service
metadata:
  name: demo-app-with-traits-development-<hash>
  namespace: dp-default-default-development-<hash>
  labels:
    openchoreo.org/component: demo-app-with-traits
    openchoreo.org/environment: development
spec:
  type: ClusterIP
  selector:
    openchoreo.org/component: demo-app-with-traits
    openchoreo.org/environment: development
    openchoreo.org/project: default
  ports:
    - name: http
      port: 80
      targetPort: 8080
```

### 3. PersistentVolumeClaim (created by persistent-volume trait)

```yaml
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: demo-app-with-traits-development-<hash>-data-storage
  namespace: dp-default-default-development-<hash>
  labels:
    openchoreo.org/component: demo-app-with-traits
    openchoreo.org/environment: development
spec:
  accessModes:
    - ReadWriteOnce
  resources:
    requests:
      storage: 5Gi # Overridden by ReleaseBinding (was 20Gi)
  storageClassName: standard # Overridden by ReleaseBinding (was "fast")
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

Remove all resources:

```bash
kubectl delete -f component-with-traits.yaml
```

## Troubleshooting

If the application is not accessible or resources are not created:

1. **Check ReleaseBinding status:**
   ```bash
   kubectl get releasebinding demo-app-with-traits-development -o yaml
   ```

2. **Check ReleaseBinding conditions:**
   ```bash
   kubectl get releasebinding demo-app-with-traits-development -o jsonpath='{.status.conditions}' | jq .
   ```

3. **Verify HTTPRoute is configured:**
   ```bash
   kubectl get httproute -A -l openchoreo.org/component=demo-app-with-traits -o yaml
   ```

4. **Check deployment status:**
   ```bash
   kubectl get deployment -A -l openchoreo.org/component=demo-app-with-traits
   ```

5. **Check pod logs:**
   ```bash
   kubectl logs -n $(kubectl get pods -A -l openchoreo.org/component=demo-app-with-traits -o jsonpath='{.items[0].metadata.namespace}') -l openchoreo.org/component=demo-app-with-traits --tail=50
   ```

6. **Verify service endpoints:**
   ```bash
   kubectl get service -A -l openchoreo.org/component=demo-app-with-traits
   ```

7. **Verify PersistentVolumeClaim (trait-specific):**
   ```bash
   kubectl get pvc -A -l openchoreo.org/component=demo-app-with-traits
   ```
