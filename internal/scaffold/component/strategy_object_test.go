// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package component

import (
	"strings"
	"testing"

	extv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
)

func TestObjectFieldStrategy_Render_OptionalOmitted(t *testing.T) {
	renderer := NewFieldRenderer(false, false, false)
	strategy := NewObjectFieldStrategy(renderer)
	b := NewYAMLBuilder()

	ctx := &FieldContext{
		Name: "config",
		Schema: &extv1.JSONSchemaProps{
			Type: typeObject,
			Properties: map[string]extv1.JSONSchemaProps{
				"name": {Type: typeString},
			},
		},
		IsRequired: false,
		HasDefault: false,
		Renderer:   renderer,
	}

	strategy.Render(b, ctx)
	got := encodeOrFatal(t, b)
	if strings.TrimSpace(got) != "" {
		t.Errorf("expected empty output for omitted optional object, got:\n%s", got)
	}
}

func TestObjectFieldStrategy_Render_OptionalCommented(t *testing.T) {
	renderer := NewFieldRenderer(false, false, false)
	strategy := NewObjectFieldStrategy(renderer)
	b := NewYAMLBuilder()

	ctx := &FieldContext{
		Name: "config",
		Schema: &extv1.JSONSchemaProps{
			Type: typeObject,
			Properties: map[string]extv1.JSONSchemaProps{
				"name": {Type: typeString},
			},
		},
		Value:      map[string]any{"name": "default_val"},
		IsRequired: false,
		HasDefault: true,
		Renderer:   renderer,
	}

	strategy.Render(b, ctx)
	want := dedent(`
		# config:
		  # name: default_val
	`)
	assertYAMLEqual(t, want, encodeOrFatal(t, b))
}

func TestObjectFieldStrategy_Render_RequiredAllOptionalChildren(t *testing.T) {
	renderer := NewFieldRenderer(false, false, false)
	strategy := NewObjectFieldStrategy(renderer)
	b := NewYAMLBuilder()

	ctx := &FieldContext{
		Name: "retryPolicy",
		Schema: &extv1.JSONSchemaProps{
			Type: typeObject,
			Properties: map[string]extv1.JSONSchemaProps{
				"attempts": {Type: typeInteger},
				"backoff":  {Type: typeInteger},
			},
		},
		Value:      map[string]any{},
		IsRequired: true,
		HasDefault: false,
		Renderer:   renderer,
	}

	strategy.Render(b, ctx)
	want := dedent(`
		retryPolicy: {}
		# retryPolicy:
		  # attempts: 0
		  # backoff: 0
	`)
	assertYAMLEqual(t, want, encodeOrFatal(t, b))
}

func TestObjectFieldStrategy_Render_RequiredAllOptionalChildren_StructuralComments(t *testing.T) {
	renderer := NewFieldRenderer(true, false, true)
	strategy := NewObjectFieldStrategy(renderer)
	b := NewYAMLBuilder()

	ctx := &FieldContext{
		Name: "retryPolicy",
		Schema: &extv1.JSONSchemaProps{
			Type:        typeObject,
			Description: "Retry configuration",
			Properties: map[string]extv1.JSONSchemaProps{
				"attempts": {Type: typeInteger},
			},
		},
		Value:        map[string]any{},
		IsRequired:   true,
		HasDefault:   false,
		AddSeparator: true,
		Renderer:     renderer,
	}

	strategy.Render(b, ctx)
	// Note the intentional leading blank line: the encoder emits a blank line
	// before the "Defaults:" separator comment. We preserve it with an extra
	// leading newline in the raw string.
	want := dedent(`

		# Defaults: Uncomment to customize
		# Retry configuration

		# Empty object, or customize:
		retryPolicy: {}
		# retryPolicy:
		  # attempts: 0
	`)
	assertYAMLEqual(t, want, encodeOrFatal(t, b))
}

func TestObjectFieldStrategy_Render_RequiredWithRequiredChildren(t *testing.T) {
	renderer := NewFieldRenderer(false, false, false)
	strategy := NewObjectFieldStrategy(renderer)
	b := NewYAMLBuilder()

	ctx := &FieldContext{
		Name: "service",
		Schema: &extv1.JSONSchemaProps{
			Type:     typeObject,
			Required: []string{"name"},
			Properties: map[string]extv1.JSONSchemaProps{
				"name":     {Type: typeString},
				"protocol": {Type: typeString},
			},
		},
		Value:      map[string]any{},
		IsRequired: true,
		HasDefault: false,
		Renderer:   renderer,
	}

	strategy.Render(b, ctx)
	// Required "name" renders active with TODO placeholder.
	// Optional "protocol" has no default and IncludeAllFields is false, so it's omitted.
	want := dedent(`
		service:
		  name: <TODO_NAME>
	`)
	assertYAMLEqual(t, want, encodeOrFatal(t, b))
}

func TestObjectFieldStrategy_BuildHeadComment(t *testing.T) {
	renderer := NewFieldRenderer(true, false, false)
	strategy := NewObjectFieldStrategy(renderer)

	tests := []struct {
		name      string
		separator bool
		comment   string
		want      string
	}{
		{
			name:      "separator and comment",
			separator: true,
			comment:   "Some description",
			want:      "\nDefaults: Uncomment to customize\nSome description",
		},
		{
			name:      "separator only",
			separator: true,
			comment:   "",
			want:      "\nDefaults: Uncomment to customize",
		},
		{
			name:      "comment only",
			separator: false,
			comment:   "Some description",
			want:      "Some description",
		},
		{
			name:      "neither",
			separator: false,
			comment:   "",
			want:      "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := &FieldContext{AddSeparator: tt.separator}
			if got := strategy.buildHeadComment(ctx, tt.comment); got != tt.want {
				t.Errorf("buildHeadComment() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestObjectFieldStrategy_BuildEmptyObjectComment(t *testing.T) {
	tests := []struct {
		name                     string
		includeStructuralComment bool
		headComment              string
		want                     string
	}{
		{
			name:                     "structural comments with head comment",
			includeStructuralComment: true,
			headComment:              "Description",
			want:                     "Description\n\nEmpty object, or customize:",
		},
		{
			name:                     "structural comments without head comment",
			includeStructuralComment: true,
			headComment:              "",
			want:                     "\nEmpty object, or customize:",
		},
		{
			name:                     "no structural comments",
			includeStructuralComment: false,
			headComment:              "Description",
			want:                     "Description",
		},
		{
			name:                     "no structural comments, no head comment",
			includeStructuralComment: false,
			headComment:              "",
			want:                     "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			renderer := NewFieldRenderer(false, false, tt.includeStructuralComment)
			strategy := NewObjectFieldStrategy(renderer)
			if got := strategy.buildEmptyObjectComment(tt.headComment); got != tt.want {
				t.Errorf("buildEmptyObjectComment() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestObjectFieldStrategy_RenderFieldsCommented_NilSchema(t *testing.T) {
	renderer := NewFieldRenderer(false, false, false)
	strategy := NewObjectFieldStrategy(renderer)
	b := NewYAMLBuilder()

	strategy.renderFieldsCommented(b, nil, nil)
	got := encodeOrFatal(t, b)
	if strings.TrimSpace(got) != "" {
		t.Errorf("expected empty output for nil schema, got:\n%s", got)
	}
}

func TestObjectFieldStrategy_RenderFieldsCommented_NilProperties(t *testing.T) {
	renderer := NewFieldRenderer(false, false, false)
	strategy := NewObjectFieldStrategy(renderer)
	b := NewYAMLBuilder()

	strategy.renderFieldsCommented(b, &extv1.JSONSchemaProps{Type: typeObject}, nil)
	got := encodeOrFatal(t, b)
	if strings.TrimSpace(got) != "" {
		t.Errorf("expected empty output for nil properties, got:\n%s", got)
	}
}

func TestObjectFieldStrategy_RenderFieldsCommented_NestedEmptyDefault(t *testing.T) {
	renderer := NewFieldRenderer(false, false, false)
	strategy := NewObjectFieldStrategy(renderer)
	b := NewYAMLBuilder()

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

	strategy.renderFieldsCommented(b, schema, map[string]any{})
	// Empty default objects collapse to "# nested: '{}'" - the "{}" is quoted
	// because quoteIfNeeded treats "{" as a special YAML character.
	want := `# nested: '{}'`
	assertYAMLEqual(t, want, encodeOrFatal(t, b))
}

func TestObjectFieldStrategy_RenderFieldsCommented_NestedNonEmptyDefault(t *testing.T) {
	renderer := NewFieldRenderer(false, false, false)
	strategy := NewObjectFieldStrategy(renderer)
	b := NewYAMLBuilder()

	schema := &extv1.JSONSchemaProps{
		Type: typeObject,
		Properties: map[string]extv1.JSONSchemaProps{
			"nested": {
				Type: typeObject,
				Properties: map[string]extv1.JSONSchemaProps{
					"inner": {Type: typeString},
				},
				Default: &extv1.JSON{Raw: []byte(`{"inner": "val"}`)},
			},
		},
	}

	strategy.renderFieldsCommented(b, schema, map[string]any{
		"nested": map[string]any{"inner": "val"},
	})
	want := dedent(`
		# nested:
		  # inner: val
	`)
	assertYAMLEqual(t, want, encodeOrFatal(t, b))
}

func TestObjectFieldStrategy_RenderFieldsCommented_NonMapValueFallback(t *testing.T) {
	renderer := NewFieldRenderer(false, false, false)
	strategy := NewObjectFieldStrategy(renderer)
	b := NewYAMLBuilder()

	schema := &extv1.JSONSchemaProps{
		Type: typeObject,
		Properties: map[string]extv1.JSONSchemaProps{
			"nested": {
				Type: typeObject,
				Properties: map[string]extv1.JSONSchemaProps{
					"inner": {Type: typeString},
				},
				Default: &extv1.JSON{Raw: []byte(`{"inner": "val"}`)},
			},
		},
	}

	// Value is not a map - should fallback to empty map, so "inner" uses example value
	strategy.renderFieldsCommented(b, schema, map[string]any{
		"nested": "not-a-map",
	})
	want := dedent(`
		# nested:
		  # inner: example
	`)
	assertYAMLEqual(t, want, encodeOrFatal(t, b))
}

func TestObjectFieldStrategy_RenderFieldsCommented_ComplexArray(t *testing.T) {
	renderer := NewFieldRenderer(false, false, false)
	strategy := NewObjectFieldStrategy(renderer)
	b := NewYAMLBuilder()

	schema := &extv1.JSONSchemaProps{
		Type: typeObject,
		Properties: map[string]extv1.JSONSchemaProps{
			"items": {
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

	strategy.renderFieldsCommented(b, schema, map[string]any{
		"items": []any{
			map[string]any{"name": "a"},
			map[string]any{"name": "b"},
		},
	})
	want := dedent(`
		# items:
		  # - name: a
		  # - name: b
	`)
	assertYAMLEqual(t, want, encodeOrFatal(t, b))
}

func TestObjectFieldStrategy_RenderFieldsCommented_SimpleArray(t *testing.T) {
	renderer := NewFieldRenderer(false, false, false)
	strategy := NewObjectFieldStrategy(renderer)
	b := NewYAMLBuilder()

	schema := &extv1.JSONSchemaProps{
		Type: typeObject,
		Properties: map[string]extv1.JSONSchemaProps{
			"tags": {
				Type: typeArray,
				Items: &extv1.JSONSchemaPropsOrArray{
					Schema: &extv1.JSONSchemaProps{Type: typeString},
				},
			},
		},
	}

	strategy.renderFieldsCommented(b, schema, map[string]any{
		"tags": []any{"web", "api"},
	})
	want := dedent(`
		# tags:
		  # - web
		  # - api
	`)
	assertYAMLEqual(t, want, encodeOrFatal(t, b))
}

func TestObjectFieldStrategy_RenderFieldsCommented_EmptyArray(t *testing.T) {
	renderer := NewFieldRenderer(false, false, false)
	strategy := NewObjectFieldStrategy(renderer)
	b := NewYAMLBuilder()

	schema := &extv1.JSONSchemaProps{
		Type: typeObject,
		Properties: map[string]extv1.JSONSchemaProps{
			"tags": {
				Type: typeArray,
				Items: &extv1.JSONSchemaPropsOrArray{
					Schema: &extv1.JSONSchemaProps{Type: typeString},
				},
			},
		},
	}

	strategy.renderFieldsCommented(b, schema, map[string]any{})
	// "[]" is quoted by quoteIfNeeded since "[" is a special YAML character.
	want := `# tags: '[]'`
	assertYAMLEqual(t, want, encodeOrFatal(t, b))
}

func TestObjectFieldStrategy_RenderFieldsCommented_PrimitivesUseExamples(t *testing.T) {
	renderer := NewFieldRenderer(false, false, false)
	strategy := NewObjectFieldStrategy(renderer)
	b := NewYAMLBuilder()

	schema := &extv1.JSONSchemaProps{
		Type: typeObject,
		Properties: map[string]extv1.JSONSchemaProps{
			"port":    {Type: typeInteger},
			"enabled": {Type: typeBoolean},
			"name":    {Type: typeString},
		},
	}

	strategy.renderFieldsCommented(b, schema, map[string]any{})
	// Fields are sorted by type then alphabetically: bool < int < string
	want := dedent(`
		# enabled: false
		# port: 0
		# name: example
	`)
	assertYAMLEqual(t, want, encodeOrFatal(t, b))
}

func TestObjectFieldStrategy_IsComplexArray(t *testing.T) {
	strategy := NewObjectFieldStrategy(NewFieldRenderer(false, false, false))

	tests := []struct {
		name   string
		schema *extv1.JSONSchemaProps
		values []any
		want   bool
	}{
		{
			name:   "empty values",
			schema: &extv1.JSONSchemaProps{Type: typeArray},
			values: []any{},
			want:   false,
		},
		{
			name:   "values contain map",
			schema: &extv1.JSONSchemaProps{Type: typeArray},
			values: []any{map[string]any{"key": "val"}},
			want:   true,
		},
		{
			name:   "values contain slice",
			schema: &extv1.JSONSchemaProps{Type: typeArray},
			values: []any{[]any{"a", "b"}},
			want:   true,
		},
		{
			name: "primitives but schema items are object",
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
			values: []any{"a", "b"},
			want:   true,
		},
		{
			name: "primitives and schema items are primitive",
			schema: &extv1.JSONSchemaProps{
				Type: typeArray,
				Items: &extv1.JSONSchemaPropsOrArray{
					Schema: &extv1.JSONSchemaProps{Type: typeString},
				},
			},
			values: []any{"a", "b"},
			want:   false,
		},
		{
			name:   "primitives with no items schema",
			schema: &extv1.JSONSchemaProps{Type: typeArray},
			values: []any{"a"},
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := strategy.isComplexArray(tt.schema, tt.values); got != tt.want {
				t.Errorf("isComplexArray() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestObjectFieldStrategy_RenderCommentedComplexArray_ObjectItems(t *testing.T) {
	renderer := NewFieldRenderer(false, false, false)
	strategy := NewObjectFieldStrategy(renderer)
	b := NewYAMLBuilder()

	values := []any{
		map[string]any{"name": "first", "port": float64(8080)},
		map[string]any{"name": "second"},
	}

	strategy.renderCommentedComplexArray(b, "servers", values, "Server list")
	// Keys within each object item are sorted alphabetically by sortedKeys().
	// The comment is passed as LineComment on the keyNode but is not rendered
	// by the custom encoder (it only renders value LineComments).
	want := dedent(`
		# servers:
		  # - name: first
		    # port: 8080
		  # - name: second
	`)
	assertYAMLEqual(t, want, encodeOrFatal(t, b))
}

func TestObjectFieldStrategy_RenderCommentedComplexArray_ScalarFallback(t *testing.T) {
	renderer := NewFieldRenderer(false, false, false)
	strategy := NewObjectFieldStrategy(renderer)
	b := NewYAMLBuilder()

	strategy.renderCommentedComplexArray(b, "items", []any{"scalar-value", 42}, "")
	want := dedent(`
		# items:
		  # - scalar-value
		  # - 42
	`)
	assertYAMLEqual(t, want, encodeOrFatal(t, b))
}
