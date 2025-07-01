// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package render

import (
	"encoding/json"

	"k8s.io/apimachinery/pkg/util/strategicpatch"
)

// merge applies a strategic merge patch to the base object using the overlay object.
func merge[T any](base, overlay *T) (*T, error) {
	if base == nil || overlay == nil {
		return nil, nil
	}

	origJSON, err := json.Marshal(base)
	if err != nil {
		return nil, err
	}

	patchJSON, err := json.Marshal(overlay)
	if err != nil {
		return nil, err
	}

	var zero T
	mergedJSON, err := strategicpatch.StrategicMergePatch(origJSON, patchJSON, zero)
	if err != nil {
		return nil, err
	}

	var merged T
	if err := json.Unmarshal(mergedJSON, &merged); err != nil {
		return nil, err
	}
	return &merged, nil
}
