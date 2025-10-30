# Component with Addons Example

This example demonstrates how to use ComponentTypeDefinitions and Addons to create extensible, composable components.

## Resources

- **component-with-addons.yaml**: Contains all resources needed for this example:
  - ComponentTypeDefinition: Defines a simple deployment component type with configurable replicas and resources
  - Addon: Defines a PVC addon that creates a PersistentVolumeClaim and mounts it to the deployment
  - Workload: The workload spec (normally created by the build process)
  - Component: A component instance using the simple-deployment type with the PVC addon
  - ComponentDeployment: Environment-specific settings that override default values for the development environment

## Key Features

### Inline Schema Format

The schemas use an inline format for parameter definitions:

```yaml
schema:
  parameters:
    volumeName: "string | required=true"
    mountPath: "string | required=true"
    containerName: "string"
  envOverrides:
    size: "string | default=10Gi"
    storageClass: "string | default=standard"
```

Format: `"type | default=value | required=true | enum=val1,val2"`

### CEL Template Expressions

Templates use CEL expressions enclosed in `${...}`:

```yaml
template:
  metadata:
    name: ${metadata.name}
  spec:
    replicas: ${spec.replicas}
```

### Addon Composition

Addons can:

- **Create** new resources (like PVCs)
- **Patch** existing resources using JSONPatch operations

## Applying the Example

```bash
# Apply all resources
kubectl apply -f samples/component-with-addons/component-with-addons.yaml

# Check the generated resources
kubectl get componentenvsnapshot -n default
kubectl get release -n default

# View the snapshot details (contains exact copies of all resources)
kubectl get componentenvsnapshot test-service-development -n default -o yaml

# View the release details (contains the resources to be deployed)
kubectl get release test-service-development -n default -o yaml

# Check the sample ConfigMap in the Release
kubectl get release test-service-development -n default -o jsonpath='{.spec.resources[0]}' | jq '.'

# Note: The ConfigMap is embedded in the Release but not yet applied to the cluster.
# The Release controller will apply these resources in a future implementation.
```

## What Happens

1. **Component Controller**:

   - Detects the component uses `componentType: deployment/simple-deployment`
   - Fetches the ComponentTypeDefinition, Addon, and Workload
   - Creates a `ComponentEnvSnapshot` with exact copies of all resources
   - The snapshot preserves the inline schema format from the original definitions

2. **ComponentDeployment Controller**:
   - Finds the corresponding ComponentEnvSnapshot
   - Creates a `Release` resource containing the Kubernetes resources to be deployed
   - The Release contains a sample ConfigMap embedded in `spec.resources`
