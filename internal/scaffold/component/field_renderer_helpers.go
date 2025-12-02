// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package component

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	extv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
)

// buildFieldComment builds a comment string for a field based on its schema.
func (r *FieldRenderer) buildFieldComment(prop *extv1.JSONSchemaProps) string {
	if prop == nil {
		return ""
	}

	if !r.includeFieldDescriptions {
		return ""
	}

	parts := []string{}

	if prop.Description != "" {
		parts = append(parts, prop.Description)
	}

	// Add enum alternatives (excluding the first value which is used as the field value)
	if len(prop.Enum) > 1 {
		alternatives := make([]string, len(prop.Enum)-1)
		for i, e := range prop.Enum[1:] {
			var enumValue any
			if err := json.Unmarshal(e.Raw, &enumValue); err == nil {
				alternatives[i] = formatDefaultValue(enumValue)
			} else {
				alternatives[i] = string(e.Raw)
			}
		}
		parts = append(parts, fmt.Sprintf("also: %s", strings.Join(alternatives, ", ")))
	}

	return strings.Join(parts, " | ")
}

// formatDefaultValue converts a default value to a YAML-friendly string.
func formatDefaultValue(v any) string {
	if v == nil {
		return ""
	}

	switch val := v.(type) {
	case string:
		return val
	case float64:
		// Check if it's a whole number (JSON numbers are float64)
		if val == float64(int64(val)) {
			return fmt.Sprintf("%d", int64(val))
		}
		return fmt.Sprintf("%v", val)
	case int64:
		return fmt.Sprintf("%d", val)
	case bool:
		return fmt.Sprintf("%t", val)
	default:
		// For complex types (arrays, objects), use JSON encoding
		b, err := json.Marshal(v)
		if err != nil {
			return fmt.Sprintf("%v", v)
		}
		return string(b)
	}
}

// sortedKeys returns the keys of a map sorted alphabetically.
// This ensures deterministic output when iterating over maps.
func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
