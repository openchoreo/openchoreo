# Compact Schema

This guide explains how to define schemas for ComponentTypes and Traits using OpenChoreo's compact schema syntax. The compact syntax provides a concise, readable alternative to verbose JSON Schema while maintaining full validation capabilities.

## Overview

Compact schemas allow you to define parameter validation rules using simple string expressions instead of complex JSON Schema objects. The syntax follows the pattern:

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
name: "string | required=true"
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
  host: "string | required=true"
  port: "integer | default=5432"
  username: "string | required=true"
  password: "string | required=true"
  options:
    ssl: "boolean | default=true"
    timeout: "integer | default=30"
```

## Constraint Markers

Constraints are specified after the pipe (`|`) separator, space-separated:

### Required and Default

```yaml
# Fields are required by default unless they have a default value
name: string                    # Required field
description: "string | default=''"  # Optional (has default)

# Explicitly set required status
email: "string | required=true"
nickname: "string | required=false"

# Default values
replicas: "integer | default=1"
environment: "string | default=production"
debug: "boolean | default=false"
```

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
price: "number | exclusiveMinimum=0 multipleOf=0.01"
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

## Custom Types

Define reusable types in the `types` section of ComponentType:

```yaml
apiVersion: v1alpha1
kind: ComponentType
metadata:
  name: web-app
spec:
  types:
    MountConfig:
      path: "string | required=true"
      subPath: "string | default=''"
      readOnly: "boolean | default=false"

    DatabaseConfig:
      host: "string | required=true"
      port: "integer | default=5432 minimum=1 maximum=65535"
      database: "string | required=true"
      username: "string | required=true"
      password: "string | required=true"

  schema:
    parameters:
      volumes: "[]MountConfig"
      database: DatabaseConfig
      replicas: "integer | default=1 minimum=1"
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
  - name: "string | required=true pattern=^[a-z][a-z0-9-]*$"
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
    name: "string | required=true"
    type: "string | enum=postgres,mysql,mongodb"

    connection:
      host: "string | required=true"
      port: "integer | required=true"
      database: "string | required=true"

      auth:
        username: "string | required=true"
        password: "string | required=true"

      pool:
        min: "integer | default=2 minimum=1"
        max: "integer | default=10 minimum=1"
        idleTimeout: "integer | default=300"
```

## Validation Rules

### Default Behavior

1. **Fields are required by default** unless they have:
   - A `default=` constraint
   - An explicit `required=false` constraint

2. **Nested objects inherit parent's required status**:
   ```yaml
   # If database is optional, all its fields can be omitted
   database: "object | required=false"
     host: string
     port: integer
   ```

3. **Arrays and maps are empty by default** if optional:
   ```yaml
   tags: "[]string | default=[]"        # Explicitly empty
   labels: "map<string>"                 # Implicitly empty if optional
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

## Error Messages and Troubleshooting

### Common Errors

**"map key must be 'string'"**
- Cause: Trying to use non-string map keys
- Fix: Maps always use string keys in Kubernetes/JSON

**"unknown type: object"**
- Cause: Using generic `object` type
- Fix: Use `map<string>` or define explicit structure

**"unknown marker: X"**
- Cause: Typo in constraint name or unsupported constraint
- Fix: Check spelling and refer to supported constraints list

**"invalid default value"**
- Cause: Default doesn't match type or constraints
- Fix: Ensure default satisfies all validation rules

### Validation Tips

1. **Test with examples**:
   ```yaml
   email: "string | format=email example=user@example.com"
   ```

2. **Use appropriate number types**:
   ```yaml
   count: integer      # For whole numbers
   ratio: number       # For decimals
   ```

3. **Be specific with patterns**:
   ```yaml
   # Good - specific pattern
   version: "string | pattern=^v[0-9]+\\.[0-9]+\\.[0-9]+$"

   # Bad - too generic
   version: "string"
   ```

4. **Consider nullability**:
   ```yaml
   optional: "string | nullable=true"  # Can be null or string
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

## Best Practices

### 1. Use Meaningful Defaults

```yaml
# Good - sensible defaults
replicas: "integer | default=1 minimum=0"
timeout: "integer | default=30 minimum=1"

# Bad - no defaults for common fields
replicas: "integer | minimum=0"
```

### 2. Validate Early with Patterns

```yaml
# Good - validate format
email: "string | format=email"
memory: "string | pattern=^[0-9]+(Mi|Gi)$"

# Bad - accept any string
email: string
memory: string
```

### 3. Use Enums for Known Values

```yaml
# Good - restrict to valid options
environment: "string | enum=dev,staging,prod"

# Bad - accept any string
environment: string
```

### 4. Document Complex Fields

```yaml
# Good - clear documentation
webhookUrl: |
  string |
  format=uri
  title="Webhook URL"
  description="HTTPS endpoint for receiving notifications"
  example="https://example.com/webhooks/k8s"
```

### 5. Group Related Fields

```yaml
# Good - logical grouping
database:
  connection:
    host: string
    port: integer
  credentials:
    username: string
    password: string

# Bad - flat structure
dbHost: string
dbPort: integer
dbUsername: string
dbPassword: string
```

## Complete Example

Here's a complete ComponentType using compact schemas:

```yaml
apiVersion: v1alpha1
kind: ComponentType
metadata:
  name: web-service
spec:
  workloadType: deployment

  types:
    HealthCheck:
      enabled: "boolean | default=true"
      path: "string | default=/health"
      port: "integer | minimum=1 maximum=65535"
      initialDelay: "integer | default=30 minimum=0"
      period: "integer | default=30 minimum=1"

    Volume:
      name: "string | required=true pattern=^[a-z][a-z0-9-]*$"
      path: "string | required=true"
      size: "string | pattern=^[0-9]+(Gi|Ti)$ default=10Gi"
      storageClass: "string | default=standard"

  schema:
    parameters:
      # Basic configuration
      image: "string | required=true"
      tag: "string | default=latest"
      replicas: "integer | default=1 minimum=0 maximum=100"

      # Resources
      resources:
        cpu: "string | pattern=^[0-9]+m?$ default=100m"
        memory: "string | pattern=^[0-9]+(Mi|Gi)$ default=256Mi"

      # Networking
      port: "integer | default=8080 minimum=1 maximum=65535"
      serviceType: "string | enum=ClusterIP,NodePort,LoadBalancer default=ClusterIP"

      # Health checks
      healthCheck: HealthCheck

      # Storage
      volumes: "[]Volume"

      # Environment
      env: "map<string>"
      secrets: "[]string"

    envOverrides:
      replicas: "integer | minimum=1 maximum=100"
      resources:
        cpu: "string | pattern=^[0-9]+m?$"
        memory: "string | pattern=^[0-9]+(Mi|Gi)$"
      nodeSelector: "map<string>"
      tolerations: "[]map<string>"
```

## See Also

- [CEL Templating](./cel-templating.md) - Dynamic resource generation
- [Patching](./patching.md) - Resource modification with traits
