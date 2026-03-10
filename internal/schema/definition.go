// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package schema

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
	apiext "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions"
	extv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apiextschema "k8s.io/apiextensions-apiserver/pkg/apiserver/schema"
	"k8s.io/apiextensions-apiserver/pkg/apiserver/schema/defaulting"
	"k8s.io/apiextensions-apiserver/pkg/apiserver/validation"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/clone"
	"github.com/openchoreo/openchoreo/internal/schema/extractor"
)

const (
	typeString  = "string"
	typeInteger = "integer"
	typeObject  = "object"
)

// Definition represents a schematized object assembled from one or more field maps.
type Definition struct {
	Schemas []map[string]any
	Options extractor.Options
}

// ResolveSectionToStructural converts a SchemaSection into a Kubernetes structural schema.
// It handles both ocSchema and openAPIV3Schema formats transparently.
// Returns nil if the section is nil or empty.
func ResolveSectionToStructural(section *v1alpha1.SchemaSection) (*apiextschema.Structural, error) {
	raw := sectionRaw(section)
	if raw == nil || len(raw.Raw) == 0 {
		return nil, nil
	}

	fields, err := unmarshalSection(raw)
	if err != nil {
		return nil, err
	}

	if section.IsOpenAPIV3() {
		return OpenAPIV3ToStructural(fields)
	}

	return ToStructural(Definition{Schemas: []map[string]any{fields}})
}

// ResolveSectionToBundle converts a SchemaSection into both structural and JSON schema formats.
// Returns nil for both if the section is nil or empty.
func ResolveSectionToBundle(section *v1alpha1.SchemaSection) (*apiextschema.Structural, *extv1.JSONSchemaProps, error) {
	raw := sectionRaw(section)
	if raw == nil || len(raw.Raw) == 0 {
		return nil, nil, nil
	}

	fields, err := unmarshalSection(raw)
	if err != nil {
		return nil, nil, err
	}

	if section.IsOpenAPIV3() {
		return OpenAPIV3ToStructuralAndJSONSchema(fields)
	}

	return ToStructuralAndJSONSchema(Definition{Schemas: []map[string]any{fields}})
}

// SectionToJSONSchema converts a SchemaSection to JSON Schema for API responses.
// Handles both ocSchema (via extractor) and openAPIV3Schema (via ref resolution).
func SectionToJSONSchema(section *v1alpha1.SchemaSection) (*extv1.JSONSchemaProps, error) {
	raw := sectionRaw(section)
	if raw == nil || len(raw.Raw) == 0 {
		return &extv1.JSONSchemaProps{
			Type:       "object",
			Properties: map[string]extv1.JSONSchemaProps{},
		}, nil
	}

	fields, err := unmarshalSection(raw)
	if err != nil {
		return nil, err
	}

	if section.IsOpenAPIV3() {
		return OpenAPIV3ToJSONSchema(fields)
	}

	return ToJSONSchema(Definition{Schemas: []map[string]any{fields}})
}

// SectionToRawJSONSchema converts a SchemaSection to a raw map for API responses.
// For openAPIV3Schema, this preserves vendor extensions (x-*) that are lost when
// converting through extv1.JSONSchemaProps. For ocSchema, it falls back to
// ToJSONSchema and re-marshals the result.
func SectionToRawJSONSchema(section *v1alpha1.SchemaSection) (map[string]any, error) {
	raw := sectionRaw(section)
	if raw == nil || len(raw.Raw) == 0 {
		return map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		}, nil
	}

	fields, err := unmarshalSection(raw)
	if err != nil {
		return nil, err
	}

	if section.IsOpenAPIV3() {
		return OpenAPIV3ToResolvedSchema(fields)
	}

	// For ocSchema, convert through the extractor and re-marshal to map
	jsonSchema, err := ToJSONSchema(Definition{Schemas: []map[string]any{fields}})
	if err != nil {
		return nil, err
	}

	return jsonSchemaToMap(jsonSchema)
}

// jsonSchemaToMap converts extv1.JSONSchemaProps to a raw map via JSON round-trip.
func jsonSchemaToMap(schema *extv1.JSONSchemaProps) (map[string]any, error) {
	data, err := json.Marshal(schema)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal schema: %w", err)
	}
	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal schema to map: %w", err)
	}
	return result, nil
}

// sectionRaw returns the raw extension from a SchemaSection, or nil if empty.
func sectionRaw(section *v1alpha1.SchemaSection) *runtime.RawExtension {
	if section == nil {
		return nil
	}
	return section.GetRaw()
}

// unmarshalSection unmarshals a raw extension into a field map.
func unmarshalSection(raw *runtime.RawExtension) (map[string]any, error) {
	var fields map[string]any
	if err := yaml.Unmarshal(raw.Raw, &fields); err != nil {
		return nil, fmt.Errorf("failed to parse schema: %w", err)
	}
	return fields, nil
}

// ToJSONSchema converts a schema definition into an OpenAPI v3 JSON schema.
//
// This is the primary entry point for converting ComponentType and Trait schemas
// from the shorthand format into standard JSON Schema that can be used for validation.
//
// Process:
//  1. Merge all schema maps (parameters, environmentConfigs, trait config) into one
//  2. Convert from shorthand syntax to full JSON Schema via extractor (internal type)
//  3. Convert from internal to v1 type (for API compatibility and JSON serialization)
//  4. Sort required fields for deterministic output
//
// Example input (shorthand):
//
//	schemas: [{replicas: "integer | default=1"}, {environment: "string"}]
//
// Example output (JSON Schema):
//
//	{type: "object", properties: {replicas: {type: "integer", default: 1}, environment: {type: "string"}}}
func ToJSONSchema(def Definition) (*extv1.JSONSchemaProps, error) {
	merged := mergeFieldMaps(def.Schemas)
	if len(merged) == 0 {
		return &extv1.JSONSchemaProps{
			Type:       "object",
			Properties: map[string]extv1.JSONSchemaProps{},
		}, nil
	}

	internalSchema, err := extractor.ExtractSchema(merged, nil, def.Options)
	if err != nil {
		return nil, fmt.Errorf("failed to convert schema to OpenAPI: %w", err)
	}

	// Convert from internal to v1 type for API responses (v1 has JSON tags for proper serialization)
	v1Schema := new(extv1.JSONSchemaProps)
	if err := extv1.Convert_apiextensions_JSONSchemaProps_To_v1_JSONSchemaProps(internalSchema, v1Schema, nil); err != nil {
		return nil, fmt.Errorf("failed to convert schema to v1: %w", err)
	}

	sortRequiredFields(v1Schema)
	return v1Schema, nil
}

// ToStructural converts the definition into a Kubernetes structural schema.
//
// Structural schemas are a stricter variant of JSON Schema used by Kubernetes for:
//  1. Server-side validation of CRD instances
//  2. Defaulting field values based on schema defaults
//  3. Pruning unknown fields during admission
//
// The conversion process:
//  1. Extract schema using shorthand syntax (returns internal type)
//  2. Validate and convert to structural format (enforces additional constraints)
//
// Structural schema constraints include:
//   - All objects must have explicit type: "object"
//   - All arrays must specify item schema
//   - No x-kubernetes-* extensions in certain contexts
//
// This is primarily used with ApplyDefaults to populate default values.
func ToStructural(def Definition) (*apiextschema.Structural, error) {
	merged := mergeFieldMaps(def.Schemas)
	if len(merged) == 0 {
		emptySchema := &apiext.JSONSchemaProps{
			Type:       "object",
			Properties: map[string]apiext.JSONSchemaProps{},
		}
		structural, err := apiextschema.NewStructural(emptySchema)
		if err != nil {
			return nil, fmt.Errorf("failed to build structural schema: %w", err)
		}
		return structural, nil
	}

	internalSchema, err := extractor.ExtractSchema(merged, nil, def.Options)
	if err != nil {
		return nil, fmt.Errorf("failed to convert schema to OpenAPI: %w", err)
	}

	structural, err := apiextschema.NewStructural(internalSchema)
	if err != nil {
		return nil, fmt.Errorf("failed to build structural schema: %w", err)
	}
	return structural, nil
}

// ToStructuralAndJSONSchema converts a schema definition to both structural and JSON schema formats.
// Use this when you need both formats, e.g., for applying defaults (structural) then validating (JSON schema).
func ToStructuralAndJSONSchema(def Definition) (*apiextschema.Structural, *extv1.JSONSchemaProps, error) {
	merged := mergeFieldMaps(def.Schemas)

	var internalSchema *apiext.JSONSchemaProps
	if len(merged) == 0 {
		internalSchema = &apiext.JSONSchemaProps{
			Type:       "object",
			Properties: map[string]apiext.JSONSchemaProps{},
		}
	} else {
		var err error
		internalSchema, err = extractor.ExtractSchema(merged, nil, def.Options)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to convert schema to OpenAPI: %w", err)
		}
	}

	structural, err := apiextschema.NewStructural(internalSchema)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to build structural schema: %w", err)
	}

	v1Schema := new(extv1.JSONSchemaProps)
	if err := extv1.Convert_apiextensions_JSONSchemaProps_To_v1_JSONSchemaProps(internalSchema, v1Schema, nil); err != nil {
		return nil, nil, fmt.Errorf("failed to convert schema to v1: %w", err)
	}

	return structural, v1Schema, nil
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

// mergeFieldMaps combines multiple schema maps into a single unified schema.
//
// ComponentType separate schemas into logical groups:
//   - schema.parameters: Component-level configuration
//   - schema.environmentConfigs: Environment-specific overrides
//   - schema.traitConfig: Trait instance configuration (for Traits)
//
// This function merges them using deep merge semantics so that:
//  1. Nested objects are merged recursively
//  2. Later schemas can extend or override earlier ones
//  3. The result matches how templates access these values (all under ${spec.*})
//
// Example:
//
//	Input: [{port: "integer"}, {replicas: "integer"}]
//	Output: {port: "integer", replicas: "integer"}
func mergeFieldMaps(maps []map[string]any) map[string]any {
	result := map[string]any{}
	for _, fields := range maps {
		mergeInto(result, fields)
	}
	return result
}

// mergeInto recursively merges src into dst, modifying dst in place.
//
// Merge behavior:
//   - If src value is a map and dst has a map at that key: recursively merge
//   - Otherwise: overwrite dst's value with a deep copy of src's value
//   - All values are deep copied to avoid shared references
//
// This ensures that nested object schemas are properly combined rather than replaced.
//
// Example:
//
//	dst: {db: {host: "string"}}
//	src: {db: {port: "integer"}, replicas: "integer"}
//	Result: {db: {host: "string", port: "integer"}, replicas: "integer"}
func mergeInto(dst map[string]any, src map[string]any) {
	if src == nil {
		return
	}
	if dst == nil {
		// should not happen, but guard anyway
		return
	}
	for k, v := range src {
		if vMap, ok := v.(map[string]any); ok {
			existing, ok := dst[k].(map[string]any)
			if !ok {
				dst[k] = clone.DeepCopy(vMap)
				continue
			}
			mergeInto(existing, vMap)
			continue
		}
		dst[k] = clone.DeepCopy(v)
	}
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
