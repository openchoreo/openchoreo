// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package schemautil

import (
	"testing"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"

	"github.com/openchoreo/openchoreo/api/v1alpha1"
)

func TestExtractStructuralSchemas_ValidSchemas(t *testing.T) {
	params := &v1alpha1.SchemaSection{
		OCSchema: &runtime.RawExtension{
			Raw: []byte(`{"replicas": "integer | default=1", "name": "string"}`),
		},
	}
	envConfigs := &v1alpha1.SchemaSection{
		OCSchema: &runtime.RawExtension{
			Raw: []byte(`{"environment": "string | default=dev"}`),
		},
	}

	basePath := field.NewPath("spec")
	paramsSchema, envSchema, errs := ExtractStructuralSchemas(params, envConfigs, basePath)

	if len(errs) != 0 {
		t.Fatalf("expected no errors, got %v", errs)
	}
	if paramsSchema == nil {
		t.Fatal("expected parameters schema to be non-nil")
	}
	if envSchema == nil {
		t.Fatal("expected environmentConfigs schema to be non-nil")
	}

	// Verify parameters schema has the expected properties
	if _, ok := paramsSchema.Properties["replicas"]; !ok {
		t.Error("expected replicas property in parameters schema")
	}
	if _, ok := paramsSchema.Properties["name"]; !ok {
		t.Error("expected name property in parameters schema")
	}

	// Verify environmentConfigs schema has the expected properties
	if _, ok := envSchema.Properties["environment"]; !ok {
		t.Error("expected environment property in environmentConfigs schema")
	}
}

func TestExtractStructuralSchemas_WithTypes(t *testing.T) {
	params := &v1alpha1.SchemaSection{
		OCSchema: &runtime.RawExtension{
			Raw: []byte(`{"$types": {"Port": {"containerPort": "integer", "protocol": "string | default=TCP"}}, "ports": "[]Port"}`),
		},
	}

	basePath := field.NewPath("spec")
	paramsSchema, envSchema, errs := ExtractStructuralSchemas(params, nil, basePath)

	if len(errs) != 0 {
		t.Fatalf("expected no errors, got %v", errs)
	}
	if paramsSchema == nil {
		t.Fatal("expected parameters schema to be non-nil")
	}
	if envSchema != nil {
		t.Fatal("expected environmentConfigs schema to be nil when not provided")
	}

	// Verify ports array property exists
	if _, ok := paramsSchema.Properties["ports"]; !ok {
		t.Error("expected ports property in parameters schema")
	}
}

func TestExtractStructuralSchemas_EmptySchemas(t *testing.T) {
	basePath := field.NewPath("spec")
	paramsSchema, envSchema, errs := ExtractStructuralSchemas(nil, nil, basePath)

	if len(errs) != 0 {
		t.Fatalf("expected no errors, got %v", errs)
	}
	if paramsSchema != nil {
		t.Fatal("expected parameters schema to be nil when not provided")
	}
	if envSchema != nil {
		t.Fatal("expected environmentConfigs schema to be nil when not provided")
	}
}

func TestExtractStructuralSchemas_FormatConsistency(t *testing.T) {
	tests := []struct {
		name       string
		parameters *v1alpha1.SchemaSection
		envConfigs *v1alpha1.SchemaSection
		wantErr    bool
	}{
		{
			name: "both ocSchema - valid",
			parameters: &v1alpha1.SchemaSection{
				OCSchema: &runtime.RawExtension{
					Raw: []byte(`{"name": "string"}`),
				},
			},
			envConfigs: &v1alpha1.SchemaSection{
				OCSchema: &runtime.RawExtension{
					Raw: []byte(`{"replicas": "integer | default=1"}`),
				},
			},
			wantErr: false,
		},
		{
			name: "both openAPIV3Schema - valid",
			parameters: &v1alpha1.SchemaSection{
				OpenAPIV3Schema: &runtime.RawExtension{
					Raw: []byte(`{"type": "object", "properties": {"name": {"type": "string"}}, "required": ["name"]}`),
				},
			},
			envConfigs: &v1alpha1.SchemaSection{
				OpenAPIV3Schema: &runtime.RawExtension{
					Raw: []byte(`{"type": "object", "properties": {"replicas": {"type": "integer", "default": 1}}}`),
				},
			},
			wantErr: false,
		},
		{
			name: "parameters ocSchema, envConfigs openAPIV3Schema - invalid",
			parameters: &v1alpha1.SchemaSection{
				OCSchema: &runtime.RawExtension{
					Raw: []byte(`{"name": "string"}`),
				},
			},
			envConfigs: &v1alpha1.SchemaSection{
				OpenAPIV3Schema: &runtime.RawExtension{
					Raw: []byte(`{"type": "object", "properties": {"replicas": {"type": "integer", "default": 1}}}`),
				},
			},
			wantErr: true,
		},
		{
			name: "parameters openAPIV3Schema, envConfigs ocSchema - invalid",
			parameters: &v1alpha1.SchemaSection{
				OpenAPIV3Schema: &runtime.RawExtension{
					Raw: []byte(`{"type": "object", "properties": {"name": {"type": "string"}}, "required": ["name"]}`),
				},
			},
			envConfigs: &v1alpha1.SchemaSection{
				OCSchema: &runtime.RawExtension{
					Raw: []byte(`{"replicas": "integer | default=1"}`),
				},
			},
			wantErr: true,
		},
		{
			name: "only parameters set - valid",
			parameters: &v1alpha1.SchemaSection{
				OCSchema: &runtime.RawExtension{
					Raw: []byte(`{"name": "string"}`),
				},
			},
			envConfigs: nil,
			wantErr:    false,
		},
		{
			name:       "only envConfigs set - valid",
			parameters: nil,
			envConfigs: &v1alpha1.SchemaSection{
				OpenAPIV3Schema: &runtime.RawExtension{
					Raw: []byte(`{"type": "object", "properties": {"replicas": {"type": "integer", "default": 1}}}`),
				},
			},
			wantErr: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			basePath := field.NewPath("spec")
			_, _, errs := ExtractStructuralSchemas(tc.parameters, tc.envConfigs, basePath)

			hasConsistencyErr := false
			for _, e := range errs {
				if e.Field == "spec.environmentConfigs" && e.Detail == "parameters and environmentConfigs must use the same schema format (both ocSchema or both openAPIV3Schema)" {
					hasConsistencyErr = true
					break
				}
			}

			if tc.wantErr && !hasConsistencyErr {
				t.Error("expected schema format consistency error, got none")
			}
			if !tc.wantErr && hasConsistencyErr {
				t.Error("unexpected schema format consistency error")
			}
		})
	}
}

func TestExtractStructuralSchemas_InvalidYAML(t *testing.T) {
	tests := []struct {
		name       string
		parameters *v1alpha1.SchemaSection
		envConfigs *v1alpha1.SchemaSection
		wantPath   string
	}{
		{
			name: "invalid parameters YAML",
			parameters: &v1alpha1.SchemaSection{
				OCSchema: &runtime.RawExtension{
					Raw: []byte(`{invalid yaml`),
				},
			},
			wantPath: "spec.parameters",
		},
		{
			name: "invalid environmentConfigs YAML",
			envConfigs: &v1alpha1.SchemaSection{
				OCSchema: &runtime.RawExtension{
					Raw: []byte(`{invalid yaml`),
				},
			},
			wantPath: "spec.environmentConfigs",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			basePath := field.NewPath("spec")
			_, _, errs := ExtractStructuralSchemas(tc.parameters, tc.envConfigs, basePath)

			if len(errs) == 0 {
				t.Fatal("expected errors for invalid YAML")
			}
			if errs[0].Field != tc.wantPath {
				t.Errorf("expected error path %q, got %q", tc.wantPath, errs[0].Field)
			}
		})
	}
}
