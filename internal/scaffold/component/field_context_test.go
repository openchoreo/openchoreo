// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package component

import (
	"reflect"
	"testing"

	extv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
)

func TestFieldContext_GetValueAsMap(t *testing.T) {
	tests := []struct {
		name  string
		value any
		want  map[string]any
	}{
		{"valid map", map[string]any{"k": "v"}, map[string]any{"k": "v"}},
		{"nil value", nil, map[string]any{}},
		{"non-map value", "string", map[string]any{}},
		{"int value", 42, map[string]any{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := &FieldContext{Value: tt.value}
			got := ctx.GetValueAsMap()
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("GetValueAsMap() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFieldContext_GetValueAsSlice(t *testing.T) {
	tests := []struct {
		name  string
		value any
		want  []any
	}{
		{"valid slice", []any{"a", "b"}, []any{"a", "b"}},
		{"nil value", nil, []any{}},
		{"non-slice value", "string", []any{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := &FieldContext{Value: tt.value}
			got := ctx.GetValueAsSlice()
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("GetValueAsSlice() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFieldContext_BuildFieldOptions(t *testing.T) {
	renderer := NewFieldRenderer(false, false, false)

	t.Run("with separator and comment", func(t *testing.T) {
		ctx := &FieldContext{AddSeparator: true, Renderer: renderer}
		opts := ctx.BuildFieldOptions("A comment")
		if len(opts) != 2 {
			t.Errorf("expected 2 options, got %d", len(opts))
		}
	})

	t.Run("no separator, no comment", func(t *testing.T) {
		ctx := &FieldContext{AddSeparator: false, Renderer: renderer}
		opts := ctx.BuildFieldOptions("")
		if len(opts) != 0 {
			t.Errorf("expected 0 options, got %d", len(opts))
		}
	})

	t.Run("separator only", func(t *testing.T) {
		ctx := &FieldContext{AddSeparator: true, Renderer: renderer}
		opts := ctx.BuildFieldOptions("")
		if len(opts) != 1 {
			t.Errorf("expected 1 option, got %d", len(opts))
		}
	})

	t.Run("comment only", func(t *testing.T) {
		ctx := &FieldContext{AddSeparator: false, Renderer: renderer}
		opts := ctx.BuildFieldOptions("comment")
		if len(opts) != 1 {
			t.Errorf("expected 1 option, got %d", len(opts))
		}
	})
}

func TestFieldContext_DetermineRenderMode(t *testing.T) {
	renderer := NewFieldRenderer(false, false, false)

	tests := []struct {
		name       string
		isRequired bool
		hasDefault bool
		want       RenderMode
	}{
		{"required without default", true, false, RenderActive},
		{"required with default", true, true, RenderCommented},
		{"optional without default", false, false, RenderCommented},
		{"optional with default", false, true, RenderCommented},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := &FieldContext{
				IsRequired: tt.isRequired,
				HasDefault: tt.hasDefault,
				Renderer:   renderer,
			}
			got := ctx.DetermineRenderMode()
			if got != tt.want {
				t.Errorf("DetermineRenderMode() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFieldContext_TypeChecks(t *testing.T) {
	tests := []struct {
		name        string
		schema      *extv1.JSONSchemaProps
		wantMap     bool
		wantObject  bool
		wantArray   bool
		wantPrimStr bool
	}{
		{
			name: "map field",
			schema: &extv1.JSONSchemaProps{
				Type: typeObject,
				AdditionalProperties: &extv1.JSONSchemaPropsOrBool{
					Schema: &extv1.JSONSchemaProps{Type: typeString},
				},
			},
			wantMap: true,
		},
		{
			name: "object field",
			schema: &extv1.JSONSchemaProps{
				Type: typeObject,
				Properties: map[string]extv1.JSONSchemaProps{
					"name": {Type: typeString},
				},
			},
			wantObject: true,
		},
		{
			name:      "array field",
			schema:    &extv1.JSONSchemaProps{Type: typeArray},
			wantArray: true,
		},
		{
			name:        "string field",
			schema:      &extv1.JSONSchemaProps{Type: typeString},
			wantPrimStr: true,
		},
		{
			name:        "integer field",
			schema:      &extv1.JSONSchemaProps{Type: typeInteger},
			wantPrimStr: true,
		},
		{
			name:        "boolean field",
			schema:      &extv1.JSONSchemaProps{Type: typeBoolean},
			wantPrimStr: true,
		},
		{
			name:        "number field",
			schema:      &extv1.JSONSchemaProps{Type: typeNumber},
			wantPrimStr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := &FieldContext{Schema: tt.schema}
			if got := ctx.IsMapField(); got != tt.wantMap {
				t.Errorf("IsMapField() = %v, want %v", got, tt.wantMap)
			}
			if got := ctx.IsObjectField(); got != tt.wantObject {
				t.Errorf("IsObjectField() = %v, want %v", got, tt.wantObject)
			}
			if got := ctx.IsArrayField(); got != tt.wantArray {
				t.Errorf("IsArrayField() = %v, want %v", got, tt.wantArray)
			}
			if got := ctx.IsPrimitiveField(); got != tt.wantPrimStr {
				t.Errorf("IsPrimitiveField() = %v, want %v", got, tt.wantPrimStr)
			}
		})
	}
}

func TestFieldContext_GetArrayItemSchema(t *testing.T) {
	t.Run("with items", func(t *testing.T) {
		itemSchema := &extv1.JSONSchemaProps{Type: typeString}
		ctx := &FieldContext{
			Schema: &extv1.JSONSchemaProps{
				Type: typeArray,
				Items: &extv1.JSONSchemaPropsOrArray{
					Schema: itemSchema,
				},
			},
		}
		got := ctx.GetArrayItemSchema()
		if got != itemSchema {
			t.Errorf("GetArrayItemSchema() = %v, want %v", got, itemSchema)
		}
	})

	t.Run("nil items", func(t *testing.T) {
		ctx := &FieldContext{Schema: &extv1.JSONSchemaProps{Type: typeArray}}
		if got := ctx.GetArrayItemSchema(); got != nil {
			t.Errorf("GetArrayItemSchema() = %v, want nil", got)
		}
	})
}

func TestFieldContext_GetMapValueSchema(t *testing.T) {
	t.Run("with additionalProperties", func(t *testing.T) {
		valueSchema := &extv1.JSONSchemaProps{Type: typeString}
		ctx := &FieldContext{
			Schema: &extv1.JSONSchemaProps{
				Type: typeObject,
				AdditionalProperties: &extv1.JSONSchemaPropsOrBool{
					Schema: valueSchema,
				},
			},
		}
		got := ctx.GetMapValueSchema()
		if got != valueSchema {
			t.Errorf("GetMapValueSchema() = %v, want %v", got, valueSchema)
		}
	})

	t.Run("nil additionalProperties", func(t *testing.T) {
		ctx := &FieldContext{Schema: &extv1.JSONSchemaProps{Type: typeObject}}
		if got := ctx.GetMapValueSchema(); got != nil {
			t.Errorf("GetMapValueSchema() = %v, want nil", got)
		}
	})
}

func TestFieldContext_ShouldOmitOptionalField(t *testing.T) {
	tests := []struct {
		name             string
		isRequired       bool
		hasDefault       bool
		includeAllFields bool
		want             bool
	}{
		{"optional no default includeAll=false", false, false, false, true},
		{"optional no default includeAll=true", false, false, true, false},
		{"optional with default", false, true, false, false},
		{"required no default", true, false, false, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			renderer := NewFieldRenderer(false, tt.includeAllFields, false)
			ctx := &FieldContext{
				IsRequired: tt.isRequired,
				HasDefault: tt.hasDefault,
				Renderer:   renderer,
			}
			if got := ctx.ShouldOmitOptionalField(); got != tt.want {
				t.Errorf("ShouldOmitOptionalField() = %v, want %v", got, tt.want)
			}
		})
	}
}
