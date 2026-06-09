// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package component

import (
	"strings"
	"testing"

	extv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
)

func TestNestedTypeRenderer_RenderMapOfCustomType_Commented(t *testing.T) {
	renderer := NewFieldRenderer(false, false, false)
	nr := NewNestedTypeRenderer(renderer)
	b := NewYAMLBuilder()

	valueSchema := &extv1.JSONSchemaProps{
		Type: typeObject,
		Properties: map[string]extv1.JSONSchemaProps{
			"name": {Type: typeString},
		},
	}

	nr.RenderMapOfCustomType(b, "configs", valueSchema, nil, RenderCommented, nil)
	// All-optional children and empty value produce empty object entries.
	want := dedent(`
		# configs:
		  # key1: {}
		  # key2: {}
	`)
	assertYAMLEqual(t, want, encodeOrFatal(t, b))
}

func TestNestedTypeRenderer_RenderMapOfCustomType_Active(t *testing.T) {
	renderer := NewFieldRenderer(false, false, false)
	nr := NewNestedTypeRenderer(renderer)
	b := NewYAMLBuilder()

	valueSchema := &extv1.JSONSchemaProps{
		Type:     typeObject,
		Required: []string{"port"},
		Properties: map[string]extv1.JSONSchemaProps{
			"port": {Type: typeInteger},
		},
	}

	nr.RenderMapOfCustomType(b, "services", valueSchema, nil, RenderActive, nil)
	want := dedent(`
		services:
		  key1:
		    port: <TODO_PORT>
		  key2:
		    port: <TODO_PORT>
	`)
	assertYAMLEqual(t, want, encodeOrFatal(t, b))
}

func TestNestedTypeRenderer_RenderMapOfCustomType_WithExistingValueForKey1(t *testing.T) {
	// This test documents the current behavior: RenderMapOfCustomType always
	// generates exactly two entries ("key1" and "key2"), and only the entry
	// matching a key in the provided value map picks up that default value.
	renderer := NewFieldRenderer(false, false, false)
	nr := NewNestedTypeRenderer(renderer)
	b := NewYAMLBuilder()

	valueSchema := &extv1.JSONSchemaProps{
		Type: typeObject,
		Properties: map[string]extv1.JSONSchemaProps{
			"host": {Type: typeString, Default: &extv1.JSON{Raw: []byte(`"localhost"`)}},
		},
	}
	existingValue := map[string]any{
		"key1": map[string]any{"host": "localhost"},
	}

	nr.RenderMapOfCustomType(b, "endpoints", valueSchema, existingValue, RenderActive, nil)
	// key1 renders its populated default as a commented field (optional with default).
	// key2 renders as an empty mapping because its value map is nil.
	want := dedent(`
		endpoints:
		  key1:
		    # host: localhost
		  key2: {}
	`)
	assertYAMLEqual(t, want, encodeOrFatal(t, b))
}

func TestNestedTypeRenderer_RenderMapOfPrimitiveArray_Commented(t *testing.T) {
	renderer := NewFieldRenderer(false, false, false)
	nr := NewNestedTypeRenderer(renderer)
	b := NewYAMLBuilder()

	arraySchema := &extv1.JSONSchemaProps{
		Type: typeArray,
		Items: &extv1.JSONSchemaPropsOrArray{
			Schema: &extv1.JSONSchemaProps{Type: typeString},
		},
	}

	nr.RenderMapOfPrimitiveArray(b, "tags", arraySchema, RenderCommented, nil)
	want := dedent(`
		# tags:
		  # key1: [example, example]
		  # key2: [example, example]
	`)
	assertYAMLEqual(t, want, encodeOrFatal(t, b))
}

func TestNestedTypeRenderer_RenderMapOfPrimitiveArray_Active(t *testing.T) {
	renderer := NewFieldRenderer(false, false, false)
	nr := NewNestedTypeRenderer(renderer)
	b := NewYAMLBuilder()

	arraySchema := &extv1.JSONSchemaProps{
		Type: typeArray,
		Items: &extv1.JSONSchemaPropsOrArray{
			Schema: &extv1.JSONSchemaProps{Type: typeInteger},
		},
	}

	nr.RenderMapOfPrimitiveArray(b, "ports", arraySchema, RenderActive, nil)
	want := dedent(`
		ports:
		  key1: [0, 0]
		  key2: [0, 0]
	`)
	assertYAMLEqual(t, want, encodeOrFatal(t, b))
}

func TestNestedTypeRenderer_RenderMapOfCustomTypeArray_Commented(t *testing.T) {
	renderer := NewFieldRenderer(false, false, false)
	nr := NewNestedTypeRenderer(renderer)
	b := NewYAMLBuilder()

	arraySchema := &extv1.JSONSchemaProps{
		Type: typeArray,
		Items: &extv1.JSONSchemaPropsOrArray{
			Schema: &extv1.JSONSchemaProps{
				Type:     typeObject,
				Required: []string{"name"},
				Properties: map[string]extv1.JSONSchemaProps{
					"name": {Type: typeString},
				},
			},
		},
	}

	nr.RenderMapOfCustomTypeArray(b, "groups", arraySchema, RenderCommented, nil)
	want := dedent(`
		# groups:
		  # key1:
		    # - name: <TODO_NAME>
		    # - name: <TODO_NAME>
		  # key2:
		    # - name: <TODO_NAME>
		    # - name: <TODO_NAME>
	`)
	assertYAMLEqual(t, want, encodeOrFatal(t, b))
}

func TestNestedTypeRenderer_RenderMapOfCustomTypeArray_Active(t *testing.T) {
	renderer := NewFieldRenderer(false, false, false)
	nr := NewNestedTypeRenderer(renderer)
	b := NewYAMLBuilder()

	arraySchema := &extv1.JSONSchemaProps{
		Type: typeArray,
		Items: &extv1.JSONSchemaPropsOrArray{
			Schema: &extv1.JSONSchemaProps{
				Type:     typeObject,
				Required: []string{"value"},
				Properties: map[string]extv1.JSONSchemaProps{
					"value": {Type: typeString},
				},
			},
		},
	}

	nr.RenderMapOfCustomTypeArray(b, "entries", arraySchema, RenderActive, nil)
	want := dedent(`
		entries:
		  key1:
		    - value: <TODO_VALUE>
		    - value: <TODO_VALUE>
		  key2:
		    - value: <TODO_VALUE>
		    - value: <TODO_VALUE>
	`)
	assertYAMLEqual(t, want, encodeOrFatal(t, b))
}

func TestNestedTypeRenderer_RenderMapOfCustomTypeArray_NilItemSchema(t *testing.T) {
	renderer := NewFieldRenderer(false, false, false)
	nr := NewNestedTypeRenderer(renderer)
	b := NewYAMLBuilder()

	arraySchema := &extv1.JSONSchemaProps{
		Type:  typeArray,
		Items: &extv1.JSONSchemaPropsOrArray{Schema: nil},
	}

	nr.RenderMapOfCustomTypeArray(b, "data", arraySchema, RenderActive, nil)
	got := encodeOrFatal(t, b)
	if strings.TrimSpace(got) != "" {
		t.Errorf("expected empty output for nil item schema, got:\n%s", got)
	}
}

func TestNestedTypeRenderer_RenderMapOfPrimitives_Commented(t *testing.T) {
	renderer := NewFieldRenderer(false, false, false)
	nr := NewNestedTypeRenderer(renderer)
	b := NewYAMLBuilder()

	nr.RenderMapOfPrimitives(b, "labels", &extv1.JSONSchemaProps{Type: typeString}, RenderCommented, nil)
	want := dedent(`
		# labels:
		  # key1: example
		  # key2: example
	`)
	assertYAMLEqual(t, want, encodeOrFatal(t, b))
}

func TestNestedTypeRenderer_RenderMapOfPrimitives_Active(t *testing.T) {
	renderer := NewFieldRenderer(false, false, false)
	nr := NewNestedTypeRenderer(renderer)
	b := NewYAMLBuilder()

	nr.RenderMapOfPrimitives(b, "counts", &extv1.JSONSchemaProps{Type: typeInteger}, RenderActive, nil)
	want := dedent(`
		counts:
		  key1: 0
		  key2: 0
	`)
	assertYAMLEqual(t, want, encodeOrFatal(t, b))
}

func TestNestedTypeRenderer_RenderArrayOfMaps_EmptyValues(t *testing.T) {
	renderer := NewFieldRenderer(false, false, false)
	nr := NewNestedTypeRenderer(renderer)
	b := NewYAMLBuilder()

	itemSchema := &extv1.JSONSchemaProps{
		Type: typeObject,
		AdditionalProperties: &extv1.JSONSchemaPropsOrBool{
			Schema: &extv1.JSONSchemaProps{Type: typeString},
		},
	}

	nr.RenderArrayOfMaps(b, "data", itemSchema, nil, RenderActive, nil)
	want := dedent(`
		data:
		  - key1: example
		    key2: example
		  - key1: example
		    key2: example
	`)
	assertYAMLEqual(t, want, encodeOrFatal(t, b))
}

func TestNestedTypeRenderer_RenderArrayOfMaps_Truncation(t *testing.T) {
	renderer := NewFieldRenderer(false, false, false)
	nr := NewNestedTypeRenderer(renderer)
	b := NewYAMLBuilder()

	itemSchema := &extv1.JSONSchemaProps{
		Type: typeObject,
		AdditionalProperties: &extv1.JSONSchemaPropsOrBool{
			Schema: &extv1.JSONSchemaProps{Type: typeString},
		},
	}

	values := []any{
		map[string]any{"a": "1"},
		map[string]any{"b": "2"},
		map[string]any{"c": "3"},
	}

	nr.RenderArrayOfMaps(b, "envVars", itemSchema, values, RenderActive, nil)
	// Exactly 2 items rendered; the third is dropped.
	want := dedent(`
		envVars:
		  - a: 1
		  - b: 2
	`)
	assertYAMLEqual(t, want, encodeOrFatal(t, b))
}

func TestNestedTypeRenderer_RenderArrayOfMaps_CommentedWithValues(t *testing.T) {
	renderer := NewFieldRenderer(false, false, false)
	nr := NewNestedTypeRenderer(renderer)
	b := NewYAMLBuilder()

	itemSchema := &extv1.JSONSchemaProps{
		Type: typeObject,
		AdditionalProperties: &extv1.JSONSchemaPropsOrBool{
			Schema: &extv1.JSONSchemaProps{Type: typeString},
		},
	}
	values := []any{map[string]any{"key": "val"}}

	nr.RenderArrayOfMaps(b, "configs", itemSchema, values, RenderCommented, nil)
	want := dedent(`
		# configs:
		  # - key: val
	`)
	assertYAMLEqual(t, want, encodeOrFatal(t, b))
}

func TestNestedTypeRenderer_RenderArrayOfMaps_ActualValues(t *testing.T) {
	renderer := NewFieldRenderer(false, false, false)
	nr := NewNestedTypeRenderer(renderer)
	b := NewYAMLBuilder()

	itemSchema := &extv1.JSONSchemaProps{
		Type: typeObject,
		AdditionalProperties: &extv1.JSONSchemaPropsOrBool{
			Schema: &extv1.JSONSchemaProps{Type: typeString},
		},
	}
	values := []any{
		map[string]any{"host": "localhost", "port": "8080"},
	}

	nr.RenderArrayOfMaps(b, "servers", itemSchema, values, RenderActive, nil)
	// Map keys within the item are sorted alphabetically.
	want := dedent(`
		servers:
		  - host: localhost
		    port: 8080
	`)
	assertYAMLEqual(t, want, encodeOrFatal(t, b))
}

func TestNestedTypeRenderer_RenderArrayOfCustomType_EmptyValues(t *testing.T) {
	renderer := NewFieldRenderer(false, false, false)
	nr := NewNestedTypeRenderer(renderer)
	b := NewYAMLBuilder()

	itemSchema := &extv1.JSONSchemaProps{
		Type: typeObject,
		Properties: map[string]extv1.JSONSchemaProps{
			"name": {Type: typeString},
			"port": {Type: typeInteger},
		},
	}

	nr.RenderArrayOfCustomType(b, "items", itemSchema, nil, RenderActive, nil)
	// All-optional children + empty value maps produce empty "{}" items.
	want := dedent(`
		items:
		  - {}
		  - {}
	`)
	assertYAMLEqual(t, want, encodeOrFatal(t, b))
}

func TestNestedTypeRenderer_RenderArrayOfCustomType_Truncation(t *testing.T) {
	renderer := NewFieldRenderer(false, false, false)
	nr := NewNestedTypeRenderer(renderer)
	b := NewYAMLBuilder()

	itemSchema := &extv1.JSONSchemaProps{
		Type:     typeObject,
		Required: []string{"name"},
		Properties: map[string]extv1.JSONSchemaProps{
			"name": {Type: typeString},
			"port": {Type: typeInteger},
		},
	}

	values := []any{
		map[string]any{"name": "a"},
		map[string]any{"name": "b"},
		map[string]any{"name": "c"},
	}

	nr.RenderArrayOfCustomType(b, "servers", itemSchema, values, RenderActive, nil)
	// Only 2 items; "c" is dropped. In active mode, required "name" uses the TODO
	// placeholder because ApplyDefaults hasn't been called on these values here.
	want := dedent(`
		servers:
		  - name: <TODO_NAME>
		  - name: <TODO_NAME>
	`)
	assertYAMLEqual(t, want, encodeOrFatal(t, b))
}

func TestNestedTypeRenderer_RenderArrayOfCustomType_CommentedNoValues(t *testing.T) {
	renderer := NewFieldRenderer(false, false, false)
	nr := NewNestedTypeRenderer(renderer)
	b := NewYAMLBuilder()

	itemSchema := &extv1.JSONSchemaProps{
		Type: typeObject,
		Properties: map[string]extv1.JSONSchemaProps{
			"name": {Type: typeString},
			"port": {Type: typeInteger},
		},
	}

	nr.RenderArrayOfCustomType(b, "servers", itemSchema, nil, RenderCommented, nil)
	// Commented mode without values uses example values for every field.
	// Properties are sorted alphabetically by sortedPropertyNames().
	want := dedent(`
		# servers:
		  # - name: example
		    # port: 0
		  # - name: example
		    # port: 0
	`)
	assertYAMLEqual(t, want, encodeOrFatal(t, b))
}

func TestNestedTypeRenderer_RenderArrayOfCustomType_CommentedNonMapValue(t *testing.T) {
	renderer := NewFieldRenderer(false, false, false)
	nr := NewNestedTypeRenderer(renderer)
	b := NewYAMLBuilder()

	itemSchema := &extv1.JSONSchemaProps{
		Type: typeObject,
		Properties: map[string]extv1.JSONSchemaProps{
			"name": {Type: typeString},
			"port": {Type: typeInteger},
		},
	}

	// Non-map item value falls back to empty map, then example values are used.
	nr.RenderArrayOfCustomType(b, "items", itemSchema, []any{"not-a-map"}, RenderCommented, nil)
	want := dedent(`
		# items:
		  # - name: example
		    # port: 0
	`)
	assertYAMLEqual(t, want, encodeOrFatal(t, b))
}

func TestNestedTypeRenderer_RenderArrayOfCustomType_CommentedPartialValues(t *testing.T) {
	renderer := NewFieldRenderer(false, false, false)
	nr := NewNestedTypeRenderer(renderer)
	b := NewYAMLBuilder()

	itemSchema := &extv1.JSONSchemaProps{
		Type: typeObject,
		Properties: map[string]extv1.JSONSchemaProps{
			"name": {Type: typeString},
			"port": {Type: typeInteger},
		},
	}

	// Only "name" provided; "port" is missing so it uses an example value.
	values := []any{map[string]any{"name": "myserver"}}

	nr.RenderArrayOfCustomType(b, "servers", itemSchema, values, RenderCommented, nil)
	want := dedent(`
		# servers:
		  # - name: myserver
		    # port: 0
	`)
	assertYAMLEqual(t, want, encodeOrFatal(t, b))
}

func TestNestedTypeRenderer_RenderMapEntries_NilSchema(t *testing.T) {
	renderer := NewFieldRenderer(false, false, false)
	nr := NewNestedTypeRenderer(renderer)
	b := NewYAMLBuilder()

	nr.renderMapEntries(b, nil)
	want := dedent(`
		key1: example
		key2: example
	`)
	assertYAMLEqual(t, want, encodeOrFatal(t, b))
}

func TestNestedTypeRenderer_RenderMapEntries_CustomType(t *testing.T) {
	renderer := NewFieldRenderer(false, false, false)
	nr := NewNestedTypeRenderer(renderer)
	b := NewYAMLBuilder()

	valueSchema := &extv1.JSONSchemaProps{
		Type: typeObject,
		Properties: map[string]extv1.JSONSchemaProps{
			"host": {Type: typeString},
		},
	}

	nr.renderMapEntries(b, valueSchema)
	want := dedent(`
		key1:
		  host: example
		key2:
		  host: example
	`)
	assertYAMLEqual(t, want, encodeOrFatal(t, b))
}

func TestNestedTypeRenderer_RenderMapEntries_Primitive(t *testing.T) {
	renderer := NewFieldRenderer(false, false, false)
	nr := NewNestedTypeRenderer(renderer)
	b := NewYAMLBuilder()

	nr.renderMapEntries(b, &extv1.JSONSchemaProps{Type: typeInteger})
	want := dedent(`
		key1: 0
		key2: 0
	`)
	assertYAMLEqual(t, want, encodeOrFatal(t, b))
}

func TestNestedTypeRenderer_RenderExampleFields_NilSchema(t *testing.T) {
	renderer := NewFieldRenderer(false, false, false)
	nr := NewNestedTypeRenderer(renderer)
	b := NewYAMLBuilder()

	nr.renderExampleFields(b, nil)
	got := encodeOrFatal(t, b)
	if strings.TrimSpace(got) != "" {
		t.Errorf("expected empty output for nil schema, got:\n%s", got)
	}
}

func TestNestedTypeRenderer_RenderExampleFields_EmptyProperties(t *testing.T) {
	renderer := NewFieldRenderer(false, false, false)
	nr := NewNestedTypeRenderer(renderer)
	b := NewYAMLBuilder()

	nr.renderExampleFields(b, &extv1.JSONSchemaProps{
		Type:       typeObject,
		Properties: map[string]extv1.JSONSchemaProps{},
	})
	got := encodeOrFatal(t, b)
	if strings.TrimSpace(got) != "" {
		t.Errorf("expected empty output for empty properties, got:\n%s", got)
	}
}

func TestNestedTypeRenderer_RenderExampleFields_AllTypes(t *testing.T) {
	renderer := NewFieldRenderer(false, false, false)
	nr := NewNestedTypeRenderer(renderer)
	b := NewYAMLBuilder()

	nr.renderExampleFields(b, &extv1.JSONSchemaProps{
		Type: typeObject,
		Properties: map[string]extv1.JSONSchemaProps{
			"name":    {Type: typeString},
			"count":   {Type: typeInteger},
			"enabled": {Type: typeBoolean},
		},
	})
	// Fields are sorted alphabetically by sortedPropertyNames().
	want := dedent(`
		count: 0
		enabled: false
		name: example
	`)
	assertYAMLEqual(t, want, encodeOrFatal(t, b))
}
