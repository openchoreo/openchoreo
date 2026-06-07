// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package schemaextract

import (
	"fmt"

	"github.com/getkin/kin-openapi/openapi3"
)

// openAPIExtractor extracts HTTP routes from an OpenAPI (v3) document. It emits
// one EndpointResource per (path, operation) pair.
type openAPIExtractor struct{}

func (openAPIExtractor) Extract(content string) ([]EndpointResource, error) {
	loader := openapi3.NewLoader()
	// Never fetch remote refs while rendering inside the controller.
	loader.IsExternalRefsAllowed = false

	doc, err := loader.LoadFromData([]byte(content))
	if err != nil {
		return nil, fmt.Errorf("load openapi document: %w", err)
	}
	if doc == nil || doc.Paths == nil {
		return empty(), nil
	}

	// Intentionally skip doc.Validate(): we only need the path/method structure,
	// and strict validation would reject otherwise-usable specs.
	var out []EndpointResource
	for path, item := range doc.Paths.Map() {
		if item == nil {
			continue
		}
		for method := range item.Operations() {
			out = append(out, EndpointResource{
				Kind:   "HTTP",
				Method: method, // upper-case HTTP verb (GET, POST, ...)
				Path:   path,   // raw template path, e.g. /users/{id}
			})
		}
	}
	return out, nil
}
