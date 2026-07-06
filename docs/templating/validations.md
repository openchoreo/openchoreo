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
  - rule: "${environmentConfigs.maxReplicas >= environmentConfigs.minReplicas}"
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
| ComponentType | ComponentContext | `metadata`, `parameters`, `environmentConfigs`, `dataplane`, `workload`, `configurations` |
| Trait | TraitContext | `metadata`, `parameters`, `environmentConfigs`, `dataplane`, `trait`, `workload`, `configurations` |

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
  parameters:
    openAPIV3Schema:
      endpointName: "string"
  environmentConfigs:
    openAPIV3Schema:
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
    - rule: "${environmentConfigs.maxReplicas >= environmentConfigs.minReplicas}"
      message: "maxReplicas must be greater than or equal to minReplicas."
  environmentConfigs:
    openAPIV3Schema:
      minReplicas: "integer | default=1 | minimum=1"
      maxReplicas: "integer | default=3 | minimum=1"
```

## Trait Pre-render and Post-render Validations

Traits and ClusterTraits support two additional validation stages beyond the shared `spec.validations` field.

### `preRenderValidations` (replaces `validations`)

On Trait and ClusterTrait, `spec.validations` is **deprecated**. Prefer `spec.preRenderValidations`, which has identical semantics — CEL rules evaluated before rendering against the trait's static context (`parameters`, `environmentConfigs`, `metadata`, `workload`, ...). The two fields are **mutually exclusive**: setting both on the same trait is rejected at admission time.

```yaml
kind: Trait
metadata:
  name: websocket-config
spec:
  preRenderValidations:
    - rule: "${parameters.endpointName in workload.endpoints}"
      message: "The specified endpointName must refer to an existing endpoint."
```

`preRenderValidations` uses the same rule structure (`rule` + `message`) and error format as `validations`. When both fields are absent, no pre-render rules run; the controller reads whichever field is set.

### `postRenderValidations`

`postRenderValidations` are CEL rules evaluated **after all traits are applied**, against the final rendered Kubernetes resources. This lets a trait assert invariants about the finished resource set — including whether its own contributions survived later traits in the stack.

Each entry selects target resources by GVK (plus an optional `where` filter), binds each match to the `resource` variable, and requires `rule` to evaluate to `true`.

| Field | Required | Description |
|-------|----------|-------------|
| `when` | No | CEL guard evaluated against the trait context; if it evaluates to `false`, the validation is skipped |
| `target.group` / `target.version` / `target.kind` | Yes | GVK of the rendered resources to select |
| `target.where` | No | CEL expression (with `resource` bound) filtering the selection to a subset |
| `target.mustMatch` | No | Defaults to `true`. When `true` and no resource matches the target, the validation fails |
| `rule` | Yes | CEL expression wrapped in `${...}`, evaluated with `resource` bound to each match; must be `true` |
| `message` | Yes | Error message shown when the rule evaluates to `false` |

`mustMatch` defaults to `true`, so a target that selects zero resources fails the release. This closes the gap where a later trait removed the resource this trait was validating.

```yaml
kind: Trait
metadata:
  name: exclusive-writer
spec:
  parameters:
    openAPIV3Schema:
      mode: "string | default=read"
  postRenderValidations:
    - when: ${parameters.mode == 'write'}
      target:
        group: apps
        version: v1
        kind: Deployment
      rule: ${resource.spec.replicas == 1}
      message: "write mode requires a single replica for exclusive access"
```

#### Verifying your own changes survived later traits

Because both `resource` and `parameters` are in scope, a single rule with CEL comprehensions can assert that everything a trait added (e.g. via `forEach` over `parameters`) is still present after the whole trait stack has run — no per-item loop construct is needed:

```yaml
postRenderValidations:
  # Assert every volume mount this trait added via forEach over parameters.mounts
  # is still present after all later traits in the stack have run.
  - target: { group: apps, version: v1, kind: Deployment }
    rule: ${parameters.mounts.all(m, resource.spec.template.spec.volumes.exists(v, v.name == m.name))}
    message: "a later trait removed one of this trait's volume mounts"
```

Post-render rule failures across all matched resources and all traits are aggregated (joined with `;`) into a single error, mirroring pre-render behavior. A malformed `where` selector is reported immediately as a hard error.

#### `forEach` — asserting a distinct resource survived per declared item

Comprehensions cover the "did any of my changes survive" case, but not the *distinct-resource-per-item* case: "for each route I declared, assert that route's own resource still exists." A per-resource rule never fires for a resource that was removed entirely, so per-item existence can't be caught that way.

`forEach` closes this. It repeats the validation for every item in a CEL-evaluated list (evaluated against the trait context), binding the loop variable (named by `var`) into `target.where` and `rule`. `mustMatch` then applies **per iteration**, so a missing resource for any single item fails that item. The semantics are identical to trait patch `forEach`: `var` is required when `forEach` is set (CRD-enforced), and the loop variable is in scope for `where` and `rule` but not for `when` (which is evaluated once, before iteration).

```yaml
postRenderValidations:
  - forEach: ${parameters.routes}
    var: route
    target:
      group: gateway.networking.k8s.io
      version: v1
      kind: HTTPRoute
      where: ${resource.metadata.name == route.name}
      mustMatch: true   # fails if this route's resource was removed by a later trait
    rule: ${resource.spec.rules.size() > 0}
    message: "a declared route lost its rules"
```

#### Gotchas

- **`targetPlane` scopes selection.** Like trait creates/patches/removes, a post-render validation targets a single plane via `targetPlane`, defaulting to `dataplane`. Set it to `observabilityplane` to assert against observability-plane resources of the same GVK.
- **`message` is a literal string.** CEL `${...}` interpolation is *not* applied to `message`. The failing `forEach` item is already identified in the aggregated error (e.g. `... (forEach route=web)`), so you don't need to embed it yourself.
- **`where` + default `mustMatch`.** With the default `mustMatch: true`, a `where` filter that narrows to zero resources is a hard failure (`no resource matched target`). If you mean "validate only when the resource is present," set `mustMatch: false` explicitly.
- **Empty `forEach` passes vacuously.** If the `forEach` list evaluates to empty, the validation passes without running any check — even with `mustMatch: true`. An empty or mistyped source list silently skips the validation.
- **Runs before OpenChoreo post-processing.** Post-render validations evaluate before OpenChoreo injects its own labels, annotations, and owner references, so rules can assert only on what the traits themselves rendered — not on platform-managed metadata.

## ComponentType Pre-render and Post-render Validations

ComponentTypes and ClusterComponentTypes support the same two additional validation stages as Traits, with the same field shapes and semantics.

### `preRenderValidations` (replaces `validations`)

On ComponentType and ClusterComponentType, `spec.validations` is **deprecated**. Prefer `spec.preRenderValidations`, which has identical semantics — CEL rules evaluated before rendering against the component context (`parameters`, `environmentConfigs`, `metadata`, `workload`, `dataplane`, ...). The two fields are **mutually exclusive**: setting both on the same ComponentType is rejected at admission time.

```yaml
kind: ComponentType
metadata:
  name: deployment/web-app
spec:
  workloadType: deployment
  preRenderValidations:
    - rule: "${size(workload.endpoints) > 0}"
      message: "A web-app must expose at least one endpoint."
```

When both fields are absent, no pre-render rules run; the controller reads whichever field is set.

### `postRenderValidations`

`postRenderValidations` on a ComponentType are CEL rules evaluated **after all traits are applied**, against the final rendered Kubernetes resources — the same stage as trait post-render validations. This lets a ComponentType author assert invariants about the finished resource set, such as "no attached trait changed my Deployment's replica count."

The field shapes (`when`, `forEach`/`var`, `target.{group,version,kind,where,mustMatch}`, `targetPlane`, `rule`, `message`), the default `mustMatch: true` behavior, and the aggregation of failures are **identical to the trait `postRenderValidations` documented above** — the only difference is that the rules bind to the component context (`parameters`, `environmentConfigs`, ...) rather than a trait's context.

```yaml
kind: ComponentType
metadata:
  name: deployment/web-app
spec:
  workloadType: deployment
  postRenderValidations:
    - target:
        group: apps
        version: v1
        kind: Deployment
      rule: ${resource.spec.replicas == parameters.replicas}
      message: "a trait changed replicas away from the component's declared value"
```

## Error Messages

When validation rules fail, error messages include the rule index, rule text, and the user-provided message:

```text
component type validation failed: rule[0] "${size(workload.endpoints) > 0}" evaluated to false: A deployment/service must expose at least one endpoint.
```

| Scenario | Format |
|----------|--------|
| Rule evaluates to `false` | `rule[i] "rule text" evaluated to false: message` |
| Rule evaluation error | `rule[i] "rule text" evaluation error: ...` |
| Rule returns non-boolean | `rule[i] "rule text" must evaluate to boolean, got T` |

Multiple failures from the same `validations` list are joined with `; `.

## Evaluation Order

1. Schema defaults are applied to parameters and environmentConfigs
2. **ComponentType pre-render validation rules** (`preRenderValidations`, or the deprecated `validations`) are evaluated using the full ComponentContext
3. Base resources are rendered from ComponentType templates
4. For each trait:
   a. Trait context is built with trait-specific parameters and environmentConfigs
   b. **Trait pre-render validation rules** (`preRenderValidations`, or the deprecated `validations`) are evaluated using the TraitContext
   c. Trait creates, patches, and removes are processed
5. **Post-render validation rules** (`postRenderValidations`) — from both the ComponentType and every trait — are evaluated against the final rendered resource set, after every trait has been applied

If any validation rule fails, rendering stops and the error is reported.
