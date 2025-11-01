# Patch Engine Overview

`internal/crd-renderer/patch` implements the patch semantics used by ComponentTypeDefinitions. It expands path filters, guards parent creation, and pushes standard RFC 6902 verbs through `github.com/evanphx/json-patch/v5`.

## Entry points

- `ApplyOperation` – render an individual `JSONPatchOperation` against a target resource once paths/values have been CEL-evaluated.
- `ApplySpec` – execute a full `PatchSpec` (target selection, optional `forEach` binding, multiple operations) against a slice of rendered resources.

## Supported operations

- `add`, `replace`, `remove` – standard RFC 6902 JSON Patch operations delegated to the upstream JSON Patch engine.
- `mergeShallow` – overlays top-level map keys while preserving existing siblings; parents are created automatically when possible.

## Path syntax additions

Paths extend JSON Pointer with:

- Array filters: `/spec/template/spec/containers[?(@.name=='app')]/env/-`
- Append marker: `/-` to push to arrays.
- Numeric segments (`/containers/0`) for index addressing.

### Path validation behavior

When a path resolves to zero elements, the behavior depends on the path type and operation:

| Path Type | Operation | Behavior | Rationale |
|-----------|-----------|----------|-----------|
| **Filter** `[?(...)]` | `add`, `replace`, `remove` | **Error** | Filters explicitly select specific elements. Zero matches indicates a configuration error (typo, non-existent container, etc.) |
| **Non-filter** | `add`, `replace` | **No-op** | Parent creation (via `ensureParentExists`) handles missing intermediate keys. Zero matches typically means operating on an empty array. |
| **Non-filter** | `remove` | **No-op** | Idempotent - removing something that doesn't exist is fine. |

**Examples:**

```yaml
# ERROR: Filter with no matches
patches:
  - op: add
    path: /spec/template/spec/containers/[?(@.name=='fluent-bit')]/volumeMounts/-
    # Error if no container named 'fluent-bit' exists

# OK: Parent creation for map keys
patches:
  - op: add
    path: /metadata/annotations/my-addon
    value: enabled
    # Creates: metadata → annotations → my-addon (even if metadata didn't exist)

# ✅ OK: Remove non-existent path (idempotent)
patches:
  - op: remove
    path: /metadata/annotations/temporary
    # No error if annotations or the key doesn't exist
```

**Why this design?**

1. **Filters are explicit selectors** - When you filter for `[?(@.name=='fluent-bit')]`, you're explicitly stating "this should exist." Zero matches is a bug.

2. **Map key traversal supports extensibility** - Addons need to add fields to structures that might not exist yet. The engine creates missing parent maps automatically.

3. **Arrays require explicit initialization** - Unlike map keys, you cannot create array elements by path. Use `/-` to append or ensure the array exists in the base template.

### RFC 6901 escaping

Paths follow RFC 6901 JSON Pointer escaping rules:
- `~` must be escaped as `~0`
- `/` must be escaped as `~1`

**Examples:**

```yaml
# Kubernetes annotation keys containing /
path: /metadata/annotations/app.kubernetes.io~1name
# Creates: metadata.annotations["app.kubernetes.io/name"]

# Map key containing ~
path: /metadata/labels/special~0key
# Creates: metadata.labels["special~key"]

# Filter value containing /
path: /containers[?(@.url=='http:~1~1example.com')]/env/-
# Matches containers where url == "http://example.com"
```

**Important:** Always escape `/` and `~` characters in:
- Map keys (annotations, labels, etc.)
- Filter comparison values
- Any path segment containing these characters

## Targeting and iteration

`PatchSpec` encapsulates:

- `target`: group/version/kind/name plus optional CEL-based `where` filter evaluated against each resource.
- `forEach` / `var`: optional CEL expression that iterates over a list and binds each element (default variable name `item`) into the inputs before running operations.

The patch engine manages the `resource` binding while operations run so CEL expressions can reference the live object.

## Parent creation rules

For `add` and `mergeShallow` operations on **map key paths**, the engine automatically creates missing intermediate structures:

```yaml
# Starting with: { apiVersion: "v1", kind: "ConfigMap" }

# This operation:
- op: add
  path: /metadata/annotations/my-addon
  value: enabled

# Creates the missing parent structures:
# metadata: {}
# metadata.annotations: {}
# metadata.annotations["my-addon"]: "enabled"
```

**Important constraints:**

- **Array elements cannot be created by path** - Numeric indices like `/containers/2` require the array to have at least 3 elements. Use `/-` to append.
- **Filters that don't match will error** - Paths with `[?(...)]` that match zero elements indicate a configuration error.
- **Only map keys are auto-created** - The engine traverses through nil map values and creates them as needed.

## Tests

Run `go test ./internal/crd-renderer/patch` to exercise filter expansion, parent creation, and the spec runner. Extend tests alongside new features.
