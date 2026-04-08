// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package component

import (
	"reflect"
	"testing"

	extv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
)

func TestGenerateExampleValueForType(t *testing.T) {
	tests := []struct {
		name   string
		schema *extv1.JSONSchemaProps
		want   any
	}{
		{
			name:   "string",
			schema: &extv1.JSONSchemaProps{Type: typeString},
			want:   "example",
		},
		{
			name:   "integer",
			schema: &extv1.JSONSchemaProps{Type: typeInteger},
			want:   0,
		},
		{
			name:   "number",
			schema: &extv1.JSONSchemaProps{Type: typeNumber},
			want:   0.0,
		},
		{
			name:   "boolean",
			schema: &extv1.JSONSchemaProps{Type: typeBoolean},
			want:   false,
		},
		{
			name:   "unknown type",
			schema: &extv1.JSONSchemaProps{Type: "custom"},
			want:   "example",
		},
		{
			name: "object with properties",
			schema: &extv1.JSONSchemaProps{
				Type: typeObject,
				Properties: map[string]extv1.JSONSchemaProps{
					"name": {Type: typeString},
					"port": {Type: typeInteger},
				},
			},
			want: map[string]any{"name": "example", "port": 0},
		},
		{
			name:   "object without properties",
			schema: &extv1.JSONSchemaProps{Type: typeObject},
			want:   map[string]any{},
		},
		{
			name: "array of objects",
			schema: &extv1.JSONSchemaProps{
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
			want: []any{
				map[string]any{"name": "example"},
				map[string]any{"name": "example"},
			},
		},
		{
			name: "array of primitives",
			schema: &extv1.JSONSchemaProps{
				Type: typeArray,
				Items: &extv1.JSONSchemaPropsOrArray{
					Schema: &extv1.JSONSchemaProps{Type: typeString},
				},
			},
			want: []any{"example", "example"},
		},
		{
			name: "array with no items schema",
			schema: &extv1.JSONSchemaProps{
				Type: typeArray,
			},
			want: []any{"example", "example"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := generateExampleValueForType(tt.schema)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("generateExampleValueForType() = %v (%T), want %v (%T)", got, got, tt.want, tt.want)
			}
		})
	}
}

func TestGenerateExamplePrimitiveArrayForType(t *testing.T) {
	tests := []struct {
		name   string
		schema *extv1.JSONSchemaProps
		want   []any
	}{
		{
			name: "string items",
			schema: &extv1.JSONSchemaProps{
				Type: typeArray,
				Items: &extv1.JSONSchemaPropsOrArray{
					Schema: &extv1.JSONSchemaProps{Type: typeString},
				},
			},
			want: []any{"example", "example"},
		},
		{
			name: "integer items",
			schema: &extv1.JSONSchemaProps{
				Type: typeArray,
				Items: &extv1.JSONSchemaPropsOrArray{
					Schema: &extv1.JSONSchemaProps{Type: typeInteger},
				},
			},
			want: []any{0, 0},
		},
		{
			name: "number items",
			schema: &extv1.JSONSchemaProps{
				Type: typeArray,
				Items: &extv1.JSONSchemaPropsOrArray{
					Schema: &extv1.JSONSchemaProps{Type: typeNumber},
				},
			},
			want: []any{0.0, 0.0},
		},
		{
			name: "boolean items",
			schema: &extv1.JSONSchemaProps{
				Type: typeArray,
				Items: &extv1.JSONSchemaPropsOrArray{
					Schema: &extv1.JSONSchemaProps{Type: typeBoolean},
				},
			},
			want: []any{false, false},
		},
		{
			name:   "nil items schema",
			schema: &extv1.JSONSchemaProps{Type: typeArray},
			want:   []any{"example", "example"},
		},
		{
			name: "nil inner schema",
			schema: &extv1.JSONSchemaProps{
				Type:  typeArray,
				Items: &extv1.JSONSchemaPropsOrArray{Schema: nil},
			},
			want: []any{"example", "example"},
		},
		{
			name: "unknown item type",
			schema: &extv1.JSONSchemaProps{
				Type: typeArray,
				Items: &extv1.JSONSchemaPropsOrArray{
					Schema: &extv1.JSONSchemaProps{Type: "custom"},
				},
			},
			want: []any{"example", "example"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := generateExamplePrimitiveArrayForType(tt.schema)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("generateExamplePrimitiveArrayForType() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetExampleValueForPrimitiveType(t *testing.T) {
	tests := []struct {
		name   string
		schema *extv1.JSONSchemaProps
		want   string
	}{
		{"string", &extv1.JSONSchemaProps{Type: typeString}, "example"},
		{"integer", &extv1.JSONSchemaProps{Type: typeInteger}, "0"},
		{"boolean", &extv1.JSONSchemaProps{Type: typeBoolean}, "false"},
		{"number", &extv1.JSONSchemaProps{Type: typeNumber}, "0.0"},
		{"unknown", &extv1.JSONSchemaProps{Type: "custom"}, "example"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getExampleValueForPrimitiveType(tt.schema)
			if got != tt.want {
				t.Errorf("getExampleValueForPrimitiveType() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestIsMapOfCustomType(t *testing.T) {
	tests := []struct {
		name   string
		schema *extv1.JSONSchemaProps
		want   bool
	}{
		{"nil schema", nil, false},
		{
			"object with properties",
			&extv1.JSONSchemaProps{
				Type: typeObject,
				Properties: map[string]extv1.JSONSchemaProps{
					"name": {Type: typeString},
				},
			},
			true,
		},
		{"object without properties", &extv1.JSONSchemaProps{Type: typeObject}, false},
		{"non-object", &extv1.JSONSchemaProps{Type: typeString}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isMapOfCustomType(tt.schema); got != tt.want {
				t.Errorf("isMapOfCustomType() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsMapOfArray(t *testing.T) {
	tests := []struct {
		name   string
		schema *extv1.JSONSchemaProps
		want   bool
	}{
		{"nil schema", nil, false},
		{"array type", &extv1.JSONSchemaProps{Type: typeArray}, true},
		{"non-array", &extv1.JSONSchemaProps{Type: typeString}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isMapOfArray(tt.schema); got != tt.want {
				t.Errorf("isMapOfArray() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsMapOfPrimitiveArray(t *testing.T) {
	tests := []struct {
		name   string
		schema *extv1.JSONSchemaProps
		want   bool
	}{
		{"non-array", &extv1.JSONSchemaProps{Type: typeString}, false},
		{
			"primitive array items",
			&extv1.JSONSchemaProps{
				Type: typeArray,
				Items: &extv1.JSONSchemaPropsOrArray{
					Schema: &extv1.JSONSchemaProps{Type: typeString},
				},
			},
			true,
		},
		{
			"object array items",
			&extv1.JSONSchemaProps{
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
			false,
		},
		{
			"nil item schema",
			&extv1.JSONSchemaProps{
				Type:  typeArray,
				Items: &extv1.JSONSchemaPropsOrArray{Schema: nil},
			},
			true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isMapOfPrimitiveArray(tt.schema); got != tt.want {
				t.Errorf("isMapOfPrimitiveArray() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsMapOfCustomTypeArray(t *testing.T) {
	tests := []struct {
		name   string
		schema *extv1.JSONSchemaProps
		want   bool
	}{
		{"non-array", &extv1.JSONSchemaProps{Type: typeString}, false},
		{
			"object array items",
			&extv1.JSONSchemaProps{
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
			true,
		},
		{
			"nil item schema",
			&extv1.JSONSchemaProps{
				Type:  typeArray,
				Items: &extv1.JSONSchemaPropsOrArray{Schema: nil},
			},
			false,
		},
		{
			"primitive item schema",
			&extv1.JSONSchemaProps{
				Type: typeArray,
				Items: &extv1.JSONSchemaPropsOrArray{
					Schema: &extv1.JSONSchemaProps{Type: typeString},
				},
			},
			false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isMapOfCustomTypeArray(tt.schema); got != tt.want {
				t.Errorf("isMapOfCustomTypeArray() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsArrayOfMaps(t *testing.T) {
	tests := []struct {
		name   string
		schema *extv1.JSONSchemaProps
		want   bool
	}{
		{"nil schema", nil, false},
		{
			"object with additionalProperties",
			&extv1.JSONSchemaProps{
				Type: typeObject,
				AdditionalProperties: &extv1.JSONSchemaPropsOrBool{
					Schema: &extv1.JSONSchemaProps{Type: typeString},
				},
			},
			true,
		},
		{"object without additionalProperties", &extv1.JSONSchemaProps{Type: typeObject}, false},
		{"non-object", &extv1.JSONSchemaProps{Type: typeString}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isArrayOfMaps(tt.schema); got != tt.want {
				t.Errorf("isArrayOfMaps() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsArrayOfCustomType(t *testing.T) {
	tests := []struct {
		name   string
		schema *extv1.JSONSchemaProps
		want   bool
	}{
		{"nil schema", nil, false},
		{
			"object with properties",
			&extv1.JSONSchemaProps{
				Type: typeObject,
				Properties: map[string]extv1.JSONSchemaProps{
					"name": {Type: typeString},
				},
			},
			true,
		},
		{"object without properties", &extv1.JSONSchemaProps{Type: typeObject}, false},
		{"non-object", &extv1.JSONSchemaProps{Type: typeString}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isArrayOfCustomType(tt.schema); got != tt.want {
				t.Errorf("isArrayOfCustomType() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSortedPropertyNames(t *testing.T) {
	props := map[string]extv1.JSONSchemaProps{
		"zebra": {Type: typeString},
		"alpha": {Type: typeString},
		"mid":   {Type: typeString},
	}

	got := sortedPropertyNames(props)
	want := []string{"alpha", "mid", "zebra"}

	if !reflect.DeepEqual(got, want) {
		t.Errorf("sortedPropertyNames() = %v, want %v", got, want)
	}
}

func TestAllChildrenOptional(t *testing.T) {
	tests := []struct {
		name   string
		schema *extv1.JSONSchemaProps
		want   bool
	}{
		{
			name: "no required fields",
			schema: &extv1.JSONSchemaProps{
				Type: typeObject,
				Properties: map[string]extv1.JSONSchemaProps{
					"name": {Type: typeString},
				},
			},
			want: true,
		},
		{
			name: "required field with default",
			schema: &extv1.JSONSchemaProps{
				Type:     typeObject,
				Required: []string{"name"},
				Properties: map[string]extv1.JSONSchemaProps{
					"name": {Type: typeString, Default: &extv1.JSON{Raw: []byte(`"default"`)}},
				},
			},
			want: true,
		},
		{
			name: "required field without default",
			schema: &extv1.JSONSchemaProps{
				Type:     typeObject,
				Required: []string{"name"},
				Properties: map[string]extv1.JSONSchemaProps{
					"name": {Type: typeString},
				},
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := allChildrenOptional(tt.schema); got != tt.want {
				t.Errorf("allChildrenOptional() = %v, want %v", got, tt.want)
			}
		})
	}
}
