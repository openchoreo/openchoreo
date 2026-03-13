# openAPIV3Schema

This guide explains how to define schemas for ComponentTypes, Traits, and Workflows using standard **OpenAPI V3 / JSON Schema** format (`openAPIV3Schema`). This is the schema format for all OpenChoreo resources, providing full compatibility with the OpenAPI specification and enabling richer tooling integration.

## Overview

Schemas allow you to define parameter validation rules for ComponentTypes, Traits, and Workflows. The `openAPIV3Schema` format uses standard JSON Schema syntax, giving you access to the full set of JSON Schema features.

```yaml
parameters:
  openAPIV3Schema:
    type: object
    properties:
      replicas:
        type: integer
        default: 1
        minimum: 1
        maximum: 100
      image:
        type: string
        description: "Container image to deploy"
    required:
      - image
```

## Basic Syntax

### Simple Types

```yaml
openAPIV3Schema:
  type: object
  properties:
    name:
      type: string
    age:
      type: integer
      minimum: 0
      maximum: 120
    price:
      type: number
      minimum: 0.01
    enabled:
      type: boolean
      default: false
```

### Required Fields

In `openAPIV3Schema` you explicitly list required fields. Fields not listed in `required` are optional:

```yaml
openAPIV3Schema:
  type: object
  properties:
    name:
      type: string                  # required (listed below)
    replicas:
      type: integer
      default: 1                    # optional (has default, not in required)
    description:
      type: string                  # optional (not in required)
  required:
    - name
```

### Enums

```yaml
openAPIV3Schema:
  type: object
  properties:
    environment:
      type: string
      enum:
        - dev
        - staging
        - prod
      default: dev
    severity:
      type: string
      enum: [info, warning, critical]
      default: warning
```

### Arrays

```yaml
openAPIV3Schema:
  type: object
  properties:
    tags:
      type: array
      items:
        type: string
      default:
        - default
      minItems: 1
      maxItems: 10
    containers:
      type: array
      items:
        type: object
        properties:
          name:
            type: string
          image:
            type: string
          port:
            type: integer
            default: 8080
        required:
          - name
          - image
```

### Maps (additionalProperties)

```yaml
openAPIV3Schema:
  type: object
  properties:
    labels:
      type: object
      additionalProperties:
        type: string
      default: {}
    ports:
      type: object
      additionalProperties:
        type: integer
```

### Nested Objects

```yaml
openAPIV3Schema:
  type: object
  properties:
    database:
      type: object
      properties:
        host:
          type: string
        port:
          type: integer
          default: 5432
        credentials:
          type: object
          properties:
            username:
              type: string
            password:
              type: string
          required:
            - username
            - password
      required:
        - host
```

## Reusable Types with $defs and $ref

Use `$defs` to define reusable type definitions and `$ref` to reference them. This avoids duplication and keeps schemas maintainable.

```yaml
openAPIV3Schema:
  type: object
  $defs:
    ResourceQuantity:
      type: object
      default: {}
      properties:
        cpu:
          type: string
          default: "100m"
        memory:
          type: string
          default: "256Mi"
    ResourceRequirements:
      type: object
      default: {}
      properties:
        requests:
          $ref: "#/$defs/ResourceQuantity"
        limits:
          $ref: "#/$defs/ResourceQuantity"
  properties:
    resources:
      $ref: "#/$defs/ResourceRequirements"
```

### $ref with Sibling Keys

OpenChoreo supports JSON Schema 2020-12 semantics where `$ref` can have sibling keys. Sibling keys override the referenced definition on conflict:

```yaml
properties:
  resources:
    $ref: "#/$defs/ResourceRequirements"
    default: {}                              # overrides the definition's default
    x-openchoreo-resources-portal:           # vendor extension as sibling
      ui:field: ResourcePicker
```

### $ref Resolution Rules

- Only **local** `$ref` paths are supported: `#/$defs/Name`
- The older `definitions` keyword (JSON Schema Draft 4/7) is **not** supported — use `$defs` (JSON Schema 2020-12)
- Remote/URL refs (e.g., `http://...`) are rejected
- **Circular references** are detected and rejected with descriptive errors
- References are resolved recursively (chained refs like `A -> B -> C` work)
- Maximum resolution depth is 64 levels
- After resolution, `$defs` is removed from the output

## Vendor Extensions (x-*)

You can add custom `x-*` keys to any schema node. These are preserved in API responses (e.g., `/componenttypes/{name}/schema`) for consumption by frontends and portals:

```yaml
openAPIV3Schema:
  type: object
  $defs:
    ResourceRequirements:
      type: object
      properties:
        cpu:
          type: string
          default: "250m"
        memory:
          type: string
          default: "256Mi"
  properties:
    repository:
      type: object
      properties:
        url:
          type: string
          description: "Git repository URL"
          x-openchoreo-backstage-portal:
            ui:field: RepoUrlPicker
            ui:options:
              allowedHosts:
                - github.com
        branch:
          type: string
          default: main
          x-openchoreo-backstage-portal:
            ui:widget: text
    resources:
      $ref: "#/$defs/ResourceRequirements"
      x-openchoreo-resources-portal:
        ui:field: ResourcePicker
```

**Important:** Vendor extensions are:
- **Preserved** in API schema responses (`SectionToRawJSONSchema`)
- **Stripped** from Kubernetes structural schemas (K8s rejects `x-*` keys)
- **Not used** in validation — they are metadata only

## Defaults

### How Defaults Work

Defaults follow standard Kubernetes defaulting behavior:

1. Missing fields with `default` values are populated automatically
2. Existing fields are never overwritten
3. Nested object defaults require `default: {}` at each level for the defaulting engine to traverse into them

### Primitive, Array, and Map Defaults

```yaml
properties:
  # Required — no default, must provide value
  name:
    type: string

  # Optional — has default
  replicas:
    type: integer
    default: 1

  # Optional array
  tags:
    type: array
    items:
      type: string
    default: []

  # Optional map
  labels:
    type: object
    additionalProperties:
      type: string
    default: {}
required:
  - name
```

### Nested Object Defaults

For Kubernetes defaulting to create nested objects, **each level needs its own `default: {}`**. Without it, the defaulting engine will not traverse into the object and nested defaults will not be applied.

```yaml
$defs:
  ResourceQuantity:
    type: object
    default: {}          # Required: tells K8s to create this object
    properties:
      cpu:
        type: string
        default: "100m"
      memory:
        type: string
        default: "256Mi"
  ResourceRequirements:
    type: object
    default: {}          # Required: tells K8s to create this object
    properties:
      requests:
        $ref: "#/$defs/ResourceQuantity"
      limits:
        $ref: "#/$defs/ResourceQuantity"
properties:
  resources:
    $ref: "#/$defs/ResourceRequirements"
```

With the above, providing `{}` for parameters will result in:
```yaml
resources:
  requests:
    cpu: "100m"
    memory: "256Mi"
  limits:
    cpu: "100m"
    memory: "256Mi"
```

Without `default: {}` on the intermediate objects, nested defaults will **not** be applied.

### Overriding Defaults via $ref Siblings

When a field references a type via `$ref` and also specifies a `default` as a sibling key, the sibling default takes precedence over the definition's default:

```yaml
$defs:
  Resources:
    type: object
    default:
      cpu: "100m"
      memory: "128Mi"
    properties:
      cpu:
        type: string
      memory:
        type: string
    required: [cpu, memory]
properties:
  # Sibling default overrides the definition's default
  resources:
    $ref: "#/$defs/Resources"
    default:
      cpu: "500m"
      memory: "256Mi"
```

This allows types to provide sensible defaults that can be customized at each usage site.

### Default Precedence Summary

When a user provides an object, field-level defaults apply to any missing fields within it. When a user does not provide an object at all, the object-level `default` is used, then field-level defaults fill in any remaining gaps. Providing an object overrides the entire object default (no merging) — this follows standard JSON Schema semantics.

```yaml
# Schema:
database:
  type: object
  default:
    host: "localhost"
  properties:
    host:
      type: string
    port:
      type: integer
      default: 5432
  required:
    - host

# When user provides nothing → database = {host: "localhost", port: 5432}
# When user provides {host: "production-db"} → database = {host: "production-db", port: 5432}
```

## String Constraints

```yaml
username:
  type: string
  minLength: 3
  maxLength: 20
  pattern: "^[a-z][a-z0-9_]*$"
email:
  type: string
  format: email
version:
  type: string
  pattern: "^v\\d+\\.\\d+\\.\\d+$"
  default: "v1.0.0"
```

## Numeric Constraints

```yaml
replicas:
  type: integer
  minimum: 1
  maximum: 100
  default: 1
threshold:
  type: number
  minimum: 0
  maximum: 1
  exclusiveMinimum: true
  default: 0.75
statusCode:
  type: integer
  enum: [200, 201, 204, 400, 404, 500]
```

## Array Constraints

```yaml
tags:
  type: array
  items:
    type: string
  minItems: 1
  maxItems: 10
```

## Map Constraints

```yaml
metadata:
  type: object
  additionalProperties:
    type: string
  minProperties: 1
  maxProperties: 10
```

## Descriptions and Documentation

```yaml
openAPIV3Schema:
  type: object
  properties:
    replicas:
      type: integer
      default: 1
      description: "Number of pod replicas to run"
    image:
      type: string
      description: "Container image to deploy"
    environment:
      type: string
      enum: [dev, staging, prod]
      default: dev
      description: "Target deployment environment"
```

Descriptions are preserved in API responses and can be used by UI generators.

## Complete Examples

### ComponentType with openAPIV3Schema

```yaml
apiVersion: openchoreo.dev/v1alpha1
kind: ComponentType
metadata:
  name: service
spec:
  workloadType: deployment

  environmentConfigs:
    openAPIV3Schema:
      type: object
      $defs:
        ResourceQuantity:
          type: object
          default: {}
          properties:
            cpu:
              type: string
              default: "100m"
            memory:
              type: string
              default: "256Mi"
        ResourceRequirements:
          type: object
          default: {}
          properties:
            requests:
              $ref: "#/$defs/ResourceQuantity"
            limits:
              $ref: "#/$defs/ResourceQuantity"
      properties:
        replicas:
          type: integer
          default: 1
        resources:
          $ref: "#/$defs/ResourceRequirements"
        imagePullPolicy:
          type: string
          default: IfNotPresent

  resources:
    - id: deployment
      template:
        apiVersion: apps/v1
        kind: Deployment
        metadata:
          name: ${metadata.name}
        spec:
          replicas: ${environmentConfigs.replicas}
          # ... rest of template
```

### Trait with openAPIV3Schema

```yaml
apiVersion: openchoreo.dev/v1alpha1
kind: Trait
metadata:
  name: observability-alert-rule
spec:
  parameters:
    openAPIV3Schema:
      type: object
      properties:
        description:
          type: string
          description: "Human-readable description of the alert rule"
        severity:
          type: string
          enum: [info, warning, critical]
          default: warning
        source:
          type: object
          properties:
            type:
              type: string
              enum: [log, metric]
            query:
              type: string
              default: ""
          required:
            - type
      required:
        - description
        - source

  environmentConfigs:
    openAPIV3Schema:
      type: object
      properties:
        enabled:
          type: boolean
          default: true
        actions:
          type: object
          properties:
            notifications:
              type: object
              properties:
                channels:
                  type: array
                  items:
                    type: string
                  default: []
```

### Workflow with openAPIV3Schema

```yaml
apiVersion: openchoreo.dev/v1alpha1
kind: Workflow
metadata:
  name: docker
spec:
  parameters:
    openAPIV3Schema:
      type: object
      required:
        - repository
      properties:
        repository:
          type: object
          required:
            - url
          properties:
            url:
              type: string
              description: "Git repository URL"
            secretRef:
              type: string
              default: ""
            revision:
              type: object
              default: {}
              properties:
                branch:
                  type: string
                  default: main
                commit:
                  type: string
                  default: ""
            appPath:
              type: string
              default: "."
        docker:
          type: object
          default: {}
          properties:
            context:
              type: string
              default: "."
            filePath:
              type: string
              default: "./Dockerfile"
  # ... rest of workflow spec
```
