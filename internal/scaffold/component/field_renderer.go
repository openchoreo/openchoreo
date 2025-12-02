// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package component

import (
	"slices"
	"sort"

	extv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
)

// separatorComment is used to visually separate required fields from optional defaults.
const separatorComment = "\nDefaults: Uncomment to customize"

// JSON Schema type constants.
const (
	typeString  = "string"
	typeInteger = "integer"
	typeNumber  = "number"
	typeBoolean = "boolean"
	typeObject  = "object"
	typeArray   = "array"
)

// exampleValue is a placeholder value for string fields.
const exampleValue = "example"

// FieldRenderer handles YAML field rendering from JSON Schema definitions.
// It isolates tree-walking/rendering rules from spec/trait/workflow assembly.
//
// Supported field types:
//   - Primitives: string, integer, number, boolean (with enum support)
//   - Arrays: []T, []CustomType, []map<T>, []map<CustomType>
//   - Maps: map<T>, map<CustomType>, map<[]T>, map<[]CustomType>
//   - Objects: inline nested objects and custom type references
//
// Options:
//   - includeFieldDescriptions: adds schema descriptions and enum alternatives as comments
//   - includeAllFields: includes optional fields without defaults (normally omitted)
type FieldRenderer struct {
	includeFieldDescriptions  bool
	includeAllFields          bool
	includeStructuralComments bool
	dispatcher                *StrategyDispatcher
}

// NewFieldRenderer creates a new FieldRenderer with the given options.
func NewFieldRenderer(includeFieldDescriptions, includeAllFields, includeStructuralComments bool) *FieldRenderer {
	renderer := &FieldRenderer{
		includeFieldDescriptions:  includeFieldDescriptions,
		includeAllFields:          includeAllFields,
		includeStructuralComments: includeStructuralComments,
	}
	// Initialize dispatcher with strategies
	renderer.dispatcher = NewStrategyDispatcher(renderer)
	return renderer
}

// RenderFields generates fields from schema and defaulted object.
// Fields are sorted with required fields first, then optional fields, with a separator comment.
func (r *FieldRenderer) RenderFields(b *YAMLBuilder, schema *extv1.JSONSchemaProps, defaultedObj map[string]any, depth int) {
	if schema == nil || schema.Properties == nil {
		return
	}

	// Get sorted field names: required first, then optional, each group sorted by type then alphabetically
	fieldNames := r.getSortedFieldNamesWithRequired(schema)

	// Track if we need to add a separator between required and optional fields
	hasEmittedRequired := false
	separatorAdded := false

	for _, name := range fieldNames {
		prop := schema.Properties[name]
		value := defaultedObj[name]
		isRequired := slices.Contains(schema.Required, name)
		hasSchemaDefault := prop.Default != nil

		// Determine if this field will be commented (optional)
		willBeCommented := !isRequired || hasSchemaDefault

		// Add separator before the first commented field (if we had required fields)
		addSeparator := willBeCommented && hasEmittedRequired && !separatorAdded
		if addSeparator {
			separatorAdded = true
		}

		if !willBeCommented {
			hasEmittedRequired = true
		}

		// Create field context
		ctx := &FieldContext{
			Name:         name,
			Schema:       &prop,
			Value:        value,
			IsRequired:   isRequired,
			HasDefault:   hasSchemaDefault,
			ParentSchema: schema,
			AddSeparator: addSeparator,
			Depth:        depth,
			Renderer:     r,
		}

		// Dispatch to appropriate strategy
		r.dispatcher.Dispatch(b, ctx)
	}
}

// getSortedFieldNames returns field names sorted by type then alphabetically.
func (r *FieldRenderer) getSortedFieldNames(schema *extv1.JSONSchemaProps) []string {
	names := make([]string, 0, len(schema.Properties))
	for name := range schema.Properties {
		names = append(names, name)
	}

	sort.Slice(names, func(i, j int) bool {
		propI := schema.Properties[names[i]]
		propJ := schema.Properties[names[j]]

		orderI := GetFieldTypeOrder(propI)
		orderJ := GetFieldTypeOrder(propJ)

		if orderI != orderJ {
			return orderI < orderJ
		}

		return names[i] < names[j]
	})

	return names
}

// getSortedFieldNamesWithRequired returns field names sorted with required fields first,
// then optional fields. Within each group, fields are sorted by type then alphabetically.
func (r *FieldRenderer) getSortedFieldNamesWithRequired(schema *extv1.JSONSchemaProps) []string {
	names := make([]string, 0, len(schema.Properties))
	for name := range schema.Properties {
		names = append(names, name)
	}

	requiredSet := make(map[string]bool)
	for _, req := range schema.Required {
		requiredSet[req] = true
	}

	sort.Slice(names, func(i, j int) bool {
		propI := schema.Properties[names[i]]
		propJ := schema.Properties[names[j]]

		// Required fields without defaults come first
		reqI := requiredSet[names[i]] && propI.Default == nil
		reqJ := requiredSet[names[j]] && propJ.Default == nil

		if reqI != reqJ {
			return reqI // required without default comes first
		}

		// Within same required status, sort by type order
		orderI := GetFieldTypeOrder(propI)
		orderJ := GetFieldTypeOrder(propJ)

		if orderI != orderJ {
			return orderI < orderJ
		}

		// Finally, alphabetically
		return names[i] < names[j]
	})

	return names
}
