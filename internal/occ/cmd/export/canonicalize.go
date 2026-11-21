// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package export

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
)

// Canonicalize copies documents into a consistent JSON representation. Encoding
// sorts mapping keys recursively; array order and numeric precision are retained.
// It does not remove fields, evaluate templates, or change configuration values.
func Canonicalize(ctx context.Context, resources []Resource) ([]Resource, error) {
	canonical := make([]Resource, 0, len(resources))
	for _, resource := range resources {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		data, err := json.Marshal(resource.Object)
		if err != nil {
			return nil, fmt.Errorf("canonicalize %s %q: encode document: %w", resource.Kind, resource.Name, err)
		}
		decoder := json.NewDecoder(bytes.NewReader(data))
		decoder.UseNumber()
		var document map[string]any
		if err := decoder.Decode(&document); err != nil {
			return nil, fmt.Errorf("canonicalize %s %q: read document: %w", resource.Kind, resource.Name, err)
		}
		if document == nil {
			return nil, fmt.Errorf("canonicalize %s %q: object is required", resource.Kind, resource.Name)
		}
		resource.Object = document
		canonical = append(canonical, resource)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return canonical, nil
}
