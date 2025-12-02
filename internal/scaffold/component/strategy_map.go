// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package component

import (
	extv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
)

// MapFieldStrategy handles rendering of map fields (objects with additionalProperties).
type MapFieldStrategy struct {
	renderer       *FieldRenderer
	nestedRenderer *NestedTypeRenderer
}

// NewMapFieldStrategy creates a new map field strategy.
func NewMapFieldStrategy(renderer *FieldRenderer, nestedRenderer *NestedTypeRenderer) *MapFieldStrategy {
	return &MapFieldStrategy{
		renderer:       renderer,
		nestedRenderer: nestedRenderer,
	}
}

// CanHandle returns true for map fields (objects with additionalProperties).
func (s *MapFieldStrategy) CanHandle(ctx *FieldContext) bool {
	return ctx.IsMapField()
}

// Render generates YAML for a map field.
//
// Rendering rules:
// - map<CustomType>: 2 example keys with nested object values
// - map<[]T>: 2 example keys with inline array values
// - map<[]CustomType>: 2 example keys with arrays of custom objects
// - map<T>: 2 example key-value pairs with primitive values
// - With default: Commented with actual values
// - Optional without default: Commented (if IncludeAllFields) or omitted
func (s *MapFieldStrategy) Render(b *YAMLBuilder, ctx *FieldContext) {
	valueSchema := ctx.GetMapValueSchema()
	comment := ctx.Renderer.buildFieldComment(ctx.Schema)
	opts := ctx.BuildFieldOptions(comment)
	mode := ctx.DetermineRenderMode()

	// Map with default - show as commented YAML map
	if ctx.HasDefault && ctx.Value != nil {
		s.renderMapWithDefault(b, ctx, opts)
		return
	}

	// Optional map without default - only show if IncludeAllFields is true
	if ctx.ShouldOmitOptionalField() {
		return
	}

	if !ctx.IsRequired && !ctx.HasDefault {
		s.renderExampleMap(b, ctx, valueSchema, RenderCommented, opts)
		return
	}

	// Required map without default - generate active example map with 2 key-value pairs
	s.renderExampleMap(b, ctx, valueSchema, mode, opts)
}

// renderMapWithDefault renders a map that has default values.
func (s *MapFieldStrategy) renderMapWithDefault(b *YAMLBuilder, ctx *FieldContext, opts []FieldOption) {
	valueMap := ctx.GetValueAsMap()

	b.InCommentedMapping(ctx.Name, func(b *YAMLBuilder) {
		for _, k := range sortedKeys(valueMap) {
			v := valueMap[k]
			// Handle array values with inline array format
			if arr, ok := v.([]any); ok {
				b.AddInlineArray(k, arr)
			} else {
				b.AddField(k, formatDefaultValue(v))
			}
		}
	}, opts...)
}

// renderExampleMap renders an example map based on value type.
func (s *MapFieldStrategy) renderExampleMap(b *YAMLBuilder, ctx *FieldContext, valueSchema *extv1.JSONSchemaProps, mode RenderMode, opts []FieldOption) {
	// No schema for values - use string default
	if valueSchema == nil {
		s.nestedRenderer.RenderMapOfPrimitives(b, ctx.Name, &extv1.JSONSchemaProps{Type: typeString}, mode, opts)
		return
	}

	// Delegate to nested renderer based on value type
	switch {
	case isMapOfCustomType(valueSchema):
		s.nestedRenderer.RenderMapOfCustomType(b, ctx.Name, valueSchema, ctx.Value, mode, opts)
	case isMapOfCustomTypeArray(valueSchema):
		s.nestedRenderer.RenderMapOfCustomTypeArray(b, ctx.Name, valueSchema, mode, opts)
	case isMapOfPrimitiveArray(valueSchema):
		s.nestedRenderer.RenderMapOfPrimitiveArray(b, ctx.Name, valueSchema, mode, opts)
	default:
		s.nestedRenderer.RenderMapOfPrimitives(b, ctx.Name, valueSchema, mode, opts)
	}
}
