// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package component

import (
	"encoding/json"
	"fmt"
	"strings"
)

// PrimitiveFieldStrategy handles rendering of primitive fields (string, integer, number, boolean).
type PrimitiveFieldStrategy struct {
	renderer *FieldRenderer
}

// NewPrimitiveFieldStrategy creates a new primitive field strategy.
func NewPrimitiveFieldStrategy(renderer *FieldRenderer) *PrimitiveFieldStrategy {
	return &PrimitiveFieldStrategy{renderer: renderer}
}

// CanHandle returns true for primitive types: string, integer, number, boolean.
func (s *PrimitiveFieldStrategy) CanHandle(ctx *FieldContext) bool {
	return ctx.IsPrimitiveField()
}

// Render generates YAML for a primitive field.
//
// Rendering rules:
// - Required without default → Placeholder: field: <TODO_FIELD>
// - Required with default → Commented: # field: value
// - Optional with default → Commented: # field: value
// - Optional without default → Commented (if IncludeAllFields) or omitted
func (s *PrimitiveFieldStrategy) Render(b *YAMLBuilder, ctx *FieldContext) {
	// Skip optional fields without defaults when IncludeAllFields is false
	if ctx.ShouldOmitOptionalField() {
		return
	}

	comment := ctx.Renderer.buildFieldComment(ctx.Schema)
	opts := ctx.BuildFieldOptions(comment)

	// Required field without default - generate placeholder
	if ctx.IsRequired && !ctx.HasDefault {
		value := s.getRequiredFieldValue(ctx)
		b.AddField(ctx.Name, value, opts...)
		return
	}

	// All other cases (has default, or optional) - render as commented
	if ctx.Value != nil {
		b.AddCommentedField(ctx.Name, formatDefaultValue(ctx.Value), opts...)
	}
}

// getRequiredFieldValue generates a placeholder value for required fields.
// If the field has enum values, uses the first enum value.
// Otherwise, generates a <TODO_FIELDNAME> placeholder.
func (s *PrimitiveFieldStrategy) getRequiredFieldValue(ctx *FieldContext) string {
	// Check for enum values
	if len(ctx.Schema.Enum) > 0 {
		var enumValue any
		if err := json.Unmarshal(ctx.Schema.Enum[0].Raw, &enumValue); err == nil {
			return formatDefaultValue(enumValue)
		}
	}

	// No enum - generate TODO placeholder
	return fmt.Sprintf("<TODO_%s>", strings.ToUpper(ctx.Name))
}
