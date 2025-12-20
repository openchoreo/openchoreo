// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package renderer

import (
	"fmt"
	"sort"
)

// ToIterableItems converts a forEach evaluation result to a slice of items.
// Arrays are returned as-is. Maps are converted to sorted slice of {key, value} entries.
//
// Supported input types:
//   - []any - returned as-is
//   - []map[string]any - converted to []any
//   - map[string]any - converted to sorted slice of {"key": k, "value": v} entries
//
// For maps, the keys are sorted alphabetically to ensure deterministic iteration order.
// This is important for:
//   - Consistent resource generation across runs
//   - Predictable test results
//   - Reproducible deployments
func ToIterableItems(result any) ([]any, error) {
	switch v := result.(type) {
	case []any:
		return v, nil

	case []map[string]any:
		// Convert []map[string]any to []any
		items := make([]any, len(v))
		for i, m := range v {
			items[i] = m
		}
		return items, nil

	case map[string]any:
		// Convert map to sorted slice of {key, value} entries
		// Sort keys for deterministic iteration order
		keys := make([]string, 0, len(v))
		for k := range v {
			keys = append(keys, k)
		}
		sort.Strings(keys)

		// Create items slice with {key, value} maps
		items := make([]any, 0, len(v))
		for _, key := range keys {
			items = append(items, map[string]any{
				"key":   key,
				"value": v[key],
			})
		}
		return items, nil

	default:
		return nil, fmt.Errorf("forEach must evaluate to array or map, got %T", result)
	}
}
