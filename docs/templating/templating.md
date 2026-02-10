# Templating

This guide covers the OpenChoreo templating system for dynamic resource generation.

## Overview

OpenChoreo's templating system enables dynamic configuration through expressions embedded in YAML/JSON structures. Expressions are enclosed in `${}` and evaluated using CEL (Common Expression Language). The templating format, helper functions, and YAML integration are OpenChoreo-specific, while CEL is used only for expression evaluation within `${}`.

Key components:
- **Templating Format**: OpenChoreo-specific YAML/JSON structure with embedded expressions
- **Expression Language**: CEL (Common Expression Language) for evaluating expressions inside `${}`
- **Helper Functions**: OpenChoreo-provided functions like `oc_omit()`, `oc_merge()`, and `oc_generate_name()`

The templating engine is generic and can be used with any context variables. This guide demonstrates the engine capabilities using examples from ComponentTypes and Traits.

## Where Expressions Can Be Used

OpenChoreo templates support expressions in three locations:

### 1. Expression as Entire Value (Standalone)

When an expression is the entire value, it returns native CEL types and preserves the original data type.

**YAML Syntax Options:**
```yaml
# Without quotes (simplest)
replicas: ${parameters.replicas}

# With quotes (same result, useful for YAML compatibility)
replicas: "${parameters.replicas}"

# With block scalar | (useful to avoid escaping issues with complex expressions)
replicas: |
  ${parameters.replicas > 0 ? parameters.replicas : 1}
```

**Examples:**
```yaml
# Returns an integer
replicas: ${parameters.replicas}

# Returns a map
labels: ${metadata.labels}

# Returns a boolean
enabled: ${has(parameters.feature) ? parameters.feature : false}

# Returns a list
volumes: ${parameters.volumes}

# Complex expression with block scalar (avoids escaping quotes in YAML)
nodeSelector: |
  ${parameters.highPerformance ? {"node-type": "compute"} : {"node-type": "standard"}}
```

### 2. Expression as Part of String Value (Interpolated)

When an expression is embedded within a string, it is automatically converted to a string and interpolated.

```yaml
# String interpolation - multiple expressions
message: "Application ${metadata.name} has ${parameters.replicas} replicas"

# URL construction
url: "https://${metadata.name}.${metadata.namespace}.svc.cluster.local:${parameters.port}"

# Image tag
image: "${parameters.registry}/${parameters.repository}:${parameters.tag}"

# Note: Single expression in quotes is still standalone (not interpolated)
# These are equivalent - both preserve integer type:
replicas: ${parameters.replicas}
replicas: "${parameters.replicas}"
```

### 3. Expression as Map Key (Dynamic Keys)

Map keys can be dynamically generated using expressions (must evaluate to strings).

```yaml
# Dynamic service port mapping
services:
  ${metadata.serviceName}: 8080
  ${metadata.name + "-metrics"}: 9090

# Dynamic labels with concatenation
labels:
  ${'app.kubernetes.io/' + metadata.name}: active
  ${parameters.labelPrefix + '/version'}: ${parameters.version}
```

## CEL Capabilities

### Map Access Notation

Both dot notation and bracket notation work for accessing map fields:

```yaml
# These are equivalent for static keys:
${workload.containers.app.image}
${workload.containers["app"].image}
```

Bracket notation is **required** for:
- **Dynamic keys** (variables): `${configurations[parameters.containerName].configs.envs}`
- **Keys with special characters**: `${resource.metadata.labels["app.kubernetes.io/name"]}`
- **Optional dynamic keys**: `${configurations[?containerName].?configs.orValue({})}`

### Membership and Existence Checks

The `in` operator checks membership in maps and lists:

```
# Check if a key exists in a map
parameters.endpointName in workload.endpoints

# Check if a value exists in a list (primitives only — strings, ints, bools)
"HTTP" in parameters.allowedTypes
```

For maps, `exists()` and `all()` iterate over **keys** — use bracket notation to access values:

```
# Check if any endpoint is of type HTTP
workload.endpoints.exists(name, workload.endpoints[name].type == 'HTTP')

# Check all endpoints have a port > 0
workload.endpoints.all(name, workload.endpoints[name].port > 0)
```

### Conditional Logic

```yaml
# Service type with default
serviceType: ${has(parameters.serviceType) ? parameters.serviceType : "ClusterIP"}

# Replicas with minimum
replicas: ${parameters.replicas > 0 ? parameters.replicas : 1}

# Conditional string value
status: ${parameters.enabled ? "active" : "inactive"}

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

### Safe Navigation

```yaml
# Optional chaining with ? for static keys
customValue: ${parameters.?custom.?value.orValue("default")}

# Optional index access with dynamic keys
containerConfig: ${configurations[?containerName].?configs.?envs.orValue([])}

# Map with optional keys (entire map must be inside CEL expression)
config: |
  ${{"required": parameters.requiredConfig, ?"optional": parameters.?optionalConfig}}
```

### Array and List Operations

```yaml
# Transform list of key-value pairs to environment variables
env: |
  ${parameters.envVars.map(e, {"name": e.key, "value": e.value})}

# Filter and transform
ports: |
  ${parameters.services.filter(s, s.enabled).map(s, {"port": s.port, "name": s.name})}

# List operations
firstItem: ${parameters.items[0]}
lastItem: ${parameters.items[size(parameters.items) - 1]}
joined: ${parameters.items.join(",")}

# Sorting lists
sortedStrings: ${parameters.names.sort()}                        # sort primitive lists (strings, ints, etc.)
sortedByName: ${parameters.items.sortBy(item, item.name)}        # sort object lists by a field

# List concatenation
combined: ${parameters.list1 + parameters.list2}
withInlineItem: ${parameters.userPorts + [{"port": 8080, "name": "http"}]}

# Flatten nested lists
flattened: ${[[1, 2], [3, 4]].flatten()}  # returns [1, 2, 3, 4]

# Flatten with depth limit
partial: ${[[1, [2, 3]], [4]].flatten(1)}  # returns [1, [2, 3], 4]

# Flatten combined with transformList - useful for iterating over all items
# across multiple containers/groups
allFiles: |
  ${configurations.transformList(containerName, cfg,
    has(cfg.files) ? cfg.files.map(f, {
      "container": containerName,
      "name": f.name
    }) : []
  ).flatten()}
```

### Map Operations

```yaml
# Transform map to list using transformList
containerList: |
  ${workload.containers.transformList(name, container, {
    "name": name,
    "image": container.image
  })}

# Transform list to map with dynamic keys
envMap: |
  ${parameters.envVars.transformMapEntry(i, v, {v.name: v.value})}

# Map transformation (map to map)
labelMap: |
  ${parameters.labels.transformMap(k, v, {"app/" + k: v})}

# Filtered map (filter first, then transform to map)
activeServices: |
  ${parameters.services.filter(s, s.enabled)
    .transformMapEntry(i, s, {s.name: s.port})}
```

### String Operations

```yaml
# String manipulation
uppercaseName: ${metadata.name.upperAscii()}
trimmedValue: ${parameters.value.trim()}
replaced: ${parameters.text.replace("old", "new")}
prefixed: ${parameters.value.startsWith("prefix")}

# Split string into list
parts: ${parameters.path.split("/")}                    # "/a/b/c" → ["", "a", "b", "c"]
limited: ${parameters.text.split(",", 2)}               # "a,b,c" → ["a", "b,c"]

# Extract substring
suffix: ${parameters.name.substring(4)}                 # "hello-world" → "o-world"
middle: ${parameters.name.substring(0, 5)}              # "hello-world" → "hello"
```

### Math Operations

```yaml
# Mathematical operations
maxValue: ${math.greatest([parameters.min, parameters.max, parameters.default])}
minValue: ${math.least([parameters.v1, parameters.v2, parameters.v3])}
rounded: ${math.ceil(parameters.floatValue)}
```

### Encoding Operations

```yaml
# Base64 encode a string (must convert to bytes first)
encoded: ${base64.encode(bytes(parameters.value))}

# Base64 decode to bytes, then convert to string
decoded: ${string(base64.decode(parameters.encodedValue))}
```

## Built-in Functions

### oc_omit()

Removes fields from output. Has two distinct behaviors depending on usage:

**1. Field-level omission** - Removes the YAML key from the template:

```yaml
# Omit individual fields
resources:
  limits:
    memory: ${parameters.memoryLimit}
    cpu: ${has(parameters.cpuLimit) ? parameters.cpuLimit : oc_omit()}
    # When cpuLimit is missing, the entire 'cpu:' line is removed

# Omit entire nested maps
metadata:
  name: ${metadata.name}
  annotations: ${has(parameters.annotations) ? parameters.annotations : oc_omit()}
  # When annotations is missing, the entire 'annotations:' key is removed
```

**2. Expression-level omission** - Removes keys from within a CEL map expression:

```yaml
# Use CEL's optional key syntax for simple optional fields
container: |
  ${{
    "image": parameters.image,
    ?"cpu": parameters.?cpu,
    ?"memory": parameters.?memory
  }}
  # Keys are only included if the value exists

# Use oc_omit() when conditional logic is involved
container: |
  ${{
    "image": parameters.image,
    "cpu": parameters.cpuLimit > 0 ? parameters.cpuLimit : oc_omit(),
    "debug": parameters.environment == "dev" ? true : oc_omit()
  }}
  # Keys are conditionally included based on logic, not just existence
```

### oc_merge(base, override, ...)
Shallow merge two or more maps (later maps override earlier ones):

```yaml
# Merge default and custom labels
labels: |
  ${oc_merge({"app": metadata.name, "version": "v1"}, parameters.customLabels)}

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

## OpenChoreo Resource Control Fields

OpenChoreo extends the templating system with special fields for dynamic resource generation:

### includeWhen - Conditional Resource Inclusion (ComponentType and Trait creates)

The `includeWhen` field on ComponentType resources and Trait creates controls whether a resource is included in the output based on a CEL expression:

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

### forEach - Dynamic Resource Generation (ComponentType, Trait creates, and Trait patches)

The `forEach` field generates multiple resources from a list or map. Available on ComponentType resources, Trait creates, and Trait patches.

- **Lists**: Each item is bound directly to the loop variable
- **Maps**: Each item has `.key` and `.value` fields; keys are iterated in **alphabetical order** for deterministic output

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

### Filtering Items in forEach

**Use `.filter()` within the forEach expression to iterate only over matching items:**

```yaml
resources:
  # Generate secrets only for enabled integrations
  - forEach: ${parameters.integrations.filter(i, i.enabled && has(i.credentials))}
    var: integration
    resource:
      apiVersion: v1
      kind: Secret
      metadata:
        name: ${oc_generate_name(metadata.name, integration.name, "secret")}
      type: Opaque
      stringData:
        api_key: ${integration.credentials.apiKey}
        api_secret: ${integration.credentials.apiSecret}

  # Create Services only for exposed ports
  - forEach: ${parameters.ports.filter(p, p.expose)}
    var: port
    resource:
      apiVersion: v1
      kind: Service
      metadata:
        name: ${oc_generate_name(metadata.name, port.name)}
      spec:
        ports:
          - port: ${port.port}
            name: ${port.name}
```

**You can chain `.filter()`, `.map()`, and other CEL list operations:**

```yaml
resources:
  # Filter enabled items, transform, then iterate
  - forEach: |
      ${parameters.configs
        .filter(c, c.enabled)
        .map(c, {"name": c.name, "data": c.value})}
    var: config
    resource:
      apiVersion: v1
      kind: ConfigMap
      metadata:
        name: ${config.name}
      data:
        value: ${config.data}
```

### Combining forEach with includeWhen

**Important:** `includeWhen` is evaluated **before** the forEach loop starts and controls whether the **entire forEach block** is included. The loop variable is **not available** in `includeWhen` expressions.

```yaml
resources:
  # includeWhen controls entire forEach block - not individual items
  - includeWhen: ${parameters.createSecrets}
    forEach: ${parameters.integrations}
    var: integration
    resource:
      apiVersion: v1
      kind: Secret
      metadata:
        name: ${integration.name}

  # WRONG - loop variable not available in includeWhen
  - includeWhen: ${integration.enabled}  # ERROR: 'integration' doesn't exist yet
    forEach: ${parameters.integrations}
    var: integration
    resource:
      apiVersion: v1
      kind: Secret
      metadata:
        name: ${integration.name}

  # CORRECT - use filter() for item-level filtering
  - forEach: ${parameters.integrations.filter(i, i.enabled)}
    var: integration
    resource:
      apiVersion: v1
      kind: Secret
      metadata:
        name: ${integration.name}
```

### Using forEach with Maps

When iterating over a map, each item has `.key` and `.value` fields. **Map keys are iterated in alphabetical order** to ensure deterministic resource generation across runs.

```yaml
resources:
  # Generate ConfigMap from map entries
  # If configFiles = {"database": "postgres://...", "cache": "redis://..."}
  # Items are iterated in order: cache, database (alphabetically sorted)
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

**Accessing nested values in maps:**

```yaml
resources:
  # Map with complex values
  # connections = {"db": {"host": "localhost", "port": 5432}, "cache": {"host": "redis", "port": 6379}}
  - forEach: ${parameters.connections}
    var: conn
    resource:
      apiVersion: v1
      kind: ConfigMap
      metadata:
        name: ${conn.key}-config
      data:
        host: ${conn.value.host}
        port: "${string(conn.value.port)}"
```

**Empty maps produce zero iterations** (no resources generated).

### Usage in Traits

Trait patches support `forEach` for iteration (over lists or maps) and `where` for conditional targeting:

```yaml
# In Trait definition
apiVersion: v1alpha1
kind: Trait
metadata:
  name: monitoring
spec:
  creates:
    # Trait creates support includeWhen and forEach (same as ComponentType resources)
    # Simple unconditional resource
    - template:
        apiVersion: monitoring.coreos.com/v1
        kind: ServiceMonitor
        metadata:
          name: ${metadata.name}
        spec:
          selector:
            matchLabels: ${metadata.podSelectors}

    # Use includeWhen for conditional resource creation
    - includeWhen: ${parameters.alerting.enabled}
      template:
        apiVersion: monitoring.coreos.com/v1
        kind: PrometheusRule
        metadata:
          name: ${metadata.name}-alerts
        spec:
          groups:
            - name: ${metadata.name}
              rules: ${parameters.alerting.rules}

    # Use forEach for generating multiple resources
    - forEach: ${parameters.volumes}
      var: vol
      template:
        apiVersion: v1
        kind: PersistentVolumeClaim
        metadata:
          name: ${oc_generate_name(metadata.name, vol.name)}
        spec:
          accessModes: ["ReadWriteOnce"]
          resources:
            requests:
              storage: ${vol.size}

    # Combine includeWhen and forEach - includeWhen controls the entire block
    - includeWhen: ${parameters.createSecrets}
      forEach: ${parameters.secrets}
      var: secret
      template:
        apiVersion: v1
        kind: Secret
        metadata:
          name: ${oc_generate_name(metadata.name, secret.name)}
        type: Opaque
        stringData: ${secret.data}

  patches:
    # Use 'where' in target for conditional patching
    - target:
        group: apps
        version: v1
        kind: Deployment
        where: ${parameters.monitoring.enabled}
      operations:
        - op: add
          path: /spec/template/metadata/annotations/prometheus.io~1scrape
          value: "true"

    # Use forEach for iterating over lists
    - forEach: ${parameters.extraPorts}
      var: port
      target:
        group: apps
        version: v1
        kind: Deployment
      operations:
        - op: add
          path: /spec/template/spec/containers/0/ports/-
          value: |
            ${{"containerPort": port.number, "name": port.name}}

    # Use forEach with maps - keys are iterated alphabetically
    - forEach: ${parameters.envVars}
      var: env
      target:
        group: apps
        version: v1
        kind: Deployment
      operations:
        - op: add
          path: /spec/template/spec/containers/0/env/-
          value:
            name: ${env.key}
            value: ${env.value}
```
