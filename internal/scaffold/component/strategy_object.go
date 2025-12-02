// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package component

import (
	"gopkg.in/yaml.v3"
	extv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
)

// ObjectFieldStrategy handles rendering of object fields with defined properties.
type ObjectFieldStrategy struct {
	renderer *FieldRenderer
}

// NewObjectFieldStrategy creates a new object field strategy.
func NewObjectFieldStrategy(renderer *FieldRenderer) *ObjectFieldStrategy {
	return &ObjectFieldStrategy{renderer: renderer}
}

// CanHandle returns true for objects with defined properties (not maps).
func (s *ObjectFieldStrategy) CanHandle(ctx *FieldContext) bool {
	return ctx.IsObjectField()
}

// Render generates YAML for an object field.
//
// Semantics (applies to both depth == 0 and depth > 0):
// - NOT required (optional): show entire structure commented out
// - Required AND all children optional: show "field: {}" + commented children
// - Required AND some children required: expand normally
func (s *ObjectFieldStrategy) Render(b *YAMLBuilder, ctx *FieldContext) {
	// Skip optional objects without defaults when IncludeAllFields is false
	if ctx.ShouldOmitOptionalField() {
		return
	}

	valueMap := ctx.GetValueAsMap()
	comment := ctx.Renderer.buildFieldComment(ctx.Schema)
	allChildrenAreOptional := allChildrenOptional(ctx.Schema)

	// Build head comment
	headComment := s.buildHeadComment(ctx, comment)

	// Case 1: Optional object - show entire structure commented out
	if !ctx.IsRequired {
		b.InCommentedMapping(ctx.Name, func(b *YAMLBuilder) {
			s.renderFieldsCommented(b, ctx.Schema, valueMap)
		}, WithHeadComment(headComment))
		return
	}

	// Case 2: Required object with all optional children - show "field: {}" pattern
	if allChildrenAreOptional {
		emptyObjComment := s.buildEmptyObjectComment(headComment)
		b.AddMapping(ctx.Name, WithHeadComment(emptyObjComment))
		b.InCommentedMapping(ctx.Name, func(b *YAMLBuilder) {
			s.renderFieldsCommented(b, ctx.Schema, valueMap)
		})
		return
	}

	// Case 3: Required object with some required children - expand normally
	b.InMapping(ctx.Name, func(b *YAMLBuilder) {
		ctx.Renderer.RenderFields(b, ctx.Schema, valueMap, ctx.Depth+1)
	}, WithHeadComment(headComment))
}

// buildHeadComment builds the head comment for the object field.
func (s *ObjectFieldStrategy) buildHeadComment(ctx *FieldContext, comment string) string {
	if ctx.AddSeparator && comment != "" {
		return separatorComment + "\n" + comment
	} else if ctx.AddSeparator {
		return separatorComment
	}
	return comment
}

// buildEmptyObjectComment builds the comment for empty object pattern (field: {}).
func (s *ObjectFieldStrategy) buildEmptyObjectComment(headComment string) string {
	emptyObjComment := headComment
	if s.renderer.includeStructuralComments {
		if emptyObjComment != "" {
			emptyObjComment += "\n"
		}
		emptyObjComment += "\nEmpty object, or customize:"
	}
	return emptyObjComment
}

// renderFieldsCommented generates all fields as commented, using default values.
func (s *ObjectFieldStrategy) renderFieldsCommented(b *YAMLBuilder, schema *extv1.JSONSchemaProps, defaultedObj map[string]any) {
	if schema == nil || schema.Properties == nil {
		return
	}

	fieldNames := s.renderer.getSortedFieldNames(schema)

	for _, name := range fieldNames {
		prop := schema.Properties[name]
		value := defaultedObj[name]

		if prop.Type == typeObject && len(prop.Properties) > 0 {
			// Nested object - show as "{}" if empty default, otherwise recurse
			if isEmptyDefault(prop.Default) {
				comment := s.renderer.buildFieldComment(&prop)
				b.AddCommentedField(name, "{}", WithLineComment(comment))
			} else {
				valueMap, _ := value.(map[string]any)
				if valueMap == nil {
					valueMap = make(map[string]any)
				}
				comment := s.renderer.buildFieldComment(&prop)
				b.InCommentedMapping(name, func(b *YAMLBuilder) {
					s.renderFieldsCommented(b, &prop, valueMap)
				}, WithHeadComment(comment))
			}
		} else if prop.Type == typeArray {
			// Array - show block format with type-aware rendering
			comment := s.renderer.buildFieldComment(&prop)
			if valueSlice, ok := value.([]any); ok && len(valueSlice) > 0 {
				// Check if array contains complex types (objects/maps)
				if s.isComplexArray(&prop, valueSlice) {
					s.renderCommentedComplexArray(b, name, valueSlice, comment)
				} else {
					// Simple primitives - use AddCommentedArray
					b.AddCommentedArray(name, valueSlice, WithLineComment(comment))
				}
			} else {
				b.AddCommentedField(name, "[]", WithLineComment(comment))
			}
		} else {
			// Primitive - show formatted value or example if no default
			comment := s.renderer.buildFieldComment(&prop)
			displayValue := value
			if value == nil {
				// No default - generate example value to guide users
				displayValue = generateExampleValueForType(&prop)
			}
			b.AddCommentedField(name, formatDefaultValue(displayValue), WithLineComment(comment))
		}
	}
}

// isComplexArray checks if an array contains complex types (objects or maps) that need special rendering.
func (s *ObjectFieldStrategy) isComplexArray(arraySchema *extv1.JSONSchemaProps, values []any) bool {
	if len(values) == 0 {
		return false
	}

	// Check if any value is a map or slice (complex type)
	for _, v := range values {
		switch v.(type) {
		case map[string]any, []any:
			return true
		}
	}

	// Also check schema - if items are objects or have additionalProperties, it's complex
	if arraySchema.Items != nil && arraySchema.Items.Schema != nil {
		itemSchema := arraySchema.Items.Schema
		if itemSchema.Type == typeObject {
			return true
		}
	}

	return false
}

// renderCommentedComplexArray renders an array with complex types (objects/maps) as properly structured commented YAML.
func (s *ObjectFieldStrategy) renderCommentedComplexArray(b *YAMLBuilder, name string, values []any, comment string) {
	// Create a commented sequence
	sequenceNode := &yaml.Node{Kind: yaml.SequenceNode}
	keyNode := &yaml.Node{Kind: yaml.ScalarNode, Value: name}

	if comment != "" {
		keyNode.LineComment = comment
	}

	b.commentedNodes[keyNode] = true
	b.commentedNodes[sequenceNode] = true

	// Render each array item
	for _, item := range values {
		if itemMap, ok := item.(map[string]any); ok {
			// Object item - render as mapping
			itemNode := &yaml.Node{Kind: yaml.MappingNode, Content: []*yaml.Node{}}
			b.commentedNodes[itemNode] = true

			itemBuilder := &YAMLBuilder{
				root:           &yaml.Node{Kind: yaml.DocumentNode},
				current:        itemNode,
				commentedNodes: b.commentedNodes,
			}

			// Render object fields
			for _, k := range sortedKeys(itemMap) {
				itemBuilder.AddField(k, formatDefaultValue(itemMap[k]))
			}

			sequenceNode.Content = append(sequenceNode.Content, itemNode)
		} else {
			// Fallback to scalar
			itemNode := &yaml.Node{Kind: yaml.ScalarNode, Value: formatDefaultValue(item)}
			b.commentedNodes[itemNode] = true
			sequenceNode.Content = append(sequenceNode.Content, itemNode)
		}
	}

	b.current.Content = append(b.current.Content, keyNode, sequenceNode)
}
