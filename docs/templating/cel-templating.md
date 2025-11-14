# CEL Templating

This guide covers the CEL (Common Expression Language) templating system used in OpenChoreo for dynamic resource generation.

## Overview

CEL templating enables dynamic configuration through expressions embedded in YAML/JSON structures. Expressions are enclosed in `${}` and can reference context variables, perform computations, and conditionally include or transform data.

The templating engine is generic and can be used with any context variables. This guide demonstrates the engine capabilities using examples from ComponentTypes and Traits.

## Expression Types

### 1. Standalone Expressions

Standalone expressions return native CEL types and preserve the original data type:

```yaml
# Returns an integer
replicas: ${parameters.replicas}

# Returns a map
labels: ${metadata.labels}

# Boolean - note: ternary operators need quotes in YAML
enabled: "${has(parameters.feature) ? parameters.feature : false}"

# Returns a list
volumes: ${parameters.volumes}
```

### 2. Interpolated Expressions

When embedded within strings, expressions are automatically converted to strings:

```yaml
# String interpolation
message: "Application ${metadata.name} has ${parameters.replicas} replicas"

# URL construction
url: "https://${metadata.name}.${metadata.namespace}.svc.cluster.local:${parameters.port}"

# Image tag
image: "${parameters.registry}/${parameters.repository}:${parameters.tag}"
```

### 3. Dynamic Map Keys

Map keys can be dynamically generated using expressions (must evaluate to strings):

```yaml
# Dynamic service port mapping
services:
  ${metadata.serviceName}: 8080
  ${metadata.name + "-metrics"}: 9090

# Dynamic labels
labels:
  ${'app.kubernetes.io/' + metadata.name}: active
  ${parameters.labelPrefix + '/version'}: ${parameters.version}
```

## Built-in Functions

### oc_omit()
Remove fields from output when used as a value. Works for both top-level and nested map keys:

```yaml
# Conditionally include field
resources:
  limits:
    memory: ${parameters.memoryLimit}
    cpu: ${has(parameters.cpuLimit) ? parameters.cpuLimit : oc_omit()}

# Omit entire nested maps conditionally
metadata:
  name: ${metadata.name}
  annotations: ${has(parameters.annotations) ? parameters.annotations : oc_omit()}
  labels: ${size(parameters.labels) > 0 ? parameters.labels : oc_omit()}

# Omit nested map keys dynamically
# Note: Complex nested expressions work best as standalone CEL at the field level
container:
  image: ${parameters.image}
  resources:
    limits:
      memory: ${parameters.memoryLimit}
      cpu: ${parameters.cpuLimit}
    requests:
      memory: ${parameters.memoryRequest}
      cpu: ${has(parameters.cpuRequest) ? parameters.cpuRequest : oc_omit()}

# Omit in arrays
volumeMounts: ${[
  {"name": "data", "mountPath": "/data"},
  has(parameters.configPath) ? {"name": "config", "mountPath": parameters.configPath} : oc_omit(),
  parameters.enableSecrets ? {"name": "secrets", "mountPath": "/secrets"} : oc_omit()
]}
```

### oc_merge(base, override, ...)
Shallow merge two or more maps (later maps override earlier ones):

```yaml
# Merge default and custom labels
labels: ${oc_merge({"app": metadata.name, "version": "v1"}, parameters.customLabels)}

# Merge environment variables
env: ${oc_merge(parameters.defaultEnv, parameters.environmentEnv)}

# Merge multiple maps
config: ${oc_merge(defaults, layer1, layer2, layer3)}
```

### oc_generate_name(...args)
Convert arguments to valid Kubernetes resource names with a hash suffix for uniqueness:

```yaml
# Create valid ConfigMap name with hash
name: ${oc_generate_name(metadata.name, "config", parameters.environment)}
# Result: "myapp-config-prod-a1b2c3d4" (lowercase, alphanumeric, hyphens + 8-char hash)

# Handle special characters and add hash
name: ${oc_generate_name("My_App", "Service!")}
# Result: "my-app-service-e5f6g7h8"

# Single argument also gets hash
name: ${oc_generate_name("Hello World!")}
# Result: "hello-world-7f83b165"
```

**Note:** The function always appends an 8-character hash suffix to ensure uniqueness. The hash is generated from the original input values, so the same inputs will always produce the same output.

### Advanced oc_omit() Patterns

Complex uses of oc_omit() for conditional field inclusion:

```yaml
# Conditionally omit fields at different levels
deployment:
  metadata:
    name: ${metadata.name}
    namespace: ${metadata.namespace}
    annotations: ${has(parameters.annotations) ? parameters.annotations : oc_omit()}
  spec:
    replicas: ${parameters.replicas}
    strategy: ${has(parameters.strategy) ? parameters.strategy : oc_omit()}
    selector:
      matchLabels: ${metadata.podSelectors}
    template:
      spec:
        nodeSelector: ${has(parameters.nodeSelector) ? parameters.nodeSelector : oc_omit()}
        tolerations: ${has(parameters.tolerations) ? parameters.tolerations : oc_omit()}
        affinity: ${has(parameters.affinity) ? parameters.affinity : oc_omit()}

# Build map with conditional keys using oc_merge
metadata:
  labels: ${oc_merge(
    {"app": metadata.name},
    has(parameters.version) ? {"version": parameters.version} : {},
    parameters.environment != "prod" ? {"env": parameters.environment} : {}
  )}

# Omit empty/null values in environment variables
env: ${parameters.envVars.filter(e, e.value != null && e.value != "").map(e, {
  "name": e.name,
  "value": string(e.value),
  "valueFrom": has(e.valueFrom) ? e.valueFrom : oc_omit()
})}

# Volume mounts with conditional fields
volumeMounts: ${parameters.mounts.map(m, {
  "name": m.name,
  "mountPath": m.path,
  "subPath": has(m.subPath) && m.subPath != "" ? m.subPath : oc_omit(),
  "readOnly": has(m.readOnly) && m.readOnly ? true : oc_omit(),
  "mountPropagation": has(m.propagation) ? m.propagation : oc_omit()
})}

# Service ports with optional fields
ports: ${parameters.ports.map(p, {
  "name": has(p.name) ? p.name : oc_omit(),
  "port": p.port,
  "targetPort": has(p.targetPort) ? p.targetPort : p.port,
  "protocol": has(p.protocol) && p.protocol != "TCP" ? p.protocol : oc_omit(),
  "nodePort": has(p.nodePort) ? p.nodePort : oc_omit()
})}
```

## Common Patterns

### Conditional Fields with Defaults

```yaml
# Service type with default
serviceType: ${has(parameters.serviceType) ? parameters.serviceType : "ClusterIP"}

# Replicas with minimum
replicas: ${parameters.replicas > 0 ? parameters.replicas : 1}

# Optional annotation with oc_omit
annotations:
  monitoring: ${has(parameters.enableMonitoring) && parameters.enableMonitoring ? "enabled" : oc_omit()}
  version: ${has(parameters.version) ? parameters.version : oc_omit()}
```

### Array Transformations

```yaml
# Transform list of key-value pairs to environment variables
env: ${parameters.envVars.map(e, {"name": e.key, "value": e.value})}

# Filter and transform
ports: ${parameters.services.filter(s, s.enabled).map(s, {"port": s.port, "name": s.name})}

# Convert list to numbered items
items: ${range(0, size(parameters.items)).map(i, {"index": i, "value": parameters.items[i]})}

# Access container images from workload metadata
containers:
- name: "app"
  image: ${workload.containers["app"].image}

# Dynamic container generation from workload
containers: ${workload.containers.transformMapEntry(name, container, {
  "name": name,
  "image": container.image,
  "ports": has(container.ports) ? container.ports : oc_omit(),
  "env": has(container.env) ? container.env : oc_omit()
})}
```

### Safe Navigation with Optional Types

```yaml
# Optional chaining with ?
customValue: ${parameters.?custom.?value.orValue("default")}

# Map with optional keys
config:
  required: ${parameters.requiredConfig}
  ?"optional": ${parameters.?optionalConfig}  # Key included only if value exists

# Annotations with safe access
annotations:
  app: ${metadata.name}
  ?"custom": ${metadata.?annotations.?customAnnotation}
```

### Complex Conditionals

```yaml
# Multi-condition logic
nodeSelector: |
  ${parameters.highPerformance ?
    {"node-type": "compute-optimized"} :
    (parameters.costOptimized ?
      {"node-type": "spot"} :
      {"node-type": "general-purpose"})}

# Conditional resource limits
resources: |
  ${parameters.resourceTier == "small" ? {
      "limits": {"memory": "512Mi", "cpu": "500m"},
      "requests": {"memory": "256Mi", "cpu": "250m"}
    } : (parameters.resourceTier == "medium" ? {
      "limits": {"memory": "2Gi", "cpu": "1000m"},
      "requests": {"memory": "1Gi", "cpu": "500m"}
    } : {
      "limits": {"memory": "4Gi", "cpu": "2000m"},
      "requests": {"memory": "2Gi", "cpu": "1000m"}
    })}
```

### Map and List Comprehensions

```yaml
# Transform list to map with dynamic keys
envMap: ${parameters.envVars.transformMapEntry(i, v, {v.name: v.value})}

# Map transformation
labelMap: ${parameters.labels.transformMap(k, v, {"app/" + k: v})}

# Filtered map (filter first, then transform)
activeServices: |
  ${parameters.services.filter(s, s.enabled)
    .transformMapEntry(i, s, {s.name: s.port})}
```

## Advanced CEL Features

### String Extensions
```yaml
# String manipulation
uppercaseName: ${metadata.name.upperAscii()}
trimmedValue: ${parameters.value.trim()}
replaced: ${parameters.text.replace("old", "new")}
prefixed: ${parameters.value.startsWith("prefix")}
```

### List Extensions
```yaml
# List operations
firstItem: ${parameters.items[0]}
lastItem: ${parameters.items[size(parameters.items) - 1]}
sortedItems: ${parameters.items.sort()}
uniqueItems: ${sets.unique(parameters.items)}
joined: ${parameters.items.join(",")}
```

### Math Extensions
```yaml
# Mathematical operations
maxValue: ${math.greatest([parameters.min, parameters.max, parameters.default])}
minValue: ${math.least([parameters.v1, parameters.v2, parameters.v3])}
rounded: ${math.ceil(parameters.floatValue)}
```

## Best Practices

### 1. Use Type-Appropriate Expressions

```yaml
# Good: Preserves integer type
replicas: ${parameters.replicas}

# Bad: Converts to string unnecessarily
replicas: "${parameters.replicas}"
```

### 2. Provide Defaults for Optional Fields

```yaml
# Good: Safe with default
serviceType: ${has(parameters.serviceType) ? parameters.serviceType : "ClusterIP"}

# Bad: May cause errors if missing
serviceType: ${parameters.serviceType}
```

### 3. Use Safe Navigation for Nested Access

```yaml
# Good: Won't error if path doesn't exist
value: ${parameters.?config.?nested.?value.orValue("default")}

# Bad: Errors if any part is missing
value: ${parameters.config.nested.value}
```

### 4. Leverage Helper Functions

```yaml
# Good: Ensures valid Kubernetes names with hash for uniqueness
name: ${oc_generate_name(metadata.name, parameters.suffix)}
# Result: "myapp-prod-a1b2c3d4"

# Bad: May produce invalid names or conflicts
name: ${metadata.name + "-" + parameters.suffix}
```

### 5. Keep Complex Logic Readable

```yaml
# Good: Clear multi-line formatting
resources: |
  ${parameters.tier == "small" ? {
      "limits": {"memory": "512Mi"},
      "requests": {"memory": "256Mi"}
    } : {
      "limits": {"memory": "2Gi"},
      "requests": {"memory": "1Gi"}
    }}

# Bad: Hard to read single line
resources: ${parameters.tier == "small" ? {"limits": {"memory": "512Mi"}, "requests": {"memory": "256Mi"}} : {"limits": {"memory": "2Gi"}, "requests": {"memory": "1Gi"}}}
```

## Common Errors and Solutions

### Error: "no such key"
**Cause**: Accessing non-existent field without safety checks
**Solution**: Use has() or optional types
```yaml
# Instead of:
value: ${parameters.optional}

# Use:
value: ${has(parameters.optional) ? parameters.optional : "default"}
# Or:
value: ${parameters.?optional.orValue("default")}
```

### Error: "type 'int' does not support field selection"
**Cause**: Trying to access fields on primitive types
**Solution**: Check type before access
```yaml
# Instead of:
value: ${parameters.count.value}

# Use:
value: ${type(parameters.count) == "int" ? parameters.count : parameters.count.value}
```

### Error: "cannot convert string to int"
**Cause**: Type mismatch in expressions
**Solution**: Use explicit conversion
```yaml
# Instead of:
replicas: ${parameters.replicaString}

# Use:
replicas: ${int(parameters.replicaString)}
```

## OpenChoreo Resource Control Fields

OpenChoreo extends the templating system with special fields for resource generation in ComponentTypes and Traits:

### includeWhen - Conditional Resource Inclusion

The `includeWhen` field controls whether a resource is included in the output based on a CEL expression:

```yaml
# In ComponentType
resources:
  # Only create HPA if auto-scaling is enabled
  - includeWhen: ${parameters.autoscaling.enabled}
    resource:
      apiVersion: autoscaling/v2
      kind: HorizontalPodAutoscaler
      metadata:
        name: ${metadata.name}
      spec:
        scaleTargetRef:
          apiVersion: apps/v1
          kind: Deployment
          name: ${metadata.name}
        minReplicas: ${parameters.autoscaling.minReplicas}
        maxReplicas: ${parameters.autoscaling.maxReplicas}

  # Create PDB only for production with multiple replicas
  - includeWhen: ${parameters.environment == "production" && parameters.replicas > 1}
    resource:
      apiVersion: policy/v1
      kind: PodDisruptionBudget
      metadata:
        name: ${metadata.name}
      spec:
        minAvailable: ${parameters.replicas - 1}
        selector:
          matchLabels: ${metadata.podSelectors}
```

### forEach - Dynamic Resource Generation

The `forEach` field generates multiple resources from a list or map:

```yaml
resources:
  # Generate ConfigMaps for each database
  - forEach: ${parameters.databases}
    var: db
    resource:
      apiVersion: v1
      kind: ConfigMap
      metadata:
        name: ${oc_generate_name(metadata.name, db.name, "config")}
      data:
        host: ${db.host}
        port: ${string(db.port)}
        database: ${db.database}

  # Create Services for each exposed port
  - forEach: ${parameters.exposedPorts}
    var: portConfig
    includeWhen: ${portConfig.expose}  # Can combine with includeWhen
    resource:
      apiVersion: v1
      kind: Service
      metadata:
        name: ${oc_generate_name(metadata.name, portConfig.name)}
      spec:
        selector: ${metadata.podSelectors}
        ports:
        - port: ${portConfig.port}
          targetPort: ${portConfig.targetPort}
          name: ${portConfig.name}
```

### Combining forEach with includeWhen

```yaml
resources:
  # Generate secrets only for enabled integrations
  - forEach: ${parameters.integrations}
    var: integration
    includeWhen: ${integration.enabled && has(integration.credentials)}
    resource:
      apiVersion: v1
      kind: Secret
      metadata:
        name: ${oc_generate_name(metadata.name, integration.name, "secret")}
      type: Opaque
      stringData:
        api_key: ${integration.credentials.apiKey}
        api_secret: ${integration.credentials.apiSecret}
```

### Using forEach with Maps

```yaml
resources:
  # Generate ConfigMap from map entries
  - forEach: ${parameters.configFiles}
    var: config
    resource:
      apiVersion: v1
      kind: ConfigMap
      metadata:
        name: ${oc_generate_name(metadata.name, config.key)}
      data:
        "${config.key}": ${config.value}
```

### Usage in Traits

These fields work the same way in Trait `creates` and `patches`:

```yaml
# In Trait definition
apiVersion: v1alpha1
kind: Trait
metadata:
  name: monitoring
spec:
  creates:
    # Conditionally create ServiceMonitor
    - includeWhen: ${parameters.monitoring.enabled}
      resource:
        apiVersion: monitoring.coreos.com/v1
        kind: ServiceMonitor
        metadata:
          name: ${metadata.name}
        spec:
          selector:
            matchLabels: ${metadata.podSelectors}

  patches:
    # Conditionally patch deployment
    - target:
        group: apps
        version: v1
        kind: Deployment
      includeWhen: ${parameters.monitoring.enabled}
      operations:
        - op: mergeShallow
          path: /spec/template/metadata/annotations
          value:
            prometheus.io/scrape: "true"
```

## See Also

- [Compact Schema](./compact-schema.md) - Define parameter schemas
- [Patching](./patching.md) - Modify resources with patches
- CEL Language Specification: https://github.com/google/cel-spec
