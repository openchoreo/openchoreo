// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package schema

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	apiext "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions"
	extv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apiextschema "k8s.io/apiextensions-apiserver/pkg/apiserver/schema"
	"k8s.io/apiextensions-apiserver/pkg/apiserver/schema/defaulting"
	"k8s.io/apiextensions-apiserver/pkg/apiserver/validation"

	"github.com/openchoreo/openchoreo/api/v1alpha1"
)

const (
	typeString  = "string"
	typeInteger = "integer"
	typeObject  = "object"
	typeArray   = "array"
)

// ResolveSectionToStructural converts a SchemaSection into a Kubernetes structural schema.
// Returns nil if the section is nil or empty.
func ResolveSectionToStructural(section *v1alpha1.SchemaSection) (*apiextschema.Structural, error) {
	fields, err := unmarshalSectionSchema(section)
	if err != nil {
		return nil, err
	}
	if fields == nil {
		return nil, nil
	}
	return OpenAPIV3ToStructural(fields)
}

// ResolveSectionToBundle converts a SchemaSection into both structural and JSON schema formats.
// Returns nil for both if the section is nil or empty.
func ResolveSectionToBundle(section *v1alpha1.SchemaSection) (*apiextschema.Structural, *extv1.JSONSchemaProps, error) {
	fields, err := unmarshalSectionSchema(section)
	if err != nil {
		return nil, nil, err
	}
	if fields == nil {
		return nil, nil, nil
	}
	return OpenAPIV3ToStructuralAndJSONSchema(fields)
}

// SectionToJSONSchema converts a SchemaSection to JSON Schema for API responses.
func SectionToJSONSchema(section *v1alpha1.SchemaSection) (*extv1.JSONSchemaProps, error) {
	fields, err := unmarshalSectionSchema(section)
	if err != nil {
		return nil, err
	}
	if fields == nil {
		return &extv1.JSONSchemaProps{
			Type:       "object",
			Properties: map[string]extv1.JSONSchemaProps{},
		}, nil
	}
	return OpenAPIV3ToJSONSchema(fields)
}

// SectionToRawJSONSchema converts a SchemaSection to a raw map for API responses.
// This preserves vendor extensions (x-*) that are lost when converting through extv1.JSONSchemaProps.
func SectionToRawJSONSchema(section *v1alpha1.SchemaSection) (map[string]any, error) {
	fields, err := unmarshalSectionSchema(section)
	if err != nil {
		return nil, err
	}
	if fields == nil {
		return map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		}, nil
	}
	return OpenAPIV3ToResolvedSchema(fields)
}

// unmarshalSectionSchema extracts and unmarshals the OpenAPI v3 schema from a SchemaSection.
// Returns (nil, nil) if the section is nil or has no schema content.
func unmarshalSectionSchema(section *v1alpha1.SchemaSection) (map[string]any, error) {
	if section == nil || section.OpenAPIV3Schema == nil || len(section.OpenAPIV3Schema.Raw) == 0 {
		return nil, nil
	}
	var fields map[string]any
	if err := json.Unmarshal(section.OpenAPIV3Schema.Raw, &fields); err != nil {
		return nil, fmt.Errorf("failed to parse schema: %w", err)
	}
	return fields, nil
}

// ApplyDefaults applies schema default values to a target object using Kubernetes defaulting logic.
//
// This function walks the structural schema and target object in parallel, filling in default
// values for missing fields. The defaulting algorithm follows the same rules as Kubernetes
// API server defaulting for CustomResourceDefinitions.
//
// Behavior:
//   - Missing fields with defaults are added
//   - Existing fields are not overwritten (even if they differ from the default)
//   - Nested objects are defaulted recursively
//
// The target map is modified in place and also returned for convenience.
//
// Example:
//
//	Schema: {replicas: {type: "integer", default: 1}, image: {type: "string"}}
//	Input:  {image: "nginx"}
//	Output: {image: "nginx", replicas: 1}
func ApplyDefaults(target map[string]any, structural *apiextschema.Structural) map[string]any {
	if structural == nil {
		return target
	}
	if target == nil {
		target = map[string]any{}
	}
	defaulting.Default(target, structural)
	return target
}

// sortRequiredFields recursively sorts the 'required' arrays in a JSON schema.
//
// This ensures deterministic output across multiple runs, which is important for:
//  1. Minimizing git diffs when schemas change
//  2. CLI/UI schema generators that rely on consistent ordering
//  3. Testing (comparing schemas requires consistent output)
//
// The function traverses the entire schema tree, sorting required fields in:
//   - Top-level objects
//   - Nested object properties
//   - Array item schemas
//   - Additional properties schemas
//
// Note: This modifies the schema in place, including nested properties.
func sortRequiredFields(schema *extv1.JSONSchemaProps) {
	if schema == nil {
		return
	}
	if len(schema.Required) > 0 {
		// Keep output deterministic for CLI/UI generators and to minimize diffs when definitions change.
		sort.Strings(schema.Required)
	}
	if schema.Properties != nil {
		for key := range schema.Properties {
			prop := schema.Properties[key]
			sortRequiredFields(&prop)
			schema.Properties[key] = prop
		}
	}
	if schema.Items != nil && schema.Items.Schema != nil {
		sortRequiredFields(schema.Items.Schema)
	}
	if schema.AdditionalProperties != nil && schema.AdditionalProperties.Schema != nil {
		sortRequiredFields(schema.AdditionalProperties.Schema)
	}
}

// ValidateWithJSONSchema validates values against a JSONSchemaProps using Kubernetes validation.
// This properly validates required fields, types, constraints, patterns, and all other JSON Schema validations.
func ValidateWithJSONSchema(values map[string]any, jsonSchema *extv1.JSONSchemaProps) error {
	if jsonSchema == nil {
		return fmt.Errorf("schema is nil")
	}

	// Convert v1 JSONSchemaProps to internal type for validator
	internalSchema := new(apiext.JSONSchemaProps)
	if err := extv1.Convert_v1_JSONSchemaProps_To_apiextensions_JSONSchemaProps(jsonSchema, internalSchema, nil); err != nil {
		return fmt.Errorf("failed to convert schema: %w", err)
	}

	// Create Kubernetes schema validator
	validator, _, err := validation.NewSchemaValidator(internalSchema)
	if err != nil {
		return fmt.Errorf("failed to create schema validator: %w", err)
	}

	// Validate the values
	result := validator.Validate(values)
	if !result.IsValid() {
		// Collect all validation errors, removing "in body" which is HTTP API terminology
		var errMsgs []string
		for _, err := range result.Errors {
			msg := strings.ReplaceAll(err.Error(), " in body", "")
			errMsgs = append(errMsgs, msg)
		}
		return fmt.Errorf("%s", strings.Join(errMsgs, "; "))
	}

	return nil
}
