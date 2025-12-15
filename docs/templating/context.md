# Template Context Variables

This guide documents the context variables available in OpenChoreo templates. Different template locations have access to different context types.

## Overview

OpenChoreo uses two context types depending on where the template is evaluated:

| Context Type | Used In | Key Variables |
|--------------|---------|---------------|
| **ComponentContext** | ComponentType `resources` | `metadata`, `parameters`, `dataplane`, `workload`, `configurations` |
| **TraitContext** | Trait `creates` and `patches` | `metadata`, `parameters`, `trait` |

## ComponentContext

ComponentContext is used when rendering ComponentType resources. It provides access to component metadata, parameters, workload information, and configurations.

### Available in ComponentType

| Location | Context | Notes |
|----------|---------|-------|
| `resources[].template` | ComponentContext | Full context |
| `resources[].includeWhen` | ComponentContext | Evaluated before forEach |
| `resources[].forEach` | ComponentContext | Expression to iterate over |
| Inside forEach iteration | ComponentContext + loop variable | Loop variable added to context |

### metadata

Platform-computed metadata for resource generation.

```yaml
# Access pattern: ${metadata.<field>}

metadata:
  # Component identity
  componentName: "my-service"           # ${metadata.componentName}
  componentUID: "a1b2c3d4-..."          # ${metadata.componentUID}

  # Project identity
  projectName: "my-project"             # ${metadata.projectName}
  projectUID: "b2c3d4e5-..."            # ${metadata.projectUID}

  # Environment identity
  environmentName: "production"         # ${metadata.environmentName}
  environmentUID: "d4e5f6g7-..."        # ${metadata.environmentUID}

  # DataPlane identity
  dataPlaneName: "my-dataplane"         # ${metadata.dataPlaneName}
  dataPlaneUID: "c3d4e5f6-..."          # ${metadata.dataPlaneUID}

  # Generated resource naming
  name: "my-service-dev-a1b2c3d4"       # ${metadata.name}
  namespace: "dp-acme-corp-dev-x1y2z3"  # ${metadata.namespace}

  # Common labels for all resources
  labels:                               # ${metadata.labels}
    openchoreo.dev/component: "my-service"
    openchoreo.dev/project: "my-project"
    # ... other platform labels

  # Common annotations for all resources
  annotations: {}                       # ${metadata.annotations}

  # Pod selectors for workload identity
  podSelectors:                         # ${metadata.podSelectors}
    openchoreo.dev/component-uid: "abc123"
    openchoreo.dev/environment-uid: "dev"
    openchoreo.dev/project-uid: "xyz789"
```

**Example usage:**

```yaml
# In ComponentType resource template
apiVersion: apps/v1
kind: Deployment
metadata:
  name: ${metadata.name}
  namespace: ${metadata.namespace}
  labels: ${metadata.labels}
spec:
  selector:
    matchLabels: ${metadata.podSelectors}
  template:
    metadata:
      labels: ${oc_merge(metadata.labels, metadata.podSelectors)}
```

### parameters

Merged component parameters with schema defaults applied. The structure depends on the ComponentType's schema definition.

```yaml
# Access pattern: ${parameters.<field>}

# Given this schema in ComponentType:
schema:
  parameters: |
    replicas: integer | default=1
    resources:
      cpu: string | default="100m"
      memory: string | default="128Mi"
    features:
      logging: boolean | default=true

# And this Component:
spec:
  parameters:
    replicas: 3
    resources:
      memory: "256Mi"

# The merged parameters context would be:
parameters:
  replicas: 3                    # ${parameters.replicas}
  resources:
    cpu: "100m"                  # ${parameters.resources.cpu} (default)
    memory: "256Mi"              # ${parameters.resources.memory} (overridden)
  features:
    logging: true                # ${parameters.features.logging} (default)
```

**Example usage:**

```yaml
spec:
  replicas: ${parameters.replicas}
  template:
    spec:
      containers:
        - name: app
          resources:
            requests:
              cpu: ${parameters.resources.cpu}
              memory: ${parameters.resources.memory}
```

### dataplane

DataPlane configuration for the target environment.

```yaml
# Access pattern: ${dataplane.<field>}

dataplane:
  secretStore: "my-secret-store"        # ${dataplane.secretStore}
  publicVirtualHost: "app.example.com"  # ${dataplane.publicVirtualHost}
```

**Example usage:**

```yaml
# Creating an ExternalSecret that references the dataplane's secret store
apiVersion: external-secrets.io/v1beta1
kind: ExternalSecret
metadata:
  name: ${metadata.name}
spec:
  secretStoreRef:
    name: ${dataplane.secretStore}
    kind: ClusterSecretStore
```

### workload

Workload specification containing container information from the build process.

```yaml
# Access patterns (both work):
#   ${workload.containers.app.image}        - dot notation
#   ${workload.containers["app"].image}     - bracket notation

workload:
  containers:
    app:                                # ${workload.containers.app}
      image: "myregistry/myapp:v1.0"    # ${workload.containers.app.image}
      command: ["./start.sh"]           # ${workload.containers.app.command}
      args: ["--port", "8080"]          # ${workload.containers.app.args}
    sidecar:
      image: "envoy:latest"
      command: []
      args: []
```

**Example usage:**

```yaml
spec:
  template:
    spec:
      containers:
        - name: app
          image: ${workload.containers.app.image}
          command: ${workload.containers.app.command}
          args: ${workload.containers.app.args}
```

**Iterating over containers:**

```yaml
# Using transformList to convert map to list
containers: |
  ${workload.containers.transformList(name, container, {
    "name": name,
    "image": container.image,
    "command": size(container.command) > 0 ? container.command : oc_omit(),
    "args": size(container.args) > 0 ? container.args : oc_omit()
  })}
```

### configurations

Configuration items (environment variables and files) extracted from workload, organized by container.

```yaml
# Access patterns (both work):
#   ${configurations.app.configs.envs}       - dot notation
#   ${configurations["app"].configs.envs}    - bracket notation
# Use bracket notation for dynamic keys: ${configurations[parameters.containerName].configs.envs}

configurations:
  app:                                  # ${configurations.app}
    configs:                            # ${configurations.app.configs}
      envs:                             # ${configurations.app.configs.envs}
        - name: "DATABASE_URL"
          value: "postgres://..."
        - name: "LOG_LEVEL"
          value: "info"
      files:                            # ${configurations.app.configs.files}
        - name: "config.yaml"
          mountPath: "/etc/app/config.yaml"
          value: "key: value\n..."
    secrets:                            # ${configurations.app.secrets}
      envs:                             # ${configurations.app.secrets.envs}
        - name: "API_KEY"
          remoteRef:
            key: "my-secret"
            property: "api-key"
      files:                            # ${configurations.app.secrets.files}
        - name: "credentials.json"
          mountPath: "/etc/app/credentials.json"
          remoteRef:
            key: "my-secret"
            property: "credentials"
```

**Structure details:**

| Field | Type | Description |
|-------|------|-------------|
| `configurations.<container>.configs.envs` | `[]EnvConfiguration` | Plain config environment variables |
| `configurations.<container>.configs.files` | `[]FileConfiguration` | Plain config files |
| `configurations.<container>.secrets.envs` | `[]EnvConfiguration` | Secret environment variables |
| `configurations.<container>.secrets.files` | `[]FileConfiguration` | Secret files |

**EnvConfiguration structure:**

```yaml
- name: "VAR_NAME"              # Environment variable name
  value: "plain-value"          # Plain text value (for configs)
  remoteRef:                    # Remote reference (for secrets)
    key: "secret-name"
    property: "secret-key"
    version: "v1"               # Optional
```

**FileConfiguration structure:**

```yaml
- name: "filename.txt"          # File name
  mountPath: "/path/to/file"    # Where to mount the file
  value: "file-contents"        # Plain text content (for configs)
  remoteRef:                    # Remote reference (for secrets)
    key: "secret-name"
    property: "secret-key"
```

**Example usage:**

```yaml
# Inject environment variables from configurations (dot notation)
env: |
  ${configurations.app.configs.envs.map(e, {
    "name": e.name,
    "value": e.value
  })}

# Create ConfigMap from config files
data: |
  ${configurations.app.configs.files.transformMapEntry(i, f, {f.name: f.value})}

# Using dynamic container name from parameters (bracket notation required)
envFrom: |
  ${has(configurations[parameters.containerName].configs.envs) &&
   configurations[parameters.containerName].configs.envs.size() > 0 ?
    [{"configMapRef": {"name": metadata.name + "-config"}}] : []}
```

## TraitContext

TraitContext is used when rendering Trait creates and patches. It provides access to metadata, trait-specific information, and trait parameters.

### Available in Traits

| Location | Context | Notes |
|----------|---------|-------|
| `creates[].template` | TraitContext | Full trait context |
| `patches[].operations[].path` | TraitContext | Path can contain expressions |
| `patches[].operations[].value` | TraitContext | Value can contain expressions |
| `patches[].forEach` | TraitContext | Expression to iterate over |
| Inside forEach iteration | TraitContext + loop variable | Loop variable added |
| `patches[].target.where` | TraitContext + `resource` | Special `resource` variable added |

### metadata

Same structure as ComponentContext metadata. See [metadata](#metadata) above.

### trait

Trait-specific metadata identifying the trait and its instance.

```yaml
# Access pattern: ${trait.<field>}

trait:
  name: "storage"                # ${trait.name} - Trait CRD name
  instanceName: "my-storage"     # ${trait.instanceName} - Instance name in Component
```

**Example usage:**

```yaml
# In Trait creates template
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: ${oc_generate_name(metadata.name, trait.instanceName)}
  labels:
    trait: ${trait.name}
    instance: ${trait.instanceName}
```

### parameters

Merged trait instance parameters with schema defaults applied. The structure depends on the Trait's schema definition.

```yaml
# Given this schema in Trait:
schema:
  parameters: |
    storageSize: string | default="1Gi"
    storageClass: string
    accessMode: string | default="ReadWriteOnce"

# And this trait instance in Component:
traits:
  - name: storage
    instanceName: my-storage
    parameters:
      storageSize: "10Gi"
      storageClass: "fast-ssd"

# The merged parameters context would be:
parameters:
  storageSize: "10Gi"            # ${parameters.storageSize}
  storageClass: "fast-ssd"       # ${parameters.storageClass}
  accessMode: "ReadWriteOnce"    # ${parameters.accessMode} (default)
```

**Note:** TraitContext does NOT have access to `workload`, `configurations`, or `dataplane`. These are only available in ComponentContext.

## Special Variables

### Loop Variables (forEach)

When using `forEach`, the loop variable is added to the context for each iteration.

```yaml
# ComponentType resource with forEach
resources:
  - forEach: ${parameters.ports}
    var: port                    # Loop variable name (defaults to "item")
    resource:
      apiVersion: v1
      kind: Service
      metadata:
        name: ${oc_generate_name(metadata.name, port.name)}
      spec:
        ports:
          - port: ${port.port}   # Access loop variable
            name: ${port.name}

# Trait patch with forEach
patches:
  - forEach: ${parameters.volumes}
    var: vol
    target:
      kind: Deployment
    operations:
      - op: add
        path: /spec/template/spec/volumes/-
        value:
          name: ${vol.name}
          persistentVolumeClaim:
            claimName: ${vol.claimName}
```

### resource Variable (where clause)

In trait patch `where` clauses, the `resource` variable provides access to the target resource being evaluated.

```yaml
patches:
  - target:
      kind: Deployment
      # Filter to only patch deployments with specific label
      where: ${resource.metadata.labels["app.kubernetes.io/component"] == "backend"}
    operations:
      - op: add
        path: /spec/template/spec/containers/0/env/-
        value:
          name: BACKEND_MODE
          value: "true"

  - target:
      kind: Service
      # Filter based on service type
      where: ${resource.spec.type == "LoadBalancer"}
    operations:
      - op: add
        path: /metadata/annotations/service.beta.kubernetes.io~1aws-load-balancer-type
        value: "nlb"
```

**Available in `resource`:**

The entire rendered Kubernetes resource is available, including:
- `resource.apiVersion`
- `resource.kind`
- `resource.metadata` (name, namespace, labels, annotations, etc.)
- `resource.spec` (resource-specific specification)

## Context Comparison

| Variable | ComponentContext | TraitContext            |
|----------|------------------|-------------------------|
| `metadata.*` | ✅ | ✅                       |
| `parameters.*` | ✅ (from Component) | ✅ (from Trait instance) |
| `dataplane.*` | ✅ | ❌ (will be added)       |
| `workload.*` | ✅ | ❌                       |
| `configurations.*` | ✅ | ❌                       |
| `trait.*` | ❌ | ✅                       |
| Loop variable | ✅ (in forEach) | ✅ (in forEach)          |
| `resource` | ❌ | ✅ (in where only)       |
