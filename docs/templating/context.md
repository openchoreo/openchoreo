# Template Context Variables

This guide documents the context variables available in OpenChoreo templates. Different template locations have access to different context types.

## Overview

OpenChoreo uses two context types depending on where the template is evaluated:

| Context Type | Used In | Key Variables |
|--------------|---------|---------------|
| **ComponentContext** | ComponentType `resources` | `metadata`, `parameters`, `envOverrides`, `dataplane`, `workload`, `configurations` |
| **TraitContext** | Trait `creates` and `patches` | `metadata`, `parameters`, `envOverrides`, `dataplane`, `trait`, `workload`, `configurations` |

## ComponentContext

ComponentContext is used when rendering ComponentType resources. It provides access to component metadata, parameters (from Component), environment overrides (from ReleaseBinding), workload information, and configurations.

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

  # Generated resource naming (use these in all resource templates)
  name: "my-service-dev-a1b2c3d4"       # ${metadata.name} - use as prefix for all resource names to avoid conflicts between components
  namespace: "dp-acme-corp-dev-x1y2z3"  # ${metadata.namespace} - use for all namespaced resources to ensure components in a project share the same namespace per environment

  # Common labels for all resources
  labels:                               # ${metadata.labels}
    openchoreo.dev/component: "my-service"
    openchoreo.dev/project: "my-project"
    # ... other platform labels

  # Common annotations for all resources
  annotations: {}                       # ${metadata.annotations}

  # Pod selectors - use for selector.matchLabels, pod template labels, and service selectors
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

Component parameters from `Component.Spec.Parameters`, pruned to the ComponentType's `schema.parameters` section with defaults applied. Use for static configuration that doesn't change across environments.

```yaml
# Access pattern: ${parameters.<field>}

# Given this schema in ComponentType:
schema:
  parameters:
    replicas: "integer | default=1"
    port: "integer | default=8080"

# And this Component:
spec:
  parameters:
    replicas: 3

# The parameters context would be:
parameters:
  replicas: 3                    # ${parameters.replicas} (from Component)
  port: 8080                     # ${parameters.port} (default from schema)
```

**Example usage:**

```yaml
spec:
  replicas: ${parameters.replicas}
  template:
    spec:
      containers:
        - name: app
          ports:
            - containerPort: ${parameters.port}
```

### envOverrides

Environment-specific overrides from `ReleaseBinding.Spec.ComponentTypeEnvOverrides`, pruned to the ComponentType's `schema.envOverrides` section with defaults applied. Use for values that vary per environment (resources, replicas, etc.).

```yaml
# Access pattern: ${envOverrides.<field>}

# Given this schema in ComponentType:
schema:
  envOverrides:
    resources:
      $default: {}
      requests:
        $default: {}
        cpu: "string | default=100m"
        memory: "string | default=128Mi"
      limits:
        $default: {}
        cpu: "string | default=500m"
        memory: "string | default=512Mi"

# And this ReleaseBinding:
spec:
  componentTypeEnvOverrides:
    resources:
      requests:
        cpu: "200m"
        memory: "256Mi"

# The envOverrides context would be:
envOverrides:
  resources:
    requests:
      cpu: "200m"                # ${envOverrides.resources.requests.cpu} (from ReleaseBinding)
      memory: "256Mi"            # ${envOverrides.resources.requests.memory} (from ReleaseBinding)
    limits:
      cpu: "500m"                # ${envOverrides.resources.limits.cpu} (default)
      memory: "512Mi"            # ${envOverrides.resources.limits.memory} (default)
```

**Example usage:**

```yaml
spec:
  template:
    spec:
      containers:
        - name: app
          resources:
            requests:
              cpu: ${envOverrides.resources.requests.cpu}
              memory: ${envOverrides.resources.requests.memory}
            limits:
              cpu: ${envOverrides.resources.limits.cpu}
              memory: ${envOverrides.resources.limits.memory}
```

**Key difference from parameters:**
- `parameters`: Static values from Component - same across all environments
- `envOverrides`: Environment-specific values from ReleaseBinding - different per environment

### dataplane

DataPlane configuration for the target environment.

```yaml
# Access pattern: ${dataplane.<field>}

dataplane:
  secretStore: "my-secret-store"              # ${dataplane.secretStore}
  publicVirtualHost: "app.example.com"        # ${dataplane.publicVirtualHost}
  observabilityPlaneRef:                      # ${dataplane.observabilityPlaneRef}
    kind: "ObservabilityPlane"                # ${dataplane.observabilityPlaneRef.kind} - "ObservabilityPlane" or "ClusterObservabilityPlane"
    name: "my-obs-plane"                      # ${dataplane.observabilityPlaneRef.name}
```

**Optional fields:** `secretStore`, `publicVirtualHost`, and `observabilityPlaneRef` are optional. If not configured on the DataPlane, the field will be absent from the context. Use `has()` to guard conditional logic:

```yaml
# Guard with has() for conditional inclusion
includeWhen: ${has(dataplane.secretStore)}

# Or use ternary for conditional values
secretStoreRef: ${has(dataplane.secretStore) ? {"name": dataplane.secretStore} : oc_omit()}
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

Workload specification containing container and endpoint information from the build process.

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
  endpoints:
    http:                               # ${workload.endpoints.http}
      type: "HTTP"                      # ${workload.endpoints.http.type}
      port: 8080                        # ${workload.endpoints.http.port}
      schema:                           # ${workload.endpoints.http.schema} (optional)
        type: "openapi"
        content: "..."
    grpc:
      type: "gRPC"
      port: 9090
```

**Endpoint types:** HTTP, REST, gRPC, GraphQL, Websocket, TCP, UDP

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

**Iterating over endpoints:**

```yaml
# Using transformList to convert endpoints map to list of ports
ports: |
  ${workload.endpoints.transformList(name, ep, {
    "name": name,
    "port": ep.port,
    "protocol": ep.type == "UDP" ? "UDP" : "TCP"
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

### Configuration Helper Methods

The `configurations` object provides several helper methods to simplify working with container configurations, environment variables, and file mounts. These helpers reduce boilerplate and make templates more readable.

**Available Helper Methods:**

| Helper Method | Description |
|---------------|-------------|
| `configurations.toContainerEnvFrom(containerName)` | Generates `envFrom` array with configMapRef and secretRef for a container |
| `configurations.toConfigEnvsByContainer()` | Returns list of config environment variable list groups by container |
| `configurations.toSecretEnvsByContainer()` | Returns list of secret environment variable list groups by container |
| `configurations.toConfigFileList()` | Flattens all config files from all containers into a single list |
| `configurations.toSecretFileList()` | Flattens all secret files from all containers into a single list |
| `configurations.toContainerVolumeMounts(containerName)` | Generates volumeMounts array for a container's files |
| `configurations.toVolumes()` | Generates volumes array for all containers' files |

For detailed documentation, examples, and usage patterns for each helper method, see [Configuration Helpers](./configuration_helpers.md).

**Quick Example:**

```yaml
# Using helper methods for cleaner templates
spec:
  template:
    spec:
      containers:
        - name: main
          image: myapp:latest
          envFrom: ${configurations.toContainerEnvFrom("main")}
          volumeMounts: ${configurations.toContainerVolumeMounts("main")}
      volumes: ${configurations.toVolumes()}
```

## TraitContext

TraitContext is used when rendering Trait creates and patches. It provides access to metadata, trait-specific information, parameters (from trait instance), and environment overrides (from ReleaseBinding).

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

### dataplane

DataPlane configuration for the target environment. Same structure as ComponentContext. The fields `secretStore`, `publicVirtualHost`, and `observabilityPlaneRef` are optional; use `has()` to guard conditional logic.

```yaml
# Access pattern: ${dataplane.<field>}

dataplane:
  secretStore: "my-secret-store"              # ${dataplane.secretStore}
  publicVirtualHost: "app.example.com"        # ${dataplane.publicVirtualHost}
  observabilityPlaneRef:                      # ${dataplane.observabilityPlaneRef}
    kind: "ObservabilityPlane"                # ${dataplane.observabilityPlaneRef.kind} - "ObservabilityPlane" or "ClusterObservabilityPlane"
    name: "my-obs-plane"                      # ${dataplane.observabilityPlaneRef.name}
```

### parameters

Trait instance parameters from `Component.Spec.Traits[].Parameters`, pruned to the Trait's `schema.parameters` section with defaults applied. Use for static configuration that doesn't change across environments.

```yaml
# Given this schema in Trait:
schema:
  parameters:
    volumeName: "string"
    mountPath: "string"
    containerName: "string | default=app"

# And this trait instance in Component:
traits:
  - name: persistent-volume
    instanceName: data-storage
    parameters:
      volumeName: "app-data"
      mountPath: "/var/data"

# The parameters context would be:
parameters:
  volumeName: "app-data"         # ${parameters.volumeName} (from trait instance)
  mountPath: "/var/data"         # ${parameters.mountPath} (from trait instance)
  containerName: "app"           # ${parameters.containerName} (default)
```

### envOverrides

Environment-specific overrides from `ReleaseBinding.Spec.TraitOverrides[instanceName]`, pruned to the Trait's `schema.envOverrides` section with defaults applied. Use for values that vary per environment.

```yaml
# Given this schema in Trait:
schema:
  envOverrides:
    size: "string | default=10Gi"
    storageClass: "string | default=standard"

# And this ReleaseBinding:
spec:
  traitOverrides:
    data-storage:              # keyed by instanceName
      size: "50Gi"
      storageClass: "fast-ssd"

# The envOverrides context would be:
envOverrides:
  size: "50Gi"                   # ${envOverrides.size} (from ReleaseBinding)
  storageClass: "fast-ssd"       # ${envOverrides.storageClass} (from ReleaseBinding)
```

**Example usage:**

```yaml
# In Trait creates template
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: ${metadata.name}-${trait.instanceName}
spec:
  accessModes:
    - ReadWriteOnce
  resources:
    requests:
      storage: ${envOverrides.size}
  storageClassName: ${envOverrides.storageClass}
```

### workload

Workload specification containing container and endpoint information. Same structure as ComponentContext workload. See [workload](#workload) section above for full details.

```yaml
# Access pattern: ${workload.<field>}

workload:
  containers:
    app:
      image: "myregistry/myapp:v1.0"
      command: ["./start.sh"]
      args: ["--port", "8080"]
  endpoints:
    http:
      type: "HTTP"
      port: 8080
```

### configurations

Configuration items (environment variables and files) extracted from workload. Same structure as ComponentContext configurations. See [configurations](#configurations) section above for full details and helper methods.

```yaml
# Access pattern: ${configurations.<containerName>.<field>}

configurations:
  app:
    configs:
      envs: [...]
      files: [...]
    secrets:
      envs: [...]
      files: [...]
```

**Available helper methods** (same as ComponentContext):
- `configurations.toContainerEnvFrom(containerName)`
- `configurations.toConfigEnvsByContainer()`
- `configurations.toSecretEnvsByContainer()`
- `configurations.toConfigFileList()`
- `configurations.toSecretFileList()`
- `configurations.toContainerVolumeMounts(containerName)`
- `configurations.toVolumes()`

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

| Variable | ComponentContext | TraitContext |
|----------|------------------|--------------|
| `metadata.*` | ✅ | ✅ |
| `parameters.*` | ✅ (from Component.Spec.Parameters) | ✅ (from Trait instance) |
| `envOverrides.*` | ✅ (from ReleaseBinding.ComponentTypeEnvOverrides) | ✅ (from ReleaseBinding.TraitOverrides) |
| `dataplane.*` | ✅ | ✅ |
| `workload.*` | ✅ | ✅ |
| `configurations.*` | ✅ | ✅ |
| `trait.*` | ❌ | ✅ |
| Loop variable | ✅ (in forEach) | ✅ (in forEach) |
| `resource` | ❌ | ✅ (in where only) |
