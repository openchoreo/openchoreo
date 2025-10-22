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

Filters can appear on any segment. If they match nothing the operation is treated as a no-op.

## Targeting and iteration

`PatchSpec` encapsulates:

- `target`: group/version/kind/name plus optional CEL-based `where` filter evaluated against each resource.
- `forEach` / `var`: optional CEL expression that iterates over a list and binds each element (default variable name `item`) into the inputs before running operations.

The patch engine manages the `resource` binding while operations run so CEL expressions can reference the live object.

## Parent creation rules

When an operation needs an object/array that does not exist yet, the engine creates intermediate maps and empty slices. Numeric array indices must already exist; append with `/-` when growing arrays.

## Tests

Run `go test ./internal/crd-renderer/patch` to exercise filter expansion, parent creation, and the spec runner. Extend tests alongside new features.
