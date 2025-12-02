// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package component

import (
	"fmt"
	"strings"

	apiext "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions"
	extv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apiextschema "k8s.io/apiextensions-apiserver/pkg/apiserver/schema"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/openchoreo/openchoreo/internal/schema"
)

// schemaProcessingResult holds the results of schema processing.
type schemaProcessingResult struct {
	jsonSchema   *extv1.JSONSchemaProps
	structural   *apiextschema.Structural
	defaultedObj map[string]any
}

// extractAndConvertSchema converts OpenChoreo schema (Parameters + Types) to JSONSchemaProps.
// This is a convenience function for extracting and converting schemas without applying defaults.
func extractAndConvertSchema(parameters, types *runtime.RawExtension) (*extv1.JSONSchemaProps, error) {
	def := schema.Definition{
		Schemas: []map[string]any{},
		Types:   nil,
	}

	if parameters != nil {
		paramsMap, err := rawExtensionToMap(parameters)
		if err != nil {
			return nil, fmt.Errorf("converting parameters: %w", err)
		}
		def.Schemas = append(def.Schemas, paramsMap)
	}

	if types != nil {
		typesMap, err := rawExtensionToMap(types)
		if err != nil {
			return nil, fmt.Errorf("converting types: %w", err)
		}
		def.Types = typesMap
	}

	jsonSchema, err := schema.ToJSONSchema(def)
	if err != nil {
		return nil, fmt.Errorf("converting to JSON schema: %w", err)
	}

	return jsonSchema, nil
}

// applyDefaultsToSchema takes a JSONSchemaProps and applies defaults to create a schemaProcessingResult.
// This is used when the schema has already been converted from OpenChoreo format.
func applyDefaultsToSchema(jsonSchema *extv1.JSONSchemaProps) (*schemaProcessingResult, error) {
	if jsonSchema == nil {
		return &schemaProcessingResult{
			jsonSchema:   nil,
			structural:   nil,
			defaultedObj: make(map[string]any),
		}, nil
	}

	structural, err := schemaToStructural(jsonSchema)
	if err != nil {
		return nil, fmt.Errorf("converting to structural schema: %w", err)
	}

	emptyObj := buildEmptyStructure(jsonSchema)
	defaultedObj := schema.ApplyDefaults(emptyObj, structural)

	return &schemaProcessingResult{
		jsonSchema:   jsonSchema,
		structural:   structural,
		defaultedObj: defaultedObj,
	}, nil
}

// schemaToStructural converts JSONSchemaProps to Structural schema.
func schemaToStructural(jsonSchema *extv1.JSONSchemaProps) (*apiextschema.Structural, error) {
	// Convert v1 JSONSchemaProps to internal type
	internalSchema := new(apiext.JSONSchemaProps)
	if err := extv1.Convert_v1_JSONSchemaProps_To_apiextensions_JSONSchemaProps(jsonSchema, internalSchema, nil); err != nil {
		return nil, fmt.Errorf("converting schema to internal type: %w", err)
	}

	structural, err := apiextschema.NewStructural(internalSchema)
	if err != nil {
		return nil, fmt.Errorf("creating structural schema: %w", err)
	}

	// Validate structural schema
	if errs := apiextschema.ValidateStructural(nil, structural); len(errs) > 0 {
		return nil, fmt.Errorf("invalid structural schema: %v", errs)
	}

	return structural, nil
}

// buildEmptyStructure creates an object with empty structures for objects without defaults.
func buildEmptyStructure(jsonSchema *extv1.JSONSchemaProps) map[string]any {
	if jsonSchema == nil || jsonSchema.Properties == nil {
		return make(map[string]any)
	}

	result := make(map[string]any)

	for name, prop := range jsonSchema.Properties {
		if prop.Type == typeObject && len(prop.Properties) > 0 {
			// Object without default needs empty structure for child defaults to apply
			if prop.Default == nil || isEmptyDefault(prop.Default) {
				result[name] = buildEmptyStructure(&prop)
			}
			// Objects WITH non-empty default: ApplyDefaults will fill them
		} else if prop.Type == typeArray {
			// Arrays without defaults need at least one item for scaffolding
			if prop.Default == nil || isEmptyDefault(prop.Default) {
				result[name] = buildEmptyArrayStructure(&prop)
			}
			// Arrays WITH defaults: ApplyDefaults will fill them
		}
		// Primitives: ApplyDefaults will fill if they have defaults
	}

	return result
}

// buildEmptyArrayStructure creates an array with example items based on array item schema.
// For arrays of objects, creates empty object structures. For primitives, returns empty array.
func buildEmptyArrayStructure(arrayProp *extv1.JSONSchemaProps) []any {
	if arrayProp.Items == nil || arrayProp.Items.Schema == nil {
		return []any{}
	}

	itemSchema := arrayProp.Items.Schema

	// For array of objects, create empty structures for each item
	if itemSchema.Type == typeObject && len(itemSchema.Properties) > 0 {
		// Create 2 example items
		item1 := buildEmptyStructure(itemSchema)
		item2 := buildEmptyStructure(itemSchema)
		return []any{item1, item2}
	}

	// For primitives and other types, ApplyDefaults will handle if there are defaults
	// Return empty array - will be populated differently during generation
	return []any{}
}

// isEmptyDefault checks if a JSON default value is empty (nil, {}, or null).
func isEmptyDefault(def *extv1.JSON) bool {
	if def == nil || len(def.Raw) == 0 {
		return true
	}
	// Check if it's empty object {}
	str := strings.TrimSpace(string(def.Raw))
	return str == "{}" || str == "null"
}
