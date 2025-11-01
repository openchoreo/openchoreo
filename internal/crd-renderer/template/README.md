# Template Engine

A CEL-backed template engine that evaluates expressions embedded in YAML structures. The engine supports inline expressions, dynamic map keys, nested structures, and type-aware evaluation.

## Quick Example

**Input Template:**

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: ${metadata.name}
  namespace: ${metadata.namespace}
  labels: ${metadata.labels}
spec:
  replicas: ${parameters.replicas}
  template:
    spec:
      containers:
        - name: app
          image: "myapp:${parameters.version}"
          env: ${parameters.env}
```

**Input Data:**

```go
inputs := map[string]any{
    "metadata": map[string]any{
        "name":      "web-service",
        "namespace": "production",
        "labels":    map[string]any{"app": "web", "env": "prod"},
    },
    "parameters": map[string]any{
        "replicas": 3,
        "version":  "v1.2.0",
        "env": []any{
            map[string]any{"name": "PORT", "value": "8080"},
        },
    },
}
```

**Output:**

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: web-service
  namespace: production
  labels:
    app: web
    env: prod
spec:
  replicas: 3
  template:
    spec:
      containers:
        - name: app
          image: "myapp:v1.2.0"
          env:
            - name: PORT
              value: "8080"
```

## Expression Types

The template engine supports three types of CEL expressions, each with different type handling behavior:

### 1. Standalone Expressions

Expressions that comprise the entire value return their **native CEL type** - strings, numbers, booleans, lists, or maps.

**Syntax:** A value containing only `${expression}` with no surrounding text.

**Examples:**

```yaml
# Integer values
replicas: ${parameters.replicas}
# Result: 3 (integer, not string "3")

# Map structures
labels: ${metadata.labels}
# Result:
#   app: web
#   env: prod

# List of maps
env: ${containers.map(c, {"name": c.name, "image": c.image})}
# Result:
#   - name: app
#     image: app:1.0
#   - name: sidecar
#     image: sidecar:latest

# Boolean values
enabled: ${has(spec.feature) ? spec.feature : false}
# Result: true (boolean, not string "true")
```

**Type Preservation:**

- `string` → `string`
- `int64` → `int64`
- `bool` → `bool`
- `map[string]any` → `map[string]any`
- `[]any` → `[]any`

### 2. Interpolated Expressions

Expressions embedded within a larger string are evaluated by CEL, then **coerced to string representation** for concatenation.

**Syntax:** A value containing `${expression}` with surrounding text or multiple expressions.

**Examples:**

```yaml
# String interpolation with numbers
message: "Application has ${spec.replicas} replicas running"
# Result: "Application has 3 replicas running"

# Multiple expressions in one string
url: "https://${metadata.name}.${metadata.namespace}.svc.cluster.local:${spec.port}"
# Result: "https://web-service.production.svc.cluster.local:8080"

# Explicit string conversion (recommended for clarity)
name: "port-${string(metadata.port)}"
# Result: "port-8080"
```

**Evaluation Process:**

1. **CEL evaluates the expression** (with strict type checking)
2. **Result is coerced to string** (if evaluation succeeds)

This means the CEL expression itself must be type-correct. For example:
- ✅ `"port-${spec.port}"` - Works (number coerced to string after evaluation)
- ❌ `"${1 + 'string'}"` - Error (CEL doesn't allow int + string)
- ✅ `"${string(1) + 'string'}"` - Works (explicit conversion makes it type-safe)

**Coercion Rules (after successful evaluation):**

- **Strings:** Used as-is
- **Numbers:** Formatted as decimal (`3` → `"3"`, `1.5` → `"1.5"`)
- **Booleans:** `"true"` or `"false"`
- **Maps/Lists:** JSON-encoded (`{"key": "value"}` → `"{\"key\":\"value\"}"`)

### 3. Dynamic Map Keys

Map keys can contain CEL expressions to compute key names dynamically. These expressions **must evaluate to strings** or an error will be returned.

**Syntax:** `${expression}` used as a map key.

**Valid Examples:**

```yaml
# String values
services:
  ${metadata.serviceName}: 8080
# Where metadata.serviceName = "web" → Result: web: 8080

# String concatenation
labels:
  ${'app.kubernetes.io/' + metadata.name}: active
# Result: app.kubernetes.io/web-service: active

# Explicit type conversion
ports:
  ${'port-' + string(metadata.port)}: http
# Result: port-8080: http
```

**Invalid Examples (will error):**

```yaml
# Pure number - ERROR
ports:
  ${metadata.port}: http
# Error: dynamic map key '${metadata.port}' must evaluate to a string, got int64: 8080

# Boolean - ERROR
flags:
  ${metadata.enabled}: active
# Error: dynamic map key '${metadata.enabled}' must evaluate to a string, got bool: true

# String concatenation without conversion - ERROR
labels:
  ${'port-' + metadata.port}: http
# Error: CEL evaluation error (type mismatch: can't add string + int64)
```

## Helper Functions

The template engine includes several built-in CEL functions:

### `omit()`

Conditionally remove fields from the output structure.

**Example:**

```yaml
annotations:
  required: "always-present"
  optional: '${has(spec.flag) && spec.flag ? "enabled" : omit()}'
```

**Result when `spec.flag = true`:**

```yaml
annotations:
  required: "always-present"
  optional: "enabled"
```

**Result when `spec.flag = false`:**

```yaml
annotations:
  required: "always-present"
```

**Using `omit()` inside map literals:**

`omit()` can also be used as values inside map literals to conditionally include fields:

```yaml
emptyDir: |
  ${parameters.sizeLimit != "" || parameters.medium != "" ? {
    "sizeLimit": parameters.sizeLimit != "" ? parameters.sizeLimit : omit(),
    "medium": parameters.medium != "" ? parameters.medium : omit()
  } : {}}
```

**Result when `parameters.sizeLimit = "1Gi"` and `parameters.medium = ""`:**

```yaml
emptyDir:
  sizeLimit: 1Gi
```

**Result when both are empty:**

```yaml
emptyDir: {}
```

This pattern is useful for building maps with optional fields based on runtime conditions, such as Kubernetes resource specifications with conditional settings.

### `merge(baseMap, overrideMap)`

Shallow merge two maps, with override values taking precedence.

**Example:**

```yaml
labels: '${merge({"team": "platform", "env": "dev"}, metadata.labels)}'
```

**Input:** `metadata.labels = {"team": "payments", "region": "us"}`

**Result:**

```yaml
labels:
  team: payments # Overridden
  env: dev # From base
  region: us # From override
```

### `sanitizeK8sResourceName(...args)`

Converts strings into valid Kubernetes resource names by:

- Removing non-alphanumeric characters
- Converting to lowercase
- Concatenating all arguments

**Example:**

```yaml
name: ${sanitizeK8sResourceName(metadata.name, "-", spec.version)}
```

**Input:** `metadata.name = "My App!"`, `spec.version = "v2.0"`

**Result:** `name: myappv20`

## CEL Extension Libraries

The template engine includes the following CEL extension libraries:

- **Strings** (`ext.Strings()`) - String manipulation functions
- **Encoders** (`ext.Encoders()`) - Base64 encoding/decoding
- **Math** (`ext.Math()`) - Mathematical operations
- **Lists** (`ext.Lists()`) - List manipulation functions
- **Sets** (`ext.Sets()`) - Set operations
- **TwoVarComprehensions** (`ext.TwoVarComprehensions()`) - Advanced transformations with two variables:
  - `transformMapEntry(indexVar, valueVar, transform)` - Transform list/map into a map with dynamic keys
  - `transformMap(indexVar, valueVar, transform)` - Transform list/map into a map with indexed keys
  - `transformList(indexVar, valueVar, transform)` - Transform list/map into a list
- **Optional Types** (`cel.OptionalTypes()`) - Safe handling of potentially missing values:
  - `obj.?field` - Optional field access (returns `optional.none()` if missing)
  - `map[?key]` - Optional map/list indexing
  - `{?key: obj.?field}` - Optional field setting in map literals
  - `optional.of(value)` - Wrap a value as optional
  - `optional.ofNonZeroValue(value)` - Wrap non-zero values, else `optional.none()`
  - `.hasValue()` - Check if optional has a value
  - `.value()` - Extract value from optional (errors if empty)
  - `.or(default)` - Provide fallback for empty optionals

See [CEL Extensions documentation](https://github.com/google/cel-go/tree/master/ext) for details.

## Usage

```go
import (
    "github.com/openchoreo/openchoreo/internal/crd-renderer/template"
)

// Create engine
engine := template.NewEngine()

// Prepare template (parsed YAML/JSON)
tpl := map[string]any{
    "name":     "${metadata.name}",
    "replicas": "${parameters.replicas}",
    "labels":   "${metadata.labels}",
}

// Prepare inputs
inputs := map[string]any{
    "metadata": map[string]any{
        "name":   "web-service",
        "labels": map[string]any{"app": "web"},
    },
    "parameters": map[string]any{
        "replicas": 3,
    },
}

// Render template
result, err := engine.Render(tpl, inputs)
if err != nil {
    // handle error
}

// Clean up omitted fields
cleaned := template.RemoveOmittedFields(result)

// Use the rendered output
```

## Performance Optimization

The template engine includes a two-level caching system for performance:

1. **Environment Cache** - Caches CEL environments based on input variable names
2. **Program Cache** - Caches compiled CEL programs per expression

This enables efficient re-rendering of templates with the same structure but different values (e.g., in loops or batch operations).

**Benchmark results** (from initial development):

- Environment caching: ~2x performance improvement
- Program caching: ~3-5x performance improvement
- Combined: ~10x faster for repeated renders

### Custom Cache Options

For testing or specialized use cases:

```go
// Disable all caching (for baseline benchmarking)
engine := template.NewEngineWithOptions(template.DisableCache())

// Disable only program cache
engine := template.NewEngineWithOptions(template.DisableProgramCacheOnly())
```

## Error Handling

The engine provides detailed error messages for common issues:

### Missing Data Errors

```go
// Check if error is due to missing data
if template.IsMissingDataError(err) {
    // Handle gracefully (e.g., in optional contexts)
}
```

**Detected errors:**

- `"no such key: <key>"` - Runtime error for missing map keys
- `"undeclared reference to '<var>'"` - Compile error for undefined variables

### Type Errors

```
CEL evaluation error in expression '${1 + "string"}': type mismatch
```

### Dynamic Key Errors

```
dynamic map key '${metadata.port}' must evaluate to a string, got int64: 8080
```

## Testing

```bash
# Run all tests
go test ./internal/crd-renderer/template/

# Run specific test
go test ./internal/crd-renderer/template/ -run TestEngineRender

# Run with verbose output
go test -v ./internal/crd-renderer/template/
```

## Common Patterns

### Conditional Field Values with Defaults

```yaml
resources:
  - template:
      apiVersion: v1
      kind: Service
      metadata:
        name: ${metadata.name}
      spec:
        type: '${has(parameters.serviceType) ? parameters.serviceType : "ClusterIP"}'
```

### Dynamic Labels

```yaml
metadata:
  labels: '${merge({"app": metadata.name, "version": parameters.version}, metadata.labels)}'
```

### Computed Names

```yaml
metadata:
  name: ${sanitizeK8sResourceName(metadata.name, parameters.environment)}
```

### Array Transformation

Concatenate lists:
```yaml
items: '${parameters.defaults + parameters.custom}'
```

**Input:**
```yaml
parameters:
  defaults: ["item1", "item2"]
  custom: ["item3", "item4"]
```

**Result:**
```yaml
items:
  - item1
  - item2
  - item3
  - item4
```

Transform list to list:
```yaml
env: '${parameters.envVars.map(e, {"name": e.key, "value": e.value})}'
```

**Input:**
```yaml
parameters:
  envVars:
    - key: PORT
      value: "8080"
    - key: HOST
      value: "0.0.0.0"
```

**Result:**
```yaml
env:
  - name: PORT
    value: "8080"
  - name: HOST
    value: "0.0.0.0"
```

Transform list to map with dynamic keys:
```yaml
envMap: '${parameters.envVars.transformMapEntry(i, v, {v.name: v.value})}'
```

**Input:**
```yaml
parameters:
  envVars:
    - name: PORT
      value: "8080"
    - name: HOST
      value: "0.0.0.0"
    - name: DEBUG
      value: "true"
```

**Result:**
```yaml
envMap:
  PORT: "8080"
  HOST: "0.0.0.0"
  DEBUG: "true"
```

### Conditional Field Omission

```yaml
spec:
  replicas: ${parameters.replicas}
  autoscaling: '${has(parameters.maxReplicas) ? {"maxReplicas": parameters.maxReplicas} : omit()}'
```

### Map with Conditionally Omitted Fields

```yaml
volumes:
  - name: cache
    emptyDir: |
      ${parameters.sizeLimit != "" || parameters.medium != "" ? {
        "sizeLimit": parameters.sizeLimit != "" ? parameters.sizeLimit : omit(),
        "medium": parameters.medium != "" ? parameters.medium : omit()
      } : {}}
```

### Safe Navigation with Optional Types

```yaml
metadata:
  annotations: |
    ${{"app": metadata.name, ?"custom": spec.?annotations.?custom}}
```

**Input:**
```yaml
metadata:
  name: "my-app"
spec: {}  # annotations field is missing
```

**Result:**
```yaml
metadata:
  annotations:
    app: "my-app"
    # custom key is omitted since spec.annotations.custom doesn't exist
```
