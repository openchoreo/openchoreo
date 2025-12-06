// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package component

import (
	"fmt"

	"gopkg.in/yaml.v3"
	extv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
)

// NestedTypeRenderer handles rendering of complex nested collection types.
// This includes:
// - map<CustomType>: maps with object values
// - map<[]T>: maps with primitive array values
// - map<[]CustomType>: maps with custom type array values
// - []map<T>: arrays of maps with primitive values
// - []map<CustomType>: arrays of maps with custom type values
// - []CustomType: arrays of custom type objects
type NestedTypeRenderer struct {
	renderer *FieldRenderer
}

// NewNestedTypeRenderer creates a new nested type renderer.
func NewNestedTypeRenderer(renderer *FieldRenderer) *NestedTypeRenderer {
	return &NestedTypeRenderer{renderer: renderer}
}

// RenderMapOfCustomType renders map<CustomType> where values are objects with properties.
// Generates 2 example map entries with nested object structures.
func (r *NestedTypeRenderer) RenderMapOfCustomType(b *YAMLBuilder, name string, valueSchema *extv1.JSONSchemaProps, value any, mode RenderMode, opts []FieldOption) {
	mappingFunc := func(b *YAMLBuilder) {
		for i := 1; i <= 2; i++ {
			keyName := fmt.Sprintf("key%d", i)

			// Get or create default value map for this entry
			var valueMap map[string]any
			if valueMapTyped, ok := value.(map[string]any); ok {
				if entryVal, ok := valueMapTyped[keyName]; ok {
					valueMap, _ = entryVal.(map[string]any)
				}
			}
			if valueMap == nil {
				valueMap = make(map[string]any)
			}

			// Generate nested object structure
			b.InMapping(keyName, func(b *YAMLBuilder) {
				r.renderer.RenderFields(b, valueSchema, valueMap, 1)
			})
		}
	}

	if mode == RenderCommented {
		b.InCommentedMappingWithFunc(name, mappingFunc, opts...)
	} else {
		b.InMapping(name, mappingFunc, opts...)
	}
}

// RenderMapOfPrimitiveArray renders map<[]primitive> where values are arrays of primitives.
// Generates 2 example map entries with inline array values.
func (r *NestedTypeRenderer) RenderMapOfPrimitiveArray(b *YAMLBuilder, name string, arraySchema *extv1.JSONSchemaProps, mode RenderMode, opts []FieldOption) {
	mappingFunc := func(b *YAMLBuilder) {
		exampleArray := generateExamplePrimitiveArrayForType(arraySchema)
		b.AddInlineArray("key1", exampleArray)
		b.AddInlineArray("key2", exampleArray)
	}

	if mode == RenderCommented {
		b.InCommentedMappingWithFunc(name, mappingFunc, opts...)
	} else {
		b.InMapping(name, mappingFunc, opts...)
	}
}

// RenderMapOfCustomTypeArray renders map<[]CustomType> where values are arrays of custom objects.
// Generates 2 example map entries, each with 2 example custom type objects.
func (r *NestedTypeRenderer) RenderMapOfCustomTypeArray(b *YAMLBuilder, name string, arraySchema *extv1.JSONSchemaProps, mode RenderMode, opts []FieldOption) {
	itemSchema := arraySchema.Items.Schema
	if itemSchema == nil {
		return
	}

	mappingFunc := func(b *YAMLBuilder) {
		for _, key := range []string{"key1", "key2"} {
			sequenceNode := b.AddSequence(key)
			// Generate 2 example array items
			for i := 0; i < 2; i++ {
				b.AddSequenceMapping(sequenceNode, func(itemBuilder *YAMLBuilder) {
					r.renderer.RenderFields(itemBuilder, itemSchema, make(map[string]any), 1)
				})
			}
		}
	}

	if mode == RenderCommented {
		b.InCommentedMappingWithFunc(name, mappingFunc, opts...)
	} else {
		b.InMapping(name, mappingFunc, opts...)
	}
}

// RenderMapOfPrimitives renders map<primitive> where values are primitive types.
// Generates 2 example key-value pairs.
func (r *NestedTypeRenderer) RenderMapOfPrimitives(b *YAMLBuilder, name string, valueSchema *extv1.JSONSchemaProps, mode RenderMode, opts []FieldOption) {
	exampleVal := getExampleValueForPrimitiveType(valueSchema)

	mappingFunc := func(b *YAMLBuilder) {
		b.AddField("key1", exampleVal)
		b.AddField("key2", exampleVal)
	}

	if mode == RenderCommented {
		b.InCommentedMappingWithFunc(name, mappingFunc, opts...)
	} else {
		b.InMapping(name, mappingFunc, opts...)
	}
}

// RenderArrayOfMaps renders []map<T> where items are maps.
// Generates 2 example array items, each containing map entries.
func (r *NestedTypeRenderer) RenderArrayOfMaps(b *YAMLBuilder, name string, itemSchema *extv1.JSONSchemaProps, values []any, mode RenderMode, opts []FieldOption) {
	sequenceNode := &yaml.Node{Kind: yaml.SequenceNode}
	keyNode := &yaml.Node{Kind: yaml.ScalarNode, Value: name}

	// Apply options
	for _, opt := range opts {
		opt(keyNode, sequenceNode)
	}

	// Mark as commented if needed
	if mode == RenderCommented {
		b.commentedNodes[keyNode] = true
		b.commentedNodes[sequenceNode] = true
	}

	// Use provided values or generate examples
	if len(values) == 0 {
		values = []any{make(map[string]any), make(map[string]any)}
	}

	for i, item := range values {
		if i >= 2 {
			break
		}

		itemMappingNode := &yaml.Node{Kind: yaml.MappingNode, Content: []*yaml.Node{}}
		if mode == RenderCommented {
			b.commentedNodes[itemMappingNode] = true
		}

		itemBuilder := &YAMLBuilder{
			root:           &yaml.Node{Kind: yaml.DocumentNode},
			current:        itemMappingNode,
			commentedNodes: b.commentedNodes,
		}

		// Render actual values if provided, otherwise generate examples
		if itemMap, ok := item.(map[string]any); ok && len(itemMap) > 0 {
			for _, k := range sortedKeys(itemMap) {
				itemBuilder.AddField(k, formatDefaultValue(itemMap[k]))
			}
		} else {
			r.renderMapEntries(itemBuilder, itemSchema.AdditionalProperties.Schema)
		}

		sequenceNode.Content = append(sequenceNode.Content, itemMappingNode)
	}

	b.current.Content = append(b.current.Content, keyNode, sequenceNode)
}

// RenderArrayOfCustomType renders []CustomType where items are objects with properties.
// Generates 2 example array items with nested object structures.
func (r *NestedTypeRenderer) RenderArrayOfCustomType(b *YAMLBuilder, name string, itemSchema *extv1.JSONSchemaProps, values []any, mode RenderMode, opts []FieldOption) {
	sequenceNode := &yaml.Node{Kind: yaml.SequenceNode}
	keyNode := &yaml.Node{Kind: yaml.ScalarNode, Value: name}

	// Apply options
	for _, opt := range opts {
		opt(keyNode, sequenceNode)
	}

	// Mark as commented if needed
	if mode == RenderCommented {
		b.commentedNodes[keyNode] = true
		b.commentedNodes[sequenceNode] = true
	}

	// Ensure we have values
	if len(values) == 0 {
		values = []any{make(map[string]any), make(map[string]any)}
	}

	// Generate each array item
	for i, item := range values {
		if i >= 2 {
			break
		}

		itemMap, _ := item.(map[string]any)
		if itemMap == nil {
			itemMap = make(map[string]any)
		}

		itemMappingNode := &yaml.Node{Kind: yaml.MappingNode, Content: []*yaml.Node{}}
		if mode == RenderCommented {
			b.commentedNodes[itemMappingNode] = true
		}

		itemBuilder := &YAMLBuilder{
			root:           &yaml.Node{Kind: yaml.DocumentNode},
			current:        itemMappingNode,
			commentedNodes: b.commentedNodes,
		}

		// Render fields based on mode and available values
		if mode == RenderCommented {
			// Commented mode: prefer actual values, fall back to examples
			if len(itemMap) > 0 {
				// Render actual default values
				for _, k := range sortedPropertyNames(itemSchema.Properties) {
					if v, ok := itemMap[k]; ok {
						itemBuilder.AddField(k, formatDefaultValue(v))
					} else {
						// Field not in default - generate example
						prop := itemSchema.Properties[k]
						itemBuilder.AddField(k, formatDefaultValue(generateExampleValueForType(&prop)))
					}
				}
			} else {
				// No default values - use example values for all fields
				r.renderExampleFields(itemBuilder, itemSchema)
			}
		} else {
			// Active mode: use normal rendering with TODO placeholders for required fields
			r.renderer.RenderFields(itemBuilder, itemSchema, itemMap, 1)
		}

		sequenceNode.Content = append(sequenceNode.Content, itemMappingNode)
	}

	b.current.Content = append(b.current.Content, keyNode, sequenceNode)
}

// renderMapEntries generates example key-value pairs for a map based on value schema.
func (r *NestedTypeRenderer) renderMapEntries(b *YAMLBuilder, valueSchema *extv1.JSONSchemaProps) {
	if valueSchema == nil {
		b.AddField("key1", "example")
		b.AddField("key2", "example")
		return
	}

	// Check if value is a custom type (object with properties)
	if isMapOfCustomType(valueSchema) {
		// Render nested objects for each map key with example values
		for _, key := range []string{"key1", "key2"} {
			b.InMapping(key, func(innerB *YAMLBuilder) {
				r.renderExampleFields(innerB, valueSchema)
			})
		}
		return
	}

	// Primitive value types
	exampleVal := getExampleValueForPrimitiveType(valueSchema)
	b.AddField("key1", exampleVal)
	b.AddField("key2", exampleVal)
}

// renderExampleFields renders all fields of an object schema with example values.
// This is used for nested objects in array/map contexts where we want examples, not TODO placeholders.
func (r *NestedTypeRenderer) renderExampleFields(b *YAMLBuilder, schema *extv1.JSONSchemaProps) {
	if schema == nil || len(schema.Properties) == 0 {
		return
	}

	// Get sorted property names (alphabetical order for examples)
	propNames := sortedPropertyNames(schema.Properties)

	for _, propName := range propNames {
		prop := schema.Properties[propName]
		exampleValue := generateExampleValueForType(&prop)
		b.AddField(propName, formatDefaultValue(exampleValue))
	}
}
