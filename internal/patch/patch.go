// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package patch

import (
	"fmt"
	"regexp"
	"strings"
)

const (
	opAdd     = "add"
	opReplace = "replace"
	opRemove  = "remove"
)

// filterPattern matches array filter expressions like [?(@.name=='app')]
var filterPattern = regexp.MustCompile(`\[\?\(.*?\)\]`)

// ApplyPatches applies a list of JSON Patch operations to a single resource.
//
// This is the core, low-level patch function with a single responsibility:
// apply operations to ONE resource. It does NOT handle:
//   - Resource targeting (finding which resources to patch)
//   - forEach iteration (applying to multiple items)
//   - CEL rendering (operations should be pre-rendered)
//   - Where clause filtering
//
// Those concerns are handled by higher-level orchestration code (e.g., addon processor).
//
// Supported operations:
//   - add, replace, remove: standard RFC 6902 JSON Patch operations
//   - mergeShallow: custom operation that overlays map keys without deep merging
//
// Path expressions support:
//   - Array filters: /containers[?(@.name=='app')]/env
//   - Array indices: /containers/0/env
//   - Append marker: /env/-
//
// The resource is modified in-place.
func ApplyPatches(resource map[string]any, operations []JSONPatchOperation) error {
	for i, operation := range operations {
		if err := applyOperation(resource, operation); err != nil {
			return fmt.Errorf("operation #%d failed: %w", i, err)
		}
	}
	return nil
}

// applyOperation applies a single patch operation to a resource.
func applyOperation(target map[string]any, operation JSONPatchOperation) error {
	path := operation.Path
	value := operation.Value

	// Route to the appropriate operation handler
	op := strings.ToLower(operation.Op)
	switch op {
	case opAdd, opReplace, opRemove:
		return applyRFC6902(target, op, path, value)
	case "mergeshallow":
		return applyMergeShallow(target, path, value)
	default:
		return fmt.Errorf("unsupported patch operation %q (supported: add, replace, remove, mergeShallow)", operation.Op)
	}
}

// applyRFC6902 executes standard JSON Patch operations after expanding the path.
//
// Path expansion allows a single operation to target multiple locations:
//   - /containers[?(@.name=='app')]/image targets all matching containers
//   - /env/- appends to an array
//
// For "add" operations, we ensure parent containers exist before applying the patch.
// If the expanded path resolves to zero locations (filter didn't match or empty array):
//   - For paths with filters [?(@.field=='value')]: returns an error (likely a misconfiguration)
//   - For paths without filters: treats as no-op (allows idempotent operations)
//
// Note: For map key traversal, expandPaths allows traversing through nil values,
// so missing intermediate keys don't cause empty results. Those are handled by ensureParentExists.
func applyRFC6902(target map[string]any, op, rawPath string, value any) error {
	// Expand paths to handle filters and special markers
	resolved, err := expandPaths(target, rawPath)
	if err != nil {
		return err
	}
	if len(resolved) == 0 {
		// If the path contains a filter expression, not finding any matches is an error
		// (e.g., trying to patch a container that doesn't exist)
		if containsFilter(rawPath) {
			return fmt.Errorf("path %q contains a filter but matched 0 elements (filter criteria not met or target does not exist)", rawPath)
		}
		// No matches for non-filter paths; treat as no-op
		// This typically means an array-based path returned no results (empty array, etc.)
		return nil
	}

	// Apply the operation to each resolved location
	for _, pointer := range resolved {
		if op == opAdd {
			// Create missing parent containers for add operations
			if err := ensureParentExists(target, pointer); err != nil {
				return err
			}
		}
		if err := applyJSONPatch(target, op, pointer, value); err != nil {
			return err
		}
	}
	return nil
}

// containsFilter checks if a path contains a JSONPath filter expression.
// Filter expressions follow the pattern [?(@.field=='value')].
func containsFilter(path string) bool {
	return filterPattern.MatchString(path)
}

// applyMergeShallow applies a shallow merge operation, overlaying top-level keys
// without recursively merging nested structures.
//
// Unlike standard merge (or strategic merge patch), mergeShallow replaces entire
// nested objects rather than deep merging them. This gives more predictable behavior
// when you want to replace a nested configuration block completely.
//
// Example:
//
//	existing: {a: {x: 1, y: 2}, b: 3}
//	overlay:  {a: {z: 3}}
//	result:   {a: {z: 3}, b: 3}  // note: a.x and a.y are gone
func applyMergeShallow(target map[string]any, rawPath string, value any) error {
	valueMap, ok := value.(map[string]any)
	if !ok {
		return fmt.Errorf("mergeShallow value must be an object")
	}

	resolved, err := expandPaths(target, rawPath)
	if err != nil {
		return err
	}
	if len(resolved) == 0 {
		// Nothing to merge into.
		return nil
	}

	for _, pointer := range resolved {
		if err := mergeShallowAtPointer(target, pointer, valueMap); err != nil {
			return err
		}
	}
	return nil
}
