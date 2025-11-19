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
# Primitives
name: string
age: integer
price: number
enabled: boolean

# With constraints
name: "string"
age: "integer | minimum=0 maximum=120"
price: "number | minimum=0.01"
enabled: "boolean | default=false"
```

### Arrays

Arrays can be defined using multiple notations:

```yaml
# Square bracket notation
tags: "[]string"
ports: "[]integer"

# Array notation
items: "array<string>"
numbers: "array<number>"

# Array of objects
mounts: "[]MountConfig"  # References custom type
configs: "[]map<string>"
```

### Maps

Maps always have string keys:

```yaml
# Simple map
labels: "map<string>"           # or "map[string]string"
annotations: "map<string>"

# Map with specific value type
ports: "map<integer>"           # Keys are strings, values are integers
settings: "map<boolean>"
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

Define reusable types in the `schema.types` section of ComponentType:

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

## Constraint Markers

Constraints are specified after the pipe (`|`) separator, space-separated:

### Required and Default

```yaml
# Fields are required by default unless they have a default value
name: string                    # Required field
description: "string | default=''"  # Optional (has default)

# Default values
replicas: "integer | default=1"
environment: "string | default=production"
debug: "boolean | default=false"
```

**Note**: The `required` marker is not supported. Fields are always required unless they have a `default` value. This enforces a clear two-state model: required (no default) or optional (has default).

#### Object Defaults

Objects follow the same rule - they're required unless they have a default. To make an object optional, define it as a custom type and provide a default value:

```yaml
# In ComponentType spec:
spec:
  schema:
    # Define the object structure as a custom type
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

      # INVALID: Empty default doesn't provide required host
      # cache: "Database | default={}"   # ERROR: host is required
```

**Key points**:
- Use custom types (in the `schema.types` section) for objects that need defaults
- An empty object default `{}` is only valid if all required fields in the type have defaults
- Object defaults must satisfy the custom type's schema validation
- Omitting an object with a default gets the default value
- Providing an object overrides the entire default (no merging)

### Validation Constraints

#### String Constraints

```yaml
username: "string | minLength=3 maxLength=20 pattern=^[a-z][a-z0-9_]*$"
email: "string | format=email"
url: "string | format=uri"
description: "string | maxLength=500"
code: "string | pattern=^[A-Z]{3}-[0-9]{3}$"
```

#### Number Constraints

```yaml
# Integer constraints
age: "integer | minimum=0 maximum=150"
priority: "integer | minimum=1 maximum=5"
step: "integer | multipleOf=5"

# Number (float) constraints
temperature: "number | minimum=-273.15"
percentage: "number | minimum=0 maximum=100"
price: "number | minimum=0 exclusiveMinimum=true multipleOf=0.01"
```

#### Array Constraints

```yaml
tags: "[]string | minItems=1 maxItems=10"
ports: "[]integer | uniqueItems=true"
items: "[]string | minItems=0 maxItems=100 uniqueItems=true"
```

### Enumerations

```yaml
environment: "string | enum=development,staging,production"
logLevel: "string | enum=debug,info,warning,error default=info"
region: "string | enum=us-east-1,us-west-2,eu-west-1,ap-south-1"
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

## Common Patterns

### Optional Configuration Blocks

```yaml
# Optional monitoring configuration
monitoring:
  enabled: "boolean | default=false"
  port: "integer | default=9090"
  path: "string | default=/metrics"
```

### Environment-Specific Overrides

```yaml
# In ComponentType
schema:
  parameters:
    replicas: "integer | default=1"
    memory: "string | default=256Mi"

  envOverrides:
    replicas: "integer | minimum=1 maximum=10"
    memory: "string | enum=256Mi,512Mi,1Gi,2Gi,4Gi"
    nodeSelector: "map<string>"
```

### Trait Configuration

```yaml
# In Trait definition
apiVersion: v1alpha1
kind: Trait
metadata:
  name: redis-cache
spec:
  schema:
    maxMemory: "string | default=256Mi pattern=^[0-9]+(Mi|Gi)$"
    evictionPolicy: "string | enum=allkeys-lru,volatile-lru,allkeys-random default=allkeys-lru"
    persistence:
      enabled: "boolean | default=false"
      storageClass: "string | default=standard"
      size: "string | default=10Gi pattern=^[0-9]+(Gi|Ti)$"
```

## Advanced Examples

### Complex Service Configuration

```yaml
services:
  - name: "string | pattern=^[a-z][a-z0-9-]*$"
    type: "string | enum=http,grpc,tcp default=http"
    port: "integer | minimum=1 maximum=65535"
    targetPort: "integer | minimum=1 maximum=65535"
    public: "boolean | default=false"

    http:
      path: "string | default=/"
      timeout: "integer | default=30 minimum=1"
      retries: "integer | default=3 minimum=0 maximum=10"

    healthCheck:
      enabled: "boolean | default=true"
      path: "string | default=/health"
      interval: "integer | default=30 minimum=5"
      threshold: "integer | default=3 minimum=1"
```

### Resource Requirements

```yaml
resources:
  tier: "string | enum=small,medium,large,custom default=small"

  custom:
    requests:
      cpu: "string | pattern=^[0-9]+m?$ default=100m"
      memory: "string | pattern=^[0-9]+(Mi|Gi)$ default=128Mi"
    limits:
      cpu: "string | pattern=^[0-9]+m?$ default=1000m"
      memory: "string | pattern=^[0-9]+(Mi|Gi)$ default=1Gi"
```

### Multi-Environment Database

```yaml
databases:
  "[]object":
    name: "string"
    type: "string | enum=postgres,mysql,mongodb"

    connection:
      host: "string"
      port: "integer"
      database: "string"

      auth:
        username: "string"
        password: "string"

      pool:
        min: "integer | default=2 minimum=1"
        max: "integer | default=10 minimum=1"
        idleTimeout: "integer | default=300"
```

## Validation Rules

### Default Behavior

1. **Fields are required by default** unless they have a `default=` constraint:
   ```yaml
   name: string                      # Required
   description: "string | default=''"  # Optional (has default)
   ```

2. **The `required` marker is not supported**: To enforce the "required unless default" pattern, the explicit `required=true` and `required=false` markers are not allowed. This simplifies schemas and makes it clear when fields are optional (they have defaults).

3. **Object fields follow the same rule**:
   ```yaml
   spec:
     schema:
       types:
         Monitoring:
           enabled: "boolean | default=false"
           port: "integer | default=9090"

       parameters:
         # Object without default is required
         database:
           host: string
           port: "integer | default=5432"

         # Object with default can be omitted (define as custom type)
         monitoring: "Monitoring | default={}"
   ```

4. **Object defaults must be valid**:
   ```yaml
   spec:
     schema:
       types:
         Cache:
           ttl: "integer | default=300"
           size: "integer | default=100"

         Database:
           host: string
           port: "integer | default=5432"

       parameters:
         # Valid: {} satisfies schema (all fields have defaults)
         cache: "Cache | default={}"

         # Invalid: {} doesn't provide required host
         # database: "Database | default={}"  # Error: host is required!
   ```

5. **Arrays and maps follow the same rule**:
   ```yaml
   # Required (no default - must provide even if empty)
   tags: "[]string"
   labels: "map<string>"

   # Optional (explicit default - can omit entirely)
   tags: "[]string | default=[]"
   labels: "map<string> | default={}"
   ```

### Type Restrictions

1. **Map keys must be strings**:
   ```yaml
   # Valid
   labels: "map<string>"
   ports: "map<integer>"    # Keys are strings, values are integers

   # Invalid - will error
   data: "map<integer, string>"  # Can't have integer keys
   ```

2. **No generic object type**:
   ```yaml
   # Invalid
   config: object

   # Valid alternatives
   config: "map<string>"    # For dynamic keys
   config:                  # For known structure
     key1: string
     key2: integer
   ```

3. **Custom types must be defined in types section**:
   ```yaml
   # Won't work without definition
   mount: MountConfig

   # Must define first
   types:
     MountConfig:
       path: string
       readOnly: boolean
   ```

## Escaping and Special Characters

### Quoting Values

Values containing special characters must be quoted. OpenChoreo supports both single and double quote styles:

```yaml
# Single quotes (use '' to escape single quotes)
description: "string | default='User''s timezone'"
jsonPath: "string | default='.status.conditions[?(@.type==\"Ready\")].status'"

# Double quotes (use backslash escaping)
pattern: "string | default=\"^[a-z]+\\d{3}$\""
message: "string | default=\"Value must be \\\"quoted\\\"\""
```

### Pipes in Values

Pipes (`|`) inside constraint values must be quoted to prevent interpretation as the type-constraint separator:

```yaml
# Pipe in regex pattern - must quote the value
format: 'string | pattern="a|b|c" default="x|y"'

# Without quotes, this would be interpreted incorrectly
# WRONG: pattern: string | pattern=a|b|c
```

### Enum Values with Spaces or Special Characters

Enum values containing spaces, commas, or other special characters must be individually quoted:

```yaml
# Enum with spaces in values - each value quoted
size: 'string | enum="extra small","small","medium","large" default="medium"'

# Multiple word enum values
tier: 'string | enum="free tier","basic tier","premium tier"'

# Enum values containing commas - quote each value
format: 'string | enum="lastname, firstname","firstname lastname","last, first, middle"'

# Mixed special characters
status: 'string | enum="pending","in-progress","done: completed","user said: \"hello, world\""'
```

The parser respects quotes when splitting enum values, so commas and other special characters inside quoted values are preserved.

### Quote Escaping Rules

**Single quotes** (YAML style):
- Escape single quote by doubling it: `''`
- Double quotes don't need escaping
- Common for JSONPath, filters, and values with double quotes

```yaml
# Single quote escaping
timezone: 'string | default=''America/New_York'''
query: 'string | default=''.items[?(@.status=="active")]'''
```

**Double quotes** (JSON/Go style):
- Escape with backslash: `\"`, `\\`, `\n`, `\t`
- Must escape backslashes as `\\`
- Common for regex patterns and escape sequences

```yaml
# Double quote escaping
regex: "string | pattern=\"^[a-z]+\\\\d{3}$\""
path: "string | default=\"C:\\\\Users\\\\Admin\""
```

### Complex Escaping Examples

```yaml
# Combining multiple special characters
description: |
  string |
  title="User's Configuration"
  default='Default config with "quotes" and pipes: a|b|c'
  pattern="^[a-z]+\\d{2,4}$"

# Enum with various special characters
status: |
  string |
  enum="pending","in-progress","done: completed","failed (error)"
  default="pending"

# Multi-line string values with quotes
helpText: |
  string |
  default="Line 1: Configure your app\nLine 2: Run 'deploy' command\nLine 3: Check \"status\""
```
