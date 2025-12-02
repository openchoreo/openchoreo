// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package component

import (
	"slices"
	"sort"

	extv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
)

// allChildrenOptional checks if all children of an object schema are optional.
// A child is considered optional if it's either:
// - Not in the required list, OR
// - In the required list but has a default value
func allChildrenOptional(schema *extv1.JSONSchemaProps) bool {
	for name, prop := range schema.Properties {
		if slices.Contains(schema.Required, name) {
			if prop.Default == nil {
				return false // Required without default
			}
		}
	}
	return true
}

// generateExampleValueForType creates an example value based on the schema type.
func generateExampleValueForType(schema *extv1.JSONSchemaProps) any {
	switch schema.Type {
	case typeString:
		return "example"
	case typeInteger:
		return 0
	case typeNumber:
		return 0.0
	case typeBoolean:
		return false
	case typeArray:
		if schema.Items != nil && schema.Items.Schema != nil {
			itemSchema := schema.Items.Schema
			if itemSchema.Type == typeObject && len(itemSchema.Properties) > 0 {
				obj := make(map[string]any)
				for propName, prop := range itemSchema.Properties {
					obj[propName] = generateExampleValueForType(&prop)
				}
				return []any{obj, obj}
			}
		}
		return generateExamplePrimitiveArrayForType(schema)
	case typeObject:
		if len(schema.Properties) > 0 {
			result := make(map[string]any)
			for propName, prop := range schema.Properties {
				result[propName] = generateExampleValueForType(&prop)
			}
			return result
		}
		return map[string]any{}
	default:
		return "example"
	}
}

// generateExamplePrimitiveArrayForType creates example values for arrays of primitives.
func generateExamplePrimitiveArrayForType(arraySchema *extv1.JSONSchemaProps) []any {
	if arraySchema.Items == nil || arraySchema.Items.Schema == nil {
		return []any{"example", "example"}
	}

	itemSchema := arraySchema.Items.Schema

	switch itemSchema.Type {
	case typeString:
		return []any{"example", "example"}
	case typeInteger:
		return []any{0, 0}
	case typeNumber:
		return []any{0.0, 0.0}
	case typeBoolean:
		return []any{false, false}
	default:
		return []any{"example", "example"}
	}
}

// getExampleValueForPrimitiveType returns an example value for a primitive type as a string.
func getExampleValueForPrimitiveType(schema *extv1.JSONSchemaProps) string {
	switch schema.Type {
	case typeString:
		return exampleValue
	case typeInteger:
		return "0"
	case typeBoolean:
		return "false"
	case typeNumber:
		return "0.0"
	default:
		return exampleValue
	}
}

// isMapOfCustomType checks if a schema represents map<CustomType>.
func isMapOfCustomType(schema *extv1.JSONSchemaProps) bool {
	if schema == nil {
		return false
	}
	return schema.Type == typeObject && len(schema.Properties) > 0
}

// isMapOfArray checks if a schema represents map<[]T>.
func isMapOfArray(schema *extv1.JSONSchemaProps) bool {
	if schema == nil {
		return false
	}
	return schema.Type == typeArray
}

// isMapOfPrimitiveArray checks if a schema represents map<[]primitive>.
func isMapOfPrimitiveArray(schema *extv1.JSONSchemaProps) bool {
	if !isMapOfArray(schema) {
		return false
	}
	itemSchema := schema.Items.Schema
	if itemSchema == nil {
		return true // No item schema = primitive
	}
	// Primitive if not an object with properties
	return !(itemSchema.Type == typeObject && len(itemSchema.Properties) > 0)
}

// isMapOfCustomTypeArray checks if a schema represents map<[]CustomType>.
func isMapOfCustomTypeArray(schema *extv1.JSONSchemaProps) bool {
	if !isMapOfArray(schema) {
		return false
	}
	itemSchema := schema.Items.Schema
	if itemSchema == nil {
		return false
	}
	return itemSchema.Type == typeObject && len(itemSchema.Properties) > 0
}

// isArrayOfMaps checks if a schema represents []map<T>.
func isArrayOfMaps(schema *extv1.JSONSchemaProps) bool {
	if schema == nil {
		return false
	}
	return schema.Type == typeObject && schema.AdditionalProperties != nil
}

// isArrayOfCustomType checks if a schema represents []CustomType.
func isArrayOfCustomType(schema *extv1.JSONSchemaProps) bool {
	if schema == nil {
		return false
	}
	return schema.Type == typeObject && len(schema.Properties) > 0
}

// sortedPropertyNames returns property names sorted alphabetically.
func sortedPropertyNames(properties map[string]extv1.JSONSchemaProps) []string {
	names := make([]string, 0, len(properties))
	for name := range properties {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
