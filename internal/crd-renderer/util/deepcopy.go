// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package util

// DeepCopy creates an optimized deep copy of any value.
//
// Optimizations:
//   - Returns nil for nil input (no allocation)
//   - Returns empty literals for empty collections (no heap allocation)
//   - Fast path for primitive types (no recursion needed)
//   - Pre-sizes destination maps and slices to avoid growth reallocations
//
// This function is safe for concurrent use and handles nested structures
// including maps, slices, and primitives.
func DeepCopy(v any) any {
	if v == nil {
		return nil
	}

	switch val := v.(type) {
	case map[string]any:
		if len(val) == 0 {
			return map[string]any{}
		}
		return deepCopyMap(val)

	case []any:
		if len(val) == 0 {
			return []any{}
		}
		return deepCopySlice(val)

	// Fast path: primitives are immutable, no copy needed
	case string, int, int64, int32, int16, int8,
		uint, uint64, uint32, uint16, uint8,
		float64, float32, bool:
		return val

	default:
		// Other types (interfaces, pointers, etc.) are returned as-is
		// The caller is responsible for knowing if these need special handling
		return val
	}
}

// deepCopyMap creates a deep copy of a map[string]any.
// The destination map is pre-sized to the source length to avoid reallocations.
func deepCopyMap(src map[string]any) map[string]any {
	dst := make(map[string]any, len(src))
	for k, v := range src {
		dst[k] = DeepCopy(v)
	}
	return dst
}

// deepCopySlice creates a deep copy of a []any slice.
// The destination slice is pre-sized to the source length to avoid reallocations.
func deepCopySlice(src []any) []any {
	dst := make([]any, len(src))
	for i, v := range src {
		dst[i] = DeepCopy(v)
	}
	return dst
}
