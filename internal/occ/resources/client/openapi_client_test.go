// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package client

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
)

func TestApiError(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		body       []byte
		wantMsg    string
	}{
		{
			name:       "structured error with message",
			statusCode: 404,
			body:       []byte(`{"code":"NOT_FOUND","error":"component \"foo\" not found"}`),
			wantMsg:    `component "foo" not found`,
		},
		{
			name:       "structured error with details",
			statusCode: 400,
			body:       []byte(`{"code":"VALIDATION_ERROR","error":"validation failed","details":[{"field":"name","message":"must not be empty"}]}`),
			wantMsg:    `validation failed; name: must not be empty`,
		},
		{
			name:       "structured error with multiple details",
			statusCode: 400,
			body:       []byte(`{"code":"VALIDATION_ERROR","error":"validation failed","details":[{"field":"name","message":"must not be empty"},{"field":"project","message":"is required"}]}`),
			wantMsg:    `validation failed; name: must not be empty; project: is required`,
		},
		{
			name:       "non-JSON body",
			statusCode: 502,
			body:       []byte(`Bad Gateway`),
			wantMsg:    `unexpected response (HTTP 502): Bad Gateway`,
		},
		{
			name:       "empty body",
			statusCode: 500,
			body:       nil,
			wantMsg:    `unexpected response status: 500`,
		},
		{
			name:       "JSON without error field",
			statusCode: 503,
			body:       []byte(`{"status":"unavailable"}`),
			wantMsg:    `unexpected response (HTTP 503): {"status":"unavailable"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := apiError(tt.statusCode, tt.body)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if got := err.Error(); got != tt.wantMsg {
				t.Errorf("apiError() = %q, want %q", got, tt.wantMsg)
			}
		})
	}
}

func TestSchemaResponseToRaw(t *testing.T) {
	tests := []struct {
		name    string
		schema  *gen.SchemaResponse
		wantErr bool
	}{
		{
			name: "valid schema with properties",
			schema: &gen.SchemaResponse{
				"type": "object",
				"properties": map[string]interface{}{
					"port": map[string]interface{}{"type": "integer"},
				},
			},
		},
		{
			name:   "empty schema",
			schema: &gen.SchemaResponse{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			raw, err := schemaResponseToRaw(tt.schema)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, raw)

			// Verify the raw message can be unmarshalled back
			var result map[string]interface{}
			assert.NoError(t, json.Unmarshal(*raw, &result))
		})
	}
}

func TestSchemaResponseToRaw_RoundTrip(t *testing.T) {
	original := &gen.SchemaResponse{
		"type": "object",
		"properties": map[string]interface{}{
			"replicas": map[string]interface{}{"type": "integer"},
		},
	}

	raw, err := schemaResponseToRaw(original)
	require.NoError(t, err)
	require.NotNil(t, raw)

	// Verify the JSON contains expected fields
	var parsed map[string]interface{}
	require.NoError(t, json.Unmarshal(*raw, &parsed))
	assert.Equal(t, "object", parsed["type"])
	props, ok := parsed["properties"].(map[string]interface{})
	require.True(t, ok)
	assert.Contains(t, props, "replicas")
}
