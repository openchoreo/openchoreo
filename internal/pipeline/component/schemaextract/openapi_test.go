// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package schemaextract

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOpenAPIExtractor(t *testing.T) {
	yamlSpec := `
openapi: 3.0.0
info:
  title: Pets API
  version: "1.0"
paths:
  /v1/pets:
    get:
      operationId: listPets
    post:
      operationId: createPet
  /v1/pets/{id}:
    get:
      operationId: getPet
`
	jsonSpec := `{
  "openapi": "3.0.0",
  "info": {"title": "T", "version": "1.0"},
  "paths": {"/health": {"get": {}}}
}`

	tests := []struct {
		name    string
		content string
		want    []EndpointResource
		wantErr bool
	}{
		{
			name:    "yaml multi-path multi-method (sorted)",
			content: yamlSpec,
			want: []EndpointResource{
				{Kind: "HTTP", Method: "GET", Path: "/v1/pets"},
				{Kind: "HTTP", Method: "POST", Path: "/v1/pets"},
				{Kind: "HTTP", Method: "GET", Path: "/v1/pets/{id}"},
			},
		},
		{
			name:    "json input",
			content: jsonSpec,
			want:    []EndpointResource{{Kind: "HTTP", Method: "GET", Path: "/health"}},
		},
		{
			name:    "no paths yields empty",
			content: `{"openapi":"3.0.0","info":{"title":"t","version":"1.0"}}`,
			want:    []EndpointResource{},
		},
		{
			name:    "malformed content errors",
			content: "this is not: : valid: yaml: openapi",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := openAPIExtractor{}.Extract(tt.content)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			sortResources(got) // extractor returns unordered; Extract() sorts
			assert.Equal(t, tt.want, got)
		})
	}
}
