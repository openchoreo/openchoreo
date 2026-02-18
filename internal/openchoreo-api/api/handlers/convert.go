// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"encoding/json"
	"fmt"
)

// convert converts between two types using JSON marshal/unmarshal round-trip.ß
// Used to convert between Kubernetes CRD types and OpenAPI generated types.
func convert[S any, D any](src S) (D, error) {
	var dst D
	data, err := json.Marshal(src)
	if err != nil {
		return dst, fmt.Errorf("marshal source: %w", err)
	}
	if err := json.Unmarshal(data, &dst); err != nil {
		return dst, fmt.Errorf("unmarshal into destination: %w", err)
	}
	return dst, nil
}

// convertList converts a slice of items between two types using JSON round-trip.
func convertList[S any, D any](items []S) ([]D, error) {
	result := make([]D, 0, len(items))
	for i, item := range items {
		converted, err := convert[S, D](item)
		if err != nil {
			return nil, fmt.Errorf("item %d: %w", i, err)
		}
		result = append(result, converted)
	}
	return result, nil
}
