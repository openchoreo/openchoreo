// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package workflow

import (
	"fmt"
	"strings"
)

// ParseSetParameters parses --set key=value pairs into nested map structure
// Example: "systemParameters.repository.url=https://github.com/org/repo"
// Returns: {"systemParameters": {"repository": {"url": "https://github.com/org/repo"}}}
func ParseSetParameters(setValues []string) (map[string]interface{}, error) {
	result := make(map[string]interface{})

	for _, setValue := range setValues {
		parts := strings.SplitN(setValue, "=", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid --set format '%s', expected: key=value", setValue)
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		if key == "" {
			return nil, fmt.Errorf("empty key in --set flag")
		}

		setNestedValue(result, key, value)
	}

	return result, nil
}

// setNestedValue sets a value in a nested map using dot notation
func setNestedValue(m map[string]interface{}, key string, value interface{}) {
	keys := strings.Split(key, ".")
	current := m

	// Navigate/create nested maps
	for i := 0; i < len(keys)-1; i++ {
		k := keys[i]
		if _, exists := current[k]; !exists {
			current[k] = make(map[string]interface{})
		}
		// Type assert to navigate deeper
		if nested, ok := current[k].(map[string]interface{}); ok {
			current = nested
		} else {
			// If the value at this key is not a map, we can't navigate deeper
			// Overwrite it with a new map
			newMap := make(map[string]interface{})
			current[k] = newMap
			current = newMap
		}
	}

	// Set the final value
	current[keys[len(keys)-1]] = value
}
