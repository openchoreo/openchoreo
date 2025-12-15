// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package component

import (
	extv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
)

// RenderMode specifies whether a field should be rendered as active YAML or commented out.
type RenderMode int

const (
	// RenderActive renders the field as active YAML that will be parsed.
	RenderActive RenderMode = iota
	// RenderCommented renders the field as commented-out YAML for reference.
	RenderCommented
)

// FieldContext contains all information needed to render a single field.
// It encapsulates the rendering decision inputs so strategies can focus on output generation.
type FieldContext struct {
	// Field identification
	Name   string
	Schema *extv1.JSONSchemaProps

	// Field value (from ApplyDefaults)
	Value any

	// Schema metadata
	IsRequired bool
	HasDefault bool

	// Parent schema (for recursive rendering)
	ParentSchema *extv1.JSONSchemaProps

	// Rendering options
	AddSeparator bool // Add "Defaults: Uncomment to customize" separator before this field
	Depth        int  // Nesting depth (0 = top-level parameters)

	// Renderer reference for recursive calls
	Renderer *FieldRenderer
}

// GetValueAsMap returns the field value as a map[string]any, or an empty map if conversion fails.
func (ctx *FieldContext) GetValueAsMap() map[string]any {
	if valueMap, ok := ctx.Value.(map[string]any); ok {
		return valueMap
	}
	return make(map[string]any)
}

// GetValueAsSlice returns the field value as []any, or an empty slice if conversion fails.
func (ctx *FieldContext) GetValueAsSlice() []any {
	if valueSlice, ok := ctx.Value.([]any); ok {
		return valueSlice
	}
	return []any{}
}

// BuildFieldOptions creates standard field options for comments and separators.
func (ctx *FieldContext) BuildFieldOptions(comment string) []FieldOption {
	opts := []FieldOption{}
	if ctx.AddSeparator {
		opts = append(opts, WithHeadComment(separatorComment))
	}
	if comment != "" {
		opts = append(opts, WithLineComment(comment))
	}
	return opts
}

// BuildFieldComment builds the comment string for this field using the renderer's settings.
func (ctx *FieldContext) BuildFieldComment() string {
	return ctx.Renderer.buildFieldComment(ctx.Schema)
}

// ShouldOmitOptionalField determines if an optional field without a default should be omitted.
// Returns true when:
// - Field is optional (not required)
// - Field has no default value
// - IncludeAllFields option is false
func (ctx *FieldContext) ShouldOmitOptionalField() bool {
	return !ctx.IsRequired && !ctx.HasDefault && !ctx.Renderer.includeAllFields
}

// DetermineRenderMode decides whether this field should be rendered as active or commented.
// The decision is based on:
// - Required fields without defaults are active
// - Fields with defaults are commented (showing the default)
// - Optional fields without defaults are commented (when IncludeAllFields is true)
func (ctx *FieldContext) DetermineRenderMode() RenderMode {
	// Required without default = active (user must provide value)
	if ctx.IsRequired && !ctx.HasDefault {
		return RenderActive
	}
	// All other cases are commented (has default, or optional)
	return RenderCommented
}

// IsMapField returns true if this field is a map (object with additionalProperties).
func (ctx *FieldContext) IsMapField() bool {
	return ctx.Schema.Type == typeObject && ctx.Schema.AdditionalProperties != nil
}

// IsObjectField returns true if this field is an object with defined properties.
func (ctx *FieldContext) IsObjectField() bool {
	return ctx.Schema.Type == typeObject && len(ctx.Schema.Properties) > 0
}

// IsArrayField returns true if this field is an array.
func (ctx *FieldContext) IsArrayField() bool {
	return ctx.Schema.Type == typeArray
}

// IsPrimitiveField returns true if this field is a primitive type.
func (ctx *FieldContext) IsPrimitiveField() bool {
	switch ctx.Schema.Type {
	case typeString, typeInteger, typeNumber, typeBoolean:
		return true
	default:
		return false
	}
}

// GetArrayItemSchema returns the schema for array items, or nil if not an array.
func (ctx *FieldContext) GetArrayItemSchema() *extv1.JSONSchemaProps {
	if ctx.Schema.Items != nil && ctx.Schema.Items.Schema != nil {
		return ctx.Schema.Items.Schema
	}
	return nil
}

// GetMapValueSchema returns the schema for map values, or nil if not a map.
func (ctx *FieldContext) GetMapValueSchema() *extv1.JSONSchemaProps {
	if ctx.Schema.AdditionalProperties != nil && ctx.Schema.AdditionalProperties.Schema != nil {
		return ctx.Schema.AdditionalProperties.Schema
	}
	return nil
}
