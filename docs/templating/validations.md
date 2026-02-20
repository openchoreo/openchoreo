# Validation Rules

This guide covers CEL-based validation rules for ComponentTypes and Traits.

## Overview

Validation rules enforce semantic constraints that JSON Schema alone cannot express. While schemas define data shape (types, ranges, required fields), validation rules express cross-field relationships, conditional requirements, and domain-specific invariants.

Rules are CEL expressions wrapped in `${...}` that must evaluate to `true`. When a rule evaluates to `false`, the associated message is surfaced as an error. All rules are evaluated (no short-circuiting), so multiple failures are reported together.

Validation rules run **after** schema defaults are applied and **before** resource rendering begins.

## Rule Structure

Validation rules are defined in the `spec.validations` field of ComponentTypes and Traits:

```yaml
validations:
  - rule: "${size(workload.endpoints) > 0}"
    message: "must expose at least one endpoint"
  - rule: "${envOverrides.maxReplicas >= envOverrides.minReplicas}"
    message: "maxReplicas must be greater than or equal to minReplicas"
```

| Field | Required | Description |
|-------|----------|-------------|
| `rule` | Yes | CEL expression wrapped in `${...}` that must evaluate to `true` |
| `message` | Yes | Error message shown when the rule evaluates to `false` |

Rules are validated at admission time: the CEL expression must parse correctly and return a boolean type. Invalid expressions are rejected before the resource is created.

## Context Variables

Validation rules have access to the same context variables as resource templates:

| CRD | Context Type | Available Variables |
|-----|-------------|---------------------|
| ComponentType | ComponentContext | `metadata`, `parameters`, `envOverrides`, `dataplane`, `workload`, `configurations` |
| Trait | TraitContext | `metadata`, `parameters`, `envOverrides`, `dataplane`, `trait`, `workload`, `configurations` |

See [Template Context Variables](./context.md) for full documentation of each variable.

## Examples

### Differentiating deployment subtypes

```yaml
# deployment/worker - must NOT expose endpoints
kind: ComponentType
metadata:
  name: worker
spec:
  workloadType: deployment
  validations:
    - rule: "${size(workload.endpoints) == 0}"
      message: "A deployment/worker must not expose endpoints. Use deployment/service instead."
```

```yaml
# deployment/service - MUST expose at least one endpoint
kind: ComponentType
metadata:
  name: service
spec:
  workloadType: deployment
  validations:
    - rule: "${size(workload.endpoints) > 0}"
      message: "A deployment/service must expose at least one endpoint."
```

```yaml
# deployment/web-app - MUST expose at least one HTTP endpoint
kind: ComponentType
metadata:
  name: web-app
spec:
  workloadType: deployment
  validations:
    - rule: "${workload.endpoints.exists(name, workload.endpoints[name].type == 'HTTP')}"
      message: "A deployment/web-app must expose at least one HTTP endpoint."
```

### Trait parameter-to-workload validation

```yaml
# Validates that the referenced endpoint exists and is a WebSocket endpoint
kind: Trait
metadata:
  name: websocket-config
spec:
  schema:
    parameters:
      endpointName: "string"
    envOverrides:
      maxConnectionsPerPod: "integer | default=1000"
  validations:
    - rule: "${parameters.endpointName in workload.endpoints && workload.endpoints[parameters.endpointName].type == 'Websocket'}"
      message: "The specified endpointName must refer to an existing Websocket endpoint."
```

### Cross-field parameter validation

```yaml
# Ensure max >= min for autoscaling
kind: ComponentType
metadata:
  name: autoscaled-service
spec:
  workloadType: deployment
  validations:
    - rule: "${size(workload.endpoints) > 0}"
      message: "Must expose at least one endpoint."
    - rule: "${envOverrides.maxReplicas >= envOverrides.minReplicas}"
      message: "maxReplicas must be greater than or equal to minReplicas."
  schema:
    envOverrides:
      minReplicas: "integer | default=1 | minimum=1"
      maxReplicas: "integer | default=3 | minimum=1"
```

## Error Messages

When validation rules fail, error messages include the rule index, rule text, and the user-provided message:

```
component type validation failed: rule[0] "${size(workload.endpoints) > 0}" evaluated to false: A deployment/service must expose at least one endpoint.
```

| Scenario | Format |
|----------|--------|
| Rule evaluates to `false` | `rule[i] "rule text" evaluated to false: message` |
| Rule evaluation error | `rule[i] "rule text" evaluation error: ...` |
| Rule returns non-boolean | `rule[i] "rule text" must evaluate to boolean, got T` |

Multiple failures from the same `validations` list are joined with `; `.

## Evaluation Order

1. Schema defaults are applied to parameters and envOverrides
2. **ComponentType validation rules** are evaluated using the full ComponentContext
3. Base resources are rendered from ComponentType templates
4. For each trait:
   a. Trait context is built with trait-specific parameters and envOverrides
   b. **Trait validation rules** are evaluated using the TraitContext
   c. Trait creates and patches are processed

If any validation rule fails, rendering stops and the error is reported.
