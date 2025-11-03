// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package patch

import (
	"fmt"
	"strconv"

	"github.com/openchoreo/openchoreo/internal/clone"
)

// mergeShallowAtPointer performs a shallow merge at the location specified by the pointer.
//
// The merge behavior:
//   - If the target location doesn't exist or is nil, set it to a copy of value
//   - If the target location is not a map, replace it with a copy of value
//   - If the target location is a map, overlay value's keys onto it (shallow merge)
//
// Shallow merge means we copy top-level keys from value, but don't recursively merge
// nested structures. If both target and value have a key "nested" that contains an object,
// value's "nested" object completely replaces target's "nested" object.
func mergeShallowAtPointer(root map[string]any, pointer string, value map[string]any) error {
	parent, last, err := navigateToParent(root, pointer, true)
	if err != nil {
		return err
	}

	switch container := parent.(type) {
	case map[string]any:
		existing, exists := container[last]
		if !exists || existing == nil {
			// Target doesn't exist, set it to a copy of value
			container[last] = clone.DeepCopy(value)
			return nil
		}
		targetMap, ok := existing.(map[string]any)
		if !ok || targetMap == nil {
			// Target exists but isn't a map, replace it
			container[last] = clone.DeepCopy(value)
			return nil
		}
		// Target is a map, perform shallow merge
		mergeShallowInto(targetMap, value)
	case []any:
		if last == "-" {
			return fmt.Errorf("mergeShallow operation cannot target append position '-'")
		}
		index, err := strconv.Atoi(last)
		if err != nil {
			return fmt.Errorf("invalid array index %q for mergeShallow", last)
		}
		if index < 0 || index >= len(container) {
			return fmt.Errorf("array index %d out of bounds for mergeShallow", index)
		}
		existing := container[index]
		if existing == nil {
			container[index] = clone.DeepCopy(value)
			return nil
		}
		targetMap, ok := existing.(map[string]any)
		if !ok || targetMap == nil {
			container[index] = clone.DeepCopy(value)
			return nil
		}
		mergeShallowInto(targetMap, value)
	default:
		return fmt.Errorf("mergeShallow parent must be object or array, got %T", parent)
	}
	return nil
}

// mergeShallowInto overlays overlay's keys onto target, modifying target in-place.
// Values are cloned to avoid sharing references between the overlay and target.
func mergeShallowInto(target map[string]any, overlay map[string]any) {
	for k, v := range overlay {
		target[k] = clone.DeepCopy(v)
	}
}

// navigateToParent traverses all but the last segment of a pointer, returning the
// parent container and the final segment name.
//
// If create is true, missing intermediate containers are auto-created using the
// same logic as ensureParentExists.
//
// Returns: (parent container, final segment name, error)
func navigateToParent(root map[string]any, pointer string, create bool) (any, string, error) {
	segments := splitPointer(pointer)
	if len(segments) == 0 {
		return root, "", nil
	}
	parentSegs := segments[:len(segments)-1]
	last := segments[len(segments)-1]

	current := any(root)
	for i, seg := range parentSegs {
		switch node := current.(type) {
		case map[string]any:
			child, exists := node[seg]
			if !exists || child == nil {
				if !create {
					return nil, "", fmt.Errorf("missing path at segment %s", seg)
				}
				// Auto-create the missing container
				next := determineNextContainerType(parentSegs, i, last)
				node[seg] = next
				child = node[seg]
			}
			current = child
		case []any:
			index, err := strconv.Atoi(seg)
			if err != nil {
				return nil, "", fmt.Errorf("expected array index at segment %s", seg)
			}
			if index < 0 || index >= len(node) {
				return nil, "", fmt.Errorf("array index %d out of bounds at segment %s", index, seg)
			}
			current = node[index]
		default:
			return nil, "", fmt.Errorf("cannot traverse segment %s on type %T", seg, node)
		}
	}
	return current, last, nil
}

// determineNextContainerType decides what type of container to create by inspecting
// the next segment in the path.
//
// Logic:
//   - If next segment is "-" → create empty array (for append)
//   - If next segment is numeric → create empty array (for indexed access)
//   - Otherwise → create empty object (for key access)
func determineNextContainerType(segments []string, index int, last string) any {
	nextSeg := last
	if index+1 < len(segments) {
		nextSeg = segments[index+1]
	}
	if nextSeg == "-" {
		return []any{}
	}
	if _, err := strconv.Atoi(nextSeg); err == nil {
		return []any{}
	}
	return map[string]any{}
}
