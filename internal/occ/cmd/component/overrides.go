// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package component

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/tidwall/sjson"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
)

// mergeOverridesWithBinding merges --set override values with existing ReleaseBinding
// This uses sjson to generically update JSON paths in the existing binding
func mergeOverridesWithBinding(existingBinding *gen.ReleaseBinding, setValues []string) (*gen.ReleaseBinding, error) {
	// Marshal existing binding to JSON
	existingJSON, err := json.Marshal(existingBinding)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal existing binding: %w", err)
	}

	jsonStr := string(existingJSON)

	// Apply each --set value using sjson
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

		// Use sjson to update the value at the given path
		jsonStr, err = sjson.Set(jsonStr, key, value)
		if err != nil {
			return nil, fmt.Errorf("failed to set value for key '%s': %w", key, err)
		}
	}

	// Unmarshal back to ReleaseBinding
	var rb gen.ReleaseBinding
	if err := json.Unmarshal([]byte(jsonStr), &rb); err != nil {
		return nil, fmt.Errorf("failed to unmarshal merged result: %w", err)
	}

	return &rb, nil
}
