// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package schemautil

import (
	"testing"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
)

// mockSchemaSource is a test implementation of schema.SchemaSource
type mockSchemaSource struct {
	types        *runtime.RawExtension
	parameters   *runtime.RawExtension
	envOverrides *runtime.RawExtension
}

func (m *mockSchemaSource) GetTypes() *runtime.RawExtension      { return m.types }
func (m *mockSchemaSource) GetParameters() *runtime.RawExtension { return m.parameters }
func (m *mockSchemaSource) GetEnvOverrides() *runtime.RawExtension {
	return m.envOverrides
}

func TestExtractStructuralSchemas_ValidSchemas(t *testing.T) {
	source := &mockSchemaSource{
		parameters: &runtime.RawExtension{
			Raw: []byte(`{"replicas": "integer | default=1", "name": "string"}`),
		},
		envOverrides: &runtime.RawExtension{
			Raw: []byte(`{"environment": "string | default=dev"}`),
		},
	}

	basePath := field.NewPath("spec", "schema")
	paramsSchema, envSchema, errs := ExtractStructuralSchemas(source, basePath)

	if len(errs) != 0 {
		t.Fatalf("expected no errors, got %v", errs)
	}
	if paramsSchema == nil {
		t.Fatal("expected parameters schema to be non-nil")
	}
	if envSchema == nil {
		t.Fatal("expected envOverrides schema to be non-nil")
	}

	// Verify parameters schema has the expected properties
	if _, ok := paramsSchema.Properties["replicas"]; !ok {
		t.Error("expected replicas property in parameters schema")
	}
	if _, ok := paramsSchema.Properties["name"]; !ok {
		t.Error("expected name property in parameters schema")
	}

	// Verify envOverrides schema has the expected properties
	if _, ok := envSchema.Properties["environment"]; !ok {
		t.Error("expected environment property in envOverrides schema")
	}
}

func TestExtractStructuralSchemas_WithTypes(t *testing.T) {
	source := &mockSchemaSource{
		types: &runtime.RawExtension{
			Raw: []byte(`{"Port": {"containerPort": "integer", "protocol": "string | default=TCP"}}`),
		},
		parameters: &runtime.RawExtension{
			Raw: []byte(`{"ports": "[]Port"}`),
		},
	}

	basePath := field.NewPath("spec", "schema")
	paramsSchema, envSchema, errs := ExtractStructuralSchemas(source, basePath)

	if len(errs) != 0 {
		t.Fatalf("expected no errors, got %v", errs)
	}
	if paramsSchema == nil {
		t.Fatal("expected parameters schema to be non-nil")
	}
	if envSchema != nil {
		t.Fatal("expected envOverrides schema to be nil when not provided")
	}

	// Verify ports array property exists
	if _, ok := paramsSchema.Properties["ports"]; !ok {
		t.Error("expected ports property in parameters schema")
	}
}

func TestExtractStructuralSchemas_EmptySchemas(t *testing.T) {
	source := &mockSchemaSource{}

	basePath := field.NewPath("spec", "schema")
	paramsSchema, envSchema, errs := ExtractStructuralSchemas(source, basePath)

	if len(errs) != 0 {
		t.Fatalf("expected no errors, got %v", errs)
	}
	if paramsSchema != nil {
		t.Fatal("expected parameters schema to be nil when not provided")
	}
	if envSchema != nil {
		t.Fatal("expected envOverrides schema to be nil when not provided")
	}
}

func TestExtractStructuralSchemas_InvalidYAML(t *testing.T) {
	tests := []struct {
		name     string
		source   *mockSchemaSource
		wantPath string
	}{
		{
			name: "invalid types YAML",
			source: &mockSchemaSource{
				types: &runtime.RawExtension{
					Raw: []byte(`{invalid yaml`),
				},
			},
			wantPath: "spec.schema.types",
		},
		{
			name: "invalid parameters YAML",
			source: &mockSchemaSource{
				parameters: &runtime.RawExtension{
					Raw: []byte(`{invalid yaml`),
				},
			},
			wantPath: "spec.schema.parameters",
		},
		{
			name: "invalid envOverrides YAML",
			source: &mockSchemaSource{
				envOverrides: &runtime.RawExtension{
					Raw: []byte(`{invalid yaml`),
				},
			},
			wantPath: "spec.schema.envOverrides",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			basePath := field.NewPath("spec", "schema")
			_, _, errs := ExtractStructuralSchemas(tc.source, basePath)

			if len(errs) == 0 {
				t.Fatal("expected errors for invalid YAML")
			}
			if errs[0].Field != tc.wantPath {
				t.Errorf("expected error path %q, got %q", tc.wantPath, errs[0].Field)
			}
		})
	}
}
