// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package component

import (
	"reflect"
	"testing"

	extv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
)

func TestApplyDefaultsToSchema_NilSchema(t *testing.T) {
	result, err := applyDefaultsToSchema(nil)
	if err != nil {
		t.Fatalf("applyDefaultsToSchema(nil) error: %v", err)
	}
	if result.jsonSchema != nil {
		t.Errorf("expected nil jsonSchema, got %v", result.jsonSchema)
	}
	if result.structural != nil {
		t.Errorf("expected nil structural, got %v", result.structural)
	}
	if len(result.defaultedObj) != 0 {
		t.Errorf("expected empty defaultedObj, got %v", result.defaultedObj)
	}
}

func TestApplyDefaultsToSchema_ValidSchema(t *testing.T) {
	schema := &extv1.JSONSchemaProps{
		Type: typeObject,
		Properties: map[string]extv1.JSONSchemaProps{
			"replicas": {
				Type:    typeInteger,
				Default: &extv1.JSON{Raw: []byte(`3`)},
			},
			"name": {
				Type: typeString,
			},
		},
	}

	result, err := applyDefaultsToSchema(schema)
	if err != nil {
		t.Fatalf("applyDefaultsToSchema() error: %v", err)
	}
	if result.jsonSchema == nil {
		t.Fatal("expected non-nil jsonSchema")
	}
	if result.structural == nil {
		t.Fatal("expected non-nil structural")
	}
	// The default for replicas should be applied
	if val, ok := result.defaultedObj["replicas"]; !ok {
		t.Error("expected replicas in defaultedObj")
	} else if val != int64(3) {
		t.Errorf("expected replicas=3, got %v (%T)", val, val)
	}
}

func TestBuildEmptyStructure(t *testing.T) {
	t.Run("nil schema", func(t *testing.T) {
		got := buildEmptyStructure(nil)
		if len(got) != 0 {
			t.Errorf("expected empty map for nil schema, got %v", got)
		}
	})

	t.Run("nil properties", func(t *testing.T) {
		got := buildEmptyStructure(&extv1.JSONSchemaProps{Type: typeObject})
		if len(got) != 0 {
			t.Errorf("expected empty map for nil properties, got %v", got)
		}
	})

	t.Run("nested object with empty default", func(t *testing.T) {
		schema := &extv1.JSONSchemaProps{
			Type: typeObject,
			Properties: map[string]extv1.JSONSchemaProps{
				"nested": {
					Type: typeObject,
					Properties: map[string]extv1.JSONSchemaProps{
						"inner": {Type: typeString},
					},
					Default: &extv1.JSON{Raw: []byte(`{}`)},
				},
			},
		}
		got := buildEmptyStructure(schema)
		if _, ok := got["nested"]; !ok {
			t.Error("expected nested key in result")
		}
	})

	t.Run("nested object with non-empty default", func(t *testing.T) {
		schema := &extv1.JSONSchemaProps{
			Type: typeObject,
			Properties: map[string]extv1.JSONSchemaProps{
				"config": {
					Type: typeObject,
					Properties: map[string]extv1.JSONSchemaProps{
						"mode": {Type: typeString},
					},
					Default: &extv1.JSON{Raw: []byte(`{"mode": "production"}`)},
				},
			},
		}
		got := buildEmptyStructure(schema)
		// Objects with non-empty defaults should NOT be in the empty structure
		if _, ok := got["config"]; ok {
			t.Error("objects with non-empty defaults should be skipped")
		}
	})

	t.Run("nested object without default", func(t *testing.T) {
		schema := &extv1.JSONSchemaProps{
			Type: typeObject,
			Properties: map[string]extv1.JSONSchemaProps{
				"nested": {
					Type: typeObject,
					Properties: map[string]extv1.JSONSchemaProps{
						"inner": {Type: typeString},
					},
				},
			},
		}
		got := buildEmptyStructure(schema)
		if _, ok := got["nested"]; !ok {
			t.Error("expected nested key for object without default")
		}
	})

	t.Run("array of objects", func(t *testing.T) {
		schema := &extv1.JSONSchemaProps{
			Type: typeObject,
			Properties: map[string]extv1.JSONSchemaProps{
				"servers": {
					Type: typeArray,
					Items: &extv1.JSONSchemaPropsOrArray{
						Schema: &extv1.JSONSchemaProps{
							Type: typeObject,
							Properties: map[string]extv1.JSONSchemaProps{
								"name": {Type: typeString},
							},
						},
					},
				},
			},
		}
		got := buildEmptyStructure(schema)
		arr, ok := got["servers"].([]any)
		if !ok {
			t.Fatal("expected servers array")
		}
		if len(arr) != 2 {
			t.Errorf("expected 2 example items, got %d", len(arr))
		}
	})

	t.Run("array without item schema", func(t *testing.T) {
		schema := &extv1.JSONSchemaProps{
			Type: typeObject,
			Properties: map[string]extv1.JSONSchemaProps{
				"tags": {
					Type: typeArray,
				},
			},
		}
		got := buildEmptyStructure(schema)
		arr, ok := got["tags"].([]any)
		if !ok {
			t.Fatal("expected tags array")
		}
		if len(arr) != 0 {
			t.Errorf("expected empty array for no item schema, got %d items", len(arr))
		}
	})

	t.Run("array with non-empty default", func(t *testing.T) {
		schema := &extv1.JSONSchemaProps{
			Type: typeObject,
			Properties: map[string]extv1.JSONSchemaProps{
				"items": {
					Type:    typeArray,
					Default: &extv1.JSON{Raw: []byte(`["a","b"]`)},
					Items: &extv1.JSONSchemaPropsOrArray{
						Schema: &extv1.JSONSchemaProps{Type: typeString},
					},
				},
			},
		}
		got := buildEmptyStructure(schema)
		// Arrays with non-empty defaults should NOT be in the empty structure
		if _, ok := got["items"]; ok {
			t.Error("arrays with non-empty defaults should be skipped")
		}
	})

	t.Run("primitive properties are skipped", func(t *testing.T) {
		schema := &extv1.JSONSchemaProps{
			Type: typeObject,
			Properties: map[string]extv1.JSONSchemaProps{
				"name": {Type: typeString},
				"port": {Type: typeInteger},
			},
		}
		got := buildEmptyStructure(schema)
		if len(got) != 0 {
			t.Errorf("expected empty map for primitives only, got %v", got)
		}
	})
}

func TestBuildEmptyArrayStructure(t *testing.T) {
	t.Run("nil items", func(t *testing.T) {
		got := buildEmptyArrayStructure(&extv1.JSONSchemaProps{Type: typeArray})
		if len(got) != 0 {
			t.Errorf("expected empty array, got %v", got)
		}
	})

	t.Run("nil item schema", func(t *testing.T) {
		got := buildEmptyArrayStructure(&extv1.JSONSchemaProps{
			Type:  typeArray,
			Items: &extv1.JSONSchemaPropsOrArray{Schema: nil},
		})
		if len(got) != 0 {
			t.Errorf("expected empty array for nil item schema, got %v", got)
		}
	})

	t.Run("object items", func(t *testing.T) {
		got := buildEmptyArrayStructure(&extv1.JSONSchemaProps{
			Type: typeArray,
			Items: &extv1.JSONSchemaPropsOrArray{
				Schema: &extv1.JSONSchemaProps{
					Type: typeObject,
					Properties: map[string]extv1.JSONSchemaProps{
						"name": {Type: typeString},
					},
				},
			},
		})
		if len(got) != 2 {
			t.Errorf("expected 2 example items, got %d", len(got))
		}
	})

	t.Run("primitive items", func(t *testing.T) {
		got := buildEmptyArrayStructure(&extv1.JSONSchemaProps{
			Type: typeArray,
			Items: &extv1.JSONSchemaPropsOrArray{
				Schema: &extv1.JSONSchemaProps{Type: typeString},
			},
		})
		if len(got) != 0 {
			t.Errorf("expected empty array for primitive items, got %v", got)
		}
	})
}

func TestIsEmptyDefault(t *testing.T) {
	tests := []struct {
		name string
		def  *extv1.JSON
		want bool
	}{
		{"nil", nil, true},
		{"empty raw", &extv1.JSON{Raw: []byte{}}, true},
		{"empty object", &extv1.JSON{Raw: []byte(`{}`)}, true},
		{"null", &extv1.JSON{Raw: []byte(`null`)}, true},
		{"empty object with spaces", &extv1.JSON{Raw: []byte(`  {}  `)}, true},
		{"non-empty object", &extv1.JSON{Raw: []byte(`{"key": "val"}`)}, false},
		{"string value", &extv1.JSON{Raw: []byte(`"hello"`)}, false},
		{"number value", &extv1.JSON{Raw: []byte(`42`)}, false},
		{"array value", &extv1.JSON{Raw: []byte(`[1,2]`)}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isEmptyDefault(tt.def); got != tt.want {
				t.Errorf("isEmptyDefault() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSchemaToStructural_ValidSchema(t *testing.T) {
	schema := &extv1.JSONSchemaProps{
		Type: typeObject,
		Properties: map[string]extv1.JSONSchemaProps{
			"name": {Type: typeString},
		},
	}

	structural, err := schemaToStructural(schema)
	if err != nil {
		t.Fatalf("schemaToStructural() error: %v", err)
	}
	if structural == nil {
		t.Fatal("expected non-nil structural schema")
	}
}

func TestFormatDefaultValue(t *testing.T) {
	tests := []struct {
		name  string
		value any
		want  string
	}{
		{"nil", nil, ""},
		{"string", "hello", "hello"},
		{"whole float64", float64(42), "42"},
		{"decimal float64", 3.14, "3.14"},
		{"int64", int64(99), "99"},
		{"bool true", true, "true"},
		{"bool false", false, "false"},
		{"slice", []string{"a", "b"}, `["a","b"]`},
		{"map", map[string]string{"k": "v"}, `{"k":"v"}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatDefaultValue(tt.value)
			if got != tt.want {
				t.Errorf("formatDefaultValue(%v) = %q, want %q", tt.value, got, tt.want)
			}
		})
	}
}

func TestSortedKeys(t *testing.T) {
	m := map[string]any{
		"zebra": 1,
		"alpha": 2,
		"mid":   3,
	}
	got := sortedKeys(m)
	want := []string{"alpha", "mid", "zebra"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("sortedKeys() = %v, want %v", got, want)
	}
}
