// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package component

import (
	extv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
)

// ArrayFieldStrategy handles rendering of array fields.
type ArrayFieldStrategy struct {
	renderer       *FieldRenderer
	nestedRenderer *NestedTypeRenderer
}

// NewArrayFieldStrategy creates a new array field strategy.
func NewArrayFieldStrategy(renderer *FieldRenderer, nestedRenderer *NestedTypeRenderer) *ArrayFieldStrategy {
	return &ArrayFieldStrategy{
		renderer:       renderer,
		nestedRenderer: nestedRenderer,
	}
}

// CanHandle returns true for array fields.
func (s *ArrayFieldStrategy) CanHandle(ctx *FieldContext) bool {
	return ctx.IsArrayField()
}

// Render generates YAML for an array field.
//
// Rendering rules:
// - []CustomType: Expanded with 2 example items (nested objects)
// - []map<T>: Array of maps with primitive values
// - []map<CustomType>: Array of maps with custom type values
// - []primitive: Inline format [val1, val2]
// - With default: Commented
// - Optional without default: Commented (if IncludeAllFields) or omitted
func (s *ArrayFieldStrategy) Render(b *YAMLBuilder, ctx *FieldContext) {
	itemSchema := ctx.GetArrayItemSchema()

	// Check for array of maps ([]map<T>)
	if itemSchema != nil && isArrayOfMaps(itemSchema) {
		s.handleArrayOfMaps(b, ctx, itemSchema)
		return
	}

	// Check for array of objects with properties ([]CustomType)
	if itemSchema != nil && isArrayOfCustomType(itemSchema) {
		s.handleArrayOfCustomType(b, ctx, itemSchema)
		return
	}

	// Array of primitives
	s.handleArrayOfPrimitives(b, ctx)
}

// handleArrayOfMaps handles []map<T> field generation.
func (s *ArrayFieldStrategy) handleArrayOfMaps(b *YAMLBuilder, ctx *FieldContext, itemSchema *extv1.JSONSchemaProps) {
	valueSlice := ctx.GetValueAsSlice()
	comment := ctx.Renderer.buildFieldComment(ctx.Schema)
	opts := ctx.BuildFieldOptions(comment)
	mode := ctx.DetermineRenderMode()

	// Array of maps with default - show as commented
	if ctx.HasDefault && ctx.Value != nil {
		s.nestedRenderer.RenderArrayOfMaps(b, ctx.Name, itemSchema, valueSlice, RenderCommented, opts)
		return
	}

	// Optional array of maps without default - only show if IncludeAllFields is true
	if ctx.ShouldOmitOptionalField() {
		return
	}

	if !ctx.IsRequired && !ctx.HasDefault {
		s.nestedRenderer.RenderArrayOfMaps(b, ctx.Name, itemSchema, valueSlice, RenderCommented, opts)
		return
	}

	// Required array of maps without default - expand with 2 example items
	s.nestedRenderer.RenderArrayOfMaps(b, ctx.Name, itemSchema, valueSlice, mode, opts)
}

// handleArrayOfCustomType handles []CustomType field generation.
func (s *ArrayFieldStrategy) handleArrayOfCustomType(b *YAMLBuilder, ctx *FieldContext, itemSchema *extv1.JSONSchemaProps) {
	valueSlice := ctx.GetValueAsSlice()
	comment := ctx.Renderer.buildFieldComment(ctx.Schema)
	opts := ctx.BuildFieldOptions(comment)
	mode := ctx.DetermineRenderMode()

	// Array of custom type with default - show as commented
	if ctx.HasDefault && ctx.Value != nil {
		s.nestedRenderer.RenderArrayOfCustomType(b, ctx.Name, itemSchema, valueSlice, RenderCommented, opts)
		return
	}

	// Optional array without default - only show if IncludeAllFields is true
	if ctx.ShouldOmitOptionalField() {
		return
	}

	if !ctx.IsRequired && !ctx.HasDefault {
		s.nestedRenderer.RenderArrayOfCustomType(b, ctx.Name, itemSchema, valueSlice, RenderCommented, opts)
		return
	}

	// Required array without default - expand with 2 example items
	s.nestedRenderer.RenderArrayOfCustomType(b, ctx.Name, itemSchema, valueSlice, mode, opts)
}

// handleArrayOfPrimitives handles array of primitive types.
func (s *ArrayFieldStrategy) handleArrayOfPrimitives(b *YAMLBuilder, ctx *FieldContext) {
	comment := ctx.Renderer.buildFieldComment(ctx.Schema)
	opts := ctx.BuildFieldOptions(comment)

	// Required array without default - generate inline example
	if ctx.IsRequired && !ctx.HasDefault {
		exampleArray := generateExamplePrimitiveArrayForType(ctx.Schema)
		b.AddInlineArray(ctx.Name, exampleArray, opts...)
		return
	}

	// Has value from ApplyDefaults - show as commented array
	if ctx.Value != nil {
		valueSlice := ctx.GetValueAsSlice()
		if len(valueSlice) == 0 {
			// Empty array - use inline format: # field: []
			b.AddCommentedInlineArray(ctx.Name, valueSlice, opts...)
		} else {
			// Non-empty array - use block format with items
			b.AddCommentedArray(ctx.Name, valueSlice, opts...)
		}
	}
}
