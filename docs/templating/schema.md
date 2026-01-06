# Schema

This guide explains how to define schemas for ComponentTypes and Traits using OpenChoreo's schema syntax. The syntax provides a concise, readable alternative to verbose JSON Schema while maintaining full validation capabilities.

## Overview

Schemas allow you to define parameter validation rules using simple string expressions instead of complex JSON Schema objects. The syntax follows the pattern:

```yaml
fieldName: "type | constraint1=value1 constraint2=value2"
```

## Basic Syntax

### Simple Types

```yaml
name: string                              # Required string
age: "integer | minimum=0 maximum=120"    # Integer with constraints
price: "number | minimum=0.01"            # Number (float) with minimum
enabled: "boolean | default=false"        # Optional boolean with default
```

### Arrays

```yaml
tags: "[]string"              # Array of strings (or "array<string>")
ports: "[]integer"            # Array of integers
mounts: "[]MountConfig"       # Array of custom type
configs: "[]map<string>"      # Array of maps
```

### Maps

```yaml
labels: "map<string>"      # Map with string values (keys always strings)
ports: "map<integer>"      # Map with integer values
settings: "map<boolean>"   # Map with boolean values
```

### Objects

For structured objects, use nested field definitions:

```yaml
database:
  host: "string"
  port: "integer | default=5432"
  username: "string"
  password: "string"
  options:
    ssl: "boolean | default=true"
    timeout: "integer | default=30"
```

## Custom Types

Define reusable types in the `schema.types` section of ComponentType. Use custom types when the object structure is reused in multiple places or when you want a self-documenting type name (e.g., `DatabaseConfig`, `Resources`).

```yaml
apiVersion: v1alpha1
kind: ComponentType
metadata:
  name: web-app
spec:
  schema:
    types:
      MountConfig:
        path: "string"
        subPath: "string | default=''"
        readOnly: "boolean | default=false"

      DatabaseConfig:
        host: "string"
        port: "integer | default=5432 minimum=1 maximum=65535"
        database: "string"
        username: "string"
        password: "string"

    parameters:
      volumes: "[]MountConfig"
      database: DatabaseConfig
      replicas: "integer | default=1 minimum=1"
```

## Defaults

All fields (primitives, arrays, maps, and objects) are required by default - values must be provided for them. To make any field optional, you must explicitly provide a default value.

**Note**: The `required` marker is not supported. Fields are always required unless they have a `default` value. This enforces a clear two-state model: required (no default) or optional (has default).

### Primitive Fields, Arrays, and Maps

```yaml
# Required - must provide value
name: string
tags: "[]string"
labels: "map<string>"

# Optional - have explicit defaults
replicas: "integer | default=1"
tags: "[]string | default=[]"
labels: "map<string> | default={}"
```

### Objects

Objects are required unless they have a default value. There are two approaches to provide defaults:

**Approach 1: Default When Referencing a Type**

Define a custom type, then specify the default when using it with the `default=` constraint:

```yaml
spec:
  schema:
    types:
      Monitoring:
        enabled: "boolean | default=false"
        port: "integer | default=9090"

      Database:
        host: string
        port: "integer | default=5432"

    parameters:
      # Valid: All fields in Monitoring have defaults, so {} is valid
      monitoring: "Monitoring | default={}"

      # Valid: Default provides the required host field
      database: "Database | default={\"host\":\"localhost\"}"

      # Different defaults for same type
      primaryDB: "Database | default={\"host\":\"primary\"}"
      replicaDB: "Database | default={\"host\":\"replica\"}"

      # INVALID: Empty default doesn't provide required host
      # cache: "Database | default={}"   # ERROR: host is required
```

**Approach 2: Default in the Definition**

Use the `$default` key directly in the object definition. This works for both inline objects and type definitions:

```yaml
# In ComponentType spec:
spec:
  schema:
    parameters:
      # Inline object with empty default
      monitoring:
        $default: {}  # Valid because all fields have defaults
        enabled: "boolean | default=false"
        port: "integer | default=9090"

      # Inline object with non-empty default (block style)
      database:
        $default:
          host: "localhost"
        host: string  # Provided by default
        port: "integer | default=5432"  # Has field-level default

      # Inline object with non-empty default (flow style)
      cache:
        $default: {"host": "localhost"}
        host: string
        ttl: "integer | default=300"  # Has field-level default

      # Multiple inline objects each with defaults
      resources:
        requests:
          $default: {}
          cpu: "string | default=100m"
          memory: "string | default=256Mi"
        limits:
          $default: {}
          cpu: "string | default=1000m"
          memory: "string | default=1Gi"
```

**How defaults are applied:**

When an object is **not provided**, the object default is used. For fields not specified in the object default, field-level defaults apply:
```yaml
# With the database schema above
parameters: {}
# Result: database = {host: "localhost", port: 5432}
# Object default provides "host", field default provides "port"
```

If the object default specifies a value for a field that also has a field-level default, the object default value takes precedence:
```yaml
# Schema with overlapping defaults:
database:
  $default:
    host: "localhost"
    port: 9999  # Object default for port
  host: string
  port: "integer | default=5432"  # Field-level default

# When object not provided:
parameters: {}
# Result: database = {host: "localhost", port: 9999}
# Object default value (9999) overrides field default (5432)
```

When an object **is provided**, the object default is ignored entirely and field-level defaults apply to missing fields:
```yaml
parameters:
  database:
    host: "production-db"
# Result: database = {host: "production-db", port: 5432}
# Object default NOT used, port gets its field-level default
```

Providing an object overrides the entire object default (no merging). This follows standard JSON Schema semantics.

**Key points about `$default`:**
- Can be used in inline object definitions and type definitions, but NOT when referencing a type
  - When referencing, use constraint markers instead: `monitoring: "Monitoring | default={}"`
- The `$default` key itself does not appear as a field in the resulting schema
- Supports both block style (`$default:` with nested pairs) and flow style (`$default: {}`)

**Default Precedence:**
When a type definition has a `$default` and the field referencing it also specifies a default:
- Field-level default (via constraint marker) **always overrides** type-level default
- This allows types to provide sensible defaults that can be customized when needed

**Using `$default` in type definitions:**

Type-level `$default` makes all references to that type automatically optional:

```yaml
types:
  Resources:
    $default: {}
    cpu: "string | default=100m"
    memory: "string | default=256Mi"

parameters:
  resources1: Resources  # Optional
  resources2: Resources  # Optional
  resources3: Resources  # Optional
```

**Composing types:**

When types reference other types, defaults cascade through the hierarchy:

```yaml
types:
  Probe:
    $default: {}
    path: "string | default=/healthz"
    port: "integer | default=8080"
    initialDelaySeconds: "integer | default=0"
    periodSeconds: "integer | default=10"

  Resources:
    $default: {}
    cpu: "string | default=100m"
    memory: "string | default=256Mi"

  Service:
    image: string
    resources: Resources      # Optional
    livenessProbe: Probe      # Optional
    readinessProbe: Probe     # Optional

  AppConfig:
    $default: {}
    replicas: "integer | default=1"
    service: "Service | default={\"image\":\"nginx:latest\"}"

parameters:
  appConfig: AppConfig
```

**Choosing where to add the default:**

Use **Approach 1 (default when referencing)** when you want different defaults for the same type in different contexts (e.g., `primaryDB` and `replicaDB` with different host defaults).

Use **Approach 2 (default in definition)** when all uses of the type should have the same default, or for inline objects that aren't reusable types.

**Why explicit defaults are required:**

Objects are required unless you explicitly provide a default, even when all nested fields have defaults. This design is intentional:

- **Deliberate optionality**: Template authors must explicitly decide which configuration blocks can be omitted, rather than having optionality inferred from nested field defaults.
- **Schema evolution safety**: The explicit default serves as a contract that must be updated when adding required fields. Without it, adding a required field would silently change an "auto-optional" object to required, breaking existing component definitions without any error at the schema level.
- **Predictable behavior**: Optionality is explicit and local to each object definition. You don't need to inspect all nested fields to determine if an object is optional - just check if it has a default.
- **Clear intent**: `$default: {}` or `default={}` signals to readers that the entire configuration block is optional, making schemas self-documenting.

Example demonstrating the safety benefit:

```yaml
# Version 1: monitoring is optional (explicit $default makes it clear)
monitoring:
  $default: {}
  enabled: "boolean | default=false"
  port: "integer | default=9090"

# Version 2: Adding a required field forces you to update the explicit default
# The existing $default: {} now fails validation, alerting you to update it
monitoring:
  $default:
    endpoint: "http://default-endpoint"  # Must provide new required field
  enabled: "boolean | default=false"
  port: "integer | default=9090"
  endpoint: string  # New required field

# Without explicit defaults: monitoring would silently become required,
# breaking existing component definitions that omit it - with no error
# at the schema level to alert you.
```

## Constraint Markers

Constraints are specified after the pipe (`|`) separator, space-separated. Beyond defaults (covered in the [Defaults](#defaults) section), you can specify validation constraints:

### Validation Constraints

```yaml
# Strings
username: "string | minLength=3 maxLength=20 pattern=^[a-z][a-z0-9_]*$"
email: "string | format=email"

# Numbers
age: "integer | minimum=0 maximum=150"
price: "number | minimum=0 exclusiveMinimum=true multipleOf=0.01"

# Arrays
tags: "[]string | minItems=1 maxItems=10"
```

### Enumerations

```yaml
environment: "string | enum=development,staging,production"
logLevel: "string | enum=debug,info,warning,error default=info"
```

### Documentation

```yaml
apiKey: "string | title='API Key' description='Authentication key for external service' example=sk-abc123"
timeout: "integer | description='Request timeout in seconds' default=30"
```

## Custom Annotations

You can add custom metadata to schema fields using the `oc:` prefix. These annotations are ignored during schema validation but can be used by UI generators and scaffolding tools.

```yaml
commitHash: "string | oc:build:inject=git.sha oc:ui:hidden=true"
advancedTimeout: "string | default='30s' oc:scaffolding=omit"
```

## Validation Rules

### Default Behavior

All default-related rules are covered in the [Defaults](#defaults) section (fields required unless they have defaults, arrays/maps/objects require explicit defaults).

### Type Restrictions

- **Map keys must be strings**: `ports: "map<integer>"` (values are integers, keys always strings)
- **No generic object type**: Use `map<string>` for dynamic keys or define structure explicitly
- **Custom types must be defined**: Reference only types defined in `schema.types` section

## Escaping and Special Characters

### Quoting and Escaping

```yaml
# Single quotes: double to escape (use for values with double quotes)
description: "string | default='User''s timezone'"

# Double quotes: backslash escape (use for regex patterns)
pattern: "string | default=\"^[a-z]+\\\\d{3}$\""

# Pipes in values must be quoted
format: 'string | pattern="a|b|c"'

# Enum values with spaces/commas - quote each value
size: 'string | enum="extra small","small","medium","large"'
format: 'string | enum="lastname, firstname","firstname lastname"'
```

**Rules:**
- Single quotes: `''` escapes `'`
- Double quotes: `\\` escapes `\`, `\"` escapes `"`
- Pipes (`|`) in values require quoting
- Enum values with spaces or commas need individual quotes

## Schema Evolution Philosophy

OpenChoreo schemas allow additional properties beyond what's defined (no `"additionalProperties": false`), enabling safe schema evolution:

- **Development**: Add fields to Component before updating ComponentType schema
- **Promotion**: Add new `envOverrides` in target environment before promoting ComponentRelease with updated schema
- **Rollback**: Rolling back to older Release works - extra fields are simply ignored
- **Safety**: Unknown fields don't cause failures, enabling gradual evolution

```yaml
# Environment prepared for promotion
envOverrides:
  replicas: 2
  monitoring: "enabled"  # Added before Release v2 arrives
```

This forward compatibility prevents deployment failures and enables gradual schema evolution.

## Mapping to JSON Schema

OpenChoreo's schema syntax is a shorthand that compiles to standard JSON Schema. This section shows how the various OpenChoreo constructs map to JSON Schema.

### Primitive Types

OpenChoreo:
```yaml
name: "string | default=John"
age: "integer | minimum=0 maximum=120"
price: "number | minimum=0.01"
enabled: "boolean | default=false"
```

JSON Schema:
```json
{
  "type": "object",
  "properties": {
    "name": {"type": "string", "default": "John"},
    "age": {"type": "integer", "minimum": 0, "maximum": 120},
    "price": {"type": "number", "minimum": 0.01},
    "enabled": {"type": "boolean", "default": false}
  },
  "required": ["age", "price"]
}
```

### Inline Objects with `$default`

OpenChoreo:
```yaml
monitoring:
  $default: {}
  enabled: "boolean | default=false"
  port: "integer | default=9090"
```

JSON Schema:
```json
{
  "type": "object",
  "properties": {
    "monitoring": {
      "type": "object",
      "default": {},
      "properties": {
        "enabled": {"type": "boolean", "default": false},
        "port": {"type": "integer", "default": 9090}
      }
    }
  }
}
```

### Custom Types

OpenChoreo:
```yaml
types:
  Resources:
    $default: {}
    cpu: "string | default=100m"
    memory: "string | default=256Mi"

parameters:
  resources: Resources
```

JSON Schema:
```json
{
  "type": "object",
  "properties": {
    "resources": {
      "type": "object",
      "default": {},
      "properties": {
        "cpu": {"type": "string", "default": "100m"},
        "memory": {"type": "string", "default": "256Mi"}
      }
    }
  }
}
```

### Overriding Type Defaults

OpenChoreo:
```yaml
types:
  Resources:
    $default: {"cpu": "100m", "memory": "128Mi"}
    cpu: string
    memory: string

parameters:
  resources: "Resources | default={\"cpu\": \"500m\", \"memory\": \"256Mi\"}"
```

JSON Schema:
```json
{
  "type": "object",
  "properties": {
    "resources": {
      "type": "object",
      "default": {
        "cpu": "500m",
        "memory": "256Mi"
      },
      "required": ["cpu", "memory"],
      "properties": {
        "cpu": {"type": "string"},
        "memory": {"type": "string"}
      }
    }
  }
}
```

The field-level default (`default={...}`) overrides the type-level default.

### Arrays and Maps

OpenChoreo:
```yaml
tags: "[]string | default=[]"
labels: "map<string> | default={}"
ports: "[]integer | minItems=1 maxItems=10"
```

JSON Schema:
```json
{
  "type": "object",
  "properties": {
    "tags": {
      "type": "array",
      "items": {"type": "string"},
      "default": []
    },
    "labels": {
      "type": "object",
      "additionalProperties": {"type": "string"},
      "default": {}
    },
    "ports": {
      "type": "array",
      "items": {"type": "integer"},
      "minItems": 1,
      "maxItems": 10
    }
  },
  "required": ["ports"]
}
```
