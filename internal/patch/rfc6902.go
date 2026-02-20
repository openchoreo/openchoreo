// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package patch

import (
	"fmt"
	"strconv"

	"github.com/openchoreo/openchoreo/internal/clone"
)

// toAnySlice converts typed slices to []any.
// Go's type system treats []T and []any as distinct types, so a type assertion
// from []map[string]any to []any fails even though each element is assignable to any.
func toAnySlice(v any) ([]any, bool) {
	switch arr := v.(type) {
	case []any:
		return arr, true
	case []map[string]any:
		result := make([]any, len(arr))
		for i, item := range arr {
			result[i] = item
		}
		return result, true
	default:
		return nil, false
	}
}

// applyJSONPatch applies a single RFC 6902 JSON Patch operation to the target document.
//
// This function uses direct in-memory manipulation instead of marshaling/unmarshaling,
// which significantly reduces allocations and improves performance.
//
// Supported operations: add, replace, remove (per RFC 6902)
func applyJSONPatch(target map[string]any, op, pointer string, value any) error {
	segments := splitPointer(pointer)
	if len(segments) == 0 {
		return fmt.Errorf("cannot apply %s operation to root document", op)
	}

	// Navigate to the container holding the target element
	container, arrayKey, lastSeg, err := navigateForPatch(target, segments)
	if err != nil {
		return err
	}

	switch op {
	case "add":
		return addValue(container, arrayKey, lastSeg, value)
	case "replace":
		return replaceValue(container, arrayKey, lastSeg, value)
	case opRemove:
		return removeValue(container, arrayKey, lastSeg)
	default:
		return fmt.Errorf("unsupported operation: %s", op)
	}
}

// navigateForPatch navigates to the correct container level for applying patch operations.
//
// Different operation types require different navigation depths:
// For map operations: returns (parent_map, "", key, nil)
//   - We modify values directly in the parent map
//
// For array operations: returns (grandparent_map, array_key, index, nil)
//   - We must return the grandparent because Go slices are value types
//   - Modifying a slice (append, insert, delete) creates a new slice
//   - We need access to grandparent[array_key] to update the reference
//
// Example: Patching /spec/containers/0/env
//   - For a map key: navigate to containers[0], return ("containers[0]", "", "env")
//   - For an array index: navigate to spec, return (spec, "containers", "0")
func navigateForPatch(root map[string]any, segments []string) (container map[string]any, arrayKey string, lastSeg string, err error) {
	if len(segments) == 0 {
		return nil, "", "", fmt.Errorf("empty pointer")
	}

	current := any(root)

	// Navigate through all segments except the last
	for i := 0; i < len(segments)-1; i++ {
		seg := segments[i]

		switch node := current.(type) {
		case map[string]any:
			child, exists := node[seg]
			if !exists {
				return nil, "", "", fmt.Errorf("path does not exist at segment %q", seg)
			}

			// Check if the next segment indicates we're accessing an array
			nextSeg := segments[i+1]
			if _, isIndex := isArrayIndex(nextSeg); isIndex || nextSeg == "-" {
				// Child should be an array, and we're about to index into it
				arr, ok := toAnySlice(child)
				if !ok {
					return nil, "", "", fmt.Errorf("expected array at %q, got %T", seg, child)
				}

				// Normalize the array to []any in the parent map for consistent handling
				node[seg] = arr

				// If this is the second-to-last segment, return here
				// because we need to modify the array in the parent map
				if i == len(segments)-2 {
					return node, seg, nextSeg, nil
				}

				// Otherwise, continue navigating into the array
				index, _ := isArrayIndex(nextSeg)
				if index < 0 || index >= len(arr) {
					return nil, "", "", fmt.Errorf("array index %d out of bounds at %q", index, seg)
				}
				current = arr[index]
				i++ // Skip the next segment since we just processed it
			} else {
				current = child
			}

		case []any:
			return nil, "", "", fmt.Errorf("unexpected array traversal at segment %q", seg)

		default:
			return nil, "", "", fmt.Errorf("cannot traverse segment %q on type %T", seg, current)
		}
	}

	// At this point, current should be a map, and we return it with the last segment
	resultMap, ok := current.(map[string]any)
	if !ok {
		return nil, "", "", fmt.Errorf("parent must be a map for non-array operations, got %T", current)
	}

	return resultMap, "", segments[len(segments)-1], nil
}

// isArrayIndex checks if a segment represents an array index.
// Returns (index, true) if it's a numeric index, or (-1, false) otherwise.
// The "-" append marker is not considered an index.
func isArrayIndex(seg string) (int, bool) {
	if seg == "-" {
		return -1, true
	}
	index, err := strconv.Atoi(seg)
	if err != nil {
		return -1, false
	}
	return index, true
}

// addValue implements the "add" operation from RFC 6902.
//
// If arrayKey is non-empty, we're modifying an array and must update the slice in container[arrayKey].
// Otherwise, we're setting a value in the container map directly.
func addValue(container map[string]any, arrayKey string, segment string, value any) error {
	valueCopy := clone.DeepCopy(value)

	if arrayKey != "" {
		// We're operating on an array element
		arr, ok := toAnySlice(container[arrayKey])
		if !ok {
			return fmt.Errorf("expected array at %q", arrayKey)
		}

		if segment == "-" {
			// Append to array
			container[arrayKey] = append(arr, valueCopy)
			return nil
		}

		index, err := strconv.Atoi(segment)
		if err != nil {
			return fmt.Errorf("invalid array index %q", segment)
		}
		if index < 0 || index > len(arr) {
			return fmt.Errorf("array index %d out of bounds for add (length %d)", index, len(arr))
		}

		// Guard against integer overflow on large arrays before allocating
		const maxJSONArraySize = 1 << 24 // 16 million elements
		if len(arr) >= maxJSONArraySize {
			return fmt.Errorf("array too large to process: %d elements", len(arr))
		}

		// Insert at index (RFC 6902 allows index == len to append)
		newArr := make([]any, len(arr)+1)
		copy(newArr[:index], arr[:index])
		newArr[index] = valueCopy
		copy(newArr[index+1:], arr[index:])
		container[arrayKey] = newArr
		return nil
	}

	// Operating on a map key
	container[segment] = valueCopy
	return nil
}

// replaceValue implements the "replace" operation from RFC 6902.
func replaceValue(container map[string]any, arrayKey string, segment string, value any) error {
	valueCopy := clone.DeepCopy(value)

	if arrayKey != "" {
		// Operating on array element
		arr, ok := toAnySlice(container[arrayKey])
		if !ok {
			return fmt.Errorf("expected array at %q", arrayKey)
		}

		if segment == "-" {
			return fmt.Errorf("replace operation cannot target append marker '-'")
		}

		index, err := strconv.Atoi(segment)
		if err != nil {
			return fmt.Errorf("invalid array index %q", segment)
		}
		if index < 0 || index >= len(arr) {
			return fmt.Errorf("array index %d out of bounds for replace (length %d)", index, len(arr))
		}

		arr[index] = valueCopy
		container[arrayKey] = arr
		return nil
	}

	// Operating on map key
	if _, exists := container[segment]; !exists {
		return fmt.Errorf("replace operation failed: key %q does not exist", segment)
	}
	container[segment] = valueCopy
	return nil
}

// removeValue implements the "remove" operation.
//
// For map keys, this implementation extends RFC 6902 to be idempotent: removing a
// non-existent key is a no-op rather than an error. This matches common Kubernetes
// cleanup patterns (e.g., "ensure this label/annotation doesn't exist").
//
// For array indices, out-of-bounds removal returns an error, as this likely indicates
// a bug in the patch logic rather than intentional cleanup.
//
// This is similar to evanphx/json-patch with AllowMissingPathOnRemove: true for maps.
func removeValue(container map[string]any, arrayKey string, segment string) error {
	if arrayKey != "" {
		// Operating on array element
		arr, ok := toAnySlice(container[arrayKey])
		if !ok {
			return fmt.Errorf("expected array at %q", arrayKey)
		}

		if segment == "-" {
			return fmt.Errorf("remove operation cannot target append marker '-'")
		}

		index, err := strconv.Atoi(segment)
		if err != nil {
			return fmt.Errorf("invalid array index %q", segment)
		}
		if index < 0 || index >= len(arr) {
			// Error on out-of-bounds: likely a bug, not intentional cleanup
			return fmt.Errorf("array index %d out of bounds for remove (length %d)", index, len(arr))
		}

		// Remove element by creating new slice
		newArr := make([]any, len(arr)-1)
		copy(newArr[:index], arr[:index])
		copy(newArr[index:], arr[index+1:])
		container[arrayKey] = newArr
		return nil
	}

	// Operating on map key - idempotent removal
	if _, exists := container[segment]; !exists {
		// Idempotent: silently succeed if key doesn't exist
		// This matches common Kubernetes cleanup patterns
		return nil
	}
	delete(container, segment)
	return nil
}

// ensureParentExists creates intermediate containers along a path as needed.
//
// For "add" operations, we want to auto-create missing parent objects/arrays
// so patch authors don't need to manually check for existence. This function
// traverses all parent segments (everything except the final one) and creates
// containers where needed.
//
// Container type is determined by inspecting the next segment:
//   - If next is "-", create an empty array (for append operations)
//   - If next is a number, we CANNOT auto-create - return error
//   - Otherwise, create an empty object
//
// The restriction on numeric indices prevents ambiguity: if we're adding to
// /spec/containers/0/env and containers doesn't exist, how many elements should
// the array have? We can't know, so we require the array to already exist.
func ensureParentExists(root map[string]any, pointer string) error {
	segments := splitPointer(pointer)
	if len(segments) == 0 {
		return nil
	}

	// Traverse all parent segments (not including the final one)
	current := any(root)
	for i := 0; i < len(segments)-1; i++ {
		seg := segments[i]

		// Try to convert typed slices to []any
		if arr, ok := toAnySlice(current); ok {
			// Current is an array, segment should be an index
			index, err := strconv.Atoi(seg)
			if err != nil {
				return fmt.Errorf("expected array index at segment %s", seg)
			}
			if index < 0 || index >= len(arr) {
				return fmt.Errorf("array index %d out of bounds at segment %s", index, seg)
			}
			current = arr[index]
			continue
		}

		node, ok := current.(map[string]any)
		if !ok {
			return fmt.Errorf("cannot traverse segment %s on type %T", seg, current)
		}

		child, exists := node[seg]
		if !exists || child == nil {
			// Determine what type of container to create
			next := segments[i+1]
			if next == "-" {
				// Next operation is append, create empty array
				node[seg] = []any{}
			} else if _, err := strconv.Atoi(next); err == nil {
				// Next operation needs a specific array index, but we can't
				// auto-create an array with that index - return error
				return fmt.Errorf("array index %s out of bounds at segment %s", next, seg)
			} else {
				// Next operation needs an object key, create empty object
				node[seg] = map[string]any{}
			}
			child = node[seg]
		}
		current = child
	}
	return nil
}
